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

package export

import (
	"encoding/json"
	"fmt"
	"io"
)

// Logger is an interface which will be used to output logging entries
type Logger interface {
	PrintEntry(e *Entry)
}

// JSONLogger is a json logger implementation
type JSONLogger struct {
	Out io.Writer
}

// PrintEntry prints logging entity
func (j JSONLogger) PrintEntry(e *Entry) {
	entryj, _ := json.Marshal(e)
	fmt.Fprintln(j.Out, string(entryj))
}
