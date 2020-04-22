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
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	log "github.com/sirupsen/logrus"
)

// Client talks to chronyd
type Client struct {
	Connection io.ReadWriter
	Sequence   uint32
}

// Communicate sends the packet to chronyd, parse response into something usable
func (n *Client) Communicate(packet RequestPacket) (ResponsePacket, error) {
	n.Sequence++
	var err error
	packet.SetSequence(n.Sequence)
	err = binary.Write(n.Connection, binary.BigEndian, packet)
	if err != nil {
		return nil, err
	}
	response := make([]uint8, 1024)
	read, err := n.Connection.Read(response)
	if err != nil {
		return nil, err
	}
	log.Debugf("Read %d bytes", read)
	r := bytes.NewReader(response)
	head := new(replyHead)
	if err = binary.Read(r, binary.BigEndian, head); err != nil {
		return nil, err
	}
	log.Debugf("response head: %+v", head)
	if head.Status == sttUnauth {
		return nil, ErrNotAuthorized
	}
	if head.Status != sttSuccess {
		return nil, fmt.Errorf("got status %s", StatusDesc[head.Status])
	}
	switch head.Reply {
	case rpyNSources:
		data := new(replySourcesContent)
		if err = binary.Read(r, binary.BigEndian, data); err != nil {
			return nil, err
		}
		log.Debugf("response data: %+v", data)
		return &ReplySources{
			replyHead: *head,
			NSources:  int(data.NSources),
		}, nil
	case rpySourceData:
		data := new(replySourceDataContent)
		if err = binary.Read(r, binary.BigEndian, data); err != nil {
			return nil, err
		}
		log.Debugf("response data: %+v", data)
		return &ReplySourceData{
			replyHead:  *head,
			sourceData: *newSourceData(data),
		}, nil
	case rpyTracking:
		data := new(replyTrackingContent)
		if err = binary.Read(r, binary.BigEndian, data); err != nil {
			return nil, err
		}
		log.Debugf("response data: %+v", data)
		return &ReplyTracking{
			replyHead: *head,
			tracking:  *newTracking(data),
		}, nil
	case rpyServerStats:
		data := new(serverStats)
		if err = binary.Read(r, binary.BigEndian, data); err != nil {
			return nil, err
		}
		log.Debugf("response data: %+v", data)
		return &ReplyServerStats{
			replyHead:   *head,
			serverStats: *data,
		}, nil
	case rpyNTPData:
		data := new(replyNTPDataContent)
		if err = binary.Read(r, binary.BigEndian, data); err != nil {
			return nil, err
		}
		log.Debugf("response data: %+v", data)
		return &ReplyNTPData{
			replyHead: *head,
			ntpData:   *newNTPData(data),
		}, nil
	default:
		return nil, fmt.Errorf("not implemented reply type %d from %+v", head.Reply, head)
	}
}
