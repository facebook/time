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

	"github.com/facebookincubator/ntp/protocol/control"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

// NTPCheck gathers NTP stats using UDP network client
type NTPCheck struct {
	Client control.NTPClient
}

func (n *NTPCheck) getReadStatusPacket() *control.NTPControlMsgHead {
	return &control.NTPControlMsgHead{
		// This is a 00, then three-bit integer indicating the NTP version number, currently three, then
		// a three-bit integer indicating the mode. It must have the value 6, indicating an NTP control message.
		VnMode: 0x1E,
		// Response Bit, Error Bit and More bit set to zero, Operation Code set to 1 (read status command/response)
		REMOp: 0x01,
	}
}

func (n *NTPCheck) getReadVariablesPacket(associationID uint16) *control.NTPControlMsgHead {
	log.Debugf("preparing assoc info packet associationID=%x", associationID)
	return &control.NTPControlMsgHead{
		// This is a 00, then three-bit integer indicating the NTP version number, currently three, then
		// a three-bit integer indicating the mode. It must have the value 6, indicating an NTP control message.
		VnMode: 0x1E,
		// Response Bit, Error Bit and More bit set to zero, Operation Code set to 2 (read variables command/response)
		REMOp:         0x02,
		AssociationID: associationID,
	}
}

// ReadStatus sends Read Status packet and returns response packet
func (n *NTPCheck) ReadStatus() (*control.NTPControlMsg, error) {
	packet := n.getReadStatusPacket()
	return n.Client.Communicate(packet)
}

// ReadVariables sends Read Variables packat for associationID.
// associationID set to 0 means 'read local system variables'
func (n *NTPCheck) ReadVariables(associationID uint16) (*control.NTPControlMsg, error) {
	packet := n.getReadVariablesPacket(associationID)
	return n.Client.Communicate(packet)
}

// Run is the main method of NTPCheck and it fetches all information to return NTPCheckResult.
// Essentially we request system status that contains list of peers, and then request variables
// for our server and each peer individually.
func (n *NTPCheck) Run() (*NTPCheckResult, error) {
	result := NewNTPCheckResult()
	packet, err := n.ReadStatus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get 'read status' packet from NTP server")
	}
	log.Debugf("Got 'read status' response:")
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
		return nil, errors.Wrap(err, "failed to parse SystemStatusWord")
	}
	result.LIDesc = control.LeapDesc[sysStatus.LI]
	result.LI = sysStatus.LI
	result.ClockSource = control.ClockSourceDesc[sysStatus.ClockSource]
	result.EventCount = sysStatus.SystemEventCounter
	result.Event = control.SystemEventDesc[sysStatus.SystemEventCode]

	infoPacket, err := n.ReadVariables(0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get 'read variables' packet from NTP server for associationID=0")
	}
	sys, err := NewSystemVariables(infoPacket)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create System structure from response packet for server")
	}
	result.SysVars = sys
	// with NTPD 4.2.8+ we can read SystemStats from system variables: 'ss_received', 'ss_declined' and so on.
	// But we currently have NTPD 4.2.6 which uses custom mode 7 NTP messaging I really don't want to implement
	// as we are moving to chrony anyway.
	log.Debugf("Got 'read variables' response:")
	log.Debugf("Version: %v", infoPacket.GetVersion())
	log.Debugf("Mode: %v", infoPacket.GetMode())
	log.Debugf("Response: %v", infoPacket.IsResponse())
	log.Debugf("Error: %v", infoPacket.HasError())
	log.Debugf("More: %v", infoPacket.HasMore())
	log.Debugf("Data string: '%s'", string(infoPacket.Data))
	assocs, err := packet.GetAssociations()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get associations list from response packet for associationID=0")
	}
	for id, peerStatus := range assocs {
		log.Debugf("Peer %x Status Word: %#v", id, peerStatus)
		assocInfo, err := n.ReadVariables(id)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get 'read variables' packet from NTP server for associationID=%x", id)
		}
		s, err := assocInfo.GetPeerStatus()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get peer status list from response packet for associationID=%x", id)
		}
		log.Debugf("Assoc ID: %x", assocInfo.AssociationID)
		log.Debugf("Peer Status: %#v", s)
		m, err := assocInfo.GetAssociationInfo()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get associations list from response packet for associationID=%x", id)
		}
		for k, v := range m {
			log.Debugf("%s: %s", k, v)
		}
		peer, err := NewPeerFromNTP(assocInfo)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create Peer structure from response packet for associationID=%x", id)
		}
		result.Peers[id] = peer

	}
	return result, nil
}

// NewNTPCheck is a contructor for NTPCheck
func NewNTPCheck(conn io.ReadWriter) *NTPCheck {
	return &NTPCheck{
		Client: control.NTPClient{Sequence: 1, Connection: conn},
	}
}
