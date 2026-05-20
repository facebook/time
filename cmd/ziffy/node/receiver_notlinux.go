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
)

type Receiver struct {
	Config *Config
}

type Sender struct {
	Config *Config
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
