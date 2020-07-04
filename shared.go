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
	"math/rand"
	"net"
	"strings"

	"github.com/apache/trafficcontrol/lib/go-tc"
	"github.com/apache/trafficcontrol/lib/go-util"
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

const DefaultHTTPRoutingName = "ccr"

type Shared struct {
	// czf matches client IPs via CIDR to Cachegroups.
	// TODO make atomic, when it's updated by a listener.
	czf *ParsedCZF
	// crc is the CRConfig from Traffic Ops
	crc *tc.CRConfig
	// dnsMatches matches client request FQDN to DS name, for DNS DSes
	dnsMatches DSMatches
	// httpDNSMatches matches client request FQDN to DS name, for the initial DNS request for an HTTP DS.
	// TODO combine matches, and have a match return the DS type?
	httpDNSMatches DSMatches
	// httpSecondDNSMatches contains map[fqdn]cache for the second DNS lookup of an HTTP DS,
	// of the form cache-name.ds-name.cdn-domain
	httpSecondDNSMatches map[string]tc.CacheName
	// dsServers matches ds to servers assigned to that DS (and hashed in order).
	dsServers map[tc.DeliveryServiceName]map[tc.CacheGroupName]DNSDSServers
	// cgRouters maps cachegroups to router servers
	cgRouters map[tc.CacheGroupName]DNSDSServers
	// serverAvailable is whether the given server is available, per the Traffic Monitor CRStates.
	serverAvailable map[tc.CacheName]bool
	// cdnDomain is the config/domain_name in the CRConfig, the TLD of the CDN.
	cdnDomain string
}

// NewShared creates a new Shared data object.
// Logs all errors, fatal and non-fatal.
// On fatal error, returns nil
func NewShared(czf *ParsedCZF, crc *tc.CRConfig, crs *tc.CRStates) *Shared {
	// TODO pre fetch and cache this, for performance. This is in the request path.
	//      Also, validate. Make sure it exists, is a valid FQDN, not empty, etc.
	iCDNDomain, ok := crc.Config["domain_name"] // : "top.comcast.net",
	if !ok {
		fmt.Printf("ERROR: GetServerForDomain: CRConfig missing config/domain_name, cannot serve!\n")
		// TODO validate on load, and refuse to load CRConfig
		return nil
	}
	cdnDomain, ok := iCDNDomain.(string)
	if !ok {
		fmt.Printf("ERROR: GetServerForDomain: CRConfig config/domain_name not a string, cannot serve!\n")
		// TODO validate on load, and refuse to load CRConfig
		return nil
	}

	sh := &Shared{czf: czf, crc: crc, cdnDomain: cdnDomain}
	err := error(nil)
	if sh.dnsMatches, sh.httpDNSMatches, err = BuildMatchesFromCRConfig(crc, cdnDomain); err != nil {
		fmt.Printf("Error building DS Matches from CRConfig: " + err.Error())
	}

	sh.httpSecondDNSMatches = BuildHTTPSecondDNSMatches(crc, cdnDomain)

	dsServers, err := BuildDSServersFromCRConfig(crc)
	sh.dsServers = dsServers
	if err != nil {
		fmt.Printf("Error building DS Servers from CRConfig: " + err.Error())
	}

	cgRouters, err := BuildCGRoutersFromCRConfig(crc)
	sh.cgRouters = cgRouters
	if err != nil {
		fmt.Printf("Error building CG Routers from CRConfig: " + err.Error())
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

type DSMatches []DSAndMatch

func (matches DSMatches) Match(fqdn string) (tc.DeliveryServiceName, bool) {
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

// type DNSDSServer struct {
// 	IP net.IP
// }

const CRConfigMatchSetProtocolDNS = `DNS`
const CRConfigMatchSetProtocolHTTP = `HTTP`
const CRConfigMatchListTypeHost = `HOST`

// BuildMatchesFromCRConfig builds DSAndMatch objects from the CRConfig Delivery Services.
//
// Note if an error is returned, the []DSAndMatch is still valid, and contains successful matches from all DSes that didn't error.
// This is very important for Self-Service: if a DS is broken, it must not break other DSes.
//
// Returns the DNS matches, HTTP DNS matches, and any errors from malformed DSes
//
func BuildMatchesFromCRConfig(crc *tc.CRConfig, cdnDomain string) ([]DSAndMatch, []DSAndMatch, error) {
	errs := []error{}
	dnsMatches := []DSAndMatch{}
	httpMatches := []DSAndMatch{}
	// fmt.Printf("DEBUG BuildMatchesFromCRConfig len(crc.DeliveryServices) %v\n", len(crc.DeliveryServices))
	for dsName, ds := range crc.DeliveryServices {
		routingName := DefaultHTTPRoutingName
		if ds.RoutingName != nil {
			routingName = *ds.RoutingName
		}
		dsMatch := DSAndMatch{DS: tc.DeliveryServiceName(dsName)}
		for _, crcMatchSet := range ds.MatchSets {
			if crcMatchSet == nil {
				errs = append(errs, errors.New("ds '"+dsName+"' had a null matchset, skipping!"))
				continue
			}
			switch crcMatchSet.Protocol {
			case CRConfigMatchSetProtocolDNS:
				dsDNSMatches, matchErrs := buildDNSMatches(crcMatchSet.MatchList)
				dsMatch.Matches = dsDNSMatches
				errs = append(errs, matchErrs...)
				dnsMatches = append(dnsMatches, dsMatch)
			case CRConfigMatchSetProtocolHTTP:
				dsHTTPMatches, matchErrs := buildHTTPMatches(crcMatchSet.MatchList, routingName, cdnDomain)
				errs = append(errs, matchErrs...)
				dsMatch.Matches = dsHTTPMatches
				httpMatches = append(httpMatches, dsMatch)
			default:
				fmt.Printf("ERROR: BuildMatcheFromCRConfig: ds '%v' had unknown match protocol %v', skipping!\n", dsName, crcMatchSet.Protocol)
			}
		}
	}
	return dnsMatches, httpMatches, util.JoinErrs(errs)
}

func buildHTTPMatches(matchLists []tc.MatchList, routingName string, cdnDomain string) ([]DNSDSMatch, []error) {
	matches := []DNSDSMatch{}
	errs := []error{}
	for _, crcMatchList := range matchLists {
		match, err := buildHTTPMatch(crcMatchList, routingName, cdnDomain)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		matches = append(matches, match)
	}
	return matches, errs
}

func buildHTTPMatch(matchList tc.MatchList, routingName string, cdnDomain string) (DNSDSMatch, error) {
	if matchList.MatchType != CRConfigMatchListTypeHost {
		// TODO handle other types? Ignore?
		return nil, errors.New("unknown match list type '" + matchList.MatchType + "'")
	}
	return NewHTTPDSMatch(matchList.Regex, routingName, cdnDomain)
}

// buildDNSMatches takes an array of MatchList for a DNS MatchSet.
// Returns the matches from the set, and any errors.
//
// If there is an error creating a Match, it is added to the returned error array,
// while successful matches continue.
//
// This is important for Self Service: a malformed DS must not break other DSes.
//
func buildDNSMatches(matchLists []tc.MatchList) ([]DNSDSMatch, []error) {
	matches := []DNSDSMatch{}
	errs := []error{}
	for _, crcMatchList := range matchLists {
		match, err := buildDNSMatch(crcMatchList)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		matches = append(matches, match)
	}
	return matches, errs
}

func buildDNSMatch(matchList tc.MatchList) (DNSDSMatch, error) {
	if matchList.MatchType != CRConfigMatchListTypeHost {
		// TODO handle other types? Ignore?
		return nil, errors.New("unknown match list type '" + matchList.MatchType + "'")
	}
	return NewDNSDSMatch(matchList.Regex)
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
						Addr:     ip.String(),
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
						Addr:     ip.String(),
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

func BuildCGRoutersFromCRConfig(crc *tc.CRConfig) (map[tc.CacheGroupName]DNSDSServers, error) {
	// TODO deduplicate with BuildDSServersFromCRConfig?
	//      How? They're largely duplicate, but they're fundamentally different types.
	//      It's possible, but Go's type system makes it a pain.
	errStrs := []string{}
	cgRouters := map[tc.CacheGroupName]DNSDSServers{}
	for routerName, router := range crc.ContentRouters {
		if router.ServerStatus == nil {
			errStrs = append(errStrs, "CRConfig router '"+routerName+"' has nil status, skipping!")
			continue
		}
		if router.Location == nil {
			errStrs = append(errStrs, "CRConfig server '"+routerName+"' has nil cachegroup, skipping!")
			continue
		}
		if (router.IP == nil && *router.IP != "") && (router.IP6 == nil && *router.IP6 != "") {
			errStrs = append(errStrs, "CRConfig server '"+routerName+"' has nil ip and ip6, skipping!")
			continue
		}
		if tc.CacheStatus(*router.ServerStatus) != tc.CacheStatusReported && tc.CacheStatus(*router.ServerStatus) != tc.CacheStatusOnline {
			continue
		}

		if router.IP != nil && *router.IP != "" {
			if ip := ParseIPOrCIDR(*router.IP); ip == nil {
				errStrs = append(errStrs, "CRConfig router '"+routerName+"' ip '"+*router.IP+"' not valid, skipping!")
			} else if ip.To4() == nil {
				errStrs = append(errStrs, "CRConfig router '"+routerName+"' ip '"+*router.IP+"' not IPv4, skipping!")
			} else {
				cg := tc.CacheGroupName(*router.Location)
				cgRouter := cgRouters[cg]
				cgRouter.V4s = append(cgRouter.V4s, DNSDSServer{
					HostName: tc.CacheName(routerName),
					Addr:     ip.String(),
					Status:   tc.CacheStatus(*router.ServerStatus),
				})
				cgRouters[cg] = cgRouter
			}
		}
		if router.IP6 != nil && *router.IP6 != "" {
			if ip := ParseIPOrCIDR(*router.IP6); ip == nil {
				errStrs = append(errStrs, "CRConfig server '"+routerName+"' ip6 '"+*router.IP6+"' not valid, skipping!")
			} else if ip.To4() != nil {
				errStrs = append(errStrs, "CRConfig server '"+routerName+"' ip6 '"+*router.IP6+"' not IPv6, skipping!")
			} else {
				cg := tc.CacheGroupName(*router.Location)
				cgRouter := cgRouters[cg]
				cgRouter.V6s = append(cgRouter.V6s, DNSDSServer{
					HostName: tc.CacheName(routerName),
					Addr:     ip.String(),
					Status:   tc.CacheStatus(*router.ServerStatus),
				})
				cgRouters[cg] = cgRouter
			}
		}
	}
	err := error(nil)
	if len(errStrs) > 0 {
		err = errors.New(strings.Join(errStrs, "\n"))
	}
	return cgRouters, err
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

// GetServerForDomain returns the IP of the server, whether to immediately return a refused (because the domain was not in the DS list), and whether to return a SERVFAIL (because there was a server error looking up the DS). Error messages are logged.
// TODO NXDOMAIN instead of refusing if it's a CDN domain e.g. top.comcast.net but just a nonexistent DS?
// TODO change to return multiple IPs, depending on DS configuration.
func (sh *Shared) GetServerForDomain(addr net.Addr, zone string, domain string, v4 bool) (string, bool, bool) {
	if !strings.HasSuffix(domain, ".") {
		fmt.Printf("EVENT: Request: %v czf zone '%v' requested A '%v' missing trailing '.' - returning Refused\n", addr.String(), zone, domain)
		return "", true, false // "", refuse, no servfail
	}
	domain = domain[:len(domain)-1] // remove trailing . because we want to match without it
	if !strings.HasSuffix(domain, sh.cdnDomain) {
		fmt.Printf("EVENT: Request: %v czf zone '%v' requested A '%v' which we're not authoritative for, returning Refused\n", addr.String(), zone, domain)
		return "", true, false // "", refuse, no servfail
	}

	// fastest lookup, so we do it first.
	// TODO change to a trie, even faster.
	if cacheName, ok := sh.httpSecondDNSMatches[domain]; ok {
		return sh.GetServerName(cacheName, v4)
	}
	if dsName, ok := sh.dnsMatches.Match(domain); ok {
		return sh.GetServerForDomainDNS(addr, zone, domain, v4, dsName)
	}
	if dsName, ok := sh.httpDNSMatches.Match(domain); ok {
		return sh.GetServerForDomainHTTP(addr, zone, domain, v4, dsName)
	}

	fmt.Printf("EVENT: Request: %v czf zone '%v' requested A '%v' - no DS match, returning Refused\n", addr.String(), zone, domain)
	return "", true, false // "", refuse, no servfail
}

func (sh *Shared) GetServerName(cacheName tc.CacheName, v4 bool) (string, bool, bool) {
	// TODO make faster. This is in the request path, and can be easily optimized.
	sv, ok := sh.crc.ContentServers[string(cacheName)]
	if !ok {
		// Should never happen. Maybe unless the CRConfig is malformed?
		return "", false, true // "", no refuse, servfail
	}
	if v4 {
		if sv.Ip == nil {
			fmt.Printf("ERROR: client requested cache.ds.cdn A for server '%v' with no IPv4 address, returning Refused\n", string(cacheName))
			return "", true, false // "", refuse, no servfail
		}
		// TODO parse IP to verify. Super-important, we REALLY don't want to give non-IPs to A reqs and heinously violate the DNS specs
		return *sv.Ip, false, false // ip, no refuse, no servfail
	}

	if sv.Ip6 == nil {
		fmt.Printf("ERROR: client requested cache.ds.cdn AAAA for server '%v' with no IPv6 address, returning Refused\n", string(cacheName))
		return "", true, false // "", refuse, no servfail
	}
	// TODO parse IP to verify. Super-important, we REALLY don't want to give non-IPs to A reqs and heinously violate the DNS specs
	return *sv.Ip6, false, false // ip, no refuse, no servfail
}

func (sh *Shared) GetServerForDomainDNS(
	addr net.Addr,
	zone string,
	domain string,
	v4 bool,
	dsName tc.DeliveryServiceName,
) (string, bool, bool) {
	dsServers, ok := sh.dsServers[dsName]
	if !ok {
		// should never happen (we found a match, but it wasn't in the list of ds servers
		fmt.Printf("EVENT: Request: %v czf zone %v requested A '%v' ds '%v' - match, but not in dsServers! should never happen! Returning ServFail\n", addr.String(), zone, domain, dsName, ok)
		return "", false, true // "", no refuse, servfail
	}

	cg := tc.CacheGroupName(zone)
	cgServers, ok := dsServers[cg]
	if !ok {
		// we found a match, but there were no servers in the found cachegroup with an Edge on this DS.
		fmt.Printf("EVENT: Request: %v czf zone %v requested A '%v' ds '%v' - match, but the requested DS had no servers in the matched cachegroup! Returning ServFail", addr.String(), zone, domain, dsName)
		return "", false, true // "", no refuse, servfail
	}

	dsServer, ok := getServer(cgServers, v4, sh.serverAvailable)
	if !ok {
		// we found a match, but there were no servers of the requested IP type on the CG assigned to the DS.
		fmt.Printf("EVENT: Request: %v czf zone %v requested A %v ds '%v' - match, but no servers of type IPv4=%v in the cg on the ds! Returning ServFail\n", addr.String(), zone, domain, dsName, ok, v4)
		return "", false, true // "", no refuse, servfail
	}

	fmt.Printf("EVENT: Request: '%v' czf zone '%v' requested A '%v' ds '%v' matched server '%+v', returning\n", addr.String(), zone, domain, dsName, dsServer)

	return dsServer.Addr, false, false // addr, no refuse, no servfail
}

// GetServerForDomainHTTP returns the server IP to return to the client, for the given HTTP DS' initial DNS request.
//
// Because it's an HTTP DS, the initial DNS request returns the IP of a Traffic Router
// (which will then be requested over HTTP by the client, and which will return a 302 to a cache).
//
func (sh *Shared) GetServerForDomainHTTP(
	addr net.Addr,
	zone string,
	domain string,
	v4 bool,
	dsName tc.DeliveryServiceName,
) (string, bool, bool) {
	// TODO add geolocation of routers
	routersMap := sh.crc.ContentRouters
	routers := []tc.CRConfigRouter{}
	for _, router := range routersMap {
		routers = append(routers, router)
	}
	if len(routers) == 0 {
		return "", false, true // "", no refuse, servfail - no routers = servfail. Also wtf, we're a router!?!
	}

	// If Routers had regular cache CGs, we could get the right CG here.
	// Since they don't, put all CGs in one big array to get.
	// TODO put self first (since the client got to us first in DNS, self should be nearest)

	allRouters := combineCGRouters(sh.cgRouters)
	router, ok := getRouter(allRouters, v4)

	if !ok {
		fmt.Printf("EVENT: Request: '%v' czf zone '%v' requested A '%v' http ds '%v' matched no router, returning servfail!\n", addr.String(), zone, domain, dsName)
		return "", false, true // "", no refuse, servfail
	}
	// TODO add Fallback CG failover
	// TODO if Fallback also fails, return self. Obviously.

	fmt.Printf("EVENT: Request: '%v' czf zone '%v' requested A '%v' http ds '%v' matched router server '%+v', returning\n", addr.String(), zone, domain, dsName, router)

	return router.Addr, false, false // addr, no refuse, no servfail
}

func getRouter(allServers DNSDSServers, v4 bool) (DNSDSServer, bool) {
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

	randI := rand.Intn(len(servers) - 1) // TODO change rand to consistent hash

	// TODO if we ever have Router health, it would be added here (like it is for the corresponding cache func)

	return servers[randI], true
}

func combineCGRouters(routers map[tc.CacheGroupName]DNSDSServers) DNSDSServers {
	// TODO this is done in the request path, and is unnecessary, and slow.
	//      If this stick around, it should be done when the CRConfig is fetched, and cached.
	svs := DNSDSServers{}
	for _, router := range routers {
		svs.V4s = append(svs.V4s, router.V4s...)
		svs.V6s = append(svs.V6s, router.V6s...)
	}
	return svs
}

// BuildHTTPSecondDNSMatches returns a map[fqdn]cache for the DNS FQDNs for HTTP Delivery Services.
//
// That is, for an HTTP DS, a client
// 1. Makes a DNS request to TR, and gets an IP back of TR itself
// 2. Makes a HTTP request to TR
// 3. Gets a 302 redirect to a FQDN, which is cache-host.ds-name.cdn-domain.
// 4. Makes a DNS request for that FQDN, and gets the IP of that cache
// 6. Makes an HTTP request to the cache.
//
// The matches here are for step 4, the second DNS lookup of an HTTP DS, for cache-name.ds-name.cdn-domain.
//
func BuildHTTPSecondDNSMatches(crc *tc.CRConfig, cdnDomain string) map[string]tc.CacheName {
	matches := map[string]tc.CacheName{}
	for svName, sv := range crc.ContentServers {
		for dsName, _ := range sv.DeliveryServices {
			// TODO only include HTTP DSes, exclude DNS DSes here.
			fqdn := svName + "." + dsName + "." + cdnDomain
			matches[fqdn] = tc.CacheName(svName)
		}
	}
	return matches
}
