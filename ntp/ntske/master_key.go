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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// LoadMasterKeyFromFile reads the hex-encoded cookie master key and decodes it to
// raw bytes. Surrounding whitespace (e.g. a trailing newline from secrets_tool) is
// trimmed before decoding. A read error, malformed hex, or under-length key is
// returned (fail closed) so a bad key never starts the server. KE and NTP decode
// identically, so they agree on the raw key.
func LoadMasterKeyFromFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ntske: read master key %q: %w", path, err)
	}
	master, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("ntske: decode hex master key %q: %w", path, err)
	}
	if len(master) < masterKeyMinLength {
		return nil, fmt.Errorf("%w: file %q decodes to %d octets, need >= %d",
			ErrMasterKeyTooShort, path, len(master), masterKeyMinLength)
	}
	return master, nil
}

// MasterKeyFingerprint returns a non-secret len + SHA-256[:4] tag, safe to log,
// so an operator can confirm KE and NTP hold the same master.
func MasterKeyFingerprint(master []byte) string {
	sum := sha256.Sum256(master)
	return fmt.Sprintf("len=%d sha256[:4]=%x", len(master), sum[:4])
}

// KeystoreConfig selects the cookie keystore; both server binaries build through
// NewKeystore so their choice cannot diverge.
type KeystoreConfig struct {
	MasterKeyPath string // set => fleet-wide DerivedKeystore; empty => dev/test InMemoryKeystore
	MaxKeys       uint32 // InMemoryKeystore ring size (dev/test path only)
}

// NewKeystore returns a DerivedKeystore when cfg.MasterKeyPath is set (fail-closed
// on a bad key), else a dev/test InMemoryKeystore seeded with SharedTestMasterKey.
// It logs the non-secret master fingerprint so KE and NTP can be compared.
func NewKeystore(cfg KeystoreConfig) (Keystore, error) {
	if cfg.MasterKeyPath == "" {
		slog.Warn("ntske: no master key path set; using in-memory dev/test keystore (cookies are not fleet-portable)")
		return NewInMemoryKeystore(InMemoryKeystoreOptions{
			MaxKeys:    cfg.MaxKeys,
			InitialKey: SharedTestMasterKey,
		})
	}
	master, err := LoadMasterKeyFromFile(cfg.MasterKeyPath)
	if err != nil {
		return nil, err
	}
	slog.Info("ntske: loaded cookie master key",
		"path", cfg.MasterKeyPath, "fingerprint", MasterKeyFingerprint(master))
	return NewDerivedKeystore(DerivedKeystoreOptions{Master: master})
}
