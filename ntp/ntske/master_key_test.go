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
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// hexKeyFile writes key hex-encoded (as the Keychain secret is stored) and returns
// the path.
func hexKeyFile(t *testing.T, key []byte) string {
	t.Helper()
	return writeKeyFile(t, []byte(hex.EncodeToString(key)))
}

func writeKeyFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "master_key")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// a hex-encoded key file decodes to the raw key.
func TestLoadMasterKeyFromFileValid(t *testing.T) {
	want := bytes.Repeat([]byte{0x2a}, masterKeyLen)
	got, err := LoadMasterKeyFromFile(hexKeyFile(t, want))
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// surrounding whitespace (e.g. a trailing newline from secrets_tool) is trimmed
// before decoding.
func TestLoadMasterKeyFromFileTrimsWhitespace(t *testing.T) {
	want := bytes.Repeat([]byte{0x2a}, masterKeyLen)
	got, err := LoadMasterKeyFromFile(writeKeyFile(t, []byte(hex.EncodeToString(want)+"\n")))
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// non-hex contents are rejected (fail closed).
func TestLoadMasterKeyFromFileInvalidHex(t *testing.T) {
	_, err := LoadMasterKeyFromFile(writeKeyFile(t, []byte("nothexZZ")))
	require.Error(t, err)
}

// a missing file is a distinct not-exist error.
func TestLoadMasterKeyFromFileMissing(t *testing.T) {
	_, err := LoadMasterKeyFromFile(filepath.Join(t.TempDir(), "nope"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// an under-length key (after decoding) is rejected (fail closed).
func TestLoadMasterKeyFromFileTooShort(t *testing.T) {
	_, err := LoadMasterKeyFromFile(hexKeyFile(t, bytes.Repeat([]byte{1}, masterKeyMinLength-1)))
	require.ErrorIs(t, err, ErrMasterKeyTooShort)
}

// an unreadable file is a distinct permission error.
func TestLoadMasterKeyFromFilePermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	path := writeKeyFile(t, bytes.Repeat([]byte{0x2a}, masterKeyLen))
	require.NoError(t, os.Chmod(path, 0o000))
	_, err := LoadMasterKeyFromFile(path)
	require.ErrorIs(t, err, os.ErrPermission)
}

// fingerprint is deterministic, key-dependent, and never contains the raw key.
func TestMasterKeyFingerprint(t *testing.T) {
	master := bytes.Repeat([]byte{0x2a}, masterKeyLen)
	fp := MasterKeyFingerprint(master)
	require.Equal(t, fp, MasterKeyFingerprint(master))
	require.NotEqual(t, fp, MasterKeyFingerprint(bytes.Repeat([]byte{0x2b}, masterKeyLen)))
	require.NotContains(t, fp, string(master))
}

// no path selects the in-memory dev/test keystore.
func TestNewKeystoreInMemoryFallback(t *testing.T) {
	ks, err := NewKeystore(KeystoreConfig{})
	require.NoError(t, err)
	_, ok := ks.(*InMemoryKeystore)
	require.True(t, ok, "expected *InMemoryKeystore, got %T", ks)
}

// a master-key path selects the DerivedKeystore.
func TestNewKeystoreDerived(t *testing.T) {
	path := hexKeyFile(t, bytes.Repeat([]byte{0x2a}, masterKeyLen))
	ks, err := NewKeystore(KeystoreConfig{MasterKeyPath: path})
	require.NoError(t, err)
	_, ok := ks.(*DerivedKeystore)
	require.True(t, ok, "expected *DerivedKeystore, got %T", ks)
}

// a bad master-key path fails closed (no silent fallback).
func TestNewKeystoreBadPath(t *testing.T) {
	_, err := NewKeystore(KeystoreConfig{MasterKeyPath: filepath.Join(t.TempDir(), "nope")})
	require.ErrorIs(t, err, os.ErrNotExist)
}
