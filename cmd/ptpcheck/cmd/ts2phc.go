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

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/facebook/time/cmd/ptpcheck/metrics"
	"github.com/facebook/time/phc"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	srcDeviceTS2PHCFlag     string
	dstDeviceTS2PHCFlag     string
	intervalTS2PHCFlag      time.Duration
	srcPinTS2PHCFlag        uint
	dstPinTS2PHCFlag        uint
	maxFreqTS2PHCFlag       float64
	firstStepTS2PHCFlag     time.Duration
	stepThresholdTS2PHCFlag time.Duration
	monitoringPort          uint
)

func init() {
	RootCmd.AddCommand(ts2phcCmd)
	ts2phcCmd.Flags().StringVarP(&srcDeviceTS2PHCFlag, "source", "s", "/dev/ptp_tcard", "Source for Time of Day (ToD) data. Only PHC devices are supported at the moment")
	ts2phcCmd.Flags().StringVarP(&dstDeviceTS2PHCFlag, "destination", "d", "eth0", "PHC to be synchronized. The clock may be identified by its character device (like /dev/ptp0) or its associated network interface (like eth0).")
	ts2phcCmd.Flags().DurationVarP(&intervalTS2PHCFlag, "interval", "i", time.Second, "Interval between syncs in nanosseconds")
	ts2phcCmd.Flags().DurationVarP(&firstStepTS2PHCFlag, "first_step", "f", 20*time.Microsecond, "The maximum offset that the servo will correct by changing the clock frequency instead of stepping the clock. This is only applied on the first update. When set to 0, the servo will not step the clock on start.")
	ts2phcCmd.Flags().UintVarP(&srcPinTS2PHCFlag, "out-pin", "n", phc.DefaultTs2PhcIndex, "output pin number of the PPS signal on source device.")
	ts2phcCmd.Flags().UintVarP(&dstPinTS2PHCFlag, "in-pin", "o", phc.DefaultTs2PhcSinkIndex, "input pin number of the PPS signal on destination device. (default 0)")
	ts2phcCmd.Flags().Float64VarP(&maxFreqTS2PHCFlag, "max_frequency", "m", 0, "maximum frequency in parts per billion (PPB) that the servo will correct by changing the clock frequency instead of stepping the clock. If unset, uses the maximum frequency reported by the PHC device.")
	ts2phcCmd.Flags().DurationVarP(&stepThresholdTS2PHCFlag, "step_threshold", "t", 0, "The maximum offset that the servo will correct by changing the clock frequency instead of stepping the clock. When set to 0, the servo will never step the clock except on start.")
	ts2phcCmd.Flags().UintVarP(&monitoringPort, "monitoring_port", "p", 9120, "port to expose the prometheus metrics endpoint. Metrics are exposed on /metrics endpoint.")
}

func ts2phcRun(srcDevicePath string, dstDeviceName string, interval time.Duration, firstStepth time.Duration, stepth time.Duration, srcPinIndex uint, metricsHandler *metrics.Handler) error {
	ppsSource, err := getPPSSourceFromPath(srcDevicePath, srcPinIndex)
	if err != nil {
		return fmt.Errorf("error opening source phc device: %w", err)
	}
	log.Debug("PPS Source enabled")
	dstDevice, err := phcDeviceFromName(dstDeviceName)
	if err != nil {
		return fmt.Errorf("error opening target phc device: %w", err)
	}
	ppsSink, err := phc.PPSSinkFromDevice(dstDevice, dstPinTS2PHCFlag)
	if err != nil {
		return fmt.Errorf("error setting target device as PPS sink: %w", err)
	}
	log.Debug("PPS Sink enabled")
	pi, err := phc.NewPiServo(interval, firstStepth, stepth, dstDevice, maxFreqTS2PHCFlag)
	if err != nil {
		return fmt.Errorf("error getting servo: %w", err)
	}

	lastTick := time.Now()
	log.Debug("Starting sync loop")
	for {
		ppsEventTime, err := ppsSink.PollPPSSink()
		if err != nil {
			log.Errorf("Error polling PPS Sink: %v", err)
			continue
		}
		log.Debugf("PPS event at %+v", ppsEventTime.UnixNano())
		srcTimestamp, err := ppsSource.Timestamp()
		if err != nil {
			log.Errorf("Error getting source timestamp: %v", err)
			continue
		}
		log.Debugf("PPS Src Timestamp: %+v", srcTimestamp.UnixNano())
		now := time.Now()
		log.Debugf("Tick took %vms sys time to call sync", now.Sub(lastTick).Milliseconds())
		lastTick = now
		if err = phc.PPSClockSync(pi, srcTimestamp, ppsEventTime, dstDevice); err != nil {
			log.Errorf("Error syncing PHC: %v", err)
		}
		go func() {
			offset := ppsEventTime.Sub(srcTimestamp)
			metricsHandler.ObserveOffset(float64(offset))
		}()
	}
}

func getPPSSourceFromPath(srcDevicePath string, pinIndex uint) (*phc.PPSSource, error) {
	srcDeviceFile, err := os.OpenFile(srcDevicePath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("error opening source device: %w", err)
	}
	ppsSource, err := phc.ActivatePPSSource(phc.FromFile(srcDeviceFile), pinIndex)
	if err != nil {
		return nil, fmt.Errorf("error activating PPS Source for PHC: %w", err)
	}

	return ppsSource, nil
}

// phcDeviceFromName returns a PHC device from a device name, which can be either an interface name or a PHC device path
func phcDeviceFromName(dstDeviceName string) (*phc.Device, error) {
	devicePath, err := phc.IfaceToPHCDevice(dstDeviceName)
	if err != nil {
		log.Info("Provided device name is not an interface, assuming it is a PHC device path")
		devicePath = dstDeviceName
	}
	// need RW permissions to issue CLOCK_ADJTIME on the device
	dstFile, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("error opening destination device: %w", err)
	}
	dstDevice := phc.FromFile(dstFile)
	return dstDevice, nil
}

var ts2phcCmd = &cobra.Command{
	Use:   "ts2phc",
	Short: "Sync PHC with external timestamps",
	Run: func(_ *cobra.Command, _ []string) {
		var metricsHandler = &metrics.Handler{}
		ConfigureVerbosity()
		go func() {
			log.Fatalf("Metrics server error: %v", metrics.RunMetricsServer(monitoringPort, metricsHandler))
		}()
		if err := ts2phcRun(srcDeviceTS2PHCFlag, dstDeviceTS2PHCFlag, intervalTS2PHCFlag, firstStepTS2PHCFlag, stepThresholdTS2PHCFlag, srcPinTS2PHCFlag, metricsHandler); err != nil {
			log.Fatal(err)
		}
	},
}
