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
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-ini/ini"
)

// API is struct for accessing calnex API
type API struct {
	Client *http.Client
	source string
}

// Status is a struct representing Calnex status JSON response
type Status struct {
	ReferenceReady    bool
	ModulesReady      bool
	MeasurementActive bool
}

// Result is a struct representing Calnex result JSON response
type Result struct {
	Result  bool
	Message string
}

// Version is a struct representing Calnex version JSON response
type Version struct {
	Firmware string
}

// Channel is a Calnex channel object
type Channel int

// Avalable Calnex channels
const (
	ChannelA Channel = iota
	ChannelB
	ChannelC
	ChannelD
	ChannelE
	ChannelF
	ChannelONE
	ChannelTWO
)

// See https://fburl.com/rnf8uthd for the source these values
// channelDatatypeMap is a Map of the channel to the data type
var channelDatatypeMap = map[Channel]string{
	ChannelA:   "tie",
	ChannelB:   "tie",
	ChannelC:   "tie",
	ChannelD:   "tie",
	ChannelE:   "tie",
	ChannelF:   "tie",
	ChannelONE: "2wayte",
	ChannelTWO: "2wayte",
}

// channelStringToCalnex is a map of String channels to a Calnex variant
var channelStringToCalnex = map[string]Channel{
	"a": ChannelA,
	"b": ChannelB,
	"c": ChannelC,
	"d": ChannelD,
	"e": ChannelE,
	"f": ChannelF,
	"1": ChannelONE,
	"2": ChannelTWO,
}

// ChannelCalnexToString is a map of Calnex channels to a String variant
var ChannelCalnexToString = map[Channel]string{
	ChannelA:   "a",
	ChannelB:   "b",
	ChannelC:   "c",
	ChannelD:   "d",
	ChannelE:   "e",
	ChannelF:   "f",
	ChannelONE: "1",
	ChannelTWO: "2",
}

// ChannelFromString returns Channel object from String version
func ChannelFromString(value string) (*Channel, error) {
	c, ok := channelStringToCalnex[value]
	if !ok {
		return nil, errBadChannel
	}

	return &c, nil
}

// String returns String friendly channel name like "a" or "2"
func (c Channel) String() string {
	return ChannelCalnexToString[c]
}

// UnmarshalText channel from string version
func (c *Channel) UnmarshalText(value []byte) error {
	cr, err := ChannelFromString(string(value))
	if err != nil {
		return err
	}
	*c = *cr
	return nil
}

// Calnex returns calnex friendly channel name like 1 or 7
func (c Channel) Calnex() int {
	return int(c)
}

// CalnexAPI returns channel name in API format like "ch2"
func (c Channel) CalnexAPI() string {
	return fmt.Sprintf("ch%d", c.Calnex())
}

// Probe is a Calnex probe protocol
type Probe int

// Probe numbers by calnex
const (
	ProbePTP Probe = 0
	ProbeNTP Probe = 2
)

// probeStringToProbe is a map of String probe to a Calnex variant
var probeStringToProbe = map[string]Probe{
	"ptp": ProbePTP,
	"ntp": ProbeNTP,
}

// probeCalnexToProbe is a map of Calnex to a probe variant
var probeCalnexAPIToProbe = map[string]Probe{
	fmt.Sprintf("%d", int(ProbePTP)): ProbePTP,
	fmt.Sprintf("%d", int(ProbeNTP)): ProbeNTP,
}

// probeToString is a map of probe to String variant
var probeToString = map[Probe]string{
	ProbePTP: "ptp",
	ProbeNTP: "ntp",
}

// probeToCalnexName is a map of probe to a Calnex specific name
var probeToCalnexName = map[Probe]string{
	ProbePTP: "PTP slave",
	ProbeNTP: "NTP client",
}

// probeToServerType is a map of probe to Calnex server name
var probeToServerType = map[Probe]string{
	ProbePTP: "master_ip",
	ProbeNTP: "server_ip",
}

// ProbeFromString returns Channel object from String version
func ProbeFromString(value string) (*Probe, error) {
	p, ok := probeStringToProbe[value]
	if !ok {
		return nil, errBadProbe
	}
	return &p, nil
}

// ProbeFromCalnex returns Channel object from String version
func ProbeFromCalnex(calnex string) (*Probe, error) {
	p, ok := probeCalnexAPIToProbe[calnex]
	if !ok {
		return nil, errBadProbe
	}
	return &p, nil
}

// String returns String friendly probe name like "ntp" or "ptp"
func (p Probe) String() string {
	return probeToString[p]
}

// UnmarshalText probe from string version
func (p *Probe) UnmarshalText(value []byte) error {
	pr, err := ProbeFromString(string(value))
	if err != nil {
		return err
	}
	*p = *pr
	return nil
}

// ServerType returns server type like "server_ip" or "master_ip"
func (p Probe) ServerType() string {
	return probeToServerType[p]
}

// CalnexName returns Calnex Name like "PTP slave" or "NTP client"
func (p Probe) CalnexName() string {
	return probeToCalnexName[p]
}

const (
	// measureURL is a base URL for to the measurement API
	measureURL = "https://%s/api/get/measure/%s/ptp_synce/%s/%s"
	dataURL    = "https://%s/api/getdata?channel=%s&datatype=%s&reset=true"

	startMeasure = "https://%s/api/startmeasurement"
	stopMeasure  = "https://%s/api/stopmeasurement"

	getSettingsURL      = "https://%s/api/getsettings"
	setSettingsURL      = "https://%s/api/setsettings"
	getStatusURL        = "https://%s/api/getstatus"
	getProblemReportURL = "https://%s/api/getproblemreport"

	clearDeviceURL = "https://%s/api/cleardevice?action=cleardevice"
	rebootURL      = "https://%s/api/reboot?action=reboot"

	versionURL  = "https://%s/api/version"
	firmwareURL = "https://%s/api/updatefirmware"
)

// Calnex Status contants
const (
	ON  = "On"
	OFF = "Off"
	YES = "Yes"
	NO  = "No"
)

var (
	errBadChannel = errors.New("channel is not recognized")
	errBadProbe   = errors.New("probe protocol is not recognized")
	errAPI        = errors.New("invalid response from API")
)

func parseResponse(response string) (string, error) {
	s := strings.Split(strings.TrimSuffix(response, "\n"), "=")
	if len(s) != 2 {
		return "", errAPI
	}

	return s[1], nil
}

// NewAPI returns an pointer of API struct with default values.
func NewAPI(source string, insecureTLS bool) *API {
	return &API{
		Client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureTLS},
			},
			Timeout: 2 * time.Minute,
		},
		source: source,
	}
}

// FetchCsv takes channel name (like 1, 2, c, d)
// it returns list of CSV lines which is []string
func (a *API) FetchCsv(channel Channel) ([][]string, error) {
	url := fmt.Sprintf(dataURL, a.source, channel, channelDatatypeMap[channel])
	resp, err := a.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(http.StatusText(resp.StatusCode))
	}

	var res [][]string
	csvReader := csv.NewReader(resp.Body)
	csvReader.Comment = '#'
	for {
		csvLine, err := csvReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			} else {
				return nil, fmt.Errorf("failed to parse csv for data from channel %s: %v", channel.String(), err)
			}
		}
		res = append(res, csvLine)
	}
	return res, nil
}

// FetchChannelProbe returns monitored protocol of the channel
func (a *API) FetchChannelProbe(channel Channel) (*Probe, error) {
	url := fmt.Sprintf(measureURL, a.source, channel.CalnexAPI(), "mode", "probe_type")
	resp, err := a.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(http.StatusText(resp.StatusCode))
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	probe, err := parseResponse(string(b))
	if err != nil {
		return nil, err
	}
	p, err := ProbeFromCalnex(probe)
	return p, err
}

// FetchChannelTargetIP returns the IP address of the server monitored on the channel
func (a *API) FetchChannelTargetIP(channel Channel, probe Probe) (string, error) {
	url := fmt.Sprintf(measureURL, a.source, channel.CalnexAPI(), probe.String(), probe.ServerType())
	resp, err := a.Client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New(http.StatusText(resp.StatusCode))
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return parseResponse(string(b))
}

// FetchUsedChannels returns list of channels in use
func (a *API) FetchUsedChannels() ([]Channel, error) {
	channels := []Channel{}
	f, err := a.FetchSettings()
	if err != nil {
		return channels, err
	}

	for ch := range ChannelCalnexToString {
		chStatus := f.Section("measure").Key(fmt.Sprintf("%s\\used", ch.CalnexAPI())).String()
		if chStatus == "Yes" {
			channels = append(channels, ch)
		}
	}
	return channels, err
}

// FetchChannelTargetName returns the hostname of the server monitored on the channel
func (a *API) FetchChannelTargetName(channel Channel, probe Probe) (string, error) {
	ip, err := a.FetchChannelTargetIP(channel, probe)
	if err != nil {
		return ip, err
	}

	hostnames, err := net.LookupAddr(ip)
	if err != nil {
		return "", err
	}

	return hostnames[0], nil
}

// FetchSettings returns the calnex settings
func (a *API) FetchSettings() (*ini.File, error) {
	url := fmt.Sprintf(getSettingsURL, a.source)
	resp, err := a.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(http.StatusText(resp.StatusCode))
	}

	return ini.Load(resp.Body)
}

// FetchStatus returns the calnex status
func (a *API) FetchStatus() (*Status, error) {
	url := fmt.Sprintf(getStatusURL, a.source)
	resp, err := a.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(http.StatusText(resp.StatusCode))
	}

	s := &Status{}
	if err = json.NewDecoder(resp.Body).Decode(s); err != nil {
		return nil, err
	}

	return s, nil
}

// FetchProblemReport saves a problem report
func (a *API) FetchProblemReport(dir string) (string, error) {
	url := fmt.Sprintf(getProblemReportURL, a.source)
	resp, err := a.Client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New(http.StatusText(resp.StatusCode))
	}

	// calnex_problem_report_2021-12-07_10-42-26.tar
	reportFileName := path.Join(dir, fmt.Sprintf("calnex_problem_report_%s.tar", time.Now().Format("2006-01-02_15-04-05")))
	reportF, err := os.Create(reportFileName)
	if err != nil {
		return "", err
	}
	defer reportF.Close()

	_, err = io.Copy(reportF, resp.Body)
	if err != nil {
		return "", err
	}

	return reportFileName, nil
}

// FetchVersion returns current Firmware Version
func (a *API) FetchVersion() (*Version, error) {
	url := fmt.Sprintf(versionURL, a.source)
	resp, err := a.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(http.StatusText(resp.StatusCode))
	}

	v := &Version{}
	if err = json.NewDecoder(resp.Body).Decode(v); err != nil {
		return nil, err
	}

	return v, nil
}

// PushVersion uploads a new Firmware Version to the device
func (a *API) PushVersion(path string) (*Result, error) {
	fw, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fw.Close()

	url := fmt.Sprintf(firmwareURL, a.source)
	buf := &bytes.Buffer{}
	_, err = buf.ReadFrom(fw)

	if err != nil {
		return nil, err
	}

	r, err := a.post(url, buf)
	return r, err
}

// PushSettings pushes the calnex settings
func (a *API) PushSettings(f *ini.File) error {
	buf, err := ToBuffer(f)
	if err != nil {
		return err
	}
	url := fmt.Sprintf(setSettingsURL, a.source)

	_, err = a.post(url, buf)
	return err
}

func (a *API) post(url string, content *bytes.Buffer) (*Result, error) {
	// content must be a bytes.Buffer or anything which supports .Len()
	// Otherwise Content-Length will not be set.
	resp, err := a.Client.Post(url, "application/x-www-form-urlencoded", content)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	r := &Result{}
	if err = json.NewDecoder(resp.Body).Decode(r); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return r, errors.New(http.StatusText(resp.StatusCode))
	}

	if !r.Result {
		return nil, errors.New(r.Message)
	}

	return r, nil
}

func (a *API) get(path string) error {
	url := fmt.Sprintf(path, a.source)
	resp, err := a.Client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New(http.StatusText(resp.StatusCode))
	}

	r := &Result{}
	if err = json.NewDecoder(resp.Body).Decode(r); err != nil {
		return err
	}

	if !r.Result {
		return errors.New(r.Message)
	}

	return nil
}

// StartMeasure starts measurement
func (a *API) StartMeasure() error {
	return a.get(startMeasure)
}

// StopMeasure stops measurement
func (a *API) StopMeasure() error {
	return a.get(stopMeasure)
}

// ClearDevice clears device data
func (a *API) ClearDevice() error {
	return a.get(clearDeviceURL)
}

// Reboot the device
func (a *API) Reboot() error {
	return a.get(rebootURL)
}
