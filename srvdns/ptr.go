package srvdns

import (
	"sync/atomic"
	"unsafe"

	"github.com/miekg/dns"
)

// ServerPtr maintains an atomic pointer to a real server, which may be safely swapped while running to hot-reload config.
type ServerPtr struct {
	real *unsafe.Pointer
}

// NewPtr creates a new ServerPtr from a Server.
func NewPtr(sv *Server) *ServerPtr {
	ptr := (unsafe.Pointer)(sv)
	return &ServerPtr{real: &ptr}
}

// Set atomically sets the underlying Server of the ServerPtr.
// This may safely be called by multiple goroutines, while ServerPtr is serving.
func (sp *ServerPtr) Set(svr *Server) {
	ptr := (unsafe.Pointer)(svr)
	atomic.StorePointer(sp.real, ptr)
}

// ServeDNS serves DNS by calling the underlying Server.
func (sv *ServerPtr) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	realSvr := (*Server)(atomic.LoadPointer(sv.real))
	realSvr.ServeDNS(w, r)
}
