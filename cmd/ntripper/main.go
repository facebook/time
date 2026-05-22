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

// ntripper reads RTCM3 correction data from an oscillatord Unix socket and
// pushes it to an NTRIP caster. It supports connecting through an HTTP
// CONNECT proxy with TLS client certificate authentication.
//
// Usage:
//
//	ntripper -caster caster.example.com:2101 -mountpoint /MOUNT01 -password secret
//	ntripper -caster caster.example.com:2101 -mountpoint /MOUNT01 -password secret \
//	         -proxy proxy.example.com:8082 -proxy-cert /path/to/cert.pem
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/facebook/time/ntrip"
	"github.com/facebook/time/ntripper/stats"
	"github.com/facebook/time/rtcm"
)

type config struct {
	socket            string
	caster            string
	mountpoint        string
	password          string
	username          string
	userAgent         string
	proxy             string
	proxyCert         string
	proxyKey          string
	reconnectInterval time.Duration
	monitoringPort    int
	logLevel          string
	dryRun            bool
}

func main() {
	var cfg config

	flag.StringVar(&cfg.socket, "socket", "/run/oscillatord/rtcm.sock",
		"path to the oscillatord RTCM Unix socket")
	flag.StringVar(&cfg.caster, "caster", "",
		"NTRIP caster address (host:port)")
	flag.StringVar(&cfg.mountpoint, "mountpoint", "",
		"NTRIP caster mountpoint (e.g., /MOUNT01)")
	flag.StringVar(&cfg.password, "password", "",
		"NTRIP SOURCE password")
	flag.StringVar(&cfg.username, "username", "",
		"NTRIP username (optional)")
	flag.StringVar(&cfg.userAgent, "useragent", "NTRIP rtcm/1.0",
		"NTRIP source agent string")
	flag.StringVar(&cfg.proxy, "proxy", "",
		"HTTP CONNECT proxy address (host:port)")
	flag.StringVar(&cfg.proxyCert, "proxy-cert", "",
		"PEM certificate for proxy TLS authentication")
	flag.StringVar(&cfg.proxyKey, "proxy-key", "",
		"PEM private key for proxy TLS authentication (defaults to proxy-cert)")
	flag.DurationVar(&cfg.reconnectInterval, "reconnect-interval", 5*time.Second,
		"delay between reconnection attempts")
	flag.IntVar(&cfg.monitoringPort, "monitoring-port", 8891,
		"port for JSON monitoring HTTP server (0 to disable)")
	flag.StringVar(&cfg.logLevel, "log-level", "info",
		"log level (debug, info, warn, error)")
	flag.BoolVar(&cfg.dryRun, "dry-run", false,
		"read frames from socket and print to stdout instead of pushing to caster")
	flag.Parse()

	if !cfg.dryRun && (cfg.caster == "" || cfg.mountpoint == "" || cfg.password == "") {
		fmt.Fprintln(os.Stderr, "required flags: -caster, -mountpoint, -password")
		flag.Usage()
		os.Exit(1)
	}
	if cfg.proxyKey == "" {
		cfg.proxyKey = cfg.proxyCert
	}

	logger := setupLogger(cfg.logLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	st := stats.NewJSONStats()
	if cfg.monitoringPort > 0 {
		go st.Start(cfg.monitoringPort)
	}

	run(ctx, cfg, logger, st)
}

func setupLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

func run(ctx context.Context, cfg config, logger *slog.Logger, st *stats.JSONStats) {
	for ctx.Err() == nil {
		err := runOnce(ctx, cfg, logger, st)
		if err == nil || ctx.Err() != nil {
			return
		}

		if errors.Is(err, os.ErrNotExist) {
			logger.Error("fatal error", "error", err)
			os.Exit(1)
		}

		st.SetConnected(0)
		st.IncReconnects()

		logger.Warn("connection error, reconnecting",
			"error", err,
			"interval", cfg.reconnectInterval,
		)
		if !sleep(ctx, cfg.reconnectInterval) {
			return
		}
	}
}

// runOnce connects to the socket and caster, then streams data until an error
// occurs or the context is cancelled.
func runOnce(ctx context.Context, cfg config, logger *slog.Logger, st *stats.JSONStats) error {
	sockConn, err := connectSocket(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("socket: %w", err)
	}
	defer sockConn.Close()

	st.SetConnected(1)

	if cfg.dryRun {
		return printFrames(ctx, sockConn, logger, st)
	}

	client, err := connectCaster(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("caster: %w", err)
	}
	defer client.Close()

	return streamFrames(ctx, sockConn, client, logger, st)
}

func connectSocket(ctx context.Context, cfg config, logger *slog.Logger) (net.Conn, error) {
	logger.Info("connecting to RTCM socket", "path", cfg.socket)
	if _, err := os.Stat(cfg.socket); err != nil {
		return nil, fmt.Errorf("socket %s: %w", cfg.socket, err)
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", cfg.socket)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", cfg.socket, err)
	}
	logger.Info("connected to RTCM socket", "path", cfg.socket)
	return conn, nil
}

func connectCaster(ctx context.Context, cfg config, logger *slog.Logger) (*ntrip.Client, error) {
	ntripCfg := ntrip.Config{
		Caster:     cfg.caster,
		Mountpoint: cfg.mountpoint,
		Password:   cfg.password,
		Username:   cfg.username,
		UserAgent:  cfg.userAgent,
	}

	opts := []ntrip.Option{
		ntrip.WithLogger(logger),
	}

	if cfg.proxy != "" {
		opts = append(opts, ntrip.WithProxy(ntrip.ProxyConfig{
			Address:  cfg.proxy,
			CertFile: cfg.proxyCert,
			KeyFile:  cfg.proxyKey,
		}))
	}

	client := ntrip.NewClient(ntripCfg, opts...)
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

func streamFrames(
	ctx context.Context,
	sockConn net.Conn,
	client *ntrip.Client,
	logger *slog.Logger,
	st *stats.JSONStats,
) error {
	scanner := rtcm.NewScanner(sockConn)
	var frameCount uint64

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		frame := scanner.Frame()
		st.IncFramesReceived()

		if _, err := client.Write(frame.Raw); err != nil {
			return fmt.Errorf("writing to caster: %w", err)
		}

		frameCount++
		if frameCount%100 == 0 {
			logger.Debug("frames forwarded", "count", frameCount)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading from socket: %w", err)
	}

	return fmt.Errorf("socket closed (EOF)")
}

func printFrames(ctx context.Context, sockConn net.Conn, logger *slog.Logger, st *stats.JSONStats) error {
	logger.Info("dry-run mode: printing frames to stdout")
	scanner := rtcm.NewScanner(sockConn)
	var frameCount uint64

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		frame := scanner.Frame()
		frameCount++
		st.IncFramesReceived()

		fmt.Printf("frame=%d type=%d len=%d\n", frameCount, frame.MessageType, len(frame.Raw))
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading from socket: %w", err)
	}
	return fmt.Errorf("socket closed (EOF)")
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
