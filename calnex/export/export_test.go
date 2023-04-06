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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/facebook/time/calnex/api"
	"github.com/stretchr/testify/require"
)

type writer struct {
	data []string
}

func (w *writer) Close() error {
	return nil
}

func (w *writer) Write(p []byte) (int, error) {
	w.data = append(w.data, string(p))
	return len(p), nil
}

func TestExport(t *testing.T) {
	w := &writer{}
	l := JSONLogger{Out: w}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if strings.Contains(r.URL.Path, "getsettings") {
			// FetchUsedChannels
			fmt.Fprintln(w, "[measure]\nch0\\used=Yes\nch0\\installed=1\nch1\\used=No\nch1\\installed=0\nch9\\used=Yes\nch9\\installed=1\nch10\\used=No\nch10\\installed=1")
		} else if strings.Contains(r.URL.Path, "ch0/signal_type") {
			// FetchChannelProbe PPS
			fmt.Fprintln(w, "measure/ch0/signal_type=1 PPS")
		} else if strings.Contains(r.URL.Path, "ch9/ptp_synce/mode/probe_type") {
			// FetchChannelProbe NTP
			fmt.Fprintln(w, "measure/ch9/ptp_synce/mode/probe_type=2")
		} else if strings.Contains(r.URL.Path, "ch0/server_ip") {
			// FetchChannelTarget PPS
			fmt.Fprintln(w, "ch0/server_ip=127.0.0.1")
		} else if strings.Contains(r.URL.Path, "measure/ch9/ptp_synce/ntp/server_ip") {
			// FetchChannelTarget NTP
			fmt.Fprintln(w, "measure/ch9/ptp_synce/ntp/server_ip=127.0.0.1")
		} else if r.URL.Query().Get("channel") == "A" {
			// FetchCsv PPS
			fmt.Fprintln(w, "1607961193.773740,-000.000000250501")
		} else if r.URL.Query().Get("channel") == "VP1" {
			// FetchCsv NTP
			fmt.Fprintln(w, "1607961194.773740,-000.000000250504")
		}
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	expected := []string{
		fmt.Sprintf("{\"double\":{\"value\":-2.50504e-7},\"int\":{\"time\":1607961194},\"normal\":{\"channel\":\"VP1\",\"target\":\"127.0.0.1\",\"protocol\":\"ntp\",\"source\":\"%s\"}}\n", parsed.Host),
		fmt.Sprintf("{\"double\":{\"value\":-2.50501e-7},\"int\":{\"time\":1607961193},\"normal\":{\"channel\":\"A\",\"target\":\"127.0.0.1\",\"protocol\":\"pps\",\"source\":\"%s\"}}\n", parsed.Host),
	}
	err := Export(parsed.Host, true, true, []api.Channel{}, l)
	require.NoError(t, err)
	require.ElementsMatch(t, expected, w.data)
}

func TestExportFail(t *testing.T) {
	err := Export("localhost", true, true, []api.Channel{}, nil)
	require.ErrorIs(t, errNoUsedChannels, err)

	err = Export("localhost", true, true, []api.Channel{api.ChannelONE}, nil)
	require.ErrorIs(t, errNoTarget, err)
}
