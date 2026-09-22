package dto

import "time"

type PlanSegmentInput struct {
	DoseRateMSVH float64  `json:"dose_rate_msvh"`
	Minutes      int      `json:"minutes"`
	Controls     []string `json:"controls"`
}

type PlanSegment struct {
	DoseRateMSVH   float64  `json:"dose_rate_msvh"`
	Minutes        int      `json:"minutes"`
	PlannedDoseMSV float64  `json:"planned_dose_msv"`
	Controls       []string `json:"controls"`
}

type CreateWorkPermitPlanRequest struct {
	PlanCode          string             `json:"plan_code" validate:"required,min=3,max=48"`
	WorkerID          uint               `json:"worker_id" validate:"required,gt=0"`
	WorkArea          string             `json:"work_area" validate:"required,min=2,max=120"`
	TaskCategory      string             `json:"task_category" validate:"required,min=2,max=80"`
	EstimatedRateMSVH float64            `json:"estimated_rate_msvh" validate:"gte=0,lte=1000"`
	PlannedMinutes    int                `json:"planned_minutes" validate:"gte=0,lte=1440"`
	Controls          []string           `json:"controls" validate:"max=20,dive,min=2,max=160"`
	Segments          []PlanSegmentInput `json:"segments" validate:"max=20,dive"`
}

type UpdateWorkPermitPlanRequest struct {
	WorkerID          uint               `json:"worker_id" validate:"required,gt=0"`
	WorkArea          string             `json:"work_area" validate:"required,min=2,max=120"`
	TaskCategory      string             `json:"task_category" validate:"required,min=2,max=80"`
	EstimatedRateMSVH float64            `json:"estimated_rate_msvh" validate:"gte=0,lte=1000"`
	PlannedMinutes    int                `json:"planned_minutes" validate:"gte=0,lte=1440"`
	Controls          []string           `json:"controls" validate:"max=20,dive,min=2,max=160"`
	Segments          []PlanSegmentInput `json:"segments" validate:"max=20,dive"`
	Version           uint               `json:"version" validate:"required,gt=0"`
}

type PlanVersionRequest struct {
	Version uint `json:"version" validate:"required,gt=0"`
}

type ReviewPlanRequest struct {
	Version  uint   `json:"version" validate:"required,gt=0"`
	Decision string `json:"decision" validate:"required,oneof=accept reject"`
	Note     string `json:"note" validate:"required,min=3,max=1000"`
}

type WorkPermitPlanResponse struct {
	ID                  uint          `json:"id"`
	PlanCode            string        `json:"plan_code"`
	WorkerID            uint          `json:"worker_id"`
	WorkerCode          string        `json:"worker_code"`
	WorkerName          string        `json:"worker_name"`
	WorkArea            string        `json:"work_area"`
	TaskCategory        string        `json:"task_category"`
	EstimatedRateMSVH   float64       `json:"estimated_rate_msvh"`
	TimeWeightedRateMSV float64       `json:"time_weighted_rate_msvh"`
	PlannedMinutes      int           `json:"planned_minutes"`
	ProjectedDoseMSV    float64       `json:"projected_dose_msv"`
	Controls            []string      `json:"controls"`
	Segments            []PlanSegment `json:"segments"`
	PermitStatus        string        `json:"permit_status"`
	Version             uint          `json:"version"`
	ReviewerID          *uint         `json:"reviewer_id,omitempty"`
	ReviewNote          string        `json:"review_note"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
	ArchivedAt          *time.Time    `json:"archived_at,omitempty"`
}
