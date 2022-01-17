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
	"os"
	"strconv"
	"time"

	"github.com/facebook/time/leaphash"
	"github.com/facebook/time/leapsectz"
	ntp "github.com/facebook/time/ntp/protocol"
	"github.com/facebook/time/timestamp"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

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

// ntpDate prints data similar to 'ntptime' command output
func ntpDate(remoteServerAddr string, remoteServerPort string, requests int) error {
	timeout := 5 * time.Second
	addr := net.JoinHostPort(remoteServerAddr, remoteServerPort)
	conn, err := net.DialTimeout("udp", addr, timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	defer conn.Close()

	// get connection file descriptor
	connFd, err := timestamp.ConnFd(conn.(*net.UDPConn))
	if err != nil {
		return err
	}

	// Allow reading of kernel timestamps via socket
	if err := timestamp.EnableSWTimestampsRx(connFd); err != nil {
		return err
	}
	err = unix.SetNonblock(connFd, false)
	if err != nil {
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
		var buf []byte

		blockingRead := make(chan bool, 1)
		go func() {
			// This calls unix.Recvmsg which has no timeout
			buf, _, clientReceiveTime, err = timestamp.ReadPacketWithRXTimestamp(connFd)
			if err != nil {
				blockingRead <- true
				return
			}

			response, err = ntp.BytesToPacket(buf)
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

// printLeap prints leap second information from the system timezone database
func printLeap(srcfile string) error {
	ls, err := leapsectz.Parse(srcfile)
	if err != nil {
		return err
	}
	for _, l := range ls {
		fmt.Println(l.Time().UTC())
	}
	return nil
}

// addFakeSecondZoneInfo reads current zoneinfo db leap seconds and add one in the end of month or year
func addFakeSecondZoneInfo(srcfile, dstfile string, offsetMonth int) error {
	ls, err := leapsectz.Parse(srcfile)
	if err != nil {
		return err
	}

	ntpEpoch := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
	now := time.Now()
	fakeLeap := time.Date(now.Year(), now.Month()+time.Month(offsetMonth), 1, 0, 0, 0, 0, time.UTC)
	fmt.Printf("Fake second added on %s\n", fakeLeap.Format(time.RFC3339))
	ls = append(ls, leapsectz.LeapSecond{
		Tleap: uint64(fakeLeap.Sub(ntpEpoch).Seconds()) + uint64(len(ls)),
		Nleap: int32(len(ls) + 1),
	})

	o, err := os.Create(dstfile)
	if err != nil {
		return err
	}

	defer o.Close()

	if err := leapsectz.Write(o, '2', ls, ""); err != nil {
		return err
	}

	return nil
}

// cli vars
var refidIP string
var fsCount int
var signFileName string
var remoteServerAddr string
var remoteServerPort int
var ntpdateRequests int
var sourceLeapSeconds string
var destLeapSeconds string
var offsetMonth int

func init() {
	RootCmd.AddCommand(utilsCmd)
	// refid
	utilsCmd.AddCommand(refidCmd)
	refidCmd.Flags().StringVarP(&refidIP, "ip", "i", "", "IP address")
	// fakeseconds
	utilsCmd.AddCommand(fakeSecondsCmd)
	fakeSecondsCmd.Flags().IntVarP(&fsCount, "count", "c", 5, "Number of entities to generate")
	// signfile
	utilsCmd.AddCommand(signFileCmd)
	signFileCmd.Flags().StringVarP(&signFileName, "file", "f", "leap-seconds.list", "File name")
	// ntpdate
	utilsCmd.AddCommand(ntpdateCmd)
	ntpdateCmd.Flags().StringVarP(&remoteServerAddr, "server", "s", "", "Server to query")
	ntpdateCmd.Flags().IntVarP(&remoteServerPort, "port", "p", 123, "Port of the remote server")
	ntpdateCmd.Flags().IntVarP(&ntpdateRequests, "requests", "r", 3, "How many requests to send")
	// printleap
	utilsCmd.AddCommand(printLeapCmd)
	printLeapCmd.Flags().StringVarP(&sourceLeapSeconds, "srcfile", "s", "/usr/share/zoneinfo/right/UTC", "Source file of leap seconds")
	// addFakeSecondZoneInfo
	utilsCmd.AddCommand(addFakeSecondZoneInfoCmd)
	addFakeSecondZoneInfoCmd.Flags().IntVarP(&offsetMonth, "month", "m", 1, "How many monthes to add to current to insert leap second")
	addFakeSecondZoneInfoCmd.Flags().StringVarP(&sourceLeapSeconds, "srcfile", "s", "/usr/share/zoneinfo/right/UTC", "Source file of leap seconds")
	addFakeSecondZoneInfoCmd.Flags().StringVarP(&destLeapSeconds, "dstfile", "d", "/usr/share/zoneinfo/right/Fake", "Destination file for fake leap seconds")

}

var utilsCmd = &cobra.Command{
	Use:   "utils",
	Short: "Collection of NTP-related utils",
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

var printLeapCmd = &cobra.Command{
	Use:   "printleap",
	Short: "Prints leap second information from the system timezone database",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		if err := printLeap(sourceLeapSeconds); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

var addFakeSecondZoneInfoCmd = &cobra.Command{
	Use:   "addfakesecond",
	Short: "Adds fake second to zoneinfo database",
	Long: `'addfakesecond' will read current zoneinfo leap second file from srcfile, add fake second
	in the end of the month (plus offset provided by -m) and write new file to dstfile`,
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		if err := addFakeSecondZoneInfo(sourceLeapSeconds, destLeapSeconds, offsetMonth); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}
