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

func checkOSSFW(t *testing.T, expectedFilePath string, expectedVersion *version.Version) {
	fw, err := NewOSSFW(expectedFilePath)
	require.NoError(t, err)

	p, err := fw.Path()
	require.NoError(t, err)
	require.Equal(t, expectedFilePath, p)

	v := fw.Version()
	require.NoError(t, err)
	require.Equal(t, expectedVersion, v)
}

func checkErrorOSSFW(t *testing.T, expectedFilePath string) {
	fw, err := NewOSSFW(expectedFilePath)
	require.Nil(t, fw)
	require.Error(t, err)
}

func TestOSSFW(t *testing.T) {
	//all files are expected to produce the same version output
	//development/beta build versions (D after build number)
	expectedVersiondev, _ := version.NewVersion("13.1.0.5583d-20210924")

	// Test case for sentinel_fw
	expectedFilePathSentineldev := "/tmp/sentinel_fw_v2.13.1.0.5583D-20210924.tar"
	checkOSSFW(t, expectedFilePathSentineldev, expectedVersiondev)

	// Test case for sentry
	expectedFilePathSentrydev := "/tmp/sentry_fw_v2.13.1.0.5583D-20210924.tar"
	checkOSSFW(t, expectedFilePathSentrydev, expectedVersiondev)

	// Test case for calnex_combined
	expectedFilePathCalnexdev := "/tmp/calnex_combined_fw_R13.1.0.5583D-20210924.tar"
	checkOSSFW(t, expectedFilePathCalnexdev, expectedVersiondev)

	//full release versions (no D)
	expectedVersion, _ := version.NewVersion("13.1.0.5583-20210924")

	// Test case for sentinel_fw
	expectedFilePathSentinel := "/tmp/sentinel_fw_v2.13.1.0.5583-20210924.tar"
	checkOSSFW(t, expectedFilePathSentinel, expectedVersion)

	// Test case for sentry
	expectedFilePathSentry := "/tmp/sentry_fw_v2.13.1.0.5583-20210924.tar"
	checkOSSFW(t, expectedFilePathSentry, expectedVersion)

	// Test case for calnex_combined
	expectedFilePathCalnex := "/tmp/calnex_combined_fw_R13.1.0.5583-20210924.tar"
	checkOSSFW(t, expectedFilePathCalnex, expectedVersion)

	// Check errors
	expectedFilePathMissingStart := "/tmp/R13.1.0.5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathMissingStart)

	//missing v2./R
	expectedFilePathMissingSecond := "/tmp/sentinel_fw_13.1.0.5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathMissingSecond)

	//misspelt sintinel
	expectedFilePathBadStart := "/tmp/sentrinel_fw_R13.1.0.5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathBadStart)

	//both v2. and R
	expectedFilePathBadSecond := "/tmp/sentinel_fw_Rv2.13.1.0.5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathBadSecond)

	//.zip not .tar
	expectedFilePathBadExtension := "/tmp/sentinel_fw_R13.1.0.5583-20210924.zip"
	checkErrorOSSFW(t, expectedFilePathBadExtension)

	//HW version not 0-9
	expectedFilePathBadHWVersion := "/tmp/sentinel_fw_v10.13.1.0.5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathBadHWVersion)

	//malformed versions
	expectedFilePathBadBuild1 := "/tmp/sentinel_fw_v2.13.1.0.D5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathBadBuild1)

	expectedFilePathBadBuild2 := "/tmp/sentinel_fw_v2.13.1D.0.5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathBadBuild2)

	expectedFilePathBadBuild3 := "/tmp/sentinel_fw_v2.13D.1.0.5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathBadBuild3)

	expectedFilePathBadBuild4 := "/tmp/sentinel_fw_v2.13.1..10.5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathBadBuild4)

	expectedFilePathBadBuild5 := "/tmp/sentinel_fw_v2.13.1.1.10.5583-20210924.tar"
	checkErrorOSSFW(t, expectedFilePathBadBuild5)
}
