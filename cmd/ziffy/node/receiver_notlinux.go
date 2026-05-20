//go:build !linux

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
	"errors"
	"net"
	"time"

	"github.com/facebook/time/ptp/protocol"
	"github.com/google/gopacket"
)

type Receiver struct {
	Config          *Config
	runningHandlers int64
}

type Sender struct {
	Config     *Config
	inputQueue []chan *SwitchTrafficInfo
}

func (r *Receiver) incRunningHandlers() int64 {
	r.runningHandlers++
	return r.runningHandlers
}

func (r *Receiver) decRunningHandlers() int64 {
	r.runningHandlers--
	return r.runningHandlers
}

func (r *Receiver) Start() error {
	return errors.New("receiver unsupported on non-linux")
}

func (s *Sender) Start() ([]*PathInfo, error) {
	return nil, errors.New("sender unsupported on non-linux")
}

func NewReceiver(...any) (*Receiver, error) {
	return nil, errors.New("receiver unsupported on non-linux")
}

func NewSender(...any) (*Sender, error) {
	return nil, errors.New("sender unsupported on non-linux")
}

func (s *Sender) popAllQueue(_ []*PathInfo) {
	s.inputQueue = nil
}

func (r *Receiver) handlePacket(_ gopacket.Packet) {
}

func parseSyncPacket(_ gopacket.Packet) (*protocol.SyncDelayReq, string, string, error) {
	return nil, "", "", errors.New("unsupported on darwin")
}

func (s *Sender) clearPaths(routes []*PathInfo) []*PathInfo {
	return routes
}

func sortSwitchesByHop(_ []SwitchTrafficInfo) {}

func formNewDest(_ *Config, _ int) net.IP {
	return nil
}

func rackSwHostnameMonitor(_ string, _ time.Duration) (string, error) {
	return "", errors.New("unsupported on darwin")
}
