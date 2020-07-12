package srvdns

import (
	"fmt"
	"net"

	"github.com/rob05c/traffic_router/shared"

	"github.com/miekg/dns"
)

type Server struct {
	Shared *shared.Shared
}

func (ha *Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
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
			serverAddr, _, _, refuse, servFail := ha.Shared.GetServerForDomain(clientAddr, zone, domain, v4)
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
			serverAddr, _, _, refuse, servFail := ha.Shared.GetServerForDomain(clientAddr, zone, domain, v4)
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
				serverAddr, _, _, refuse, servFail := ha.Shared.GetServerForDomain(clientAddr, zone, domain, v4)
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
				serverAddr, _, _, refuse, servFail := ha.Shared.GetServerForDomain(clientAddr, zone, domain, v4)
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
