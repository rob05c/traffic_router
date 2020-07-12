package srvhttp

import (
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/rob05c/traffic_router/rfc"
	"github.com/rob05c/traffic_router/shared"
)

type Server struct {
	Shared *shared.Shared
}

func New(sharedObj *shared.Shared) *Server {
	return &Server{Shared: sharedObj}
}

func (sv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clientAddrStr := r.RemoteAddr
	// func ResolveIPAddr(network, address string) (*IPAddr, error)

	zone := "" // zone == cachegroup
	ipStr, _, err := net.SplitHostPort(clientAddrStr)
	if err != nil {
		// TODO SERVFAIL here
		fmt.Println("ERROR: failed to split client ip:port '" + clientAddrStr + "', czf zone will be empty: " + err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		fmt.Println("ERROR: failed to parse client ip '" + ipStr + "' addr '" + clientAddrStr + "', czf zone will be empty")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	zone = sv.Shared.GetCZF().GetZone(ip)

	clientAddr, err := net.ResolveIPAddr("ip", ipStr)
	if err != nil {
		fmt.Println("ERROR: failed to parse client ip addr '" + r.RemoteAddr + "': " + err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	isV4 := ip.To4() != nil

	// TODO determine how to handle requests with ports (as-is, they'll be rejected as not matching any DS)
	// requestedDomain, _, err := net.SplitHostPort(r.Host)
	requestedDomain := r.Host
	if err != nil {
		fmt.Println("ERROR: failed to parse client requested url '" + r.Host + "' addr '" + clientAddrStr + "', returning Internal Server Error")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	requestedDomain += "." //TODO change Shared.GetServerForDomain to not expect the . and remove it with DNS

	_, cacheHostName, dsName, refuse, servFail := sv.Shared.GetServerForDomain(clientAddr, zone, requestedDomain, isV4)
	if servFail {
		// GetServerForDomain already logged. // TODO change to return err instead of logging itself
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if refuse {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "This server does not handle requested domain.")
		return
	}

	redirectFQDN := cacheHostName + "." + dsName + "." + sv.Shared.GetCDNDomain()
	redirectURL := "http://" + redirectFQDN + r.URL.Path // TODO add HTTPS support, for HTTPS DSes
	if r.URL.RawQuery != "" {
		redirectURL += "?" + r.URL.RawQuery
	}
	w.Header().Set(rfc.HdrLocation, redirectURL)
	w.WriteHeader(http.StatusFound) // TODO make configurable
}
