package pollercrstates

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/apache/trafficcontrol/lib/go-tc"
	"github.com/rob05c/traffic_router/shared"
)

var ErrNotStarted = errors.New("not started")
var ErrAlreadyStarted = errors.New("already started")
var ErrNoMonitors = errors.New("no monitors")
var ErrNoPollInterval = errors.New("no poll interval")

// Poller polls Monitors every Interval, and updates the CRStates.
type Poller struct {
	// monitors is the array of Traffic Monitor FQDNs to poll
	Monitors []string // TODO change to URL?
	Shared   *shared.Shared
	Interval time.Duration

	stopChan       chan struct{}
	started        bool
	currentMonitor int
}

func (po *Poller) Start() error {
	if len(po.Monitors) == 0 {
		return ErrNoMonitors
	}
	if po.Interval == 0 {
		return ErrNoPollInterval
	}
	if po.started {
		return ErrAlreadyStarted
	}
	po.currentMonitor = 0
	if po.stopChan == nil {
		po.stopChan = make(chan struct{})
	}
	go poll(po)
	po.started = true
	return nil
}

func (po *Poller) Stop() error {
	if !po.started {
		return ErrNotStarted
	}
	<-po.stopChan
	po.started = false
	return nil
}

func poll(po *Poller) {
	timer := time.NewTimer(po.Interval)
	for {
		select {
		case <-timer.C:
			pollMonitor(po)
			timer.Reset(po.Interval)
		case <-po.stopChan:
			if !timer.Stop() {
				<-timer.C
			}
			return
		}
	}
}

func pollMonitor(po *Poller) {
	triedMonitors := 0
	crStates := &tc.CRStates{}
	for {
		if triedMonitors == len(po.Monitors) {
			fmt.Println("ERROR: CRITICAL! pollercrstates: all monitors failed, CRStates Poll failed! Trying again after interval.")
		}

		monitorFQDN := po.Monitors[po.currentMonitor]
		urlStr := "http://" + monitorFQDN + "/publish/CrStates"
		resp, err := http.Get(urlStr) // TODO use client, add timeouts
		po.currentMonitor = (po.currentMonitor + 1) % len(po.Monitors)
		triedMonitors++
		if err != nil {
			fmt.Println("ERROR: pollercrstates: getting CRStates from monitor '" + monitorFQDN + "', trying next monitor: " + err.Error())
			continue
		}
		defer resp.Body.Close()

		if err := json.NewDecoder(resp.Body).Decode(crStates); err != nil {
			fmt.Println("ERROR: pollercrstates: decoding CRStates from monitor '" + monitorFQDN + "', trying next monitor: " + err.Error())
			resp.Body.Close() // optimization
			continue
		}
		break
	}
	po.Shared.SetCRStates(crStates)
}
