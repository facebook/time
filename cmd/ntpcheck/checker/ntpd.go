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
	"io"

	"github.com/facebook/time/ntp/control"

	log "github.com/sirupsen/logrus"
)

var vnMode = control.MakeVnMode(3, control.Mode)

type ntpClient interface {
	Communicate(packet *control.NTPControlMsgHead) (*control.NTPControlMsg, error)
	CommunicateWithData(packet *control.NTPControlMsgHead, data []uint8) (*control.NTPControlMsg, error)
}

// NTPCheck gathers NTP stats using UDP network client
type NTPCheck struct {
	Client ntpClient
}

func (n *NTPCheck) getReadStatusPacket() *control.NTPControlMsgHead {
	return &control.NTPControlMsgHead{
		// This is a 00, then three-bit integer indicating the NTP version number, currently three, then
		// a three-bit integer indicating the mode. It must have the value 6, indicating an NTP control message.
		VnMode: vnMode,
		// Response Bit, Error Bit and More bit set to zero, Operation Code set to 1 (read status command/response)
		REMOp: control.OpReadStatus,
	}
}

func (n *NTPCheck) getReadVariablesPacket(associationID uint16) *control.NTPControlMsgHead {
	log.Debugf("preparing assoc info packet associationID=%x", associationID)
	return &control.NTPControlMsgHead{
		// This is a 00, then three-bit integer indicating the NTP version number, currently three, then
		// a three-bit integer indicating the mode. It must have the value 6, indicating an NTP control message.
		VnMode: vnMode,
		// Response Bit, Error Bit and More bit set to zero, Operation Code set to 2 (read variables command/response)
		REMOp:         control.OpReadVariables,
		AssociationID: associationID,
	}
}

// ReadStatus sends Read Status packet and returns response packet
func (n *NTPCheck) ReadStatus() (*control.NTPControlMsg, error) {
	packet := n.getReadStatusPacket()
	return n.Client.Communicate(packet)
}

// ReadVariables sends Read Variables packet for associationID.
// associationID set to 0 means 'read local system variables'
func (n *NTPCheck) ReadVariables(associationID uint16) (*control.NTPControlMsg, error) {
	packet := n.getReadVariablesPacket(associationID)
	return n.Client.Communicate(packet)
}

// ReadServerVariables sends Read Variables packet asking for server vars and returns response packet
func (n *NTPCheck) ReadServerVariables() (*control.NTPControlMsg, error) {
	packet := n.getReadVariablesPacket(0)
	// ntpd for some reason requires all those fields to be present in request (order doesn't matter though)
	vars := "ss_uptime,ss_reset,ss_received,ss_thisver,ss_oldver,ss_badformat,ss_badauth,ss_declined,ss_restricted,ss_limited,ss_kodsent,ss_processed"
	return n.Client.CommunicateWithData(packet, []uint8(vars))
}

// Run is the main method of NTPCheck and it fetches all information to return NTPCheckResult.
// Essentially we request system status that contains list of peers, and then request variables
// for our server and each peer individually.
func (n *NTPCheck) Run() (*NTPCheckResult, error) {
	result := NewNTPCheckResult()
	packet, err := n.ReadStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get 'read status' packet from NTP server: %w", err)
	}
	log.Debug("Got 'read status' response:")
	log.Debugf("Version: %v", packet.GetVersion())
	log.Debugf("Mode: %v", packet.GetMode())
	log.Debugf("Response: %v", packet.IsResponse())
	log.Debugf("Error: %v", packet.HasError())
	log.Debugf("More: %v", packet.HasMore())
	log.Debugf("Data: %v", packet.Data)
	log.Debugf("Data: %v", len(packet.Data))
	log.Debugf("Data string: '%s'", string(packet.Data))

	sysStatus, err := packet.GetSystemStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to parse SystemStatusWord: %w", err)
	}
	result.LIDesc = control.LeapDesc[sysStatus.LI]
	result.LI = sysStatus.LI
	result.ClockSource = control.ClockSourceDesc[sysStatus.ClockSource]
	result.EventCount = sysStatus.SystemEventCounter
	result.Event = control.SystemEventDesc[sysStatus.SystemEventCode]

	infoPacket, err := n.ReadVariables(0)
	if err != nil {
		return nil, fmt.Errorf("failed to get 'read variables' packet from NTP server for associationID=0: %w", err)
	}
	log.Debug("Got 'read variables' response:")
	log.Debugf("Version: %v", infoPacket.GetVersion())
	log.Debugf("Mode: %v", infoPacket.GetMode())
	log.Debugf("Response: %v", infoPacket.IsResponse())
	log.Debugf("Error: %v", infoPacket.HasError())
	log.Debugf("More: %v", infoPacket.HasMore())
	log.Debugf("Data string: '%s'", string(infoPacket.Data))

	sys, err := NewSystemVariablesFromNTP(infoPacket)
	if err != nil {
		return nil, fmt.Errorf("failed to create System structure from response packet: %w", err)
	}
	result.SysVars = sys

	assocs, err := packet.GetAssociations()
	if err != nil {
		return nil, fmt.Errorf("failed to get associations list from response packet for associationID=0: %w", err)
	}
	for id, peerStatus := range assocs {
		log.Debugf("Peer %x Status Word: %#v", id, peerStatus)
		assocInfo, err := n.ReadVariables(id)
		if err != nil {
			return nil, fmt.Errorf("failed to get 'read variables' packet from NTP server for associationID=%x: %w", id, err)
		}
		s, err := assocInfo.GetPeerStatus()
		if err != nil {
			return nil, fmt.Errorf("failed to get peer status list from response packet for associationID=%x: %w", id, err)
		}
		log.Debugf("Assoc ID: %x", assocInfo.AssociationID)
		log.Debugf("Peer Status: %#v", s)
		m, err := assocInfo.GetAssociationInfo()
		if err != nil {
			return nil, fmt.Errorf("failed to get associations list from response packet for associationID=%x: %w", id, err)
		}
		for k, v := range m {
			log.Debugf("%s: %s", k, v)
		}
		peer, err := NewPeerFromNTP(assocInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to create Peer structure from response packet for associationID=%x: %w", id, err)
		}
		result.Peers[id] = peer
	}
	return result, nil
}

// ServerStats return server stats
func (n *NTPCheck) ServerStats() (*ServerStats, error) {
	serverVars, err := n.ReadServerVariables()
	if err != nil {
		return nil, fmt.Errorf("failed to get 'server variables' packet from NTP server: %w", err)
	}

	log.Debug("Got system 'read variables' response:")
	log.Debugf("Version: %v", serverVars.GetVersion())
	log.Debugf("Mode: %v", serverVars.GetMode())
	log.Debugf("Response: %v", serverVars.IsResponse())
	log.Debugf("Error: %v", serverVars.HasError())
	log.Debugf("More: %v", serverVars.HasMore())
	log.Debugf("Data string: '%s'", string(serverVars.Data))

	if serverVars.HasError() || (len(serverVars.Data) <= 0) {
		return nil, fmt.Errorf("Got bad 'server variables' response %+v", serverVars)
	}

	serverStats, err := NewServerStatsFromNTP(serverVars)
	if err != nil {
		return nil, fmt.Errorf("failed to create ServerStats structure from response packet: %w", err)
	}

	return serverStats, nil
}

// NewNTPCheck is a constructor for NTPCheck
func NewNTPCheck(conn io.ReadWriter) *NTPCheck {
	return &NTPCheck{
		Client: &control.NTPClient{Sequence: 1, Connection: conn},
	}
}
