package dto

import "time"

type PlanSegmentRequest struct {
	DoseRateMSVH *float64 `json:"dose_rate_msvh" validate:"required,gte=0,lte=1000"`
	Minutes      *int     `json:"minutes" validate:"required,gt=0,lte=1440"`
	Controls     []string `json:"controls" validate:"required,min=1,max=20,dive,min=2,max=160"`
}

type PlanSegmentResponse struct {
	DoseRateMSVH   float64  `json:"dose_rate_msvh"`
	Minutes        int      `json:"minutes"`
	Controls       []string `json:"controls"`
	PlannedDoseMSV float64  `json:"planned_dose_msv"`
}

type CreateWorkPermitPlanRequest struct {
	PlanCode          string               `json:"plan_code" validate:"required,min=3,max=48"`
	WorkerID          uint                 `json:"worker_id" validate:"required,gt=0"`
	WorkArea          string               `json:"work_area" validate:"required,min=2,max=120"`
	TaskCategory      string               `json:"task_category" validate:"required,min=2,max=80"`
	EstimatedRateMSVH *float64             `json:"estimated_rate_msvh" validate:"omitempty,gte=0,lte=1000"`
	PlannedMinutes    *int                 `json:"planned_minutes" validate:"omitempty,gt=0,lte=1440"`
	Controls          []string             `json:"controls" validate:"omitempty,min=1,max=20,dive,min=2,max=160"`
	Segments          []PlanSegmentRequest `json:"segments" validate:"omitempty,max=20,dive,required"`
}

type UpdateWorkPermitPlanRequest struct {
	WorkerID          uint                 `json:"worker_id" validate:"required,gt=0"`
	WorkArea          string               `json:"work_area" validate:"required,min=2,max=120"`
	TaskCategory      string               `json:"task_category" validate:"required,min=2,max=80"`
	EstimatedRateMSVH *float64             `json:"estimated_rate_msvh" validate:"omitempty,gte=0,lte=1000"`
	PlannedMinutes    *int                 `json:"planned_minutes" validate:"omitempty,gt=0,lte=1440"`
	Controls          []string             `json:"controls" validate:"omitempty,min=1,max=20,dive,min=2,max=160"`
	Segments          []PlanSegmentRequest `json:"segments" validate:"omitempty,max=20,dive,required"`
	Version           uint                 `json:"version" validate:"required,gt=0"`
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
	ID                uint                  `json:"id"`
	PlanCode          string                `json:"plan_code"`
	WorkerID          uint                  `json:"worker_id"`
	WorkerCode        string                `json:"worker_code"`
	WorkerName        string                `json:"worker_name"`
	WorkArea          string                `json:"work_area"`
	TaskCategory      string                `json:"task_category"`
	EstimatedRateMSVH float64               `json:"estimated_rate_msvh"`
	PlannedMinutes    int                   `json:"planned_minutes"`
	ProjectedDoseMSV  float64               `json:"projected_dose_msv"`
	Controls          []string              `json:"controls"`
	Segments          []PlanSegmentResponse `json:"segments"`
	PermitStatus      string                `json:"permit_status"`
	Version           uint                  `json:"version"`
	ReviewerID        *uint                 `json:"reviewer_id,omitempty"`
	ReviewNote        string                `json:"review_note"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
	ArchivedAt        *time.Time            `json:"archived_at,omitempty"`
}
