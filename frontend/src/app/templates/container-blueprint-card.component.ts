import { ChangeDetectionStrategy, Component, computed, signal } from '@angular/core';
import { ContainerBlueprintData } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

@Component({
  selector: 'app-container-blueprint-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (mode() === 'blueprint') {
      <div class="bp-wrap">
        <div class="bp-toolbar">
          <span class="bp-iso font-mono">ISO {{ data().isoCode || '45G1' }} · {{ data().shortCode || "40' HC" }}</span>
          <div class="unit-toggle" role="group" aria-label="Measurement units">
            <button
              type="button"
              class="unit-btn"
              [class.active]="unit() === 'metric'"
              [attr.aria-pressed]="unit() === 'metric'"
              (click)="unit.set('metric')"
            >
              Metric
            </button>
            <button
              type="button"
              class="unit-btn"
              [class.active]="unit() === 'imperial'"
              [attr.aria-pressed]="unit() === 'imperial'"
              (click)="unit.set('imperial')"
            >
              Imperial
            </button>
          </div>
        </div>

        <figure class="bp-figure">
          <svg
            class="bp-svg"
            viewBox="0 0 420 156"
            role="img"
            [attr.aria-label]="(data().shortCode || 'Container') + ' dimensional blueprint diagram'"
          >
            <!-- Technical blueprint backdrop (theme-adaptive Light/Dark) -->
            <rect x="4" y="4" width="412" height="148" rx="8" fill="var(--surface-l1)" stroke="var(--border-l0)" stroke-width="1" />
            <!-- Container side elevation (isometric hint) -->
            <polygon points="48,40 318,40 348,24 78,24" fill="var(--tile-blue-bg)" stroke="var(--accent)" stroke-width="1.5" />
            <rect x="48" y="40" width="270" height="76" fill="var(--surface-l0)" stroke="var(--accent)" stroke-width="1.6" />
            <polygon points="318,40 348,24 348,100 318,116" fill="var(--chip-blue-bg)" stroke="var(--accent)" stroke-width="1.5" />
            <!-- Corrugation ribs -->
            <g stroke="var(--chip-blue-border)" stroke-width="1">
              <line x1="74" y1="44" x2="74" y2="112" />
              <line x1="98" y1="44" x2="98" y2="112" />
              <line x1="122" y1="44" x2="122" y2="112" />
              <line x1="146" y1="44" x2="146" y2="112" />
              <line x1="170" y1="44" x2="170" y2="112" />
              <line x1="194" y1="44" x2="194" y2="112" />
              <line x1="218" y1="44" x2="218" y2="112" />
              <line x1="242" y1="44" x2="242" y2="112" />
              <line x1="266" y1="44" x2="266" y2="112" />
              <line x1="290" y1="44" x2="290" y2="112" />
            </g>
            <!-- Door lock bars -->
            <line x1="330" y1="36" x2="330" y2="108" stroke="var(--tile-blue-val)" stroke-width="1.4" />
            <line x1="338" y1="32" x2="338" y2="104" stroke="var(--tile-blue-val)" stroke-width="1.4" />

            <!-- Length dimension callout -->
            <line x1="48" y1="132" x2="318" y2="132" stroke="var(--text-secondary)" stroke-width="1.1" />
            <line x1="48" y1="126" x2="48" y2="138" stroke="var(--text-secondary)" stroke-width="1.1" />
            <line x1="318" y1="126" x2="318" y2="138" stroke="var(--text-secondary)" stroke-width="1.1" />
            <rect x="142" y="122" width="84" height="18" rx="4" fill="var(--chip-blue-bg)" stroke="var(--chip-blue-border)" />
            <text x="184" y="135" text-anchor="middle" font-size="11" font-weight="700" fill="var(--chip-blue-text)" font-family="ui-monospace, monospace">
              L {{ activeInt().length }}
            </text>

            <!-- Height dimension callout -->
            <line x1="26" y1="40" x2="26" y2="116" stroke="var(--text-secondary)" stroke-width="1.1" />
            <line x1="20" y1="40" x2="32" y2="40" stroke="var(--text-secondary)" stroke-width="1.1" />
            <line x1="20" y1="116" x2="32" y2="116" stroke="var(--text-secondary)" stroke-width="1.1" />
            <text x="19" y="81" text-anchor="middle" font-size="10" font-weight="700" fill="var(--text-primary)" font-family="ui-monospace, monospace" transform="rotate(-90 19 81)">
              H {{ activeInt().height }}
            </text>

            <!-- Width callout -->
            <text x="380" y="64" text-anchor="middle" font-size="10" font-weight="700" fill="var(--text-primary)" font-family="ui-monospace, monospace">
              W {{ activeInt().width }}
            </text>
            <text x="380" y="80" text-anchor="middle" font-size="9.5" fill="var(--text-muted)" font-family="sans-serif">
              Door
            </text>
            <text x="380" y="93" text-anchor="middle" font-size="9.5" font-weight="600" fill="var(--text-secondary)" font-family="ui-monospace, monospace">
              {{ activeDoor().width }} × {{ activeDoor().height }}
            </text>
          </svg>
        </figure>

        <div class="dim-grid">
          <div class="dim-card">
            <span class="dim-label">Internal (L × W × H)</span>
            <span class="dim-val font-mono">{{ activeInt().length }} × {{ activeInt().width }} × {{ activeInt().height }}</span>
          </div>
          <div class="dim-card">
            <span class="dim-label">Door Opening</span>
            <span class="dim-val font-mono">{{ activeDoor().width }} × {{ activeDoor().height }}</span>
          </div>
          <div class="dim-card">
            <span class="dim-label">Cubic Capacity</span>
            <span class="dim-val font-mono">{{ unit() === 'metric' ? (data().volumeCbm || '—') : (data().volumeCuFt || data().volumeCbm || '—') }}</span>
          </div>
        </div>
        <div class="bp-footer">
          <button type="button" class="copy-spec-btn" (click)="copySpecs()">
            Copy Dimensions
          </button>
        </div>
      </div>
    } @else if (mode() === 'capacity') {
      <div class="cap-grid">
        <div class="cap-box">
          <div class="cap-head">
            <span class="cap-title">Euro Pallets</span>
            <span class="cap-badge green">{{ data().euroPallets ?? '—' }}</span>
          </div>
          <p class="cap-note">{{ data().euroNote || '1200 × 800 mm (single tier)' }}</p>
        </div>
        <div class="cap-box">
          <div class="cap-head">
            <span class="cap-title">US / GMA Pallets</span>
            <span class="cap-badge neutral">{{ data().usPallets ?? '—' }}</span>
          </div>
          <p class="cap-note">{{ data().usNote || '48" × 40" (single tier)' }}</p>
        </div>
      </div>
      <div class="weight-row">
        @if (data().maxPayload) {
          <div class="weight-chip">
            <span class="w-lbl">Max Payload</span>
            <span class="w-val font-mono">{{ data().maxPayload }}</span>
          </div>
        }
        @if (data().tareWeight) {
          <div class="weight-chip">
            <span class="w-lbl">Tare</span>
            <span class="w-val font-mono">{{ data().tareWeight }}</span>
          </div>
        }
        @if (data().maxGross) {
          <div class="weight-chip">
            <span class="w-lbl">Max Gross</span>
            <span class="w-val font-mono">{{ data().maxGross }}</span>
          </div>
        }
      </div>
    }
  `,
  styles: [
    `
      .bp-wrap {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      .bp-toolbar {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
      }
      .bp-iso {
        font-size: 12px;
        font-weight: 700;
        color: var(--chip-blue-text);
      }
      .unit-toggle {
        display: inline-flex;
        padding: 2px;
        border-radius: 8px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
      }
      .unit-btn {
        border: 0;
        background: transparent;
        padding: 3px 9px;
        border-radius: 6px;
        font-size: 11.5px;
        font-weight: 600;
        color: var(--text-muted);
        cursor: pointer;
      }
      .unit-btn.active {
        background: var(--surface-l0);
        color: var(--text-primary);
        box-shadow: var(--shadow-card);
      }
      .bp-figure {
        margin: 0;
      }
      .bp-svg {
        display: block;
        width: 100%;
        height: auto;
        max-height: 164px;
      }
      .dim-grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
        gap: 6px;
      }
      .dim-card {
        padding: 7px 9px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
        display: flex;
        flex-direction: column;
        gap: 2px;
      }
      .dim-label {
        font-size: 10.5px;
        color: var(--text-muted);
      }
      .dim-val {
        font-size: 12px;
        font-weight: 600;
        color: var(--text-primary);
      }
      .bp-footer {
        display: flex;
        justify-content: flex-end;
      }
      .copy-spec-btn {
        min-block-size: 26px;
        padding: 4px 10px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        font-size: 12px;
        font-weight: 600;
        cursor: pointer;
      }
      .copy-spec-btn:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .cap-grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
        gap: 8px;
        margin-block-end: 8px;
      }
      .cap-box {
        padding: 9px 11px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
      }
      .cap-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
      }
      .cap-title {
        font-size: 12.5px;
        font-weight: 700;
        color: var(--text-primary);
      }
      .cap-badge {
        font-family: var(--font-mono);
        font-size: 12px;
        font-weight: 700;
        padding: 2px 8px;
        border-radius: 999px;
        border: 1px solid transparent;
      }
      .cap-badge.green {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .cap-badge.neutral {
        background: var(--chip-neutral-bg);
        border-color: var(--chip-neutral-border);
        color: var(--chip-neutral-text);
      }
      .cap-note {
        margin: 5px 0 0;
        font-size: 11.5px;
        color: var(--text-muted);
        line-height: 1.4;
      }
      .weight-row {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
      }
      .weight-chip {
        flex: 1 1 130px;
        padding: 6px 9px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
        display: flex;
        flex-direction: column;
        gap: 2px;
      }
      .w-lbl {
        font-size: 10.5px;
        color: var(--text-muted);
      }
      .w-val {
        font-size: 11.5px;
        font-weight: 600;
        color: var(--text-primary);
      }
    `,
  ],
})
export class ContainerBlueprintCardComponent extends BaseCardTemplate<ContainerBlueprintData> {
  readonly unit = signal<'metric' | 'imperial'>('metric');
  readonly data = computed(() => this.node().data ?? {});
  readonly mode = computed(() => this.data().mode ?? 'summary');

  readonly activeInt = computed(() => {
    const d = this.data();
    if (this.unit() === 'imperial' && d.intImperial) {
      return d.intImperial;
    }
    return d.intMetric ?? { length: '12.03 m', width: '2.35 m', height: '2.69 m' };
  });

  readonly activeDoor = computed(() => {
    const d = this.data();
    if (this.unit() === 'imperial' && d.doorImperial) {
      return d.doorImperial;
    }
    return d.doorMetric ?? { width: '2.34 m', height: '2.58 m' };
  });

  copySpecs(): void {
    const d = this.data();
    const dims = this.activeInt();
    const summary = `${d.shortCode || 'Container'} (ISO ${d.isoCode || ''}): ${dims.length} × ${dims.width} × ${dims.height} | ${d.volumeCbm || ''}`;
    this.triggerCopy('Container Specs', summary);
  }
}
