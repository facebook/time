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

package daemon

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/facebook/time/fbclock"

	"github.com/facebook/time/ptp/linearizability"

	"github.com/facebook/time/phc"
	ptp "github.com/facebook/time/ptp/protocol"
)

var errNotEnoughData = fmt.Errorf("not enough data points")

// connect creates connection to unix socket in unixgram mode
func connect(address, local string, timeout time.Duration) (*net.UnixConn, error) {
	deadline := time.Now().Add(timeout)

	addr, err := net.ResolveUnixAddr("unixgram", address)
	if err != nil {
		return nil, err
	}
	localAddr, _ := net.ResolveUnixAddr("unixgram", local)
	conn, err := net.DialUnix("unixgram", localAddr, addr)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(local, 0666); err != nil {
		return nil, err
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	log.Debugf("connected to %s", address)
	return conn, nil
}

// dataPoint is what we store in datapoint ring buffer
type dataPoint struct {
	ingressTimeNS     int64
	masterOffsetNS    float64
	pathDelayNS       float64
	freqAdjustmentPPB float64
	clockAccuracyNS   float64
}

func (d *dataPoint) SanityCheck() error {
	if d.ingressTimeNS == 0 {
		return fmt.Errorf("ingress time is 0")
	}
	if d.masterOffsetNS == 0 {
		return fmt.Errorf("master offset is 0")
	}
	if d.pathDelayNS == 0 {
		return fmt.Errorf("path dealy is 0")
	}
	if d.freqAdjustmentPPB == 0 {
		return fmt.Errorf("frequency adjustment is 0")
	}
	if d.clockAccuracyNS == 0 {
		return fmt.Errorf("clock accuracy is 0")
	}
	if time.Duration(d.clockAccuracyNS) >= ptp.ClockAccuracyUnknown.Duration() {
		return fmt.Errorf("clock accuracy is unknown")
	}
	return nil
}

// Daemon is a component of fbclock that
// runs continuously,
// talks to ptp4l,
// does the math
// and populates shared memory for client library to read from.
type Daemon struct {
	//*fb303.FacebookBase
	//server thrift.Server

	cfg   *Config
	state *daemonState
	stats StatsServer
	l     Logger

	// function to get PHC time from configured PHC device
	getPHCTime func() (time.Time, error)
	// function to get PHC freq from configured PHC device
	getPHCFreqPPB func() (float64, error)
}

// minRingSize calculate how many datapoint we need to have in a ring buffer
// in order to provide aggregate values over 1 minute
func minRingSize(configuredRingSize int, interval time.Duration) int {
	size := configuredRingSize
	if time.Duration(size)*interval < time.Minute {
		size = int(math.Ceil(float64(time.Minute) / float64(interval)))
	}
	return size
}

// New creates new fbclock-daemon
func New(cfg *Config, stats StatsServer, l Logger) (*Daemon, error) {
	// we need at least 1m of samples for aggregate values
	effectiveRingSize := minRingSize(cfg.RingSize, cfg.Interval)
	s := &Daemon{
		stats: stats,
		state: newDaemonState(effectiveRingSize),
		cfg:   cfg,
		l:     l,
	}
	phcDevice, err := phc.IfaceToPHCDevice(cfg.Iface)
	if err != nil {
		return nil, fmt.Errorf("finding PHC device for %q: %w", cfg.Iface, err)
	}
	// function to get time from phc
	s.getPHCTime = func() (time.Time, error) { return phc.TimeFromDevice(phcDevice) }
	s.getPHCFreqPPB = func() (float64, error) { return phc.FrequencyPPBFromDevice(phcDevice) }
	// calculated values
	s.stats.SetCounter("m_ns", 0)
	s.stats.SetCounter("w_ns", 0)
	s.stats.SetCounter("drift_ppb", 0)
	s.stats.SetCounter("time_since_ingress_ns", 0)
	// error counters
	s.stats.SetCounter("data_error", 0)
	s.stats.SetCounter("phc_error", 0)
	s.stats.SetCounter("processing_error", 0)
	s.stats.SetCounter("data_sanity_check_error", 0)
	// values collected from ptp4l
	s.stats.SetCounter("ingress_time_ns", 0)
	s.stats.SetCounter("master_offset_ns", 0)
	s.stats.SetCounter("path_delay_ns", 0)
	s.stats.SetCounter("freq_adj_ppb", 0)
	s.stats.SetCounter("clock_accuracy_ns", 0)
	// aggregated values
	s.stats.SetCounter("master_offset_ns.60.abs_max", 0)
	s.stats.SetCounter("path_delay_ns.60.abs_max", 0)
	s.stats.SetCounter("freq_adj_ppb.60.abs_max", 0)
	return s, nil
}

func (s *Daemon) calcW() (float64, error) {
	lastN := s.state.takeDataPoint(s.cfg.RingSize)
	if len(lastN) != s.cfg.RingSize {
		return 0, fmt.Errorf("%w getting M: want %d, got %d", errNotEnoughData, s.cfg.RingSize, len(lastN))
	}
	params := prepareMathParameters(lastN)
	logSample := &LogSample{
		MasterOffsetNS:          params["offset"][0],
		MasterOffsetMeanNS:      mean(params["offset"]),
		MasterOffsetStddevNS:    stddev(params["offset"]),
		PathDelayNS:             params["delay"][0],
		PathDelayMeanNS:         mean(params["delay"]),
		PathDelayStddevNS:       stddev(params["delay"]),
		FreqAdjustmentPPB:       params["freq"][0],
		FreqAdjustmentMeanPPB:   mean(params["freq"]),
		FreqAdjustmentStddevPPB: stddev(params["freq"]),
		ClockAccuracyMean:       mean(params["clockaccuracie"]),
	}
	mRaw, err := s.cfg.Math.mExpr.Evaluate(mapOfInterface(params))
	if err != nil {
		return 0, err
	}
	m := mRaw.(float64)
	logSample.MeasurementNS = m
	s.stats.SetCounter("m_ns", int64(m))

	// push m to ring buffer
	s.state.pushM(m)

	ms := s.state.takeM(s.cfg.RingSize)
	if len(ms) != s.cfg.RingSize {
		return 0, fmt.Errorf("%w getting W: want %d, got %d", errNotEnoughData, s.cfg.RingSize, len(ms))
	}

	parameters := map[string]interface{}{
		"m": ms,
	}
	logSample.MeasurementMeanNS = mean(ms)
	logSample.MeasurementStddevNS = stddev(ms)

	wRaw, err := s.cfg.Math.wExpr.Evaluate(parameters)
	if err != nil {
		return 0, err
	}
	w := wRaw.(float64)
	logSample.WindowNS = w
	if err := s.l.Log(logSample); err != nil {
		log.Errorf("failed to log sample: %v", err)
	}
	s.stats.SetCounter("w_ns", int64(w))
	return w, nil
}

func (s *Daemon) calcDriftPPB() (float64, error) {
	lastN := s.state.takeDataPoint(s.cfg.RingSize)
	if len(lastN) != s.cfg.RingSize {
		return 0, fmt.Errorf("%w calculating drift: want %d, got %d", errNotEnoughData, s.cfg.RingSize, len(lastN))
	}
	params := prepareMathParameters(lastN)
	driftRaw, err := s.cfg.Math.driftExpr.Evaluate(mapOfInterface(params))
	if err != nil {
		return 0, err
	}
	drift := driftRaw.(float64)
	return drift, nil
}

func (s *Daemon) gmDataFromSocket(local string) (targets []string, err error) {
	timeout := s.cfg.Interval / 2
	conn, err := connect(s.cfg.PTP4Lsock, local, timeout)
	defer func() {
		if conn != nil {
			conn.Close()
			if f, err := conn.File(); err == nil {
				f.Close()
			}
		}
		// make sure there is no leftover socket
		os.RemoveAll(local)
	}()
	if err != nil {
		return targets, fmt.Errorf("failed to connect to ptp4l: %w", err)
	}

	c := &ptp.MgmtClient{
		Connection: conn,
	}
	tlv, err := c.UnicastMasterTableNP()
	if err != nil {
		return targets, fmt.Errorf("getting UNICAST_MASTER_TABLE_NP from ptp4l: %w", err)
	}

	for _, entry := range tlv.UnicastMasterTable.UnicastMasters {
		// skip the current best master
		if entry.Selected {
			continue
		}
		// skip GMs we didn't get announce from
		if entry.PortState == ptp.UnicastMasterStateWait {
			continue
		}
		server := entry.Address.String()
		targets = append(targets, server)
	}
	return
}

func (s *Daemon) dataFromSocket(local string) (*dataPoint, error) {
	timeout := s.cfg.Interval / 2
	conn, err := connect(s.cfg.PTP4Lsock, local, timeout)
	defer func() {
		if conn != nil {
			conn.Close()
			if f, err := conn.File(); err == nil {
				f.Close()
			}
		}
		// make sure there is no leftover socket
		os.RemoveAll(local)
	}()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ptp4l: %w", err)
	}

	c := &ptp.MgmtClient{
		Connection: conn,
	}
	status, err := c.TimeStatusNP()
	if err != nil {
		return nil, fmt.Errorf("failed to get TIME_STATUS_NP: %w", err)
	}
	log.Debugf("TIME_STATUS_NP: %+v", status)

	pds, err := c.ParentDataSet()
	if err != nil {
		return nil, fmt.Errorf("failed to get PARENT_DATA_SET: %w", err)
	}
	cds, err := c.CurrentDataSet()
	if err != nil {
		return nil, fmt.Errorf("failed to get CURRENT_DATA_SET: %w", err)
	}
	accuracyNS := pds.GrandmasterClockQuality.ClockAccuracy.Duration().Nanoseconds()

	return &dataPoint{
		ingressTimeNS:   status.IngressTimeNS,
		masterOffsetNS:  float64(status.MasterOffsetNS),
		pathDelayNS:     cds.MeanPathDelay.Nanoseconds(),
		clockAccuracyNS: float64(int64(status.GMPresent) * accuracyNS),
	}, nil
}

func (s *Daemon) calculateSHMData(data *dataPoint) (*fbclock.Data, error) {
	if err := data.SanityCheck(); err != nil {
		s.stats.UpdateCounterBy("data_sanity_check_error", 1)
		return nil, fmt.Errorf("sanity checking data point: %w", err)
	}
	s.stats.SetCounter("data_sanity_check_error", 0)

	// store datapoint in ring buffer
	s.state.pushDataPoint(data)

	// calculate W
	w, err := s.calcW()
	if err != nil {
		return nil, fmt.Errorf("calculating W: %w", err)
	}

	wUint := uint64(w)
	if wUint == 0 {
		return nil, fmt.Errorf("value of W is 0")
	}
	// drift is in PPB, parts per billion.
	// 1ns = 1/billions of second, so we can just say that
	// hValue is measured in ns per second
	hValue, err := s.calcDriftPPB()
	if err != nil {
		return nil, fmt.Errorf("calculating drift: %w", err)
	}
	s.stats.SetCounter("drift_ppb", int64(hValue))
	return &fbclock.Data{
		IngressTimeNS:        data.ingressTimeNS,
		ErrorBoundNS:         wUint,
		HoldoverMultiplierNS: hValue,
	}, nil
}

func (s *Daemon) doWork(shm *fbclock.Shm, data *dataPoint) error {
	// push stats
	s.stats.SetCounter("master_offset_ns", int64(data.masterOffsetNS))
	s.stats.SetCounter("path_delay_ns", int64(data.pathDelayNS))
	s.stats.SetCounter("ingress_time_ns", data.ingressTimeNS)
	s.stats.SetCounter("freq_adj_ppb", int64(data.freqAdjustmentPPB))
	s.stats.SetCounter("clock_accuracy_ns", int64(data.clockAccuracyNS))
	// try and calculate how long ago was the ingress time
	// use clock_gettime as the fastest and widely available method
	if phcTime, err := s.getPHCTime(); err != nil {
		log.Warningf("Failed to get PHC time from %s: %v", s.cfg.Iface, err)
	} else {
		if data.ingressTimeNS > 0 {
			s.state.updateIngressTimeNS(data.ingressTimeNS)
		}
		it := s.state.ingressTimeNS()
		if it > 0 {
			timeSinceIngress := phcTime.UnixNano() - it
			s.stats.SetCounter("time_since_ingress_ns", timeSinceIngress)
			log.Debugf("Time since ingress: %dns", timeSinceIngress)
		} else {
			log.Warningf("No data for time since ingress")
		}
	}
	// store everything in shared memory
	d, err := s.calculateSHMData(data)
	if err != nil {
		if errors.Is(err, errNotEnoughData) {
			log.Warning(err)
			return nil
		}
		return err
	}
	if err := fbclock.StoreFBClockData(shm.File.Fd(), *d); err != nil {
		return err
	}
	// aggregated stats over 1 minute
	maxDp := s.state.aggregateDataPointsMax(minRingSize(s.cfg.RingSize, s.cfg.Interval))
	s.stats.SetCounter("master_offset_ns.60.abs_max", int64(maxDp.masterOffsetNS))
	s.stats.SetCounter("path_delay_ns.60.abs_max", int64(maxDp.pathDelayNS))
	s.stats.SetCounter("freq_adj_ppb.60.abs_max", int64(maxDp.freqAdjustmentPPB))
	return nil
}

func targetsDiff(oldTargets []string, targets []string) (added []string, removed []string) {
	m := map[string]bool{}
	for _, k := range oldTargets {
		m[k] = true
	}
	for _, k := range targets {
		if m[k] {
			delete(m, k)
		} else {
			added = append(added, k)
		}
	}
	for k := range m {
		removed = append(removed, k)
	}
	return
}

func (s *Daemon) runLinearizabilityTests(ctx context.Context) {
	testers := map[string]*linearizability.Tester{}
	oldTargets := []string{}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m := new(sync.Mutex)

	local := filepath.Join("/var/run/", fmt.Sprintf("fbclock.%d.linear.sock", os.Getpid()))
	ticker := time.NewTicker(s.cfg.LinearizabilityTestInterval)
	defer ticker.Stop()
	for ; true; <-ticker.C { // first run without delay, then at interval
		eg := new(errgroup.Group)
		currentResults := map[string]*linearizability.TestResult{}

		targets, err := s.gmDataFromSocket(local)
		if err != nil {
			log.Errorf("getting linearizability test targets from ptp4l: %v", err)
			continue
		}
		log.Debugf("targets: %v, err: %v", targets, err)
		// log when set of targets changes
		added, removed := targetsDiff(oldTargets, targets)
		if len(added) > 0 || len(removed) > 0 {
			log.Infof("new set of linearizability test targets. Added: %v, Removed: %v. Resulting set: %v", added, removed, targets)
			// we never remove testers, as stopping background listener goroutine is not easy
			oldTargets = targets
		}

		for _, server := range targets {
			server := server
			log.Debugf("talking to %s", server)
			lt, found := testers[server]
			if !found { // create tester and start listener only once
				cfg := &linearizability.TestConfig{
					Timeout:   time.Second,
					Server:    server,
					Interface: s.cfg.Iface,
				}
				lt, err = linearizability.NewTester(cfg)
				if err != nil {
					log.Errorf("creating tester: %v", err)
					continue
				}
				testers[server] = lt
				go func() {
					server := server
					lt := lt
					if err := lt.RunListener(ctx); err != nil {
						log.Errorf("running listener for %s: %v", server, err)
					}
				}()
			}
			eg.Go(func() error {
				res := lt.RunTest(ctx)
				m.Lock()
				currentResults[server] = &res
				m.Unlock()
				return nil
			})
		}
		err = eg.Wait()
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error(err)
		}

		// get result, log it and push it into ring buffer
		for _, res := range currentResults {
			good, err := res.Good()
			if err != nil {
				log.Errorf(res.Explain())
				continue
			}
			if !good {
				log.Warningf(res.Explain())
			}
			log.Debugf("got linearizability result: %s", res.Explain())
			s.state.pushLinearizabilityTestResult(res)
		}
		// add stats from linearizability checks
		stats := linearizability.ProcessMonitoringResults("linearizability.", currentResults)
		for k, v := range stats {
			s.stats.SetCounter(k, int64(v))
		}
	}
}

func (s *Daemon) Run(ctx context.Context) error {
	shm, err := fbclock.OpenFBClockSHM()
	if err != nil {
		return fmt.Errorf("opening fbclock shm: %w", err)
	}
	defer shm.Close()

	if s.cfg.LinearizabilityTestInterval != 0 {
		go s.runLinearizabilityTests(ctx)
	}
	local := filepath.Join("/var/run/", fmt.Sprintf("fbclock.%d.sock", os.Getpid()))

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for ; true; <-ticker.C { // first run without delay, then at interval
		data, err := s.dataFromSocket(local)
		if err != nil {
			log.Error(err)
			s.stats.UpdateCounterBy("data_error", 1)
			continue
		}
		s.stats.SetCounter("data_error", 0)
		// get PHC freq adjustment
		freqPPB, err := s.getPHCFreqPPB()
		if err != nil {
			log.Error(err)
			s.stats.UpdateCounterBy("phc_error", 1)
			continue
		}
		s.stats.SetCounter("phc_error", 0)
		data.freqAdjustmentPPB = freqPPB
		if err := s.doWork(shm, data); err != nil {
			log.Error(err)
			s.stats.UpdateCounterBy("processing_error", 1)
			continue
		}
		s.stats.SetCounter("processing_error", 0)
	}
	return nil
}
