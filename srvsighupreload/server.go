package srvsighupreload

import (
	"crypto/tls"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/signal"
	"time"

	"github.com/rob05c/traffic_router/config"
	"github.com/rob05c/traffic_router/loadconfig"
	"github.com/rob05c/traffic_router/pollercrstates"
	"github.com/rob05c/traffic_router/shared"
	"github.com/rob05c/traffic_router/srvdns"
	"github.com/rob05c/traffic_router/srvhttp"
)

// Listen starts listening for SIGHUP signals (typical of service reload commands), and reloads the config file when it receives one.
// The config file it reloads is the one received as a command-line argument on startup. The startup file given may not be changed without a restart.
// Likewise, the ports being served on require a restart to change.
// If there is an error loading the config file, the error is logged, and the existing server is left unchanged.
func Listen(
	filename string,
	dnsServer *srvdns.ServerPtr,
	httpServer *srvhttp.ServerPtr,
	certGetter *srvhttp.CertGetter,
	crStatesPoller *pollercrstates.Poller,
) {
	// TODO add the abiliity to change ports.
	//      (will require stopping the old servers and creating new ones, presumably passing in pointers to them)
	c := make(chan os.Signal, 1)
	sig := unix.SIGHUP
	signal.Notify(c, sig)
	for range c {
		TryReloadConfig(filename, dnsServer, httpServer, certGetter, crStatesPoller)
	}
}

// TryReloadConfig attemps to reload the given config file, and set the server pointers to its reloaded state.
// On error, logs but leaves the servers serving what they were before, does not crash or stop.
func TryReloadConfig(
	fileName string,
	dnsServer *srvdns.ServerPtr,
	httpServer *srvhttp.ServerPtr,
	certGetter *srvhttp.CertGetter,
	crStatesPoller *pollercrstates.Poller,
) {
	shared, cfg, err := loadconfig.LoadConfig(fileName)
	if err != nil {
		fmt.Println("ERROR: reloading config file '" + fileName + "' new config not updated! : " + err.Error())
		return
	}

	UpdateCerts(shared.GetCerts(), certGetter)
	UpdateCRStatesPoller(crStatesPoller, cfg, shared)
	dnsServer.Set(&srvdns.Server{Shared: shared})
	httpServer.Set(&srvhttp.Server{Shared: shared})
	fmt.Println("INFO reloaded config file")
}

// UpdateCerts updates certGetter with certs, deleting certs in the getter and not in certs, and adding to the getter new certificates in certs but not in certGetter.
func UpdateCerts(certs map[string]*tls.Certificate, certGetter *srvhttp.CertGetter) {
	hosts := certGetter.Hosts()
	for host, _ := range hosts {
		if _, ok := certs[host]; ok {
			continue
		}
		certGetter.Delete(host)
	}
	for host, cert := range certs {
		// TODO This currently updates all certs.
		//      Add reading the file timestamp, and not reloading certs that haven't changed.
		certGetter.Add(host, cert)
	}
}

func UpdateCRStatesPoller(crStatesPoller *pollercrstates.Poller, cfg *config.Config, shared *shared.Shared) {
	if err := crStatesPoller.Stop(); err != nil {
		fmt.Println("ERROR: updating CRStates Poller: stopping: " + err.Error())
	}

	crStatesPoller.Monitors = cfg.Monitors
	crStatesPoller.Shared = shared
	crStatesPoller.Interval = time.Duration(cfg.CRStatesPollIntervalMS) * time.Millisecond

	if err := crStatesPoller.Start(); err != nil {
		fmt.Println("ERROR: updating CRStates Poller: starting: " + err.Error())
		// TODO fatal?
	}
}
