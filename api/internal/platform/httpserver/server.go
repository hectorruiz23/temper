package httpserver

import (
	"log"
	"net"
	"net/http"
)

type Server struct {
	http *http.Server
}

func New(addr, token string) *Server {
	return &Server{
		http: &http.Server{
			Addr:     addr,
			Handler:  NewHandler(token),
			ErrorLog: log.Default(),
		},
	}
}

func NewHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/health" {
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		if !validToken(r, token) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "route not found")
	})
}

func (s *Server) Serve(listener net.Listener) error {
	return s.http.Serve(listener)
}
