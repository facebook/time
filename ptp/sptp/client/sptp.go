/*
Copyright (c) Facebook, Inc. and its affiliates.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"

	"github.com/facebook/time/phc"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/servo"
	"github.com/facebook/time/timestamp"
)

// Servo abstracts away servo
type Servo interface {
	SyncInterval(float64)
	Sample(offset int64, localTs uint64) (float64, servo.State)
	SetMaxFreq(float64)
	MeanFreq() float64
}

// SPTP is a Simple Unicast PTP client
type SPTP struct {
	cfg *Config

	pi Servo

	stats StatsServer

	phc PHCIface

	bestGM string

	clients    map[string]*Client
	priorities map[string]int

	clockID ptp.ClockIdentity
	genConn UDPConn
	// listening connection on port 319
	eventConn UDPConnWithTS
}

// NewSPTP creates SPTP client
func NewSPTP(cfg *Config, stats StatsServer) (*SPTP, error) {
	p := &SPTP{
		cfg:   cfg,
		stats: stats,
	}
	if err := p.init(); err != nil {
		return nil, err
	}
	if err := p.initClients(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *SPTP) initClients() error {
	p.clients = map[string]*Client{}
	p.priorities = map[string]int{}
	for server, prio := range p.cfg.Servers {
		// normalize the address
		ns := net.ParseIP(server).String()
		c, err := newClient(ns, p.clockID, p.eventConn, &p.cfg.Measurement, p.stats)
		if err != nil {
			return fmt.Errorf("initializing client %q: %w", ns, err)
		}
		p.clients[ns] = c
		p.priorities[ns] = prio
	}
	return nil
}

func (p *SPTP) init() error {
	iface, err := net.InterfaceByName(p.cfg.Iface)
	if err != nil {
		return err
	}

	cid, err := ptp.NewClockIdentity(iface.HardwareAddr)
	if err != nil {
		return err
	}
	p.clockID = cid

	// bind to general port
	genConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("::"), Port: ptp.PortGeneral})
	if err != nil {
		return err
	}
	p.genConn = genConn
	// bind to event port
	eventConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("::"), Port: ptp.PortEvent})
	if err != nil {
		return err
	}

	// get FD of the connection. Can be optimized by doing this when connection is created
	connFd, err := timestamp.ConnFd(eventConn)
	if err != nil {
		return err
	}

	localEventAddr := eventConn.LocalAddr()
	localEventIP := localEventAddr.(*net.UDPAddr).IP
	if err = enableDSCP(connFd, localEventIP, p.cfg.DSCP); err != nil {
		return fmt.Errorf("setting DSCP on event socket: %w", err)
	}

	// we need to enable HW or SW timestamps on event port
	switch p.cfg.Timestamping {
	case "": // auto-detection
		if err = timestamp.EnableHWTimestamps(connFd, p.cfg.Iface); err != nil {
			if err = timestamp.EnableSWTimestamps(connFd); err != nil {
				return fmt.Errorf("failed to enable timestamps on port %d: %w", ptp.PortEvent, err)
			}
			log.Warningf("Failed to enable hardware timestamps on port %d, falling back to software timestamps", ptp.PortEvent)
		} else {
			log.Infof("Using hardware timestamps")
		}
	case HWTIMESTAMP:
		if err = timestamp.EnableHWTimestamps(connFd, p.cfg.Iface); err != nil {
			return fmt.Errorf("failed to enable hardware timestamps on port %d: %w", ptp.PortEvent, err)
		}
	case SWTIMESTAMP:
		if err = timestamp.EnableSWTimestamps(connFd); err != nil {
			return fmt.Errorf("failed to enable software timestamps on port %d: %w", ptp.PortEvent, err)
		}
	default:
		return fmt.Errorf("unknown type of typestamping: %q", p.cfg.Timestamping)
	}
	// set it to blocking mode, otherwise recvmsg will just return with nothing most of the time
	if err = unix.SetNonblock(connFd, false); err != nil {
		return fmt.Errorf("failed to set event socket to blocking: %w", err)
	}
	p.eventConn = newUDPConnTS(eventConn)

	// Configure TX timestamp attempts and timemouts
	timestamp.AttemptsTXTS = p.cfg.AttemptsTXTS
	timestamp.TimeoutTXTS = p.cfg.TimeoutTXTS

	phcDev, err := NewPHC(p.cfg.Iface)
	if err != nil {
		return err
	}
	p.phc = phcDev

	freq, err := p.phc.FrequencyPPB()
	log.Debugf("starting PHC frequency: %v", freq)
	if err != nil {
		return err
	}

	servoCfg := servo.DefaultServoConfig()
	// update first step threshold if it's configured
	if p.cfg.FirstStepThreshold != 0 {
		// allow stepping clock on first update
		servoCfg.FirstUpdate = true
		servoCfg.FirstStepThreshold = int64(p.cfg.FirstStepThreshold)
	}
	pi := servo.NewPiServo(servoCfg, servo.DefaultPiServoCfg(), -freq)
	maxFreq, err := p.phc.MaxFreqPPB()
	if err != nil {
		log.Warningf("max PHC frequency error: %v", err)
		maxFreq = phc.DefaultMaxClockFreqPPB
	} else {
		pi.SetMaxFreq(maxFreq)
	}
	log.Debugf("max PHC frequency: %v", maxFreq)
	piFilterCfg := servo.DefaultPiServoFilterCfg()
	servo.NewPiServoFilter(pi, piFilterCfg)
	p.pi = pi
	return nil
}

// RunListener starts a listener, must be run before any client-server interactions happen
func (p *SPTP) RunListener(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	var err error
	// get packets from general port
	eg.Go(func() error {
		// it's done in non-blocking way, so if context is cancelled we exit correctly
		doneChan := make(chan error, 1)
		go func() {
			for {
				response := make([]uint8, 1024)
				n, addr, err := p.genConn.ReadFromUDP(response)
				if err != nil {
					doneChan <- err
					return
				}
				log.Debugf("got packet on port 320, n = %v, addr = %v", n, addr)
				cc, found := p.clients[addr.IP.String()]
				if !found {
					log.Warningf("ignoring packets from server %v", addr)
					continue
				}
				cc.inChan <- &inPacket{data: response[:n]}
			}
		}()
		select {
		case <-ctx.Done():
			log.Debugf("cancelled general port receiver")
			return ctx.Err()
		case err = <-doneChan:
			return err
		}
	})
	// get packets from event port
	eg.Go(func() error {
		// it's done in non-blocking way, so if context is cancelled we exit correctly
		doneChan := make(chan error, 1)
		go func() {
			for {
				response, addr, rxtx, err := p.eventConn.ReadPacketWithRXTimestamp()
				if err != nil {
					doneChan <- err
					return
				}
				log.Debugf("got packet on port 319, addr = %v", addr)
				ip := timestamp.SockaddrToIP(addr)
				cc, found := p.clients[ip.String()]
				if !found {
					log.Warningf("ignoring packets from server %v", ip)
					continue
				}
				cc.inChan <- &inPacket{data: response, ts: rxtx}
			}
		}()
		select {
		case <-ctx.Done():
			log.Debugf("cancelled event port receiver")
			return ctx.Err()
		case err = <-doneChan:
			return err
		}
	})

	return eg.Wait()
}

func (p *SPTP) processResults(results map[string]*RunResult) {
	gmsTotal := len(results)
	gmsAvailable := 0
	announces := []*ptp.Announce{}
	idsToClients := map[ptp.ClockIdentity]string{}
	localPrioMap := map[ptp.ClockIdentity]int{}
	for addr, res := range results {
		s := runResultToStats(addr, res, p.priorities[addr], addr == p.bestGM)
		p.stats.SetGMStats(s)
		if res.Error == nil {
			log.Debugf("result %s: %+v", addr, res.Measurement)
		} else {
			log.Errorf("result %s: %+v", addr, res.Error)
			continue
		}
		if res.Measurement == nil {
			log.Errorf("result for %s is missing Measurement", addr)
			continue
		}
		gmsAvailable++
		announces = append(announces, &res.Measurement.Announce)
		idsToClients[res.Measurement.Announce.GrandmasterIdentity] = addr
		localPrioMap[res.Measurement.Announce.GrandmasterIdentity] = p.priorities[addr]
	}
	p.stats.SetCounter("sptp.gms.total", int64(gmsTotal))
	if gmsTotal != 0 {
		p.stats.SetCounter("sptp.gms.available_pct", int64((float64(gmsAvailable)/float64(gmsTotal))*100))
	} else {
		p.stats.SetCounter("sptp.gms.available_pct", int64(0))
	}
	best := bmca(announces, localPrioMap)
	if best == nil {
		log.Warningf("no Best Master selected")
		return
	}
	bestAddr := idsToClients[best.GrandmasterIdentity]
	bm := results[bestAddr].Measurement
	if p.bestGM != bestAddr {
		log.Warningf("new best master selected: %q (%s)", bestAddr, bm.Announce.GrandmasterIdentity)
		p.bestGM = bestAddr
	}

	log.Infof("best master: %v, offset: %v, delay: %v", bestAddr, bm.Offset, bm.Delay)
	freqAdj, state := p.pi.Sample(int64(bm.Offset), uint64(bm.Timestamp.UnixNano()))
	log.Infof("freqAdj: %v, state: %s(%d)", freqAdj, state, state)
	switch state {
	case servo.StateJump:
		if err := p.phc.Step(-1 * bm.Offset); err != nil {
			log.Errorf("failed to step freq by %v: %v", -1*bm.Offset, err)
		}
	default:
		if err := p.phc.AdjFreqPPB(-1 * freqAdj); err != nil {
			log.Errorf("failed to adjust freq to %v: %v", -1*freqAdj, err)
		}
	}
}

func (p *SPTP) runInternal(ctx context.Context, interval time.Duration) error {
	timeout := time.Duration(0.1 * float64(interval))
	p.pi.SyncInterval(interval.Seconds())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var lock sync.Mutex

	tick := func() {
		eg, ictx := errgroup.WithContext(ctx)
		results := map[string]*RunResult{}
		for addr, c := range p.clients {
			addr := addr
			c := c
			eg.Go(func() error {
				res := c.RunOnce(ictx, timeout)
				lock.Lock()
				defer lock.Unlock()
				results[addr] = res
				return nil
			})
		}
		err := eg.Wait()
		if err != nil {
			log.Errorf("run failed: %v", err)
		}
		p.processResults(results)
	}

	tick()
	for {
		select {
		case <-ctx.Done():
			log.Debugf("cancelled main loop")
			freqAdj := p.pi.MeanFreq()
			log.Infof("Existing, setting freq to: %v", -1*freqAdj)
			if err := p.phc.AdjFreqPPB(-1 * freqAdj); err != nil {
				log.Errorf("failed to adjust freq to %v: %v", -1*freqAdj, err)
			}
			return ctx.Err()
		case <-ticker.C:
			tick()
		}
	}
}

// Run makes things run, continuously
func (p *SPTP) Run(ctx context.Context, interval time.Duration) error {
	go func() {
		log.Debugf("starting listener")
		if err := p.RunListener(ctx); err != nil {
			log.Fatal(err)
		}
	}()
	return p.runInternal(ctx, interval)
}
