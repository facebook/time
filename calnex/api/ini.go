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
	"bytes"

	"github.com/go-ini/ini"
)

// ToBuffer converts an INI file to a bytes.Buffer
func ToBuffer(f *ini.File) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	ini.PrettyFormat = false
	ini.PrettySection = false
	_, err := f.WriteTo(buf)
	return buf, err
}
