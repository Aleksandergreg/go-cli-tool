// Package webapp serves the local, read-only OpsQuest mission companion.
package webapp

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const companionCookie = "opsquest_companion"

//go:embed static/*
var staticFiles embed.FS

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

	// connMu guards connections that have not started a request yet.
	// http.Server.Shutdown leaves such connections open for several seconds,
	// and browsers routinely preconnect, so Close must end them itself.
	connMu  sync.Mutex
	closing bool
	unused  map[net.Conn]struct{}

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
		unused:       make(map[net.Conn]struct{}),
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
		ConnState:         server.trackConnection,
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

// Close stops accepting companion requests, closes active event streams and
// unused connections, and force-closes anything still open when ctx expires.
func (s *Server) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var closeErr error
	s.closeOnce.Do(func() {
		close(s.done)
		s.closeUnusedConnections()
		closeErr = s.httpServer.Shutdown(ctx)
		if errors.Is(closeErr, http.ErrServerClosed) {
			closeErr = nil
		}
		if closeErr != nil {
			_ = s.httpServer.Close()
		}
	})
	return closeErr
}

func (s *Server) trackConnection(conn net.Conn, state http.ConnState) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if state != http.StateNew {
		delete(s.unused, conn)
		return
	}
	if s.closing {
		_ = conn.Close()
		return
	}
	s.unused[conn] = struct{}{}
}

func (s *Server) closeUnusedConnections() {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	s.closing = true
	for conn := range s.unused {
		_ = conn.Close()
		delete(s.unused, conn)
	}
}
