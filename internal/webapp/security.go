package webapp

import (
	"crypto/subtle"
	"net/http"
)

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
