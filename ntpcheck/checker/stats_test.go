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

package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_noPeerExitCode(t *testing.T) {
	// Check that no "good" pier triggers exit code
	s := SystemVariables{}
	peers := make(map[uint16]*Peer, 1)
	peers[0] = &Peer{}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers:   peers,
	}
	_, err := NewNTPStats(r)
	assert.EqualError(t, err, "nothing to calculate stats from: no good peers present")
}
