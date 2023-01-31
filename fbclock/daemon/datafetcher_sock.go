package daemon

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	log "github.com/sirupsen/logrus"
)

// SockFetcher provides data fetcher implementation using ptp4l Sock
type SockFetcher struct {
	DataFetcher
}

// FetchGMs fetches gm data from ptp4l socket
func (sf *SockFetcher) FetchGMs(cfg *Config) (targets []string, err error) {
	local := filepath.Join("/var/run/", fmt.Sprintf("fbclock.%d.linear.sock", os.Getpid()))
	timeout := cfg.Interval / 2
	conn, err := connect(cfg.PTPClientAddress, local, timeout)
	defer func() {
		if conn != nil {
			conn.Close()
			if f, err := conn.File(); err == nil {
				f.Close()
			}
		}
		// make sure there is no leftover socket
		os.RemoveAll(local)
	}()
	if err != nil {
		return targets, fmt.Errorf("failed to connect to ptp4l: %w", err)
	}

	c := &ptp.MgmtClient{
		Connection: conn,
	}
	tlv, err := c.UnicastMasterTableNP()
	if err != nil {
		return targets, fmt.Errorf("getting UNICAST_MASTER_TABLE_NP from ptp4l: %w", err)
	}

	for _, entry := range tlv.UnicastMasterTable.UnicastMasters {
		// skip the current best master
		if entry.Selected {
			continue
		}
		// skip GMs we didn't get announce from
		if entry.PortState == ptp.UnicastMasterStateWait {
			continue
		}
		server := entry.Address.String()
		targets = append(targets, server)
	}
	return
}

// FetchStats fetches stats from ptp4l socket
func (sf *SockFetcher) FetchStats(cfg *Config) (*DataPoint, error) {
	local := filepath.Join("/var/run/", fmt.Sprintf("fbclock.%d.sock", os.Getpid()))
	timeout := cfg.Interval / 2
	conn, err := connect(cfg.PTPClientAddress, local, timeout)
	defer func() {
		if conn != nil {
			conn.Close()
			if f, err := conn.File(); err == nil {
				f.Close()
			}
		}
		// make sure there is no leftover socket
		os.RemoveAll(local)
	}()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ptp4l: %w", err)
	}

	c := &ptp.MgmtClient{
		Connection: conn,
	}
	status, err := c.TimeStatusNP()
	if err != nil {
		return nil, fmt.Errorf("failed to get TIME_STATUS_NP: %w", err)
	}
	log.Debugf("TIME_STATUS_NP: %+v", status)

	pds, err := c.ParentDataSet()
	if err != nil {
		return nil, fmt.Errorf("failed to get PARENT_DATA_SET: %w", err)
	}
	cds, err := c.CurrentDataSet()
	if err != nil {
		return nil, fmt.Errorf("failed to get CURRENT_DATA_SET: %w", err)
	}
	accuracyNS := pds.GrandmasterClockQuality.ClockAccuracy.Duration().Nanoseconds()

	return &DataPoint{
		IngressTimeNS:   status.IngressTimeNS,
		MasterOffsetNS:  float64(status.MasterOffsetNS),
		PathDelayNS:     cds.MeanPathDelay.Nanoseconds(),
		ClockAccuracyNS: float64(int64(status.GMPresent) * accuracyNS),
	}, nil
}

// connect creates connection to unix socket in unixgram mode
func connect(address, local string, timeout time.Duration) (*net.UnixConn, error) {
	deadline := time.Now().Add(timeout)

	addr, err := net.ResolveUnixAddr("unixgram", address)
	if err != nil {
		return nil, err
	}
	localAddr, _ := net.ResolveUnixAddr("unixgram", local)
	conn, err := net.DialUnix("unixgram", localAddr, addr)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(local, 0666); err != nil {
		return nil, err
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	log.Debugf("connected to %s", address)
	return conn, nil
}
