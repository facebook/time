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
	"io"

	"github.com/facebookincubator/ntp/protocol/chrony"
	"github.com/facebookincubator/ntp/protocol/control"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// ChronyCheck gathers NTP stats using chronyc/chronyd protocol client
type ChronyCheck struct {
	Client chrony.Client
}

// chrony reports all float measures in seconds, while NTP and this tool operate ms
func secToMS(seconds float64) float64 {
	return 1000 * seconds
}

// Run is the main method of ChronyCheck and it fetches all information to return NTPCheckResult.
// Essentially we request tracking info, num of peers, and then request source_data and ntp_data
// for each peer individually.
func (n *ChronyCheck) Run() (*NTPCheckResult, error) {
	var packet chrony.ResponsePacket
	var err error
	result := NewNTPCheckResult()
	// tracking data
	trackReq := chrony.NewTrackingPacket()
	packet, err = n.Client.Communicate(trackReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get 'tracking' response")
	}
	log.Debugf("Got 'tracking' response:")
	log.Debugf("Status: %v", packet.GetStatus())
	tracking, ok := packet.(*chrony.ReplyTracking)
	if !ok {
		return nil, errors.Errorf("Got wrong 'tracking' response %+v", packet)
	}
	result.LIDesc = control.LeapDesc[uint8(tracking.LeapStatus)]
	result.LI = uint8(tracking.LeapStatus)
	result.Event = "clock_sync" // no real events for chrony
	result.SysVars = &SystemVariables{
		Leap:      int(tracking.LeapStatus),
		Stratum:   int(tracking.Stratum),
		RootDelay: secToMS(tracking.RootDelay),
		RootDisp:  secToMS(tracking.RootDispersion),
		RefID:     chrony.RefidAsHEX(tracking.RefID),
		RefTime:   tracking.RefTime.String(),
		Offset:    secToMS(tracking.RMSOffset),
		Frequency: tracking.FreqPPM,
	}

	// server stats (best effort, works only over unix socket)
	statsReq := chrony.NewServerStatsPacket()
	packet, err = n.Client.Communicate(statsReq)
	if err == nil {
		stats, ok := packet.(*chrony.ReplyServerStats)
		if !ok {
			return nil, errors.Errorf("Got wrong 'serverstats' response %+v", packet)
		}
		result.ServerStats = &ServerStats{
			PacketsReceived: uint64(stats.NTPHits),
			PacketsDropped:  uint64(stats.NTPDrops),
		}
		log.Debugf("ServerStats: %v", result.ServerStats)
	} else if err != chrony.ErrNotAuthorized {
		return nil, errors.Wrap(err, "failed to get 'serverstats' response")
	}

	// sources list
	sourcesReq := chrony.NewSourcesPacket()
	packet, err = n.Client.Communicate(sourcesReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get 'sources' response")
	}
	sources, ok := packet.(*chrony.ReplySources)
	if !ok {
		return nil, errors.Errorf("Got wrong 'sources' response %+v", packet)
	}
	log.Debugf("Got %d sources", sources.NSources)

	// per-source data
	for i := 0; i < int(sources.NSources); i++ {
		log.Debugf("Fetching source #%d info", i)
		sourceDataReq := chrony.NewSourceDataPacket(int32(i))
		packet, err = n.Client.Communicate(sourceDataReq)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get 'sourcedata' response for source #%d", i)
		}
		sourceData, ok := packet.(*chrony.ReplySourceData)
		if !ok {
			return nil, errors.Errorf("Got wrong 'sourcedata' response %+v", packet)
		}
		ntpDataReq := chrony.NewNTPDataPacket(sourceData.IPAddr)
		// try to get ntpdata, which is available only through socket
		var ntpData *chrony.ReplyNTPData
		packet, err = n.Client.Communicate(ntpDataReq)
		if err == nil {
			ntpData, ok = packet.(*chrony.ReplyNTPData)
			if !ok {
				return nil, errors.Errorf("Got wrong 'ntpdata' response %+v", packet)
			}
		} else if err != chrony.ErrNotAuthorized {
			return nil, errors.Wrapf(err, "failed to get 'ntpdata' response for source #%d", i)
		}
		// unauthorized when asked for ntp data
		if ntpData == nil {
			result.Incomplete = true
		}
		peer, err := NewPeerFromChrony(sourceData, ntpData)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create Peer structure from response packet for peer=%s", sourceData.IPAddr)
		}
		result.Peers[uint16(i)] = peer
		// if main sync source, update ClockSource info
		if sourceData.State == chrony.SourceStateSync {
			if sourceData.Mode == chrony.SourceModeRef {
				result.ClockSource = "local"
			} else {
				result.ClockSource = "ntp"
			}
		}
	}

	return result, nil
}

// NewChronyCheck is a constructor for ChronyCheck
func NewChronyCheck(conn io.ReadWriter) *ChronyCheck {
	return &ChronyCheck{
		Client: chrony.Client{Sequence: 1, Connection: conn},
	}
}
