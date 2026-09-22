package dosebudget

import (
	"fmt"
	"sort"
	"strings"
)

// Segment is one stage of a segmented dose budget. Rate is in mSv/h and
// minutes is the planned exposure duration for that stage.
type Segment struct {
	DoseRateMSVH float64  `json:"dose_rate_msvh"`
	Minutes      int      `json:"minutes"`
	Controls     []string `json:"controls"`
}

// SegmentDose is the evaluated dose contribution of one segment.
type SegmentDose struct {
	Index        int      `json:"index"`
	DoseRateMSVH float64  `json:"dose_rate_msvh"`
	Minutes      int      `json:"minutes"`
	PlannedDose  float64  `json:"planned_dose_msv"`
	Controls     []string `json:"controls"`
	Formula      string   `json:"formula"`
}

const MaxSegments = 20

// NormalizeSegments trims and de-duplicates controls per segment, validates
// every numeric bound and enforces the shared 1440-minute total.
func NormalizeSegments(segments []Segment) ([]Segment, error) {
	if len(segments) == 0 {
		return nil, fmt.Errorf("%w: at least one dose budget segment is required", ErrInvalidDoseInput)
	}
	if len(segments) > MaxSegments {
		return nil, fmt.Errorf("%w: at most %d dose budget segments are allowed", ErrInvalidDoseInput, MaxSegments)
	}
	normalized := make([]Segment, 0, len(segments))
	totalMinutes := 0
	for i, segment := range segments {
		if !finite(segment.DoseRateMSVH) || segment.DoseRateMSVH < 0 || segment.DoseRateMSVH > 1000 {
			return nil, fmt.Errorf("%w: segment %d dose_rate_msvh must be between 0 and 1000", ErrInvalidDoseInput, i+1)
		}
		if segment.Minutes <= 0 || segment.Minutes > 1440 {
			return nil, fmt.Errorf("%w: segment %d minutes must be from 1 to 1440", ErrInvalidDoseInput, i+1)
		}
		totalMinutes += segment.Minutes
		if totalMinutes > 1440 {
			return nil, fmt.Errorf("%w: total planned minutes across segments must not exceed 1440", ErrInvalidDoseInput)
		}
		controls := make([]string, 0, len(segment.Controls))
		seen := map[string]bool{}
		for _, control := range segment.Controls {
			control = strings.TrimSpace(control)
			if control == "" || seen[strings.ToLower(control)] {
				continue
			}
			if len(control) < 2 || len(control) > 160 {
				return nil, fmt.Errorf("%w: segment %d controls must each be 2 to 160 characters", ErrInvalidDoseInput, i+1)
			}
			seen[strings.ToLower(control)] = true
			controls = append(controls, control)
		}
		if len(controls) == 0 {
			return nil, fmt.Errorf("%w: segment %d requires at least one exposure control", ErrInvalidDoseInput, i+1)
		}
		sort.Strings(controls)
		normalized = append(normalized, Segment{
			DoseRateMSVH: segment.DoseRateMSVH, Minutes: segment.Minutes, Controls: controls,
		})
	}
	return normalized, nil
}

// SegmentPlannedDose returns the dose contribution of one segment.
func SegmentPlannedDose(rateMSVH float64, minutes int) float64 {
	return rateMSVH * float64(minutes) / 60.0
}

// SegmentBreakdown evaluates the ordered dose contribution of each segment.
func SegmentBreakdown(segments []Segment) []SegmentDose {
	breakdown := make([]SegmentDose, 0, len(segments))
	for i, segment := range segments {
		dose := SegmentPlannedDose(segment.DoseRateMSVH, segment.Minutes)
		breakdown = append(breakdown, SegmentDose{
			Index: i + 1, DoseRateMSVH: segment.DoseRateMSVH, Minutes: segment.Minutes,
			PlannedDose: roundDose(dose), Controls: append([]string(nil), segment.Controls...),
			Formula: fmt.Sprintf("segment_%d_dose_msv = %.6g mSv/h * %d min / 60 = %.6f mSv",
				i+1, segment.DoseRateMSVH, segment.Minutes, roundDose(dose)),
		})
	}
	return breakdown
}

// TotalSegmentMinutes sums the planned duration across segments.
func TotalSegmentMinutes(segments []Segment) int {
	total := 0
	for _, segment := range segments {
		total += segment.Minutes
	}
	return total
}

// PlannedDoseFromSegments sums the dose contribution of every segment.
func PlannedDoseFromSegments(segments []Segment) float64 {
	planned := 0.0
	for _, segment := range segments {
		planned += SegmentPlannedDose(segment.DoseRateMSVH, segment.Minutes)
	}
	return roundDose(planned)
}

// TimeWeightedRate returns the dose-weighted average rate (mSv/h) across all
// segments, i.e. total planned dose divided by total hours.
func TimeWeightedRate(segments []Segment) float64 {
	minutes := TotalSegmentMinutes(segments)
	if minutes <= 0 {
		return 0
	}
	return roundDose(PlannedDoseFromSegments(segments) * 60.0 / float64(minutes))
}

// CombinedControls returns the sorted de-duplicated union of segment controls.
func CombinedControls(segments []Segment) []string {
	seen := map[string]bool{}
	combined := []string{}
	for _, segment := range segments {
		for _, control := range segment.Controls {
			if !seen[strings.ToLower(control)] {
				seen[strings.ToLower(control)] = true
				combined = append(combined, control)
			}
		}
	}
	sort.Strings(combined)
	return combined
}
