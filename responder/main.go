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
	"context"
	"flag"
	"fmt"
	syscall "golang.org/x/sys/unix"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"

	"github.com/facebookincubator/ntp/responder/announce"
	"github.com/facebookincubator/ntp/responder/checker"
	"github.com/facebookincubator/ntp/responder/server"
	"github.com/facebookincubator/ntp/responder/stats"
	log "github.com/sirupsen/logrus"
)

const pprofHTTP = "localhost:6060"

func main() {
	s := server.Server{}

	var (
		debugger       bool
		logLevel       string
		monitoringport int
	)

	flag.StringVar(&logLevel, "loglevel", "info", "Set a log level. Can be: debug, info, warning, error")
	flag.StringVar(&s.ListenConfig.Iface, "interface", "lo", "Interface to add IPs to")
	flag.StringVar(&s.RefID, "refid", "OLEG", "Reference ID of the server")
	flag.IntVar(&s.ListenConfig.Port, "port", 123, "Port to run service on")
	flag.IntVar(&monitoringport, "monitoringport", 0, "Port to run monitoring server on")
	flag.IntVar(&s.Stratum, "stratum", 1, "Stratum of the server")
	flag.IntVar(&s.Workers, "workers", runtime.NumCPU()*100, "How many workers (routines) to run")
	flag.Var(&s.ListenConfig.IPs, "ip", fmt.Sprintf("IP to listen to. Repeat for multiple. Default: %s", server.DefaultServerIPs))
	flag.BoolVar(&debugger, "pprof", false, "Enable pprof")
	flag.BoolVar(&s.ListenConfig.ShouldAnnounce, "announce", false, "Advertize IPs")
	flag.DurationVar(&s.ExtraOffset, "extraoffset", 0, "Extra offset to return to clients")

	flag.Parse()
	s.ListenConfig.IPs.SetDefault()

	switch logLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warning":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.Fatalf("Unrecognized log level: %v", logLevel)
	}

	if s.Workers < 1 {
		log.Fatalf("Will not start without workers")
	}

	if debugger {
		log.Warningf("Staring profiler on %s", pprofHTTP)
		go func() {
			log.Println(http.ListenAndServe(pprofHTTP, nil))
		}()
	}

	if s.ListenConfig.ShouldAnnounce {
		log.Warningf("Will announce VIPs")
	}

	// Monitoring
	// Replace with your implementation of Stats
	st := &stats.JSONStats{}
	go st.Start(monitoringport)

	// Replace with your implementation of Announce
	s.Announce = &announce.NoopAnnounce{}

	ch := &checker.SimpleChecker{
		ExpectedListeners: int64(len(s.ListenConfig.IPs)),
		ExpectedWorkers:   int64(s.Workers),
	}

	// context is used in server in case work needs to be interrupted internally
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Handle interrupt for graceful shutdown
	sigStop := make(chan os.Signal, 1)
	shutdownFinish := make(chan struct{})
	signal.Notify(sigStop, syscall.SIGINT)
	signal.Notify(sigStop, syscall.SIGQUIT)
	signal.Notify(sigStop, syscall.SIGTERM)

	s.Stats = st
	s.Checker = ch

	go func() {
		select {
		case <-sigStop:
			log.Warning("Graceful shutdown")
			s.Stop()
			close(shutdownFinish)
			return
		case <-ctx.Done():
			log.Error("Internal error shutdown")
			s.Stop()
			close(shutdownFinish)
			return
		}
	}()

	go s.Start(ctx, cancelFunc)
	<-shutdownFinish
}
