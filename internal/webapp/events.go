package webapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
)

type publication struct {
	id   uint64
	data []byte
}

// ReportAttempt publishes the latest sanitized session snapshot without
// blocking command execution. Slow browser subscribers skip stale snapshots
// and converge on the newest one.
func (s *Server) ReportAttempt(event game.AttemptEvent) {
	event = game.CloneAttemptEvent(event)
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	s.mu.Lock()
	s.sequence++
	item := publication{id: s.sequence, data: data}
	s.current = item
	s.hasCurrent = true
	for subscriber := range s.subscribers {
		select {
		case subscriber <- item:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- item:
			default:
			}
		}
	}
	s.mu.Unlock()
}

func (s *Server) handleEvents(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, supported := writer.(http.Flusher)
	if !supported {
		http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")

	subscriber := make(chan publication, 1)
	s.mu.Lock()
	s.subscribers[subscriber] = struct{}{}
	current, available := s.current, s.hasCurrent
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subscribers, subscriber)
		s.mu.Unlock()
	}()

	if available {
		writeSSE(writer, current)
	} else {
		fmt.Fprint(writer, "event: waiting\ndata: {}\n\n")
	}
	flusher.Flush()

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case item := <-subscriber:
			writeSSE(writer, item)
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprint(writer, ": keep-alive\n\n")
			flusher.Flush()
		case <-request.Context().Done():
			return
		case <-s.done:
			return
		}
	}
}

func writeSSE(writer http.ResponseWriter, item publication) {
	fmt.Fprintf(writer, "id: %d\nevent: snapshot\ndata: %s\n\n", item.id, item.data)
}
