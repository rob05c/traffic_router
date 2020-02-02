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
	addr := w.RemoteAddr()

	msg := dns.Msg{}
	msg.SetReply(r)
	switch r.Question[0].Qtype {
	case dns.TypeA:
		domain := msg.Question[0].Name

		zone := "" // zone == cachegroup
		isIPv4 := true
		if ipStr, _, err := net.SplitHostPort(addr.String()); err != nil {
			// TODO SERVFAIL here
			fmt.Println("ERROR: failed to split client ip:port '" + addr.String() + "', czf zone will be empty: " + err.Error())
		} else if ip := net.ParseIP(ipStr); ip == nil {
			// TODO SERVFAIL here
			fmt.Println("ERROR: failed to parse client ip '" + ipStr + "' addr '" + addr.String() + "', czf zone will be empty")
		} else {
			isIPv4 = ip.To4() != nil
			zone = ha.Shared.GetCZF().GetZone(ip)
		}

		dsName, ok := matchFQDN(domain, ha.Shared.matches)
		if !ok {
			fmt.Printf(`EVENT: Request: %v czf zone %v requested A %v ds (%v %v) - no match, returning Refused`, addr.String(), zone, domain, dsName, ok)
			msg.Rcode = dns.RcodeRefused
			w.WriteMsg(&msg)
			return
		}

		dsServers, ok := ha.Shared.dsServers[dsName]
		if !ok {
			// should never happen (we found a match, but it wasn't in the list of ds servers
			fmt.Printf(`EVENT: Request: %v czf zone %v requested A %v ds '%v' - match, but not in dsServers! should never happen! Returning ServFail`, addr.String(), zone, domain, dsName, ok)
			msg.Rcode = dns.RcodeServerFailure
			w.WriteMsg(&msg)
			return
		}

		cg := tc.CacheGroupName(zone)
		cgServers, ok := dsServers[cg]
		if !ok {
			// we found a match, but there were no servers in the found cachegroup with an Edge on this DS.
			fmt.Printf(`EVENT: Request: %v czf zone %v requested A %v ds '%v' - match, but the requested DS had no servers in the matched cachegruopo! Returning ServFail`, addr.String(), zone, domain, dsName)
			msg.Rcode = dns.RcodeServerFailure
			w.WriteMsg(&msg)
			return
		}

		dsServer, ok := getServer(cgServers, isIPv4, ha.Shared.serverAvailable)
		if !ok {
			// we found a match, but there were no servers of the requested IP type on the CG assigned to the DS.
			fmt.Printf(`EVENT: Request: %v czf zone %v requested A %v ds '%v' - match, but no servers of type IPv4=%v in the cg on the ds! Returning ServFail`, addr.String(), zone, domain, dsName, ok, isIPv4)
			msg.Rcode = dns.RcodeServerFailure
			w.WriteMsg(&msg)
			return
		}

		msg.Authoritative = true
		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
			A:   net.ParseIP(dsServer.Addr), // TODO change DNSDSServer to store IP
		})
		w.WriteMsg(&msg)
		return
	default:
		fmt.Println("EVENT: Request: " + addr.String() + " requested: unhandled type") // TODO event log
		w.WriteMsg(&msg)
		return
	}
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
