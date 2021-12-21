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

package shm

import (
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestNTPSHMStruct(t *testing.T) {
	testBytes := []byte{1, 0, 0, 0, 240, 64, 0, 0, 189, 86, 202, 96, 0, 0, 0, 0, 51, 1, 0, 0, 189, 86, 202, 96, 0, 0, 0, 0, 34, 252, 0, 0, 0, 0, 0, 0, 236, 255, 255, 255, 3, 0, 0, 0, 0, 0, 0, 0, 121, 176, 4, 0, 182, 231, 216, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	testNTPSHM := NTPSHM{
		Mode:                 1,
		Count:                16624,
		ClockTimeStampSec:    1623873213,
		ClockTimeStampUSec:   307,
		ReceiveTimeStampSec:  1623873213,
		ReceiveTimeStampUSec: 64546,
		Leap:                 0,
		Precision:            -20,
		Nsamples:             3,
		Valid:                0,
		ClockTimeStampNSec:   307321,
		ReceiveTimeStampNSec: 64546742,
		Dummy:                [8]int32{0, 0, 0, 0, 0, 0, 0, 0},
	}

	s, err := ptrToNTPSHM(uintptr(unsafe.Pointer(&testBytes[0])))
	require.NoError(t, err)
	require.Equal(t, testNTPSHM, *s)

	require.True(t, time.Unix(1623873213, 307321).Equal(s.ClockTimeStamp()))
	require.True(t, time.Unix(1623873213, 64546742).Equal(s.ReceiveTimeStamp()))
}

func TestNTPSHMReadID(t *testing.T) {
	id, err := Create()
	// Happens when we have no permissions
	if err != nil {
		t.SkipNow()
	}
	require.NotEqual(t, 0, id)

	shm, err := ReadID(id)
	require.NoError(t, err)
	require.NotNil(t, shm)
}
