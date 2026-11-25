import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CardNode, PatternRule } from '../models/card-node.model';
import { VexBridgeService } from '../services/vex-bridge.service';
import { PatternsDialogComponent } from './patterns-dialog.component';

describe('PatternsDialogComponent', () => {
  const sampleRule: PatternRule = {
    id: 'iso-container',
    name: 'ISO 6346 Shipping Container',
    description: 'Decodes 11-char container IDs (e.g. MSCU1234565) with Mod-11 validation.',
    enabled: true,
    priority: 90,
    regex: '^(?P<bic>[A-Z]{4})(?P<serial>\\d{6})(?P<check>\\d)$',
    checksum: 'iso6346',
    titleTemplate: 'Container · {{match}}',
    layout: 'segments',
    linkRules: [{ targetEntityKey: 'ocean-carrier:{{bic}}' }],
    liveHttp: {
      urlTemplate: 'https://api.example.test/track/{{match}}',
      auth: 'none',
      trigger: 'manual',
      resultCard: {
        layout: 'facts',
        title: 'Live Tracking',
        summary: '{{json.status}}',
      },
    },
    liveMcp: { serverId: 'github', toolName: 'get_issue' },
  };

  let bridgeMock: {
    listPatternRules: ReturnType<typeof vi.fn>;
    upsertPatternRule: ReturnType<typeof vi.fn>;
    deletePatternRule: ReturnType<typeof vi.fn>;
    testPatternRule: ReturnType<typeof vi.fn>;
    recordCardUsed: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    localStorage.clear();
    bridgeMock = {
      listPatternRules: vi.fn().mockResolvedValue([sampleRule]),
      upsertPatternRule: vi.fn().mockResolvedValue([sampleRule]),
      deletePatternRule: vi.fn().mockResolvedValue([]),
      testPatternRule: vi.fn().mockResolvedValue(null),
      recordCardUsed: vi.fn().mockResolvedValue(undefined),
    };
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('places Live Test bar at top with non-truncated placeholder and renders matched segments card live', async () => {
    const matchedCard: CardNode = {
      id: 'preview-iso-container',
      layout: 'segments',
      icon: '🚢',
      title: 'MSC Container · MSCU1234565',
      chips: [{ label: 'ISO 6346', tone: 'blue' }],
      data: {
        segments: [
          { value: 'MSCU', role: 'OWNER', meaning: 'MSC', tone: 'blue' },
          { value: '123456', role: 'SERIAL', meaning: 'Freight Container', tone: 'amber' },
          { value: '5', role: 'MOD-11', meaning: 'Valid', tone: 'green' },
        ],
        explanation: 'ISO 6346 intermodal container code.',
      },
      defaultExpanded: true,
    };
    bridgeMock.testPatternRule.mockResolvedValueOnce(matchedCard).mockResolvedValueOnce(null);

    TestBed.configureTestingModule({
      imports: [PatternsDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(PatternsDialogComponent);
    await fixture.componentInstance.refresh();
    await fixture.whenStable();

    const firstSection = fixture.nativeElement.querySelector('.dialog-body > section') as HTMLElement;
    expect(firstSection.classList.contains('live-test-card')).toBe(true);

    const input = firstSection.querySelector('#pattern-test-input') as HTMLInputElement;
    expect(input.placeholder).toContain('MSCU1234565');

    fixture.componentInstance.testSample.set('MSCU1234565');
    await fixture.componentInstance.runTest();
    await fixture.whenStable();

    expect(bridgeMock.testPatternRule).toHaveBeenCalledWith(sampleRule, 'MSCU1234565');
    expect(fixture.nativeElement.querySelector('.preview-banner')?.textContent).toContain(
      'ISO 6346 Shipping Container'
    );
    expect(fixture.nativeElement.querySelector('app-rolodex-card-shell')).not.toBeNull();
    expect(fixture.nativeElement.querySelector('app-segments-card')).not.toBeNull();

    // Non-matching sample shows clear error
    fixture.componentInstance.testSample.set('NOPE');
    await fixture.componentInstance.runTest();
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.err-msg')?.textContent).toContain('NOPE');
  });

  it('renders clean rule cards (example, linked entity pills, Live GET/Live MCP badges, Enabled switch) without dumping raw regex', async () => {
    TestBed.configureTestingModule({
      imports: [PatternsDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(PatternsDialogComponent);
    await fixture.componentInstance.refresh();
    await fixture.whenStable();

    const card = fixture.nativeElement.querySelector('.rule-card') as HTMLElement;
    expect(card.textContent).toContain('Example: MSCU1234565');
    expect(card.textContent).toContain('ocean-carrier:{{bic}}');
    expect(card.textContent).toContain('Live GET');
    expect(card.textContent).toContain('Live MCP');
    expect(card.textContent).not.toContain('(?P<bic>');

    // Per-rule Enabled toggle switch persists via upsertPatternRule
    const toggleSwitch = card.querySelector('button[role="switch"]') as HTMLButtonElement;
    expect(toggleSwitch.getAttribute('aria-checked')).toBe('true');

    bridgeMock.upsertPatternRule.mockResolvedValueOnce([{ ...sampleRule, enabled: false }]);
    toggleSwitch.click();
    await fixture.whenStable();

    expect(bridgeMock.upsertPatternRule).toHaveBeenCalledWith({
      ...sampleRule,
      enabled: false,
    });
  });

  it('tucks raw regex + JSON editor inside collapsible Edit JSON disclosure with inline validator errors', async () => {
    TestBed.configureTestingModule({
      imports: [PatternsDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(PatternsDialogComponent);
    await fixture.componentInstance.refresh();
    await fixture.whenStable();

    // Collapsed by default
    expect(fixture.nativeElement.querySelector('#pattern-json-editor')).toBeNull();

    // Clicking Edit JSON opens disclosure and loads rule
    fixture.componentInstance.loadIntoEditor(sampleRule);
    await fixture.whenStable();

    const textarea = fixture.nativeElement.querySelector('#pattern-json-editor') as HTMLTextAreaElement;
    expect(textarea).not.toBeNull();
    expect(textarea.value).toContain('"id": "iso-container"');
    expect(fixture.nativeElement.querySelector('.regex-code')?.textContent).toContain('(?P<bic>');

    // Invalid layout triggers inline validator error and disables Save
    fixture.componentInstance.onDraftChange(
      JSON.stringify({ ...sampleRule, layout: 'legacy-invalid' })
    );
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.validation-errors')?.textContent).toContain(
      'Invalid layout'
    );

    // Inline secret in liveHttp.auth triggers inline validator error
    fixture.componentInstance.onDraftChange(
      JSON.stringify({
        ...sampleRule,
        liveHttp: {
          ...sampleRule.liveHttp,
          auth: 'Bearer ghp_secret123',
        },
      })
    );
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.validation-errors')?.textContent).toContain(
      'inline secrets forbidden'
    );

    // Valid draft saves cleanly and emits patternsChanged
    const changedSpy = vi.fn();
    fixture.componentInstance.patternsChanged.subscribe(changedSpy);
    fixture.componentInstance.loadIntoEditor(sampleRule);
    await fixture.componentInstance.saveDraft();
    await fixture.whenStable();

    expect(bridgeMock.upsertPatternRule).toHaveBeenCalledWith(sampleRule);
    expect(changedSpy).toHaveBeenCalledTimes(1);
  });

  it('focuses modal title (tabindex="-1") on open rather than the Close button', async () => {
    TestBed.configureTestingModule({
      imports: [PatternsDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(PatternsDialogComponent);
    const dialogEl = fixture.nativeElement.querySelector('dialog') as HTMLDialogElement;
    dialogEl.showModal = vi.fn(() => {
      dialogEl.setAttribute('open', '');
    });

    const titleEl = fixture.nativeElement.querySelector('#patterns-dialog-title') as HTMLElement;
    expect(titleEl.getAttribute('tabindex')).toBe('-1');
    const focusSpy = vi.spyOn(titleEl, 'focus');

    fixture.componentRef.setInput('open', true);
    await fixture.whenStable();

    expect(dialogEl.showModal).toHaveBeenCalled();
    expect(focusSpy).toHaveBeenCalledWith({ preventScroll: true });
  });
});
