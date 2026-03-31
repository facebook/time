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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/facebook/time/caliper"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	serialRegexp = regexp.MustCompile(`^PF\d{6}$`)
	nameRegexp   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
)

var (
	serial            string
	name              string
	model             string
	antennaGen        string
	coaxCableLength   float64
	launchCableLength float64
	outputDir         string
	logLevel          string
)

var rootCmd = &cobra.Command{
	Use:   "caliper <tor-file>",
	Short: "Calculate end-to-end latency from GPS antenna to GNSS receiver",
	Long: `Caliper parses Luciol LOR-220 OTDR .tor files, auto-detects reflective peaks
(OA, OB, OC, OD), computes delays between them, generates SVG plots, and
writes measurement data as a JSON file.`,
	Args: cobra.ExactArgs(1),
	Run:  run,
}

func init() {
	rootCmd.Flags().StringVar(&serial, "serial", "", "serial number of the Huber-Suhner GNSS receiver (required)")
	rootCmd.Flags().StringVar(&name, "name", "", "device name (required)")
	rootCmd.Flags().StringVar(&model, "model", string(caliper.GNSSoF16RxE),
		"receiver model: GNSSoF16-RxE or GNSSPoF16-4RxE")
	rootCmd.Flags().StringVar(&antennaGen, "antenna-gen", string(caliper.Gen2Phase2),
		"antenna generation: gen2-p0, gen2-p1, gen2-p2, gen2a-p2")
	rootCmd.Flags().Float64Var(&coaxCableLength, "coax-cable-length", 0,
		"length in meters of the RG58 coax cable at the SMA port")
	rootCmd.Flags().Float64Var(&launchCableLength, "launch-cable-length", 3.0,
		"length in meters of the launch cable (peaks within this distance are ignored)")
	rootCmd.Flags().StringVarP(&outputDir, "output-dir", "o", ".",
		"directory to write output files")
	rootCmd.Flags().StringVar(&logLevel, "loglevel", "info", "log level: debug, info, warning, error")

	_ = rootCmd.MarkFlagRequired("serial")
	_ = rootCmd.MarkFlagRequired("name")

	_ = rootCmd.RegisterFlagCompletionFunc("model", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{
			string(caliper.GNSSoF16RxE) + "\tQ-ODC-12 connector model",
			string(caliper.GNSSPoF164RxE) + "\tLC/FC APC connector model",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	_ = rootCmd.RegisterFlagCompletionFunc("antenna-gen", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{
			string(caliper.Gen2Phase0) + "\tGen2 Phase 0 (20.5ns electrical delay)",
			string(caliper.Gen2Phase1) + "\tGen2 Phase 1 (20.5ns electrical delay)",
			string(caliper.Gen2Phase2) + "\tGen2 Phase 2 (39.3ns electrical delay)",
			string(caliper.Gen2aPhase2) + "\tGen2a Phase 2 (39.3ns electrical delay)",
		}, cobra.ShellCompDirectiveNoFileComp
	})
}

func setLogLevel() {
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
}

func run(_ *cobra.Command, args []string) {
	setLogLevel()

	torPath := args[0]

	if !nameRegexp.MatchString(name) {
		log.Fatalf(
			"--name must contain only alphanumeric characters, dots, hyphens, and underscores, got %q", name,
		)
	}

	serial = strings.ToUpper(serial)
	if !serialRegexp.MatchString(serial) {
		log.Fatalf("--serial must match format PF followed by 6 digits (e.g. PF000142), got %q", serial)
	}

	receiverModel := caliper.ReceiverModel(model)
	switch receiverModel {
	case caliper.GNSSoF16RxE, caliper.GNSSPoF164RxE:
	default:
		log.Fatalf("--model must be one of: %s, %s", caliper.GNSSoF16RxE, caliper.GNSSPoF164RxE)
	}

	antGen := caliper.AntennaGen(antennaGen)
	switch antGen {
	case caliper.Gen2Phase0, caliper.Gen2Phase1, caliper.Gen2Phase2, caliper.Gen2aPhase2:
	default:
		log.Fatalf("--antenna-gen must be one of: %s, %s, %s, %s",
			caliper.Gen2Phase0, caliper.Gen2Phase1, caliper.Gen2Phase2, caliper.Gen2aPhase2)
	}

	// Parse the TOR file
	log.Infof("Parsing TOR file: %s", torPath)
	tor, err := caliper.ParseTOR(torPath)
	if err != nil {
		log.Fatalf("Failed to parse TOR file: %v", err)
	}
	log.Infof("Parsed %d data points, refractive index: %.4f", len(tor.DataPoints), tor.RefractiveIndex)

	// Detect peaks
	log.Info("Detecting peaks...")
	peaks, err := caliper.DetectPeaks(tor, launchCableLength)
	if err != nil {
		log.Fatalf("Failed to detect peaks: %v", err)
	}
	for _, p := range peaks {
		log.Infof("  %s: distance=%.3f m, amplitude=%.3f dB, time=%.3f ns",
			p.Label, p.DistanceM, p.AmplitudeDB, p.TimeNs)
	}

	// Compute result
	result, err := caliper.ComputeResult(
		tor, peaks, name, serial, receiverModel, antGen, coaxCableLength, launchCableLength,
	)
	if err != nil {
		log.Fatalf("Failed to compute result: %v", err)
	}

	log.Infof("Delays:")
	log.Infof("  SMA port offset (FO Out-SMA):   %.3f ns", result.Delays.SMAPortOffsetNs)
	log.Infof("  RX delay (OA-OB):              %.3f ns", result.Delays.RxDelayNs)
	log.Infof("  Cable length (OB-OC):            %.3f m", peaks[2].DistanceM-peaks[1].DistanceM)
	log.Infof("  Cable delay (OB-OC):            %.3f ns", result.Delays.CableDelayNs)
	log.Infof("  Antenna optical delay (OC-OD):  %.3f ns", result.Delays.AntennaOpticalDelayNs)
	log.Infof("  Antenna electrical delay:        %.3f ns", result.Delays.AntennaElectricalDelayNs)
	log.Infof("  System total (antenna-SMA):      %.3f ns", result.Delays.TotalDelayNs)
	log.Infof("  Coax cable (%.3f m RG58):       %.3f ns", result.Delays.CoaxCableLengthM, result.Delays.CoaxCableDelayNs)
	log.Infof("  End-to-end delay:                %.3f ns", result.Delays.EndToEndDelayNs)

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Write JSON
	jsonPath := filepath.Join(outputDir, "caliper_"+name+".json")
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}
	jsonData = append(jsonData, '\n')
	if err := os.WriteFile(jsonPath, jsonData, 0600); err != nil {
		log.Fatalf("Failed to write JSON: %v", err)
	}
	log.Infof("JSON written to %s", jsonPath)

	// Generate and write full SVG
	svgFullPath := filepath.Join(outputDir, "caliper_"+name+".svg")
	if err := writeSVG(svgFullPath, func(buf *bytes.Buffer) error {
		return caliper.GenerateSVG(tor, peaks, result, buf)
	}); err != nil {
		log.Fatalf("Failed to write full SVG: %v", err)
	}
	log.Infof("Full SVG written to %s", svgFullPath)

	// Generate and write zoomed SVG (first 50 ns)
	svgZoomedPath := filepath.Join(outputDir, "caliper_"+name+"_50ns.svg")
	if err := writeSVG(svgZoomedPath, func(buf *bytes.Buffer) error {
		return caliper.GenerateSVGZoomed(tor, peaks, result, buf, 50)
	}); err != nil {
		log.Fatalf("Failed to write zoomed SVG: %v", err)
	}
	log.Infof("Zoomed SVG written to %s", svgZoomedPath)

	// Generate and write cable-end SVG (8m before OC to 2m after OD)
	if len(peaks) >= 4 {
		ri := tor.RefractiveIndex
		ocTimeNs := peaks[2].TimeNs
		odTimeNs := peaks[3].TimeNs
		marginBeforeNs := 8.0 * ri / caliper.SpeedOfLight * 1e9
		marginAfterNs := 2.0 * ri / caliper.SpeedOfLight * 1e9
		cableEndMinNs := ocTimeNs - marginBeforeNs
		cableEndMaxNs := odTimeNs + marginAfterNs

		svgCableEndPath := filepath.Join(outputDir, "caliper_"+name+"_cable_end.svg")
		if err := writeSVG(svgCableEndPath, func(buf *bytes.Buffer) error {
			return caliper.GenerateSVGWindow(tor, peaks, result, buf, cableEndMinNs, cableEndMaxNs)
		}); err != nil {
			log.Fatalf("Failed to write cable-end SVG: %v", err)
		}
		log.Infof("Cable-end SVG written to %s", svgCableEndPath)
	}
}

func writeSVG(path string, generate func(*bytes.Buffer) error) error {
	var buf bytes.Buffer
	if err := generate(&buf); err != nil {
		return fmt.Errorf("generating SVG %s: %w", path, err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("writing SVG %s: %w", path, err)
	}
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
