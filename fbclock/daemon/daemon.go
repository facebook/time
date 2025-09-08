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
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/facebook/time/fbclock"
	"github.com/facebook/time/fbclock/stats"

	"github.com/facebook/time/leapsectz"
	"github.com/facebook/time/phc"
	"github.com/facebook/time/ptp/linearizability"
	ptp "github.com/facebook/time/ptp/protocol"
)

const (
	utcOffsetOriginalS int32  = 10    // UTC-TAI offset was 10s before leap seconds started (1972)
	leapDurationS      uint64 = 62500 // 17.36 hours https://chrony-project.org/doc/4.6/chrony.conf.html
	monPrefix          string = "linearizability."
)

var errNotEnoughData = errors.New("not enough data points")
var errNoTestResults = errors.New("no test results")
var errNoPHC = errors.New("phc error")
var errCorrectness = errors.New("sanity checking data point error")

// defaultTargets is a list of targets if no available
var defaultTargets = []string{"::1", "::2", "::3"}

// DataPoint is what we store in DataPoint ring buffer
type DataPoint struct {
	// IngressTimeNS represents ingress time in NanoSeconds
	IngressTimeNS int64
	// MasterOffsetNS represents master offset in NanoSeconds
	MasterOffsetNS float64
	// PathDelayNS represents path delay in NanoSeconds
	PathDelayNS float64
	// FreqAdjustmentPPB represents freq adjustment in parts per billion
	FreqAdjustmentPPB float64
	// ClockAccuracyNS represents clock accurary in nanoseconds
	ClockAccuracyNS float64
}

// SanityCheck checks datapoint for correctness
func (d *DataPoint) SanityCheck() error {
	if d.IngressTimeNS == 0 {
		return fmt.Errorf("ingress time is 0")
	}
	if d.MasterOffsetNS == 0 {
		return fmt.Errorf("master offset is 0")
	}
	if d.PathDelayNS == 0 {
		return fmt.Errorf("path delay is 0")
	}
	if d.FreqAdjustmentPPB == 0 {
		return fmt.Errorf("frequency adjustment is 0")
	}
	if d.ClockAccuracyNS == 0 {
		return fmt.Errorf("clock accuracy is 0")
	}
	if time.Duration(d.ClockAccuracyNS) >= ptp.ClockAccuracyUnknown.Duration() {
		return fmt.Errorf("clock accuracy is unknown")
	}
	return nil
}

// DataFetcher is the data fetcher interface
type DataFetcher interface {
	//function to gm data
	FetchGMs(cfg *Config) (targest []string, err error)
	//function to fetch stats
	FetchStats(cfg *Config) (*DataPoint, error)
}

// Daemon is a component of fbclock that
// runs continuously,
// talks to ptp4l,
// does the math
// and populates shared memory for client library to read from.
type Daemon struct {
	DataFetcher
	cfg   *Config
	state *daemonState
	stats stats.Server
	l     Logger

	// function to get PHC time from configured PHC device
	getPHCTime       func() (time.Time, error)
	getPHCAndSysTime func() (time.Time, time.Time, uint32, error)
	// function to get PHC freq from configured PHC device
	getPHCFreqPPB func() (float64, error)
}

// minRingSize calculate how many DataPoint we need to have in a ring buffer
// in order to provide aggregate values over 1 minute
func minRingSize(configuredRingSize int, interval time.Duration) int {
	size := configuredRingSize
	if time.Duration(size)*interval < time.Minute {
		size = int(math.Ceil(float64(time.Minute) / float64(interval)))
	}
	return size
}

type clockSmearing struct {
	smearingStartS uint64 // time (TAI) when smearing starts
	smearingEndS   uint64 // time (TAI) when smearing ends
	utcOffsetPreS  int32  // DTAI offset prior to Leap Second Event Time
	utcOffsetPostS int32  // DTAI offset post Leap Second Event Time
}

func leapSeconds() ([]leapsectz.LeapSecond, error) {
	leaps, err := leapsectz.Parse("")
	if err != nil {
		return []leapsectz.LeapSecond{}, err
	}
	if len(leaps) < 2 {
		return []leapsectz.LeapSecond{}, fmt.Errorf("not enough leap seconds in the file")
	}

	previousLeap := leaps[len(leaps)-2]
	latestLeap := leaps[len(leaps)-1]

	return []leapsectz.LeapSecond{previousLeap, latestLeap}, nil
}

func leapSecondSmearing(leaps []leapsectz.LeapSecond) *clockSmearing {
	// need a minimum of 2 published leap second events in tzdata
	if len(leaps) < 2 {
		return &clockSmearing{}
	}
	latestLeap := leaps[len(leaps)-1]
	previousLeap := leaps[len(leaps)-2]
	utcOffsetPreS := previousLeap.Nleap + utcOffsetOriginalS
	utcOffsetPostS := latestLeap.Nleap + utcOffsetOriginalS

	// this is the leap second adjustment time which is either 23:59:60 UTC or 00:00:00 UTC of following day
	// if we don't render a timestamp of 23:59:60 UTC
	leapSecondEventTimeS := latestLeap.Tleap - uint64(latestLeap.Nleap) + 1
	// smearing starts at leap second event time and ends 18.06 hours after
	smearingStartS := leapSecondEventTimeS + uint64(utcOffsetPreS)
	smearingEndS := leapSecondEventTimeS + leapDurationS + uint64(utcOffsetPreS)

	return &clockSmearing{
		smearingStartS: smearingStartS,
		smearingEndS:   smearingEndS,
		utcOffsetPreS:  utcOffsetPreS,
		utcOffsetPostS: utcOffsetPostS,
	}
}

// noTestResults generates a map of error test results
func noTestResults(targets []string) map[string]linearizability.TestResult {
	r := map[string]linearizability.TestResult{}
	for _, target := range targets {
		r[target] = linearizability.SPTPHTTPTestResult{Error: errNoTestResults}
	}
	return r
}

// New creates new fbclock-daemon
func New(cfg *Config, stats stats.Server, l Logger) (*Daemon, error) {
	// we need at least 1m of samples for aggregate values
	effectiveRingSize := minRingSize(cfg.RingSize, cfg.Interval)
	s := &Daemon{
		stats: stats,
		state: newDaemonState(effectiveRingSize),
		cfg:   cfg,
		l:     l,
	}
	if cfg.SPTP {
		s.DataFetcher = &HTTPFetcher{}
	} else {
		s.DataFetcher = &SockFetcher{}
	}

	phcDevice, err := phc.IfaceToPHCDevice(cfg.Iface)
	if err != nil {
		return nil, fmt.Errorf("finding PHC device for %q: %w", cfg.Iface, err)
	}

	// Keep file open for the lifetime of the fbclock
	f, err := os.OpenFile(phcDevice, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	dev := phc.FromFile(f)

	// function to get time from phc
	s.getPHCTime = func() (time.Time, error) { return dev.Time() }
	s.getPHCAndSysTime = func() (time.Time, time.Time, uint32, error) {
		data, err := dev.ReadSysoffExtended()
		if err != nil {
			return time.Time{}, time.Time{}, 0, err
		}
		best := data.BestSample()
		phcTime := best.PHCTime
		monoTime := best.SysTime
		return phcTime, monoTime, uint32(best.SysClockID), nil //nolint:gosec
	}
	s.getPHCFreqPPB = func() (float64, error) { return dev.FreqPPB() }
	// calculated values
	s.stats.SetCounter("m_ns", 0)
	s.stats.SetCounter("w_ns", 0)
	s.stats.SetCounter("drift_ppb", 0)
	// error counters
	s.stats.SetCounter("data_error", 0)
	s.stats.SetCounter("processing_error", 0)
	s.stats.SetCounter("data_sanity_check_error", 0)
	s.stats.SetCounter("monotonictime_error", 0)
	s.stats.SetCounter("phc_read_error", 0)
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

func (s *Daemon) calculateSHMData(data *DataPoint, leaps []leapsectz.LeapSecond) (*fbclock.Data, error) {
	if err := data.SanityCheck(); err != nil {
		s.stats.UpdateCounterBy("data_sanity_check_error", 1)
		return nil, fmt.Errorf("%w: %w", errCorrectness, err)
	}
	s.stats.SetCounter("data_sanity_check_error", 0)

	// store DataPoint in ring buffer
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

	clockSmearing := leapSecondSmearing(leaps)
	return &fbclock.Data{
		IngressTimeNS:        data.IngressTimeNS,
		ErrorBoundNS:         wUint,
		HoldoverMultiplierNS: hValue,
		SmearingStartS:       clockSmearing.smearingStartS,
		SmearingEndS:         clockSmearing.smearingEndS,
		UTCOffsetPreS:        clockSmearing.utcOffsetPreS,
		UTCOffsetPostS:       clockSmearing.utcOffsetPostS,
	}, nil
}

func (s *Daemon) doWork(shm *fbclock.Shm, data *DataPoint) error {
	// push stats
	s.stats.SetCounter("master_offset_ns", int64(data.MasterOffsetNS))
	s.stats.SetCounter("path_delay_ns", int64(data.PathDelayNS))
	s.stats.SetCounter("ingress_time_ns", data.IngressTimeNS)
	s.stats.SetCounter("freq_adj_ppb", int64(data.FreqAdjustmentPPB))
	s.stats.SetCounter("clock_accuracy_ns", int64(data.ClockAccuracyNS))
	// try and calculate how long ago was the ingress time
	// use clock_gettime as the fastest and widely available method
	phcTime, err := s.getPHCTime()
	if err != nil {
		return fmt.Errorf("Failed to get PHC time from %s: %w", s.cfg.Iface, errors.Join(errNoPHC, err))
	}

	if data.IngressTimeNS > 0 {
		s.state.updateIngressTimeNS(data.IngressTimeNS)
	}
	it := s.state.ingressTimeNS()
	if it > 0 {
		timeSinceIngress := phcTime.UnixNano() - it
		log.Debugf("Time since ingress: %dns", timeSinceIngress)
	} else {
		log.Warning("No data for time since ingress")
	}
	// read tzdata for leap seconds
	leaps, err := leapSeconds()
	if err != nil {
		log.Warningf("Failed to get leap seconds: %v", err)
	}
	// store everything in shared memory
	d, err := s.calculateSHMData(data, leaps)
	if err != nil {
		if errors.Is(err, errNotEnoughData) {
			log.Warning(err)
			return nil
		}
		return err
	}
	s.state.lastStoredData = d
	if err := fbclock.StoreFBClockData(shm.File.Fd(), *d); err != nil {
		return err
	}
	// aggregated stats over 1 minute
	maxDp := s.state.aggregateDataPointsMax(minRingSize(s.cfg.RingSize, s.cfg.Interval))
	s.stats.SetCounter("master_offset_ns.60.abs_max", int64(maxDp.MasterOffsetNS))
	s.stats.SetCounter("path_delay_ns.60.abs_max", int64(maxDp.PathDelayNS))
	s.stats.SetCounter("freq_adj_ppb.60.abs_max", int64(maxDp.FreqAdjustmentPPB))
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
	testers := map[string]linearizability.Tester{}
	oldTargets := []string{}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m := new(sync.Mutex)

	ticker := time.NewTicker(s.cfg.LinearizabilityTestInterval)
	defer ticker.Stop()
	for ; true; <-ticker.C { // first run without delay, then at interval
		eg := new(errgroup.Group)
		currentResults := map[string]linearizability.TestResult{}
		targets, err := s.DataFetcher.FetchGMs(s.cfg)
		if err != nil {
			log.Errorf("getting linearizability test targets: %v", err)
			if len(oldTargets) > 0 {
				linearizability.ProcessMonitoringResults(monPrefix, noTestResults(oldTargets), s.stats)
			} else {
				linearizability.ProcessMonitoringResults(monPrefix, noTestResults(defaultTargets), s.stats)
			}
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
			if !found {
				if s.cfg.SPTP {
					lt, err = linearizability.NewSPTPHTTPTester(server, fmt.Sprintf("http://%s/", s.cfg.PTPClientAddress), s.cfg.LinearizabilityTestMaxGMOffset)
				} else {
					lt, err = linearizability.NewPTPTester(server, s.cfg.Iface, linearizability.IEEE1588)
				}
				if err != nil {
					log.Errorf("creating tester: %v", err)
					continue
				}
				testers[server] = lt
			}
			eg.Go(func() error {
				res := lt.RunTest(ctx)
				m.Lock()
				currentResults[server] = res
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
				log.Error(res.Explain())
				continue
			}
			if !good {
				log.Warning(res.Explain())
			}
			log.Debugf("got linearizability result: %s", res.Explain())
			s.state.pushLinearizabilityTestResult(res)
		}
		// add stats from linearizability checks
		linearizability.ProcessMonitoringResults(monPrefix, currentResults, s.stats)
	}
}

func calcCoeffPPB(prev, cur *fbclock.DataV2) (int64, error) {
	if prev.SysclockTimeNS == 0 {
		// first run, no previous data
		return 0, nil
	}
	// calculate the ratio of PHC time to system time (how faster (or slower) PHC ticks compared to system time)
	mCoeff := float64(cur.PHCTimeNS-prev.PHCTimeNS) / float64(cur.SysclockTimeNS-prev.SysclockTimeNS)
	// calculate the ratio in parts per billion (PPB)
	coefPPB := int64(mCoeff*float64(time.Second) - float64(time.Second))
	// check continuity of extrapolated PHC time
	sysTimeDiff := cur.SysclockTimeNS - prev.SysclockTimeNS - 1
	phcTimeFixed := prev.PHCTimeNS + sysTimeDiff + coefPPB*sysTimeDiff/time.Second.Nanoseconds()
	var err error
	if phcTimeFixed > cur.PHCTimeNS {
		err = fmt.Errorf("PHC time is not monotonic: %d > %d", phcTimeFixed, cur.PHCTimeNS)
	}
	return coefPPB, err
}

// populateDataV2 populates fbclock.DataV2 with data from fbclock.Data plus calculated values
func (s *Daemon) populateDataV2(shmv2 *fbclock.Shm) {
	prevDataV2 := fbclock.DataV2{}

	fastTicker := time.NewTicker(10 * time.Millisecond)
	defer fastTicker.Stop()
	for ; true; <-fastTicker.C { // first run without delay, then at interval
		if s.state.lastStoredData != nil {
			// we need a copy of the latest v1 data to fill in parts of v2 data
			// the pointer dereference is needed to avoid partial reads of the data
			curData := *s.state.lastStoredData
			phcTime, sysTime, clockID, err := s.getPHCAndSysTime()
			if err != nil {
				log.Errorf("reading PHC time from %s: %v", s.cfg.Iface, err)
				s.stats.UpdateCounterBy("phc_read_error", 1)
				continue
			}
			dataV2 := fbclock.DataV2{
				IngressTimeNS:        curData.IngressTimeNS,
				ErrorBoundNS:         curData.ErrorBoundNS,
				HoldoverMultiplierNS: curData.HoldoverMultiplierNS,
				SmearingStartS:       curData.SmearingStartS,
				UTCOffsetPreS:        int16(curData.UTCOffsetPreS),  //nolint:gosec
				UTCOffsetPostS:       int16(curData.UTCOffsetPostS), //nolint:gosec
				PHCTimeNS:            phcTime.UnixNano(),
				SysclockTimeNS:       sysTime.UnixNano(),
				ClockID:              clockID,
				CoefPPB:              0,
			}
			if dataV2.CoefPPB, err = calcCoeffPPB(&prevDataV2, &dataV2); err != nil {
				log.Warning(err)
				s.stats.UpdateCounterBy("monotonictime_error", 1)
			}
			if err := fbclock.StoreFBClockDataV2(shmv2.File.Fd(), dataV2); err != nil {
				log.Errorf("writing dataV2 to shm: %v", err)
			}
			prevDataV2 = dataV2
		}
	}
}

// Run a daemon
func (s *Daemon) Run(ctx context.Context) error {
	shm, err := fbclock.OpenFBClockSHM()
	if err != nil {
		return fmt.Errorf("opening fbclock shm: %w", err)
	}
	defer shm.Close()

	if s.cfg.LinearizabilityTestInterval != 0 {
		go s.runLinearizabilityTests(ctx)
	}

	if s.cfg.EnableDataV2 {
		shmv2, err := fbclock.OpenFBClockSHMv2()
		if err != nil {
			return fmt.Errorf("opening fbclock shm v2: %w", err)
		}
		defer shmv2.Close()
		go s.populateDataV2(shmv2)
	}

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for ; true; <-ticker.C { // first run without delay, then at interval
		data, err := s.DataFetcher.FetchStats(s.cfg)
		if err != nil {
			log.Error(err)
			s.stats.UpdateCounterBy("data_error", 1)
			continue
		}
		s.stats.SetCounter("data_error", 0)
		// get PHC freq adjustment
		freqPPB, err := s.getPHCFreqPPB()
		if err != nil {
			return err
		}
		data.FreqAdjustmentPPB = freqPPB
		if err := s.doWork(shm, data); err != nil {
			if errors.Is(err, errNoPHC) {
				return err
			} else if errors.Is(err, errCorrectness) {
				log.Warning(err)
			} else {
				log.Error(err)
			}
			s.stats.UpdateCounterBy("processing_error", 1)
			continue
		}
		s.stats.SetCounter("processing_error", 0)
	}
	return nil
}
