import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CardNode, PatternRule } from '../models/card-node.model';
import { VexBridgeService } from '../services/vex-bridge.service';
import { PatternsDialogComponent } from './patterns-dialog.component';

describe('PatternsDialogComponent', () => {
  const sampleRule: PatternRule = {
    id: 'test-ref',
    name: 'Test Ref Rule',
    regex: '^REF-(?P<num>\\d{4})$',
    titleTemplate: 'Ref {{num}}',
    template: 'pattern-breakdown',
  };

  let bridgeMock: {
    listPatternRules: ReturnType<typeof vi.fn>;
    upsertPatternRule: ReturnType<typeof vi.fn>;
    deletePatternRule: ReturnType<typeof vi.fn>;
    testPatternRule: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    localStorage.clear();
    bridgeMock = {
      listPatternRules: vi.fn().mockResolvedValue([sampleRule]),
      upsertPatternRule: vi.fn().mockResolvedValue([sampleRule]),
      deletePatternRule: vi.fn().mockResolvedValue([]),
      testPatternRule: vi.fn().mockResolvedValue(null),
    };
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('updates textarea reactively in zoneless mode when loadIntoEditor and resetDraft run', async () => {
    TestBed.configureTestingModule({
      imports: [PatternsDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(PatternsDialogComponent);
    await fixture.whenStable();

    const textarea = fixture.nativeElement.querySelector('#pattern-json-editor') as HTMLTextAreaElement;
    expect(textarea.value).toContain('"id": "custom-ref"');

    fixture.componentInstance.loadIntoEditor(sampleRule);
    await fixture.whenStable();
    expect(textarea.value).toContain('"id": "test-ref"');

    fixture.componentInstance.resetDraft();
    await fixture.whenStable();
    expect(textarea.value).toContain('"id": "custom-ref"');
  });

  it('runs live pattern test against mocked bridge and renders preview or error', async () => {
    const matchedCard: CardNode = {
      id: 'preview-test-ref',
      template: 'pattern-breakdown',
      icon: '🧩',
      title: 'Ref 1234',
      chips: [],
      data: {},
      children: [
        {
          id: 'nested-1',
          template: 'location',
          icon: '🌐',
          title: 'Linked',
          chips: [],
          data: {},
        },
      ],
    };
    bridgeMock.testPatternRule.mockResolvedValueOnce(matchedCard).mockResolvedValueOnce(null);

    TestBed.configureTestingModule({
      imports: [PatternsDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(PatternsDialogComponent);
    fixture.componentInstance.loadIntoEditor(sampleRule);
    fixture.componentInstance.testSample.set('REF-1234');
    await fixture.componentInstance.runTest();
    await fixture.whenStable();

    expect(bridgeMock.testPatternRule).toHaveBeenCalledWith(sampleRule, 'REF-1234');
    expect(fixture.nativeElement.querySelector('.preview-box')?.textContent).toContain('Ref 1234');
    expect(fixture.nativeElement.querySelector('.preview-sub')?.textContent).toContain(
      'Nests 1 linked card(s)'
    );

    // Non-matching sample renders error message
    fixture.componentInstance.testSample.set('NOPE');
    await fixture.componentInstance.runTest();
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.err-msg')?.textContent).toContain('No match for "NOPE"');
  });

  it('saves valid draft JSON, updates rules signal, and emits patternsChanged', async () => {
    TestBed.configureTestingModule({
      imports: [PatternsDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(PatternsDialogComponent);
    const changedSpy = vi.fn();
    fixture.componentInstance.patternsChanged.subscribe(changedSpy);

    fixture.componentInstance.loadIntoEditor(sampleRule);
    await fixture.componentInstance.saveDraft();
    await fixture.whenStable();

    expect(bridgeMock.upsertPatternRule).toHaveBeenCalledWith(sampleRule);
    expect(fixture.componentInstance.rules()).toEqual([sampleRule]);
    expect(changedSpy).toHaveBeenCalledTimes(1);
  });
});
