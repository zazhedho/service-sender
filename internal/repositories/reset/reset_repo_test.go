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
		done <- serveTokenConsumeCommands(serverConn)
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

func serveTokenConsumeCommands(conn net.Conn) error {
	reader := bufio.NewReader(conn)
	used := false
	for {
		args, err := readRESPCommand(reader)
		if err != nil {
			return err
		}
		if strings.EqualFold(args[0], "hello") {
			fmt.Fprint(conn, "-ERR unknown command 'hello'\r\n")
			continue
		}
		if !strings.EqualFold(args[0], "getdel") || len(args) != 2 || args[1] != "reset:token:hash" {
			fmt.Fprint(conn, "-ERR expected atomic token consumption\r\n")
			return fmt.Errorf("unexpected command: %v", args)
		}
		if used {
			fmt.Fprint(conn, "$-1\r\n")
			return nil
		}
		fmt.Fprint(conn, "$16\r\nuser@example.com\r\n")
		used = true
	}
}

func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
	if err != nil {
		return nil, err
	}
	args := make([]string, count)
	for i := range args {
		if _, err = reader.ReadString('\n'); err != nil {
			return nil, err
		}
		line, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		args[i] = strings.TrimSuffix(line, "\r\n")
	}
	return args, nil
}
