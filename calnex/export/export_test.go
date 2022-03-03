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
	data string
}

func (w *writer) Close() error {
	return nil
}

func (w *writer) Write(p []byte) (int, error) {
	w.data = string(p)
	return len(p), nil
}

func TestExport(t *testing.T) {
	w := &writer{}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if strings.Contains(r.URL.Path, "getsettings") {
			// FetchUsedChannels
			fmt.Fprintln(w, "[measure]\nch0\\used=No\nch9\\used=Yes\nch10\\used=No")
		} else if strings.Contains(r.URL.Path, "probe_type") {
			// FetchChannelProtocol
			fmt.Fprintln(w, "measure/ch9/ptp_synce/mode/probe_type=2")
		} else if strings.Contains(r.URL.Path, "measure/ch9/ptp_synce/ntp/server_ip") {
			// FetchChannelTargetName
			fmt.Fprintln(w, "measure/ch9/ptp_synce/ntp/server_ip=127.0.0.1")
		} else if strings.Contains(r.URL.Path, "api/getdata") {
			// FetchCsv
			fmt.Fprintln(w, "1607961193.773740,-000.000000250501")
		}
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	expected := fmt.Sprintf("{\"float\":{\"value\":-2.50501e-7},\"int\":{\"time\":1607961193},\"normal\":{\"channel\":\"VP1\",\"target\":\"127.0.0.1\",\"protocol\":\"ntp\",\"source\":\"%s\"}}\n", parsed.Host)
	err := Export(parsed.Host, true, []api.Channel{}, w)
	require.NoError(t, err)
	require.Equal(t, expected, w.data)
}

func TestExportFail(t *testing.T) {
	w := &writer{}
	err := Export("localhost", true, []api.Channel{}, w)
	require.ErrorIs(t, errNoUsedChannels, err)

	err = Export("localhost", true, []api.Channel{api.ChannelONE}, w)
	require.ErrorIs(t, errNoTarget, err)
}
