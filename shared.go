package main

// shared is the object containing shared data used by the handlers, e.g. CZF, CRConfig, etc.
//
// Most functions are either used by handlers and NOT modified, or used by event listeners to update data.
//
// All functions which are safe for handlers say they are safe for usage by handlers.
// If func does not say it is safe for use by handlers, IT IS NOT.
//

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/apache/trafficcontrol/lib/go-tc"
)

//
// Steps to match a request to an ip:
// 1. Get the client's IP, from the request   (Data: ClientIP)
// 2. Get the client's request FQDN           (Data: ClientIP, RequestFQDN)
// 3. Find the FQDN to the DS                (Data: ClientIP, RequestFQDN, DS)
// 4. Find the CG nearest to the ClientIP  (Data: ClientIP, RequestFQDN, DS, CG)
// 5.
//

// TODO change to determine cg/sv/ds on CRConfig load, and assign each a number, so it's indexing an array instead of a string hash lookup? Way faster.

type Shared struct {
	// czf matches client IPs via CIDR to Cachegroups.
	// TODO make atomic, when it's updated by a listener.
	czf *ParsedCZF
	// crc is the CRConfig from Traffic Ops
	crc *tc.CRConfig
	// matches matches client request FQDN to DS name.
	matches []DSAndMatch
	// dsServers matches ds to servers assigned to that DS (and hashed in order).
	dsServers map[tc.DeliveryServiceName]map[tc.CacheGroupName]DNSDSServers
	// serverAvailable is whether the given server is available, per the Traffic Monitor CRStates.
	serverAvailable map[tc.CacheName]bool
}

func NewShared(czf *ParsedCZF, crc *tc.CRConfig, crs *tc.CRStates) *Shared {
	sh := &Shared{czf: czf, crc: crc}
	err := error(nil)
	if sh.matches, err = BuildMatchesFromCRConfig(crc); err != nil {
		fmt.Printf("Error building DS Matches from CRConfig: " + err.Error())
	}
	dsServers, err := BuildDSServersFromCRConfig(crc)
	sh.dsServers = dsServers
	if err != nil {
		fmt.Printf("Error building DS Matches from CRConfig: " + err.Error())
	}
	sh.serverAvailable = BuildServerAvailableFromCRStates(crs)
	return sh
}

// GetCZF gets the Coverage Zone File (CZF).
// The returned CZF MUST NOT be modified.
//
// Safe for use by handlers.
//
func (sh *Shared) GetCZF() *ParsedCZF {
	return sh.czf
}

// GetCRConfig gets the Content Router Config (CRConfig).
// The returned CRConfig MUST NOT be modified.
//
// Safe for use by handlers.
//
func (sh *Shared) GetCRConfig() *tc.CRConfig {
	return sh.crc
}

type DNSDS struct {
	Name string // TODO necesary?

	// Servers map[tc.CacheGroupName][]DNSDSServer
}

// DNSDSServers contains hash rings of servers to route to, for a certain deliveryservice and cachegroup.
// These have already been hashed, but will need checked for health before sending clients to them.
// If a server is unhealthy, the next server in the hash ring should be used instead.
// TODO sort by hash
type DNSDSServers struct {
	V4s []DNSDSServer
	V6s []DNSDSServer
}

type DNSDSServer struct {
	HostName tc.CacheName
	Addr     string         // Addr is the IPv4 or IPv6 address of the server, to be returned via DNS.
	Status   tc.CacheStatus // Status is the TO Status (Reported, ONLINE, etc) - TODO change to int, faster compare?
}

type DSAndMatch struct {
	Matches []DNSDSMatch
	DS      tc.DeliveryServiceName
}

// type DNSDSServer struct {
// 	IP net.IP
// }

const CRConfigMatchSetProtocolDNS = `DNS`
const CRConfigMatchListTypeHost = `HOST`

// func NewDNSDSMatch(matchStr string) (DNSDSMatch, error) {

// BuildMatchesFromCRConfig builts DSAndMatch objects from the CRConfig Delivery Services.
// Note if an error is returned, the []DSAndMatch is still valid, and contains successful matches from all DSes that didn't error.
// This is very important for Self-Service: if a DS is broken, it must not break other DSes.
func BuildMatchesFromCRConfig(crc *tc.CRConfig) ([]DSAndMatch, error) {
	errStrs := []string{}
	matches := []DSAndMatch{}
	// fmt.Printf("DEBUG BuildMatchesFromCRConfig len(crc.DeliveryServices) %v\n", len(crc.DeliveryServices))
	for dsName, ds := range crc.DeliveryServices {
		dsMatch := DSAndMatch{DS: tc.DeliveryServiceName(dsName)}
		for _, crcMatchSet := range ds.MatchSets {
			if crcMatchSet == nil {
				errStrs = append(errStrs, "ds '"+dsName+"' had a null matchset, skipping!")
				continue
			}
			if crcMatchSet.Protocol != CRConfigMatchSetProtocolDNS {
				continue
			}
			for _, crcMatchList := range crcMatchSet.MatchList {
				if crcMatchList.MatchType != CRConfigMatchListTypeHost {
					// TODO handle other types? Ignore?
					errStrs = append(errStrs, "ds '"+dsName+"' had unknown match list type '"+crcMatchList.MatchType+"'")
					continue
				}
				match, err := NewDNSDSMatch(crcMatchList.Regex)
				if err != nil {
					errStrs = append(errStrs, "ds '"+dsName+"' compiling regex '"+crcMatchList.Regex+"': "+err.Error())
					continue
				}
				dsMatch.Matches = append(dsMatch.Matches, match)
			}
		}
		matches = append(matches, dsMatch)
	}
	err := error(nil)
	if len(errStrs) > 0 {
		err = errors.New(strings.Join(errStrs, "; "))
	}
	return matches, err
}

func BuildDSServersFromCRConfig(crc *tc.CRConfig) (map[tc.DeliveryServiceName]map[tc.CacheGroupName]DNSDSServers, error) {
	errStrs := []string{}
	dsServers := map[tc.DeliveryServiceName]map[tc.CacheGroupName]DNSDSServers{}
	for serverName, server := range crc.ContentServers {
		if server.ServerStatus == nil {
			errStrs = append(errStrs, "CRConfig server '"+serverName+"' has nil status, skipping!")
			continue
		}
		if server.CacheGroup == nil {
			errStrs = append(errStrs, "CRConfig server '"+serverName+"' has nil cachegroup, skipping!")
			continue
		}
		if (server.Ip == nil && *server.Ip != "") && (server.Ip6 == nil && *server.Ip6 != "") {
			errStrs = append(errStrs, "CRConfig server '"+serverName+"' has nil ip and ip6, skipping!")
			continue
		}
		if tc.CacheStatus(*server.ServerStatus) != tc.CacheStatusReported && tc.CacheStatus(*server.ServerStatus) != tc.CacheStatusOnline {
			continue
		}
		for dsNameStr, _ := range server.DeliveryServices {
			dsName := tc.DeliveryServiceName(dsNameStr)
			if dsServers[dsName] == nil {
				dsServers[dsName] = map[tc.CacheGroupName]DNSDSServers{}
			}
			if server.Ip != nil && *server.Ip != "" {
				if ip := ParseIPOrCIDR(*server.Ip); ip == nil {
					errStrs = append(errStrs, "CRConfig server '"+serverName+"' ip '"+*server.Ip+"' not valid, skipping!")
				} else if ip.To4() == nil {
					errStrs = append(errStrs, "CRConfig server '"+serverName+"' ip '"+*server.Ip+"' not IPv4, skipping!")
				} else {
					cg := tc.CacheGroupName(*server.CacheGroup)
					dsServer := dsServers[dsName][cg]
					dsServer.V4s = append(dsServer.V4s, DNSDSServer{
						HostName: tc.CacheName(serverName),
						Addr:     *server.Ip,
						Status:   tc.CacheStatus(*server.ServerStatus),
					})
					dsServers[dsName][cg] = dsServer
				}
			}
			if server.Ip6 != nil && *server.Ip6 != "" {
				if ip := ParseIPOrCIDR(*server.Ip6); ip == nil {
					errStrs = append(errStrs, "CRConfig server '"+serverName+"' ip6 '"+*server.Ip6+"' not valid, skipping!")
				} else if ip.To4() != nil {
					errStrs = append(errStrs, "CRConfig server '"+serverName+"' ip6 '"+*server.Ip6+"' not IPv6, skipping!")
				} else {
					cg := tc.CacheGroupName(*server.CacheGroup)
					dsServer := dsServers[dsName][cg]
					dsServer.V6s = append(dsServer.V6s, DNSDSServer{
						HostName: tc.CacheName(serverName),
						Addr:     *server.Ip6,
						Status:   tc.CacheStatus(*server.ServerStatus),
					})
					dsServers[dsName][cg] = dsServer
				}
			}
		}
	}
	err := error(nil)
	if len(errStrs) > 0 {
		err = errors.New(strings.Join(errStrs, "\n"))
	}
	return dsServers, err
}

func ParseIPOrCIDR(str string) net.IP {
	if ip := net.ParseIP(str); ip != nil {
		return ip
	}
	ip, _, _ := net.ParseCIDR(str)
	return ip
}

func BuildServerAvailableFromCRStates(crs *tc.CRStates) map[tc.CacheName]bool {
	avail := map[tc.CacheName]bool{}
	for cacheName, isAvail := range crs.Caches {
		avail[cacheName] = isAvail.IsAvailable
	}
	return avail
}
