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

package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/facebook/time/ptp/ptp4u/server"
	"github.com/facebook/time/ptp/ptp4u/stats"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
)

func main() {
	c := &server.Config{}

	var ipaddr string
	var pprofaddr string

	flag.IntVar(&c.DSCP, "dscp", 0, "DSCP for PTP packets, valid values are between 0-63 (used by send workers)")
	flag.StringVar(&ipaddr, "ip", "::", "IP to bind on")
	flag.StringVar(&pprofaddr, "pprofaddr", "", "host:port for the pprof to bind")
	flag.StringVar(&c.Interface, "iface", "eth0", "Set the interface")
	flag.StringVar(&c.LogLevel, "loglevel", "warning", "Set a log level. Can be: debug, info, warning, error")
	flag.DurationVar(&c.MinSubInterval, "minsubinterval", 1*time.Second, "Minimum interval of the sync/announce subscription messages")
	flag.DurationVar(&c.MaxSubDuration, "maxsubduration", 1*time.Hour, "Maximum sync/announce/delay_resp subscription duration")
	flag.StringVar(&c.TimestampType, "timestamptype", timestamp.HWTIMESTAMP, fmt.Sprintf("Timestamp type. Can be: %s, %s", timestamp.HWTIMESTAMP, timestamp.SWTIMESTAMP))
	flag.DurationVar(&c.UTCOffset, "utcoffset", 37*time.Second, "Set the UTC offset. Ignored if shm or leapsectz are set")
	flag.BoolVar(&c.Leapsectz, "leapsectz", false, "Leapsectz to determine UTC offset periodically")
	flag.BoolVar(&c.SHM, "shm", false, "Use Share Memory Segment to determine UTC offset periodically (leapsectz has a priority)")
	flag.IntVar(&c.SendWorkers, "workers", 100, "Set the number of send workers")
	flag.IntVar(&c.RecvWorkers, "recvworkers", 10, "Set the number of receive workers")
	flag.IntVar(&c.MonitoringPort, "monitoringport", 8888, "Port to run monitoring server on")
	flag.IntVar(&c.QueueSize, "queue", 0, "Size of the queue to send out packets")
	flag.DurationVar(&c.MetricInterval, "metricinterval", 1*time.Minute, "Interval of resetting metrics")

	flag.Parse()

	switch c.LogLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warning":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.Fatalf("Unrecognized log level: %v", c.LogLevel)
	}

	if c.DSCP < 0 || c.DSCP > 63 {
		log.Fatalf("Unsupported DSCP value %v", c.DSCP)
	}

	switch c.TimestampType {
	case timestamp.SWTIMESTAMP:
		log.Warning("Software timestamps greatly reduce the precision")
		fallthrough
	case timestamp.HWTIMESTAMP:
		log.Debugf("Using %s timestamps", c.TimestampType)
	default:
		log.Fatalf("Unrecognized timestamp type: %s", c.TimestampType)
	}

	c.IP = net.ParseIP(ipaddr)
	found, err := c.IfaceHasIP()
	if err != nil {
		log.Fatal(err)
	}
	if !found {
		log.Fatalf("IP '%s' is not found on interface '%s'", c.IP, c.Interface)
	}

	if pprofaddr != "" {
		log.Warningf("Staring profiler on %s", pprofaddr)
		go func() {
			log.Println(http.ListenAndServe(pprofaddr, nil))
		}()
	}

	if c.Leapsectz {
		if err := c.SetUTCOffsetFromLeapsectz(); err != nil {
			log.Fatalf("Failed to set UTC offset: %v", err)
		}
	} else if c.SHM {
		if err := c.SetUTCOffsetFromSHM(); err != nil {
			log.Fatalf("Failed to set UTC offset: %v", err)
		}
	}
	log.Infof("UTC offset is: %v", c.UTCOffset)

	// Monitoring
	// Replace with your implementation of Stats
	st := stats.NewJSONStats()
	go st.Start(c.MonitoringPort)

	s := server.Server{
		Config: c,
		Stats:  st,
	}

	if err := s.Start(); err != nil {
		log.Fatalf("Server run failed: %v", err)
	}
}
