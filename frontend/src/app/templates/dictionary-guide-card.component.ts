import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { DictionaryGuideData, DictionaryStep } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

@Component({
  selector: 'app-dictionary-guide-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="guide-body">
      @if (steps().length > 0) {
        <ol
          class="timeline"
          [style.--step-count]="steps().length"
          [attr.aria-label]="timelineLabel()"
        >
          @for (step of steps(); track step.index) {
            <li
              class="timeline-step"
              [class.completed]="step.index < activeStep()"
              [class.active]="step.index === activeStep()"
              [attr.aria-current]="step.index === activeStep() ? 'step' : null"
            >
              <div class="step-track">
                <span class="step-badge">{{ step.index }}</span>
                @if (!$last) {
                  <span class="step-line" aria-hidden="true"></span>
                }
              </div>
              <div class="step-label">{{ step.label }}</div>
              @if (step.caption) {
                <div class="step-caption">{{ step.caption }}</div>
              }
            </li>
          }
        </ol>
      }

      <p class="guide-summary">{{ summary() }}</p>

      @if (bullets().length > 0) {
        <ul class="guide-bullets">
          @for (item of bullets(); track $index) {
            <li>{{ item }}</li>
          }
        </ul>
      }
    </div>
  `,
  styles: [
    `
      .guide-body {
        display: flex;
        flex-direction: column;
        gap: 10px;
      }
      .timeline {
        list-style: none;
        margin: 0;
        padding: 10px;
        display: grid;
        grid-template-columns: repeat(var(--step-count, 4), minmax(0, 1fr));
        gap: 6px;
        border-radius: 9px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
      }
      .timeline-step {
        display: flex;
        flex-direction: column;
        gap: 3px;
      }
      .step-track {
        display: flex;
        align-items: center;
        gap: 4px;
      }
      .step-badge {
        width: 20px;
        height: 20px;
        border-radius: 50%;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 700;
        border: 1.5px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-muted);
        flex-shrink: 0;
      }
      .timeline-step.completed .step-badge {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .timeline-step.active .step-badge {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .step-line {
        flex: 1;
        height: 2px;
        background: var(--border-divider);
        border-radius: 2px;
      }
      .timeline-step.completed .step-line {
        background: var(--chip-green-border);
      }
      .timeline-step.active .step-line {
        background: var(--chip-blue-border);
      }
      .step-label {
        font-size: 11.5px;
        font-weight: 700;
        color: var(--text-primary);
      }
      .timeline-step.active .step-label {
        color: var(--chip-blue-text);
      }
      .step-caption {
        font-size: 10.5px;
        color: var(--text-muted);
        line-height: 1.25;
      }
      .guide-summary {
        margin: 0;
        font-size: 12.5px;
        line-height: 1.55;
        color: var(--text-secondary);
      }
      .guide-bullets {
        margin: 0;
        padding-inline-start: 18px;
        display: flex;
        flex-direction: column;
        gap: 3px;
        font-size: 12px;
        color: var(--text-secondary);
        line-height: 1.45;
      }
    `,
  ],
})
export class DictionaryGuideCardComponent extends BaseCardTemplate<DictionaryGuideData> {
  readonly activeStep = computed(() => this.node().data?.activeStep ?? 1);
  readonly timelineLabel = computed(() => this.node().data?.timelineLabel || 'Process steps');
  readonly steps = computed<DictionaryStep[]>(() => {
    const raw = this.node().data?.steps;
    return Array.isArray(raw) ? raw : [];
  });
  readonly bullets = computed<string[]>(() => {
    const raw = this.node().data?.bullets;
    return Array.isArray(raw) ? raw : [];
  });
  readonly summary = computed(
    () => this.node().data?.summary || this.node().description || 'Plain-English reference guide.'
  );
}
