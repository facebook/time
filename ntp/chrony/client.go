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
	"sync"
	"sync/atomic"
)

// Client talks to chronyd
type Client struct {
	Connection io.ReadWriter
	Sequence   uint32
	sync.Mutex
}

// Communicate sends the packet to chronyd, parse response into something usable
func (n *Client) Communicate(packet RequestPacket) (ResponsePacket, error) {
	seq := atomic.AddUint32(&n.Sequence, 1)
	packet.SetSequence(seq)

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, packet); err != nil {
		return nil, fmt.Errorf("failed to encode packet: %w", err)
	}

	response := make([]uint8, 1024)

	n.Lock()
	if _, err := n.Connection.Write(buf.Bytes()); err != nil {
		n.Unlock()
		return nil, fmt.Errorf("failed to write packet to connection: %w", err)
	}

	read, err := n.Connection.Read(response)
	n.Unlock()
	if err != nil {
		return nil, fmt.Errorf("connection.Read failed: %w", err)
	}

	if read == 0 {
		return nil, fmt.Errorf("no data received")
	}

	return decodePacket(response[:read])
}
