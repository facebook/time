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

package phc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/facebook/time/phc/unix" // a temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut
	"github.com/facebook/time/servo"
)

// PPSSource represents a PPS source
type PPSSource struct {
	PHCDevice   DeviceController
	state       PPSSourceState
	peroutPhase int
	PPSPinIndex uint
}

// PPSSourceState represents the state of a PPS source
type PPSSourceState int

const (
	// UnknownStatus is the initial state of a PPS source, which means PPS may or may not be configured
	UnknownStatus PPSSourceState = iota
	// PPSSet means the underlying device is activated as a PPS source
	PPSSet
)

// PPS related constants
const (
	ptpPeroutDutyCycle     = (1 << 1)
	ptpPeroutPhase         = (1 << 2)
	defaultTs2PhcChannel   = 0
	DefaultTs2PhcIndex     = 3
	DefaultTs2PhcSinkIndex = 0
	defaultPulseWidth      = uint32(500000000)
	// should default to 0 if config specified. Otherwise -1 (ignore phase)
	defaultPeroutPhase = int32(-1) //nolint:all
	// ppsStartDelay is the delay in seconds before the first PPS signal is sent
	ppsStartDelay         = 2
	defaultPollerInterval = 40 * time.Millisecond
	PPSPollMaxAttempts    = 20
	defaultMaxFreqAdj     = 500000.0
)

// ServoController abstracts away servo
type ServoController interface {
	Sample(offset int64, localTs uint64) (float64, servo.State)
}

// Timestamper represents a device that can return a Timestamp
type Timestamper interface {
	Timestamp() (*time.Time, error)
}

// DeviceController defines a subset of functions to interact with a phc device. Enables mocking.
type DeviceController interface {
	Time() (time.Time, error)
	setPinFunc(index uint, pf int, ch uint) error
	setPTPPerout(req *PtpPeroutRequest) error
	File() *os.File
	AdjFreq(freq float64) error
	Step(offset time.Duration) error
	Read(buf []byte) (int, error)
	Fd() uintptr
	extTTSRequest(req *PtpExttsRequest) error
}

// PPSPoller represents a device which can be polled for PPS events
type PPSPoller interface {
	pollPPSSink() (time.Time, error)
}

// FrequencyGetter is an interface for getting PHC frequency and max frequency adjustment
type FrequencyGetter interface {
	MaxFreqAdjPPB() (float64, error)
	FreqPPB() (float64, error)
}

// PPSSink represents a device which is a sink of PPS signals
type PPSSink struct {
	InputPin        uint
	Polarity        uint32
	PulseWidth      uint32
	Device          DeviceController
	CollectedEvents []*time.Time
	pollDescriptor  unix.PollFd
}

// ActivatePPSSource configures the PHC device to be a PPS timestamp source
func ActivatePPSSource(dev DeviceController, pinIndex uint) (*PPSSource, error) {
	err := dev.setPinFunc(pinIndex, unix.PTP_PF_PEROUT, defaultTs2PhcChannel)
	if err != nil {
		log.Printf("Failed to set PPS Perout on pin index %d, channel %d, PHC %s. Error: %s. Continuing bravely on...",
			pinIndex, defaultTs2PhcChannel, dev.File().Name(), err)
	}

	ts, err := dev.Time()
	if err != nil {
		return nil, fmt.Errorf("failed (clock_gettime) on %s", dev.File().Name())
	}

	// Initialize the PTPPeroutRequest struct
	peroutRequest := &PtpPeroutRequest{}
	peroutRequest.Index = uint32(defaultTs2PhcChannel) // nolint:gosec
	peroutRequest.Period = PtpClockTime{Sec: 1, Nsec: 0}

	// Set flags and pulse width
	pulsewidth := defaultPulseWidth

	// TODO: skip this block if pulsewidth unset once pulsewidth is configurable
	peroutRequest.Flags |= ptpPeroutDutyCycle
	peroutRequest.On = PtpClockTime{
		Sec:  int64(pulsewidth / 1e9),
		Nsec: pulsewidth % 1e9,
	}

	// Set phase or start time
	// TODO: reintroduce peroutPhase != -1 condition once peroutPhase is configurable
	peroutRequest.StartOrPhase = PtpClockTime{
		Sec:  int64(ts.Second() + ppsStartDelay),
		Nsec: 0,
	}

	err = dev.setPTPPerout(peroutRequest)

	if err != nil {
		peroutRequest.Flags &^= ptpPeroutDutyCycle
		err = dev.setPTPPerout(peroutRequest)

		if err != nil {
			return nil, fmt.Errorf("error retrying PTP_PEROUT_REQUEST2 with DUTY_CYCLE flag unset for backwards compatibility, %w", err)
		}
	}

	return &PPSSource{PHCDevice: dev, state: PPSSet, PPSPinIndex: pinIndex}, nil
}

// Timestamp returns the timestamp of the last PPS output edge from the given PPS source
// A Pointer is returned to avoid additional memory allocation
func (ppsSource *PPSSource) Timestamp() (*time.Time, error) {
	if ppsSource.state != PPSSet {
		return nil, fmt.Errorf("PPS source not set")
	}

	currTime, err := ppsSource.PHCDevice.Time()
	if err != nil {
		return nil, fmt.Errorf("error getting time (clock_gettime) on %s", ppsSource.PHCDevice.File().Name())
	}

	// subtract device perout phase from current time to get the time of the last perout output edge
	// TODO: optimize section below using binary operations instead of type conversions
	currTime = currTime.Add(-time.Duration(ppsSource.peroutPhase))
	sourceTs, err := unix.TimeToTimespec(currTime)
	if err != nil {
		return nil, err
	}

	sourceTs.Nsec = 0
	//nolint:unconvert
	currTime = time.Unix(int64(sourceTs.Sec), int64(sourceTs.Nsec))
	currTime = currTime.Add(time.Duration(ppsSource.peroutPhase))

	return &currTime, nil
}

// NewPiServo returns a servo.PiServo object configure for synchronizing the given device. maxFreq 0 is equivalent to no maxFreq
func NewPiServo(interval time.Duration, firstStepth time.Duration, stepth time.Duration, device FrequencyGetter, maxFreq float64) (*servo.PiServo, error) {
	servoCfg := servo.DefaultServoConfig()
	if firstStepth != 0 {
		// allow stepping clock on first update
		servoCfg.FirstUpdate = true
		servoCfg.FirstStepThreshold = int64(firstStepth)
	}
	servoCfg.StepThreshold = int64(stepth)
	freq, err := device.FreqPPB()
	if err != nil {
		return nil, err
	}

	pi := servo.NewPiServo(servoCfg, servo.DefaultPiServoCfg(), -freq)
	pi.SyncInterval(interval.Seconds())

	if maxFreq == 0 {
		maxFreq, err = device.MaxFreqAdjPPB()
		if err != nil {
			log.Printf("unable to get max frequency adjustment from device, using default: %f", defaultMaxFreqAdj)
			maxFreq = defaultMaxFreqAdj
		}
	}

	pi.SetMaxFreq(maxFreq)

	return pi, nil
}

// PPSClockSync adjusts the frequency of the destination device based on the PPS from the ppsSource
func PPSClockSync(pi ServoController, ppsSource Timestamper, dstEventTimestamp time.Time, dstDevice DeviceController) error {
	srcTimestamp, err := ppsSource.Timestamp() // seconds set, nanosseconds set to 0
	if err != nil {
		return fmt.Errorf("error getting timestamp from PPS source: %w", err)
	}

	phcOffset := dstEventTimestamp.Sub(*srcTimestamp)
	//nolint:gosec
	freqAdj, servoState := pi.Sample(int64(phcOffset), uint64(dstEventTimestamp.UnixNano())) // unix nano is never negative

	log.Printf("%s offset %10d servo %s freq %+7.0f", dstDevice.File().Name(), int64(phcOffset), servoState.String(), freqAdj)

	switch servoState {
	case servo.StateJump:
		if err := dstDevice.Step(-phcOffset); err != nil {
			return fmt.Errorf("failed to step clock by %v: %w", -phcOffset, err)
		}
	case servo.StateLocked:
		if err := dstDevice.AdjFreq(-freqAdj); err != nil {
			return fmt.Errorf("failed to adjust freq to %v: %w", -freqAdj, err)
		}
	case servo.StateInit:
		return nil
	default:
		return fmt.Errorf("skipping clock update: servo state is %v", servoState)
	}
	return nil
}

// PPSSinkFromDevice configures the targetDevice to be a PPS sink and report PPS timestamp events
func PPSSinkFromDevice(targetDevice DeviceController, pinIndex uint) (*PPSSink, error) {
	pfd := unix.PollFd{
		Events: unix.POLLIN | unix.POLLPRI,
		Fd:     int32(targetDevice.Fd()),
	}
	ppsSink := PPSSink{
		InputPin:        pinIndex,
		Polarity:        unix.PTP_RISING_EDGE,
		PulseWidth:      defaultPulseWidth,
		Device:          targetDevice,
		CollectedEvents: []*time.Time{},
		pollDescriptor:  pfd,
	}

	req := &PtpExttsRequest{
		Flags: unix.PTP_ENABLE_FEATURE | ppsSink.Polarity,
		Index: uint32(ppsSink.InputPin), //nolint:gosec
	}

	err := targetDevice.setPinFunc(pinIndex, unix.PTP_PF_EXTTS, defaultTs2PhcChannel)
	if err != nil {
		return nil, fmt.Errorf("error setting extts input pin for device %s: %w", targetDevice.File().Name(), err)
	}

	err = targetDevice.extTTSRequest(req)
	if err != nil {
		return nil, fmt.Errorf("error during extts request for device %s: %w", targetDevice.File().Name(), err)
	}

	return &ppsSink, nil
}

// getPPSEventTimestamp reads the first PPS event from the sink file descriptor and returns the timestamp of the event
func (ppsSink *PPSSink) getPPSEventTimestamp() (time.Time, error) {
	var event PtpExttsEvent
	buf := make([]byte, binary.Size(event))
	_, err := ppsSink.Device.Read(buf)
	if err != nil {
		return time.Time{}, fmt.Errorf("error reading from sink %v: %w", ppsSink.Device.File().Name(), err)
	}
	event = *(*PtpExttsEvent)(unsafe.Pointer(&buf[0])) // urgh...
	if uint(event.Index) != ppsSink.InputPin {
		return time.Time{}, fmt.Errorf("extts on unexpected pin index %d, expected %d", event.Index, ppsSink.InputPin)
	}

	eventTime := event.T.Time()

	return eventTime, nil
}

// pollFd polls the file descriptor for an event, and returns the number of events, the file descriptor, and error
func pollFd(pfd unix.PollFd) (int, unix.PollFd, error) {
	fdSlice := []unix.PollFd{pfd}
	for {
		eventCount, err := unix.Poll(fdSlice, int(defaultPollerInterval.Milliseconds()))
		// swallow EINTR
		if !errors.Is(err, syscall.EINTR) {
			return eventCount, fdSlice[0], err
		}
	}
}

// pollPPSSink polls the sink for the timestamp of the first available PPS event
func (ppsSink *PPSSink) pollPPSSink() (time.Time, error) {
	eventCount, newPollDescriptor, err := pollFd(ppsSink.pollDescriptor)
	ppsSink.pollDescriptor = newPollDescriptor
	ppsSink.CollectedEvents = []*time.Time{}

	if err != nil || eventCount <= 0 {
		return time.Time{}, fmt.Errorf("error polling sink %s, event count: %d, error: %w", ppsSink.Device.File().Name(), eventCount, err)
	}

	if ppsSink.pollDescriptor.Revents&unix.POLLERR != 0 {
		return time.Time{}, fmt.Errorf("error polling sink %v: POLLERR", ppsSink.Device.File().Name())
	}

	result, err := ppsSink.getPPSEventTimestamp()
	if err != nil {
		return time.Time{}, fmt.Errorf("error while fetching pps event on sink %v: %w", ppsSink.Device.File().Name(), err)
	}
	return result, nil
}

// PollLatestPPSEvent returns the timestamp of the latest available PPS event
func PollLatestPPSEvent(ppsSink PPSPoller) (resultEvent time.Time, err error) {
	var newEvent time.Time

	for attempts := 0; attempts < PPSPollMaxAttempts; attempts++ {
		newEvent, err = ppsSink.pollPPSSink()
		if err != nil {
			// Error without resultEvent, we try again
			if resultEvent.IsZero() {
				continue
			}
			// Error but we already have an event, stop retrying
			return resultEvent, nil
		}
		// New event, it is the new result event
		resultEvent = newEvent
	}

	// we exceeded max attempts, so we return the last event
	return resultEvent, err
}
