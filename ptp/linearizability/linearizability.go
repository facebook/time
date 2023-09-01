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

package linearizability

import (
	"context"
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"
)

// TestResult is what we get after the test run
type TestResult interface {
	Target() string
	Good() (bool, error)
	Explain() string
	Err() error
}

// TestConfig is a configuration for Tester
type TestConfig interface{}

// Tester is basically a half of PTP unicast client
type Tester interface {
	RunTest(ctx context.Context) TestResult
}

// ProcessMonitoringResults returns map of metrics based on TestResults
func ProcessMonitoringResults(prefix string, results map[string]TestResult) map[string]int {
	failed := 0
	broken := 0
	skipped := 0

	for _, tr := range results {
		good, err := tr.Good()
		if err != nil {
			if errors.Is(err, ErrGrantDenied) {
				log.Debugf("denied grant is just drained GM")
				skipped++
			} else {
				broken++
			}
		} else {
			if !good {
				failed++
			}
		}
	}
	// general stats to JSON output
	output := map[string]int{}
	output[fmt.Sprintf("%sfailed_tests", prefix)] = failed
	output[fmt.Sprintf("%sbroken_tests", prefix)] = broken
	output[fmt.Sprintf("%sskipped_tests", prefix)] = skipped
	output[fmt.Sprintf("%stotal_tests", prefix)] = len(results)
	output[fmt.Sprintf("%spassed_tests", prefix)] = len(results) - skipped - failed - broken
	return output
}
