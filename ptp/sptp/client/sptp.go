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
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

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
	SetLastFreq(float64)
	MeanFreq() float64
	IsSpike(offset int64) bool
	GetState() servo.State
}

// SPTP is a Simple Unicast PTP client
type SPTP struct {
	cfg *Config

	pi Servo

	stats StatsServer

	clock Clock

	bestGM string

	clients    map[string]*Client
	priorities map[string]int
	backoff    map[string]*backoff
	lastTick   time.Time

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
	p.backoff = map[string]*backoff{}
	if p.cfg.ParallelTX {
		log.Info("Initialise clients with parallel TX feature")
	}
	for server, prio := range p.cfg.Servers {
		// normalize the address
		ip := net.ParseIP(server)
		ns := ip.String()
		econn := p.eventConn
		if p.cfg.ParallelTX {
			conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: ip, Port: ptp.PortEvent})
			if err != nil {
				return err
			}
			econn, err = NewUDPConnTSConfig(conn, p.cfg)
			if err != nil {
				return err
			}
		}
		c, err := NewClient(ns, ptp.PortEvent, p.clockID, econn, p.cfg, p.stats)
		if err != nil {
			return fmt.Errorf("initializing client %q: %w", ns, err)
		}
		p.clients[ns] = c
		p.priorities[ns] = prio
		p.backoff[ns] = newBackoff(p.cfg.Backoff)
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

	p.eventConn, err = NewUDPConnTSConfig(eventConn, p.cfg)
	if err != nil {
		return err
	}

	// Configure TX timestamp attempts and timemouts
	timestamp.AttemptsTXTS = p.cfg.AttemptsTXTS
	timestamp.TimeoutTXTS = p.cfg.TimeoutTXTS

	if p.cfg.FreeRunning {
		log.Warningf("operating in FreeRunning mode, will NOT adjust clock")
		p.clock = &FreeRunningClock{}
	} else {
		if p.cfg.Timestamping == timestamp.HW {
			phcDev, err := NewPHC(p.cfg.Iface)
			if err != nil {
				return err
			}
			p.clock = phcDev
		} else {
			p.clock = &SysClock{}
		}
	}

	freq, err := p.clock.FrequencyPPB()
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
	maxFreq, err := p.clock.MaxFreqPPB()
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

// ptping probing if packet is ptping before discarding it
// It's used for external pingers such as ptping and not required for sptp itself
func (p *SPTP) ptping(sourceIP net.IP, sourcePort int, response []byte, rxtx time.Time) error {
	// Has to be a delay request from ptping
	b := &ptp.SyncDelayReq{}
	if err := ptp.FromBytes(response, b); err != nil {
		return fmt.Errorf("failed to read delay request %w", err)
	}
	c, err := NewClient(sourceIP.String(), sourcePort, p.clockID, p.eventConn, p.cfg, p.stats)
	if err != nil {
		return fmt.Errorf("failed to respond to a delay request %w", err)
	}
	if err := c.handleDelayReq(rxtx); err != nil {
		return fmt.Errorf("failed to respond to a delay request %w", err)
	}
	return nil
}

// RunListener starts a listener, must be run before any client-server interactions happen
func (p *SPTP) RunListener(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
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
				if addr == nil {
					doneChan <- fmt.Errorf("received packet on port 320 with nil source address")
					return
				}
				log.Debugf("got packet on port 320, n = %v, addr = %v", n, addr)
				cc, found := p.clients[addr.IP.String()]
				if !found {
					log.Warningf("ignoring packets from server %v", addr)
					continue
				}
				cc.inChan <- &InPacket{data: response[:n]}
			}
		}()
		select {
		case <-ctx.Done():
			log.Debugf("cancelled general port receiver")
			return ctx.Err()
		case err := <-doneChan:
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
					log.Warningf("ignoring packets from server %v. Trying ptping", ip)
					// Try ptping
					if err = p.ptping(ip, timestamp.SockaddrToPort(addr), response, rxtx); err != nil {
						log.Warning(err)
					}
					continue
				}
				cc.inChan <- &InPacket{data: response, ts: rxtx}
			}
		}()
		select {
		case <-ctx.Done():
			log.Debugf("cancelled event port receiver")
			return ctx.Err()
		case err := <-doneChan:
			return err
		}
	})

	return eg.Wait()
}

func (p *SPTP) handleExchangeError(addr string, err error) {
	if errors.Is(err, errBackoff) {
		b := p.backoff[addr].tick()
		log.Debugf("backoff %s: %d seconds", addr, b)
	} else {
		log.Errorf("result %s: %+v", addr, err)
		b := p.backoff[addr].bump()
		if b != 0 {
			log.Warningf("backoff %s: extended by %d", addr, b)
		}
	}
}

func (p *SPTP) setMeanFreq() float64 {
	freqAdj := p.pi.MeanFreq()
	p.pi.SetLastFreq(freqAdj)
	if err := p.clock.AdjFreqPPB(-1 * freqAdj); err != nil {
		log.Errorf("failed to adjust freq to %v: %v", -1*freqAdj, err)
	}
	return freqAdj
}

// reprioritize is pushing former "best gm" to the back of the list
func (p *SPTP) reprioritize(bestAddr string) {
	// by how much we should shift the list
	k := p.priorities[bestAddr] - 1
	for addr, prio := range p.priorities {
		p.priorities[addr] = prio - k
		if p.priorities[addr] < 1 {
			// 0 -> 4, -1 -> 3...
			p.priorities[addr] = p.priorities[addr] + len(p.priorities)
		}
	}
}

func (p *SPTP) processResults(results map[string]*RunResult) {
	defer func() {
		for addr, res := range results {
			s := runResultToStats(addr, res, p.priorities[addr], addr == p.bestGM)
			p.stats.SetGMStats(s)
		}
	}()

	isBadTick := false
	now := time.Now()
	if !p.lastTick.IsZero() {
		tickDuration := now.Sub(p.lastTick)
		log.Debugf("tick took %vms sys time", tickDuration.Milliseconds())
		// +-10% of interval
		p.stats.SetTickDuration(tickDuration)
		if 100*tickDuration > 110*p.cfg.Interval || 100*tickDuration < 90*p.cfg.Interval {
			log.Warningf("tick took %vms, which is outside of expected +-10%% from the interval %vms", tickDuration.Milliseconds(), p.cfg.Interval.Milliseconds())
			isBadTick = true
		}
	}
	p.lastTick = now
	gmsTotal := len(results)
	gmsAvailable := 0
	announces := []*ptp.Announce{}
	idsToClients := map[ptp.ClockIdentity]string{}
	localPrioMap := map[ptp.ClockIdentity]int{}
	for addr, res := range results {
		if res.Error == nil {
			p.backoff[addr].reset()
			log.Debugf("result %s: %+v", addr, res.Measurement)
		} else {
			p.handleExchangeError(addr, res.Error)
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
	p.stats.SetGmsTotal(gmsTotal)
	if gmsTotal != 0 {
		p.stats.SetGmsAvailable(int((float64(gmsAvailable) / float64(gmsTotal)) * 100))
	} else {
		p.stats.SetGmsAvailable(0)
	}
	best := bmca(announces, localPrioMap, p.cfg)
	if best == nil {
		log.Warningf("no Best Master selected")
		p.bestGM = ""
		freqAdj := p.setMeanFreq()
		log.Infof("offset Unknown s%d freq %+7.0f path delay Unknown", servo.StateHoldover, freqAdj)
		return
	}
	bestAddr := idsToClients[best.GrandmasterIdentity]
	bm := results[bestAddr].Measurement
	if p.bestGM != bestAddr {
		p.reprioritize(bestAddr)
		log.Warningf("new best master selected: %q (%s)", bestAddr, bm.Announce.GrandmasterIdentity)
		p.bestGM = bestAddr
	}
	bmOffset := int64(bm.Offset)
	bmDelay := bm.Delay.Nanoseconds()
	log.Debugf("best master %q (%s)", bestAddr, bm.Announce.GrandmasterIdentity)
	isSpike := p.pi.IsSpike(bmOffset)
	var state servo.State
	var freqAdj float64
	if isSpike {
		p.stats.IncFiltered()
		freqAdj = p.setMeanFreq()
		if p.pi.GetState() == servo.StateLocked {
			state = servo.StateFilter
		} else {
			state = servo.StateInit
		}
		results[bestAddr].Measurement = nil
	} else if isBadTick {
		freqAdj = p.setMeanFreq()
		state = servo.StateHoldover
	} else {
		freqAdj, state = p.pi.Sample(bmOffset, uint64(bm.Timestamp.UnixNano()))
	}
	log.Infof("offset %10d s%d freq %+7.0f path delay %10d (%6d:%6d)", bmOffset, state, freqAdj, bmDelay, bm.C2SDelay, bm.S2CDelay)
	switch state {
	case servo.StateJump:
		if err := p.clock.Step(-1 * bm.Offset); err != nil {
			log.Errorf("failed to step freq by %v: %v", -1*bm.Offset, err)
		}
	case servo.StateLocked:
		if err := p.clock.AdjFreqPPB(-1 * freqAdj); err != nil {
			log.Errorf("failed to adjust freq to %v: %v", -1*freqAdj, err)
		}
		if err := p.clock.SetSync(); err != nil {
			log.Errorf("failed to set clock sync state")
		}
	}
}

func (p *SPTP) runInternal(ctx context.Context) error {
	p.pi.SyncInterval(p.cfg.Interval.Seconds())
	var lock sync.Mutex

	tick := func() {
		eg, ictx := errgroup.WithContext(ctx)
		results := map[string]*RunResult{}
		for addr, c := range p.clients {
			addr := addr
			c := c
			if p.backoff[addr].active() {
				// skip talking to this GM, we are in backoff mode
				lock.Lock()
				results[addr] = &RunResult{
					Server: addr,
					Error:  errBackoff,
				}
				lock.Unlock()
				continue
			}
			eg.Go(func() error {
				res := c.RunOnce(ictx, p.cfg.ExchangeTimeout)
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

	timer := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			log.Debugf("cancelled main loop")
			freqAdj := p.pi.MeanFreq()
			log.Infof("Existing, setting freq to: %v", -1*freqAdj)
			if err := p.clock.AdjFreqPPB(-1 * freqAdj); err != nil {
				log.Errorf("failed to adjust freq to %v: %v", -1*freqAdj, err)
			}
			return ctx.Err()
		case <-timer.C:
			timer.Reset(p.cfg.Interval)
			tick()
		}
	}
}

// Run makes things run, continuously
func (p *SPTP) Run(ctx context.Context) error {
	go func() {
		log.Debugf("starting listener")
		if err := p.RunListener(ctx); err != nil {
			log.Fatal(err)
		}
	}()
	return p.runInternal(ctx)
}
