package srvhttp

import (
	"net/http"
	"sync/atomic"
	"unsafe"
)

// ServerPtr maintains an atomic pointer to a real server, which may be safely swapped while running to hot-reload config.
type ServerPtr struct {
	realSvr *unsafe.Pointer
}

// NewPtr creates a new ServerPtr from a Server.
func NewPtr(realSvr *Server) *ServerPtr {
	p := (unsafe.Pointer)(realSvr)
	return &ServerPtr{realSvr: &p}
}

// Set atomically sets the underlying Server of the ServerPtr.
// This may safely be called by multiple goroutines, while ServerPtr is serving.
func (sp *ServerPtr) Set(svr *Server) {
	ptr := (unsafe.Pointer)(svr)
	atomic.StorePointer(sp.realSvr, ptr)
}

// ServeHTTP serves HTTP by calling the underlying Server.
func (h *ServerPtr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	realSvr := (*Server)(atomic.LoadPointer(h.realSvr))
	realSvr.ServeHTTP(w, r)
}
