//go:build !darwin

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
	"syscall"
	"time"
	"unsafe"

	"github.com/spf13/cobra"
)

// cannot import sys/timex.h
const clockMonotonic = 4

func getRawMonotonic() float64 {
	var ts syscall.Timespec
	_, _, _ = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, clockMonotonic, uintptr(unsafe.Pointer(&ts)), 0)
	return float64(ts.Sec) + float64(ts.Nsec)/float64(1e9)
}

// track reports wall time / mono time difference on stdout
func track(interval time.Duration) {
	fmt.Println("Wall timestamp\t\t\tWall difference\tMono difference\tMono raw diff\tOffset mono\tOffset mono raw")
	startTime := time.Now()
	rawStart := getRawMonotonic()

	var prevWallElapsed time.Duration
	var prevMonoElapsed time.Duration
	for {
		now := time.Now()
		nowMonotonic := getRawMonotonic()

		wallElapsed := now.Truncate(0).Sub(startTime.Truncate(0))
		monoElapsed := now.Sub(startTime)
		wallElapsedS := float64(wallElapsed) / float64(time.Second)
		monoElapsedS := float64(monoElapsed) / float64(time.Second)
		monoRawElapsedS := nowMonotonic - rawStart
		offsetMonoUs := float64(wallElapsed-monoElapsed) / float64(time.Microsecond)
		offsetMonoRawUs := float64(wallElapsed)/float64(time.Microsecond) - monoRawElapsedS*float64(1e6)
		fmt.Printf("[%s]\t%.7fs\t%.7fs\t%.7fs\t%.2fus\t\t%.2fus\n", now.Format(time.RFC3339), wallElapsedS, monoElapsedS, monoRawElapsedS, offsetMonoUs, offsetMonoRawUs)

		if (prevWallElapsed != 0) && int64(wallElapsed-prevWallElapsed) < 1000*int64(time.Millisecond) {
			fmt.Println("^^^ BANG! Wall time goes back")
		}
		if (prevMonoElapsed != 0) && int64(monoElapsed-prevMonoElapsed) < 1000*int64(time.Millisecond) {
			fmt.Println("^^^ BANG! Monotonic time goes back")
		}
		prevWallElapsed = wallElapsed
		prevMonoElapsed = monoElapsed

		time.Sleep(interval)
	}
}

var trackInterval time.Duration

func init() {
	// track
	utilsCmd.AddCommand(trackCmd)
	trackCmd.Flags().DurationVarP(&trackInterval, "interval", "i", time.Second, "Measurement interval")
}

var trackCmd = &cobra.Command{
	Use:   "track",
	Short: "Allows to compare monotonic with wall clock.",
	Long: `Allows to compare monotonic with wall clock.
Legend:
  * Wall timestamp - local date and time with TZ
  * Wall difference - wall time elapsed since the start
  * Mono difference - monotonic time elapsed since the start
  * Mono raw diff - monotonic raw time elapsed since the start
  * Offset mono - offset between monotonic and wall elapsed
  * Offset mono raw - offset between monotonic raw and wall elapsed`,
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()
		track(trackInterval)
	},
}
