package service

import (
	"encoding/json"
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
	segmentInputs, err := segmentRequests(request.EstimatedRateMSVH, request.PlannedMinutes, request.Controls, request.Segments)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	segments, projection, controlsJSON, err := normalizeSegments(segmentInputs)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	plan := model.WorkPermitPlan{
		PlanCode: strings.ToUpper(strings.TrimSpace(request.PlanCode)), WorkerID: request.WorkerID,
		WorkArea: strings.TrimSpace(request.WorkArea), TaskCategory: strings.TrimSpace(request.TaskCategory),
		EstimatedRateMSVH: projection.TimeWeightedRateMSV, PlannedMinutes: sumSegmentMinutes(segments),
		ControlsJSON: controlsJSON, SegmentsJSON: mustEncodeSegments(segments),
		PermitStatus: constants.PermitStatusDraft, Version: 1, CreatedBy: actor.ID,
	}
	if err := service.plans.Create(&plan); err != nil {
		if repository.IsUniqueViolation(err) {
			return dto.WorkPermitPlanResponse{}, Conflict("duplicate_plan_code", "plan_code already exists", err)
		}
		return dto.WorkPermitPlanResponse{}, Internal("could not create work permit plan", err)
	}
	response, err := planResponse(plan, worker)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
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
	segmentInputs, err := segmentRequests(request.EstimatedRateMSVH, request.PlannedMinutes, request.Controls, request.Segments)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	segments, projection, controlsJSON, err := normalizeSegments(segmentInputs)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	updated := before
	updated.WorkerID = request.WorkerID
	updated.WorkArea = strings.TrimSpace(request.WorkArea)
	updated.TaskCategory = strings.TrimSpace(request.TaskCategory)
	updated.EstimatedRateMSVH = projection.TimeWeightedRateMSV
	updated.PlannedMinutes = sumSegmentMinutes(segments)
	updated.ControlsJSON = controlsJSON
	updated.SegmentsJSON = mustEncodeSegments(segments)
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
	return planResponse(updated, worker)
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
	return planResponse(plan, worker)
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
		response, err := planResponse(plan, worker)
		if err != nil {
			return nil, dto.PageMeta{}, err
		}
		responses = append(responses, response)
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
	return planResponse(after, worker)
}

func segmentRequests(
	rate *float64,
	minutes *int,
	controls []string,
	segments []dto.PlanSegmentRequest,
) ([]dto.PlanSegmentRequest, error) {
	if segments != nil {
		if len(segments) == 0 {
			return nil, BadRequest("segments_required", "at least one dose budget segment is required")
		}
		return segments, nil
	}
	if rate == nil {
		zeroRate := 0.0
		rate = &zeroRate
	}
	if minutes == nil || len(controls) == 0 {
		return nil, BadRequest("segments_required", "at least one dose budget segment with controls is required")
	}
	legacyRate := *rate
	legacyMinutes := *minutes
	return []dto.PlanSegmentRequest{{DoseRateMSVH: &legacyRate, Minutes: &legacyMinutes, Controls: controls}}, nil
}

func normalizeSegments(values []dto.PlanSegmentRequest) ([]dosebudget.Segment, dosebudget.Projection, string, error) {
	segments := make([]dosebudget.Segment, 0, len(values))
	allControls := []string{}
	seenControls := map[string]bool{}
	for _, value := range values {
		controls, err := normalizeControlList(value.Controls)
		if err != nil {
			return nil, dosebudget.Projection{}, "", BadRequest("controls_required", "each segment requires at least one concrete exposure control")
		}
		for _, control := range controls {
			key := strings.ToLower(control)
			if !seenControls[key] {
				seenControls[key] = true
				allControls = append(allControls, control)
			}
		}
		if value.DoseRateMSVH == nil || value.Minutes == nil {
			return nil, dosebudget.Projection{}, "", BadRequest("invalid_plan_segments", "each segment requires a finite dose rate and minute duration")
		}
		segments = append(segments, dosebudget.Segment{DoseRateMSVH: *value.DoseRateMSVH, Minutes: *value.Minutes, Controls: controls})
	}
	projection, err := dosebudget.CalculateSegmentProjection(0, segments)
	if err != nil {
		return nil, dosebudget.Projection{}, "", BadRequest("invalid_plan_segments", err.Error())
	}
	controlsJSON, err := json.Marshal(allControls)
	if err != nil {
		return nil, dosebudget.Projection{}, "", Internal("could not encode plan controls", err)
	}
	return projection.Segments, projection, string(controlsJSON), nil
}

func normalizeControlList(values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) < 2 || len(value) > 160 || seen[strings.ToLower(value)] {
			continue
		}
		seen[strings.ToLower(value)] = true
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil, BadRequest("controls_required", "at least one concrete exposure control is required")
	}
	return normalized, nil
}

func sumSegmentMinutes(segments []dosebudget.Segment) int {
	total := 0
	for _, segment := range segments {
		total += segment.Minutes
	}
	return total
}

func mustEncodeSegments(segments []dosebudget.Segment) string {
	encoded, err := json.Marshal(segments)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func decodePlanSegments(plan model.WorkPermitPlan) ([]dosebudget.Segment, error) {
	if strings.TrimSpace(plan.SegmentsJSON) == "" {
		controls := []string{}
		if err := json.Unmarshal([]byte(plan.ControlsJSON), &controls); err != nil {
			return nil, Internal("stored plan controls are invalid", err)
		}
		projection, err := dosebudget.CalculateProjection(0, plan.EstimatedRateMSVH, plan.PlannedMinutes)
		if err != nil {
			return nil, Internal("stored plan dose assumptions are invalid", err)
		}
		segment := projection.Segments[0]
		segment.Controls = controls
		return []dosebudget.Segment{segment}, nil
	}
	segments := []dosebudget.Segment{}
	if err := json.Unmarshal([]byte(plan.SegmentsJSON), &segments); err != nil {
		return nil, Internal("stored plan segments are invalid", err)
	}
	if len(segments) == 0 {
		return nil, Internal("stored plan segments are empty", nil)
	}
	for _, segment := range segments {
		if segment.Minutes <= 0 || len(segment.Controls) == 0 {
			return nil, Internal("stored plan segments are invalid", nil)
		}
	}
	if _, err := dosebudget.CalculateSegmentProjection(0, segments); err != nil {
		return nil, Internal("stored plan dose assumptions are invalid", err)
	}
	return segments, nil
}

func planResponse(plan model.WorkPermitPlan, worker model.WorkerProfile) (dto.WorkPermitPlanResponse, error) {
	segments, err := decodePlanSegments(plan)
	if err != nil {
		return dto.WorkPermitPlanResponse{}, err
	}
	controls := []string{}
	segmentResponses := make([]dto.PlanSegmentResponse, 0, len(segments))
	plannedDose := 0.0
	seenControls := map[string]bool{}
	for _, segment := range segments {
		plannedDose += segment.PlannedDose
		segmentResponses = append(segmentResponses, dto.PlanSegmentResponse{
			DoseRateMSVH: segment.DoseRateMSVH, Minutes: segment.Minutes,
			Controls: segment.Controls, PlannedDoseMSV: segment.PlannedDose,
		})
		for _, control := range segment.Controls {
			key := strings.ToLower(control)
			if !seenControls[key] {
				seenControls[key] = true
				controls = append(controls, control)
			}
		}
	}
	return dto.WorkPermitPlanResponse{
		ID: plan.ID, PlanCode: plan.PlanCode, WorkerID: plan.WorkerID, WorkerCode: worker.WorkerCode,
		WorkerName: worker.DisplayName, WorkArea: plan.WorkArea, TaskCategory: plan.TaskCategory,
		EstimatedRateMSVH: plan.EstimatedRateMSVH, PlannedMinutes: plan.PlannedMinutes,
		ProjectedDoseMSV: plannedDose, Controls: controls, Segments: segmentResponses,
		PermitStatus: plan.PermitStatus, Version: plan.Version,
		ReviewerID: plan.ReviewerID, ReviewNote: plan.ReviewNote, CreatedAt: plan.CreatedAt,
		UpdatedAt: plan.UpdatedAt, ArchivedAt: plan.ArchivedAt,
	}, nil
}

func planAudit(plan model.WorkPermitPlan) map[string]any {
	return map[string]any{
		"id": plan.ID, "plan_code": plan.PlanCode, "worker_id": plan.WorkerID, "work_area": plan.WorkArea,
		"task_category": plan.TaskCategory, "estimated_rate_msvh": plan.EstimatedRateMSVH,
		"planned_minutes": plan.PlannedMinutes, "controls": plan.ControlsJSON, "segments": plan.SegmentsJSON,
		"permit_status": plan.PermitStatus, "version": plan.Version, "reviewer_id": plan.ReviewerID,
	}
}
