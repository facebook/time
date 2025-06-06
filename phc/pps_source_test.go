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
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/facebook/time/hostendian"
	"github.com/facebook/time/servo"

	"github.com/facebook/time/phc/unix" // a temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type Finisher func()

func SetupMocks(t *testing.T) (servoMock *MockServoController, mockDeviceController *MockDeviceController, finish Finisher) {
	dstController := gomock.NewController(t)
	srcController := gomock.NewController(t)
	servoController := gomock.NewController(t)
	defer srcController.Finish()
	defer dstController.Finish()
	defer servoController.Finish()
	servoMock = NewMockServoController(servoController)
	mockDeviceController = NewMockDeviceController(dstController)
	return servoMock, mockDeviceController, func() {
		srcController.Finish()
		dstController.Finish()
		servoController.Finish()
	}
}

func SetupMockPoller(t *testing.T) (*MockPPSPoller, Finisher) {
	ctrl := gomock.NewController(t)
	mockPPSPoller := NewMockPPSPoller(ctrl)
	return mockPPSPoller, ctrl.Finish
}

func TestActivatePPSSource(t *testing.T) {
	// Prepare
	_, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	var actualPeroutRequest *PtpPeroutRequest
	gomock.InOrder(
		// Should set default pin to PPS
		mockDeviceController.EXPECT().setPinFunc(uint(4), unix.PTP_PF_PEROUT, uint(0)).Return(nil),
		// Should call Time once
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500000000), nil),
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(nil).Do(func(arg *PtpPeroutRequest) { actualPeroutRequest = arg }),
	)

	// Should call setPTPPerout with correct parameters
	expectedPeroutRequest := &PtpPeroutRequest{
		Index:        uint32(0),
		Flags:        uint32(2),
		StartOrPhase: PtpClockTime{Sec: 2},
		Period:       PtpClockTime{Sec: 1},
		On:           PtpClockTime{Nsec: 500000000},
	}

	// Act
	ppsSource, err := ActivatePPSSource(mockDeviceController, 4)

	// Assert
	require.NoError(t, err)
	require.EqualValues(t, expectedPeroutRequest, actualPeroutRequest, "setPTPPerout parameter mismatch")
	require.Equal(t, PPSSet, ppsSource.state)
}

func TestActivatePPSSourceIgnoreSetPinFailure(t *testing.T) {
	// Prepare
	_, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	gomock.InOrder(
		// If ioctl set pin fails, we continue bravely on...
		mockDeviceController.EXPECT().setPinFunc(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("error")),
		mockDeviceController.EXPECT().File().Return(os.NewFile(3, "mock_file")),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500000000), nil),
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(nil),
	)

	// Act
	ppsSource, err := ActivatePPSSource(mockDeviceController, 0)

	// Assert
	require.NoError(t, err)
	require.Equal(t, PPSSet, ppsSource.state)
}

func TestActivatePPSSourceSetPTPPeroutFailure(t *testing.T) {
	// Prepare
	_, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	var actualPeroutRequest *PtpPeroutRequest
	gomock.InOrder(
		mockDeviceController.EXPECT().setPinFunc(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("error")),
		mockDeviceController.EXPECT().File().Return(os.NewFile(3, "mock_file")),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500000000), nil),
		// If first attempt to set PTPPerout fails
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(fmt.Errorf("error")),
		// Should retry setPTPPerout with backward compatible flag
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(nil).Do(func(arg *PtpPeroutRequest) { actualPeroutRequest = arg }),
	)
	expectedPeroutRequest := &PtpPeroutRequest{
		Index:        uint32(0),
		Flags:        uint32(0x0),
		StartOrPhase: PtpClockTime{Sec: 2},
		Period:       PtpClockTime{Sec: 1},
		On:           PtpClockTime{Nsec: 500000000},
	}

	// Act
	ppsSource, err := ActivatePPSSource(mockDeviceController, 0)

	// Assert
	require.NoError(t, err)
	require.EqualValues(t, expectedPeroutRequest, actualPeroutRequest, "setPTPPerout parameter mismatch")
	require.Equal(t, PPSSet, ppsSource.state)
}

func TestActivatePPSSourceSetPTPPeroutDoubleFailure(t *testing.T) {
	// Prepare
	_, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	gomock.InOrder(
		mockDeviceController.EXPECT().setPinFunc(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("error")),
		mockDeviceController.EXPECT().File().Return(os.NewFile(3, "mock_file")),
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500000000), nil),
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(fmt.Errorf("error")),
		mockDeviceController.EXPECT().setPTPPerout(gomock.Any()).Return(fmt.Errorf("error")),
	)

	// Act
	ppsSource, err := ActivatePPSSource(mockDeviceController, 0)

	// Assert
	require.Error(t, err)
	require.Nil(t, ppsSource)
}

func TestGetPPSTimestampSourceUnset(t *testing.T) {
	_, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSource := PPSSource{PHCDevice: mockDeviceController}

	// Act
	_, err := ppsSource.Timestamp()

	// Assert
	require.Error(t, err)
}

func TestGetPPSTimestampMoreThanHalfSecondShouldRemoveNanosseconds(t *testing.T) {
	_, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSource := PPSSource{PHCDevice: mockDeviceController, state: PPSSet, peroutPhase: 23312}
	mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500023313), nil)

	// Act
	timestamp, err := ppsSource.Timestamp()

	// Assert
	expected := time.Unix(1075896000, 23312)
	require.NoError(t, err)
	require.EqualValues(t, expected, timestamp)
}

func TestGetPPSTimestampLessThanHalfSecondShouldRemoveNanosseconds(t *testing.T) {
	// Prepare
	_, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSource := PPSSource{PHCDevice: mockDeviceController, state: PPSSet, peroutPhase: 23312}
	mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500023312), nil)

	// Act
	timestamp, err := ppsSource.Timestamp()

	// Assert
	expected := time.Unix(1075896000, 23312)
	require.NoError(t, err)
	require.EqualValues(t, expected, timestamp)
}

func TestGetPPSTimestampUnphased(t *testing.T) {
	// Prepare
	_, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSource := PPSSource{PHCDevice: mockDeviceController, state: PPSSet}
	mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 500000000), nil)

	// Act
	timestamp, err := ppsSource.Timestamp()

	// Assert
	expected := time.Unix(1075896000, 0)
	require.NoError(t, err)
	require.EqualValues(t, expected, timestamp)
}

func TestTimeToTimespec(t *testing.T) {
	someTime := time.Unix(1075896000, 500000000)
	result, err := unix.TimeToTimespec(someTime)
	require.NoError(t, err, "TimeToTimespec")
	require.Equal(t, result, unix.Timespec{Sec: 1075896000, Nsec: 500000000})
}

func TestPPSClockSyncServoLockedSuccess(t *testing.T) {
	// Prepare
	servoMock, mockDeviceController, finish := SetupMocks(t)
	defer finish()

	ppsSourceTimestamp := time.Unix(1075896000, 100)

	gomock.InOrder(
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 23313), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateLocked),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
		mockDeviceController.EXPECT().AdjFreq(-0.1).Return(nil),
	)

	// Act
	err := PPSClockSync(servoMock, ppsSourceTimestamp, time.Unix(1075896000, 23312), mockDeviceController)

	// Assert
	require.NoError(t, err)
}

func TestPPSClockSyncServoLockedFailure(t *testing.T) {
	// Prepare
	servoMock, mockDeviceController, finish := SetupMocks(t)
	defer finish()

	ppsSourceTimestamp := time.Unix(1075896000, 100)
	gomock.InOrder(
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075894000, 23313), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateLocked),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
		mockDeviceController.EXPECT().AdjFreq(-0.1).Return(fmt.Errorf("error")),
		servoMock.EXPECT().Unlock(),
	)

	// Act
	err := PPSClockSync(servoMock, ppsSourceTimestamp, time.Unix(1075896000, 23312), mockDeviceController)

	// Assert
	require.Error(t, err)
}

func TestPPSClockSyncServoJumpSuccess(t *testing.T) {
	// Prepare
	servoMock, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSourceTimestamp := time.Unix(1075896000, 100)
	gomock.InOrder(
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075894000, 23313), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateJump),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
		// TODO: Improve comparison as, due to issues with typing, gomock comparison is not precise
		mockDeviceController.EXPECT().AdjFreq(-0.1).Return(nil),
		mockDeviceController.EXPECT().Step(time.Duration(1999999976788)).Return(nil),
	)

	// Act
	err := PPSClockSync(servoMock, ppsSourceTimestamp, time.Unix(1075894000, 23312), mockDeviceController)

	// Assert
	require.NoError(t, err)
}

func TestPPSClockSyncServoJumpFailure(t *testing.T) {
	// Prepare
	servoMock, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSourceTimestamp := time.Unix(1075896000, 100)
	gomock.InOrder(
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075894000, 23313), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateJump),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
		mockDeviceController.EXPECT().AdjFreq(gomock.Any()).Return(fmt.Errorf("error")),
		servoMock.EXPECT().Unlock(),
	)

	// Act
	err := PPSClockSync(servoMock, ppsSourceTimestamp, time.Unix(1075896000, 23312), mockDeviceController)

	// Assert
	require.Error(t, err)
}

func TestPPSClockSyncServoInit(t *testing.T) {
	// Prepare
	servoMock, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSourceTimestamp := time.Unix(1075896000, 100)
	gomock.InOrder(
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896000, 23313), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateInit),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
	)

	// Act
	err := PPSClockSync(servoMock, ppsSourceTimestamp, time.Unix(1075896000, 23312), mockDeviceController)

	// Assert
	require.NoError(t, err)
}

func TestPPSClockSyncInvalidPPSEvent(t *testing.T) {
	// Prepare
	servoMock, mockDeviceController, finish := SetupMocks(t)
	defer finish()
	ppsSourceTimestamp := time.Unix(1075896000, 100)
	gomock.InOrder(
		mockDeviceController.EXPECT().Time().Return(time.Unix(1075896002, 23313), nil),
		servoMock.EXPECT().Sample(gomock.Any(), gomock.Any()).Return(0.1, servo.StateInit),
		mockDeviceController.EXPECT().File().Return(os.NewFile(0, "test")),
	)

	// Act
	err := PPSClockSync(servoMock, ppsSourceTimestamp, time.Unix(1075896000, 23312), mockDeviceController)

	// Assert
	require.Error(t, err)
}

func TestNewPiServo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockFrequencyGetter := NewMockFrequencyGetter(ctrl)
	gomock.InOrder(
		mockFrequencyGetter.EXPECT().FreqPPB().Return(1.0, nil),
		mockFrequencyGetter.EXPECT().MaxFreqAdjPPB().Return(3.0, nil),
	)

	servo, err := NewPiServo(time.Duration(1), time.Duration(1), time.Duration(0), mockFrequencyGetter, 0.0)

	require.NoError(t, err)
	require.Equal(t, int64(1), servo.Servo.FirstStepThreshold)
	require.Equal(t, true, servo.Servo.FirstUpdate)
	require.Equal(t, -1.0, servo.MeanFreq())
	require.Equal(t, "INIT", servo.GetState().String())
	require.Equal(t, 3.0, servo.GetMaxFreq())
}

func TestNewPiServoFreqPPBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockFrequencyGetter := NewMockFrequencyGetter(ctrl)
	gomock.InOrder(
		mockFrequencyGetter.EXPECT().FreqPPB().Return(1.0, fmt.Errorf("error")),
	)

	_, err := NewPiServo(time.Duration(1), time.Duration(1), time.Duration(0), mockFrequencyGetter, 0.0)

	require.Error(t, err)
}

func TestNewPiServoMaxFreqError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockFrequencyGetter := NewMockFrequencyGetter(ctrl)
	gomock.InOrder(
		mockFrequencyGetter.EXPECT().FreqPPB().Return(1.0, nil),
		mockFrequencyGetter.EXPECT().MaxFreqAdjPPB().Return(12345.0, fmt.Errorf("error")),
	)

	servo, err := NewPiServo(time.Duration(1), time.Duration(1), time.Duration(0), mockFrequencyGetter, 0.0)

	require.NoError(t, err)
	require.Equal(t, int64(1), servo.Servo.FirstStepThreshold)
	require.Equal(t, true, servo.Servo.FirstUpdate)
	require.Equal(t, -1.0, servo.MeanFreq())
	require.Equal(t, "INIT", servo.GetState().String())
	require.Equal(t, 500000.0, servo.GetMaxFreq())
}

func TestNewPiServoUseMaxFreq(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockFrequencyGetter := NewMockFrequencyGetter(ctrl)
	gomock.InOrder(
		mockFrequencyGetter.EXPECT().FreqPPB().Return(1.0, nil),
	)

	servo, err := NewPiServo(time.Duration(1), time.Duration(1), time.Duration(0), mockFrequencyGetter, 2.0)

	require.NoError(t, err)
	require.Equal(t, int64(1), servo.Servo.FirstStepThreshold)
	require.Equal(t, true, servo.Servo.FirstUpdate)
	require.Equal(t, -1.0, servo.MeanFreq())
	require.Equal(t, "INIT", servo.GetState().String())
	require.Equal(t, 2.0, servo.GetMaxFreq())
}

func TestNewPiServoStepth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockFrequencyGetter := NewMockFrequencyGetter(ctrl)
	gomock.InOrder(
		mockFrequencyGetter.EXPECT().FreqPPB().Return(1.0, nil),
	)

	servo, err := NewPiServo(time.Duration(1), time.Duration(1), time.Duration(10), mockFrequencyGetter, 2.0)

	require.NoError(t, err)
	require.Equal(t, int64(1), servo.Servo.FirstStepThreshold)
	require.Equal(t, true, servo.Servo.FirstUpdate)
	require.Equal(t, -1.0, servo.MeanFreq())
	require.Equal(t, "INIT", servo.GetState().String())
	require.Equal(t, 2.0, servo.GetMaxFreq())
	require.Equal(t, int64(10), servo.Servo.StepThreshold)
}

func TestNewPiServoNoFirstStep(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockFrequencyGetter := NewMockFrequencyGetter(ctrl)
	gomock.InOrder(
		mockFrequencyGetter.EXPECT().FreqPPB().Return(1.0, nil),
	)

	servo, err := NewPiServo(time.Duration(1), time.Duration(0), time.Duration(0), mockFrequencyGetter, 2.0)

	require.NoError(t, err)
	require.Equal(t, false, servo.Servo.FirstUpdate)
	require.Equal(t, -1.0, servo.MeanFreq())
	require.Equal(t, "INIT", servo.GetState().String())
	require.Equal(t, 2.0, servo.GetMaxFreq())
}

func TestPPSSink_getPPSEventTimestamp(t *testing.T) {
	// Create a mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a mock DeviceController
	mockDevice := NewMockDeviceController(ctrl)

	// Create a PPSSink instance
	ppsSink := &PPSSink{
		Device:   mockDevice,
		InputPin: 1,
	}

	// Test cases
	t.Run("successful read", func(t *testing.T) {
		// Prepare
		event := PtpExttsEvent{Index: 1, T: PtpClockTime{Sec: 1}}

		mockDevice.EXPECT().Read(gomock.Any()).Return(1, nil).Do(func(buf []byte) {
			var intBuffer bytes.Buffer
			err := binary.Write(&intBuffer, hostendian.Order, &event)
			require.NoError(t, err)
			copy(buf, intBuffer.Bytes())
			fmt.Print(buf)
		})

		// Act
		timestamp, err := ppsSink.getPPSEventTimestamp()

		// Assert
		require.NoError(t, err)
		require.Equal(t, timestamp, time.Unix(1, 0))
	})

	t.Run("read error", func(t *testing.T) {
		// Prepare
		mockDevice.EXPECT().Read(gomock.Any()).Return(0, fmt.Errorf("read error"))
		mockDevice.EXPECT().File().Return(os.NewFile(0, "test"))

		// Act
		timestamp, err := ppsSink.getPPSEventTimestamp()

		// Assert
		require.Error(t, err)
		require.Zero(t, timestamp)
	})

	t.Run("unexpected channel", func(t *testing.T) {
		// Prepare
		event := PtpExttsEvent{Index: 2, T: PtpClockTime{Sec: 1}}

		mockDevice.EXPECT().Read(gomock.Any()).Return(1, nil).Do(func(buf []byte) {
			var intBuffer bytes.Buffer
			err := binary.Write(&intBuffer, hostendian.Order, &event)
			require.NoError(t, err)
			copy(buf, intBuffer.Bytes())
		})

		// Act
		timestamp, err := ppsSink.getPPSEventTimestamp()

		// Assert
		require.Error(t, err)
		require.Zero(t, timestamp)
	})
}
