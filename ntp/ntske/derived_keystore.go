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

package ntske

import (
	"bytes"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/facebook/time/ntp/protocol"
)

// Cookie sealing key: K_id = HKDF-SHA256(master, salt=BE32(id), info=label).
const (
	cookieSealInfoLabel        = "fbnts-cookie-seal-v1" // HKDF info; domain separation
	masterKeyMinLength         = 32                     // security rests on master entropy
	cookieKeyID         uint32 = 0                      // fixed Key ID; rotation/windowing deferred
)

// ErrMasterKeyTooShort is returned when the master is under masterKeyMinLength octets.
var ErrMasterKeyTooShort = errors.New("ntske: master key too short")

// deriveCookieKey derives the masterKeyLen-octet sealing key for key id. Pure
// (no clock/state/I/O) so any host reconstructs it from (master, id) alone.
func deriveCookieKey(master []byte, id uint32) ([]byte, error) {
	if len(master) < masterKeyMinLength {
		return nil, fmt.Errorf("%w: got %d octets, need >= %d",
			ErrMasterKeyTooShort, len(master), masterKeyMinLength)
	}
	salt := binary.BigEndian.AppendUint32(nil, id)
	key, err := hkdf.Key(sha256.New, master, salt, cookieSealInfoLabel, masterKeyLen)
	if err != nil {
		return nil, fmt.Errorf("ntske: derive cookie key: %w", err)
	}
	return key, nil
}

// DerivedKeystore seals and opens cookies with keys derived on demand from one
// immutable master, so a cookie sealed on one host opens on another.
type DerivedKeystore struct {
	master []byte
}

var _ Keystore = (*DerivedKeystore)(nil)

type DerivedKeystoreOptions struct {
	Master []byte // >= masterKeyMinLength octets
}

// NewDerivedKeystore validates the master and returns a keystore ready to seal
// and open cookies. The master is cloned; it is never mutated afterwards.
func NewDerivedKeystore(opts DerivedKeystoreOptions) (*DerivedKeystore, error) {
	if len(opts.Master) < masterKeyMinLength {
		return nil, fmt.Errorf("%w: got %d octets, need >= %d",
			ErrMasterKeyTooShort, len(opts.Master), masterKeyMinLength)
	}
	return &DerivedKeystore{master: bytes.Clone(opts.Master)}, nil
}

// SealCookie derives the fixed-id sealing key and seals c2s || s2c via the shared
// envelope (see sealEnvelope). Nonce randomness comes from crypto/rand.
func (ks *DerivedKeystore) SealCookie(aeadID protocol.AEADAlgorithm, c2s, s2c []byte) ([]byte, error) {
	sealingKey, err := deriveCookieKey(ks.master, cookieKeyID)
	if err != nil {
		return nil, err
	}
	return sealEnvelope(rand.Reader, cookieKeyID, sealingKey, aeadID, c2s, s2c)
}

// OpenCookie re-derives the sealing key from the cookie's Key ID and opens it via
// the shared envelope (see openEnvelope).
func (ks *DerivedKeystore) OpenCookie(cookie []byte) (protocol.AEADAlgorithm, []byte, []byte, error) {
	return openEnvelope(cookie, func(id uint32) ([]byte, error) {
		return deriveCookieKey(ks.master, id)
	})
}
