package main

import (
	"fmt"
	"math/rand"
	"net"

	"github.com/apache/trafficcontrol/lib/go-tc"

	"github.com/miekg/dns"
)

type Handler struct {
	Shared *Shared
}

func (ha *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	clientAddr := w.RemoteAddr()

	zone := "" // zone == cachegroup
	if ipStr, _, err := net.SplitHostPort(clientAddr.String()); err != nil {
		// TODO SERVFAIL here
		fmt.Println("ERROR: failed to split client ip:port '" + clientAddr.String() + "', czf zone will be empty: " + err.Error())
	} else if ip := net.ParseIP(ipStr); ip == nil {
		// TODO SERVFAIL here
		fmt.Println("ERROR: failed to parse client ip '" + ipStr + "' addr '" + clientAddr.String() + "', czf zone will be empty")
	} else {
		zone = ha.Shared.GetCZF().GetZone(ip)
	}

	msg := dns.Msg{}
	msg.SetReply(r)
	for _, question := range r.Question {
		domain := question.Name
		switch question.Qtype {
		case dns.TypeA:
			v4 := true // A record => v4
			serverAddr, refuse, servFail := ha.Shared.GetServerForDomain(clientAddr, zone, domain, v4)
			if servFail {
				msg.Rcode = dns.RcodeServerFailure
				w.WriteMsg(&msg)
				return
			} else if refuse {
				msg.Rcode = dns.RcodeRefused
				w.WriteMsg(&msg)
				return
			}
			msg.Answer = append(msg.Answer, &dns.A{
				// TODO CRConfig ttl
				Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(serverAddr), // TODO change DNSDSServer to store IP
			})
		case dns.TypeAAAA:
			v4 := false // A record => v4
			serverAddr, refuse, servFail := ha.Shared.GetServerForDomain(clientAddr, zone, domain, v4)
			if servFail {
				msg.Rcode = dns.RcodeServerFailure
				w.WriteMsg(&msg)
				return
			} else if refuse {
				msg.Rcode = dns.RcodeRefused
				w.WriteMsg(&msg)
				return
			}
			ip := net.ParseIP(serverAddr)
			fmt.Println("aaaa ip '" + ip.String() + "'")
			msg.Answer = append(msg.Answer, &dns.AAAA{
				// TODO CRConfig ttl
				Hdr:  dns.RR_Header{Name: domain, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60},
				AAAA: net.ParseIP(serverAddr), // TODO change DNSDSServer to store IP
			})
		case dns.TypeANY:
			// TODO remove duplicate code
			{
				v4 := true // A record => v4
				serverAddr, refuse, servFail := ha.Shared.GetServerForDomain(clientAddr, zone, domain, v4)
				if servFail {
					msg.Rcode = dns.RcodeServerFailure
					w.WriteMsg(&msg)
					return
				} else if refuse {
					msg.Rcode = dns.RcodeRefused
					w.WriteMsg(&msg)
					return
				}
				msg.Answer = append(msg.Answer, &dns.A{
					// TODO CRConfig ttl
					Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
					A:   net.ParseIP(serverAddr), // TODO change DNSDSServer to store IP
				})
			}
			{
				v4 := false // A record => v4
				serverAddr, refuse, servFail := ha.Shared.GetServerForDomain(clientAddr, zone, domain, v4)
				if servFail {
					msg.Rcode = dns.RcodeServerFailure
					w.WriteMsg(&msg)
					return
				} else if refuse {
					msg.Rcode = dns.RcodeRefused
					w.WriteMsg(&msg)
					return
				}
				ip := net.ParseIP(serverAddr)
				fmt.Println("aaaa ip '" + ip.String() + "'")
				msg.Answer = append(msg.Answer, &dns.AAAA{
					// TODO CRConfig ttl
					Hdr:  dns.RR_Header{Name: domain, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60},
					AAAA: net.ParseIP(serverAddr), // TODO change DNSDSServer to store IP
				})
			}
		default:
			fmt.Println("EVENT: Request: " + clientAddr.String() + " requested: unhandled type") // TODO event log
			msg.Rcode = dns.RcodeRefused
			w.WriteMsg(&msg)
			return
		}
	}

	msg.Authoritative = true
	w.WriteMsg(&msg)
}

func matchFQDN(fqdn string, matches []DSAndMatch) (tc.DeliveryServiceName, bool) {
	// fmt.Printf("DEBUG matchFQDN len(matches) %v\n", len(matches))
	for _, dsMatch := range matches {
		for _, ma := range dsMatch.Matches {
			if ma.Match(fqdn) {
				return dsMatch.DS, true
			}
		}
	}
	return "", false
}

// getServer finds a server from the list, for the given IP type.
// TODO consistent-hash DNSDSServers.
// TODO use fallback CG if cg is unavailable.
func getServer(allServers DNSDSServers, v4 bool, serverAvailable map[tc.CacheName]bool) (DNSDSServer, bool) {
	servers := allServers.V4s
	if !v4 {
		servers = allServers.V6s
	}
	// fmt.Printf("DEBUG servers '%v' v4 %v server '%v'\n", allServers, v4, servers)
	if len(servers) == 0 {
		return DNSDSServer{}, false
	} else if len(servers) == 1 {
		return servers[0], true // must check, because math.Intn(0) panics.
	}

	randI := rand.Intn(len(servers) - 1)
	startI := randI
	sv := servers[randI]
	for {
		if serverAvailable[sv.HostName] {
			break
		}
		randI++
		if randI == len(servers) {
			randI = 0 // loop around if we're at the end
		}
		if randI == startI {
			return DNSDSServer{}, false // we looped over all servers, and none were available
		}
	}

	return servers[randI], true
}
