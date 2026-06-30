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

package upgrade

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalSourcePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "valid path", path: "/tmp/sa5x_V1.6.5.cfw"},
		{name: "empty path", path: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LocalSource{FilePath: tt.path}.Path()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.path, got)
		})
	}
}

func TestLocalSourceVersion(t *testing.T) {
	// firmware.ParseVersion reads the version from a trailing
	// "?v=V<major>.<minor>.<patch>.<commit>" footer.
	fwPath := filepath.Join(t.TempDir(), "sa5x_V1.6.5.cfw")
	require.NoError(t, os.WriteFile(fwPath, []byte("?v=V1.6.5.12345678,f=sa5x_V1.6.5.cfw"), 0o600))

	got, err := LocalSource{FilePath: fwPath}.Version()
	require.NoError(t, err)
	require.Equal(t, "1.6.5", got.String())

	_, err = LocalSource{FilePath: ""}.Version()
	require.Error(t, err)
}
