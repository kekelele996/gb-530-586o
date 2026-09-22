package dosebudget

import (
	"math"
	"testing"
)

func TestCalculateProjectionConvertsMinutesToHours(t *testing.T) {
	tests := []struct {
		name        string
		current     float64
		rate        float64
		minutes     int
		wantPlanned float64
		wantTotal   float64
	}{
		{"one hour", 5, 2, 60, 2, 7},
		{"quarter hour", 1, 4, 15, 1, 2},
		{"fractional rate", 3.2, 0.42, 45, 0.315, 3.515},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CalculateProjection(test.current, test.rate, test.minutes)
			if err != nil {
				t.Fatalf("CalculateProjection returned error: %v", err)
			}
			if got.PlannedDoseMSV != test.wantPlanned || got.ProjectedTotalMSV != test.wantTotal {
				t.Fatalf("projection = %+v, want planned=%v total=%v", got, test.wantPlanned, test.wantTotal)
			}
		})
	}
	if _, err := CalculateProjection(0, math.NaN(), 60); err == nil {
		t.Fatal("NaN rate unexpectedly accepted")
	}
	if _, err := CalculateProjection(0, 1, 0); err == nil {
		t.Fatal("zero minutes unexpectedly accepted")
	}
}

func TestCalculateSegmentProjectionSumsSegmentsAndWeightsRate(t *testing.T) {
	got, err := CalculateSegmentProjection(2, []Segment{
		{DoseRateMSVH: 3, Minutes: 30},
		{DoseRateMSVH: 1, Minutes: 30},
	})
	if err != nil {
		t.Fatalf("CalculateSegmentProjection returned error: %v", err)
	}
	if got.PlannedDoseMSV != 2 || got.ProjectedTotalMSV != 4 || got.TimeWeightedRateMSV != 2 {
		t.Fatalf("projection = %+v, want planned 2, total 4, weighted rate 2", got)
	}
	if got.Segments[0].PlannedDose != 1.5 || got.Segments[1].PlannedDose != 0.5 {
		t.Fatalf("segment doses = %+v, want 1.5 and 0.5", got.Segments)
	}
}

func TestCalculateSegmentProjectionRejectsInvalidSegments(t *testing.T) {
	tests := []struct {
		name     string
		segments []Segment
	}{
		{"empty", nil},
		{"zero duration", []Segment{{DoseRateMSVH: 1, Minutes: 0}}},
		{"negative rate", []Segment{{DoseRateMSVH: -0.1, Minutes: 30}}},
		{"rate above limit", []Segment{{DoseRateMSVH: 1000.1, Minutes: 30}}},
		{"total duration above 1440", []Segment{{DoseRateMSVH: 1, Minutes: 720}, {DoseRateMSVH: 1, Minutes: 721}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CalculateSegmentProjection(0, test.segments); err == nil {
				t.Fatalf("%s unexpectedly accepted", test.name)
			}
		})
	}
}
