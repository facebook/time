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

package checks

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/facebook/time/calnex/api"
	"github.com/stretchr/testify/require"
)

func TestGNSS(t *testing.T) {
	r := GNSSRemediation{}
	c := GNSS{Remediation: r}
	require.Equal(t, "GNSS", c.Name())

	sampleResp := "{\"antennaStatus\":\"OK\",\"locked\":true,\"lockedSatellites\":9,\"surveyComplete\":true,\"surveyPercentComplete\":100}"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err := c.Run(parsed.Host, true)
	require.NoError(t, err)
}

func TestGNSSNoSatellites(t *testing.T) {
	r := GNSSRemediation{}
	c := GNSS{Remediation: r}
	require.Equal(t, "GNSS", c.Name())

	sampleResp := "{\"antennaStatus\":\"OK\",\"locked\":true,\"lockedSatellites\":2,\"surveyComplete\":true,\"surveyPercentComplete\":100}"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err := c.Run(parsed.Host, true)
	require.ErrorContains(t, err, "gnss: not enough satellites")
}

func TestGNSSNoAntenna(t *testing.T) {
	r := GNSSRemediation{}
	c := GNSS{Remediation: r}
	require.Equal(t, "GNSS", c.Name())

	sampleResp := "{\"antennaStatus\":\"BAD\",\"locked\":true,\"lockedSatellites\":9,\"surveyComplete\":true,\"surveyPercentComplete\":100}"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err := c.Run(parsed.Host, true)
	require.ErrorContains(t, err, "gnss: antenna status is: BAD")
}

func TestGNSSError(t *testing.T) {
	r := GNSSRemediation{}
	c := GNSS{Remediation: r}
	require.Equal(t, "GNSS", c.Name())

	err := c.Run("1.2.3.4", false)
	require.Error(t, err)

	want, _ := r.Remediate()
	got, err := c.Remediate()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestHTTP(t *testing.T) {
	r := HTTPRemediation{}
	c := HTTP{Remediation: r}
	require.Equal(t, "HTTP", c.Name())

	sampleResp := "{\"channelLinksReady\":true,\"ipAddressReady\":true,\"measurementActive\":true,\"measurementReady\":true,\"modulesReady\":true,\"referenceReady\":true}"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err := c.Run(parsed.Host, true)
	require.NoError(t, err)
}

func TestHTTPError(t *testing.T) {
	r := HTTPRemediation{}
	c := HTTP{Remediation: r}
	require.Equal(t, "HTTP", c.Name())

	err := c.Run("1.2.3.4", false)
	require.Error(t, err)

	want, _ := r.Remediate()
	got, err := c.Remediate()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestPing(t *testing.T) {
	r := PingRemediation{}
	c := Ping{Remediation: r}
	require.Equal(t, "Ping", c.Name())

	err := c.Run("::1", false)
	require.NoError(t, err)
}

func TestPingError(t *testing.T) {
	r := PingRemediation{}
	c := Ping{Remediation: r}
	require.Equal(t, "Ping", c.Name())

	err := c.Run("1.2.3.4", false)
	require.Error(t, err)

	want, _ := r.Remediate()
	got, err := c.Remediate()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestPSU(t *testing.T) {
	r := PSURemediation{}
	c := PSU{Remediation: r}
	require.Equal(t, "PSU", c.Name())

	sampleResp := "{\"power_supply_good\":true,\"supplies\":[{\"comms_good\":true,\"name\":\"PSU_module_A\",\"status_good\":true},{\"comms_good\":true,\"name\":\"PSU_module_B\",\"status_good\":true}]}"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err := c.Run(parsed.Host, true)
	require.NoError(t, err)
}

func TestPSUSingleBad(t *testing.T) {
	r := PSURemediation{}
	c := PSU{Remediation: r}
	require.Equal(t, "PSU", c.Name())

	sampleResp := "{\"power_supply_good\":false,\"supplies\":[{\"comms_good\":true,\"name\":\"PSU_module_A\",\"status_good\":true},{\"comms_good\":true,\"name\":\"PSU_module_B\",\"status_good\":false}]}"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err := c.Run(parsed.Host, true)
	require.ErrorContains(t, err, "psu: failed power supply #1: PSU_module_B")
}

func TestPSUBBad(t *testing.T) {
	r := PSURemediation{}
	c := PSU{Remediation: r}
	require.Equal(t, "PSU", c.Name())

	sampleResp := "{\"power_supply_good\":false}"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err := c.Run(parsed.Host, true)
	require.ErrorContains(t, err, "psu: failed power supply")
}

func TestPSUError(t *testing.T) {
	r := PSURemediation{}
	c := PSU{Remediation: r}
	require.Equal(t, "PSU", c.Name())

	err := c.Run("1.2.3.4", false)
	require.Error(t, err)

	want, _ := r.Remediate()
	got, err := c.Remediate()
	require.NoError(t, err)
	require.Equal(t, want, got)
}
