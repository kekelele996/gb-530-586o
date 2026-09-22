package dosebudget

import (
	"math"
	"reflect"
	"testing"
)

func TestNormalizeSegments(t *testing.T) {
	t.Run("normalizes trims dedupes and sorts controls", func(t *testing.T) {
		segments := []Segment{
			{DoseRateMSVH: 0.6, Minutes: 15, Controls: []string{"  Shielding ", "shielding", "Dosimeter"}},
			{DoseRateMSVH: 0.33, Minutes: 30, Controls: []string{"time check"}},
		}
		got, err := NormalizeSegments(segments)
		if err != nil {
			t.Fatalf("NormalizeSegments returned error: %v", err)
		}
		want := []Segment{
			{DoseRateMSVH: 0.6, Minutes: 15, Controls: []string{"Dosimeter", "Shielding"}},
			{DoseRateMSVH: 0.33, Minutes: 30, Controls: []string{"time check"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("normalized segments = %+v, want %+v", got, want)
		}
	})

	t.Run("rejects empty segments", func(t *testing.T) {
		if _, err := NormalizeSegments(nil); err == nil {
			t.Fatal("empty segments unexpectedly accepted")
		}
	})

	t.Run("rejects segment without controls", func(t *testing.T) {
		_, err := NormalizeSegments([]Segment{{DoseRateMSVH: 1, Minutes: 10, Controls: []string{" "}}})
		if err == nil {
			t.Fatal("blank controls unexpectedly accepted")
		}
	})

	t.Run("rejects out of range rates", func(t *testing.T) {
		for _, rate := range []float64{-0.1, 1000.01, math.NaN(), math.Inf(1)} {
			if _, err := NormalizeSegments([]Segment{{DoseRateMSVH: rate, Minutes: 10, Controls: []string{"barrier"}}}); err == nil {
				t.Fatalf("rate %v unexpectedly accepted", rate)
			}
		}
	})

	t.Run("rejects out of range minutes", func(t *testing.T) {
		for _, minutes := range []int{0, -5, 1441} {
			if _, err := NormalizeSegments([]Segment{{DoseRateMSVH: 1, Minutes: minutes, Controls: []string{"barrier"}}}); err == nil {
				t.Fatalf("minutes %d unexpectedly accepted", minutes)
			}
		}
	})

	t.Run("rejects totals beyond 1440 minutes", func(t *testing.T) {
		segments := []Segment{
			{DoseRateMSVH: 1, Minutes: 800, Controls: []string{"barrier"}},
			{DoseRateMSVH: 1, Minutes: 641, Controls: []string{"barrier"}},
		}
		if _, err := NormalizeSegments(segments); err == nil {
			t.Fatal("total of 1441 minutes unexpectedly accepted")
		}
	})

	t.Run("accepts exactly 1440 total minutes", func(t *testing.T) {
		segments := []Segment{
			{DoseRateMSVH: 1, Minutes: 720, Controls: []string{"barrier"}},
			{DoseRateMSVH: 2, Minutes: 720, Controls: []string{"barrier"}},
		}
		if _, err := NormalizeSegments(segments); err != nil {
			t.Fatalf("1440 total minutes unexpectedly rejected: %v", err)
		}
	})
}

func TestSegmentAggregations(t *testing.T) {
	segments := []Segment{
		{DoseRateMSVH: 0.6, Minutes: 15, Controls: []string{"shielding"}},
		{DoseRateMSVH: 0.33, Minutes: 30, Controls: []string{"time check"}},
	}
	if got := TotalSegmentMinutes(segments); got != 45 {
		t.Fatalf("TotalSegmentMinutes = %d, want 45", got)
	}
	// 0.6*15/60 + 0.33*30/60 = 0.15 + 0.165 = 0.315
	if got := PlannedDoseFromSegments(segments); got != 0.315 {
		t.Fatalf("PlannedDoseFromSegments = %v, want 0.315", got)
	}
	// 0.315 / 0.75h = 0.42 mSv/h
	if got := TimeWeightedRate(segments); got != 0.42 {
		t.Fatalf("TimeWeightedRate = %v, want 0.42", got)
	}
	breakdown := SegmentBreakdown(segments)
	if len(breakdown) != 2 || breakdown[0].PlannedDose != 0.15 || breakdown[1].PlannedDose != 0.165 {
		t.Fatalf("SegmentBreakdown = %+v", breakdown)
	}
	if breakdown[0].Index != 1 || breakdown[1].Index != 2 {
		t.Fatalf("segment indexes = %d, %d, want 1, 2", breakdown[0].Index, breakdown[1].Index)
	}
	combined := CombinedControls(segments)
	wantControls := []string{"shielding", "time check"}
	if !reflect.DeepEqual(combined, wantControls) {
		t.Fatalf("CombinedControls = %v, want %v", combined, wantControls)
	}
}

func TestCalculateSegmentProjection(t *testing.T) {
	segments := []Segment{
		{DoseRateMSVH: 2, Minutes: 30, Controls: []string{"barrier"}},
		{DoseRateMSVH: 4, Minutes: 15, Controls: []string{"barrier"}},
	}
	projection, breakdown, err := CalculateSegmentProjection(5, segments)
	if err != nil {
		t.Fatalf("CalculateSegmentProjection returned error: %v", err)
	}
	// planned = 2*0.5 + 4*0.25 = 2; projected = 7; weighted rate = 2*60/45 = 2.666667
	if projection.PlannedDoseMSV != 2 || projection.ProjectedTotalMSV != 7 {
		t.Fatalf("projection doses = %+v", projection)
	}
	if projection.TimeWeightedRateMSV != 2.666667 {
		t.Fatalf("time weighted rate = %v, want 2.666667", projection.TimeWeightedRateMSV)
	}
	if len(breakdown) != 2 {
		t.Fatalf("breakdown length = %d, want 2", len(breakdown))
	}
	if _, _, err := CalculateSegmentProjection(1, []Segment{{DoseRateMSVH: 1, Minutes: 10}}); err == nil {
		t.Fatal("segment without controls unexpectedly accepted")
	}
}
