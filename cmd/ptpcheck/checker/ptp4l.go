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

func preparePTP4lConn(address string) (conn *net.UnixConn, cleanup func(), err error) {
	if address == "" {
		cleanup = func() {}
		return nil, cleanup, fmt.Errorf("preparing ptp4l connection: target address is empty")
	}
	base, _ := path.Split(address)
	local := path.Join(base, fmt.Sprintf("ptpcheck.%d.sock", os.Getpid()))
	// cleanup
	cleanup = func() {
		if conn != nil {
			if err = conn.Close(); err != nil {
				log.Warningf("closing connection: %v", err)
			}
		}
		if err = os.RemoveAll(local); err != nil {
			log.Warningf("removing socket: %v", err)
		}
	}
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
	return
}

// PrepareMgmtClient creates a ptp.MgmtClient with connection to ptp4l over unix socket
func PrepareMgmtClient(address string) (c *ptp.MgmtClient, cleanup func(), err error) {
	timeout := 5 * time.Second
	deadline := time.Now().Add(timeout)
	var conn *net.UnixConn
	conn, cleanup, err = preparePTP4lConn(address)
	if err != nil {
		return nil, cleanup, err
	}

	if err = conn.SetReadDeadline(deadline); err != nil {
		return nil, cleanup, err
	}
	return &ptp.MgmtClient{
		Connection: conn,
	}, cleanup, err
}

// RunPTP4L will talk over conn and return PTPCheckResult
func RunPTP4L(c *ptp.MgmtClient) (*PTPCheckResult, error) {
	var err error
	currentDataSet, err := c.CurrentDataSet()
	if err != nil {
		return nil, fmt.Errorf("getting CURRENT_DATA_SET management TLV: %w", err)
	}
	log.Debugf("CurrentDataSet: %+v", currentDataSet)
	defaultDataSet, err := c.DefaultDataSet()
	if err != nil {
		return nil, fmt.Errorf("getting DEFAULT_DATA_SET management TLV: %w", err)
	}
	log.Debugf("DefaultDataSet: %+v", defaultDataSet)
	parentDataSet, err := c.ParentDataSet()
	if err != nil {
		return nil, fmt.Errorf("getting PARENT_DATA_SET management TLV: %w", err)
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

	portServiceStats, err := c.PortServiceStatsNP()
	// it's a non-standard ptp4l thing, might be missing
	if err != nil {
		log.Warningf("couldn't get PortServiceStatsNP: %v", err)
	} else {
		log.Debugf("PortServiceStatsNP: %+v", portServiceStats)
		result.PortServiceStats = &portServiceStats.PortServiceStats
	}
	return result, nil
}
