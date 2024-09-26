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

package phc

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/facebook/time/servo"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type Finisher func()

func SetupMocks(t *testing.T) (servoMock *MockServoController, srcMock *MockTimestamper, mockDeviceController *MockDeviceController, finish Finisher) {
	dstController := gomock.NewController(t)
	srcController := gomock.NewController(t)
	servoController := gomock.NewController(t)
	defer srcController.Finish()
	defer dstController.Finish()
	defer servoController.Finish()
	servoMock = NewMockServoController(servoController)
	mockDeviceController = NewMockDeviceController(dstController)
	mockTimestamper := NewMockTimestamper(srcController)
	return servoMock, mockTimestamper, mockDeviceController, func() {
		srcController.Finish()
		dstController.Finish()
		servoController.Finish()
	}
}

func TestActivatePPSSource(t *testing.T) {
	// Prepare
	_, _, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	var actualPeroutRequest PTPPeroutRequest
	gomock.InOrder(
		// Should set default pin to PPS
		mockDeviceController.EXPECT().setPinFunc(uint(0), PinFuncPerOut, uint(0)).Return(nil),
		// Should call Time once
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500000000), nil),
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(nil).Do(func(arg PTPPeroutRequest) { actualPeroutRequest = arg }),
	)

	// Should call setPTPPerout with correct parameters
	expectedPeroutRequest := PTPPeroutRequest{
		Index:        uint32(0),
		Flags:        uint32(2),
		StartOrPhase: PTPClockTime{Sec: 2},
		Period:       PTPClockTime{Sec: 1},
		On:           PTPClockTime{NSec: 500000000},
	}

	// Act
	ppsSource, err := ActivatePPSSource(mockDeviceController)

	// Assert
	require.NoError(t, err)
	require.EqualValues(t, expectedPeroutRequest, actualPeroutRequest, "setPTPPerout parameter mismatch")
	require.Equal(t, PPSSet, ppsSource.state)
}

func TestActivatePPSSourceIgnoreSetPinFailure(t *testing.T) {
	// Prepare
	_, _, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	gomock.InOrder(
		// If ioctl set pin fails, we continue bravely on...
		mockDeviceController.EXPECT().setPinFunc(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("error")),
		mockDeviceController.EXPECT().File().Return(os.NewFile(3, "mock_file")),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500000000), nil),
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(nil),
	)

	// Act
	ppsSource, err := ActivatePPSSource(mockDeviceController)

	// Assert
	require.NoError(t, err)
	require.Equal(t, PPSSet, ppsSource.state)
}

func TestActivatePPSSourceSetPTPPeroutFailure(t *testing.T) {
	// Prepare
	_, _, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	var actualPeroutRequest PTPPeroutRequest
	gomock.InOrder(
		mockDeviceController.EXPECT().setPinFunc(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("error")),
		mockDeviceController.EXPECT().File().Return(os.NewFile(3, "mock_file")),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500000000), nil),
		// If first attempt to set PTPPerout fails
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(fmt.Errorf("error")),
		// Should retry setPTPPerout with backward compatible flag
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(nil).Do(func(arg PTPPeroutRequest) { actualPeroutRequest = arg }),
	)
	expectedPeroutRequest := PTPPeroutRequest{
		Index:        uint32(0),
		Flags:        uint32(0x0),
		StartOrPhase: PTPClockTime{Sec: 2},
		Period:       PTPClockTime{Sec: 1},
		On:           PTPClockTime{NSec: 500000000},
	}

	// Act
	ppsSource, err := ActivatePPSSource(mockDeviceController)

	// Assert
	require.NoError(t, err)
	require.EqualValues(t, expectedPeroutRequest, actualPeroutRequest, "setPTPPerout parameter mismatch")
	require.Equal(t, PPSSet, ppsSource.state)
}

func TestActivatePPSSourceSetPTPPeroutDoubleFailure(t *testing.T) {
	// Prepare
	_, _, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	gomock.InOrder(
		mockDeviceController.EXPECT().setPinFunc(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("error")),
		mockDeviceController.EXPECT().File().Return(os.NewFile(3, "mock_file")),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500000000), nil),
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(fmt.Errorf("error")),
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(fmt.Errorf("error")),
	)

	// Act
	ppsSource, err := ActivatePPSSource(mockDeviceController)

	// Assert
	require.Error(t, err)
	require.Nil(t, ppsSource)
}

func TestGetPPSTimestampSourceUnset(t *testing.T) {
	_, _, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSource := PPSSource{PHCDevice: mockDeviceController}

	// Act
	_, err := ppsSource.Timestamp()

	// Assert
	require.Error(t, err)
}

func TestGetPPSTimestampMoreThanHalfNanossecondShouldAddSecond(t *testing.T) {
	_, _, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSource := PPSSource{PHCDevice: mockDeviceController, state: PPSSet, peroutPhase: 23312}
	mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500023313), nil)

	// Act
	timestamp, err := ppsSource.Timestamp()

	// Assert
	expected := time.Unix(1075896001, 23312)
	require.NoError(t, err)
	require.EqualValues(t, expected, *timestamp)
}

func TestGetPPSTimestampLessThanHalfNanossecondShouldKeepNanosseconds(t *testing.T) {
	// Prepare
	_, _, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSource := PPSSource{PHCDevice: mockDeviceController, state: PPSSet, peroutPhase: 23312}
	mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500023312), nil)

	// Act
	timestamp, err := ppsSource.Timestamp()

	// Assert
	expected := time.Unix(1075896000, 500023312)
	require.NoError(t, err)
	require.EqualValues(t, expected, *timestamp)
}

func TestTimeToTimespec(t *testing.T) {
	someTime := time.Unix(1075896000, 500000000)
	result := timeToTimespec(someTime)
	require.Equal(t, result, unix.Timespec{Sec: 1075896000, Nsec: 500000000})
}

func TestPPSClockSyncServoLockedSuccess(t *testing.T) {
	// Prepare
	servoMock, mockTimestamper, mockDeviceController, finish := SetupMocks(t)
	defer finish()

	ppsTimestamp := time.Unix(1075896000, 100)

	gomock.InOrder(
		mockTimestamper.EXPECT().Timestamp().Return(&ppsTimestamp, nil),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 0), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateLocked),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
		mockDeviceController.EXPECT().AdjFreq(-0.1).Return(nil),
	)

	// Act
	err := PPSClockSync(servoMock, mockTimestamper, mockDeviceController)

	// Assert
	require.NoError(t, err)
}

func TestPPSClockSyncServoLockedFailure(t *testing.T) {
	// Prepare
	servoMock, mockTimestamper, mockDeviceController, finish := SetupMocks(t)
	defer finish()

	ppsTimestamp := time.Unix(1075896000, 100)
	gomock.InOrder(
		mockTimestamper.EXPECT().Timestamp().Return(&ppsTimestamp, nil),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 0), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateLocked),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
		mockDeviceController.EXPECT().AdjFreq(-0.1).Return(fmt.Errorf("error")),
	)

	// Act
	err := PPSClockSync(servoMock, mockTimestamper, mockDeviceController)

	// Assert
	require.Error(t, err)
}

func TestPPSClockSyncServoJumpSuccess(t *testing.T) {
	// Prepare
	servoMock, mockTimestamper, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsTimestamp := time.Unix(1075896000, 100)
	gomock.InOrder(
		mockTimestamper.EXPECT().Timestamp().Return(&ppsTimestamp, nil),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 0), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateJump),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
		// TODO: Improve comparison as, due to issues with typing, gomock comparison is not precise
		mockDeviceController.EXPECT().Step(time.Duration(100)).Return(nil),
	)

	// Act
	err := PPSClockSync(servoMock, mockTimestamper, mockDeviceController)

	// Assert
	require.NoError(t, err)
}

func TestPPSClockSyncServoJumpFailure(t *testing.T) {
	// Prepare
	servoMock, mockTimestamper, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsTimestamp := time.Unix(1075896000, 100)
	gomock.InOrder(
		mockTimestamper.EXPECT().Timestamp().Return(&ppsTimestamp, nil),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 0), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateJump),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
		// TODO: Improve comparison as, due to issues with typing, gomock comparison is not precise
		mockDeviceController.EXPECT().Step(gomock.Any()).Return(fmt.Errorf("error")),
	)

	// Act
	err := PPSClockSync(servoMock, mockTimestamper, mockDeviceController)

	// Assert
	require.Error(t, err)
}

func TestPPSClockSyncSrcFailure(t *testing.T) {
	// Prepare
	servoMock, mockTimestamper, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	gomock.InOrder(
		mockTimestamper.EXPECT().Timestamp().Return(nil, fmt.Errorf("error")),
	)

	// Act
	err := PPSClockSync(servoMock, mockTimestamper, mockDeviceController)

	// Assert
	require.Error(t, err)
}

func TestPPSClockSyncDstFailure(t *testing.T) {
	// Prepare
	servoMock, mockTimestamper, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsTimestamp := time.Unix(1075896000, 100)
	gomock.InOrder(
		mockTimestamper.EXPECT().Timestamp().Return(&ppsTimestamp, nil),
		mockDeviceController.EXPECT().Time().Return(time.Now(), fmt.Errorf("error")),
	)

	// Act
	err := PPSClockSync(servoMock, mockTimestamper, mockDeviceController)

	// Assert
	require.Error(t, err)
}
