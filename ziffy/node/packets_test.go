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

package node

import (
	"testing"

	ptp "github.com/facebookincubator/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestFormSyncPacket(t *testing.T) {
	pkt := formSyncPacket(ptp.MessageSync, 4, 33000)
	require.Equal(t, uint8(ZiffyHexa), pkt.ControlField)
	require.Equal(t, uint16(4), pkt.SequenceID)
	require.Equal(t, uint16(33000), pkt.SourcePortIdentity.PortNumber)
	require.Equal(t, ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0), pkt.SdoIDAndMsgType)

	pkt = formSyncPacket(ptp.MessageDelayReq, 7, 12345)
	require.Equal(t, uint8(ZiffyHexa), pkt.ControlField)
	require.Equal(t, uint16(7), pkt.SequenceID)
	require.Equal(t, uint16(12345), pkt.SourcePortIdentity.PortNumber)
	require.Equal(t, ptp.NewSdoIDAndMsgType(ptp.MessageDelayReq, 0), pkt.SdoIDAndMsgType)
}
