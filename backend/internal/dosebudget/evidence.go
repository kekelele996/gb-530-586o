package dosebudget

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const BoundaryStatement = "Offline ALARA planning result only. It is not a work permit, dosimeter reading, regulatory determination, or medical advice."

type Snapshot struct {
	WorkerID               uint      `json:"worker_id"`
	WorkerCode             string    `json:"worker_code"`
	WorkerVersion          uint      `json:"worker_version"`
	PlanID                 uint      `json:"plan_id"`
	PlanCode               string    `json:"plan_code"`
	PlanVersion            uint      `json:"plan_version"`
	PeriodStart            time.Time `json:"period_start"`
	PeriodEnd              time.Time `json:"period_end"`
	EstimatedRateMSVH      float64   `json:"estimated_rate_msvh"`
	PlannedMinutes         int       `json:"planned_minutes"`
	Controls               []string  `json:"controls"`
	Segments               []Segment `json:"segments"`
	IncludedExposureIDs    []uint    `json:"included_exposure_ids"`
	ExcludedExposureIDs    []uint    `json:"excluded_exposure_ids"`
	AdministrativeLimitMSV float64   `json:"administrative_limit_msv"`
	LegalLimitMSV          float64   `json:"legal_limit_msv"`
	NearLegalRatio         float64   `json:"near_legal_ratio"`
	ThresholdVersion       string    `json:"threshold_version"`
}

type Evidence struct {
	PeriodStart          time.Time `json:"period_start"`
	PeriodEnd            time.Time `json:"period_end"`
	VerifiedEntryCount   int       `json:"verified_entry_count"`
	ExcludedEntryCount   int       `json:"excluded_entry_count"`
	CorrectedChainCount  int       `json:"corrected_chain_count"`
	Formula              string    `json:"formula"`
	ProjectionFormula    string    `json:"projection_formula"`
	Segments             []Segment `json:"segments"`
	AdministrativeLimit  float64   `json:"administrative_limit_msv"`
	AnnualLegalLimit     float64   `json:"annual_legal_limit_msv"`
	NearLegalRatio       float64   `json:"near_legal_ratio"`
	ThresholdVersion     string    `json:"threshold_version"`
	RequiresManualReview bool      `json:"requires_manual_review"`
	EscalationReason     string    `json:"escalation_reason"`
	BoundaryStatement    string    `json:"boundary_statement"`
}

func BuildArtifacts(snapshot Snapshot, summary PeriodSummary, decision Decision) (string, string, error) {
	snapshot.Controls = append([]string(nil), snapshot.Controls...)
	sort.Strings(snapshot.Controls)
	snapshot.Segments = cloneSegments(snapshot.Segments)
	for index := range snapshot.Segments {
		snapshot.Segments[index].Controls = append([]string(nil), snapshot.Segments[index].Controls...)
	}
	snapshot.IncludedExposureIDs = append([]uint(nil), summary.IncludedEntryIDs...)
	snapshot.ExcludedExposureIDs = append([]uint(nil), summary.ExcludedEntryIDs...)
	evidence := Evidence{
		PeriodStart: snapshot.PeriodStart, PeriodEnd: snapshot.PeriodEnd,
		VerifiedEntryCount: summary.VerifiedEntryCount, ExcludedEntryCount: summary.ExcludedEntryCount,
		CorrectedChainCount: summary.CorrectedChainCount,
		Formula:             "period_dose_msv = sum(verified exposure entries, including immutable reversals and replacements)",
		ProjectionFormula:   projectionFormula(snapshot.Segments),
		Segments:            snapshot.Segments,
		AdministrativeLimit: snapshot.AdministrativeLimitMSV, AnnualLegalLimit: snapshot.LegalLimitMSV,
		NearLegalRatio: snapshot.NearLegalRatio, ThresholdVersion: snapshot.ThresholdVersion,
		RequiresManualReview: decision.RequiresManualReview, EscalationReason: decision.EscalationExplanation,
		BoundaryStatement: BoundaryStatement,
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return "", "", fmt.Errorf("encode assessment input snapshot: %w", err)
	}
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return "", "", fmt.Errorf("encode assessment evidence: %w", err)
	}
	return string(snapshotJSON), string(evidenceJSON), nil
}

func cloneSegments(segments []Segment) []Segment {
	cloned := make([]Segment, len(segments))
	copy(cloned, segments)
	return cloned
}

func projectionFormula(segments []Segment) string {
	terms := make([]string, 0, len(segments))
	for _, segment := range segments {
		terms = append(terms, fmt.Sprintf("(%s mSv/h * %d min / 60)", formatNumber(segment.DoseRateMSVH), segment.Minutes))
	}
	return "projected_dose_msv = period_dose_msv + " + strings.Join(terms, " + ")
}

func formatNumber(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}

func RiskRank(value string) int {
	ranks := map[string]int{"within_admin": 1, "above_admin": 2, "near_legal": 3, "above_legal": 4, "invalid": 5}
	return ranks[value]
}
