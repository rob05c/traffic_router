package srvhttp

import (
	"io"
	"net/http"

	"github.com/rob05c/traffic_router/shared"
)

type Server struct {
	Shared *shared.Shared
}

func New(sharedObj *shared.Shared) *Server {
	return &Server{Shared: sharedObj}
}

func (h *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hallo, Welt!")
}
