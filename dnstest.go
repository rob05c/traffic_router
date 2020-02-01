package main

import (
	"fmt"
	"github.com/miekg/dns"
	"log"
	"strconv"
)

func TestDomains() map[string]string {
	return map[string]string{
		"foo.test.": "1.2.3.4",
		"bar.test.": "104.198.14.52",
	}
}

func main() {
	go func() {
		srv := &dns.Server{Addr: ":" + strconv.Itoa(53), Net: "udp"}
		srv.Handler = &Handler{Domains: TestDomains()}
		fmt.Println("Serving UDP...")
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("Failed to set udp listener %s\n", err.Error())
		}
	}()

	srv := &dns.Server{Addr: ":" + strconv.Itoa(53), Net: "tcp"}
	srv.Handler = &Handler{Domains: TestDomains()}
	fmt.Println("Serving TCP...")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to set udp listener %s\n", err.Error())
	}
}
