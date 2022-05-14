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

package clock

import (
	"fmt"
	"net"
	"time"

	osc "github.com/facebook/time/oscillatord"
	ptp "github.com/facebook/time/ptp/protocol"
)

const timeout = time.Second

type oscillatorState struct {
	Offset     time.Duration
	ClockClass ptp.ClockClass
}

// https://datatracker.ietf.org/doc/html/rfc8173#section-7.6.2.4
// https://datatracker.ietf.org/doc/html/rfc8173#section-7.6.2.5
func oscillatorStateFromStatus(status *osc.Status) *oscillatorState {
	c := &oscillatorState{
		ClockClass: ClockClassUncalibrated,
		Offset:     0,
	}

	// Safety check in case oscillatord returns an empty struct
	if status.Clock.Class > 0 {
		c.ClockClass = ptp.ClockClass(status.Clock.Class)
		c.Offset = status.Clock.Offset
	}

	return c
}

func oscillatord() (*oscillatorState, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", osc.MonitoringPort))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}

	status, err := osc.ReadStatus(conn)
	if err != nil {
		return nil, err
	}
	return oscillatorStateFromStatus(status), nil
}
