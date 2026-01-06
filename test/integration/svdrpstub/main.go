package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

func main() {
	addr := getenv("SVDRP_ADDR", ":6419")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	defer ln.Close()

	log.Printf("svdrp stub listening on %s", addr)

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

		upper := strings.ToUpper(cmdline)
		fields := strings.Fields(upper)
		cmd := fields[0]

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
			// Timers for today.
			// We intentionally create:
			// - a 2-way overlap (collision only when dvb_cards=2)
			// - a 3-way overlap (critical when dvb_cards=2)
			// - a standalone OK timer
			today := time.Now().In(time.Local).Format("2006-01-02")
			writeMulti(w, 250, []string{
				fmt.Sprintf("1 1:C-1-1-10:%s:1000:1100:50:99:Collision A:", today),
				fmt.Sprintf("2 1:C-2-2-20:%s:1030:1130:50:99:Collision B:", today),
				fmt.Sprintf("3 1:C-3-3-30:%s:1200:1300:50:99:OK C:", today),
				fmt.Sprintf("4 1:C-1-1-10:%s:1400:1500:50:99:Critical A:", today),
				fmt.Sprintf("5 1:C-2-2-20:%s:1400:1500:50:99:Critical B:", today),
				fmt.Sprintf("6 1:C-3-3-30:%s:1400:1500:50:99:Critical C:", today),
			}, "0 timers")

		case "NEWT":
			id := nextTimerID.Add(1)
			writeLine(w, fmt.Sprintf("250 %d", id))

		case "MODT", "DELT", "CHAN", "HITK", "UPDR", "LSTR", "DELR":
			// Minimal success responses for commands the UI may trigger.
			// LSTR path lookup: return an empty path.
			if cmd == "LSTR" {
				writeMulti(w, 250, []string{""}, "0")
				break
			}
			writeLine(w, "250 OK")

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
