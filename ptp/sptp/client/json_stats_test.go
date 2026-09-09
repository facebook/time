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

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/facebook/time/ptp/pdelay"
	ptp "github.com/facebook/time/ptp/protocol"
	gmstats "github.com/facebook/time/ptp/sptp/stats"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func TestJSONStats(t *testing.T) {
	stats, err := NewJSONStats()
	require.NoError(t, err)
	port, err := getFreePort()
	require.Nil(t, err, "Failed to allocate port")
	url := fmt.Sprintf("http://localhost:%d", port)
	go stats.Start("::1", port, time.Second, nil)
	time.Sleep(time.Second)

	stats.SetTickDuration(time.Millisecond)

	gm0 := &gmstats.Stat{
		GMAddress: "192.168.0.10",
		Error:     "mymy",
	}
	stats.SetGMStats(gm0)

	gm1 := &gmstats.Stat{
		GMAddress:         "192.168.0.13",
		Error:             "",
		GMPresent:         1,
		IngressTime:       1676997604198536785,
		MeanPathDelay:     float64(299995 * time.Microsecond),
		Offset:            float64(-100001 * time.Microsecond),
		PortIdentity:      "000000.0086.09c621",
		Priority1:         1,
		Priority2:         2,
		Priority3:         3,
		Selected:          true,
		StepsRemoved:      1,
		CorrectionFieldRX: int64(6 * time.Microsecond),
		CorrectionFieldTX: int64(4 * time.Microsecond),
	}
	stats.SetGMStats(gm1)

	require.NoError(t, err)

	gms, err := gmstats.FetchStats(url)
	require.NoError(t, err)
	expectedStats := gmstats.Stats{
		gm0,
		gm1,
	}
	require.Equal(t, expectedStats, gms)
}

func TestHeaders(t *testing.T) {
	stats, err := NewJSONStats()
	require.NoError(t, err)
	port, err := getFreePort()
	require.Nil(t, err, "Failed to allocate port")
	url := fmt.Sprintf("http://localhost:%d", port)
	go stats.Start("::1", port, time.Second, nil)
	time.Sleep(time.Second)

	c := http.Client{
		Timeout: time.Second * 2,
	}

	resp, err := c.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, applicationJSON, resp.Header.Get(contentType))
}

// httpGet issues a GET without http.Get's constant-url requirement
func httpGet(t *testing.T, url string) (*http.Response, error) {
	t.Helper()
	return httpDo(t, http.MethodGet, url)
}

// httpHead issues a HEAD without http.Head's constant-url requirement
func httpHead(t *testing.T, url string) (*http.Response, error) {
	t.Helper()
	return httpDo(t, http.MethodHead, url)
}

func httpDo(t *testing.T, method, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, nil)
	require.NoError(t, err)
	return http.DefaultClient.Do(req)
}

func pingTestServer(t *testing.T, pinger Pinger) (string, *JSONStats) {
	t.Helper()
	port, err := getFreePort()
	require.NoError(t, err)
	stats, err := NewJSONStats()
	require.NoError(t, err)
	go stats.Start("::1", port, time.Minute, pinger)
	url := fmt.Sprintf("http://localhost:%d", port)
	require.Eventually(t, func() bool {
		resp, err := httpGet(t, url)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return true
	}, 5*time.Second, 10*time.Millisecond)
	return url, stats
}

func TestPingHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pinger := NewMockPinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any(), netip.MustParseAddr("ff02::6b")).
		Return(pdelay.Results{{
			Responder: netip.MustParseAddr("2401:db00::1"),
			T1:        time.Unix(1700000000, 0),
			T2:        time.Unix(1700000000, 100),
			T3:        time.Unix(1700000000, 200),
			T4:        time.Unix(1700000000, 310),
		}}, nil)

	url, _ := pingTestServer(t, pinger)
	res, err := pdelay.FetchPing(t.Context(), url, "ff02::6b", 5*time.Second)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.Equal(t, netip.MustParseAddr("2401:db00::1"), res[0].Responder)
	require.True(t, res[0].Valid())
}

func TestPingHandlerBadTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	url, _ := pingTestServer(t, NewMockPinger(ctrl))

	resp, err := httpGet(t, url+"/ping")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPingHandlerInFlightCountsAsRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pinger := NewMockPinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(nil, ErrPingInFlight)

	url, stats := pingTestServer(t, pinger)

	resp, err := httpGet(t, url+"/ping?target=ff02::6b")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	counters := stats.GetCounters()
	require.Equal(t, int64(1), counters["ptp.sptp.ping.rejected"], "contention is not a probe failure")
	require.Zero(t, counters["ptp.sptp.ping.errors"])
}

func TestPingHandlerInFlight(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pinger := NewMockPinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(nil, ErrPingInFlight)

	url, _ := pingTestServer(t, pinger)
	resp, err := httpGet(t, url+"/ping?target=ff02::6b")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

// a GET route also matches HEAD, which must not consume the ping slot or emit PTP traffic
func TestPingHandlerRejectsHead(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// no EXPECT: the mock fails the test if Ping is called at all
	url, _ := pingTestServer(t, NewMockPinger(ctrl))

	resp, err := httpHead(t, url+"/ping?target=ff02::6b")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestPingHandlerEmptyResultIsAnArray(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pinger := NewMockPinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(nil, nil)

	url, _ := pingTestServer(t, pinger)
	resp, err := httpGet(t, url+"/ping?target=ff02::6b")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	// a nil slice would marshal to null, which is not the documented contract
	require.JSONEq(t, "[]", string(body))
}

func TestPingHandlerRejectsForeignMulticast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// no EXPECT: the mock fails the test if Ping is called at all
	url, _ := pingTestServer(t, NewMockPinger(ctrl))

	resp, err := httpGet(t, url+"/ping?target=ff02::1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Ping must receive the unmapped address that policy approved, with the zone kept
// because it selects the egress interface
func TestPingHandlerAllowsPDelayGroups(t *testing.T) {
	for _, tt := range []struct{ group, want string }{
		{ptp.PDelayMulticastIPv4, ptp.PDelayMulticastIPv4},
		{ptp.PDelayMulticastIPv6, ptp.PDelayMulticastIPv6},
		{ptp.PDelayMulticastIPv6 + "%lo", ptp.PDelayMulticastIPv6 + "%lo"},
		{"::ffff:" + ptp.PDelayMulticastIPv4, ptp.PDelayMulticastIPv4},
	} {
		t.Run(tt.group, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			pinger := NewMockPinger(ctrl)
			pinger.EXPECT().Ping(gomock.Any(), netip.MustParseAddr(tt.want)).Return(pdelay.Results{}, nil)

			res, err := pdelay.FetchPing(t.Context(), mustPingServer(t, pinger), tt.group, 5*time.Second)
			require.NoError(t, err)
			require.Empty(t, res)
		})
	}
}

// an unspecified or broadcast address is never a probe target
func TestPingHandlerRejectsNonProbeTargets(t *testing.T) {
	for _, target := range []string{"::", "0.0.0.0", "255.255.255.255"} {
		t.Run(target, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			// no EXPECT: the mock fails if Ping is reached
			url := mustPingServer(t, NewMockPinger(ctrl))

			resp, err := httpGet(t, url+"/ping?target="+target)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

// ptping targets a host that is not a configured GM, so unicast must pass through
func TestPingHandlerAllowsUnicast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pinger := NewMockPinger(ctrl)
	want := netip.MustParseAddr("2401:db00::1")
	pinger.EXPECT().Ping(gomock.Any(), want).Return(pdelay.Results{{Responder: want}}, nil)

	res, err := pdelay.FetchPing(t.Context(), mustPingServer(t, pinger), want.String(), 5*time.Second)
	require.NoError(t, err)
	require.Len(t, res, 1)
}

// POST must not fall through to the root stats handler
func TestPingHandlerRejectsPost(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// no EXPECT: the mock fails the test if Ping is called at all
	url, _ := pingTestServer(t, NewMockPinger(ctrl))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/ping?target=ff02::6b", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// oncall sees probe activity through the existing counters endpoint
func TestPingHandlerCounters(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pinger := NewMockPinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(pdelay.Results{}, nil)
	pinger.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(nil, errors.New("send failed"))

	url, stats := pingTestServer(t, pinger)

	// one success, one send failure, one rejected multicast group
	_, err := pdelay.FetchPing(t.Context(), url, ptp.PDelayMulticastIPv6, 5*time.Second)
	require.NoError(t, err)
	_, err = pdelay.FetchPing(t.Context(), url, ptp.PDelayMulticastIPv6, 5*time.Second)
	require.Error(t, err)
	_, err = pdelay.FetchPing(t.Context(), url, "ff02::1", 5*time.Second)
	require.Error(t, err)

	// a refused method never reaches the pinger, so it counts as rejected
	resp, err := httpDo(t, http.MethodPost, url+"/ping?target=ff02::6b")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

	// the three counters partition every request exactly once
	counters := stats.GetCounters()
	require.Equal(t, int64(1), counters["ptp.sptp.ping.requests"], "only the completed probe")
	require.Equal(t, int64(1), counters["ptp.sptp.ping.errors"], "the send failure")
	require.Equal(t, int64(2), counters["ptp.sptp.ping.rejected"], "foreign group and bad method")
}

func TestPingHandlerSendFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pinger := NewMockPinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(nil, errors.New("send failed"))

	url, _ := pingTestServer(t, pinger)
	resp, err := httpGet(t, url+"/ping?target=ff02::6b")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestPingHandlerUnavailable(t *testing.T) {
	url, _ := pingTestServer(t, nil)
	resp, err := httpGet(t, url+"/ping?target=ff02::6b")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

// the server's 1s WriteTimeout must not truncate a reply that waits for responses
func TestPingHandlerOutlivesWriteTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pinger := NewMockPinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ netip.Addr) (pdelay.Results, error) {
			time.Sleep(1200 * time.Millisecond)
			return pdelay.Results{{Responder: netip.MustParseAddr("2401:db00::1")}}, nil
		})

	url, _ := pingTestServer(t, pinger)
	res, err := pdelay.FetchPing(t.Context(), url, "ff02::6b", 5*time.Second)
	require.NoError(t, err)
	require.Len(t, res, 1)
}

func mustPingServer(t *testing.T, pinger Pinger) string {
	t.Helper()
	url, _ := pingTestServer(t, pinger)
	return url
}

func TestStartBindsConfiguredHostOnly(t *testing.T) {
	port, err := getFreePort()
	require.NoError(t, err)
	stats, err := NewJSONStats()
	require.NoError(t, err)
	go stats.Start("::1", port, time.Minute, nil)

	v6 := fmt.Sprintf("http://[::1]:%d/", port)
	require.Eventually(t, func() bool {
		resp, err := httpGet(t, v6)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return true
	}, 5*time.Second, 10*time.Millisecond, "an ::1 bind must serve ::1")

	_, err = httpGet(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
	require.Error(t, err, "IPv4 loopback must not reach an ::1 bind")
}

// the default bind must serve ::1 and refuse IPv4, which is what keeps /ping
// off-box; consumers moved to [::1] in the preceding diff
func TestStartDefaultHostIsLoopbackOnly(t *testing.T) {
	port, err := getFreePort()
	require.NoError(t, err)
	stats, err := NewJSONStats()
	require.NoError(t, err)
	go stats.Start(DefaultConfig().MonitoringHost, port, time.Minute, nil)

	require.Eventually(t, func() bool {
		resp, err := httpGet(t, fmt.Sprintf("http://[::1]:%d/", port))
		if err != nil {
			return false
		}
		resp.Body.Close()
		return true
	}, 5*time.Second, 10*time.Millisecond, "default must serve ::1")

	_, err = httpGet(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
	require.Error(t, err, "default must not be reachable on IPv4")
}

// the default keeps the server on-box; an empty host must not fall through to
// net.JoinHostPort's wildcard bind
func TestStartHostDefaulting(t *testing.T) {
	require.Equal(t, "::1", DefaultConfig().MonitoringHost)

	for _, host := range []string{"", "::1"} {
		port, err := getFreePort()
		require.NoError(t, err)
		stats, err := NewJSONStats()
		require.NoError(t, err)
		go stats.Start(host, port, time.Minute, nil)

		require.Eventually(t, func() bool {
			resp, err := httpGet(t, fmt.Sprintf("http://[::1]:%d/", port))
			if err != nil {
				return false
			}
			resp.Body.Close()
			return true
		}, 5*time.Second, 10*time.Millisecond, "host %q must serve ::1", host)

		_, err = httpGet(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
		require.Error(t, err, "host %q must not bind the wildcard", host)
	}
}
