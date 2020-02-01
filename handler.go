package main

import (
	"github.com/miekg/dns"
	"net"
)

type Handler struct {
	Domains map[string]string
}

func (ha *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)
	switch r.Question[0].Qtype {
	case dns.TypeA:
		domain := msg.Question[0].Name
		address, ok := ha.Domains[domain]
		if ok {
			msg.Authoritative = true
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(address),
			})
		} else {
			msg.Rcode = dns.RcodeRefused
		}
	}
	w.WriteMsg(&msg)
}
