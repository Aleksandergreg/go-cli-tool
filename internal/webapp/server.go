// Package webapp serves the local, read-only OpsQuest mission companion.
package webapp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
)

const companionCookie = "opsquest_companion"

//go:embed static/*
var staticFiles embed.FS

type publication struct {
	id   uint64
	data []byte
}

// Server is a loopback-only HTTP projection of the active game session.
// Command execution and validation never enter this package.
type Server struct {
	baseURL      string
	host         string
	pairToken    string
	sessionToken string
	httpServer   *http.Server
	done         chan struct{}
	closeOnce    sync.Once

	pairMu sync.Mutex
	paired bool

	mu          sync.Mutex
	sequence    uint64
	current     publication
	hasCurrent  bool
	subscribers map[chan publication]struct{}
}

// Start creates a companion bound to an ephemeral IPv4 loopback port. The
// returned URL performs a one-time pairing exchange and must be opened while
// the play command remains active.
func Start(ctx context.Context) (*Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on loopback: %w", err)
	}
	pairToken, err := randomToken()
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("create pairing token: %w", err)
	}
	sessionToken, err := randomToken()
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("create companion session: %w", err)
	}

	host := listener.Addr().String()
	server := &Server{
		baseURL:      "http://" + host,
		host:         host,
		pairToken:    pairToken,
		sessionToken: sessionToken,
		done:         make(chan struct{}),
		subscribers:  make(map[chan publication]struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/pair", server.handlePair)
	mux.HandleFunc("/api/state", server.requireSession(server.handleState))
	mux.HandleFunc("/api/events", server.requireSession(server.handleEvents))
	mux.HandleFunc("/app.css", server.requireSession(server.handleCSS))
	mux.HandleFunc("/app.js", server.requireSession(server.handleJavaScript))
	mux.HandleFunc("/", server.requireSession(server.handleIndex))
	server.httpServer = &http.Server{
		Handler:           server.validateRequest(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       75 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}

	go func() {
		_ = server.httpServer.Serve(listener)
	}()
	go func() {
		select {
		case <-ctx.Done():
			shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = server.Close(shutdownContext)
		case <-server.done:
		}
	}()
	return server, nil
}

func randomToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// URL returns the one-time local pairing URL.
func (s *Server) URL() string {
	return s.baseURL + "/pair?token=" + url.QueryEscape(s.pairToken)
}

// Close stops accepting companion requests and closes active event streams.
func (s *Server) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var closeErr error
	s.closeOnce.Do(func() {
		close(s.done)
		closeErr = s.httpServer.Shutdown(ctx)
		if errors.Is(closeErr, http.ErrServerClosed) {
			closeErr = nil
		}
	})
	return closeErr
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

func (s *Server) validateRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		s.writeSecurityHeaders(writer)
		if request.Host != s.host {
			http.Error(writer, "invalid companion host", http.StatusMisdirectedRequest)
			return
		}
		if origin := request.Header.Get("Origin"); origin != "" && origin != s.baseURL {
			http.Error(writer, "invalid companion origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) writeSecurityHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; img-src 'self' data:; object-src 'none'; script-src 'self'; style-src 'self'")
	writer.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	writer.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("X-Frame-Options", "DENY")
}

func (s *Server) handlePair(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := request.URL.Query().Get("token")
	s.pairMu.Lock()
	valid := !s.paired && constantTimeEqual(token, s.pairToken)
	if valid {
		s.paired = true
	}
	s.pairMu.Unlock()
	if !valid {
		http.Error(writer, "pairing link is invalid or has already been used", http.StatusUnauthorized)
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name:     companionCookie,
		Value:    s.sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

func constantTimeEqual(actual, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(companionCookie)
		if err != nil || !constantTimeEqual(cookie.Value, s.sessionToken) {
			http.Error(writer, "open the pairing URL printed by OpsQuest", http.StatusUnauthorized)
			return
		}
		next(writer, request)
	}
}

func (s *Server) handleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet || request.URL.Path != "/" {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(writer, request)
		return
	}
	s.serveStatic(writer, "static/index.html", "text/html; charset=utf-8")
}

func (s *Server) handleCSS(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.serveStatic(writer, "static/app.css", "text/css; charset=utf-8")
}

func (s *Server) handleJavaScript(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.serveStatic(writer, "static/app.js", "text/javascript; charset=utf-8")
}

func (s *Server) serveStatic(writer http.ResponseWriter, name, contentType string) {
	content, err := staticFiles.ReadFile(name)
	if err != nil {
		http.Error(writer, "companion asset unavailable", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", contentType)
	_, _ = writer.Write(content)
}

func (s *Server) handleState(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	item, available := s.current, s.hasCurrent
	s.mu.Unlock()
	if !available {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-OpsQuest-Event-ID", fmt.Sprint(item.id))
	_, _ = writer.Write(item.data)
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
