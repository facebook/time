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
	"strings"
	"testing"
)

// testDocHash is the hash testDoc carries on its own "#h" line
const testDocHash = "44dcf58c e28d25aa b36612c8 f3d3e8b5 a8fdf478"

// leapSecondLines is how many leap second entries testDoc holds
const leapSecondLines = 28

// TestHashShouldMatch verifies that the hash value computed from testDoc
// matches the hash value within testDoc
func TestHashShouldMatch(t *testing.T) {
	hash := Compute(testDoc)
	if hash != testDocHash {
		t.Fatalf("invalid hash value, got '%s', expected '%s'", hash, testDocHash)
	}
}

// TestHashIgnoresCommentSpacing pins the document's promise that the hash ignores comments and whitespace.
func TestHashIgnoresCommentSpacing(t *testing.T) {
	tight := strings.ReplaceAll(testDoc, "\t# ", "# ")
	if n := strings.Count(testDoc, "\t# "); n != leapSecondLines {
		t.Fatalf("tightened %d lines, expected the %d leap second lines", n, leapSecondLines)
	}
	if hash := Compute(tight); hash != testDocHash {
		t.Fatalf("invalid hash value, got '%s', expected '%s'", hash, testDocHash)
	}
}

// TestHashIgnoresCarriageReturns pins the same promise for a CRLF copy of the document.
func TestHashIgnoresCarriageReturns(t *testing.T) {
	crlf := strings.ReplaceAll(testDoc, "\n", "\r\n")
	if hash := Compute(crlf); hash != testDocHash {
		t.Fatalf("invalid hash value, got '%s', expected '%s'", hash, testDocHash)
	}
}

func FuzzCompute(f *testing.F) {
	f.Fuzz(func(t *testing.T, input string) {
		_ = Compute(input)
	})
}
