package main

import (
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"log"
	"os"
	"strconv"
)

func main() {
	cfgFile := flag.String("cfg", "", "Config file path")
	flag.Parse()
	if *cfgFile == "" {
		fmt.Println("usage: ./dnstest -cfg config/file/path.json")
		os.Exit(1)
	}

	cfg, err := LoadConfig(*cfgFile)
	if err != nil {
		fmt.Println("Error loading config file '" + *cfgFile + "': " + err.Error())
		os.Exit(1)
	}

	czf, err := LoadCZF(cfg.CZFPath)
	if err != nil {
		fmt.Println("Error loading czf file '" + cfg.CZFPath + "': " + err.Error())
		os.Exit(1)
	}

	crc, err := LoadCRConfig(cfg.CRConfigPath)
	if err != nil {
		fmt.Println("Error loading CRConfig file '" + cfg.CRConfigPath + "': " + err.Error())
		os.Exit(1)
	}

	crs, err := LoadCRStates(cfg.CRStatesPath)
	if err != nil {
		fmt.Println("Error loading CRStates file '" + cfg.CRStatesPath + "': " + err.Error())
		os.Exit(1)
	}

	// fmt.Printf("DEBUG crc.config '%v': %+v\n", cfg.CRConfigPath, crc.Config)

	czfParsedNets, err := ParseCZNets(czf.CoverageZones)
	if err != nil {
		fmt.Println("Error parsing czf networks '" + cfg.CZFPath + "': " + err.Error())
		os.Exit(1)
	}

	parsedCZF := &ParsedCZF{Revision: czf.Revision, CustomerName: czf.CustomerName, CoverageZones: czfParsedNets}

	shared := NewShared(parsedCZF, crc, crs)
	if shared == nil {
		fmt.Println("ERROR: fatal error creating Shared object, see log for details.")
		os.Exit(1)
	}

	go func() {
		srv := &dns.Server{Addr: ":" + strconv.Itoa(53), Net: "udp"}
		srv.Handler = &Handler{Shared: shared}
		fmt.Println("Serving UDP...")
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("Failed to set udp listener %s\n", err.Error())
		}
	}()

	srv := &dns.Server{Addr: ":" + strconv.Itoa(53), Net: "tcp"}
	srv.Handler = &Handler{Shared: shared}
	fmt.Println("Serving TCP...")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to set udp listener %s\n", err.Error())
	}
}
