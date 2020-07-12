package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/rob05c/traffic_router/loadconfig"
	"github.com/rob05c/traffic_router/srvdns"
	"github.com/rob05c/traffic_router/srvhttp"
	"github.com/rob05c/traffic_router/srvsighupreload"

	"github.com/miekg/dns"
)

func main() {
	cfgFile := flag.String("cfg", "", "Config file path")
	flag.Parse()
	if *cfgFile == "" {
		fmt.Println("usage: ./dnstest -cfg config/file/path.json")
		os.Exit(1)
	}

	shared, err := loadconfig.LoadConfig(*cfgFile)
	if err != nil {
		fmt.Println("Error loading config file '" + *cfgFile + "': " + err.Error())
		os.Exit(1)
	}

	dnsSvr := srvdns.NewPtr(&srvdns.Server{Shared: shared})
	httpSvr := srvhttp.NewPtr(&srvhttp.Server{Shared: shared})

	go func() {
		srv := &dns.Server{
			Addr:    ":" + strconv.Itoa(53),
			Net:     "udp",
			Handler: dnsSvr,
		}
		fmt.Println("Serving DNS UDP...")
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("Failed to set udp listener %s\n", err.Error())
		}
	}()
	go func() {
		srv := &dns.Server{
			Addr:    ":" + strconv.Itoa(53),
			Net:     "tcp",
			Handler: dnsSvr,
		}
		fmt.Println("Serving DNS TCP...")
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("Failed to set udp listener %s\n", err.Error())
		}
	}()

	go func() {
		svr := &http.Server{
			Handler: httpSvr,
			//			TLSConfig:    tlsConfig,
			Addr: fmt.Sprintf(":%d", 80), // TODO make configurable
			// ConnState: connState,
			// IdleTimeout:  idleTimeout,
			// ReadTimeout:  readTimeout,
			// WriteTimeout: writeTimeout,
		}
		fmt.Println("Serving HTTP...")
		if err := svr.ListenAndServe(); err != nil {
			log.Fatalf("ERROR: HTTP listener %s\n", err.Error())
		}
	}()

	srvsighupreload.Listen(*cfgFile, dnsSvr, httpSvr)
}
