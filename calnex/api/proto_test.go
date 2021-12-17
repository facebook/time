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

package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIProto(t *testing.T) {
	legitAPIProtoNamesToAPIProto := map[string]APIProto{
		"http":  HTTP,
		"https": HTTPS,
	}

	var aproto APIProto
	require.Equal(t, HTTPS, aproto)

	keys := []string{}
	for protoS, proto := range legitAPIProtoNamesToAPIProto {
		err := aproto.Set(protoS)
		require.NoError(t, err)
		require.Equal(t, proto, aproto)
		require.Equal(t, protoS, aproto.String())
		keys = append(keys, protoS)
	}

	require.ElementsMatch(t, keys, strings.Split(aproto.Type(), ", "))

	wrongAPIProtoNames := []string{"", "?", "z", "dns"}
	for _, protoS := range wrongAPIProtoNames {
		err := aproto.Set(protoS)
		require.ErrorIs(t, errBadAPIProto, err)
	}
}
