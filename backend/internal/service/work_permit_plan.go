package service

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"radiation-dose-budget-control/backend/internal/constants"
	"radiation-dose-budget-control/backend/internal/dosebudget"
	"radiation-dose-budget-control/backend/internal/dto"
	"radiation-dose-budget-control/backend/internal/model"
	"radiation-dose-budget-control/backend/internal/repository"
)

type WorkPermitPlanService struct {
	plans       *repository.WorkPermitPlanRepository
	workers     *repository.WorkerProfileRepository
	assessments *repository.DoseBudgetAssessmentRepository
	audit       *AuditService
}

func NewWorkPermitPlanService(
	plans *repository.WorkPermitPlanRepository,
	workers *repository.WorkerProfileRepository,
	assessments *repository.DoseBudgetAssessmentRepository,
	audit *AuditService,
) *WorkPermitPlanService {
	return &WorkPermitPlanService{plans: plans, workers: workers, assessments: assessments, audit: audit}
}

func (service *WorkPermitPlanService) Create(request dto.CreateWorkPermitPlanRequest, actor dto.Actor, requestID string) (dto.WorkPermitPlanResponse, error) {
	worker, err := service.workers.Find(request.WorkerID)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, MapRepositoryError("worker profile", err)
	}
	if worker.ProfileStatus != constants.ProfileStatusActive {
		return dto.WorkPermitPlanResponse{}, Conflict("worker_not_active", "new plans require an active worker profile", nil)
	}
	segments, err := resolvePlanSegments(&request.Segments, request.EstimatedRateMSVH, request.PlannedMinutes, request.Controls)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	encodedSegments, weightedRate, totalMinutes, combinedControlsJSON, err := encodeSegments(segments)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	plan := model.WorkPermitPlan{
		PlanCode: strings.ToUpper(strings.TrimSpace(request.PlanCode)), WorkerID: request.WorkerID,
		WorkArea: strings.TrimSpace(request.WorkArea), TaskCategory: strings.TrimSpace(request.TaskCategory),
		EstimatedRateMSVH: weightedRate, PlannedMinutes: totalMinutes,
		ControlsJSON: combinedControlsJSON, SegmentsJSON: encodedSegments,
		PermitStatus: constants.PermitStatusDraft, Version: 1, CreatedBy: actor.ID,
	}
	if err := service.plans.Create(&plan); err != nil {
		if repository.IsUniqueViolation(err) {
			return dto.WorkPermitPlanResponse{}, Conflict("duplicate_plan_code", "plan_code already exists", err)
		}
		return dto.WorkPermitPlanResponse{}, Internal("could not create work permit plan", err)
	}
	response := planResponse(plan, worker)
	if err := service.audit.Record(actor, requestID, "plan.created", "work_permit_plan", auditID(plan.ID),
		map[string]any{"plan_code": plan.PlanCode}, nil, planAudit(plan)); err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	return response, nil
}

func (service *WorkPermitPlanService) Update(id uint, request dto.UpdateWorkPermitPlanRequest, actor dto.Actor, requestID string) (dto.WorkPermitPlanResponse, error) {
	before, err := service.plans.Find(id)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, MapRepositoryError("work permit plan", err)
	}
	if before.PermitStatus != constants.PermitStatusDraft {
		return dto.WorkPermitPlanResponse{}, Conflict("invalid_state", "only draft plans can be edited", nil)
	}
	worker, err := service.workers.Find(request.WorkerID)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, MapRepositoryError("worker profile", err)
	}
	if worker.ProfileStatus != constants.ProfileStatusActive {
		return dto.WorkPermitPlanResponse{}, Conflict("worker_not_active", "plan worker must have an active profile", nil)
	}
	segments, err := resolvePlanSegments(&request.Segments, request.EstimatedRateMSVH, request.PlannedMinutes, request.Controls)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	encodedSegments, weightedRate, totalMinutes, combinedControlsJSON, err := encodeSegments(segments)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	updated := before
	updated.WorkerID = request.WorkerID
	updated.WorkArea = strings.TrimSpace(request.WorkArea)
	updated.TaskCategory = strings.TrimSpace(request.TaskCategory)
	updated.EstimatedRateMSVH = weightedRate
	updated.PlannedMinutes = totalMinutes
	updated.ControlsJSON = combinedControlsJSON
	updated.SegmentsJSON = encodedSegments
	if err := service.plans.Update(updated, request.Version); err != nil {
		if strings.Contains(err.Error(), repository.ErrVersionConflict.Error()) {
			return dto.WorkPermitPlanResponse{}, Conflict("version_conflict", "plan changed or left draft state; refresh before updating", err)
		}
		return dto.WorkPermitPlanResponse{}, Internal("could not update work permit plan", err)
	}
	updated.Version = request.Version + 1
	updated.UpdatedAt = time.Now().UTC()
	if err := service.audit.Record(actor, requestID, "plan.updated", "work_permit_plan", auditID(id),
		map[string]any{"expected_version": request.Version}, planAudit(before), planAudit(updated)); err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	return planResponse(updated, worker), nil
}

func (service *WorkPermitPlanService) Get(id uint) (dto.WorkPermitPlanResponse, error) {
	plan, err := service.plans.Find(id)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, MapRepositoryError("work permit plan", err)
	}
	worker, err := service.workers.Find(plan.WorkerID)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, MapRepositoryError("plan worker", err)
	}
	return planResponse(plan, worker), nil
}

func (service *WorkPermitPlanService) List(page, pageSize int, status, workerFilter string) ([]dto.WorkPermitPlanResponse, dto.PageMeta, error) {
	if status != "" && !constants.IsPermitStatus(status) {
		return nil, dto.PageMeta{}, BadRequest("invalid_permit_status", "permit_status filter is not recognized")
	}
	workerID, err := parseUintFilter(workerFilter)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	plans, total, err := service.plans.List(page, pageSize, status, workerID)
	if err != nil {
		return nil, dto.PageMeta{}, Internal("could not list work permit plans", err)
	}
	responses := make([]dto.WorkPermitPlanResponse, 0, len(plans))
	for _, plan := range plans {
		worker, err := service.workers.Find(plan.WorkerID)
		if err != nil {
			return nil, dto.PageMeta{}, MapRepositoryError("plan worker", err)
		}
		responses = append(responses, planResponse(plan, worker))
	}
	return responses, pageMeta(page, pageSize, total), nil
}

func (service *WorkPermitPlanService) Archive(id uint, request dto.PlanVersionRequest, actor dto.Actor, requestID string) (dto.WorkPermitPlanResponse, error) {
	before, err := service.plans.Find(id)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, MapRepositoryError("work permit plan", err)
	}
	if !constants.CanTransitionPermit(before.PermitStatus, constants.PermitStatusArchived) {
		return dto.WorkPermitPlanResponse{}, Conflict("invalid_state", "only reviewed plans can be archived", nil)
	}
	now := time.Now().UTC()
	if err := service.plans.Transition(id, request.Version, before.PermitStatus, constants.PermitStatusArchived, map[string]any{"archived_at": now}); err != nil {
		return dto.WorkPermitPlanResponse{}, Conflict("version_conflict", "plan changed before archive", err)
	}
	after := before
	after.PermitStatus = constants.PermitStatusArchived
	after.Version = request.Version + 1
	after.ArchivedAt = &now
	if err := service.audit.Record(actor, requestID, "plan.archived", "work_permit_plan", auditID(id),
		map[string]any{"expected_version": request.Version}, planAudit(before), planAudit(after)); err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	worker, err := service.workers.Find(after.WorkerID)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, MapRepositoryError("plan worker", err)
	}
	return planResponse(after, worker), nil
}

// resolvePlanSegments validates the segmented budget. A nil slice means the
// segments field was omitted (older clients), so the legacy single-segment
// fields are used instead; an explicitly empty slice is rejected.
func resolvePlanSegments(
	inputs *[]dto.PlanSegmentInput,
	legacyRate float64,
	legacyMinutes int,
	legacyControls []string,
) ([]dosebudget.Segment, error) {
	if inputs == nil || *inputs == nil {
		if !finiteNumber(legacyRate) || legacyRate < 0 || legacyRate > 1000 {
			return nil, BadRequest("invalid_rate", "estimated_rate_msvh must be between 0 and 1000")
		}
		if legacyMinutes <= 0 || legacyMinutes > 1440 {
			return nil, BadRequest("invalid_minutes", "planned_minutes must be from 1 to 1440")
		}
		legacy := []dto.PlanSegmentInput{{
			DoseRateMSVH: legacyRate, Minutes: legacyMinutes, Controls: legacyControls,
		}}
		inputs = &legacy
	}
	if len(*inputs) == 0 {
		return nil, BadRequest("segments_required", "at least one dose budget segment is required")
	}
	segments := make([]dosebudget.Segment, 0, len(*inputs))
	for _, input := range *inputs {
		segments = append(segments, dosebudget.Segment{
			DoseRateMSVH: input.DoseRateMSVH, Minutes: input.Minutes, Controls: input.Controls,
		})
	}
	normalized, err := dosebudget.NormalizeSegments(segments)
	if err != nil {
		return nil, BadRequest("invalid_segments", err.Error())
	}
	return normalized, nil
}

func encodeSegments(segments []dosebudget.Segment) (encoded string, weightedRate float64, totalMinutes int, combinedControlsJSON string, err error) {
	encodedBytes, marshalErr := json.Marshal(segments)
	if marshalErr != nil {
		return "", 0, 0, "", Internal("could not encode plan segments", marshalErr)
	}
	controls, marshalErr := json.Marshal(dosebudget.CombinedControls(segments))
	if marshalErr != nil {
		return "", 0, 0, "", Internal("could not encode plan controls", marshalErr)
	}
	return string(encodedBytes), dosebudget.TimeWeightedRate(segments),
		dosebudget.TotalSegmentMinutes(segments), string(controls), nil
}

// decodePlanSegments returns the stored segments. Plans created before the
// segmented budget feature have no segments_json, so they fall back to a
// single segment built from the plan's aggregate fields and stored controls.
func decodePlanSegments(plan model.WorkPermitPlan) []dosebudget.Segment {
	segments := []dosebudget.Segment{}
	if strings.TrimSpace(plan.SegmentsJSON) != "" && plan.SegmentsJSON != "[]" {
		if err := json.Unmarshal([]byte(plan.SegmentsJSON), &segments); err == nil && len(segments) > 0 {
			return segments
		}
	}
	controls := []string{}
	_ = json.Unmarshal([]byte(plan.ControlsJSON), &controls)
	return []dosebudget.Segment{{
		DoseRateMSVH: plan.EstimatedRateMSVH, Minutes: plan.PlannedMinutes, Controls: controls,
	}}
}

func planResponse(plan model.WorkPermitPlan, worker model.WorkerProfile) dto.WorkPermitPlanResponse {
	segments := decodePlanSegments(plan)
	breakdown := dosebudget.SegmentBreakdown(segments)
	segmentResponses := make([]dto.PlanSegment, 0, len(segments))
	for i, segment := range segments {
		segmentResponses = append(segmentResponses, dto.PlanSegment{
			DoseRateMSVH: segment.DoseRateMSVH, Minutes: segment.Minutes,
			PlannedDoseMSV: breakdown[i].PlannedDose,
			Controls:       append([]string(nil), segment.Controls...),
		})
	}
	controls := dosebudget.CombinedControls(segments)
	return dto.WorkPermitPlanResponse{
		ID: plan.ID, PlanCode: plan.PlanCode, WorkerID: plan.WorkerID, WorkerCode: worker.WorkerCode,
		WorkerName: worker.DisplayName, WorkArea: plan.WorkArea, TaskCategory: plan.TaskCategory,
		EstimatedRateMSVH:   dosebudget.TimeWeightedRate(segments),
		TimeWeightedRateMSV: dosebudget.TimeWeightedRate(segments),
		PlannedMinutes:      dosebudget.TotalSegmentMinutes(segments),
		ProjectedDoseMSV:    dosebudget.PlannedDoseFromSegments(segments),
		Controls:            controls, Segments: segmentResponses, PermitStatus: plan.PermitStatus, Version: plan.Version,
		ReviewerID: plan.ReviewerID, ReviewNote: plan.ReviewNote, CreatedAt: plan.CreatedAt,
		UpdatedAt: plan.UpdatedAt, ArchivedAt: plan.ArchivedAt,
	}
}

func finiteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func planAudit(plan model.WorkPermitPlan) map[string]any {
	return map[string]any{
		"id": plan.ID, "plan_code": plan.PlanCode, "worker_id": plan.WorkerID, "work_area": plan.WorkArea,
		"task_category": plan.TaskCategory, "estimated_rate_msvh": plan.EstimatedRateMSVH,
		"planned_minutes": plan.PlannedMinutes, "controls": plan.ControlsJSON,
		"segments": plan.SegmentsJSON, "permit_status": plan.PermitStatus,
		"version": plan.Version, "reviewer_id": plan.ReviewerID,
	}
}
