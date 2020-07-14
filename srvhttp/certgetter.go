package srvhttp

import (
	"crypto/tls"
	"strings"
	"sync"
)

// CertGetter provides a type to be used for dynamically adding and removing certificates from a running HTTPS Server.
// To use, create a CertGetter pointer, then pass it to MakeGetCertificateFunc and set the function it returns as the tls.Config.GetCertificate, before starting your HTTPS Server.
//
// Pass the tls.Config to tls.Listen, create an http.Server, and call server.Serve(listener).
//
// All functions of CertGetter may safely be called while a server is listening and serving.
//
// Must not be copied after first use. Take a reference and pass around the pointer.
type CertGetter struct {
	m sync.Map
}

func (cg *CertGetter) Add(host string, cert *tls.Certificate) { cg.m.Store(host, cert) }

// Get returns the certificate for the given FQDN.
// If the literal FQDN is not found, a wildcard match is searched for all the way up.
// TODO change to take the tls.ClientHelloInfo, and properly check ciphers, and support multiple certs for the same FQDN.
func (cg *CertGetter) Get(fqdn string) (*tls.Certificate, bool) {
	cert, ok := cg.m.Load(fqdn)
	if ok {
		return cert.(*tls.Certificate), true
	}
	// literal FQDN wasn't found, try to wildcard-search all the way up.
	for {
		dotI := strings.Index(fqdn, ".")
		if dotI < 0 {
			return nil, false
		}
		fqdn = fqdn[dotI+1:]
		cert, ok = cg.m.Load("*." + fqdn)
		if ok {
			return cert.(*tls.Certificate), true
		}
	}
}
func (cg *CertGetter) Delete(host string) { cg.m.Delete(host) }

// Hosts returns the list of hosts in the CertGetter.
// This is not guaranteed to be atomic if other goroutines are concurrently calling Add.
func (cg *CertGetter) Hosts() map[string]struct{} {
	hosts := map[string]struct{}{}
	cg.m.Range(func(key, value interface{}) bool {
		hosts[key.(string)] = struct{}{}
		return true
	})
	return hosts
}

// MakGetCertificateFunc takes a CertGetter pointer, and returns a func to be set as a tls.Config.GetCertificate.
func MakeGetCertificateFunc(cg *CertGetter) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(in *tls.ClientHelloInfo) (*tls.Certificate, error) {
		cert, _ := cg.Get(in.ServerName)
		return cert, nil
	}
}
