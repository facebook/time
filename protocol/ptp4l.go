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

package protocol

// Support has been included for some non-standard extensions provided by the ptp4l implementation; the TLVs IDPortStatsNP and IDTimeStatusNP
// Implemented as present in linuxptp master d95f4cd6e4a7c6c51a220c58903110a2326885e7

import (
	"fmt"
)

// ptp4l-specific management TLV ids
const (
	IDPortStatsNP  ManagementID = 0xC005
	IDTimeStatusNP ManagementID = 0xC000
)

// PortStats is a ptp4l struct containing port statistics
type PortStats struct {
	RXMsgType [16]uint64
	TXMsgType [16]uint64
}

// PortStatsNP is a ptp4l struct containing port identinity and statistics
type PortStatsNP struct {
	PortIdentity PortIdentity
	PortStats    PortStats
}

// ManagementMsgPortStatsNP is header + PortStatsNP
type ManagementMsgPortStatsNP struct {
	ManagementMsgHead
	ManagementTLVHead
	PortStatsNP
}

// ScaledNS is some struct used by ptp4l to report phase change
type ScaledNS struct {
	NanosecondsMSB        uint16
	NanosecondsLSB        uint64
	FractionalNanoseconds uint16
}

// TimeStatusNP is a ptp4l struct containing actually useful instance metrics
type TimeStatusNP struct {
	MasterOffsetNS             int64
	IngressTimeNS              int64 // this is PHC time
	CumulativeScaledRateOffset int32
	ScaledLastGmPhaseChange    int32
	GMTimeBaseIndicator        uint16
	LastGmPhaseChange          ScaledNS
	GMPresent                  int32
	GMIdentity                 ClockIdentity
}

// ManagementMsgTimeStatusNP is header + TimeStatusNP
type ManagementMsgTimeStatusNP struct {
	ManagementMsgHead
	ManagementTLVHead
	TimeStatusNP
}

// ManagementMsgBeforeData is a header + tlv header
type ManagementMsgBeforeData struct {
	ManagementMsgHead
	ManagementTLVHead
}

// PortStatsNPRequest prepares request packet for PORT_STATS_NP request
func PortStatsNPRequest() *ManagementMsgBeforeData {
	// we send request with no portStats data just like pmc does
	return &ManagementMsgBeforeData{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:    NewSdoIDAndMsgType(MessageManagement, 0),
				Version:            Version,
				MessageLength:      headerSize,
				SourcePortIdentity: identity,
				LogMessageInterval: mgmtLogMessageInterval,
			},
			TargetPortIdentity:   defaultTargetPortIdentity,
			StartingBoundaryHops: 0,
			BoundaryHops:         0,
			ActionField:          GET,
		},
		ManagementTLVHead: ManagementTLVHead{
			TLVHead: TLVHead{
				TLVType:     TLVManagement,
				LengthField: tlvBaseSize,
			},
			ManagementID: IDPortStatsNP,
		},
	}
}

// PortStatsNP sends PORT_STATS_NP request and returns response
func (c *MgmtClient) PortStatsNP() (*PortStatsNP, error) {
	req := PortStatsNPRequest()
	res, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	p, ok := res.(*ManagementMsgPortStatsNP)
	if !ok {
		return nil, fmt.Errorf("got unexpected management packet %T, expected %T", res, p)
	}
	return &p.PortStatsNP, nil
}

// TimeStatusNPRequest prepares request packet for TIME_STATUS_NP request
func TimeStatusNPRequest() *ManagementMsgBeforeData {
	// we send request with no TimeStatusNP data just like pmc does
	return &ManagementMsgBeforeData{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:    NewSdoIDAndMsgType(MessageManagement, 0),
				Version:            Version,
				MessageLength:      headerSize,
				SourcePortIdentity: identity,
				LogMessageInterval: mgmtLogMessageInterval,
			},
			TargetPortIdentity:   defaultTargetPortIdentity,
			StartingBoundaryHops: 0,
			BoundaryHops:         0,
			ActionField:          GET,
		},
		ManagementTLVHead: ManagementTLVHead{
			TLVHead: TLVHead{
				TLVType:     TLVManagement,
				LengthField: tlvBaseSize,
			},
			ManagementID: IDTimeStatusNP,
		},
	}
}

// TimeStatusNP sends TIME_STATUS_NP request and returns response
func (c *MgmtClient) TimeStatusNP() (*TimeStatusNP, error) {
	req := TimeStatusNPRequest()
	res, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	p, ok := res.(*ManagementMsgTimeStatusNP)
	if !ok {
		return nil, fmt.Errorf("got unexpected management packet %T, expected %T", res, p)
	}
	return &p.TimeStatusNP, nil
}
