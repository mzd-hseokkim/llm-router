package proxy

import (
	"bufio"
	"io"
	"strings"
)

// SSEEvent represents a single dispatched Server-Sent Event.
type SSEEvent struct {
	Type string // "event:" value; defaults to "message" if absent
	Data string // concatenated "data:" lines joined by "\n"
	ID   string // "id:" value
}

// ParseSSE reads r and sends complete SSE events to the returned channel.
// The channel is closed when r is exhausted or returns an error.
// Events without any data lines are silently dropped (per SSE spec).
//
// The scanner uses a 1 MiB buffer to handle large JSON payloads in a single
// SSE data line.
func ParseSSE(r io.Reader) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)
	go func() {
		defer close(ch)

		scan := bufio.NewScanner(r)
		scan.Buffer(make([]byte, 64*1024), 1024*1024)

		var cur SSEEvent
		var dataLines []string

		for scan.Scan() {
			line := scan.Text()

			if line == "" {
				// Blank line = dispatch event (SSE spec §9.2.6)
				if len(dataLines) > 0 {
					cur.Data = strings.Join(dataLines, "\n")
					if cur.Type == "" {
						cur.Type = "message"
					}
					ch <- cur
				}
				cur = SSEEvent{}
				dataLines = dataLines[:0]
				continue
			}

			switch {
			case strings.HasPrefix(line, "data:"):
				v := line[5:]
				if len(v) > 0 && v[0] == ' ' {
					v = v[1:]
				}
				dataLines = append(dataLines, v)
			case strings.HasPrefix(line, "event:"):
				cur.Type = strings.TrimSpace(line[6:])
			case strings.HasPrefix(line, "id:"):
				cur.ID = strings.TrimSpace(line[3:])
			// "retry:" fields and comment lines (":...") are intentionally ignored.
			}
		}
	}()
	return ch
}
