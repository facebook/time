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

package chrony

// LoggerInterface is an interface for debug logging.
type LoggerInterface interface {
	Printf(format string, v ...interface{})
}

type noopLogger struct{}

func (noopLogger) Printf(_ string, _ ...interface{}) {}

// Logger is a default debug logger which simply discards all messages.
// It can be overridden by setting the global variable to a different implementation, like std log
//
//	chrony.Logger = log.New(os.Stderr, "", 0)
//
// or logrus
//
//	chrony.Logger = logrus.StandardLogger()
var Logger LoggerInterface = &noopLogger{}
