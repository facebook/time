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

package leaphash

import (
	"testing"
)

// TestHashShouldMatch verifies that the hash value computed from testDoc
// matches the hash value within testDoc
func TestHashShouldMatch(t *testing.T) {
	hash := Compute(testDoc)
	expected := "44dcf58c e28d25aa b36612c8 f3d3e8b5 a8fdf478"
	if hash != expected {
		t.Fatalf("invalid hash value, got '%s', expected '%s'", hash, expected)
	}
}

func FuzzCompute(f *testing.F) {
	f.Fuzz(func(t *testing.T, input string) {
		_ = Compute(input)
	})
}
