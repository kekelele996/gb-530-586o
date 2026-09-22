import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormArray, FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { finalize } from 'rxjs';
import { PlansStore } from '../stores/plans.store';
import { WorkersStore } from '../stores/workers.store';
import { useAuth } from '../hooks/use-auth';
import { SafetyBoundaryBannerComponent } from '../components/common/safety-boundary-banner.component';
import { apiErrorMessage } from '../utils/api-error';
import { PlanInput, PlanSegment, WorkPermitPlan } from '../types/permit';

@Component({
  standalone: true,
  imports: [
    CommonModule, ReactiveFormsModule, MatButtonModule, MatFormFieldModule, MatInputModule,
    MatSelectModule, SafetyBoundaryBannerComponent,
  ],
  template: `
    <div class="page">
      <header class="page-head">
        <div><span class="eyebrow">Dose assumption register</span><h1>Work plan scenarios</h1><p>Edit segmented rates, durations and controls before generating an immutable assessment.</p></div>
        <button *ngIf="auth.canPlan()" mat-flat-button color="primary" (click)="newPlan()">{{ formOpen() ? 'Reset form' : 'New scenario' }}</button>
      </header>
      <app-safety-boundary-banner title="Planning scenario, not a permit" detail="An accepted scenario still requires the site's independent work authorization process and qualified RPO judgment." />
      <p class="error-banner" *ngIf="error()">{{ error() }}</p>
      <form *ngIf="formOpen()" class="inline-form" [formGroup]="form" (ngSubmit)="save()">
        <mat-form-field class="span-2" appearance="outline"><mat-label>Plan code</mat-label><input matInput formControlName="plan_code" [readonly]="!!editing()"></mat-form-field>
        <mat-form-field class="span-3" appearance="outline"><mat-label>Worker</mat-label><mat-select formControlName="worker_id"><mat-option *ngFor="let worker of activeWorkers()" [value]="worker.id">{{ worker.worker_code }} · {{ worker.display_name }}</mat-option></mat-select></mat-form-field>
        <mat-form-field class="span-3" appearance="outline"><mat-label>Work area</mat-label><input matInput formControlName="work_area"></mat-form-field>
        <mat-form-field class="span-4" appearance="outline"><mat-label>Task category</mat-label><input matInput formControlName="task_category"></mat-form-field>

        <div class="span-12 segments-head">
          <div><span class="eyebrow">Dose budget segments</span><strong>Every segment needs at least one concrete control.</strong></div>
          <button mat-stroked-button type="button" (click)="addSegment()" [disabled]="segmentControls.length >= 20">Add segment</button>
        </div>
        <div class="span-12 segment-list" formArrayName="segments">
          <div class="segment-row" *ngFor="let segment of segmentControls; let i = index" [formGroupName]="i">
            <span class="segment-index">{{ i + 1 }}</span>
            <mat-form-field appearance="outline"><mat-label>Dose rate mSv/h</mat-label><input matInput type="number" min="0" max="1000" step="0.01" formControlName="dose_rate_msvh"></mat-form-field>
            <mat-form-field appearance="outline"><mat-label>Minutes</mat-label><input matInput type="number" min="1" max="1440" step="1" formControlName="minutes"></mat-form-field>
            <mat-form-field class="segment-controls" appearance="outline"><mat-label>Controls, separated by commas</mat-label><textarea matInput rows="1" formControlName="controls_text"></textarea></mat-form-field>
            <strong class="segment-dose">{{ segmentDose(i) | number:'1.3-3' }} mSv</strong>
            <button mat-button type="button" color="warn" (click)="removeSegment(i)" [disabled]="segmentControls.length === 1">Remove</button>
          </div>
        </div>
        <div class="span-4 preview"><strong>{{ totalMinutes }}</strong><span>total minutes / 1440 max</span></div>
        <div class="span-4 preview"><strong>{{ weightedRate | number:'1.3-3' }}</strong><span>time-weighted mSv/h</span></div>
        <div class="span-4 preview"><strong>{{ totalDose | number:'1.3-3' }}</strong><span>planned mSv total</span></div>
        <p class="span-12 validation-note" *ngIf="totalMinutes > 1440">Total segment duration cannot exceed 1440 minutes.</p>
        <div class="span-12 form-actions"><button mat-button type="button" (click)="closeForm()">Cancel</button><button mat-flat-button color="primary" type="submit" [disabled]="form.invalid || saving() || totalMinutes > 1440">{{ saving() ? 'Saving' : editing() ? 'Update draft' : 'Create draft' }}</button></div>
      </form>
      <div class="section-title"><h2>Scenario register</h2><span>{{ plans.plans().length }} plans · select a draft to edit</span></div>
      <div class="surface">
        <table class="data-table">
          <thead><tr><th>Plan</th><th>Worker</th><th>Area / task</th><th>Segmented dose budget</th><th>Controls</th><th>Status</th><th></th></tr></thead>
          <tbody>
            <tr *ngFor="let plan of plans.plans()" [class.selected]="editing()?.id === plan.id">
              <td><strong>{{ plan.plan_code }}</strong><br><span class="code muted">v{{ plan.version }}</span></td>
              <td>{{ plan.worker_name }}<br><span class="code muted">{{ plan.worker_code }}</span></td>
              <td>{{ plan.work_area }}<br><span class="muted">{{ plan.task_category }}</span></td>
              <td class="number">
                <div class="segment-line" *ngFor="let segment of plan.segments">{{ segment.dose_rate_msvh | number:'1.2-3' }} mSv/h × {{ segment.minutes }} min = {{ segment.planned_dose_msv | number:'1.3-3' }} mSv</div>
                <strong>{{ plan.projected_dose_msv | number:'1.3-3' }} mSv / {{ plan.planned_minutes }} min</strong>
              </td>
              <td><span class="control" *ngFor="let control of plan.controls">{{ control }}</span></td>
              <td><span class="plan-status" [attr.data-status]="plan.permit_status">{{ statusLabel(plan.permit_status) }}</span></td>
              <td><button *ngIf="auth.canPlan() && plan.permit_status === 'draft'" mat-button (click)="edit(plan)">Edit</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  `,
  styles: [`
    .segments-head { display: flex; justify-content: space-between; align-items: center; gap: 16px; padding: 10px 0 0; }
    .segment-list { display: grid; gap: 8px; }
    .segment-row { display: grid; grid-template-columns: 28px minmax(130px, .8fr) minmax(110px, .6fr) minmax(280px, 2fr) 110px 76px; gap: 10px; align-items: center; padding: 8px; border: 1px solid var(--line); background: #f8f8f3; }
    .segment-index { display: grid; place-items: center; width: 24px; height: 24px; border-radius: 50%; background: #286858; color: white; font-weight: 800; font-size: 11px; }
    .segment-row mat-form-field { width: 100%; }
    .segment-dose { font-variant-numeric: tabular-nums; color: #286858; text-align: right; }
    .preview { min-height: 56px; display: flex; flex-direction: column; justify-content: center; padding: 0 12px; border-left: 3px solid #c47d10; }
    .preview strong { font-size: 20px; font-variant-numeric: tabular-nums; } .preview span { color: var(--muted); font-size: 10px; text-transform: uppercase; }
    .validation-note { margin: 0; color: #8c2929; font-size: 12px; font-weight: 700; }
    .number { font-variant-numeric: tabular-nums; white-space: nowrap; }
    .segment-line { font-size: 11px; color: var(--muted); }
    .control { display: inline-block; margin: 2px 4px 2px 0; padding: 2px 6px; background: #e7ece7; border-radius: 2px; font-size: 10px; }
    .plan-status { display: inline-flex; padding: 3px 7px; border: 1px solid #b6c2ba; border-radius: 3px; font-size: 10px; font-weight: 800; white-space: nowrap; }
    .plan-status[data-status="pending_rpo_review"] { color: #76510b; border-color: #d6b262; background: #fff1ca; }
    .plan-status[data-status="planning_accepted"] { color: #185847; border-color: #8eb8a8; background: #e5f1eb; }
    .plan-status[data-status="rejected"] { color: #8c2929; border-color: #d89591; background: #f8dfde; }
    @media (max-width: 900px) { .segment-row { grid-template-columns: 28px 1fr 1fr; } .segment-controls, .segment-dose, .segment-row button { grid-column: span 3; } }
  `],
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PlansPage implements OnInit {
  readonly plans = inject(PlansStore);
  readonly workers = inject(WorkersStore);
  readonly auth = useAuth();
  private readonly builder = new FormBuilder().nonNullable;
  readonly editing = signal<WorkPermitPlan | null>(null);
  readonly formOpen = signal(false);
  readonly saving = signal(false);
  readonly error = signal('');
  readonly activeWorkers = computed(() => this.workers.workers().filter(worker => worker.profile_status === 'active'));
  readonly form = this.builder.group({
    plan_code: ['ALARA-', [Validators.required, Validators.minLength(3)]],
    worker_id: [0, [Validators.required, Validators.min(1)]],
    work_area: ['', [Validators.required, Validators.minLength(2)]],
    task_category: ['', [Validators.required, Validators.minLength(2)]],
    segments: this.builder.array([this.createSegment(0.1, 30, 'time limit, distance markers')]),
  });

  ngOnInit(): void { this.workers.load(); this.plans.load(); }

  get segmentControls() { return this.form.controls.segments.controls; }
  get totalMinutes(): number {
    return this.segmentControls.reduce((total, segment) => total + (Number(segment.controls.minutes.value) || 0), 0);
  }
  get totalDose(): number {
    return this.segmentControls.reduce((total, _, index) => total + this.segmentDose(index), 0);
  }
  get weightedRate(): number { return this.totalMinutes > 0 ? this.totalDose * 60 / this.totalMinutes : 0; }

  segmentDose(index: number): number {
    const segment = this.segmentControls[index];
    const rate = Number(segment.controls.dose_rate_msvh.value) || 0;
    const minutes = Number(segment.controls.minutes.value) || 0;
    return rate * minutes / 60;
  }

  createSegment(rate = 0.1, minutes = 30, controls = '') {
    return this.builder.group({
      dose_rate_msvh: [rate, [Validators.required, Validators.min(0), Validators.max(1000)]],
      minutes: [minutes, [Validators.required, Validators.min(1), Validators.max(1440)]],
      controls_text: [controls, Validators.required],
    });
  }

  addSegment(segment?: PlanSegment): void {
    this.form.controls.segments.push(this.createSegment(
      segment?.dose_rate_msvh ?? 0.1,
      segment?.minutes ?? 30,
      segment?.controls.join(', ') ?? '',
    ));
  }

  removeSegment(index: number): void {
    if (this.segmentControls.length > 1) this.form.controls.segments.removeAt(index);
  }

  private resetSegments(segments: PlanSegment[]): void {
    while (this.form.controls.segments.length) this.form.controls.segments.removeAt(0);
    (segments.length ? segments : [{ dose_rate_msvh: 0.1, minutes: 30, controls: ['time limit', 'distance markers'], planned_dose_msv: 0 }])
      .forEach(segment => this.addSegment(segment));
  }

  newPlan(): void {
    this.editing.set(null); this.formOpen.set(true);
    this.form.reset({ plan_code: 'ALARA-', worker_id: this.activeWorkers()[0]?.id ?? 0, work_area: '', task_category: '' });
    this.resetSegments([]);
  }

  edit(plan: WorkPermitPlan): void {
    this.editing.set(plan); this.formOpen.set(true);
    this.form.reset({ plan_code: plan.plan_code, worker_id: plan.worker_id, work_area: plan.work_area, task_category: plan.task_category });
    this.resetSegments(plan.segments);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  closeForm(): void { this.formOpen.set(false); this.editing.set(null); }

  save(): void {
    if (this.form.invalid || this.totalMinutes > 1440) return;
    const raw = this.form.getRawValue();
    const segments = raw.segments.map(segment => ({
      dose_rate_msvh: Number(segment.dose_rate_msvh),
      minutes: Number(segment.minutes),
      controls: segment.controls_text.split(',').map(value => value.trim()).filter(Boolean),
    }));
    if (!segments.length || segments.some(segment => !segment.controls.length)) {
      this.error.set('Every segment needs at least one concrete control.');
      return;
    }
    const input: PlanInput = {
      plan_code: raw.plan_code, worker_id: raw.worker_id, work_area: raw.work_area,
      task_category: raw.task_category, segments,
    };
    const editing = this.editing();
    const request = editing
      ? this.plans.update(editing.id, { worker_id: input.worker_id, work_area: input.work_area, task_category: input.task_category, segments: input.segments, version: editing.version })
      : this.plans.create(input);
    this.saving.set(true); this.error.set('');
    request.pipe(finalize(() => this.saving.set(false))).subscribe({
      next: () => this.closeForm(),
      error: error => this.error.set(apiErrorMessage(error)),
    });
  }

  statusLabel(status: string): string { return status.replaceAll('_', ' '); }
}
