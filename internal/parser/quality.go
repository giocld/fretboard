package parser

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"fretboard/internal/model"
)

// Tab-quality metadata keys written by ApplyQuality. "quality" and
// "quality_timing" are shared keys; the metric sub-keys are parser-specific
// and stored at higher precision than the composite.
const (
	metaKeyQuality      = "quality"
	metaKeyQualityTim   = "quality_timing"
	metaKeyQualityRow   = "quality_row_ratio"
	metaKeyQualityDash  = "quality_dash"
	metaKeyQualityAlign = "quality_bar_align"
)

// Timing words classifying parse-time tab quality.
const (
	TimingSolid       = "solid"
	TimingApproximate = "approximate"
	TimingSloppy      = "sloppy"
)

// Composite-score thresholds mapping a QualityResult to a TimingWord. A
// composite >= solidThreshold reads as solid, <= sloppyThreshold as sloppy,
// and anything between as approximate.
const (
	solidThreshold  = 0.85
	sloppyThreshold = 0.60
)

// QualityResult holds the parse-time quality metrics for one tab.
type QualityResult struct {
	TimingWord      string
	RowRatio        float64
	DashConsistency float64
	BarAlign        float64
}

// tabRowRegex matches the leading designator of a standard 6-string tab row:
// one of eADGBE (lowercase e = high E string, uppercase E = low E) directly
// followed by a bar pipe, a dash, or a fret digit. Header lines ("EADGBE"),
// lyrics, and chord names fail the trailing character check and are not
// counted as rows.
var tabRowRegex = regexp.MustCompile(`^\s*[eADGBE][-|0-9]`)

// ScoreTab measures parse-time quality of raw tab lines (cleaned or not;
// blank lines are ignored). RowRatio is the share of non-blank lines that
// look like 6-string tab rows; DashConsistency and BarAlign are computed
// over those rows only, so header/rhythm/lyric lines cannot drag them down.
func ScoreTab(lines []string) QualityResult {
	rows := make([]string, 0, len(lines))
	nonBlank := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		nonBlank++
		if tabRowRegex.MatchString(l) {
			rows = append(rows, l)
		}
	}
	q := QualityResult{}
	if nonBlank > 0 {
		q.RowRatio = float64(len(rows)) / float64(nonBlank)
	}
	q.DashConsistency = dashConsistency(rows)
	q.BarAlign = barAlign(rows)
	q.TimingWord = timingWord(q)
	return q
}

// dashConsistency returns the fraction of dash runs inside bar delimiters
// that span at least 2 columns. A lone single-dash gap between notes is the
// signature of hand-misaligned spacing (rows above/below are off by one
// column), so it is penalized. Runs before the first or after the last pipe
// lie outside the bar grid and are ignored, as are rows with no pipe at all.
func dashConsistency(rows []string) float64 {
	good, total := 0, 0
	for _, l := range rows {
		first := strings.IndexByte(l, '|')
		last := strings.LastIndexByte(l, '|')
		if first < 0 {
			continue
		}
		for i := 0; i < len(l); {
			if l[i] != '-' {
				i++
				continue
			}
			j := i
			for j < len(l) && l[j] == '-' {
				j++
			}
			if i > first && j <= last {
				total++
				if j-i >= 2 {
					good++
				}
			}
			i = j
		}
	}
	if total == 0 {
		return 0
	}
	return float64(good) / float64(total)
}

// barAlign returns 1 - mean variance of bar-boundary columns across rows.
// For each pipe index (1st, 2nd, ...) the column of that pipe is collected
// from every row that has it; indexes present in fewer than two rows are
// skipped so ragged trailing bars cannot skew the score. The variance of
// each boundary is averaged over all boundaries, so a one-column drift in
// a single row of six lowers the score by a few points and severe
// misalignment clamps it to 0.
func barAlign(rows []string) float64 {
	colsByIndex := map[int][]int{}
	order := []int{}
	for _, l := range rows {
		k := 0
		for i := range l {
			if l[i] != '|' {
				continue
			}
			if _, seen := colsByIndex[k]; !seen {
				order = append(order, k)
			}
			colsByIndex[k] = append(colsByIndex[k], i)
			k++
		}
	}
	if len(order) == 0 {
		return 0
	}
	sum, count := 0.0, 0
	for _, k := range order {
		cols := colsByIndex[k]
		if len(cols) < 2 {
			continue
		}
		mean := 0.0
		for _, c := range cols {
			mean += float64(c)
		}
		mean /= float64(len(cols))
		v := 0.0
		for _, c := range cols {
			d := float64(c) - mean
			v += d * d
		}
		v /= float64(len(cols))
		sum += v
		count++
	}
	if count == 0 {
		return 0
	}
	return 1 - math.Min(1, sum/float64(count))
}

// compositeScore combines the three metrics into one 0..1 score. Equal
// weights: row presence, dash consistency, and boundary alignment matter
// equally, and all three are already 0..1.
func compositeScore(q QualityResult) float64 {
	return (q.RowRatio + q.DashConsistency + q.BarAlign) / 3
}

// timingWord maps the composite score to a timing word.
func timingWord(q QualityResult) string {
	switch c := compositeScore(q); {
	case c >= solidThreshold:
		return TimingSolid
	case c <= sloppyThreshold:
		return TimingSloppy
	default:
		return TimingApproximate
	}
}

// ApplyQuality writes the quality metrics into tab metadata:
//
//	quality          composite score, 2 decimals
//	quality_timing   TimingWord ("solid" | "approximate" | "sloppy")
//	quality_row_ratio, quality_dash, quality_bar_align   metric sub-keys, 3 decimals
//
// Safe on a nil tab or nil Metadata map.
func ApplyQuality(tab *model.Tab, q QualityResult) {
	if tab == nil {
		return
	}
	if tab.Metadata == nil {
		tab.Metadata = map[string]string{}
	}
	tab.Metadata[metaKeyQuality] = fmt.Sprintf("%.2f", compositeScore(q))
	tab.Metadata[metaKeyQualityTim] = q.TimingWord
	tab.Metadata[metaKeyQualityRow] = fmt.Sprintf("%.3f", q.RowRatio)
	tab.Metadata[metaKeyQualityDash] = fmt.Sprintf("%.3f", q.DashConsistency)
	tab.Metadata[metaKeyQualityAlign] = fmt.Sprintf("%.3f", q.BarAlign)
}
