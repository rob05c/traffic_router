# Traffic Router

Traffic Router is a prototype reimplementation of the Apache Traffic Control Traffic Router in Go.

Apache Traffic Control is an Open Source implementation of a Content Delivery Network.

Traffic Router is a component of Traffic Control which routes CDN requests. It is both an authoritative DNS server, and an HTTP server.

See:
https://github.com/apache/trafficcontrol
https://github.com/apache/trafficcontrol/tree/master/traffic_router

## Status

This is still a prototype. It's incomplete, and missing many features of ATC Traffic Router.

### Implemented

- DNS Delivery Service request handling
- Initial HTTP DNS request handling (edge.ds-name.cdn-domain.example)
- DNS handling for second HTTP lookup (edge-name.ds-name.cdn-domain.example)

### To Do

- Add maxmind to geolocation (currently just coverage zone file)
- Fix initial HTTP DNS req, which returns routers, to geo-locate and return closest, instead of random
- Add returning multiple servers to DNS reqs (both DNS and initial-HTTP DSes), based on DS settings
- Add HTTP server for HTTP Delivery Service requests
- DNSSEC
- Add HTTP Certificate handling, for HTTP Delivery Services
- Client Steering
- Add Capabilities handling
- Add Topologies handling
- Change server selection within CG to Consistent Hash, matching the existing Traffic Router, instead of random
- Add failover, if all servers in CacheGroup are Unavailable, use Fallback CacheGroup

### Performance

No real performance testing has been done yet.

Here is a list of potential performance bottlenecks which should be tested and potentially fixed.

- Fix to only geo-lookup after confirming FQDN is valid. Drastically improve performance under attack
- Use Tries for FQDN matches, faster lookups
