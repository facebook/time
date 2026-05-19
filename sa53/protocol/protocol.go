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

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"go.bug.st/serial"
)

const (
	cmdGetFirmwareVersion string = "\\{swrev?}"
	cmdReset              string = "{reset}"
	cmdBsl                string = "{bsl}"
	cmdUpgrade            string = "{upload,cpu}"
	ansBsl                string = "[=BSL]"
	ansReset              string = "[>Loading...]"
	ansBoot               string = "[>Microchip SA5X]"
	ansComplete           string = "[=1]"
	ansError              string = "[!1]"
)

// normalReadTimeout is the read timeout Drain restores on its way out. Match
// the bookmark's choice; callers that want a different value should call
// SetReadTimeout themselves after Drain returns.
const normalReadTimeout = 2 * time.Second

// drainReadTimeout is the short timeout Drain installs while flushing.
const drainReadTimeout = 10 * time.Millisecond

// maxResponseBytes bounds how much we'll buffer waiting for "\r\n". Anything
// larger than this is almost certainly a runaway from a wedged chip.
const maxResponseBytes = 4096

// ErrFWFormat is to check whether we should upgrade broken device
var ErrFWFormat = fmt.Errorf("SA53 cannot return FW version, possibly broken image")

// ErrDeviceError is wrapped into the error returned by ParseValue / Get when
// the SA53 replies with a "[!N]" device error. Use errors.Is to detect.
var ErrDeviceError = errors.New("SA53 device error")

// SerialReadWriter is the subset of serial.Port the protocol package uses. It
// lets callers wrap the underlying port with tees (e.g. raw byte logging) or
// substitute fakes in tests.
type SerialReadWriter interface {
	io.ReadWriter
	SetReadTimeout(time.Duration) error
}

// Mac represents SA53 MAC object
type Mac struct {
	device   string
	port     SerialReadWriter
	closer   io.Closer // nil = caller owns the port
	fwMajor  int
	fwMinor  int
	fwPatch  int
	fwCommit string
}

// Init is to create Mac structure and open serial port
func Init(device string) (*Mac, error) {
	mode := &serial.Mode{
		BaudRate: 57600,
	}

	port, err := serial.Open(device, mode)
	if err != nil {
		return nil, fmt.Errorf("cannot open serial port %s: %w", device, err)
	}

	m := &Mac{
		device: device,
		port:   port,
		closer: port,
	}
	return m, nil
}

// Close is to close serial port
func (m *Mac) Close() {
	if m.closer != nil {
		m.closer.Close()
	}
}

// SetReadTimeout sets the read timeout on the underlying port.
func (m *Mac) SetReadTimeout(d time.Duration) error {
	return m.port.SetReadTimeout(d)
}

// WrapPort installs a wrapping interceptor around the underlying port. The
// callback receives the current port and returns a wrapped version that will
// be used for all subsequent reads and writes. Use this to inject a raw-byte
// logging tee (rawlog.LoggingPort) without leaking serial-library knowledge
// into the caller.
func (m *Mac) WrapPort(wrap func(SerialReadWriter) SerialReadWriter) {
	m.port = wrap(m.port)
}

// Drain consumes pending bytes on the port without blocking, then restores
// the normal read timeout. Call before Cmd / Get to clear stale or
// asynchronous bytes left over from a previous interaction.
func (m *Mac) Drain() {
	if err := m.port.SetReadTimeout(drainReadTimeout); err != nil {
		return
	}
	defer func() { _ = m.port.SetReadTimeout(normalReadTimeout) }()
	tmp := make([]byte, 256)
	for {
		n, err := m.port.Read(tmp)
		if err != nil || n == 0 {
			return
		}
	}
}

// ReadResponse reads from the port until it sees a "\r\n" terminator or a
// read error. Returns the response without the terminator. A read that
// returns (0, nil) is treated as a timeout and surfaces as an error so
// callers don't quietly accept truncated responses.
func (m *Mac) ReadResponse() (string, error) {
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 64)
	for {
		n, err := m.port.Read(tmp)
		if err != nil {
			return "", fmt.Errorf("read: %w", err)
		}
		if n == 0 {
			return "", fmt.Errorf("read timeout, partial response: %q", string(buf))
		}
		buf = append(buf, tmp[:n]...)
		if before, _, ok := bytes.Cut(buf, []byte("\r\n")); ok {
			return string(before), nil
		}
		if len(buf) > maxResponseBytes {
			return "", fmt.Errorf("response exceeded %d bytes without terminator: %q", maxResponseBytes, string(buf))
		}
	}
}

// Cmd drains pending bytes, writes raw to the port, and returns the
// response. Use for arbitrary protocol commands like "\{swrev?}" sent from
// the `--cmd` path.
func (m *Mac) Cmd(raw string) (string, error) {
	m.Drain()
	if _, err := m.port.Write([]byte(raw)); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return m.ReadResponse()
}

// Get queries a single named parameter via "{get,<param>}" and returns the
// parsed value. Device errors ("[!N]") are surfaced as errors wrapping
// ErrDeviceError. Callers should Drain() on non-device-error failures
// before retrying so a wedged port doesn't stay wedged.
func (m *Mac) Get(param string) (string, error) {
	if _, err := m.port.Write([]byte("{get," + param + "}")); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	resp, err := m.ReadResponse()
	if err != nil {
		return "", err
	}
	return ParseValue(resp)
}

// ParseValue extracts the value from a "[=value]" response. "[!N]" replies
// are surfaced as errors wrapping ErrDeviceError with the numeric code.
func ParseValue(resp string) (string, error) {
	if v, ok := strings.CutPrefix(resp, "[!"); ok {
		if code, ok := strings.CutSuffix(v, "]"); ok {
			return "", fmt.Errorf("%w: %s", ErrDeviceError, code)
		}
	}
	if v, ok := strings.CutPrefix(resp, "[="); ok {
		if val, ok := strings.CutSuffix(v, "]"); ok {
			return val, nil
		}
	}
	return "", fmt.Errorf("unexpected response: %q", resp)
}

func (m *Mac) parseMacFirmware(fw string) error {
	// We know that firmware version is always returned as following string
	// [=Vx.x.xx.0.XXXXXX,Vy.y] with CPU and FPGA FW version
	// We care about CPU version only
	n, err := fmt.Sscanf(fw, "[=V%d.%d.%d.0.%8s", &m.fwMajor, &m.fwMinor, &m.fwPatch, &m.fwCommit)
	if n != 4 {
		return fmt.Errorf("wrong firmware version format: got %q (parsed %d/4 fields, expected '[=Vx.x.x.0.XXXXXXXX,Vy.y]')", fw, n)
	}
	return err
}

// ReadFirmware is used to read firmware srting and parse it into Mac structure
func (m *Mac) ReadFirmware() error {
	res, err := m.cmdResult(cmdGetFirmwareVersion)
	if err != nil {
		return err
	}

	if res == ansError {
		return ErrFWFormat
	}

	err = m.parseMacFirmware(res)
	return err
}

// FormatFWVersion return string representation of firmware version
func (m *Mac) FormatFWVersion() string {
	return fmt.Sprintf("V%d.%d.%d", m.fwMajor, m.fwMinor, m.fwPatch)
}

// Version returns firmware as an int value
func (m *Mac) Version() int {
	return m.fwMajor*0x10000 + m.fwMinor*0x100 + m.fwPatch
}

// readResult is the firmware-side reader. Fills a buffer until the last two
// bytes are "\r\n". Kept distinct from ReadResponse because the firmware
// upgrade flow has been validated against this exact behavior on real
// hardware; do not redirect callers without re-validating.
func (m *Mac) readResult() (string, error) {
	var r int
	buff := make([]byte, 4101)
	for {
		n, err := m.Read(buff[r:])
		if err != nil {
			return "", err
		}

		if n == 0 {
			break
		}
		r += n

		if bytes.Equal(buff[r-2:r], []byte("\r\n")) {
			break
		}
	}
	return string(buff[:r-2]), nil
}

// WaitBoot waiting for boot logo to show
func (m *Mac) WaitBoot() error {
	res, err := m.readResult()
	if err != nil {
		return err
	}

	if res != ansBoot {
		return fmt.Errorf("wrong answer after boot: %s", res)
	}

	return nil
}

// cmdResult is the firmware-side write+read helper. Distinct from Cmd so the
// firmware upgrade flow keeps its validated behavior (no drain, fixed-buffer
// reader).
func (m *Mac) cmdResult(cmd string) (string, error) {
	_, err := m.port.Write([]byte(cmd))
	if err != nil {
		return "", err
	}
	return m.readResult()
}

// Reset is to reset the MAC
func (m *Mac) Reset() error {
	res, err := m.cmdResult(cmdReset)
	if err != nil {
		return err
	}

	if res != ansReset {
		return fmt.Errorf("reset command fail, the result is %s", res)
	}
	return nil
}

// Upgrade is to switch MAC to Upgrade mode
func (m *Mac) Upgrade() error {
	res, err := m.cmdResult(cmdBsl)
	if err != nil {
		return err
	}

	if res != ansBsl {
		return fmt.Errorf("switch to Upgrade mode error, answer: %s", res)
	}
	return nil
}

// XModemInit is to init XModem receiver in MAC
func (m *Mac) XModemInit() error {
	_, err := m.Write([]byte(cmdUpgrade))
	if err != nil {
		return err
	}
	b := make([]byte, 2)
	n, err := m.port.Read(b)
	if err != nil {
		return err
	}
	if n == 0 || b[0] != 'C' {
		return fmt.Errorf("device doesn't wait for XModem transfer")
	}
	return nil
}

// XModemDone is to check the result of XModem transmission
func (m *Mac) XModemDone() error {
	res, err := m.readResult()
	if err != nil {
		return fmt.Errorf("firmware upgrade completed with error, answer: %w", err)
	}

	if res != ansComplete {
		return fmt.Errorf("firmware upgrade completed with error, answer: %s", res)
	}
	return nil
}

// Read implements io.Reader interface
func (m *Mac) Read(b []byte) (int, error) {
	return m.port.Read(b)
}

// Write implements io.Writer interface
func (m *Mac) Write(b []byte) (int, error) {
	return m.port.Write(b)
}
