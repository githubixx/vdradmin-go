package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type timerEntry struct {
	ID  int64
	Str string
}

var timersMu sync.Mutex
var timers []timerEntry

func main() {
	addr := getenv("SVDRP_ADDR", ":6419")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	defer ln.Close()

	log.Printf("svdrp stub listening on %s", addr)

	initTimers()

	var nextTimerID atomic.Int64
	nextTimerID.Store(100)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handleConn(conn, &nextTimerID)
	}
}

func initTimers() {
	today := time.Now().In(time.Local).Format("2006-01-02")
	timersMu.Lock()
	defer timersMu.Unlock()
	// Timers for today.
	// We intentionally create:
	// - a 2-way overlap (collision only when dvb_cards=2)
	// - a 3-way overlap (critical when dvb_cards=2)
	// - a standalone OK timer
	timers = []timerEntry{
		{ID: 1, Str: fmt.Sprintf("1:C-1-1-10:%s:1000:1100:50:99:Collision A:", today)},
		{ID: 2, Str: fmt.Sprintf("1:C-2-2-20:%s:1030:1130:50:99:Collision B:", today)},
		{ID: 3, Str: fmt.Sprintf("1:C-3-3-30:%s:1200:1300:50:99:OK C:", today)},
		{ID: 4, Str: fmt.Sprintf("1:C-1-1-10:%s:1400:1500:50:99:Critical A:", today)},
		{ID: 5, Str: fmt.Sprintf("1:C-2-2-20:%s:1400:1500:50:99:Critical B:", today)},
		{ID: 6, Str: fmt.Sprintf("1:C-3-3-30:%s:1400:1500:50:99:Critical C:", today)},
	}
}

func handleConn(conn net.Conn, nextTimerID *atomic.Int64) {
	defer conn.Close()

	// Welcome.
	_, _ = conn.Write([]byte("220 svdrp-stub ready\r\n"))

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

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
		if len(fields) == 0 {
			continue
		}
		cmd := strings.ToUpper(fields[0])

		switch cmd {
		case "QUIT":
			writeLine(w, "221 bye")
			_ = w.Flush()
			return

		case "STAT":
			// Used by Ping as: "STAT disk".
			writeLine(w, "250 OK")

		case "LSTC":
			// Channels: a few channels on distinct transponders.
			// The client ignores a trailing "0 channels" line.
			writeMulti(w, 250, []string{
				"1 C-1-1-10 News:provider",
				"2 C-2-2-20 Sports:provider",
				"3 C-3-3-30 Movies:provider",
			}, "0 channels")

		case "LSTT":
			timersMu.Lock()
			out := make([]string, 0, len(timers))
			for _, tm := range timers {
				out = append(out, fmt.Sprintf("%d %s", tm.ID, tm.Str))
			}
			timersMu.Unlock()
			writeMulti(w, 250, out, "0 timers")

		case "NEWT":
			// NEWT <active:channel:day:start:stop:priority:lifetime:file:aux>
			arg := strings.TrimSpace(cmdline[len(fields[0]):])
			id := nextTimerID.Add(1)
			timersMu.Lock()
			timers = append(timers, timerEntry{ID: id, Str: arg})
			timersMu.Unlock()
			writeLine(w, fmt.Sprintf("250 %d", id))

		case "MODT":
			// MODT <id> <active:channel:day:start:stop:priority:lifetime:file:aux>
			if len(fields) < 3 {
				writeLine(w, "501 Missing argument")
				break
			}
			id, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				writeLine(w, "501 Invalid ID")
				break
			}
			idx := strings.Index(cmdline, fields[1])
			arg := ""
			if idx >= 0 {
				rest := strings.TrimSpace(cmdline[idx+len(fields[1]):])
				arg = strings.TrimSpace(rest)
			}
			timersMu.Lock()
			updated := false
			for i := range timers {
				if timers[i].ID == id {
					timers[i].Str = arg
					updated = true
					break
				}
			}
			timersMu.Unlock()
			if !updated {
				writeLine(w, "550 Timer not found")
				break
			}
			writeLine(w, "250 OK")

		case "DELT":
			// DELT <id>
			if len(fields) < 2 {
				writeLine(w, "501 Missing argument")
				break
			}
			id, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				writeLine(w, "501 Invalid ID")
				break
			}
			timersMu.Lock()
			before := len(timers)
			filtered := timers[:0]
			for _, tm := range timers {
				if tm.ID != id {
					filtered = append(filtered, tm)
				}
			}
			timers = filtered
			after := len(timers)
			timersMu.Unlock()
			if before == after {
				writeLine(w, "550 Timer not found")
				break
			}
			writeLine(w, "250 OK")

		case "LSTR":
			// Recordings list - minimal test data
			writeMulti(w, 250, []string{
				"1 02.02.26 20:15 0:45* Test Recording~News Special",
				"2 01.02.26 19:00 1:30 Another Recording~Documentary",
			}, "2 recordings")

		case "CHAN", "HITK", "UPDR", "DELR":
			// Minimal success responses for commands the UI may trigger.
			writeLine(w, "250 OK")
			// Single-line OK only.

		case "LSTE":
			// No EPG in this stub.
			writeLine(w, "550 No schedule")

		default:
			// Unknown command.
			writeLine(w, "500 Unknown")
		}

		_ = w.Flush()
	}
}

func writeMulti(w *bufio.Writer, code int, lines []string, final string) {
	for _, l := range lines {
		writeLine(w, fmt.Sprintf("%03d-%s", code, l))
	}
	writeLine(w, fmt.Sprintf("%03d %s", code, final))
}

func writeLine(w *bufio.Writer, line string) {
	_, _ = w.WriteString(line + "\r\n")
}

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}
