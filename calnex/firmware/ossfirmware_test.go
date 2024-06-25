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

package firmware

import (
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"
)

func TestOSSFW(t *testing.T) {
	expectedFilePath := "/tmp/sentinel_fw_v2.13.1.0.5583D-20210924.tar"
	expectedVersion, _ := version.NewVersion("2.13.1.0.5583d-20210924")
	fw, err := NewOSSFW(expectedFilePath)
	require.NoError(t, err)

	p, err := fw.Path()
	require.NoError(t, err)
	require.Equal(t, expectedFilePath, p)

	v := fw.Version()
	require.NoError(t, err)
	require.Equal(t, expectedVersion, v)
}
