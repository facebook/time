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
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DefaultMaxClockFreqPPB value came from linuxptp project (clockadj.c)
const DefaultMaxClockFreqPPB = 500000.0

// Time returns PTPClockTime as time.Time
func (t PTPClockTime) Time() time.Time {
	return time.Unix(t.Sec, int64(t.NSec))
}

// TimeMethod is method we use to get time
type TimeMethod string

// Methods we support to get time
const (
	MethodSyscallClockGettime    TimeMethod = "syscall_clock_gettime"
	MethodIoctlSysOffsetExtended TimeMethod = "ioctl_PTP_SYS_OFFSET_EXTENDED"
)

// SupportedMethods is a list of supported TimeMethods
var SupportedMethods = []TimeMethod{MethodSyscallClockGettime, MethodIoctlSysOffsetExtended}

// PinFunc type represents the pin function values.
type PinFunc int

// Type implements cobra.Value
func (pf *PinFunc) Type() string { return "{ PPS-In | PPS-Out | PhySync | None }" }

// String implements flags.Value
func (pf PinFunc) String() string {
	switch pf {
	case PinFuncNone:
		return "None"
	case PinFuncExtTS:
		return "PPS-In" // user friendly
	case PinFuncPerOut:
		return "PPS-Out" // user friendly
	case PinFuncPhySync:
		return "PhySync"
	default:
		return fmt.Sprintf("!(PinFunc=%d)", int(pf))
	}
}

// Set implements flags.Value
func (pf *PinFunc) Set(s string) error {
	switch strings.ToLower(s) {
	case "none", "-":
		*pf = PinFuncNone
	case "pps-in", "ppsin", "extts":
		*pf = PinFuncExtTS
	case "pps-out", "ppsout", "perout":
		*pf = PinFuncPerOut
	case "phy-sync", "physync", "sync":
		*pf = PinFuncPhySync
	default:
		return fmt.Errorf("use either of: %s", pf.Type())
	}
	return nil
}

// Pin functions corresponding to `enum ptp_pin_function` in linux/ptp_clock.h
const (
	PinFuncNone    PinFunc = iota // PTP_PF_NONE
	PinFuncExtTS                  // PTP_PF_EXTTS
	PinFuncPerOut                 // PTP_PF_PEROUT
	PinFuncPhySync                // PTP_PF_PHYSYNC
)

func ifaceInfoToPHCDevice(info *EthtoolTSinfo) (string, error) {
	if info.PHCIndex < 0 {
		return "", fmt.Errorf("interface doesn't support PHC")
	}
	return fmt.Sprintf("/dev/ptp%d", info.PHCIndex), nil
}

// IfaceToPHCDevice returns path to PHC device associated with given network card iface
func IfaceToPHCDevice(iface string) (string, error) {
	info, err := IfaceInfo(iface)
	if err != nil {
		return "", fmt.Errorf("getting interface %s info: %w", iface, err)
	}
	return ifaceInfoToPHCDevice(info)
}

// Time returns time we got from network card
func Time(iface string, method TimeMethod) (time.Time, error) {
	device, err := IfaceToPHCDevice(iface)
	if err != nil {
		return time.Time{}, err
	}

	f, err := os.Open(device)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	dev := FromFile(f)

	switch method {
	case MethodSyscallClockGettime:
		return dev.Time()
	case MethodIoctlSysOffsetExtended:
		extended, err := dev.ReadSysoffExtended1()
		if err != nil {
			return time.Time{}, err
		}
		latest := extended.TS[extended.NSamples-1]
		return latest[1].Time(), nil
	default:
		return time.Time{}, fmt.Errorf("unknown method to get PHC time %q", method)
	}
}

// Device represents a PHC device
type Device os.File

// DeviceController defines a subset of functions to interact with a phc device. Enables mocking.
type DeviceController interface {
	Time() (time.Time, error)
	setPinFunc(index uint, pf PinFunc, ch uint) error
	setPTPPerout(req PTPPeroutRequest) error
	File() *os.File
}

// FromFile returns a *Device corresponding to an *os.File
func FromFile(file *os.File) *Device { return (*Device)(file) }

// File returns the underlying *os.File
func (dev *Device) File() *os.File { return (*os.File)(dev) }

// Fd returns the underlying file descriptor
func (dev *Device) Fd() uintptr { return dev.File().Fd() }

// ClockID derives the clock ID from the file descriptor number - see clock_gettime(3), FD_TO_CLOCKID macros
func (dev *Device) ClockID() int32 { return int32((int(^dev.Fd()) << 3) | 3) }

// Time returns time from the PTP device using the clock_gettime syscall
func (dev *Device) Time() (time.Time, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(dev.ClockID(), &ts); err != nil {
		return time.Time{}, fmt.Errorf("failed clock_gettime: %w", err)
	}
	return time.Unix(ts.Unix()), nil
}

// ioctl makes a unis.SYS_IOCTL unix.Syscall with the given device, request and argument
func (dev *Device) ioctl(req uintptr, arg unsafe.Pointer) (err error) {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, dev.Fd(), req, uintptr(arg))
	if errno != 0 {
		err = fmt.Errorf("errno %w during IOCTL %d on FD %s", errno, req, dev.File().Name())
	}
	return err
}

// ReadSysoffExtended reads the precise time from the PHC along with SYS time to measure the call delay.
// The nsamples parameter is set to ExtendedNumProbes.
func (dev *Device) ReadSysoffExtended() (*PTPSysOffsetExtended, error) {
	return dev.readSysoffExtended(ExtendedNumProbes)
}

// ReadSysoffExtended1 reads the precise time from the PHC along with SYS time to measure the call delay.
// The nsamples parameter is set to 1.
func (dev *Device) ReadSysoffExtended1() (*PTPSysOffsetExtended, error) {
	return dev.readSysoffExtended(1)
}

// ReadSysoffPrecise reads the precise time from the PHC along with SYS time to measure the call delay.
func (dev *Device) ReadSysoffPrecise() (*PTPSysOffsetPrecise, error) {
	return dev.readSysoffPrecise()
}

func (dev *Device) readSysoffExtended(nsamples int) (*PTPSysOffsetExtended, error) {
	res := &PTPSysOffsetExtended{
		NSamples: uint32(nsamples),
	}
	err := dev.ioctl(ioctlPTPSysOffsetExtended, unsafe.Pointer(res))
	if err != nil {
		return nil, fmt.Errorf("failed PTP_SYS_OFFSET_EXTENDED: %w", err)
	}
	return res, nil
}

func (dev *Device) readSysoffPrecise() (*PTPSysOffsetPrecise, error) {
	res := &PTPSysOffsetPrecise{}
	if err := dev.ioctl(ioctlPTPSysOffsetPrecise, unsafe.Pointer(res)); err != nil {
		return nil, fmt.Errorf("failed PTP_SYS_OFFSET_PRECISE: %w", err)
	}
	return res, nil
}

// readCaps reads PTP capabilities using ioctl
func (dev *Device) readCaps() (*PTPClockCaps, error) {
	caps := &PTPClockCaps{}
	if err := dev.ioctl(ioctlPTPClockGetcaps, unsafe.Pointer(caps)); err != nil {
		return nil, fmt.Errorf("clock didn't respond properly: %w", err)
	}
	return caps, nil
}

// readPinDesc reads a single PTP pin descriptor
func (dev *Device) readPinDesc(index int, desc *PinDesc) error {
	var raw rawPinDesc

	raw.Index = uint32(index)
	if err := dev.ioctl(iocPinGetfunc, unsafe.Pointer(&raw)); err != nil {
		return fmt.Errorf("%s: ioctl(PTP_PIN_GETFUNC) failed: %w", dev.File().Name(), err)
	}
	desc.Name = unix.ByteSliceToString(raw.Name[:])
	desc.Index = uint(raw.Index)
	desc.Func = PinFunc(raw.Func)
	desc.Chan = uint(raw.Chan)
	desc.dev = dev
	return nil
}

// readPinDesc reads a single PTP pin descriptor
func (dev *Device) setPinFunc(index uint, pf PinFunc, ch uint) error {
	var raw rawPinDesc

	raw.Index = uint32(index) //#nosec G115
	raw.Func = uint32(pf)     //#nosec G115
	raw.Chan = uint32(ch)     //#nosec G115
	if err := dev.ioctl(iocPinSetfunc, unsafe.Pointer(&raw)); err != nil {
		return fmt.Errorf("%s: ioctl(PTP_PIN_SETFUNC) failed: %w", dev.File().Name(), err)
	}
	return nil
}

// ReadPins reads all PTP pin descriptors
func (dev *Device) ReadPins() ([]PinDesc, error) {
	caps, err := dev.readCaps()
	if err != nil {
		return nil, err
	}
	npins := int(caps.NPins)
	desc := make([]PinDesc, npins)
	for i := 0; i < npins; i++ {
		if err := dev.readPinDesc(i, &desc[i]); err != nil {
			return nil, err
		}
	}
	return desc, nil
}

// MaxFreqAdjPPB reads max value for frequency adjustments (in PPB) from ptp device
func (dev *Device) MaxFreqAdjPPB() (maxFreq float64, err error) {
	caps, err := dev.readCaps()
	if err != nil {
		return 0, err
	}
	return caps.maxAdj(), nil
}

func (dev *Device) setPTPPerout(req PTPPeroutRequest) error {
	return dev.ioctl(ioctlPTPPeroutRequest2, unsafe.Pointer(&req))
}

// FreqPPB reads PHC device frequency in PPB (parts per billion)
func (dev *Device) FreqPPB() (freqPPB float64, err error) { return freqPPBFromDevice(dev) }

// AdjFreq adjusts the PHC clock frequency in PPB
func (dev *Device) AdjFreq(freqPPB float64) error { return clockAdjFreq(dev, freqPPB) }

// Step steps the PHC clock by given duration
func (dev *Device) Step(step time.Duration) error { return clockStep(dev, step) }
