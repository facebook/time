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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/facebook/time/calnex/api"
	"github.com/stretchr/testify/require"
)

func TestFirmware(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "calnex")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	filepath := path.Join(dir, "sentinel_fw_v2.13.1.0.5583D-20210924.tar")
	f, err := os.Create(filepath)
	require.NoError(t, err)
	require.NotNil(t, f)
	f.Close()

	fw := &OSSFW{
		Filepath: filepath,
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if strings.Contains(r.URL.Path, "version") {
			// FetchVersion
			fmt.Fprintln(w, "{ \"firmware\": \"2.11.1.0.5583D-20210924\" }")
		} else if strings.Contains(r.URL.Path, "getstatus") {
			// FetchStatus
			fmt.Fprintln(w, "{\n\"referenceReady\": true,\n\"modulesReady\": true,\n\"measurementActive\": true\n}")
		} else if strings.Contains(r.URL.Path, "stopmeasurement") {
			// StopMeasure
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		} else if strings.Contains(r.URL.Path, "updatefirmware") {
			// PushVersion
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		} else if strings.Contains(r.URL.Path, "instrument/status") {
			fmt.Fprintln(w, "{\"Channels\":{\"1\":{\"Progress\":-1,\"Slot\":\"1\",\"State\":\"Ready\",\"Type\":\"10G Packet Module (V2)\"},\"2\":{\"Progress\":-1,\"Slot\":\"1\",\"State\":\"Ready\",\"Type\":\"10G Packet Module (V2)\"},\"C\":{\"Progress\":-1,\"Slot\":\"C\",\"State\":\"Ready\",\"Type\":\"Clock Module\"},\"D\":{\"Progress\":-1,\"Slot\":\"C\",\"State\":\"Ready\",\"Type\":\"Clock Module\"}},\"Modules\":{\"1\":{\"Channels\":[\"1\",\"2\"],\"Progress\":-1,\"State\":\"Ready\",\"Type\":\"Packet Module (V2)\"},\"C\":{\"Channels\":[\"C\",\"D\"],\"Progress\":-1,\"State\":\"Ready\",\"Type\":\"Clock Module\"}}}")
		}
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err = Firmware(parsed.Host, true, fw, true, false)
	require.NoError(t, err)
}

func TestFirmwareForce(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "calnex")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	filepath := path.Join(dir, "sentinel_fw_v2.13.1.0.5583D-20210924.tar")
	f, err := os.Create(filepath)
	require.NoError(t, err)
	require.NotNil(t, f)
	f.Close()

	fw := &OSSFW{
		Filepath: filepath,
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if strings.Contains(r.URL.Path, "version") {
			// FetchVersion
			fmt.Fprintln(w, "{ \"firmware\": \"2.13.1.0.5583D-20210924\" }")
		} else if strings.Contains(r.URL.Path, "getstatus") {
			// FetchStatus
			fmt.Fprintln(w, "{\n\"referenceReady\": true,\n\"modulesReady\": true,\n\"measurementActive\": true\n}")
		} else if strings.Contains(r.URL.Path, "stopmeasurement") {
			// StopMeasure
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		} else if strings.Contains(r.URL.Path, "updatefirmware") {
			// PushVersion
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		} else if strings.Contains(r.URL.Path, "instrument/status") {
			fmt.Fprintln(w, "{\"Channels\":{\"1\":{\"Progress\":-1,\"Slot\":\"1\",\"State\":\"Ready\",\"Type\":\"10G Packet Module (V2)\"},\"2\":{\"Progress\":-1,\"Slot\":\"1\",\"State\":\"Ready\",\"Type\":\"10G Packet Module (V2)\"},\"C\":{\"Progress\":-1,\"Slot\":\"C\",\"State\":\"Ready\",\"Type\":\"Clock Module\"},\"D\":{\"Progress\":-1,\"Slot\":\"C\",\"State\":\"Ready\",\"Type\":\"Clock Module\"}},\"Modules\":{\"1\":{\"Channels\":[\"1\",\"2\"],\"Progress\":-1,\"State\":\"Ready\",\"Type\":\"Packet Module (V2)\"},\"C\":{\"Channels\":[\"C\",\"D\"],\"Progress\":-1,\"State\":\"Ready\",\"Type\":\"Clock Module\"}}}")
		}
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err = Firmware(parsed.Host, true, fw, true, true)
	require.NoError(t, err)
}

func TestFirmwareInProgress(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "calnex")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	filepath := path.Join(dir, "sentinel_fw_v2.13.1.0.5583D-20210924.tar")
	f, err := os.Create(filepath)
	require.NoError(t, err)
	require.NotNil(t, f)
	f.Close()

	fw := &OSSFW{
		Filepath: filepath,
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if strings.Contains(r.URL.Path, "version") {
			// FetchVersion
			fmt.Fprintln(w, "{ \"firmware\": \"2.13.1.0.5583D-20210924\" }")
		} else if strings.Contains(r.URL.Path, "getstatus") {
			// FetchStatus
			fmt.Fprintln(w, "{\n\"referenceReady\": true,\n\"modulesReady\": true,\n\"measurementActive\": true\n}")
		} else if strings.Contains(r.URL.Path, "stopmeasurement") {
			// StopMeasure
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		} else if strings.Contains(r.URL.Path, "updatefirmware") {
			// PushVersion
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		} else if strings.Contains(r.URL.Path, "instrument/status") {
			fmt.Fprintln(w, "{\"Channels\":{\"1\":{\"Progress\":42,\"Slot\":\"1\",\"State\":\"Ready\",\"Type\":\"10G Packet Module (V2)\"},\"2\":{\"Progress\":42,\"Slot\":\"1\",\"State\":\"Ready\",\"Type\":\"10G Packet Module (V2)\"},\"C\":{\"Progress\":42,\"Slot\":\"C\",\"State\":\"Ready\",\"Type\":\"Clock Module\"},\"D\":{\"Progress\":42,\"Slot\":\"C\",\"State\":\"Ready\",\"Type\":\"Clock Module\"}},\"Modules\":{\"1\":{\"Channels\":[\"1\",\"2\"],\"Progress\":42,\"State\":\"Ready\",\"Type\":\"Packet Module (V2)\"},\"C\":{\"Channels\":[\"C\",\"D\"],\"Progress\":42,\"State\":\"Ready\",\"Type\":\"Clock Module\"}}}")
		}
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()

	err = Firmware(parsed.Host, true, fw, true, false)
	require.NoError(t, err)
}
