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

package ntpshm

import (
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func Test_NTPSHMStruct(t *testing.T) {
	testBytes := []byte{1, 0, 0, 0, 236, 72, 0, 0, 110, 133, 75, 96, 0, 0, 0, 0, 122, 156, 13, 0, 0, 0, 0, 0, 72, 133, 75, 96, 0, 0, 0, 0, 169, 250, 6, 0, 0, 0, 0, 0, 226, 255, 255, 255, 0, 0, 0, 0, 0, 0, 0, 0, 70, 63, 43, 53, 50, 36, 67, 27, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	testNTPSHM := NTPSHM{
		Mode:                 1,
		Count:                18668,
		ClockTimeStampSec:    1615562094,
		ClockTimeStampUSec:   892026,
		ReceiveTimeStampSec:  1615562056,
		ReceiveTimeStampUSec: 457385,
		Leap:                 0,
		Precision:            -30,
		Nsamples:             0,
		Valid:                0,
		ClockTimeStampNSec:   892026694,
		ReceiveTimeStampNSec: 457385010,
		Dummy:                [8]int32{0, 0, 0, 0, 0, 0, 0, 0},
	}

	assert.Equal(t, NTPSHMSize, int(unsafe.Sizeof(testNTPSHM)))

	s := ptrToNTPSHM(uintptr(unsafe.Pointer(&testBytes[0])))
	assert.Equal(t, testNTPSHM, *s)

	assert.True(t, time.Unix(1615562056, 457385010).Equal(s.ReceiveTimeStamp()))
	assert.True(t, time.Unix(1615562094, 892026694).Equal(s.ClockTimeStamp()))
}

func Test_NTPSHMReadID(t *testing.T) {
	id, err := Create()
	// Happens when we have no permissions
	if err != nil {
		t.SkipNow()
	}
	assert.NotEqual(t, 0, id)

	shm, err := ReadID(id)
	assert.Nil(t, err)
	assert.NotNil(t, shm)
}
