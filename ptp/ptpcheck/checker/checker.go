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

package checker

import (
	"fmt"
	"net"
	"os"
	"path"
	"time"

	log "github.com/sirupsen/logrus"

	ptp "github.com/facebook/time/ptp/protocol"
)

// PTPCheckResult is selected parts of various stats we expose to users, abstracting away protocol implementation
type PTPCheckResult struct {
	OffsetFromMasterNS  float64
	GrandmasterPresent  bool
	StepsRemoved        int
	MeanPathDelayNS     float64
	ClockIdentity       string
	GrandmasterIdentity string
	IngressTimeNS       int64
	PortStatsTX         map[string]uint64
	PortStatsRX         map[string]uint64
}

// Run will talk over conn and return PTPCheckResult
func Run(c *ptp.MgmtClient) (*PTPCheckResult, error) {
	var err error
	currentDataSet, err := c.CurrentDataSet()
	if err != nil {
		return nil, err
	}
	log.Debugf("CurrentDataSet: %+v", currentDataSet)
	defaultDataSet, err := c.DefaultDataSet()
	if err != nil {
		return nil, err
	}
	log.Debugf("DefaultDataSet: %+v", defaultDataSet)
	parentDataSet, err := c.ParentDataSet()
	if err != nil {
		return nil, err
	}
	log.Debugf("ParentDataSet: %+v", parentDataSet)

	/* gmPresent calculation from non-standard TIME_STATUS_NP

	 if (cid_eq(&c->dad.pds.grandmasterIdentity, &c->dds.clockIdentity))
	            tsn->gmPresent = 0;
	        else
				tsn->gmPresent = 1;

	where dad.pds is PARENT_DATA_SET and dds is DEFAULT_DATA_SET
	*/
	gmPresent := defaultDataSet.ClockIdentity != parentDataSet.GrandmasterIdentity

	result := &PTPCheckResult{
		OffsetFromMasterNS:  currentDataSet.OffsetFromMaster.Nanoseconds(),
		GrandmasterPresent:  gmPresent,
		MeanPathDelayNS:     currentDataSet.MeanPathDelay.Nanoseconds(),
		StepsRemoved:        int(currentDataSet.StepsRemoved),
		ClockIdentity:       defaultDataSet.ClockIdentity.String(),
		GrandmasterIdentity: parentDataSet.GrandmasterIdentity.String(),
		PortStatsTX:         map[string]uint64{},
		PortStatsRX:         map[string]uint64{},
	}

	portStats, err := c.PortStatsNP()
	// it's a non-standard ptp4l thing, might be missing
	if err != nil {
		log.Warningf("couldn't get PortStatsNP: %v", err)
	} else {
		log.Debugf("PortStatsNP: %+v", portStats)
		for k, v := range ptp.MessageTypeToString {
			result.PortStatsRX[v] = portStats.PortStats.RXMsgType[k]
			result.PortStatsTX[v] = portStats.PortStats.TXMsgType[k]
		}
	}

	timeStatus, err := c.TimeStatusNP()
	// it's a non-standard ptp4l thing, might be missing
	if err != nil {
		log.Warningf("couldn't get TimeStatusNP: %v", err)
	} else {
		log.Debugf("TimeStatusNP: %+v", timeStatus)
		result.IngressTimeNS = timeStatus.IngressTimeNS
	}
	return result, nil
}

// PrepareClient creates a ptp.MgmtClient with connection to ptp4l over unix socket
func PrepareClient(address string) (c *ptp.MgmtClient, cleanup func(), err error) {
	timeout := 5 * time.Second
	base, _ := path.Split(address)
	var conn *net.UnixConn
	local := path.Join(base, fmt.Sprintf("ptpcheck.%d.sock", os.Getpid()))
	// cleanup
	cleanup = func() {
		if conn != nil {
			if err := conn.Close(); err != nil {
				log.Warningf("closing connection: %v", err)
			}
		}
		if err := os.RemoveAll(local); err != nil {
			log.Warningf("removing socket: %v", err)
		}
	}
	deadline := time.Now().Add(timeout)

	addr, err := net.ResolveUnixAddr("unixgram", address)
	if err != nil {
		return nil, cleanup, err
	}
	localAddr, _ := net.ResolveUnixAddr("unixgram", local)
	conn, err = net.DialUnix("unixgram", localAddr, addr)
	if err != nil {
		return nil, cleanup, err
	}

	if err := os.Chmod(local, 0666); err != nil {
		return nil, cleanup, err
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, cleanup, err
	}
	return &ptp.MgmtClient{
		Connection: conn,
	}, cleanup, err
}

// RunCheck is a simple wrapper to connect to address and run Run()
func RunCheck(address string) (*PTPCheckResult, error) {
	c, cleanup, err := PrepareClient(address)
	defer cleanup()
	if err != nil {
		return nil, err
	}
	log.Debugf("connected to %s", address)
	return Run(c)
}
