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

/*
Package hostendian provides way to check the endianness of the
machine this code is running on.

While it's not needed most of the time, but software sometimes will combine
BigEndian and Host Endian (which typically is LittleEndian, but it's not guaranteed)
data in one structure sent over unix socket, and we need to work with it regardless.
*/
package hostendian

import (
	"encoding/binary"
	"unsafe"
)

// Order of the bytes
var Order binary.ByteOrder = binary.LittleEndian

// IsBigEndian is a flag determining if value is in Big Endian
var IsBigEndian bool

func init() {
	var i uint16 = 0x0100
	ptr := unsafe.Pointer(&i)
	if *(*byte)(ptr) == 0x01 {
		// we are on the big endian machine
		IsBigEndian = true
		Order = binary.BigEndian
	}
}
