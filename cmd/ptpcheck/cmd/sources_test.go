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

package cmd

import (
	"bytes"
	"testing"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/stats"
	"github.com/stretchr/testify/require"
)

func TestSourcesRenderSPTP(t *testing.T) {
	umt := stats.Stats{
		{
			GMAddress:    "foo.test.example.com.",
			Selected:     true,
			PortIdentity: "aabbcc.fffe.ddeeff",
			ClockQuality: ptp.ClockQuality{ClockClass: 6, ClockAccuracy: 0x21, OffsetScaledLogVariance: 0x4e5d},
			Priority1:    128, Priority2: 128, Priority3: 1,
			Offset: -42, MeanPathDelay: 1234,
			CorrectionFieldTX: 100, CorrectionFieldRX: 200,
		},
		{
			GMAddress:    "bar.test.example.com.",
			Selected:     false,
			PortIdentity: "112233.fffe.445566",
			ClockQuality: ptp.ClockQuality{ClockClass: 6, ClockAccuracy: 0x21, OffsetScaledLogVariance: 0x4e5d},
			Priority1:    128, Priority2: 128, Priority3: 2,
			Offset: 87, MeanPathDelay: 5678,
			CorrectionFieldTX: 300, CorrectionFieldRX: 400,
		},
	}

	var buf bytes.Buffer
	err := sourcesRenderSPTP(&buf, umt, true)
	require.NoError(t, err)

	want := "" +
		"+----------+--------------------+-----------------------+--------+----------+-----------+------------+-----------+--------------+-------+\n" +
		"| SELECTED |      IDENTITY      |        ADDRESS        | CLOCK  | VARIANCE | P1:P2:P3  | OFFSET(NS) | DELAY(NS) | CF TX:RX(NS) | ERROR |\n" +
		"+----------+--------------------+-----------------------+--------+----------+-----------+------------+-----------+--------------+-------+\n" +
		"| true     | aabbcc.fffe.ddeeff | foo.test.example.com. | 6:0x21 | 0x4e5d   | 128:128:1 |        -42 |      1234 | 100:200      |       |\n" +
		"| false    | 112233.fffe.445566 | bar.test.example.com. | 6:0x21 | 0x4e5d   | 128:128:2 |         87 |      5678 | 300:400      |       |\n" +
		"+----------+--------------------+-----------------------+--------+----------+-----------+------------+-----------+--------------+-------+\n"
	require.Equal(t, want, buf.String())
}
