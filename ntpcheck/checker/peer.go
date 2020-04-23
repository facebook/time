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
	"strconv"

	"github.com/facebookincubator/ntp/protocol/chrony"
	"github.com/facebookincubator/ntp/protocol/control"
	"github.com/pkg/errors"
)

// Here go peer selection statuses, as described in http://doc.ntp.org/current-stable/decode.html#peer
const (
	// SelReject means peer is discarded as not valid (TEST10-TEST13)
	SelReject uint8 = 0
	// SelFalseTick means peer is discarded by intersection algorithm
	SelFalseTick uint8 = 1
	// SelExcess means peer is discarded by table overflow (not used)
	SelExcess uint8 = 2
	// SelOutlier means peer is discarded by the cluster algorithm
	SelOutlier uint8 = 3
	// SelCandidate means peer is included by the combine algorithm
	SelCandidate uint8 = 4
	// SelBackup means peer is a backup (more than tos maxclock sources)
	SelBackup uint8 = 5
	// SelSYSPeer means peer is a system peer (main syncronization source)
	SelSYSPeer uint8 = 6
	// SelPPSPeer means peer is a PPS peer (when the prefer peer is valid)
	SelPPSPeer uint8 = 7
)

// Peer contains parsed information from Peer Variables and peer status word, as described in http://doc.ntp.org/current-stable/ntpq.html
type Peer struct {
	// from PeerStatusWord
	Configured   bool
	AuthPossible bool
	Authentic    bool
	Reachable    bool
	Broadcast    bool
	Selection    uint8
	Condition    string
	// from variables
	SRCAdr     string
	SRCPort    int
	DSTAdr     string
	DSTPort    int
	Leap       int
	Stratum    int
	Precision  int
	RootDelay  float64
	RootDisp   float64
	RefID      string
	RefTime    string
	Reach      uint8
	Unreach    int
	HMode      int
	PMode      int
	HPoll      int
	PPoll      int
	Headway    int
	Flash      uint16
	Flashers   []string
	Offset     float64
	Delay      float64
	Dispersion float64
	Jitter     float64
	Xleave     float64
	Rec        string
	FiltDelay  string
	FiltOffset string
	FiltDisp   string
}

// sanityCheckPeerVars checks if we parsed enough info from NTPD response
func sanityCheckPeerVars(p *Peer) error {
	if p == nil {
		return errors.New("No peer")
	}
	if p.Stratum == 0 {
		return errors.New("Incomplete data, stratum 0 in peer variables")
	}
	if p.PPoll == 0 || p.HPoll == 0 {
		return errors.New("Incomplete data, poll 0 in peer variables")
	}
	return nil
}

// NewPeerFromNTP constructs Peer from NTPControlMsg packet
func NewPeerFromNTP(p *control.NTPControlMsg) (*Peer, error) {
	psWord, err := p.GetPeerStatus()
	if err != nil {
		return nil, err
	}
	m, err := p.GetAssociationInfo()
	if err != nil {
		return nil, err
	}
	var reach, flash uint16
	// data comes as k=v pairs in packet, and those kv pairs are parsed by GetAssociationInfo.
	// If data is severely corrupted GetAssociationInfo will return error.
	// It's ok to have some fields missing, thus we don't check for errors below.
	leap, _ := strconv.Atoi(m["leap"])
	unreach, _ := strconv.Atoi(m["unreach"])
	pmode, _ := strconv.Atoi(m["pmode"])
	hpoll, _ := strconv.Atoi(m["hpoll"])
	offset, _ := strconv.ParseFloat(m["offset"], 64)
	dispersion, _ := strconv.ParseFloat(m["dispersion"], 64)
	dstport, _ := strconv.Atoi(m["dstport"])
	fmt.Sscan(m["reach"], &reach)
	ppoll, _ := strconv.Atoi(m["ppoll"])
	headway, _ := strconv.Atoi(m["headway"])
	fmt.Sscan(m["flash"], &flash)
	jitter, _ := strconv.ParseFloat(m["jitter"], 64)
	rootdelay, _ := strconv.ParseFloat(m["rootdelay"], 64)
	precision, _ := strconv.Atoi(m["precision"])
	delay, _ := strconv.ParseFloat(m["delay"], 64)
	stratum, _ := strconv.Atoi(m["stratum"])
	hmode, _ := strconv.Atoi(m["hmode"])
	srcport, _ := strconv.Atoi(m["srcport"])
	xleave, _ := strconv.ParseFloat(m["xleave"], 64)
	rootdisp, _ := strconv.ParseFloat(m["rootdisp"], 64)
	peer := Peer{
		// from PeerStatusWord
		Configured:   psWord.PeerStatus.Configured,
		AuthPossible: psWord.PeerStatus.AuthEnabled,
		Authentic:    psWord.PeerStatus.AuthOK,
		Reachable:    psWord.PeerStatus.Reachable,
		Broadcast:    psWord.PeerStatus.Broadcast,
		Selection:    psWord.PeerSelection,
		Condition:    control.PeerSelect[psWord.PeerSelection],
		// from variables
		Leap:       leap,
		Unreach:    unreach,
		PMode:      pmode,
		HPoll:      hpoll,
		DSTAdr:     m["dstadr"],
		Rec:        m["rec"],
		FiltOffset: m["filtoffset"],
		RefTime:    m["reftime"],
		Offset:     offset,
		FiltDisp:   m["filtdisp"],
		SRCAdr:     m["srcadr"],
		Dispersion: dispersion,
		DSTPort:    dstport,
		RefID:      m["refid"],
		Reach:      uint8(reach),
		PPoll:      ppoll,
		Headway:    headway,
		Flash:      uint16(flash),
		Flashers:   control.ReadFlashStatusWord(uint16(flash)),
		Jitter:     jitter,
		RootDelay:  rootdelay,
		Precision:  precision,
		Delay:      delay,
		Stratum:    stratum,
		HMode:      hmode,
		FiltDelay:  m["filtdelay"],
		SRCPort:    srcport,
		Xleave:     xleave,
		RootDisp:   rootdisp,
	}
	if err := sanityCheckPeerVars(&peer); err != nil {
		return nil, err
	}
	return &peer, nil
}

// this mapping is not 100% correct, but fits the purpose of the tool
var chronyToPeerSelection = map[chrony.SourceStateType]uint8{
	chrony.SourceStateSync:        SelSYSPeer,
	chrony.SourceStateUnreach:     SelReject, // not a direct mapping
	chrony.SourceStateFalseTicket: SelFalseTick,
	chrony.SourceStateJittery:     SelReject, // ditto
	chrony.SourceStateCandidate:   SelCandidate,
	chrony.SourceStateOutlier:     SelOutlier,
}

// NewPeerFromChrony constructs Peer from two chrony packets
func NewPeerFromChrony(s *chrony.ReplySourceData, p *chrony.ReplyNTPData) (*Peer, error) {
	// clear auth and interlieved flag
	flash := s.Flags & chrony.NTPFlagsTests
	// don't report all flashers if peer is unreachable
	if flash > 0 {
		flash ^= chrony.NTPFlagsTests // negate bits as original NTPD flashers have opposite meaning
	}
	// all NTP measures are in ms, and chrony reports all in seconds, thus secToMS everywhere
	peer := Peer{
		Configured:   true,
		AuthPossible: false,
		Authentic:    s.Flags&chrony.NTPFlagAuthenticated != 0,
		Reachable:    s.Reachability == 255, // all 8 attempts
		Broadcast:    false,
		Selection:    chronyToPeerSelection[s.State],
		Condition:    chrony.SourceStateDesc[s.State],
		Flash:        flash,
		Flashers:     chrony.ReadNTPTestFlags(s.Flags),
		Offset:       -1 * secToMS(s.OrigLatestMeas), // sourceData offset and NTPData offset sign has opposite meaning
		PPoll:        int(s.Poll),
		HPoll:        int(s.Poll),
		Stratum:      int(s.Stratum),
		SRCAdr:       s.IPAddr.String(),
		Reach:        uint8(s.Reachability),
	}
	// populate data from ntpdata struct
	if p != nil {
		refID := chrony.RefidAsHEX(p.RefID)
		// Only stratum 1 servers can have GPS or something else as string refID
		if p.Stratum == 1 {
			refIDStr := chrony.RefidToString(p.RefID)
			if len(refIDStr) > 0 {
				refID = refIDStr
			}
		}
		peer.Leap = int(p.Leap)
		peer.HPoll = int(p.Poll)
		peer.DSTAdr = p.LocalAddr.String()
		peer.RefTime = p.RefTime.String()
		peer.Offset = secToMS(p.Offset)
		peer.Dispersion = secToMS(p.PeerDispersion)
		peer.DSTPort = int(p.RemotePort)
		peer.RefID = refID
		peer.PPoll = int(p.Poll)
		peer.Jitter = secToMS(p.PeerDispersion) // best approx we have
		peer.RootDelay = secToMS(p.RootDelay)
		peer.Precision = int(p.Precision)
		peer.Delay = secToMS(p.PeerDelay)
		peer.RootDisp = secToMS(p.RootDispersion)
	}
	// no need for sanity check as we are not parsing k=v pairs in case of chrony proto
	return &peer, nil
}
