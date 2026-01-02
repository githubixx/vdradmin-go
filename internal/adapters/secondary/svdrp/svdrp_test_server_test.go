package svdrp_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type svdrpConnStep struct {
	expect string
	// Raw SVDRP response lines to write (each should already start with a 3-digit code).
	// Each line will be terminated with CRLF.
	respond []string
	// If true, the server closes the connection after reading this command (without responding).
	closeAfterRead bool
}

type svdrpConnScript struct {
	welcome string
	steps   []svdrpConnStep
}

type svdrpTestServer struct {
	t      *testing.T
	ln     net.Listener
	host   string
	port   int
	scripts []svdrpConnScript

	mu        sync.Mutex
	accepted  int
	closed    bool
	closeOnce sync.Once
}

func newSVDRPTestServer(t *testing.T, scripts []svdrpConnScript) *svdrpTestServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	host, portStr, _ := strings.Cut(ln.Addr().String(), ":")
	port := 0
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	s := &svdrpTestServer{t: t, ln: ln, host: host, port: port, scripts: scripts}
	go s.acceptLoop()
	return s
}

func (s *svdrpTestServer) Addr() (string, int) { return s.host, s.port }

func (s *svdrpTestServer) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		_ = s.ln.Close()
	})
}

func (s *svdrpTestServer) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			s.t.Errorf("accept: %v", err)
			return
		}

		var script svdrpConnScript
		s.mu.Lock()
		idx := s.accepted
		s.accepted++
		if idx < len(s.scripts) {
			script = s.scripts[idx]
		}
		s.mu.Unlock()

		go s.handleConn(conn, idx, script)
	}
}

func (s *svdrpTestServer) handleConn(conn net.Conn, idx int, script svdrpConnScript) {
	defer func() { _ = conn.Close() }()

	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)

	welcome := script.welcome
	if strings.TrimSpace(welcome) == "" {
		welcome = "220 VDR SVDRP mock ready"
	}
	_, _ = w.WriteString(welcome + "\r\n")
	_ = w.Flush()

	for stepIndex, step := range script.steps {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := r.ReadString('\n')
		if err != nil {
			s.t.Errorf("conn %d step %d: read command: %v", idx, stepIndex, err)
			return
		}
		cmd := strings.TrimSpace(line)

		if step.expect != "" && cmd != step.expect {
			s.t.Errorf("conn %d step %d: expected %q, got %q", idx, stepIndex, step.expect, cmd)
			return
		}

		if step.closeAfterRead {
			return
		}

		for _, respLine := range step.respond {
			_, _ = w.WriteString(respLine + "\r\n")
		}
		_ = w.Flush()
	}

	// After the scripted interaction, allow the client to send QUIT during Close().
	for {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimSpace(line)
		if cmd == "QUIT" {
			return
		}
	}
}
