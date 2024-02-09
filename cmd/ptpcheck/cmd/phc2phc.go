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
)

func init() {
	RootCmd.AddCommand(phc2phcCmd)
	phc2phcCmd.Flags().StringVarP(&srcDeviceFlag, "source", "s", "/dev/ptp0", "Source PHC device")
	phc2phcCmd.Flags().StringVarP(&dstDeviceFlag, "destination", "d", "/dev/ptp2", "Destination PHC device")
	phc2phcCmd.Flags().DurationVarP(&intervalFlag, "interval", "i", time.Second, "Interval between syncs. Frequency")
	phc2phcCmd.Flags().DurationVarP(&stepthFlag, "step", "f", 0, "First step threshold")
}

func phc2phcRun(srcDevice string, dstDevice string, interval time.Duration, stepth time.Duration) error {
	freq, err := phc.FrequencyPPBFromDevice(dstDevice)
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

	maxFreq, err := phc.MaxFreqAdjPPBFromDevice(dstDevice)
	if err != nil {
		log.Warningf("max PHC frequency error: %v", err)
		maxFreq = phc.DefaultMaxClockFreqPPB
	} else {
		pi.SetMaxFreq(maxFreq)
	}
	log.Debugf("max PHC frequency: %v", maxFreq)

	for ; ; time.Sleep(interval) {
		extendedSrc, err := phc.ReadPTPSysOffsetExtended(srcDevice, phc.ExtendedNumProbes)
		if err != nil {
			log.Errorf("failed to read data from %v: %v", srcDevice, err)
			continue
		}
		extendedDst, err := phc.ReadPTPSysOffsetExtended(dstDevice, phc.ExtendedNumProbes)
		if err != nil {
			log.Errorf("failed to read data from %v: %v", dstDevice, err)
			continue
		}

		phcOffset := phc.OffsetBetweenExtendedReadings(extendedSrc, extendedDst)
		timeAndOffsetSrc := phc.SysoffEstimateExtended(extendedSrc)
		timeAndOffsetDst := phc.SysoffEstimateExtended(extendedDst)
		freqAdj, state := pi.Sample(int64(phcOffset), uint64(timeAndOffsetDst.SysTime.UnixNano()))
		log.Infof("offset %12d freq %+9.0f path delay %5d", phcOffset, freqAdj, timeAndOffsetSrc.Delay.Nanoseconds()+timeAndOffsetDst.Delay.Nanoseconds())
		if state == servo.StateJump {
			if err := phc.ClockStep(dstDevice, -phcOffset); err != nil {
				log.Errorf("failed to step clock by %v: %v", -phcOffset, err)
				continue
			}
		} else {
			if err := phc.ClockAdjFreq(dstDevice, -freqAdj); err != nil {
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
