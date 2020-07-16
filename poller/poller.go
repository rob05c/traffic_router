package poller

import (
	"errors"
	"time"
)

var ErrNotStarted = errors.New("not started")
var ErrAlreadyStarted = errors.New("already started")
var ErrNoPollInterval = errors.New("no poll interval")
var ErrNoIPoller = errors.New("no IPoller object")

type IPoller interface {
	Poll()
	Reset()
}

type iPoller struct {
	f func()
}

func (ip *iPoller) Poll() {
	ip.f()
}

func (ip *iPoller) Reset() {}

// MakeIPoller takes a poll func and returns an IPoller object
func MakeIPoller(f func()) IPoller {
	return &iPoller{f: f}
}

type Poller struct {
	Interval time.Duration
	IPoller  IPoller

	stopChan chan struct{}
	started  bool
}

func (po *Poller) Start() error {
	if po.Interval == 0 {
		return ErrNoPollInterval
	}
	if po.IPoller == nil {
		return ErrNoIPoller
	}
	if po.started {
		return ErrAlreadyStarted
	}
	if po.stopChan == nil {
		po.stopChan = make(chan struct{})
	}
	po.IPoller.Reset()
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
			po.IPoller.Poll()
			timer.Reset(po.Interval)
		case <-po.stopChan:
			if !timer.Stop() {
				<-timer.C
			}
			return
		}
	}
}
