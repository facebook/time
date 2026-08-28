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

package daemon

import (
	"cmp"
	"fmt"
	"net"
	"time"

	"github.com/facebook/time/ntp/chrony"

	log "github.com/sirupsen/logrus"
)

// chronyAddress is chronyd's cmdmon endpoint. Not configurable: the bound
// describes this host's clock, so tracking must come from this host's chronyd.
const chronyAddress = "127.0.0.1:323"

// ChronyFetcher reads tracking data from chronyd. FetchGMs and FetchStats are
// left to the embedded interface; chrony mode never calls them.
type ChronyFetcher struct {
	DataFetcher
	// address overrides chronyAddress, for tests only
	address string
}

// connect dials chronyd's cmdmon over UDP: same protocol as its unix socket,
// allowed from localhost by default, so no privileges or bind-mounted socket.
func (cf *ChronyFetcher) connect(cfg *Config) (*chrony.Client, net.Conn, error) {
	timeout := cfg.Interval / 2
	address := cmp.Or(cf.address, chronyAddress)
	conn, err := net.DialTimeout("udp", address, timeout)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to chronyd at %s: %w", address, err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to set read deadline: %w", err)
	}
	return &chrony.Client{Connection: conn}, conn, nil
}

// FetchTracking fetches tracking data from chronyd
func (cf *ChronyFetcher) FetchTracking(cfg *Config) (*chrony.Tracking, error) {
	client, conn, err := cf.connect(cfg)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Warningf("closing chronyd connection: %v", err)
		}
	}()

	packet, err := client.Communicate(chrony.NewTrackingPacket())
	if err != nil {
		return nil, fmt.Errorf("failed to get tracking from chronyd: %w", err)
	}
	tracking, ok := packet.(*chrony.ReplyTracking)
	if !ok {
		return nil, fmt.Errorf("unexpected response type from chronyd: %T", packet)
	}
	log.Debugf("chrony tracking: %+v", tracking.Tracking)
	return &tracking.Tracking, nil
}
