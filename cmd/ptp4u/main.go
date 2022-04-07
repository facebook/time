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

	"github.com/facebook/time/ptp/ptp4u/drain"
	"github.com/facebook/time/ptp/ptp4u/server"
	"github.com/facebook/time/ptp/ptp4u/stats"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Set reasonable defaults for Dynamic config
	c := &server.Config{
		DynamicConfig: server.DynamicConfig{
			ClockAccuracy:  0x21,
			ClockClass:     6,
			DrainInterval:  30 * time.Second,
			MaxSubDuration: 1 * time.Hour,
			MetricInterval: 1 * time.Minute,
			MinSubInterval: 1 * time.Second,
			UTCOffset:      37 * time.Second,
		},
	}

	var ipaddr string

	flag.IntVar(&c.DSCP, "dscp", 0, "DSCP for PTP packets, valid values are between 0-63 (used by send workers)")
	flag.IntVar(&c.MonitoringPort, "monitoringport", 8888, "Port to run monitoring server on")
	flag.IntVar(&c.QueueSize, "queue", 0, "Size of the queue to send out packets")
	flag.IntVar(&c.RecvWorkers, "recvworkers", 10, "Set the number of receive workers")
	flag.IntVar(&c.SendWorkers, "workers", 100, "Set the number of send workers")
	flag.StringVar(&c.ConfigFile, "config", "", "Path to a config with dynamic settings")
	flag.StringVar(&c.DebugAddr, "pprofaddr", "", "host:port for the pprof to bind")
	flag.StringVar(&c.Interface, "iface", "eth0", "Set the interface")
	flag.StringVar(&c.LogLevel, "loglevel", "warning", "Set a log level. Can be: debug, info, warning, error")
	flag.StringVar(&c.PidFile, "pidfile", "/var/run/ptp4u.pid", "Pid file location")
	flag.StringVar(&c.TimestampType, "timestamptype", timestamp.HWTIMESTAMP, fmt.Sprintf("Timestamp type. Can be: %s, %s", timestamp.HWTIMESTAMP, timestamp.SWTIMESTAMP))
	flag.StringVar(&ipaddr, "ip", "::", "IP to bind on")
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

	if c.ConfigFile != "" {
		dc, err := server.ReadDynamicConfig(c.ConfigFile)
		if err != nil {
			log.Fatal(err)
		}
		c.DynamicConfig = *dc
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

	if c.DebugAddr != "" {
		log.Warningf("Staring profiler on %s", c.DebugAddr)
		go func() {
			log.Println(http.ListenAndServe(c.DebugAddr, nil))
		}()
	}

	log.Infof("UTC offset is: %v", c.UTCOffset)

	// Monitoring
	// Replace with your implementation of Stats
	st := stats.NewJSONStats()
	go st.Start(c.MonitoringPort)

	// drain check
	check := &drain.FileDrain{FileName: "/var/tmp/kill_ptp4u"}
	checks := []drain.Drain{check}

	s := server.Server{
		Config: c,
		Stats:  st,
		Checks: checks,
	}

	if err := s.Start(); err != nil {
		log.Fatalf("Server run failed: %v", err)
	}
}
