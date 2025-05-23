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

	"github.com/facebook/time/clock"
	"github.com/facebook/time/phc"
	"github.com/facebook/time/servo"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

var (
	srcDeviceFlag string
	dstDeviceFlag string
	intervalFlag  time.Duration
	stepthFlag    time.Duration
	offsetFlag    time.Duration
)

func init() {
	RootCmd.AddCommand(phc2phcCmd)
	phc2phcCmd.Flags().StringVarP(&srcDeviceFlag, "source", "s", "/dev/ptp0", "Source PHC device")
	phc2phcCmd.Flags().StringVarP(&dstDeviceFlag, "destination", "d", "/dev/ptp2", "Destination PHC device")
	phc2phcCmd.Flags().DurationVarP(&intervalFlag, "interval", "i", time.Second, "Interval between syncs. Frequency")
	phc2phcCmd.Flags().DurationVarP(&offsetFlag, "offset", "o", 0, "Specify the offset between the source and destination times")
	phc2phcCmd.Flags().DurationVarP(&stepthFlag, "step", "f", 0, "First step threshold")
}

func phc2phcRun(srcDevice string, dstDevice string, interval time.Duration, stepth time.Duration) error {
	src, err := os.Open(srcDevice)
	if err != nil {
		return fmt.Errorf("opening device %q to read frequency: %w", srcDevice, err)
	}
	defer src.Close()
	srcdev := phc.FromFile(src)
	// we need RW permissions to issue CLOCK_ADJTIME on the device, even with empty struct
	dst, err := os.OpenFile(dstDevice, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening device %q to set frequency: %w", dstDevice, err)
	}
	defer dst.Close()
	dstdev := phc.FromFile(dst)

	freq, err := dstdev.FreqPPB()
	if err != nil {
		return err
	}
	log.Debugf("starting PHC frequency: %v", freq)

	servoCfg := servo.DefaultServoConfig()
	if stepth != 0 {
		// allow stepping clock on first update
		servoCfg.FirstUpdate = true
		servoCfg.FirstStepThreshold = int64(stepth)
	}
	pi := servo.NewPiServo(servoCfg, servo.DefaultPiServoCfg(), -freq)
	pi.SyncInterval(interval.Seconds())

	maxFreq, err := dstdev.MaxFreqAdjPPB()
	if err != nil {
		log.Warningf("max PHC frequency error: %v", err)
		maxFreq = phc.DefaultMaxClockFreqPPB
	}
	pi.SetMaxFreq(maxFreq)

	log.Debugf("max PHC frequency: %v", maxFreq)

	for ; ; time.Sleep(interval) {
		var phcOffset time.Duration
		var timeAndOffsetSrc, timeAndOffsetDst phc.SysoffResult
		if preciseSrc, err := srcdev.ReadSysoffPrecise(); err != nil {
			log.Warningf("Failed to read precise offset from %v: %v", srcDevice, err)
			extendedSrc, err := srcdev.ReadSysoffExtended()
			if err != nil {
				log.Errorf("failed to read data from %v: %v", srcDevice, err)
				continue
			}
			extendedDst, err := dstdev.ReadSysoffExtended()
			if err != nil {
				log.Errorf("failed to read data from %v: %v", dstDevice, err)
				continue
			}

			phcOffset, err = extendedDst.Sub(extendedSrc)
			if err != nil {
				log.Errorf("failed to calculate offset between %v and %v: %v", srcDevice, dstDevice, err)
				continue
			}
			timeAndOffsetSrc = extendedSrc.BestSample()
			timeAndOffsetDst = extendedDst.BestSample()
		} else {
			preciseDst, err := dstdev.ReadSysoffPrecise()
			if err != nil {
				log.Errorf("failed to read data from %v: %v", srcDevice, err)
				continue
			}
			phcOffset = preciseDst.Sub(preciseSrc)
			timeAndOffsetSrc = phc.SysoffFromPrecise(preciseSrc)
			timeAndOffsetDst = phc.SysoffFromPrecise(preciseDst)
		}
		phcOffset += offsetFlag
		freqAdj, state := pi.Sample(int64(phcOffset), uint64(timeAndOffsetDst.SysTime.UnixNano()))
		log.Infof("offset %12d freq %+9.0f path delay %5d", phcOffset, freqAdj, timeAndOffsetSrc.Delay.Nanoseconds()+timeAndOffsetDst.Delay.Nanoseconds())
		if state == servo.StateJump {
			if err := dstdev.Step(-phcOffset); err != nil {
				log.Errorf("failed to step clock by %v: %v", -phcOffset, err)
				continue
			}
		} else {
			if err := dstdev.AdjFreq(-freqAdj); err != nil {
				log.Errorf("failed to adjust freq to %v: %v", -freqAdj, err)
				continue
			}
			if err := clock.SetSync(unix.CLOCK_REALTIME); err != nil {
				log.Errorf("failed to set sys clock sync state: %v", err)
			}
		}
	}
}

var phc2phcCmd = &cobra.Command{
	Use:   "phc2phc",
	Short: "Sync 2 PHCs",
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()
		if err := phc2phcRun(srcDeviceFlag, dstDeviceFlag, intervalFlag, stepthFlag); err != nil {
			log.Fatal(err)
		}
	},
}
