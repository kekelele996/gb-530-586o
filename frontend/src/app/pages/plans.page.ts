import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AbstractControl, FormArray, FormBuilder, FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
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
import { PlanInput, PlanSegmentInput, WorkPermitPlan } from '../types/permit';

type SegmentForm = FormGroup<{
  dose_rate_msvh: FormControl<number>;
  minutes: FormControl<number>;
  controls: FormControl<string>;
}>;

@Component({
  standalone: true,
  imports: [
    CommonModule, ReactiveFormsModule, MatButtonModule, MatFormFieldModule, MatInputModule,
    MatSelectModule, SafetyBoundaryBannerComponent,
  ],
  template: `
    <div class="page">
      <header class="page-head">
        <div><span class="eyebrow">Dose assumption register</span><h1>Work plan scenarios</h1><p>Build the budget from dose-rate segments and controls before generating an immutable assessment.</p></div>
        <button *ngIf="auth.canPlan()" mat-flat-button color="primary" (click)="newPlan()">{{ formOpen() ? 'Reset form' : 'New scenario' }}</button>
      </header>
      <app-safety-boundary-banner title="Planning scenario, not a permit" detail="An accepted scenario still requires the site's independent work authorization process and qualified RPO judgment." />
      <p class="error-banner" *ngIf="error()">{{ error() }}</p>
      <form *ngIf="formOpen()" class="inline-form" [formGroup]="form" (ngSubmit)="save()">
        <mat-form-field class="span-2" appearance="outline"><mat-label>Plan code</mat-label><input matInput formControlName="plan_code" [readonly]="!!editing()"></mat-form-field>
        <mat-form-field class="span-3" appearance="outline"><mat-label>Worker</mat-label><mat-select formControlName="worker_id"><mat-option *ngFor="let worker of activeWorkers()" [value]="worker.id">{{ worker.worker_code }} · {{ worker.display_name }}</mat-option></mat-select></mat-form-field>
        <mat-form-field class="span-3" appearance="outline"><mat-label>Work area</mat-label><input matInput formControlName="work_area"></mat-form-field>
        <mat-form-field class="span-4" appearance="outline"><mat-label>Task category</mat-label><input matInput formControlName="task_category"></mat-form-field>

        <div class="span-12 segments" formArrayName="segments">
          <div class="segments-head">
            <h3>Dose budget segments</h3>
            <button mat-button type="button" (click)="addSegment()" [disabled]="segments.length >= 20">Add segment</button>
          </div>
          <p class="segments-hint">Each segment needs its own dose rate, duration and at least one control. Total duration must stay within 1440 minutes.</p>
          <div class="segment-row" *ngFor="let segment of segments.controls; let i = index" [formGroupName]="i">
            <span class="segment-index">S{{ i + 1 }}</span>
            <mat-form-field appearance="outline"><mat-label>Dose rate mSv/h</mat-label><input matInput type="number" min="0" max="1000" step="0.01" formControlName="dose_rate_msvh"></mat-form-field>
            <mat-form-field appearance="outline"><mat-label>Minutes</mat-label><input matInput type="number" min="1" max="1440" step="1" formControlName="minutes"></mat-form-field>
            <mat-form-field class="segment-controls" appearance="outline"><mat-label>Controls, comma separated</mat-label><input matInput formControlName="controls"></mat-form-field>
            <span class="segment-dose">{{ segmentDose(segment) | number:'1.3-3' }} mSv</span>
            <button mat-button type="button" class="remove-segment" (click)="removeSegment(i)" [disabled]="segments.length === 1">Remove</button>
          </div>
        </div>

        <div class="span-3 preview"><strong>{{ totalMinutes() }}</strong><span>total minutes</span></div>
        <div class="span-3 preview"><strong>{{ weightedRate() | number:'1.3-3' }}</strong><span>time-weighted mSv/h</span></div>
        <div class="span-3 preview"><strong>{{ projectedPreview | number:'1.3-3' }}</strong><span>planned mSv</span></div>
        <div class="span-3 preview" [class.invalid]="totalMinutes() > 1440"><strong [class.over-limit]="totalMinutes() > 1440">{{ 1440 - totalMinutes() }}</strong><span>minutes to daily cap</span></div>
        <div class="span-12 form-actions"><button mat-button type="button" (click)="closeForm()">Cancel</button><button mat-flat-button color="primary" type="submit" [disabled]="form.invalid || totalMinutes() > 1440 || saving()">{{ saving() ? 'Saving' : editing() ? 'Update draft' : 'Create draft' }}</button></div>
      </form>
      <div class="section-title"><h2>Scenario register</h2><span>{{ plans.plans().length }} plans · select a draft to edit</span></div>
      <div class="surface">
        <table class="data-table">
          <thead><tr><th>Plan</th><th>Worker</th><th>Area / task</th><th>Segments</th><th>Assumption</th><th>Controls</th><th>Status</th><th></th></tr></thead>
          <tbody>
            <tr *ngFor="let plan of plans.plans()" [class.selected]="editing()?.id === plan.id">
              <td><strong>{{ plan.plan_code }}</strong><br><span class="code muted">v{{ plan.version }}</span></td>
              <td>{{ plan.worker_name }}<br><span class="code muted">{{ plan.worker_code }}</span></td>
              <td>{{ plan.work_area }}<br><span class="muted">{{ plan.task_category }}</span></td>
              <td class="number">{{ plan.segments.length }} segment<span *ngIf="plan.segments.length !== 1">s</span><br><span class="muted">{{ plan.planned_minutes }} min total</span></td>
              <td class="number">{{ plan.time_weighted_rate_msvh | number:'1.2-3' }} mSv/h × {{ plan.planned_minutes }} min<br><strong>{{ plan.projected_dose_msv | number:'1.3-3' }} mSv</strong></td>
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
    .preview { min-height: 56px; display: flex; flex-direction: column; justify-content: center; padding: 0 12px; border-left: 3px solid #c47d10; }
    .preview strong { font-size: 20px; font-variant-numeric: tabular-nums; } .preview span { color: var(--muted); font-size: 10px; text-transform: uppercase; }
    .preview.invalid { border-left-color: #8c2929; } .over-limit { color: #8c2929; }
    .number { font-variant-numeric: tabular-nums; white-space: nowrap; }
    .control { display: inline-block; margin: 2px 4px 2px 0; padding: 2px 6px; background: #e7ece7; border-radius: 2px; font-size: 10px; }
    .plan-status { display: inline-flex; padding: 3px 7px; border: 1px solid #b6c2ba; border-radius: 3px; font-size: 10px; font-weight: 800; white-space: nowrap; }
    .plan-status[data-status="pending_rpo_review"] { color: #76510b; border-color: #d6b262; background: #fff1ca; }
    .plan-status[data-status="planning_accepted"] { color: #185847; border-color: #8eb8a8; background: #e5f1eb; }
    .plan-status[data-status="rejected"] { color: #8c2929; border-color: #d89591; background: #f8dfde; }
    .segments { border: 1px solid #b6c2ba; background: #f4f6f3; padding: 12px; }
    .segments-head { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
    .segments-head h3 { margin: 0; font-size: 14px; text-transform: uppercase; letter-spacing: .04em; color: var(--muted); }
    .segments-hint { margin: 2px 0 10px; font-size: 11px; color: var(--muted); }
    .segment-row { display: grid; grid-template-columns: 34px minmax(130px, 1fr) minmax(110px, .8fr) minmax(220px, 2fr) auto auto; gap: 10px; align-items: center; margin-bottom: 8px; }
    .segment-index { font-weight: 800; color: #286858; font-variant-numeric: tabular-nums; }
    .segment-row mat-form-field { width: 100%; }
    .segment-dose { font-variant-numeric: tabular-nums; font-weight: 700; white-space: nowrap; }
    .remove-segment { color: #8c2929; }
    @media (max-width: 980px) { .segment-row { grid-template-columns: 30px 1fr 1fr; } .segment-controls, .segment-dose, .remove-segment { grid-column: 2 / -1; } }
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
    segments: this.builder.array<SegmentForm>([]),
  });

  ngOnInit(): void { this.workers.load(); this.plans.load(); }

  get segments(): FormArray<SegmentForm> { return this.form.controls.segments; }

  totalMinutes(): number {
    return this.segments.controls.reduce((sum, segment) => sum + (Number(segment.get('minutes')?.value) || 0), 0);
  }

  get projectedPreview(): number {
    return this.segments.controls.reduce((sum, segment) => {
      const rate = Number(segment.get('dose_rate_msvh')?.value) || 0;
      const minutes = Number(segment.get('minutes')?.value) || 0;
      return sum + rate * minutes / 60;
    }, 0);
  }

  weightedRate(): number {
    const minutes = this.totalMinutes();
    return minutes > 0 ? this.projectedPreview * 60 / minutes : 0;
  }

  segmentDose(segment: AbstractControl): number {
    const value = (segment as FormGroup).getRawValue() as { dose_rate_msvh: number; minutes: number };
    return (Number(value.dose_rate_msvh) || 0) * (Number(value.minutes) || 0) / 60;
  }

  private segmentGroup(rate = 0.1, minutes = 30, controls = 'time limit, distance markers') {
    return this.builder.group({
      dose_rate_msvh: [rate, [Validators.required, Validators.min(0), Validators.max(1000)]],
      minutes: [minutes, [Validators.required, Validators.min(1), Validators.max(1440)]],
      controls: [controls, [Validators.required, Validators.pattern('.*\\S.*')]],
    });
  }

  addSegment(rate?: number, minutes?: number, controls?: string): void {
    this.segments.push(this.segmentGroup(rate, minutes, controls));
  }

  removeSegment(index: number): void {
    if (this.segments.length > 1) this.segments.removeAt(index);
  }

  private resetSegments(initial?: WorkPermitPlan): void {
    while (this.segments.length) this.segments.removeAt(0);
    const source = initial?.segments.length ? initial.segments : [{ dose_rate_msvh: 0.1, minutes: 30, controls: ['time limit, distance markers'] }];
    for (const segment of source) {
      this.addSegment(segment.dose_rate_msvh, segment.minutes, segment.controls.join(', '));
    }
  }

  newPlan(): void {
    this.editing.set(null); this.formOpen.set(true);
    this.form.reset({ plan_code: 'ALARA-', worker_id: this.activeWorkers()[0]?.id ?? 0, work_area: '', task_category: '' });
    this.resetSegments();
  }

  edit(plan: WorkPermitPlan): void {
    this.editing.set(plan); this.formOpen.set(true);
    this.form.reset({ plan_code: plan.plan_code, worker_id: plan.worker_id, work_area: plan.work_area, task_category: plan.task_category });
    this.resetSegments(plan);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  closeForm(): void { this.formOpen.set(false); this.editing.set(null); }

  save(): void {
    if (this.form.invalid || this.totalMinutes() > 1440) return;
    const raw = this.form.getRawValue();
    const segments: PlanSegmentInput[] = raw.segments.map(segment => ({
      dose_rate_msvh: segment.dose_rate_msvh,
      minutes: segment.minutes,
      controls: segment.controls.split(',').map(value => value.trim()).filter(Boolean),
    }));
    if (segments.length === 0 || segments.some(segment => segment.controls.length === 0)) {
      this.error.set('Every segment needs at least one concrete control.');
      return;
    }
    const input: PlanInput = {
      plan_code: raw.plan_code, worker_id: raw.worker_id, work_area: raw.work_area,
      task_category: raw.task_category, segments,
    };
    const editing = this.editing();
    const request = editing
      ? this.plans.update(editing.id, { worker_id: input.worker_id, work_area: input.work_area, task_category: input.task_category, segments, version: editing.version })
      : this.plans.create(input);
    this.saving.set(true); this.error.set('');
    request.pipe(finalize(() => this.saving.set(false))).subscribe({
      next: () => this.closeForm(),
      error: error => this.error.set(apiErrorMessage(error)),
    });
  }

  statusLabel(status: string): string { return status.replaceAll('_', ' '); }
}
