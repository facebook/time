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
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	version "github.com/hashicorp/go-version"
)

// OSSFW is an open source implementation of the FW interface
type OSSFW struct {
	filepath string
	version  *version.Version
}

// NewOSSFW returns initialized version of OSSFW
func NewOSSFW(source string) (*OSSFW, error) {
	var vs string

	fw := &OSSFW{
		filepath: source,
	}

	basename := filepath.Base(fw.filepath)
	// Extract version from filename
	// sentinel_fw_v2.13.1.0.5583D-20210924.tar -> 13.1.0.5583
	// calnex_combined_fw_R21.0.0.9705-20241111.tar -> 21.0.0.9705

	re := regexp.MustCompile(`(sentry_fw_|sentinel_fw_|calnex_combined_fw_)(v[0-9]\.|R)(.*)\.tar`)

	matches := re.FindStringSubmatch(basename)

	if len(matches) == 4 {
		vs = matches[3]
	} else {
		return nil, fmt.Errorf("invalid filename, expected prefix sentinel_fw_, sentry_fw_, or calnex_combined_fw_ followed by 'v2.' or 'R': %s", basename)
	}
	v, err := version.NewVersion(strings.ToLower(vs))
	if err != nil {
		return nil, err
	}
	//expect major, minor, patch, build
	if len(v.Segments()) != 4 {
		return nil, fmt.Errorf("invalid version format, expected 4 segments: %v", v.Segments())
	}
	fw.version = v
	return fw, err
}

// Version downloads latest firmware version
// sentinel_fw_v2.13.1.0.5583D-20210924.tar
func (f *OSSFW) Version() *version.Version {
	return f.version
}

// Path downloads latest firmware version
func (f *OSSFW) Path() (string, error) {
	return f.filepath, nil
}
