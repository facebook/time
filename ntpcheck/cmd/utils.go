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

package cmd

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"net"
	"syscall"
	"unsafe"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/facebookincubator/ntp/protocol/ntp"
	"github.com/facebookincubator/ntp/leaphash"
)

// cannot import sys/timex.h
const clockMonotonic = 4

const (
	timeOK = iota
	timeIns
	timeDel
	timeOOP
	timeWait
	timeError
)

// man 2 adjtimex
var timexToDesc = map[int]string{
	timeOK: "TIME_OK		Clock synchronized, no leap second adjustment pending.",
	timeIns: "TIME_INS	Indicates that a leap second will be added at the end of the UTC day.",
	timeDel: "TIME_DEL	Indicates that a leap second will be deleted at the end of the UTC day.",
	timeOOP: "TIME_OOP	Insertion of a leap second is in progress.",
	timeWait: "TIME_WAIT	A leap-second insertion or deletion has been completed.",
	timeError: "TIME_ERROR	The system clock is not synchronized to a reliable server.",
}

func getRawMonotonic() float64 {
	var ts syscall.Timespec
	_, _, _ = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, clockMonotonic, uintptr(unsafe.Pointer(&ts)), 0)
	return float64(ts.Sec) + float64(ts.Nsec)/float64(1e9)
}

// track reports wall time / mono time difference on stdout
func track(interval time.Duration) {
	fmt.Println("Wall timestamp\t\t\tWall difference\tMono difference\tMono raw diff\tOffset mono\tOffset mono raw")
	startTime := time.Now()
	rawStart := getRawMonotonic()

	var prevWallElapsed time.Duration
	var prevMonoElapsed time.Duration
	for {
		now := time.Now()
		nowMonotonic := getRawMonotonic()

		wallElapsed := now.Truncate(0).Sub(startTime.Truncate(0))
		monoElapsed := now.Sub(startTime)
		wallElapsedS := float64(wallElapsed) / float64(time.Second)
		monoElapsedS := float64(monoElapsed) / float64(time.Second)
		monoRawElapsedS := nowMonotonic - rawStart
		offsetMonoUs := float64(wallElapsed-monoElapsed) / float64(time.Microsecond)
		offsetMonoRawUs := float64(wallElapsed)/float64(time.Microsecond) - monoRawElapsedS*float64(1e6)
		fmt.Printf("[%s]\t%.7fs\t%.7fs\t%.7fs\t%.2fus\t\t%.2fus\n", now.Format(time.RFC3339), wallElapsedS, monoElapsedS, monoRawElapsedS, offsetMonoUs, offsetMonoRawUs)

		if (prevWallElapsed != 0) && int64(wallElapsed-prevWallElapsed) < 1000*int64(time.Millisecond) {
			fmt.Println("^^^ BANG! Wall time goes back")
		}
		if (prevMonoElapsed != 0) && int64(monoElapsed-prevMonoElapsed) < 1000*int64(time.Millisecond) {
			fmt.Println("^^^ BANG! Monotonic time goes back")
		}
		prevWallElapsed = wallElapsed
		prevMonoElapsed = monoElapsed

		time.Sleep(interval)
	}
}

// refID converts ip into ReFID format and prints it on stdout
func refID(ipStr string) error {
	if ipStr == "" {
		return fmt.Errorf("Error: no IP provided")
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("%q is not a valid IP address", ipStr)
	}

	// output IPv4 as-is
	ipv4 := ip.To4()
	if ipv4 != nil {
		fmt.Println(ipv4)
		return nil
	}

	hashed := md5.Sum(ip)
	fmt.Println(net.IPv4(hashed[0], hashed[1], hashed[2], hashed[3]).String())
	return nil
}

// fakeSeconds displays N fake leap seconds on stdout
func fakeSeconds(secondsCount int) {
	ntpEpoch := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)
	now := time.Now()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// the timestamp in this format is number of seconds from NTP Epoch
	fmt.Printf("# Generating %d fake leap seconds:\n", secondsCount)
	for i := 0; i < secondsCount; i++ {
		firstOfFakeMonth := firstOfThisMonth.AddDate(0, i+1, 0)
		delta := int((firstOfFakeMonth.Sub(ntpEpoch)).Seconds())
		fmt.Printf("%d  XX  # %s\n", delta, firstOfFakeMonth.Format(time.RFC3339))
	}
}

// signFile generates hash for leap Second hashfile and prints is on stdout
func signFile(fileName string) {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		fmt.Printf("Error opening %q: %s\n", fileName, err)
	}

	fmt.Printf("#h %s\n", leaphash.Compute(string(data)))
}

// clockState report system clock state via adjtimex syscall
func clockState() {
	if state, err := syscall.Adjtimex(&syscall.Timex{}); err != nil {
		fmt.Printf("Error calling adjtimex(2): %s", err)
	} else {
		if desc, ok := timexToDesc[state]; ok {
			fmt.Println(desc)
		} else {
			fmt.Printf("Error: %v state is not recognized\n", state)
		}
	}
}

// ntpTime prints data similar to 'ntptime' command output
func ntpTime() {
	var buf syscall.Timex
	if state, err := syscall.Adjtimex(&buf); err != nil {
		fmt.Printf("Error calling adjtimex(2): %s", err)
	} else {
		if desc, ok := timexToDesc[state]; ok {
			fmt.Printf("adjtimex() returns code %d (%s)\n", state, desc)
		} else {
			fmt.Printf("Error: %v state is not recognized\n", state)
		}

		var offset float64
		// 0x2000 is STA_NANO
		if buf.Status&0x2000 != 0 {
			offset = float64(buf.Offset) / 1000.0 // ns -> us
		} else {
			offset = float64(buf.Offset)
		}

		fmt.Printf("  modes 0x%x,\n", buf.Modes)
		fmt.Printf("  offset %.3f us, frequency %.3f ppm, interval %d s\n", offset, float64(buf.Freq)/65536.0, buf.Shift)
		fmt.Printf("  maximum error %d us, estimated error %d us,\n", buf.Maxerror, buf.Esterror)
		fmt.Printf("  status 0x%x,\n", buf.Status)
		fmt.Printf("  time constant %d, precision %d.000 us, tolerance %d ppm,\n", buf.Constant, buf.Precision, buf.Tolerance/65535)
	}
}

// ntpDate prints data similar to 'ntptime' command output
func ntpDate(remoteServerAddr string, remoteServerPort string, requests int) error {
	timeout := 5 * time.Second
	addr := net.JoinHostPort(remoteServerAddr, remoteServerPort)
	conn, err := net.DialTimeout("udp", addr, timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	defer conn.Close()

	// Allow reading of hardware/kernel timestamps via socket
	if err := ntp.EnableKernelTimestampsSocket(conn.(*net.UDPConn)); err != nil {
		return err
	}

	fmt.Printf("Server: %s, Requests: %d\n", addr, requests)
	var sumAvgNetworkDelay int64
	var sumOffset int64

	for i := 0; i < requests; i++ {
		clientTransmitTime := time.Now()
		sec, frac := ntp.Time(clientTransmitTime)

		request := &ntp.Packet{
			Settings:   0x1B,
			TxTimeSec:  sec,
			TxTimeFrac: frac,
		}

		if err := binary.Write(conn, binary.BigEndian, request); err != nil {
			return fmt.Errorf("failed to send request, %w", err)
		}

		var response *ntp.Packet
		var clientReceiveTime time.Time

		blockingRead := make(chan bool, 1)
		go func() {
			// This calls syscall.Recvmsg which has no timeout
			response, clientReceiveTime, _, err = ntp.ReadPacketWithKernelTimestamp(conn.(*net.UDPConn))
			blockingRead <- true
		}()

		select {
		case <-blockingRead:
			if err != nil {
				return err
			}
		case <-time.After(timeout):
			return fmt.Errorf("timeout waiting for reply from server for %v", timeout)
		}

		serverReceiveTime := ntp.Unix(response.RxTimeSec, response.RxTimeFrac)
		serverTransmitTime := ntp.Unix(response.TxTimeSec, response.TxTimeFrac)

		avgNetworkDelay := ntp.AvgNetworkDelay(clientTransmitTime, serverReceiveTime, serverTransmitTime, clientReceiveTime)
		currentRealTime := ntp.CurrentRealTime(serverTransmitTime, avgNetworkDelay)
		offset := ntp.CalculateOffset(currentRealTime, time.Now())

		sumAvgNetworkDelay += avgNetworkDelay
		sumOffset += offset

		// On last request calculate everything
		if i == requests-1 {
			fmt.Printf("Last:\n")
			fmt.Printf("Stratum: %d, Current time: %s\n", response.Stratum, currentRealTime)
			fmt.Printf("Offset: %fs (%fms), Network delay: %fs (%fms)\n", float64(offset)/float64(time.Second.Nanoseconds()), float64(offset)/float64(time.Millisecond.Nanoseconds()), float64(avgNetworkDelay)/float64(time.Second.Nanoseconds()), float64(avgNetworkDelay)/float64(time.Millisecond.Nanoseconds()))
		}
	}

	avgNetworkDelay := float64(sumAvgNetworkDelay) / float64(requests)
	avgOffset := float64(sumOffset) / float64(requests)

	fmt.Printf("Average:\n")
	fmt.Printf("Offset: %fs (%fms), Network delay: %fs (%fms)\n", avgOffset/float64(time.Second.Nanoseconds()), avgOffset/float64(time.Millisecond.Nanoseconds()), avgNetworkDelay/float64(time.Second.Nanoseconds()), avgNetworkDelay/float64(time.Millisecond.Nanoseconds()))
	return nil
}

// cli vars
var trackInterval time.Duration
var refidIP string
var fsCount int
var signFileName string
var remoteServerAddr string
var remoteServerPort int
var ntpdateRequests int

func init() {
	RootCmd.AddCommand(utilsCmd)
	// track
	utilsCmd.AddCommand(trackCmd)
	trackCmd.Flags().DurationVarP(&trackInterval, "interval", "i", time.Second, "Measurement interval")
	// refid
	utilsCmd.AddCommand(refidCmd)
	refidCmd.Flags().StringVarP(&refidIP, "ip", "i", "", "IP address")
	// fakeseconds
	utilsCmd.AddCommand(fakeSecondsCmd)
	fakeSecondsCmd.Flags().IntVarP(&fsCount, "count", "c", 5, "Number of entities to generate")
	// signfile
	utilsCmd.AddCommand(signFileCmd)
	signFileCmd.Flags().StringVarP(&signFileName, "file", "f", "leap-seconds.list", "File name")
	// clockstate
	utilsCmd.AddCommand(clockStateCmd)
	// ntptime
	utilsCmd.AddCommand(ntpTimeCmd)
	// ntpdate
	utilsCmd.AddCommand(ntpdateCmd)
	ntpdateCmd.Flags().StringVarP(&remoteServerAddr, "server", "s", "", "Server to query")
	ntpdateCmd.Flags().IntVarP(&remoteServerPort, "port", "p", 123, "Port of the remote server")
	ntpdateCmd.Flags().IntVarP(&ntpdateRequests, "requests", "r", 3, "How many requests to send")
}

var utilsCmd = &cobra.Command{
	Use:   "utils",
	Short: "Collection of NTP-related utils",
}

var trackCmd = &cobra.Command{
	Use:   "track",
	Short: "Allows to compare monotonic with wall clock.",
	Long: `Allows to compare monotonic with wall clock.
Legend:
  * Wall timestamp - local date and time with TZ
  * Wall difference - wall time elapsed since the start
  * Mono difference - monotonic time elapsed since the start
  * Mono raw diff - monotonic raw time elapsed since the start
  * Offset mono - offset between monotonic and wall elapsed
  * Offset mono raw - offset between monotonic raw and wall elapsed`,
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		track(trackInterval)
	},
}

var refidCmd = &cobra.Command{
	Use:   "refid",
	Short: "Converts IP address to refid",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		err := refID(refidIP)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

var fakeSecondsCmd = &cobra.Command{
	Use:   "fakeseconds",
	Short: "Prints some fake seconds.",
	Long: `Prints some fake seconds (potential slots when leap seconds
might happen in leap-seconds format. For reference:
https://www.ietf.org/timezones/data/leap-seconds.list`,
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		fakeSeconds(fsCount)
	},
}

var signFileCmd = &cobra.Command{
	Use:   "signfile",
	Short: "Generate hash signature for leap-seconds.list",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		signFile(signFileName)
	},
}

var clockStateCmd = &cobra.Command{
	Use:   "clockstate",
	Short: "Print kernel clock state with description.",
	Long: `Print kernel clock state with description.
Useful for checking if kernel noticed leap second. Uses adjtimex(2) to get info.`,
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		clockState()
	},
}

var ntpTimeCmd = &cobra.Command{
	Use:   "ntptime",
	Short: "Print OS kernel output that is similar to ntp_gettime() and ntp_adjtime() output of 'ntptime' utility.",
	Long:  "'ntptime' utility is a part of ntp package. This command produces similar output.",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		ntpTime()
	},
}

var ntpdateCmd = &cobra.Command{
	Use:   "ntpdate",
	Short: "Query remote server. Acts like ntp. Acts like ntpdate -q",
	Long:  "'ntpdate' will query remote server and compare the time difference.",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		if remoteServerAddr == "" {
			fmt.Println("server must be specified")
			os.Exit(1)
		}
		if err := ntpDate(remoteServerAddr, strconv.Itoa(remoteServerPort), ntpdateRequests); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}
