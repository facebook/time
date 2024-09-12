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

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type deviceControllerMock struct {
	mock.Mock
}

func (_m *deviceControllerMock) Time() (time.Time, error) {
	ret := _m.Called()
	return ret.Get(0).(time.Time), ret.Error(1)
}

func (_m *deviceControllerMock) setPinFunc(index uint, pf PinFunc, ch uint) error {
	ret := _m.Called(index, pf, ch)
	return ret.Error(0)
}

func (_m *deviceControllerMock) setPTPPerout(req PTPPeroutRequest) error {
	ret := _m.Called(req)
	return ret.Error(0)
}

func (_m *deviceControllerMock) File() *os.File {
	ret := _m.Called()
	return ret.Get(0).(*os.File)
}

func TestIfaceInfoToPHCDevice(t *testing.T) {
	info := &EthtoolTSinfo{
		PHCIndex: 0,
	}
	got, err := ifaceInfoToPHCDevice(info)
	require.NoError(t, err)
	require.Equal(t, "/dev/ptp0", got)

	info.PHCIndex = 23
	got, err = ifaceInfoToPHCDevice(info)
	require.NoError(t, err)
	require.Equal(t, "/dev/ptp23", got)

	info.PHCIndex = -1
	_, err = ifaceInfoToPHCDevice(info)
	require.Error(t, err)
}

func TestMaxAdjFreq(t *testing.T) {
	caps := &PTPClockCaps{
		MaxAdj: 1000000000,
	}

	got := caps.maxAdj()
	require.InEpsilon(t, 1000000000.0, got, 0.00001)

	caps.MaxAdj = 0
	got = caps.maxAdj()
	require.InEpsilon(t, 500000.0, got, 0.00001)
}

func TestActivatePPSSource(t *testing.T) {
	// Prepare
	mockDevice := new(deviceControllerMock)

	// Should set default pin to PPS
	mockDevice.On("setPinFunc", uint(0), PinFuncPerOut, uint(0)).Return(nil).Once()

	// Should call Time once
	mockDevice.On("Time").Return(time.Unix(824635825488, 1397965136), nil).Once()

	// Should issue ioctlPTPPeroutRequest2
	expectedPeroutRequest := PTPPeroutRequest{
		Index:        uint32(0),
		Flags:        uint32(0x2),
		StartOrPhase: PTPClockTime{Sec: 51},
		Period:       PTPClockTime{Sec: 1},
		On:           PTPClockTime{NSec: 500000000},
	}
	mockDevice.On("setPTPPerout", expectedPeroutRequest).Return(nil).Once()

	// Act
	err := ActivatePPSSource(mockDevice)

	// Assert calls
	require.NoError(t, err)
	mockDevice.AssertExpectations(t)
}

func TestActivatePPSSourceIgnoreSetPinFailure(t *testing.T) {
	// Prepare
	mockDevice := new(deviceControllerMock)
	mockDevice.On("File").Return(os.NewFile(3, "mock_file"))
	mockDevice.On("Time").Return(time.Unix(824635825488, 1397965136), nil)

	// If ioctl set pin fails, we continue bravely on...
	mockDevice.On("setPinFunc", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("error")).Once()
	mockDevice.On("setPTPPerout", mock.Anything).Return(nil).Once()

	// Act
	err := ActivatePPSSource(mockDevice)

	// Assert calls
	mockDevice.AssertExpectations(t)
	require.NoError(t, err)
}

func TestActivatePPSSourceSetPTPPeroutFailure(t *testing.T) {
	// Prepare
	mockDevice := new(deviceControllerMock)
	mockDevice.On("File").Return(os.NewFile(3, "mock_file"))
	mockDevice.On("Time").Return(time.Unix(824635825488, 1397965136), nil)

	mockDevice.On("setPinFunc", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("error")).Once()
	// If first attempt to set PTPPerout fails
	mockDevice.On("setPTPPerout", mock.Anything).Return(fmt.Errorf("error")).Once()

	// Should retry setPTPPerout with backward compatible flag
	expectedPeroutRequest := PTPPeroutRequest{
		Index:        uint32(0),
		Flags:        uint32(0x0),
		StartOrPhase: PTPClockTime{Sec: 51},
		Period:       PTPClockTime{Sec: 1},
		On:           PTPClockTime{NSec: 500000000},
	}
	mockDevice.On("setPTPPerout", expectedPeroutRequest).Return(nil).Once()

	// Act
	err := ActivatePPSSource(mockDevice)

	// Assert
	mockDevice.AssertExpectations(t)
	require.NoError(t, err)
}

func TestActivatePPSSourceSetPTPPeroutDoubleFailure(t *testing.T) {
	// Prepare
	mockDevice := new(deviceControllerMock)
	mockDevice.On("File").Return(os.NewFile(3, "mock_file"))
	mockDevice.On("Time").Return(time.Unix(824635825488, 1397965136), nil)
	mockDevice.On("setPinFunc", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("error")).Once()
	mockDevice.On("setPTPPerout", mock.Anything).Return(fmt.Errorf("error")).Once()
	mockDevice.On("setPTPPerout", mock.Anything).Return(fmt.Errorf("error")).Once()

	// Act
	err := ActivatePPSSource(mockDevice)

	// Assert
	mockDevice.AssertExpectations(t)
	require.Error(t, err)
}
