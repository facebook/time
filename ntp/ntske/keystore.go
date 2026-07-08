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
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/facebook/time/ntp/protocol"
	"github.com/facebook/time/ntp/protocol/nts"
)

/*
Cookie keystore (RFC 8915 §6).
An NTS cookie is server-opaque state: the client stores it verbatim and echoes
it back, and only the server can open it. It carries the two session keys
(C2S and S2C) negotiated during the NTS-KE handshake so the server does not
have to keep per-client state.
Wire format (chrony-compatible; see chrony's nts_ke_server.c):
	 0                   1                   2                   3
	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+---------------------------------------------------------------+
	|                       Key ID (4 octets)                       |
	+---------------------------------------------------------------+
	|                                                               |
	~                        Nonce (16 octets)                      ~
	|                                                               |
	+---------------------------------------------------------------+
	|                                                               |
	~          AES-SIV ciphertext  =  tag(16) ‖ (C2S ‖ S2C)         ~
	|                                                               |
	+---------------------------------------------------------------+
- Key ID: big-endian identifier of the master key that sealed this cookie.
  It is carried in the clear so the server can pick the right key on open; it
  is not authenticated as associated data, but tampering with it selects a
  different (or unknown) master key and thus fails the AES-SIV tag check.
- Nonce: 16 random octets. The cookie is always sealed with AES-SIV-CMAC-512
  (RFC 5297), a deterministic AEAD. To keep cookies unlinkable across issuance
  we fold this random nonce into the SIV as associated data, so identical key
  material produces distinct cookies. This matches chrony, which passes the
  nonce as the SIV nonce component.
- Ciphertext: tink's AES-SIV output — the 16-octet synthetic IV (tag) followed
  by the encrypted C2S ‖ S2C key material.
The *session* AEAD algorithm (the one the client will use for NTP, negotiated
via NTS-KE) is NOT stored in the cookie. It is inferred from the total cookie
length, because the C2S/S2C key length is fixed per algorithm:
	 68 octets  ->  AEAD_AES_128_GCM_SIV  (id 30, 16-octet session keys)
	164 octets  ->  AEAD_AES_SIV_CMAC_512 (id 17, 64-octet session keys)
Note the distinction between the two AES-SIV-CMAC-512 uses here: the *keystore*
always seals cookies with it (a fixed 64-octet master key), independent of
whichever AEAD the *session* negotiated.
*/

const (
	// masterAEADID is the algorithm the keystore uses to seal every cookie,
	// regardless of the negotiated session algorithm. AES-SIV-CMAC-512 is a
	// deterministic AEAD whose 16-octet synthetic IV doubles as the auth tag.
	masterAEADID = protocol.AEADAESSIVCMAC512
)
const (
	cookieKeyIDLen = 4  // big-endian master-key identifier
	cookieNonceLen = 16 // random per-cookie nonce, mixed into the SIV as AD
	sivTagLen      = 16 // AES-SIV synthetic IV prepended to the ciphertext
	masterKeyLen   = 64 // AES-SIV-CMAC-512 master key
	// cookieOverhead is everything in a cookie that is not session key material:
	// Key ID + Nonce + SIV tag.
	cookieOverhead = cookieKeyIDLen + cookieNonceLen + sivTagLen
	// defaultMaxKeys is the master-key ring capacity: the current key plus this
	// many previous keys are retained so cookies issued just before a rotation
	// remain openable until they age out.
	defaultMaxKeys uint32 = 3
)

// Sentinel errors. Compare with errors.Is. As in the nts package, the
// underlying tink verification error is surfaced as text on ErrCookieVerify but
// deliberately kept out of the errors.Is chain to insulate callers from tink
// internals.
var (
	ErrCookieTooShort       = errors.New("ntske: cookie too short")
	ErrCookieMalformed      = errors.New("ntske: cookie malformed")
	ErrUnknownKeyID         = errors.New("ntske: unknown cookie key id")
	ErrUnsupportedAlgorithm = errors.New("ntske: unsupported aead algorithm")
	ErrKeyLength            = errors.New("ntske: session key length mismatch")
	ErrCookieVerify         = errors.New("ntske: cookie verification failed")
)

// Keystore seals session keys into opaque cookies and opens them again. A
// single Keystore is shared across all NTS-KE and NTP request handlers, so
// implementations must be safe for concurrent use.
type Keystore interface {
	// SealCookie encrypts the C2S and S2C session keys for the negotiated
	// aeadID into a fresh cookie.
	SealCookie(aeadID protocol.AEADAlgorithm, c2s, s2c []byte) ([]byte, error)
	// OpenCookie decrypts a cookie, returning the negotiated session aeadID and
	// the C2S and S2C keys. The returned key slices are freshly allocated.
	OpenCookie(cookie []byte) (aeadID protocol.AEADAlgorithm, c2s, s2c []byte, err error)
}

type InMemoryKeystore struct {
	mu      sync.RWMutex
	ring    map[uint32][]byte // Key ID -> 64-octet master key
	order   []uint32          // Key ID order, older first
	current uint32            // Key ID used for sealing
	nextID  uint32            // Next key ID to use
	maxKeys uint32            // ring capacity (current + maxKeys-1 previous keys)
	rand    io.Reader         // nonce and master-key source; crypto/rand by default
}

// check that InMemoryKeystore implements the Keystore interface
var _ Keystore = (*InMemoryKeystore)(nil)

type InMemoryKeystoreOptions struct {
	MaxKeys uint32
	Rand    io.Reader
}

// NewInMemoryKeystore returns a keystore seeded with one freshly generated
// master key, ready to seal and open cookies.
func NewInMemoryKeystore(opts InMemoryKeystoreOptions) (*InMemoryKeystore, error) {
	ks := &InMemoryKeystore{
		ring:    make(map[uint32][]byte),
		maxKeys: defaultMaxKeys,
		rand:    rand.Reader,
	}

	if opts.MaxKeys != 0 {
		ks.maxKeys = opts.MaxKeys
	}
	if opts.Rand != nil {
		ks.rand = opts.Rand
	}
	if err := ks.Rotate(); err != nil {
		return nil, err
	}
	return ks, nil
}

// Rotate generates a new master key, makes it the sealing key, and ages out the
// oldest key if the ring is over capacity. Cookies sealed with a still-present
// key remain openable.
func (ks *InMemoryKeystore) Rotate() error {
	// generate 64 byte random master key
	key := make([]byte, masterKeyLen)
	if _, err := io.ReadFull(ks.rand, key); err != nil {
		return fmt.Errorf("ntske: generate master key: %w", err)
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	// assign incremental ID
	ks.nextID++
	id := ks.nextID
	ks.ring[id] = key
	ks.order = append(ks.order, id)
	ks.current = id
	// check maxKey capacity and eventually delete older ones
	for len(ks.order) > int(ks.maxKeys) {
		oldest := ks.order[0]
		ks.order = ks.order[1:]
		delete(ks.ring, oldest)
	}
	return nil
}

// SealCookie encrypts c2s ‖ s2c under the current master key. c2s and s2c must
// both have the key length required by aeadID.
func (ks *InMemoryKeystore) SealCookie(aeadID protocol.AEADAlgorithm, c2s, s2c []byte) ([]byte, error) {
	// validate aeadID and keyLen
	keyLen, err := aeadIDToKeyLen(aeadID)
	if err != nil {
		return nil, err
	}
	if len(c2s) != keyLen || len(s2c) != keyLen {
		return nil, fmt.Errorf("%w: aead=%d wants %d-octet keys, got c2s=%d s2c=%d",
			ErrKeyLength, aeadID, keyLen, len(c2s), len(s2c))
	}
	nonce := make([]byte, cookieNonceLen)
	if _, err := io.ReadFull(ks.rand, nonce); err != nil {
		return nil, fmt.Errorf("ntske: read nonce: %w", err)
	}
	ks.mu.RLock()
	keyID := ks.current
	master := ks.ring[keyID]
	ks.mu.RUnlock()
	aead, err := nts.NewAEAD(masterAEADID, master)
	if err != nil {
		return nil, fmt.Errorf("ntske: master aead: %w", err)
	}
	plaintext := make([]byte, 0, 2*keyLen)
	plaintext = append(plaintext, c2s...)
	plaintext = append(plaintext, s2c...)

	_, ct, err := aead.Seal(nonce, plaintext)
	if err != nil {
		return nil, fmt.Errorf("ntske: seal cookie: %w", err)
	}
	out := make([]byte, cookieKeyIDLen+cookieNonceLen+len(ct))
	binary.BigEndian.PutUint32(out[0:cookieKeyIDLen], keyID)
	copy(out[cookieKeyIDLen:cookieKeyIDLen+cookieNonceLen], nonce)
	copy(out[cookieKeyIDLen+cookieNonceLen:], ct)
	return out, nil
}

// OpenCookie decrypts a cookie and returns the negotiated session aeadID with
// the C2S and S2C keys.
func (ks *InMemoryKeystore) OpenCookie(cookie []byte) (protocol.AEADAlgorithm, []byte, []byte, error) {
	aeadID, err := CookieAEADID(cookie)
	if err != nil {
		return 0, nil, nil, err
	}
	keyLen, err := aeadIDToKeyLen(aeadID)
	if err != nil {
		return 0, nil, nil, err
	}
	keyID := binary.BigEndian.Uint32(cookie[0:cookieKeyIDLen])
	nonce := cookie[cookieKeyIDLen : cookieKeyIDLen+cookieNonceLen]
	ct := cookie[cookieKeyIDLen+cookieNonceLen:]
	ks.mu.RLock()
	master, ok := ks.ring[keyID]
	ks.mu.RUnlock()
	if !ok {
		return 0, nil, nil, fmt.Errorf("%w: keyID=%d", ErrUnknownKeyID, keyID)
	}
	aead, err := nts.NewAEAD(masterAEADID, master)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("ntske: master aead: %w", err)
	}
	pt, err := aead.Open(nonce, nil, ct)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("%w: %s", ErrCookieVerify, err.Error())
	}
	if len(pt) != 2*keyLen {
		return 0, nil, nil, fmt.Errorf("%w: decrypted %d octets, want %d",
			ErrCookieMalformed, len(pt), 2*keyLen)
	}
	c2s := bytes.Clone(pt[:keyLen])
	s2c := bytes.Clone(pt[keyLen:])
	return aeadID, c2s, s2c, nil
}

// CookieAEADID reports the negotiated session AEAD algorithm ID encoded by a
// cookie's total length, without decrypting it.
func CookieAEADID(cookie []byte) (protocol.AEADAlgorithm, error) {
	if len(cookie) < cookieOverhead {
		return 0, fmt.Errorf("%w: len=%d, need at least %d", ErrCookieTooShort, len(cookie), cookieOverhead)
	}
	ptLen := len(cookie) - cookieOverhead
	if ptLen == 0 || ptLen%2 != 0 {
		return 0, fmt.Errorf("%w: key material length %d is not a positive even number",
			ErrCookieMalformed, ptLen)
	}
	return keyLenToAEADID(ptLen / 2)
}

// keyLenToAEADID maps a per-direction session key length to its AEAD algorithm ID.
func keyLenToAEADID(keyLen int) (protocol.AEADAlgorithm, error) {
	switch keyLen {
	case 16:
		return protocol.AEADAES128GCMSIV, nil
	case 64:
		return protocol.AEADAESSIVCMAC512, nil
	default:
		return 0, fmt.Errorf("%w: %d-octet session key", ErrUnsupportedAlgorithm, keyLen)
	}
}

// aeadIDToKeyLen maps an AEAD algorithm ID to its per-direction session key length.
func aeadIDToKeyLen(aeadID protocol.AEADAlgorithm) (int, error) {
	switch aeadID {
	case protocol.AEADAES128GCMSIV:
		return 16, nil
	case protocol.AEADAESSIVCMAC512:
		return 64, nil
	default:
		return 0, fmt.Errorf("%w: aead id %d", ErrUnsupportedAlgorithm, aeadID)
	}
}
