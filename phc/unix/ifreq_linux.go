//go:build linux

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

package unix

import (
	"unsafe"
)

// An Ifreq is a type-safe wrapper around the raw ifreq struct. An Ifreq
// contains an interface name and a union of arbitrary data which can be
// accessed using the Ifreq's methods. To create an Ifreq, use the NewIfreq
// function.
type Ifreq struct{ raw ifreq }

// NewIfreq creates an Ifreq with the input network interface name
func NewIfreq(name string) (*Ifreq, error) {
	if len(name) >= IFNAMSIZ {
		return nil, EINVAL
	}

	var ifr ifreq
	copy(ifr.Ifrn[:], name)

	return &Ifreq{raw: ifr}, nil
}

// Name returns the interface name associated with the Ifreq.
func (ifr *Ifreq) Name() string {
	return ByteSliceToString(ifr.raw.Ifrn[:])
}

// An ifreqData is an Ifreq which carries pointer data.
// To produce an ifreqData, use the Ifreq.withData method.
type ifreqData struct {
	name [IFNAMSIZ]byte
	// A type separate from ifreq is required in order to comply with the
	// unsafe.Pointer rules since the "pointer-ness" of data would not be
	// preserved if it were cast into the byte array of a raw ifreq.
	data unsafe.Pointer
	// Pad to the same size as ifreq.
	_ [len(ifreq{}.Ifru) - SizeofPtr]byte
}

// withData produces an ifreqData with the pointer p set for ioctls which require
// arbitrary pointer data.
func (ifr Ifreq) withData(p unsafe.Pointer) ifreqData {
	return ifreqData{
		name: ifr.raw.Ifrn,
		data: p,
	}
}
