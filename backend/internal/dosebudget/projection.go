package dosebudget

import (
	"fmt"
	"math"
)

type Segment struct {
	DoseRateMSVH float64  `json:"dose_rate_msvh"`
	Minutes      int      `json:"minutes"`
	Controls     []string `json:"controls"`
	PlannedDose  float64  `json:"planned_dose_msv"`
}

type Projection struct {
	CurrentDoseMSV      float64
	PlannedDoseMSV      float64
	ProjectedTotalMSV   float64
	TimeWeightedRateMSV float64
	Segments            []Segment
}

func CalculateProjection(currentDose, estimatedRateMSVH float64, plannedMinutes int) (Projection, error) {
	return CalculateSegmentProjection(currentDose, []Segment{{
		DoseRateMSVH: estimatedRateMSVH,
		Minutes:      plannedMinutes,
	}})
}

func CalculateSegmentProjection(currentDose float64, segments []Segment) (Projection, error) {
	if !finite(currentDose) || currentDose < 0 {
		return Projection{}, fmt.Errorf("%w: current dose must be finite and non-negative", ErrInvalidDoseInput)
	}
	if len(segments) == 0 {
		return Projection{}, fmt.Errorf("%w: at least one plan segment is required", ErrInvalidDoseInput)
	}
	if len(segments) > 20 {
		return Projection{}, fmt.Errorf("%w: a plan cannot contain more than 20 segments", ErrInvalidDoseInput)
	}

	totalMinutes := 0
	plannedDose := 0.0
	calculated := make([]Segment, 0, len(segments))
	for index, segment := range segments {
		if !finite(segment.DoseRateMSVH) || segment.DoseRateMSVH < 0 || segment.DoseRateMSVH > 1000 {
			return Projection{}, fmt.Errorf("%w: segment %d dose rate must be finite and from 0 to 1000 mSv/h", ErrInvalidDoseInput, index+1)
		}
		if segment.Minutes <= 0 || segment.Minutes > 1440 {
			return Projection{}, fmt.Errorf("%w: segment %d minutes must be from 1 to 1440", ErrInvalidDoseInput, index+1)
		}
		totalMinutes += segment.Minutes
		if totalMinutes > 1440 {
			return Projection{}, fmt.Errorf("%w: total planned minutes cannot exceed 1440", ErrInvalidDoseInput)
		}
		segmentDose := segment.DoseRateMSVH * float64(segment.Minutes) / 60.0
		if !finite(segmentDose) {
			return Projection{}, fmt.Errorf("%w: segment %d dose overflow", ErrInvalidDoseInput, index+1)
		}
		segmentDose = roundDose(segmentDose)
		plannedDose += segmentDose
		segment.PlannedDose = segmentDose
		calculated = append(calculated, segment)
	}
	plannedDose = roundDose(plannedDose)
	projected := currentDose + plannedDose
	if !finite(projected) {
		return Projection{}, fmt.Errorf("%w: projection overflow", ErrInvalidDoseInput)
	}
	weightedRate := plannedDose * 60.0 / float64(totalMinutes)
	if !finite(weightedRate) {
		return Projection{}, fmt.Errorf("%w: time-weighted rate overflow", ErrInvalidDoseInput)
	}
	return Projection{
		CurrentDoseMSV: currentDose, PlannedDoseMSV: plannedDose,
		ProjectedTotalMSV:   roundDose(projected),
		TimeWeightedRateMSV: roundDose(weightedRate),
		Segments:            calculated,
	}, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func roundDose(value float64) float64 {
	if value < 0 {
		return 0
	}
	return math.Round(value*1000000) / 1000000
}
