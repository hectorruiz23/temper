package httpserver

import (
	"log"
	"net"
	"net/http"
)

type Server struct {
	http *http.Server
}

func New(addr string, handler http.Handler) *Server {
	return &Server{
		http: &http.Server{
			Addr:     addr,
			Handler:  handler,
			ErrorLog: log.Default(),
		},
	}
}

func NewHandler(token string) http.Handler {
	return NewRouter(token)
}

func (s *Server) Serve(listener net.Listener) error {
	return s.http.Serve(listener)
}
