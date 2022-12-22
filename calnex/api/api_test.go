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
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/go-ini/ini"
	"github.com/stretchr/testify/require"
)

func TestChannel(t *testing.T) {
	legitChannelNamesToChannel := map[string]Channel{
		"1":   ChannelONE,
		"2":   ChannelTWO,
		"c":   ChannelC,
		"d":   ChannelD,
		"VP1": ChannelVP1,
	}
	for channelS, channel := range legitChannelNamesToChannel {
		c, err := ChannelFromString(channelS)
		require.NoError(t, err)
		require.Equal(t, channel, *c)

		c = new(Channel)
		err = c.UnmarshalText([]byte(channelS))
		require.NoError(t, err)
		require.Equal(t, channel, *c)
	}

	wrongChannelNames := []string{"", "?", "z", "foo"}
	for _, channelS := range wrongChannelNames {
		c, err := ChannelFromString(channelS)
		require.Nil(t, c)
		require.ErrorIs(t, errBadChannel, err)

		c = new(Channel)
		err = c.UnmarshalText([]byte(channelS))
		require.ErrorIs(t, errBadChannel, err)
	}
}

func TestProbe(t *testing.T) {
	legitProbeNamesToProbe := map[string]Probe{
		"ntp": ProbeNTP,
		"ptp": ProbePTP,
		"pps": ProbePPS,
	}
	for probeS, probe := range legitProbeNamesToProbe {
		p, err := ProbeFromString(probeS)
		require.NoError(t, err)
		require.Equal(t, probe, *p)

		p = new(Probe)
		err = p.UnmarshalText([]byte(probeS))
		require.NoError(t, err)
		require.Equal(t, probe, *p)
	}
	wrongProbeNames := []string{"", "?", "z", "dns"}
	for _, probeS := range wrongProbeNames {
		p, err := ProbeFromString(probeS)
		require.Nil(t, p)
		require.ErrorIs(t, errBadProbe, err)

		p = new(Probe)
		err = p.UnmarshalText([]byte(probeS))
		require.ErrorIs(t, errBadProbe, err)
	}
}

func TestProbeFromCalnex(t *testing.T) {
	legitProbeNamesToProbe := map[string]Probe{
		"2":     ProbeNTP,
		"0":     ProbePTP,
		"1 PPS": ProbePPS,
	}
	for probeH, probe := range legitProbeNamesToProbe {
		p, err := ProbeFromCalnex(probeH)
		require.NoError(t, err)
		require.Equal(t, probe, *p)
	}
	wrongProbeNames := []string{"", "?", "z", "dns"}
	for _, probe := range wrongProbeNames {
		p, err := ProbeFromCalnex(probe)
		require.Nil(t, p)
		require.ErrorIs(t, errBadProbe, err)
	}
}

func TestCalnexName(t *testing.T) {
	require.Equal(t, "NTP", ProbeNTP.CalnexName())
	require.Equal(t, "PTP", ProbePTP.CalnexName())
	require.Equal(t, "1 PPS", ProbePPS.CalnexName())
}

func TestTLSSetting(t *testing.T) {
	calnexAPI := NewAPI("localhost", false)
	// Never ever ever allow insecure over https
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	require.Equal(t, transport, calnexAPI.Client.Transport)

	calnexAPI = NewAPI("localhost", true)
	transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	require.Equal(t, transport, calnexAPI.Client.Transport)
}

func TestFetchCsv(t *testing.T) {
	sampleResp := "1607961193.773740,-000.000000250501"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	legitChannelNames := []Channel{ChannelONE, ChannelTWO, ChannelC, ChannelD, ChannelVP22}

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()
	for _, channel := range legitChannelNames {
		lines, err := calnexAPI.FetchCsv(channel, true)
		require.NoError(t, err)
		require.Equal(t, 1, len(lines))
		require.Equal(t, sampleResp, strings.Join(lines[0], ","))
	}
}

func TestFetchCsvNoData(t *testing.T) {
	sampleResp := "{\"message\": \"No data available\", \"result\": true}"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()
	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()
	lines, err := calnexAPI.FetchCsv(ChannelVP22, true)
	require.Error(t, err)
	require.Nil(t, lines)
}

func TestFetchChannelProtocol_NTP(t *testing.T) {
	sampleResp := "measure/ch9/ptp_synce/mode/probe_type=2"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	probe, err := calnexAPI.FetchChannelProbe(ChannelVP1)
	require.NoError(t, err)
	require.Equal(t, ProbeNTP, *probe)
}

func TestFetchChannelProtocol_PTP(t *testing.T) {
	sampleResp := "measure/ch10/ptp_synce/mode/probe_type=0"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	probe, err := calnexAPI.FetchChannelProbe(ChannelVP2)
	require.NoError(t, err)
	require.Equal(t, ProbePTP, *probe)
}

func TestFetchChannelProtocol_PPS(t *testing.T) {
	sampleResp := "measure/ch0/signal_type=1 PPS"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	probe, err := calnexAPI.FetchChannelProbe(ChannelA)
	require.NoError(t, err)
	require.Equal(t, ProbePPS, *probe)
}

func TestFetchChannelTarget_NTP(t *testing.T) {
	sampleResp := "measure/ch9/ptp_synce/ntp/server_ip=fd00:3116:301a::3e"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	target, err := calnexAPI.FetchChannelTarget(ChannelVP1, ProbeNTP)
	require.NoError(t, err)
	require.Equal(t, "fd00:3116:301a::3e", target)
}

func TestFetchChannelTarget_PTP(t *testing.T) {
	sampleResp := "measure/ch9/ptp_synce/ptp/master_ip=fd00:3116:301a::3e"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	target, err := calnexAPI.FetchChannelTarget(ChannelVP1, ProbePTP)
	require.NoError(t, err)
	require.Equal(t, "fd00:3116:301a::3e", target)
}

func TestFetchChannelTarget_PPS(t *testing.T) {
	sampleResp := "measure/ch0/signal_type=1 PPS"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	target, err := calnexAPI.FetchChannelTarget(ChannelA, ProbePPS)
	require.NoError(t, err)
	require.Equal(t, "1 PPS", target)
}

func TestFetchUsedChannels(t *testing.T) {
	sampleResp := "[measure]\nch0\\used=Yes\nch0\\installed=1\nch7\\used=No\nch7\\installed=1\nch29\\used=Yes\nch29\\installed=0\nch30\\used=Yes\nch30\\installed=1\n"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	expected := []Channel{ChannelA, ChannelVP22}
	used, err := calnexAPI.FetchUsedChannels()
	require.NoError(t, err)
	require.ElementsMatch(t, expected, used)
}

func TestFetchSettings(t *testing.T) {
	sampleResp := "[measure]\nch0\\synce_enabled=Off\n"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	f, err := calnexAPI.FetchSettings()
	require.NoError(t, err)
	require.Equal(t, f.Section("measure").Key("ch0\\synce_enabled").Value(), OFF)
}

func TestFetchStatus(t *testing.T) {
	sampleResp := "{\n\"referenceReady\": true,\n\"modulesReady\": true,\n\"measurementActive\": false\n}"
	expected := &Status{
		ModulesReady:      true,
		ReferenceReady:    true,
		MeasurementActive: false,
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	f, err := calnexAPI.FetchStatus()
	require.NoError(t, err)
	require.Equal(t, expected, f)
}

func TestPushSettings(t *testing.T) {
	sampleResp := "{\n\"result\": true\n}"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	sampleConfig := "[measure]\nch0\\synce_enabled=Off\n"
	f, err := ini.Load([]byte(sampleConfig))
	require.NoError(t, err)

	err = calnexAPI.PushSettings(f)
	require.NoError(t, err)
}

func TestFetchVersion(t *testing.T) {
	sampleResp := "{\"firmware\": \"2.13.1.0.5583D-20210924\"}"
	expected := &Version{
		Firmware: "2.13.1.0.5583D-20210924",
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	f, err := calnexAPI.FetchVersion()
	require.NoError(t, err)
	require.Equal(t, expected, f)
}

func TestPushVersion(t *testing.T) {
	sampleResp := "{\n\"result\" : true,\n\"message\" : \"Installing firmware Version: 2.13.1.0.5583D-20210924\"\n}"
	expected := &Result{
		Result:  true,
		Message: "Installing firmware Version: 2.13.1.0.5583D-20210924",
	}
	// Firmware file itself
	fw, err := os.CreateTemp("/tmp", "calnex")
	require.NoError(t, err)
	defer fw.Close()
	defer os.Remove(fw.Name())
	_, err = fw.WriteString("Hello Calnex!")
	require.NoError(t, err)

	// Firmware file saved via http
	fwres, err := os.CreateTemp("/tmp", "calnex")
	require.NoError(t, err)
	defer os.Remove(fwres.Name())

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		defer r.Body.Close()
		defer fwres.Close()
		_, err := io.Copy(fwres, r.Body)
		require.NoError(t, err)

		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	r, err := calnexAPI.PushVersion(fw.Name())
	require.NoError(t, err)
	require.Equal(t, expected, r)

	originalFW, err := os.ReadFile(fw.Name())
	require.NoError(t, err)

	uploadedFW, err := os.ReadFile(fwres.Name())
	require.NoError(t, err)

	require.Equal(t, originalFW, uploadedFW)
}

func TestPost(t *testing.T) {
	sampleResp := "{\n\"result\" : true,\n\"message\" : \"LGTM\"\n}"
	expected := &Result{
		Result:  true,
		Message: "LGTM",
	}
	postData := []byte("Whatever")
	serverReceived := &bytes.Buffer{}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		defer r.Body.Close()
		_, err := serverReceived.ReadFrom(r.Body)
		require.NoError(t, err)
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	buf := bytes.NewBuffer(postData)
	r, err := calnexAPI.post(parsed.String(), buf)
	require.NoError(t, err)
	require.Equal(t, expected, r)
	require.Equal(t, postData, serverReceived.Bytes())
}

func TestGet(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if strings.Contains(r.URL.Path, "getstatus") {
			// FetchStatus
			fmt.Fprintln(w, "{\n\"referenceReady\": true,\n\"modulesReady\": true,\n\"measurementActive\": true\n}")
		} else {
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		}
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	err := calnexAPI.StartMeasure()
	require.NoError(t, err)

	err = calnexAPI.StopMeasure()
	require.NoError(t, err)

	err = calnexAPI.ClearDevice()
	require.NoError(t, err)

	err = calnexAPI.Reboot()
	require.NoError(t, err)
}

func TestHTTPError(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	f := ini.Empty()
	err := calnexAPI.PushSettings(f)
	require.Error(t, err)
}

func TestFetchProblemReport(t *testing.T) {
	expectedReportContent := "I am a problem report"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprint(w, expectedReportContent)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	dir, err := os.MkdirTemp("/tmp", "calnex")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	reportFilePath, err := calnexAPI.FetchProblemReport(dir)
	require.NoError(t, err)
	require.FileExists(t, reportFilePath)
	defer os.Remove(reportFilePath)

	require.Contains(t, reportFilePath, "calnex_problem_report_")
	require.Contains(t, reportFilePath, ".tar")

	reportContent, err := os.ReadFile(reportFilePath)
	require.NoError(t, err)

	require.Equal(t, expectedReportContent, string(reportContent))
}

func TestPushCert(t *testing.T) {
	sampleResp := "{\n\"result\" : true,\n\"message\" : \"The API Interface will now be restarted\"\n}"
	expected := &Result{
		Result:  true,
		Message: "The API Interface will now be restarted",
	}
	// @lint-ignore PRIVATEKEY insecure-private-key-storage
	cert := []byte("-----BEGIN CERTIFICATE-----\nI am a certificate\n-----END CERTIFICATE-----\n-----BEGIN RSA PRIVATE KEY-----I am a key-----END RSA PRIVATE KEY-----")

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, cert, body)
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	r, err := calnexAPI.PushCert(cert)
	require.NoError(t, err)
	require.Equal(t, expected, r)
}

func TestParseResponse(t *testing.T) {
	expected := "500 mV"
	r, err := parseResponse("ch0\\trig_level=500 mV")
	require.NoError(t, err)
	require.Equal(t, expected, r)

	r, err = parseResponse("invalid")
	require.Equal(t, "", r)
	require.ErrorIs(t, errAPI, err)

	r, err = parseResponse("too=many=parts")
	require.Equal(t, "", r)
	require.ErrorIs(t, errAPI, err)
}

func TestMeasureChannelDatatypeMap(t *testing.T) {
	for i := 0; i <= 5; i++ {
		require.Equal(t, TE, MeasureChannelDatatypeMap[Channel(i)])
	}

	for i := 6; i <= 8; i++ {
		_, ok := MeasureChannelDatatypeMap[Channel(i)]
		require.False(t, ok)
	}

	for i := 9; i <= 40; i++ {
		require.Equal(t, TWOWAYTE, MeasureChannelDatatypeMap[Channel(i)])
	}
}

func TestGnssStatus(t *testing.T) {
	sampleResp := "{\"antennaStatus\":\"OK\",\"locked\":true,\"lockedSatellites\":9,\"surveyComplete\":true,\"surveyPercentComplete\":100}"
	expected := &GNSS{
		AntennaStatus:         "OK",
		Locked:                true,
		LockedSatellites:      9,
		SurveyComplete:        true,
		SurveyPercentComplete: 100,
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	g, err := calnexAPI.GnssStatus()
	require.NoError(t, err)
	require.Equal(t, expected, g)
}
