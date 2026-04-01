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

package caliper

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"math"
	"strconv"
	"time"
)

const (
	svgWidth      = 1600
	svgHeight     = 920
	svgMarginL    = 100
	svgMarginR    = 40
	svgMarginT    = 60
	svgMarginB    = 200
	svgPlotWidth  = svgWidth - svgMarginL - svgMarginR
	svgPlotHeight = svgHeight - svgMarginT - svgMarginB
)

// GenerateSVG creates an SVG plot of the full OTDR trace with peaks annotated
func GenerateSVG(tor *TORFile, peaks []Peak, result *MeasurementResult, w io.Writer) error {
	return generateSVG(tor, peaks, result, w, 0, 0)
}

// GenerateSVGZoomed creates an SVG plot of the OTDR trace zoomed to the first
// maxTimeNs nanoseconds, with peaks annotated.
func GenerateSVGZoomed(tor *TORFile, peaks []Peak, result *MeasurementResult, w io.Writer, maxTimeNs float64) error {
	return generateSVG(tor, peaks, result, w, 0, maxTimeNs)
}

// GenerateSVGWindow creates an SVG plot of the OTDR trace for a specific time
// window (minTimeNs to maxTimeNs), with peaks annotated.
func GenerateSVGWindow(
	tor *TORFile, peaks []Peak, result *MeasurementResult, w io.Writer, minTimeNs, maxTimeNs float64,
) error {
	return generateSVG(tor, peaks, result, w, minTimeNs, maxTimeNs)
}

func generateSVG(
	tor *TORFile, peaks []Peak, result *MeasurementResult, w io.Writer, minTimeNs, maxTimeNs float64,
) error {
	if tor == nil {
		return fmt.Errorf("tor must not be nil")
	}
	if result == nil {
		return fmt.Errorf("result must not be nil")
	}
	if len(tor.DataPoints) == 0 {
		return fmt.Errorf("no data points to plot")
	}

	ri := tor.RefractiveIndex

	// Filter data points if zoomed
	points := tor.DataPoints
	if minTimeNs > 0 || maxTimeNs > 0 {
		points = make([]DataPoint, 0, len(tor.DataPoints))
		for _, dp := range tor.DataPoints {
			t := dp.TimeNs(ri)
			if minTimeNs > 0 && t < minTimeNs {
				continue
			}
			if maxTimeNs > 0 && t > maxTimeNs {
				continue
			}
			points = append(points, dp)
		}
		if len(points) == 0 {
			return fmt.Errorf("no data points within %.0f-%.0f ns window", minTimeNs, maxTimeNs)
		}
	}

	// Filter peaks to visible range
	visiblePeaks := peaks
	if minTimeNs > 0 || maxTimeNs > 0 {
		visiblePeaks = nil
		for _, p := range peaks {
			if minTimeNs > 0 && p.TimeNs < minTimeNs {
				continue
			}
			if maxTimeNs > 0 && p.TimeNs > maxTimeNs {
				continue
			}
			visiblePeaks = append(visiblePeaks, p)
		}
	}

	// Compute axis ranges
	minT := math.Inf(1)
	maxT := math.Inf(-1)
	minA := math.Inf(1)
	maxA := math.Inf(-1)
	for _, dp := range points {
		t := dp.TimeNs(ri)
		if t < minT {
			minT = t
		}
		if t > maxT {
			maxT = t
		}
		if dp.AmplitudeDB < minA {
			minA = dp.AmplitudeDB
		}
		if dp.AmplitudeDB > maxA {
			maxA = dp.AmplitudeDB
		}
	}
	ampRange := maxA - minA
	if ampRange == 0 {
		ampRange = 1.0
	}
	minA -= ampRange * 0.05
	maxA += ampRange * 0.05

	timeRange := maxT - minT
	if timeRange == 0 {
		timeRange = 1.0
	}
	ampRange = maxA - minA

	scaleX := func(tNs float64) float64 {
		return svgMarginL + (tNs-minT)/timeRange*svgPlotWidth
	}
	scaleY := func(aDB float64) float64 {
		return svgMarginT + (1-(aDB-minA)/ampRange)*svgPlotHeight
	}

	var buf bytes.Buffer
	buf.Grow(len(points) * 20)

	fmt.Fprintf(&buf, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`,
		svgWidth, svgHeight, svgWidth, svgHeight)
	buf.WriteString("\n")

	fmt.Fprintf(&buf, `<rect width="%d" height="%d" fill="white"/>`, svgWidth, svgHeight)
	buf.WriteString("\n")

	title := "Caliper: OTDR Trace Data"
	if minTimeNs > 0 && maxTimeNs > 0 {
		title = fmt.Sprintf("Caliper: OTDR Trace Data (%.0f-%.0f ns)", minTimeNs, maxTimeNs)
	} else if maxTimeNs > 0 {
		title = fmt.Sprintf("Caliper: OTDR Trace Data (first %.0f ns)", maxTimeNs)
	}
	fmt.Fprintf(&buf,
		`<text x="%d" y="30" text-anchor="middle" font-family="monospace" font-size="16" font-weight="bold">%s</text>`,
		svgWidth/2, html.EscapeString(title))
	buf.WriteString("\n")

	fmt.Fprintf(&buf,
		`<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="#ccc" stroke-width="1"/>`,
		svgMarginL, svgMarginT, svgPlotWidth, svgPlotHeight)
	buf.WriteString("\n")

	// Grid lines and axis labels - X axis
	xTicks := niceTicksFloat(minT, maxT, 10)
	for _, t := range xTicks {
		x := scaleX(t)
		fmt.Fprintf(&buf, `<line x1="%.1f" y1="%d" x2="%.1f" y2="%d" stroke="#eee" stroke-width="1"/>`,
			x, svgMarginT, x, svgMarginT+svgPlotHeight)
		fmt.Fprintf(&buf,
			`<text x="%.1f" y="%d" text-anchor="middle" font-family="monospace" font-size="11">%.1f</text>`,
			x, svgHeight-svgMarginB+20, t)
		buf.WriteString("\n")
	}

	fmt.Fprintf(&buf,
		`<text x="%d" y="%d" text-anchor="middle" font-family="monospace" font-size="13">Time (ns)</text>`,
		svgMarginL+svgPlotWidth/2, svgHeight-10)
	buf.WriteString("\n")

	// Grid lines and axis labels - Y axis
	yTicks := niceTicksFloat(minA, maxA, 8)
	for _, a := range yTicks {
		y := scaleY(a)
		fmt.Fprintf(&buf, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#eee" stroke-width="1"/>`,
			svgMarginL, y, svgMarginL+svgPlotWidth, y)
		fmt.Fprintf(&buf,
			`<text x="%d" y="%.1f" text-anchor="end" font-family="monospace" font-size="11" dominant-baseline="middle">%.1f</text>`,
			svgMarginL-8, y, a)
		buf.WriteString("\n")
	}

	fmt.Fprintf(&buf,
		`<text x="15" y="%d" text-anchor="middle" font-family="monospace" font-size="13" transform="rotate(-90, 15, %d)">Amplitude (dB)</text>`,
		svgMarginT+svgPlotHeight/2, svgMarginT+svgPlotHeight/2)
	buf.WriteString("\n")

	// Trace path
	buf.WriteString(`<path d="`)
	fbuf := make([]byte, 0, 32)
	for i, dp := range points {
		x := scaleX(dp.TimeNs(ri))
		y := scaleY(dp.AmplitudeDB)
		if i == 0 {
			buf.WriteByte('M')
		} else {
			buf.WriteString(" L")
		}
		fbuf = strconv.AppendFloat(fbuf[:0], x, 'f', 1, 64)
		buf.Write(fbuf)
		buf.WriteByte(',')
		fbuf = strconv.AppendFloat(fbuf[:0], y, 'f', 1, 64)
		buf.Write(fbuf)
	}
	buf.WriteString(`" fill="none" stroke="#2563eb" stroke-width="1"/>`)
	buf.WriteString("\n")

	// Peak annotations
	colors := []string{"#dc2626", "#16a34a", "#d97706", "#9333ea"}
	labelYOffsets := []int{0, 16, 0, 16}
	for i, p := range visiblePeaks {
		x := scaleX(p.TimeNs)
		y := scaleY(p.AmplitudeDB)
		color := colors[i%len(colors)]

		fmt.Fprintf(&buf,
			`<line x1="%.1f" y1="%d" x2="%.1f" y2="%d" stroke="%s" stroke-width="1" stroke-dasharray="4,3"/>`,
			x, svgMarginT, x, svgMarginT+svgPlotHeight, color)

		fmt.Fprintf(&buf, `<circle cx="%.1f" cy="%.1f" r="5" fill="%s"/>`, x, y, color)

		labelY := svgMarginT + 15 + labelYOffsets[i%len(labelYOffsets)]
		fmt.Fprintf(&buf,
			`<text x="%.1f" y="%d" font-family="monospace" font-size="11" font-weight="bold" fill="%s">%s %.2f ns</text>`,
			x+6, labelY, color, html.EscapeString(p.Label), p.TimeNs)
		buf.WriteString("\n")
	}

	// Info block below the graph
	infoY := svgMarginT + svgPlotHeight + 55
	lineHeight := 18
	infoLines := []string{
		fmt.Sprintf("Device Name: %s", result.DeviceName),
		fmt.Sprintf("Device Model: %s", result.Model),
		fmt.Sprintf("Antenna Generation: %s", result.AntennaGen),
		fmt.Sprintf("Serial Number: %s", result.SerialNumber),
		fmt.Sprintf("Capture Date/Time: %s", tor.DateTime.Format("2006-01-02 15:04:05 UTC")),
		fmt.Sprintf("Parsed Date/Time: %s", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")),
		fmt.Sprintf("Resolution: %.4f m", tor.Resolution),
		fmt.Sprintf("Wavelength: %d nm", tor.Wavelength),
		fmt.Sprintf("Backscatter Coefficient: %.2f dB", tor.BackscatterCoefficient),
		fmt.Sprintf("Refractive Index: %.4f", tor.RefractiveIndex),
	}
	for i, line := range infoLines {
		fmt.Fprintf(&buf, `<text x="%d" y="%d" font-family="monospace" font-size="12" fill="#333">%s</text>`,
			svgMarginL, infoY+i*lineHeight, html.EscapeString(line))
		buf.WriteString("\n")
	}

	buf.WriteString("</svg>\n")

	_, err := w.Write(buf.Bytes())
	return err
}

func niceTicksFloat(lo, hi float64, count int) []float64 {
	if lo == hi {
		return []float64{lo}
	}
	rawStep := (hi - lo) / float64(count)
	magnitude := math.Pow(10, math.Floor(math.Log10(rawStep)))
	normalized := rawStep / magnitude

	var step float64
	switch {
	case normalized <= 1.5:
		step = magnitude
	case normalized <= 3.5:
		step = 2 * magnitude
	case normalized <= 7.5:
		step = 5 * magnitude
	default:
		step = 10 * magnitude
	}

	start := math.Ceil(lo/step) * step
	var ticks []float64
	for v := start; v <= hi && len(ticks) < count*2; v += step {
		ticks = append(ticks, v)
	}
	return ticks
}
