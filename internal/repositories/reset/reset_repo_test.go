package repositoryreset

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestConsumeTokenIsSingleUse(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(serverConn)
		used := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- err
				return
			}
			count, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
			if err != nil {
				done <- err
				return
			}
			args := make([]string, count)
			for i := range args {
				if _, err = reader.ReadString('\n'); err != nil {
					done <- err
					return
				}
				line, err = reader.ReadString('\n')
				if err != nil {
					done <- err
					return
				}
				args[i] = strings.TrimSuffix(line, "\r\n")
			}
			if strings.EqualFold(args[0], "hello") {
				fmt.Fprint(serverConn, "-ERR unknown command 'hello'\r\n")
				continue
			}
			if !strings.EqualFold(args[0], "getdel") || len(args) != 2 || args[1] != "reset:token:hash" {
				fmt.Fprint(serverConn, "-ERR expected atomic token consumption\r\n")
				done <- fmt.Errorf("unexpected command: %v", args)
				return
			}
			if used {
				fmt.Fprint(serverConn, "$-1\r\n")
				done <- nil
				return
			}
			fmt.Fprint(serverConn, "$16\r\nuser@example.com\r\n")
			used = true
		}
	}()
	client := redis.NewClient(&redis.Options{
		Protocol: 2, DisableIdentity: true, MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) { return clientConn, nil },
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	repo := NewPasswordResetRepository(client)
	email, err := repo.ConsumeToken(ctx, "hash")
	if err != nil || email != "user@example.com" {
		t.Fatalf("first consumption: %q, %v", email, err)
	}
	if _, err := repo.ConsumeToken(ctx, "hash"); err != redis.Nil {
		t.Fatalf("used token: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
