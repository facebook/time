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

// Code generated by MockGen. DO NOT EDIT.
// Source: time/ptp/sptp/client/clock.go

// Package client is a generated GoMock package.
package client

import (
	reflect "reflect"
	time "time"

	gomock "go.uber.org/mock/gomock"
)

// MockClock is a mock of Clock interface.
type MockClock struct {
	ctrl     *gomock.Controller
	recorder *MockClockMockRecorder
}

// MockClockMockRecorder is the mock recorder for MockClock.
type MockClockMockRecorder struct {
	mock *MockClock
}

// NewMockClock creates a new mock instance.
func NewMockClock(ctrl *gomock.Controller) *MockClock {
	mock := &MockClock{ctrl: ctrl}
	mock.recorder = &MockClockMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClock) EXPECT() *MockClockMockRecorder {
	return m.recorder
}

// AdjFreqPPB mocks base method.
func (m *MockClock) AdjFreqPPB(freq float64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AdjFreqPPB", freq)
	ret0, _ := ret[0].(error)
	return ret0
}

// AdjFreqPPB indicates an expected call of AdjFreqPPB.
func (mr *MockClockMockRecorder) AdjFreqPPB(freq interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AdjFreqPPB", reflect.TypeOf((*MockClock)(nil).AdjFreqPPB), freq)
}

// FrequencyPPB mocks base method.
func (m *MockClock) FrequencyPPB() (float64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FrequencyPPB")
	ret0, _ := ret[0].(float64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FrequencyPPB indicates an expected call of FrequencyPPB.
func (mr *MockClockMockRecorder) FrequencyPPB() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FrequencyPPB", reflect.TypeOf((*MockClock)(nil).FrequencyPPB))
}

// MaxFreqPPB mocks base method.
func (m *MockClock) MaxFreqPPB() (float64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MaxFreqPPB")
	ret0, _ := ret[0].(float64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// MaxFreqPPB indicates an expected call of MaxFreqPPB.
func (mr *MockClockMockRecorder) MaxFreqPPB() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MaxFreqPPB", reflect.TypeOf((*MockClock)(nil).MaxFreqPPB))
}

// SetSync mocks base method.
func (m *MockClock) SetSync() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetSync")
	ret0, _ := ret[0].(error)
	return ret0
}

// SetSync indicates an expected call of SetSync.
func (mr *MockClockMockRecorder) SetSync() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetSync", reflect.TypeOf((*MockClock)(nil).SetSync))
}

// Step mocks base method.
func (m *MockClock) Step(step time.Duration) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Step", step)
	ret0, _ := ret[0].(error)
	return ret0
}

// Step indicates an expected call of Step.
func (mr *MockClockMockRecorder) Step(step interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Step", reflect.TypeOf((*MockClock)(nil).Step), step)
}
