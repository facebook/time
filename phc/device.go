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
	"net"
	"unsafe"

	"github.com/vtolstov/go-ioctl"
	"golang.org/x/sys/unix"
)

// Missing from sys/unix package, defined in Linux include/uapi/linux/ptp_clock.h
const (
	ptpMaxSamples = 25
	ptpClkMagic   = '='
)

// ioctlPTPSysOffsetExtended is an IOCTL to get extended offset
var ioctlPTPSysOffsetExtended = ioctl.IOWR(ptpClkMagic, 9, unsafe.Sizeof(PTPSysOffsetExtended{}))

// ioctlPTPSysOffsetPrecise is an IOCTL to get precise offset
var ioctlPTPSysOffsetPrecise = ioctl.IOWR(ptpClkMagic, 8, unsafe.Sizeof(PTPSysOffsetPrecise{}))

// ioctlPTPClockGetCaps is an IOCTL to get PTP clock capabilities
var ioctlPTPClockGetcaps = ioctl.IOR(ptpClkMagic, 1, unsafe.Sizeof(PTPClockCaps{}))

// iocPinGetfunc is an IOCTL req corresponding to PTP_PIN_GETFUNC in linux/ptp_clock.h
var iocPinGetfunc = ioctl.IOWR(ptpClkMagic, 6, unsafe.Sizeof(rawPinDesc{}))

// iocPinSetfunc is an IOCTL req corresponding to PTP_PIN_SETFUNC in linux/ptp_clock.h
var iocPinSetfunc = ioctl.IOW(ptpClkMagic, 7, unsafe.Sizeof(rawPinDesc{}))

// Ifreq is the request we send with SIOCETHTOOL IOCTL
// as per Linux kernel's include/uapi/linux/if.h
type Ifreq struct {
	Name [unix.IFNAMSIZ]byte
	Data uintptr
}

// EthtoolTSinfo holds a device's timestamping and PHC association
// as per Linux kernel's include/uapi/linux/ethtool.h
type EthtoolTSinfo struct {
	Cmd            uint32
	SOtimestamping uint32
	PHCIndex       int32
	TXTypes        uint32
	TXReserved     [3]uint32
	RXFilters      uint32
	RXReserved     [3]uint32
}

// PTPSysOffsetExtended as defined in linux/ptp_clock.h
type PTPSysOffsetExtended struct {
	NSamples uint32    /* Desired number of measurements. */
	Reserved [3]uint32 /* Reserved for future use. */
	/*
	 * Array of [system, phc, system] time stamps. The kernel will provide
	 * 3*n_samples time stamps.
	 * - system time right before reading the lowest bits of the PHC timestamp
	 * - PHC time
	 * - system time immediately after reading the lowest bits of the PHC timestamp
	 */
	TS [ptpMaxSamples][3]PTPClockTime
}

// PTPSysOffsetPrecise as defined in linux/ptp_clock.h
type PTPSysOffsetPrecise struct {
	Device      PTPClockTime
	SysRealTime PTPClockTime
	SysMonoRaw  PTPClockTime
	Reserved    [4]uint32 /* Reserved for future use. */
}

// PinDesc represents the C struct ptp_pin_desc as defined in linux/ptp_clock.h
type PinDesc struct {
	Name  string  // Hardware specific human readable pin name
	Index uint    // Pin index in the range of zero to ptp_clock_caps.n_pins - 1
	Func  PinFunc // Which of the PTP_PF_xxx functions to use on this pin
	Chan  uint    // The specific channel to use for this function.
}

type rawPinDesc struct {
	Name  [64]byte  // Hardware specific human readable pin name
	Index uint32    // Pin index in the range of zero to ptp_clock_caps.n_pins - 1
	Func  uint32    // Which of the PTP_PF_xxx functions to use on this pin
	Chan  uint32    // The specific channel to use for this function.
	Rsv   [5]uint32 // Reserved for future use.
}

// PTPClockCaps as defined in linux/ptp_clock.h
type PTPClockCaps struct {
	MaxAdj  int32 /* Maximum frequency adjustment in parts per billon. */
	NAalarm int32 /* Number of programmable alarms. */
	NExtTs  int32 /* Number of external time stamp channels. */
	NPerOut int32 /* Number of programmable periodic signals. */
	PPS     int32 /* Whether the clock supports a PPS callback. */
	NPins   int32 /* Number of input/output pins. */
	/* Whether the clock supports precise system-device cross timestamps */
	CrossTimestamping int32
	/* Whether the clock supports adjust phase */
	AdjustPhase int32
	Rsv         [12]int32 /* Reserved for future use. */
}

func (caps *PTPClockCaps) maxAdj() float64 {
	if caps == nil || caps.MaxAdj == 0 {
		return DefaultMaxClockFreqPPB
	}
	return float64(caps.MaxAdj)
}

// IfaceInfo uses SIOCETHTOOL ioctl to get information for the give nic, i.e. eth0.
func IfaceInfo(iface string) (*EthtoolTSinfo, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create socket for ioctl: %w", err)
	}
	defer unix.Close(fd)
	// this is what we want to be populated, but we need to provide Cmd first
	data := &EthtoolTSinfo{
		Cmd: unix.ETHTOOL_GET_TS_INFO,
	}
	// actual request we send
	ifreq := &Ifreq{}
	// set Name in the request
	copy(ifreq.Name[:unix.IFNAMSIZ-1], iface)
	// pointer to the data we need to be populated
	ifreq.Data = uintptr(unsafe.Pointer(data))
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL, uintptr(fd),
		uintptr(unix.SIOCETHTOOL),
		uintptr(unsafe.Pointer(ifreq)),
	)
	if errno != 0 {
		return nil, fmt.Errorf("failed get phc ID: %w", errno)
	}
	return data, nil
}

// IfaceData has both net.Interface and EthtoolTSinfo
type IfaceData struct {
	Iface  net.Interface
	TSInfo EthtoolTSinfo
}

// IfacesInfo is like net.Interfaces() but with added EthtoolTSinfo
func IfacesInfo() ([]IfaceData, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	res := []IfaceData{}
	for _, iface := range ifaces {
		data, err := IfaceInfo(iface.Name)
		if err != nil {
			return nil, err
		}
		res = append(res,
			IfaceData{
				Iface:  iface,
				TSInfo: *data,
			})
	}
	return res, nil
}

// DeviceFromIface returns a path to a PHC device from a network interface
func DeviceFromIface(iface string) (string, error) {
	ifaces, err := IfacesInfo()
	if err != nil {
		return "", err
	}
	if len(ifaces) == 0 {
		return "", fmt.Errorf("no network devices found")
	}

	for _, d := range ifaces {
		if d.Iface.Name == iface {
			if d.TSInfo.PHCIndex < 0 {
				return "", fmt.Errorf("no PHC support for %s", iface)
			}
			return fmt.Sprintf("/dev/ptp%d", d.TSInfo.PHCIndex), nil
		}
	}
	return "", fmt.Errorf("%s interface is not found", iface)
}
