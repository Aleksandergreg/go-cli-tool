package webapp

import (
	"fmt"
	"net/http"
)

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
