package pollercrconfig

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/apache/trafficcontrol/lib/go-tc"
	"github.com/rob05c/traffic_router/poller"
	"github.com/rob05c/traffic_router/shared"
)

func MakePoller(interval time.Duration, monitors []string, shared *shared.Shared) (*poller.Poller, *IPoller) {
	// TODO make interval part of shared, for threadsafe updating
	iPoller := &IPoller{
		Monitors: monitors,
		Shared:   shared,
	}
	poller := &poller.Poller{
		Interval: interval,
		IPoller:  iPoller,
	}
	return poller, iPoller
}

// Poller polls Monitors every Interval, and updates the CRStates.
type IPoller struct {
	// monitors is the array of Traffic Monitor FQDNs to poll
	// TODO make Monitors part of Shared, so it can be set/get in a threadsafe manner
	Monitors []string // TODO change to URL?
	Shared   *shared.Shared

	currentMonitor int
}

func (po *IPoller) Reset() {
	po.currentMonitor = 0
}

func (po *IPoller) Poll() {
	if len(po.Monitors) == 0 {
		fmt.Println("ERROR: CRITICAL! pollercrstates: no monitors! Cannot poll!")
		return
	}

	triedMonitors := 0
	crConfig := &tc.CRConfig{}
	for {
		if triedMonitors == len(po.Monitors) {
			fmt.Println("ERROR: CRITICAL! pollercrconfig: all monitors failed, CRConfig Poll failed! Trying again after interval.")
		}

		monitorFQDN := po.Monitors[po.currentMonitor]
		urlStr := "http://" + monitorFQDN + "/publish/CrConfig"
		resp, err := http.Get(urlStr) // TODO use client, add timeouts
		po.currentMonitor = (po.currentMonitor + 1) % len(po.Monitors)
		triedMonitors++
		if err != nil {
			fmt.Println("ERROR: pollercrconfig: getting CRConfig from monitor '" + monitorFQDN + "', trying next monitor: " + err.Error())
			continue
		}
		defer resp.Body.Close()

		if err := json.NewDecoder(resp.Body).Decode(crConfig); err != nil {
			fmt.Println("ERROR: pollercrstates: decoding CRConfig from monitor '" + monitorFQDN + "', trying next monitor: " + err.Error())
			resp.Body.Close() // optimization
			continue
		}
		break
	}
	po.Shared.SetCRConfig(crConfig)
}
