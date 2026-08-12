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

package control

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// headSizeBytes is the wire size of NTPControlMsgHead, which the Data section follows.
const headSizeBytes = 12

// NTPClient is our client to talk to network. The main reason it exists is keeping track of Sequence number.
type NTPClient struct {
	Sequence   uint16
	Connection io.ReadWriter
}

// CommunicateWithData sends package + data over connection, bumps Sequence num and parses (possibly multiple) response packets into NTPControlMsg packet.
// This function will always return single NTPControlMsg, even if under the hood it was split across multiple packets.
// Resulting NTPControlMsg will have Data section composed of combined Data sections from all packages.
func (n *NTPClient) CommunicateWithData(packet *NTPControlMsgHead, data []uint8) (*NTPControlMsg, error) {
	packet.Sequence = n.Sequence
	if len(data) > 0 {
		packet.Count = uint16(len(data))
	}
	n.Sequence++
	// create a buffer where we can compose full payload
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, packet)
	if err != nil {
		return nil, err
	}
	err = binary.Write(buf, binary.BigEndian, data)
	if err != nil {
		return nil, err
	}
	// send full payload
	_, err = n.Connection.Write(buf.Bytes())
	if err != nil {
		return nil, err
	}
	var resultHead *NTPControlMsgHead
	resultData := make([]uint8, 0)
	// read packets till More flag is not set
	for {
		response := make([]uint8, 1024)
		head := new(NTPControlMsgHead)
		read, err := n.Connection.Read(response)
		if err != nil {
			return nil, err
		}
		if read < headSizeBytes {
			return nil, fmt.Errorf("truncated response: got %d bytes, want at least %d", read, headSizeBytes)
		}
		r := bytes.NewReader(response[:headSizeBytes])
		if err = binary.Read(r, binary.BigEndian, head); err != nil {
			return nil, err
		}
		if int(head.Count) > read-headSizeBytes {
			return nil, fmt.Errorf("response declares %d data octets, but only %d were received", head.Count, read-headSizeBytes)
		}
		resultData = append(resultData, response[headSizeBytes:headSizeBytes+int(head.Count)]...)
		if !head.HasMore() {
			resultHead = head
			break
		}
	}
	return &NTPControlMsg{NTPControlMsgHead: *resultHead, Data: resultData}, nil
}

// Communicate sends package over connection, bumps Sequence num and parses (possibly multiple) response packets into NTPControlMsg packet.
// This function will always return single NTPControlMsg, even if under the hood it was split across multiple packets.
// Resulting NTPControlMsg will have Data section composed of combined Data sections from all packages.
func (n *NTPClient) Communicate(packet *NTPControlMsgHead) (*NTPControlMsg, error) {
	return n.CommunicateWithData(packet, nil)
}
