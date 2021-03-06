package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/rob05c/traffic_router/loadconfig"
	"github.com/rob05c/traffic_router/pollercrconfig"
	"github.com/rob05c/traffic_router/pollercrstates"
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

	shared, cfg, err := loadconfig.LoadConfig(*cfgFile)
	if err != nil {
		fmt.Println("Error loading config file '" + *cfgFile + "': " + err.Error())
		os.Exit(1)
	}

	crStatesPollInterval := time.Duration(cfg.CRStatesPollIntervalMS) * time.Millisecond
	crStatesPoller, crStatesIPoller := pollercrstates.MakePoller(crStatesPollInterval, cfg.Monitors, shared)

	crConfigPollInterval := time.Duration(cfg.CRConfigPollIntervalMS) * time.Millisecond
	crConfigPoller, crConfigIPoller := pollercrconfig.MakePoller(crConfigPollInterval, cfg.Monitors, shared)

	if err := crStatesPoller.Start(); err != nil {
		fmt.Println("Error starting CRStates poller: " + err.Error())
		os.Exit(1)
	}

	if err := crConfigPoller.Start(); err != nil {
		fmt.Println("Error starting CRConfig poller: " + err.Error())
		os.Exit(1)
	}

	dnsSvr := srvdns.NewPtr(&srvdns.Server{Shared: shared})
	httpSvr := srvhttp.NewPtr(&srvhttp.Server{Shared: shared})

	// TODO add default cert, for when no match is found
	certGetter := &srvhttp.CertGetter{}

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

	go func() {
		tlsConfig := &tls.Config{
			GetCertificate: srvhttp.MakeGetCertificateFunc(certGetter),
		}

		svr := &http.Server{
			Handler:   httpSvr,
			TLSConfig: tlsConfig,
			Addr:      fmt.Sprintf(":%d", 443), // TODO make configurable
			// ConnState: connState,
			// IdleTimeout:  idleTimeout,
			// ReadTimeout:  readTimeout,
			// WriteTimeout: writeTimeout,
		}

		listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", 443), tlsConfig)
		if err != nil {
			log.Fatalf("ERROR: HTTPS listener %s\n", err.Error())
		}

		fmt.Println("Serving HTTPS...")
		if err := svr.Serve(listener); err != nil {
			log.Fatalf("ERROR: HTTP server %s\n", err.Error())
		}
	}()

	srvsighupreload.Listen(
		*cfgFile,
		dnsSvr,
		httpSvr,
		certGetter,
		crStatesPoller,
		crStatesIPoller,
		crConfigPoller,
		crConfigIPoller,
	)
}
