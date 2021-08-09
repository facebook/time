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

package chrony

import (
	"net"
	"time"
)

// original C++ versions of those consts/structs
// are in https://github.com/mlichvar/chrony/blob/master/candm.h

// ReplyType identifies reply packet type
type ReplyType uint16

// CommandType identifies command type in both request and repy
type CommandType uint16

// ModeType identifies source (peer) mode
type ModeType uint16

// SourceStateType identifies source (peer) state
type SourceStateType uint16

// ResponseStatusType identifies response status
type ResponseStatusType uint16

// PacketType - request or reply
type PacketType uint8

// we implement latest (at the moment) protocol version
const protoVersionNumber uint8 = 6
const maxDataLen = 396

// packet types
const (
	pktTypeCmdRequest PacketType = 1
	pktTypeCmdReply   PacketType = 2
)

// request types. Only those we suppor, there are more
const (
	reqNSources    CommandType = 14
	reqSourceData  CommandType = 15
	reqTracking    CommandType = 33
	reqServerStats CommandType = 54
)

// reply types
const (
	rpyNSources     ReplyType = 2
	rpySourceData   ReplyType = 3
	rpyTracking     ReplyType = 5
	rpyServerStats  ReplyType = 14
	rpyServerStats2 ReplyType = 22
)

// source modes
const (
	SourceModeClient ModeType = 0
	SourceModePeer   ModeType = 1
	SourceModeRef    ModeType = 2
)

// source state
const (
	SourceStateSync        SourceStateType = 0
	SourceStateUnreach     SourceStateType = 1
	SourceStateFalseTicket SourceStateType = 2
	SourceStateJittery     SourceStateType = 3
	SourceStateCandidate   SourceStateType = 4
	SourceStateOutlier     SourceStateType = 5
)

// source data flags
const (
	FlagNoselect uint16 = 0x1
	FlagPrefer   uint16 = 0x2
	FlagTrust    uint16 = 0x4
	FlagRequire  uint16 = 0x8
)

// ntpdata flags
const (
	NTPFlagsTests        uint16 = 0x3ff
	NTPFlagInterleaved   uint16 = 0x4000
	NTPFlagAuthenticated uint16 = 0x8000
)

// response status codes
//nolint:varcheck,deadcode,unused
const (
	sttSuccess            ResponseStatusType = 0
	sttFailed             ResponseStatusType = 1
	sttUnauth             ResponseStatusType = 2
	sttInvalid            ResponseStatusType = 3
	sttNoSuchSource       ResponseStatusType = 4
	sttInvalidTS          ResponseStatusType = 5
	sttNotEnabled         ResponseStatusType = 6
	sttBadSubnet          ResponseStatusType = 7
	sttAccessAllowed      ResponseStatusType = 8
	sttAccessDenied       ResponseStatusType = 9
	sttNoHostAccess       ResponseStatusType = 10
	sttSourceAlreadyKnown ResponseStatusType = 11
	sttTooManySources     ResponseStatusType = 12
	sttNoRTC              ResponseStatusType = 13
	sttBadRTCFile         ResponseStatusType = 14
	sttInactive           ResponseStatusType = 15
	sttBadSample          ResponseStatusType = 16
	sttInvalidAF          ResponseStatusType = 17
	sttBadPktVersion      ResponseStatusType = 18
	sttBadPktLength       ResponseStatusType = 19
)

// StatusDesc provides mapping from ResponseStatusType to string
var StatusDesc = [20]string{
	"SUCCESS",
	"FAILED",
	"UNAUTH",
	"INVALID",
	"NOSUCHSOURCE",
	"INVALIDTS",
	"NOTENABLED",
	"BADSUBNET",
	"ACCESSALLOWED",
	"ACCESSDENIED",
	"NOHOSTACCESS",
	"SOURCEALREADYKNOWN",
	"TOOMANYSOURCES",
	"NORTC",
	"BADRTCFILE",
	"INACTIVE",
	"BADSAMPLE",
	"INVALIDAF",
	"BADPKTVERSION",
	"BADPKTLENGTH",
}

// SourceStateDesc provides mapping from SourceStateType to string
var SourceStateDesc = [6]string{
	"sync",
	"unreach",
	"falseticket",
	"jittery",
	"candidate",
	"outlier",
}

// RequestHead is the first (common) part of the request,
// in a format that can be directly passed to binary.Write
type RequestHead struct {
	Version  uint8
	PKTType  PacketType
	Res1     uint8
	Res2     uint8
	Command  CommandType
	Attempt  uint16
	Sequence uint32
	Pad1     uint32
	Pad2     uint32
}

// GetCommand returns request packet command
func (r *RequestHead) GetCommand() CommandType {
	return r.Command
}

// SetSequence sets request packet sequence number
func (r *RequestHead) SetSequence(n uint32) {
	r.Sequence = n
}

// RequestPacket is an iterface to abstract all different outgoing packets
type RequestPacket interface {
	GetCommand() CommandType
	SetSequence(n uint32)
}

// ResponsePacket is an interface to abstract all different incoming packets
type ResponsePacket interface {
	GetCommand() CommandType
	GetType() PacketType
	GetStatus() ResponseStatusType
}

// RequestSources - packet to request number of sources (peers)
type RequestSources struct {
	RequestHead
	// we actually need this to send proper packet
	data [maxDataLen]uint8 //nolint:unused,structcheck
}

// RequestSourceData - packet to request source data for source id
type RequestSourceData struct {
	RequestHead
	Index int32
	EOR   int32
	// we pass i32 - 4 bytes
	data [maxDataLen - 4]uint8 //nolint:unused,structcheck
}

// RequestNTPData - packet to request NTP data for peer IP.
// As of now, it's only allowed by Chrony over unix socket connection.
type RequestNTPData struct {
	RequestHead
	IPAddr ipAddr
	EOR    int32
	// we pass at max ipv6 addr - 16 bytes
	data [maxDataLen - 16]uint8 //nolint:unused,structcheck
}

// RequestServerStats - packet to request server stats
type RequestServerStats struct {
	RequestHead
	// we actually need this to send proper packet
	data [maxDataLen]uint8 //nolint:unused,structcheck
}

// RequestTracking - packet to request 'tracking' data
type RequestTracking struct {
	RequestHead
	// we actually need this to send proper packet
	data [maxDataLen]uint8 //nolint:unused,structcheck
}

// ReplyHead is the first (common) part of the reply packet,
// in a format that can be directly passed to binary.Read
type ReplyHead struct {
	Version  uint8
	PKTType  PacketType
	Res1     uint8
	Res2     uint8
	Command  CommandType
	Reply    ReplyType
	Status   ResponseStatusType
	Pad1     uint16
	Pad2     uint16
	Pad3     uint16
	Sequence uint32
	Pad4     uint32
	Pad5     uint32
}

// GetCommand returns reply packet command
func (r *ReplyHead) GetCommand() CommandType {
	return r.Command
}

// GetType returns reply packet type
func (r *ReplyHead) GetType() PacketType {
	return r.PKTType
}

// GetStatus returns reply packet status
func (r *ReplyHead) GetStatus() ResponseStatusType {
	return r.Status
}

type replySourcesContent struct {
	NSources uint32
	EOR      int32
}

// ReplySources is a usable version of a reply to 'sources' command
type ReplySources struct {
	ReplyHead
	NSources int
}

type replySourceDataContent struct {
	IPAddr         ipAddr
	Poll           int16
	Stratum        uint16
	State          SourceStateType
	Mode           ModeType
	Flags          uint16
	Reachability   uint16
	SinceSample    uint32
	OrigLatestMeas chronyFloat
	LatestMeas     chronyFloat
	LatestMeasErr  chronyFloat
	EOR            int32
}

// SourceData contains parsed version of 'source data' reply
type SourceData struct {
	IPAddr         net.IP
	Poll           int16
	Stratum        uint16
	State          SourceStateType
	Mode           ModeType
	Flags          uint16
	Reachability   uint16
	SinceSample    uint32
	OrigLatestMeas float64
	LatestMeas     float64
	LatestMeasErr  float64
}

func newSourceData(r *replySourceDataContent) *SourceData {
	return &SourceData{
		IPAddr:         r.IPAddr.ToNetIP(),
		Poll:           r.Poll,
		Stratum:        r.Stratum,
		State:          r.State,
		Mode:           r.Mode,
		Flags:          r.Flags,
		Reachability:   r.Reachability,
		SinceSample:    r.SinceSample,
		OrigLatestMeas: r.OrigLatestMeas.ToFloat(),
		LatestMeas:     r.LatestMeas.ToFloat(),
		LatestMeasErr:  r.LatestMeasErr.ToFloat(),
	}
}

// ReplySourceData is a usable version of 'source data' reply for given source id
type ReplySourceData struct {
	ReplyHead
	SourceData
}

type replyTrackingContent struct {
	RefID              uint32
	IPAddr             ipAddr // our current sync source
	Stratum            uint16
	LeapStatus         uint16
	RefTime            timeSpec
	CurrentCorrection  chronyFloat
	LastOffset         chronyFloat
	RMSOffset          chronyFloat
	FreqPPM            chronyFloat
	ResidFreqPPM       chronyFloat
	SkewPPM            chronyFloat
	RootDelay          chronyFloat
	RootDispersion     chronyFloat
	LastUpdateInterval chronyFloat
	EOR                int32
}

// Tracking contains parsed version of 'tracking' reply
type Tracking struct {
	RefID              uint32
	IPAddr             net.IP
	Stratum            uint16
	LeapStatus         uint16
	RefTime            time.Time
	CurrentCorrection  float64
	LastOffset         float64
	RMSOffset          float64
	FreqPPM            float64
	ResidFreqPPM       float64
	SkewPPM            float64
	RootDelay          float64
	RootDispersion     float64
	LastUpdateInterval float64
}

func newTracking(r *replyTrackingContent) *Tracking {
	return &Tracking{
		RefID:              r.RefID,
		IPAddr:             r.IPAddr.ToNetIP(),
		Stratum:            r.Stratum,
		LeapStatus:         r.LeapStatus,
		RefTime:            r.RefTime.ToTime(),
		CurrentCorrection:  r.CurrentCorrection.ToFloat(),
		LastOffset:         r.LastOffset.ToFloat(),
		RMSOffset:          r.RMSOffset.ToFloat(),
		FreqPPM:            r.FreqPPM.ToFloat(),
		ResidFreqPPM:       r.ResidFreqPPM.ToFloat(),
		SkewPPM:            r.SkewPPM.ToFloat(),
		RootDelay:          r.RootDelay.ToFloat(),
		RootDispersion:     r.RootDispersion.ToFloat(),
		LastUpdateInterval: r.LastUpdateInterval.ToFloat(),
	}
}

// ReplyTracking has usable 'tracking' response
type ReplyTracking struct {
	ReplyHead
	Tracking
}

// ServerStats contains parsed version of 'serverstats' reply
type ServerStats struct {
	NTPHits  uint32
	CMDHits  uint32
	NTPDrops uint32
	CMDDrops uint32
	LogDrops uint32
}

// ReplyServerStats is a usable version of 'serverstats' response
type ReplyServerStats struct {
	ReplyHead
	ServerStats
}

// ServerStats2 contains parsed version of 'serverstats2' reply
type ServerStats2 struct {
	NTPHits     uint32
	NKEHits     uint32
	CMDHits     uint32
	NTPDrops    uint32
	NKEDrops    uint32
	CMDDrops    uint32
	LogDrops    uint32
	NTPAuthHits uint32
}

// ReplyServerStats2 is a usable version of 'serverstats2' response
type ReplyServerStats2 struct {
	ReplyHead
	ServerStats2
}

// here go request constuctors

// NewSourcesPacket creates new packet to request number of sources (peers)
func NewSourcesPacket() *RequestSources {
	return &RequestSources{
		RequestHead: RequestHead{
			Version: protoVersionNumber,
			PKTType: pktTypeCmdRequest,
			Command: reqNSources,
		},
	}
}

// NewTrackingPacket creates new packet to request 'tracking' information
func NewTrackingPacket() *RequestTracking {
	return &RequestTracking{
		RequestHead: RequestHead{
			Version: protoVersionNumber,
			PKTType: pktTypeCmdRequest,
			Command: reqTracking,
		},
	}
}

// NewSourceDataPacket creates new packet to request 'source data' information about source with given ID
func NewSourceDataPacket(sourceID int32) *RequestSourceData {
	return &RequestSourceData{
		RequestHead: RequestHead{
			Version: protoVersionNumber,
			PKTType: pktTypeCmdRequest,
			Command: reqSourceData,
		},
		Index: sourceID,
	}
}

// NewServerStatsPacket creates new packet to request 'serverstats' information
func NewServerStatsPacket() *RequestServerStats {
	return &RequestServerStats{
		RequestHead: RequestHead{
			Version: protoVersionNumber,
			PKTType: pktTypeCmdRequest,
			Command: reqServerStats,
		},
	}
}
