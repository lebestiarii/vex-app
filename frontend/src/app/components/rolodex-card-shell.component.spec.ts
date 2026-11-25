import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CardNode } from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { VexBridgeService } from '../services/vex-bridge.service';
import { RolodexCardShellComponent } from './rolodex-card-shell.component';

describe('RolodexCardShellComponent', () => {
  let bridgeMock: {
    recordCardUsed: ReturnType<typeof vi.fn>;
    fetchPatternLiveHTTP: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    localStorage.clear();
    bridgeMock = {
      recordCardUsed: vi.fn().mockResolvedValue(undefined),
      fetchPatternLiveHTTP: vi.fn().mockResolvedValue(null),
    };
  });

  afterEach(() => {
    localStorage.clear();
  });

  async function renderShell(node: CardNode<any>, depth = 0, selected = false) {
    TestBed.configureTestingModule({
      imports: [RolodexCardShellComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(RolodexCardShellComponent);
    fixture.componentRef.setInput('node', node);
    fixture.componentRef.setInput('depth', depth);
    fixture.componentRef.setInput('selected', selected);
    await fixture.whenStable();
    return fixture;
  }

  it('renders the pin button ONLY on Level 0 pinnable cards and highlights active pins on link cards too', async () => {
    const linkNode: CardNode = {
      id: 'link-l0',
      layout: 'link',
      icon: '🌐',
      title: 'CBP ACE Portal',
      chips: [{ label: 'PROD', tone: 'red', dot: 'red' }],
      data: { url: 'https://ace.cbp.dhs.gov' },
    };

    const l0 = await renderShell(linkNode, 0);
    const pinBtn = l0.nativeElement.querySelector('.pin-btn') as HTMLElement | null;
    expect(pinBtn).not.toBeNull();
    expect(pinBtn!.classList.contains('pinned')).toBe(false);
    expect(pinBtn!.querySelector('.pin-glyph')?.classList.contains('muted')).toBe(true);

    pinBtn!.click();
    await l0.whenStable();
    expect(pinBtn!.classList.contains('pinned')).toBe(true);
    expect(pinBtn!.querySelector('.pin-glyph')?.classList.contains('muted')).toBe(false);

    TestBed.resetTestingModule();
    const l1 = await renderShell(linkNode, 1);
    expect(l1.nativeElement.querySelector('.pin-btn')).toBeNull();

    TestBed.resetTestingModule();
    const unpinnable = await renderShell({ ...linkNode, pinnable: false }, 0);
    expect(unpinnable.nativeElement.querySelector('.pin-btn')).toBeNull();
  });

  it('renders Link layout as a Single-Button clickable card without chevron, opens URL, and records usage', async () => {
    const node: CardNode = {
      id: 'single-link-1',
      layout: 'link',
      icon: '🌐',
      title: 'CBP ACE Portal',
      chips: [{ label: 'PROD', tone: 'red', dot: 'red' }],
      data: { url: 'https://ace.cbp.dhs.gov' },
    };

    const fixture = await renderShell(node, 1);
    const state = TestBed.inject(RolodexStateService);
    const openSpy = vi.spyOn(state, 'openExternalUrl').mockImplementation(() => {});

    const btn = fixture.nativeElement.querySelector('button.single-link-card') as HTMLButtonElement | null;
    expect(btn).not.toBeNull();
    expect(fixture.nativeElement.querySelector('.chevron')).toBeNull();
    expect(fixture.nativeElement.querySelector('.external-arrow')?.textContent?.trim()).toBe('↗');

    btn!.click();
    expect(openSpy).toHaveBeenCalledWith('https://ace.cbp.dhs.gov');
    expect(bridgeMock.recordCardUsed).toHaveBeenCalledWith('single-link-1');
  });

  it('omits the expand chevron whenever a card has neither leaf body nor children (zero empty accordions)', async () => {
    const emptyGroup: CardNode = {
      id: 'empty-group',
      layout: 'group',
      icon: '📁',
      title: 'Empty Group',
      chips: [],
      data: {},
      children: [],
    };
    const emptyFacts: CardNode = {
      id: 'empty-facts',
      layout: 'facts',
      icon: '📄',
      title: 'Empty Facts',
      chips: [],
      data: { facts: [] },
    };

    const gFix = await renderShell(emptyGroup, 0);
    expect(gFix.nativeElement.querySelector('.chevron')).toBeNull();
    expect(gFix.nativeElement.querySelector('button.title-trigger')).toBeNull();
    expect(gFix.componentInstance.isExpandable()).toBe(false);

    TestBed.resetTestingModule();
    const fFix = await renderShell(emptyFacts, 0);
    expect(fFix.nativeElement.querySelector('.chevron')).toBeNull();
    expect(fFix.componentInstance.isExpandable()).toBe(false);
  });

  it('keeps group cards strictly header-only and caps recursive nesting at depth 2', async () => {
    const deepTree: CardNode = {
      id: 'group-ams',
      layout: 'group',
      icon: '🇳🇱',
      title: 'Amsterdam Gateway (AMS)',
      defaultExpanded: true,
      chips: [{ label: 'NLAMS · AMS', tone: 'blue' }],
      data: { meta: { ianaTimezone: 'Europe/Amsterdam' } },
      children: [
        {
          id: 'child-l1',
          layout: 'group',
          icon: '📦',
          title: 'Level 1 Group',
          defaultExpanded: true,
          chips: [],
          data: {},
          children: [
            {
              id: 'child-l2',
              layout: 'facts',
              icon: '🔹',
              title: 'Level 2 Leaf',
              defaultExpanded: true,
              chips: [],
              data: { facts: [{ label: 'Host', value: 'leaf.internal', mono: true }] },
              children: [
                {
                  id: 'child-l3-ignored',
                  layout: 'facts',
                  icon: '🚫',
                  title: 'Level 3 Should Not Render',
                  defaultExpanded: true,
                  chips: [],
                  data: { facts: [{ label: 'Hidden', value: 'true' }] },
                },
              ],
            },
          ],
        },
      ],
    };

    const fixture = await renderShell(deepTree, 0);
    const rootArticle = fixture.nativeElement.querySelector('article[data-depth="0"]') as HTMLElement;
    expect(rootArticle).not.toBeNull();
    expect(rootArticle.querySelector(':scope > .expanded-body > .leaf-slot')).toBeNull();

    expect(fixture.nativeElement.querySelector('[data-depth="1"]')).not.toBeNull();
    expect(fixture.nativeElement.querySelector('[data-depth="2"]')).not.toBeNull();
    expect(fixture.nativeElement.textContent).toContain('Level 2 Leaf');
    expect(fixture.nativeElement.textContent).not.toContain('Level 3 Should Not Render');
  });

  it('renders live IANA clock chips as monospace HH:MM across any layout', async () => {
    const factsNode: CardNode = {
      id: 'facts-clock',
      layout: 'facts',
      icon: '🕒',
      title: 'UTC Hub',
      chips: [
        { label: 'Etc/UTC', tone: 'neutral' },
        { label: 'HQ', tone: 'blue' },
      ],
      data: { facts: [{ label: 'Code', value: 'UTC' }] },
    };

    const fixture = await renderShell(factsNode, 0);
    const state = TestBed.inject(RolodexStateService);
    state.clockNow.set(new Date('2026-11-24T09:42:00.000Z'));
    await fixture.whenStable();

    const chips = fixture.nativeElement.querySelectorAll('.glance-chip') as NodeListOf<HTMLElement>;
    expect(chips[0].textContent?.trim()).toBe('09:42');
    expect(chips[0].classList.contains('is-mono')).toBe(true);
    expect(chips[1].textContent?.trim()).toBe('HQ');
  });

  it('records usage on expand, supports linkedSignal reset, and triggers primary actions for convert/expandable', async () => {
    const convertNode: CardNode = {
      id: 'conv-1',
      layout: 'convert',
      icon: '⚖️',
      title: '10 kg = 22.05 lb',
      defaultExpanded: false,
      chips: [],
      data: { input: '10 kg', output: '22.05 lb' },
    };

    const fixture = await renderShell(convertNode, 0, true);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    expect(fixture.nativeElement.querySelector('.rolodex-card')?.classList.contains('selected')).toBe(true);

    // Primary action on convert copies output + records usage
    fixture.componentInstance.triggerPrimaryAction();
    expect(copySpy).toHaveBeenCalledWith('Converted Value', '22.05 lb');
    expect(bridgeMock.recordCardUsed).toHaveBeenCalledWith('conv-1');

    // Expanding toggles state and records usage only when opening
    bridgeMock.recordCardUsed.mockClear();
    fixture.componentInstance.toggleExpanded();
    await fixture.whenStable();
    expect(fixture.componentInstance.isExpanded()).toBe(true);
    expect(bridgeMock.recordCardUsed).toHaveBeenCalledWith('conv-1');

    bridgeMock.recordCardUsed.mockClear();
    fixture.componentInstance.toggleExpanded();
    await fixture.whenStable();
    expect(fixture.componentInstance.isExpanded()).toBe(false);
    expect(bridgeMock.recordCardUsed).not.toHaveBeenCalled();

    // linkedSignal resets cleanly on new node input
    fixture.componentRef.setInput('node', { ...convertNode, id: 'conv-2', defaultExpanded: true });
    await fixture.whenStable();
    expect(fixture.componentInstance.isExpanded()).toBe(true);
  });

  it('hydrates manual liveHttp child card on click and degrades gracefully with muted unavailable chip on error', async () => {
    const segNode: CardNode = {
      id: 'pattern-github-issue-pr-1',
      layout: 'segments',
      icon: '🐙',
      title: 'GitHub · octo/vex#42',
      defaultExpanded: true,
      chips: [{ label: 'Issue / PR', tone: 'blue' }],
      data: {
        segments: [
          { value: 'octo', role: 'OWNER', meaning: 'Org', tone: 'blue' },
          { value: 'vex', role: 'REPO', meaning: 'Repo', tone: 'amber' },
          { value: '#42', role: 'NUMBER', meaning: 'Issue', tone: 'green' },
        ],
        explanation: 'GitHub issue reference.',
      },
      actions: [
        {
          id: 'fetch-live-http',
          label: 'Fetch Live Data',
          type: 'http',
          target: 'https://api.github.com/repos/octo/vex/issues/42',
          env: {
            liveHttpRuleId: 'github-issue-pr',
            liveHttpRawQuery: 'octo/vex#42',
            liveHttpAuth: 'gh_cli',
          },
        },
      ],
    };

    const hydratedNode: CardNode = {
      ...segNode,
      actions: [],
      children: [
        {
          id: 'pattern-github-issue-pr-1-live-http',
          layout: 'facts',
          icon: '⚡',
          title: 'Add liveHttp GET support',
          defaultExpanded: true,
          chips: [{ label: 'open', tone: 'green' }],
          data: {
            summary: 'Opened by @octocat',
            facts: [{ label: 'State', value: 'open' }],
          },
        },
      ],
    };
    bridgeMock.fetchPatternLiveHTTP.mockResolvedValueOnce(hydratedNode);

    const fixture = await renderShell(segNode, 0);
    const fetchBtn = fixture.nativeElement.querySelector('.live-fetch-btn') as HTMLButtonElement | null;
    expect(fetchBtn).not.toBeNull();
    expect(fetchBtn!.textContent).toContain('Fetch Live Data');

    fetchBtn!.click();
    await fixture.whenStable();

    expect(bridgeMock.fetchPatternLiveHTTP).toHaveBeenCalledWith('github-issue-pr', 'octo/vex#42');
    expect(fixture.nativeElement.textContent).toContain('Add liveHttp GET support');
    expect(fixture.nativeElement.querySelector('.live-fetch-btn')).toBeNull();

    // Failure case keeps local segments intact and appends muted Live lookup unavailable chip
    TestBed.resetTestingModule();
    bridgeMock.fetchPatternLiveHTTP.mockRejectedValueOnce(new Error('offline'));
    const failFix = await renderShell(segNode, 0);
    const failBtn = failFix.nativeElement.querySelector('.live-fetch-btn') as HTMLButtonElement;
    failBtn.click();
    await failFix.whenStable();

    expect(failFix.nativeElement.textContent).toContain('GitHub issue reference.');
    expect(failFix.nativeElement.textContent).toContain('Live lookup unavailable');
  });
});
