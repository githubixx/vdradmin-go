package svdrp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClient_GetChannels_RespectsContextDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)

		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		w := bufio.NewWriter(conn)
		_, _ = w.WriteString("220 svdrp-test ready\r\n")
		_ = w.Flush()

		r := bufio.NewReader(conn)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmdline := strings.TrimSpace(line)
			if cmdline == "" {
				continue
			}

			fields := strings.Fields(cmdline)
			cmd := strings.ToUpper(fields[0])
			switch cmd {
			case "LSTC":
				// Intentionally delay longer than the client's context deadline.
				time.Sleep(250 * time.Millisecond)
				_, _ = w.WriteString("250 0 channels\r\n")
				_ = w.Flush()
			case "QUIT":
				_, _ = w.WriteString("221 bye\r\n")
				_ = w.Flush()
				return
			default:
				_, _ = w.WriteString("500 Unknown\r\n")
				_ = w.Flush()
			}
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	client := NewClient("127.0.0.1", addr.Port, 5*time.Second)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = client.GetChannels(ctx)
	if err == nil {
		t.Fatal("expected context deadline exceeded, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %T: %v", err, err)
	}

	select {
	case <-serverDone:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("server goroutine did not exit")
	}
}

func TestClient_sendCommandLocked_ContextAlreadyDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := NewClient("127.0.0.1", 1, 1*time.Second)
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.sendCommandLocked(ctx, fmt.Sprintf("LSTC"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %T: %v", err, err)
	}
}
