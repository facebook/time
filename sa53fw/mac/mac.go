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

package mac

import (
	"bytes"
	"fmt"

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

// ErrFWFormat is to check whether we should upgrade broken device
var ErrFWFormat = fmt.Errorf("SA53 cannot return FW version, possibly broken image")

// Mac represents SA53 MAC object
type Mac struct {
	device   string
	port     serial.Port
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
		return nil, err
	}

	m := &Mac{
		device: device,
		port:   port,
	}
	return m, nil
}

// Close is to close serial port
func (m *Mac) Close() {
	m.port.Close()
}

func (m *Mac) parseMacFirmware(fw string) error {
	// We know that firmware version is always returned as following string
	// [=Vx.x.xx.0.XXXXXX,Vy.y] with CPU and FPGA FW version
	// We care about CPU version only
	n, err := fmt.Sscanf(fw, "[=V%d.%d.%d.0.%8s", &m.fwMajor, &m.fwMinor, &m.fwPatch, &m.fwCommit)
	if n != 4 {
		return fmt.Errorf("wrong firmware version format: %s, %d", fw, n)
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
