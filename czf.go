package main

import (
	"encoding/json"
	"errors"
	"net"
	"os"
)

type CZF struct {
	Revision      string                     `json:"revision"`
	CustomerName  string                     `json:"customerName"`
	CoverageZones map[string]CZFCoverageZone `json:"coverageZones"`
}

type ParsedCZF struct {
	Revision      string
	CustomerName  string
	CoverageZones map[string]ParsedCZFCoverageZone
}

type CZFCoverageZone struct {
	Network     []string  `json:"network"`
	Network6    []string  `json:"network6"`
	Coordinates CZFLatLon `json:"coordinates"`
}

type ParsedCZFCoverageZone struct {
	Network     []*net.IPNet `json:"network"`
	Network6    []*net.IPNet `json:"network6"`
	Coordinates CZFLatLon    `json:"coordinates"`
}

type CZFLatLon struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func LoadCZF(path string) (*CZF, error) {
	fi, err := os.Open(path)
	if err != nil {
		return nil, errors.New("loading file: " + err.Error())
	}
	defer fi.Close()
	cz := CZF{}
	if err := json.NewDecoder(fi).Decode(&cz); err != nil {
		return nil, errors.New("decoding: " + err.Error())
	}
	return &cz, nil
}

func ParseCZNets(czs map[string]CZFCoverageZone) (map[string]ParsedCZFCoverageZone, error) {
	parsed := map[string]ParsedCZFCoverageZone{}
	for zoneName, zone := range czs {
		parsedZone, err := ParseCZNet(zone)
		if err != nil {
			return nil, errors.New("parsing zone '" + zoneName + "': " + err.Error())
		}
		parsed[zoneName] = parsedZone
	}
	return parsed, nil
}

func ParseCZNet(cz CZFCoverageZone) (ParsedCZFCoverageZone, error) {
	parsed := ParsedCZFCoverageZone{Coordinates: cz.Coordinates}
	for _, network := range cz.Network {
		_, ipnet, err := net.ParseCIDR(network)
		if err != nil {
			return ParsedCZFCoverageZone{}, errors.New("parsing v4 '" + network + "': " + err.Error())
		}
		if isV4 := ipnet.IP.To4() != nil; !isV4 {
			return ParsedCZFCoverageZone{}, errors.New("parsing v4 '" + network + "': not IPv4")
		}
		parsed.Network = append(parsed.Network, ipnet)
	}
	for _, network := range cz.Network6 {
		_, ipnet, err := net.ParseCIDR(network)
		if err != nil {
			return ParsedCZFCoverageZone{}, errors.New("parsing '" + network + "': " + err.Error())
		}
		if isV4 := ipnet.IP.To4() != nil; isV4 {
			return ParsedCZFCoverageZone{}, errors.New("parsing v6 '" + network + "': not IPv6")
		}
		parsed.Network6 = append(parsed.Network6, ipnet)
	}
	return parsed, nil
}

// GetZone returns the CoverageZone name for ip. If no zone matches, returns "".
func (cz *ParsedCZF) GetZone(ip net.IP) string {
	// TODO use something faster, like https://github.com/yl2chen/cidranger

	if isV4 := ip.To4() != nil; isV4 {
		// fmt.Println("DEBUG client is v4")
		for zoneName, zone := range cz.CoverageZones {
			for _, network := range zone.Network {
				if network.Contains(ip) {
					return zoneName
				}
			}
		}
	} else {
		// fmt.Println("DEBUG client is '" + ip.String() + "' not v4")
		for zoneName, zone := range cz.CoverageZones {
			for _, network := range zone.Network6 {
				if network.Contains(ip) {
					return zoneName
				}
			}
		}
	}
	return ""
}
