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
	"math"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/stats"
	log "github.com/sirupsen/logrus"
)

// SPTPHTTPTestResult is what we get after the test run
type SPTPHTTPTestResult struct {
	Config     SPTPHTTPTestConfig
	Offset     float64
	Error      error
	ClockClass ptp.ClockClass
}

// Target returns value of server
func (tr SPTPHTTPTestResult) Target() string {
	return tr.Config.Server
}

// Good check if the test passed
func (tr SPTPHTTPTestResult) Good() (bool, error) {
	if tr.Error != nil {
		return false, tr.Error
	}
	if math.Abs(tr.Offset) > float64(tr.Config.LinearizabilityTestMaxGMOffset.Nanoseconds()) {
		if tr.ClockClass != ptp.ClockClass6 {
			log.Warningf("linearizability test against %v ignored because the clock class is not Locked", tr.Config.Server)
			return true, nil
		}
		return false, nil
	}
	return true, nil
}

// Explain provides plain text explanation of linearizability test result
func (tr SPTPHTTPTestResult) Explain() string {
	msg := fmt.Sprintf("linearizability test against %q", tr.Config.Server)
	good, err := tr.Good()
	if good {
		return fmt.Sprintf("%s passed", msg)
	}
	if err != nil {
		return fmt.Sprintf("%s couldn't be completed because of error: %v", msg, tr.Error)
	}
	return fmt.Sprintf("%s failed because the offset %.2fns is > %v", msg, tr.Offset, tr.Config.LinearizabilityTestMaxGMOffset)
}

// Err returns an error value of the SPTPHTTPTestResult
func (tr SPTPHTTPTestResult) Err() error {
	return tr.Error
}

// SPTPHTTPTestConfig is a configuration for Tester
type SPTPHTTPTestConfig struct {
	Server                         string
	sptpurl                        string
	LinearizabilityTestMaxGMOffset time.Duration
}

// Target sets the server to test
func (p *SPTPHTTPTestConfig) Target(server string) {
	p.Server = server
}

// SPTPHTTPTester queries sptp http api to get linearizability metrics
type SPTPHTTPTester struct {
	cfg *SPTPHTTPTestConfig

	// measurement result
	result *SPTPHTTPTestResult
}

// NewSPTPHTTPTester initializes a Tester
func NewSPTPHTTPTester(server string, sptpurl string, linearizabilityTestMaxGMOffset time.Duration) (*SPTPHTTPTester, error) {
	cfg := &SPTPHTTPTestConfig{
		Server:                         server,
		sptpurl:                        sptpurl,
		LinearizabilityTestMaxGMOffset: linearizabilityTestMaxGMOffset,
	}

	t := &SPTPHTTPTester{
		cfg: cfg,
	}
	return t, nil
}

// Close the connection
func (lt *SPTPHTTPTester) Close() error {
	return nil
}

// RunTest performs one Tester run and will exit on completion.
// It collects sptp linearizability metrics from sptp http api.
// The result of the test will be returned, including any error arising during the test.
// Warning: the listener must be started via RunListener before calling this function.
func (lt *SPTPHTTPTester) RunTest(_ context.Context) TestResult {
	result := SPTPHTTPTestResult{
		Config: *lt.cfg,
		Error:  nil,
	}

	log.Debugf("test starting %s", lt.cfg.Server)
	stats, err := stats.FetchStats(lt.cfg.sptpurl)
	if err != nil {
		result.Error = err
		return result
	}

	found := false
	for _, s := range stats {
		if s.GMAddress == lt.cfg.Server {
			result.Offset = s.Offset
			result.ClockClass = s.ClockQuality.ClockClass
			if s.Error != "" {
				result.Error = errors.New(s.Error)
			}
			found = true
			break
		}
	}
	if !found {
		result.Error = fmt.Errorf("failed to find offset results for %s ", lt.cfg.Server)
	}
	lt.result = &result

	log.Debugf("test done %s", lt.cfg.Server)
	return result
}
