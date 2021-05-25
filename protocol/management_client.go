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

// management client is used to talk to (presumably local) PTP server using Management packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

// MgmtClient talks to ptp server over unix socket
type MgmtClient struct {
	Connection io.ReadWriter
	Sequence   uint16
}

// SendPacket sends packet, incrementing sequence counter
func (c *MgmtClient) SendPacket(packet Packet) error {
	c.Sequence++
	packet.SetSequence(c.Sequence)
	return binary.Write(c.Connection, binary.BigEndian, packet)
}

// Communicate sends the management the packet, parses response into something usable
func (c *MgmtClient) Communicate(packet ManagementPacket) (Packet, error) {
	var err error

	if err := c.SendPacket(packet); err != nil {
		return nil, err
	}
	response := make([]uint8, 1024)
	n, err := c.Connection.Read(response)
	if err != nil {
		return nil, err
	}
	p, err := decodeMgmtPacket(response[:n])
	if err != nil {
		return nil, err
	}
	errorPacket, ok := p.(*ManagementMsgErrorStatus)
	if ok {
		return nil, fmt.Errorf("got Management Error in response: %v", errorPacket.ManagementErrorStatusTLV.ManagementErrorID)
	}
	return p, nil
}

// ParentDataSet sends PARENT_DATA_SET request and returns response
func (c *MgmtClient) ParentDataSet() (*ParentDataSetTLV, error) {
	req := ParentDataSetRequest()
	res, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	p, ok := res.(*ManagementMsgParentDataSet)
	if !ok {
		return nil, fmt.Errorf("got unexpected management packet %T, expected %T", res, p)
	}
	return &p.ParentDataSetTLV, nil
}

// DefaultDataSet sends DEFAULT_DATA_SET request and returns response
func (c *MgmtClient) DefaultDataSet() (*DefaultDataSetTLV, error) {
	req := DefaultDataSetRequest()
	res, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	p, ok := res.(*ManagementMsgDefaultDataSet)
	if !ok {
		return nil, fmt.Errorf("got unexpected management packet %T, expected %T", res, p)
	}
	return &p.DefaultDataSetTLV, nil
}

// CurrentDataSet sends CURRENT_DATA_SET request and returns response
func (c *MgmtClient) CurrentDataSet() (*CurrentDataSetTLV, error) {
	req := CurrentDataSetRequest()
	res, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	p, ok := res.(*ManagementMsgCurrentDataSet)
	if !ok {
		return nil, fmt.Errorf("got unexpected management packet %T, expected %T", res, p)
	}
	return &p.CurrentDataSetTLV, nil
}
