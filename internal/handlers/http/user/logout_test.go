package handleruser

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"service-sender/infrastructure/database"
	interfaceuser "service-sender/internal/interfaces/user"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type logoutServiceStub struct {
	interfaceuser.ServiceUserInterface
	logout func(string) error
}

func (s *logoutServiceStub) LogoutUser(token string) error {
	return s.logout(token)
}

func TestLogoutBlacklistsBeforeRemovingRedisSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	events := make(chan string, 4)
	startLogoutRedis(t, events)
	handler := NewUserHandler(&logoutServiceStub{logout: func(token string) error {
		if token != "test-jwt" {
			t.Errorf("LogoutUser token = %q, want test-jwt", token)
		}
		events <- "blacklist"
		return nil
	}}, nil)

	response := runLogoutRequest(handler)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got, want := <-events, "blacklist"; got != want {
		t.Fatalf("first event = %q, want %q", got, want)
	}
	if got, want := <-events, "redis:get token_session:test-jwt"; got != want {
		t.Fatalf("second event = %q, want %q", got, want)
	}
}

func TestLogoutDoesNotRemoveRedisSessionWhenBlacklistFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	events := make(chan string, 4)
	startLogoutRedis(t, events)
	handler := NewUserHandler(&logoutServiceStub{logout: func(string) error {
		events <- "blacklist"
		return errors.New("database unavailable")
	}}, nil)

	response := runLogoutRequest(handler)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := <-events; got != "blacklist" {
		t.Fatalf("first event = %q, want blacklist", got)
	}
	select {
	case got := <-events:
		t.Fatalf("Redis session touched after blacklist failure: %q", got)
	default:
	}
}

func runLogoutRequest(handler *HandlerUser) *httptest.ResponseRecorder {
	engine := gin.New()
	engine.POST("/logout", func(ctx *gin.Context) {
		ctx.Set("token", "test-jwt")
		handler.Logout(ctx)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/logout", nil))
	return response
}

func startLogoutRedis(t *testing.T, events chan<- string) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	client := redis.NewClient(&redis.Options{
		Addr: "redis.test:6379", Protocol: 2, MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) { return clientConn, nil },
	})
	previous := database.RedisClient
	database.RedisClient = client
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer serverConn.Close()
		_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(serverConn)
		for {
			command, err := readRedisCommand(reader)
			if err != nil {
				return
			}
			switch strings.ToUpper(command[0]) {
			case "CLIENT":
				_, _ = fmt.Fprint(serverConn, "+OK\r\n")
			case "HELLO":
				_, _ = fmt.Fprint(serverConn, "-ERR unknown command 'HELLO'\r\n")
			case "GET":
				events <- "redis:" + strings.Join(command, " ")
				_, _ = fmt.Fprint(serverConn, "$-1\r\n")
				return
			default:
				_, _ = fmt.Fprint(serverConn, "-ERR unexpected command\r\n")
				return
			}
		}
	}()
	t.Cleanup(func() {
		database.RedisClient = previous
		_ = client.Close()
		_ = clientConn.Close()
		<-serverDone
	})
}

func readRedisCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 || line[0] != '*' {
		return nil, fmt.Errorf("invalid Redis command")
	}
	count, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}
	command := make([]string, count)
	for i := range command {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		length, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "$")))
		if err != nil || length < 0 {
			return nil, fmt.Errorf("invalid Redis argument")
		}
		value := make([]byte, length+2)
		if _, err := io.ReadFull(reader, value); err != nil {
			return nil, err
		}
		command[i] = string(value[:length])
	}
	return command, nil
}
