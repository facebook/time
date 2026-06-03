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

package ubx

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// realBannerTime03 is the actual stderr line cfgtool emitted when
// connecting to the OCP TimeCard's u-blox F9T on time03.qzi1.
const realBannerTime03 = "status: rx0: Receiver detected at baudrate 115200: TIM 2.01 (ZED-F9T)"

func TestParseBannerLineRealCapture(t *testing.T) {
	info, ok := parseBannerLine(realBannerTime03)
	require.True(t, ok)
	require.Equal(t, "TIM 2.01", info.Firmware)
	require.Equal(t, "ZED-F9T", info.Model)
	require.Equal(t, 115200, info.Baudrate)
}

func TestParseBannerLineDumpSubcommandPrefix(t *testing.T) {
	info, ok := parseBannerLine(
		"dump: rx0: Receiver detected at baudrate 9600: ROM SPG 5.10 (NEO-M9N)",
	)
	require.True(t, ok)
	require.Equal(t, "ROM SPG 5.10", info.Firmware)
	require.Equal(t, "NEO-M9N", info.Model)
	require.Equal(t, 9600, info.Baudrate)
}

func TestParseBannerLineWithExtraWhitespace(t *testing.T) {
	info, ok := parseBannerLine(
		"  status: rx0: Receiver detected at baudrate 38400:   TIM 2.20  ( RCB-F9T )  ",
	)
	require.True(t, ok)
	require.Equal(t, "TIM 2.20", info.Firmware)
	require.Equal(t, "RCB-F9T", info.Model)
	require.Equal(t, 38400, info.Baudrate)
}

func TestParseBannerLineNonBannerLine(t *testing.T) {
	for _, line := range []string{
		"",
		"status: rx0: Connecting to receiver at port ser:///dev/ttyS4",
		"status: Dumping receiver status...",
		"random log line",
		"Receiver detected at baudrate XYZ: foo (bar)", // non-numeric baud
	} {
		_, ok := parseBannerLine(line)
		require.Falsef(t, ok, "line should not match: %q", line)
	}
}

func TestFirmwareVersionPopulated(t *testing.T) {
	info := &StatusInfo{Firmware: "TIM 2.01", Model: "ZED-F9T", Baudrate: 115200}
	v, err := info.FirmwareVersion()
	require.NoError(t, err)
	require.Equal(t, "TIM 2.01", v)
}

func TestFirmwareVersionMissing(t *testing.T) {
	info := &StatusInfo{}
	_, err := info.FirmwareVersion()
	require.ErrorIs(t, err, ErrBannerNotFound)
}

func TestScanForBannerStdoutMatch(t *testing.T) {
	stdout := strings.NewReader(
		"some prefix line\n" +
			realBannerTime03 + "\n" +
			"more streaming output\n",
	)
	stderr := strings.NewReader("")

	killCalled := false
	info, err := scanForBanner(stdout, stderr, func() error {
		killCalled = true
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "TIM 2.01", info.Firmware)
	require.True(t, killCalled, "kill should be invoked once banner is found")
}

func TestScanForBannerStderrMatch(t *testing.T) {
	stdout := strings.NewReader("")
	stderr := strings.NewReader(realBannerTime03 + "\n")

	info, err := scanForBanner(stdout, stderr, func() error { return nil })
	require.NoError(t, err)
	require.Equal(t, "ZED-F9T", info.Model)
}

func TestScanForBannerNoMatch(t *testing.T) {
	stdout := strings.NewReader("nothing relevant here\n")
	stderr := strings.NewReader("status: Connecting...\n")

	_, err := scanForBanner(stdout, stderr, func() error { return nil })
	require.ErrorIs(t, err, ErrBannerNotFound)
	require.Contains(t, err.Error(), "nothing relevant here")
	require.Contains(t, err.Error(), "status: Connecting...")
}

func TestScanForBannerPortLocked(t *testing.T) {
	// Real cfgtool stderr output observed on time03.qzi1 when
	// oscillatord was holding /dev/ttyS4 open.
	stdout := strings.NewReader("")
	stderr := strings.NewReader(
		"dump: rx0: Connecting to receiver at port ser:///dev/ttyS4\n" +
			"dump: port(ser:///dev/ttyS4@9600) Failed locking device: Resource temporarily unavailable\n",
	)

	_, err := scanForBanner(stdout, stderr, func() error { return nil })
	require.ErrorIs(t, err, ErrPortLocked)
	require.NotErrorIs(t, err, ErrBannerNotFound)
	require.Contains(t, err.Error(), "Failed locking device")
}

func TestScanForBannerPortLockedEAGAINOnly(t *testing.T) {
	// Some kernels / cfgtool versions may surface only the EAGAIN
	// substring without "Failed locking device". Match on either.
	stdout := strings.NewReader("")
	stderr := strings.NewReader(
		"dump: serial open: Resource temporarily unavailable\n",
	)

	_, err := scanForBanner(stdout, stderr, func() error { return nil })
	require.ErrorIs(t, err, ErrPortLocked)
}

func TestScanForBannerInterleavedStreams(t *testing.T) {
	// Banner arrives on stderr while stdout is producing streaming
	// data; scanForBanner should pick the banner up immediately.
	stdout := bytes.NewBufferString(
		"epoch 1 data\nepoch 2 data\nepoch 3 data\n",
	)
	stderr := bytes.NewBufferString(realBannerTime03 + "\n")

	info, err := scanForBanner(stdout, stderr, func() error { return nil })
	require.NoError(t, err)
	require.Equal(t, "TIM 2.01", info.Firmware)
}
