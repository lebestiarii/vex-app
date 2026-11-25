import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ActionListLayoutData,
  CardNode,
  ConvertLayoutData,
  FactsLayoutData,
  GroupLayoutData,
  LinkLayoutData,
  SegmentsLayoutData,
  SnippetLayoutData,
  StepsLayoutData,
  TableLayoutData,
} from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { VexBridgeService } from '../services/vex-bridge.service';
import { ActionListCardComponent } from './action-list-card.component';
import { ConvertCardComponent } from './convert-card.component';
import { FactsCardComponent } from './facts-card.component';
import { GroupCardComponent } from './group-card.component';
import { LinkCardComponent } from './link-card.component';
import { SegmentsCardComponent } from './segments-card.component';
import { SnippetCardComponent } from './snippet-card.component';
import { StepsCardComponent } from './steps-card.component';
import { TableCardComponent } from './table-card.component';

describe('9 Pure Generic Layout Components', () => {
  let bridgeMock: { recordCardUsed: ReturnType<typeof vi.fn> };

  beforeEach(() => {
    localStorage.clear();
    bridgeMock = {
      recordCardUsed: vi.fn().mockResolvedValue(undefined),
    };
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('1. GroupCardComponent renders zero leaf body', async () => {
    TestBed.configureTestingModule({
      imports: [GroupCardComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(GroupCardComponent);
    const node: CardNode<GroupLayoutData> = {
      id: 'grp-1',
      layout: 'group',
      icon: '📦',
      title: '40ft High Cube',
      chips: [],
      data: { meta: { iso: '45G1' } },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();
    expect(fixture.nativeElement.textContent?.trim()).toBe('');
  });

  it('2. LinkCardComponent opens external URL and records usage when instantiated directly', async () => {
    TestBed.configureTestingModule({
      imports: [LinkCardComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(LinkCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const openSpy = vi.spyOn(state, 'openExternalUrl').mockImplementation(() => {});

    const node: CardNode<LinkLayoutData> = {
      id: 'lnk-1',
      layout: 'link',
      icon: '🌐',
      title: 'SkyCargo Track',
      chips: [],
      data: { url: 'https://www.skycargo.com/track' },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    const btn = fixture.nativeElement.querySelector('button.link-fallback-btn') as HTMLButtonElement;
    btn.click();
    expect(openSpy).toHaveBeenCalledWith('https://www.skycargo.com/track');
    expect(bridgeMock.recordCardUsed).toHaveBeenCalledWith('lnk-1');
  });

  it('3. ActionListCardComponent renders stoplight badges, mono primary, subtitle, and copy/open_url buttons', async () => {
    TestBed.configureTestingModule({
      imports: [ActionListCardComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(ActionListCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();
    const openSpy = vi.spyOn(state, 'openExternalUrl').mockImplementation(() => {});

    const node: CardNode<ActionListLayoutData> = {
      id: 'act-1',
      layout: 'action-list',
      icon: '🖥️',
      title: 'AMS EDI Servers',
      chips: [],
      data: {
        rows: [
          {
            badge: { label: 'DEV', tone: 'green', dot: 'green' },
            primary: 'ams-dev-edi01.local:2222',
            mono: true,
            subtitle: 'Sandbox SFTP',
            actions: [
              { label: 'Copy', type: 'copy', value: 'ams-dev-edi01.local' },
              { label: 'SSH', type: 'copy', value: 'ssh ops@ams-dev-edi01.local -p 2222' },
              { label: 'Portal', type: 'open_url', value: 'https://ams-dev-edi01.local', tone: 'green', dot: 'green' },
            ],
          },
        ],
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    expect(fixture.nativeElement.querySelector('.row-badge')?.textContent?.trim()).toBe('DEV');
    expect(fixture.nativeElement.querySelector('.row-primary')?.classList.contains('font-mono')).toBe(true);
    expect(fixture.nativeElement.querySelector('.row-subtitle')?.textContent?.trim()).toBe('Sandbox SFTP');

    const btns = fixture.nativeElement.querySelectorAll('button.row-action-btn') as NodeListOf<HTMLButtonElement>;
    expect(btns.length).toBe(3);

    btns[0].click();
    expect(copySpy).toHaveBeenCalledWith('DEV Copy', 'ams-dev-edi01.local');
    btns[1].click();
    expect(copySpy).toHaveBeenCalledWith('DEV SSH', 'ssh ops@ams-dev-edi01.local -p 2222');
    btns[2].click();
    expect(openSpy).toHaveBeenCalledWith('https://ams-dev-edi01.local');
    expect(bridgeMock.recordCardUsed).toHaveBeenCalledWith('act-1');
  });

  it('4. FactsCardComponent switches views[] and renders [Copy] ONLY when fact.copyable === true', async () => {
    TestBed.configureTestingModule({
      imports: [FactsCardComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(FactsCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    const node: CardNode<FactsLayoutData> = {
      id: 'facts-1',
      layout: 'facts',
      icon: '📐',
      title: '40ft HC Dimensions',
      chips: [],
      data: {
        summary: 'Internal & door clearance.',
        views: [
          {
            label: 'Metric',
            facts: [
              { label: 'Length', value: '12.03 m', mono: true },
              { label: 'Spec Sheet', value: 'ISO-45G1-1203x235x269', mono: true, copyable: true },
            ],
          },
          {
            label: 'Imperial',
            facts: [
              { label: 'Length', value: `39' 6"`, mono: true },
            ],
          },
        ],
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    expect(fixture.nativeElement.querySelector('.facts-summary')?.textContent?.trim()).toBe(
      'Internal & door clearance.'
    );
    // Only the 1 copyable fact gets a Copy button
    const copyBtns = fixture.nativeElement.querySelectorAll('button.fact-copy-btn') as NodeListOf<HTMLButtonElement>;
    expect(copyBtns.length).toBe(1);
    copyBtns[0].click();
    expect(copySpy).toHaveBeenCalledWith('Spec Sheet', 'ISO-45G1-1203x235x269');
    expect(bridgeMock.recordCardUsed).toHaveBeenCalledWith('facts-1');

    // Switch to Imperial view
    const viewTabs = fixture.nativeElement.querySelectorAll('button.view-pill') as NodeListOf<HTMLButtonElement>;
    expect(viewTabs.length).toBe(2);
    viewTabs[1].click();
    await fixture.whenStable();

    expect(fixture.nativeElement.textContent).toContain(`39' 6"`);
    expect(fixture.nativeElement.querySelectorAll('button.fact-copy-btn').length).toBe(0);
  });

  it('5. SegmentsCardComponent renders 2-4 Blue/Amber/Green tiles, click-to-copy, and explanation', async () => {
    TestBed.configureTestingModule({
      imports: [SegmentsCardComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(SegmentsCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    const node: CardNode<SegmentsLayoutData> = {
      id: 'seg-1',
      layout: 'segments',
      icon: '🧩',
      title: 'House Bill · 7031234567',
      chips: [],
      data: {
        segments: [
          { value: '7', role: 'MODE', meaning: 'Ocean Import', tone: 'blue' },
          { value: '03', role: 'BRANCH', meaning: 'Seattle SEA', tone: 'amber' },
          { value: '1234567', role: 'SEQUENCE', meaning: 'Shipment #', tone: 'green' },
        ],
        explanation: 'Forwarder-issued House Bill of Lading.',
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    const tiles = fixture.nativeElement.querySelectorAll('button.segment-tile') as NodeListOf<HTMLButtonElement>;
    expect(tiles.length).toBe(3);
    expect(tiles[0].getAttribute('data-tone')).toBe('blue');
    expect(tiles[1].getAttribute('data-tone')).toBe('amber');
    expect(tiles[2].getAttribute('data-tone')).toBe('green');
    expect(fixture.nativeElement.querySelector('.segments-explanation')?.textContent?.trim()).toBe(
      'Forwarder-issued House Bill of Lading.'
    );

    tiles[1].click();
    expect(copySpy).toHaveBeenCalledWith('BRANCH', '03');
  });

  it('6. StepsCardComponent renders 1-based timeline, active/completed states, summary, and bullets', async () => {
    TestBed.configureTestingModule({
      imports: [StepsCardComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(StepsCardComponent);

    const node: CardNode<StepsLayoutData> = {
      id: 'steps-1',
      layout: 'steps',
      icon: '📖',
      title: 'What is an HBL?',
      chips: [],
      data: {
        activeStep: 2,
        steps: [
          { label: 'Booking', caption: 'Reserved' },
          { label: 'Origin HBL', caption: 'Issued' },
          { label: 'Transit', caption: 'Vessel' },
        ],
        summary: 'Issued by a freight forwarder.',
        bullets: ['Must match ISF filing'],
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    const items = fixture.nativeElement.querySelectorAll('li.timeline-step') as NodeListOf<HTMLLIElement>;
    expect(items.length).toBe(3);
    expect(items[0].classList.contains('completed')).toBe(true);
    expect(items[1].classList.contains('active')).toBe(true);
    expect(items[1].getAttribute('aria-current')).toBe('step');
    expect(items[2].classList.contains('completed')).toBe(false);
    expect(fixture.nativeElement.querySelector('.steps-summary')?.textContent?.trim()).toBe(
      'Issued by a freight forwarder.'
    );
    expect(fixture.nativeElement.querySelectorAll('.steps-bullets li').length).toBe(1);
  });

  it('7. ConvertCardComponent renders Input ⇄ Output strip, Copy button, and equivalent pills', async () => {
    TestBed.configureTestingModule({
      imports: [ConvertCardComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(ConvertCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    const node: CardNode<ConvertLayoutData> = {
      id: 'conv-1',
      layout: 'convert',
      icon: '⚖️',
      title: '100 kg = 220.46 lb',
      chips: [],
      data: {
        input: '100 kg',
        output: '220.46 lb',
        equivalents: [
          { label: 'Metric Ton', value: '0.10 t' },
          { label: 'Ounces', value: '3,527.40 oz' },
        ],
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    expect(fixture.nativeElement.querySelector('.conv-input')?.textContent?.trim()).toBe('100 kg');
    expect(fixture.nativeElement.querySelector('.conv-output')?.textContent?.trim()).toBe('220.46 lb');

    (fixture.nativeElement.querySelector('button.copy-btn') as HTMLButtonElement).click();
    expect(copySpy).toHaveBeenCalledWith('Converted Value', '220.46 lb');

    const equivs = fixture.nativeElement.querySelectorAll('button.equiv-chip') as NodeListOf<HTMLButtonElement>;
    equivs[0].click();
    expect(copySpy).toHaveBeenCalledWith('Metric Ton', '0.10 t');
  });

  it('8. TableCardComponent renders mono columns and exports RFC-4180 CSV & GFM Markdown', async () => {
    TestBed.configureTestingModule({
      imports: [TableCardComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(TableCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    const node: CardNode<TableLayoutData> = {
      id: 'tbl-1',
      layout: 'table',
      icon: '📊',
      title: 'Container Matrix',
      chips: [],
      data: {
        columns: ['Code', 'Type', 'Notes'],
        rows: [
          ['22G1', `20' Dry`, 'Standard, dry'],
          ['45G1', `40' HC`, 'Extra | height'],
        ],
        monoColumns: [0],
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    const firstRowCells = fixture.nativeElement.querySelectorAll('tbody tr:first-child td');
    expect(firstRowCells[0].classList.contains('font-mono')).toBe(true);
    expect(firstRowCells[1].classList.contains('font-mono')).toBe(false);

    const csvBtn = fixture.nativeElement.querySelector('button[aria-label="Copy table as CSV"]') as HTMLButtonElement;
    csvBtn.click();
    expect(copySpy).toHaveBeenCalledWith(
      'CSV',
      `Code,Type,Notes\n22G1,20' Dry,"Standard, dry"\n45G1,40' HC,Extra | height`
    );

    const mdBtn = fixture.nativeElement.querySelector('button[aria-label="Copy table as Markdown"]') as HTMLButtonElement;
    mdBtn.click();
    expect(copySpy).toHaveBeenCalledWith(
      'Markdown',
      `| Code | Type | Notes |\n| --- | --- | --- |\n| 22G1 | 20' Dry | Standard, dry |\n| 45G1 | 40' HC | Extra \\| height |`
    );
  });

  it('9. SnippetCardComponent live-interpolates {{vars}} and copies rendered monospace block', async () => {
    TestBed.configureTestingModule({
      imports: [SnippetCardComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(SnippetCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    const node: CardNode<SnippetLayoutData> = {
      id: 'snip-1',
      layout: 'snippet',
      icon: '💻',
      title: 'Kubectl Pod Logs',
      chips: [],
      data: {
        language: 'bash',
        body: 'kubectl logs -n {{namespace}} {{pod}} --tail=200',
        vars: [
          { key: 'namespace', label: 'Namespace', defaultValue: 'prod' },
          { key: 'pod', label: 'Pod Name', defaultValue: 'api-0' },
        ],
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    expect(fixture.nativeElement.querySelector('.code-pre')?.textContent?.trim()).toBe(
      'kubectl logs -n prod api-0 --tail=200'
    );

    const inputs = fixture.nativeElement.querySelectorAll('input.var-input') as NodeListOf<HTMLInputElement>;
    expect(inputs.length).toBe(2);
    inputs[0].value = 'staging';
    inputs[0].dispatchEvent(new Event('input'));
    inputs[1].value = 'worker-3';
    inputs[1].dispatchEvent(new Event('input'));
    await fixture.whenStable();

    expect(fixture.nativeElement.querySelector('.code-pre')?.textContent?.trim()).toBe(
      'kubectl logs -n staging worker-3 --tail=200'
    );

    (fixture.nativeElement.querySelector('button.copy-btn') as HTMLButtonElement).click();
    expect(copySpy).toHaveBeenCalledWith(
      'Kubectl Pod Logs',
      'kubectl logs -n staging worker-3 --tail=200'
    );
    expect(bridgeMock.recordCardUsed).toHaveBeenCalledWith('snip-1');
  });
});
