import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AppComponent } from './app.component';
import { CardNode } from './models/card-node.model';
import { RolodexStateService } from './services/rolodex-state.service';
import { VexBridgeService } from './services/vex-bridge.service';

describe('AppComponent', () => {
  const makeFactsCard = (id: string, title: string): CardNode => ({
    id,
    layout: 'facts',
    icon: '📄',
    title,
    chips: [],
    data: { facts: [{ label: 'Code', value: id }] },
  });

  const makeLinkCard = (id: string, title: string, url: string): CardNode => ({
    id,
    layout: 'link',
    icon: '🌐',
    title,
    chips: [],
    data: { url },
  });

  const makeConvertCard = (id: string, output: string): CardNode => ({
    id,
    layout: 'convert',
    icon: '⚖️',
    title: `Convert = ${output}`,
    chips: [],
    data: { input: '10 kg', output },
  });

  let bridgeMock: {
    isDesktop: ReturnType<typeof signal<boolean>>;
    getUserPrefs: ReturnType<typeof vi.fn>;
    searchRolodex: ReturnType<typeof vi.fn>;
    listRecentCards: ReturnType<typeof vi.fn>;
    recordCardUsed: ReturnType<typeof vi.fn>;
    listCollections: ReturnType<typeof vi.fn>;
    getLogInfo: ReturnType<typeof vi.fn>;
    listModules: ReturnType<typeof vi.fn>;
    listStarterModules: ReturnType<typeof vi.fn>;
    listPatternRules: ReturnType<typeof vi.fn>;
    hideWindow: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    localStorage.clear();
    bridgeMock = {
      isDesktop: signal<boolean>(true),
      getUserPrefs: vi.fn().mockResolvedValue({ onboardingCompleted: true, powerUserMode: false }),
      searchRolodex: vi.fn().mockResolvedValue([]),
      listRecentCards: vi.fn().mockResolvedValue([]),
      recordCardUsed: vi.fn().mockResolvedValue(undefined),
      listCollections: vi.fn().mockResolvedValue([
        { collection: 'air-carriers', cardCount: 48 },
        { collection: 'containers', cardCount: 12 },
      ]),
      getLogInfo: vi.fn().mockResolvedValue({ filePath: '', level: 'info', sizeBytes: 0, metricsAddr: '' }),
      listModules: vi.fn().mockResolvedValue([]),
      listStarterModules: vi.fn().mockResolvedValue([]),
      listPatternRules: vi.fn().mockResolvedValue([]),
      hideWindow: vi.fn().mockResolvedValue(undefined),
    };
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('renders Pinned + Recent sections (deduplicated) and subtle total indexed count when search is empty', async () => {
    const pinnedCard = makeFactsCard('air-176', 'Emirates SkyCargo (EK)');
    const recentCard = makeFactsCard('guide-hbl', 'What is an HBL?');
    bridgeMock.listRecentCards.mockResolvedValue([pinnedCard, recentCard]);

    TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });

    const state = TestBed.inject(RolodexStateService);
    state.togglePin(pinnedCard);

    const fixture = TestBed.createComponent(AppComponent);
    await fixture.whenStable();
    await fixture.componentInstance.refreshResults();
    await fixture.whenStable();

    expect(bridgeMock.listRecentCards).toHaveBeenCalledWith(6);
    expect(bridgeMock.listCollections).toHaveBeenCalled();

    const hint = fixture.nativeElement.querySelector('.bar-hint')?.textContent?.trim();
    expect(hint).toContain('60 indexed cards');

    const sections = fixture.nativeElement.querySelectorAll('.section-meta');
    expect(sections.length).toBe(2);
    expect(sections[0].textContent?.trim()).toBe('Pinned (1/3)');
    expect(sections[1].textContent?.trim()).toBe('Recent');

    // Pinned card must not be duplicated inside Recent
    expect(fixture.componentInstance.pinnedEmptyCards().map((c) => c.id)).toEqual(['air-176']);
    expect(fixture.componentInstance.recentEmptyCards().map((c) => c.id)).toEqual(['guide-hbl']);
  });

  it('requests limit 10 for active queries and orders pinned cards ahead of unpinned results', async () => {
    bridgeMock.searchRolodex.mockResolvedValue([
      makeFactsCard('exact-ams', 'Amsterdam Gateway (AMS)'),
      makeFactsCard('pinned-card', 'Pinned Runbook'),
      makeFactsCard('ctx-edi', 'AMS EDI Servers'),
    ]);

    TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });

    const state = TestBed.inject(RolodexStateService);
    state.togglePin('pinned-card');

    const fixture = TestBed.createComponent(AppComponent);
    await fixture.whenStable();

    fixture.componentInstance.query.set('AMS');
    await fixture.componentInstance.refreshResults();
    await fixture.whenStable();

    expect(bridgeMock.searchRolodex).toHaveBeenLastCalledWith('AMS', 10);
    expect(fixture.componentInstance.orderedSearchCards().map((c) => c.id)).toEqual([
      'pinned-card',
      'exact-ams',
      'ctx-edi',
    ]);
  });

  it('supports full keyboard navigation: ArrowDown/Up, Enter (link/convert/expand), and two-stage Escape', async () => {
    const linkCard = makeLinkCard('lnk-1', 'ACE Portal', 'https://ace.cbp.dhs.gov');
    const convCard = makeConvertCard('conv-1', '22.05 lb');
    const factsCard = makeFactsCard('facts-1', 'Carrier 176');

    bridgeMock.searchRolodex.mockResolvedValue([linkCard, convCard, factsCard]);

    TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });

    const state = TestBed.inject(RolodexStateService);
    const openSpy = vi.spyOn(state, 'openExternalUrl').mockImplementation(() => {});
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    const fixture = TestBed.createComponent(AppComponent);
    await fixture.whenStable();

    fixture.componentInstance.query.set('test');
    await fixture.componentInstance.refreshResults();
    await fixture.whenStable();

    const searchInput = fixture.nativeElement.querySelector('#rolodex-search') as HTMLInputElement;

    // Initial selection is index 0 (linkCard) -> Enter opens URL
    expect(fixture.componentInstance.selectedIndex()).toBe(0);
    searchInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    expect(openSpy).toHaveBeenCalledWith('https://ace.cbp.dhs.gov');
    expect(bridgeMock.recordCardUsed).toHaveBeenCalledWith('lnk-1');

    // ArrowDown moves to index 1 (convCard) -> Enter copies converted output
    searchInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    await fixture.whenStable();
    expect(fixture.componentInstance.selectedIndex()).toBe(1);
    searchInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    expect(copySpy).toHaveBeenCalledWith('Converted Value', '22.05 lb');

    // ArrowDown moves to index 2 (factsCard) -> Enter expands card
    searchInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    await fixture.whenStable();
    expect(fixture.componentInstance.selectedIndex()).toBe(2);
    searchInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('app-facts-card')).not.toBeNull();

    // ArrowUp moves back to index 1
    searchInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true }));
    await fixture.whenStable();
    expect(fixture.componentInstance.selectedIndex()).toBe(1);

    // First Escape clears query instead of hiding window
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await fixture.whenStable();
    expect(fixture.componentInstance.query()).toBe('');
    expect(bridgeMock.hideWindow).not.toHaveBeenCalled();

    // Second Escape (with empty query) hides window
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(bridgeMock.hideWindow).toHaveBeenCalledTimes(1);
  });

  it('auto-expands the card immediately when a search returns exactly 1 result', async () => {
    bridgeMock.searchRolodex.mockResolvedValue([makeFactsCard('air-176', 'Emirates SkyCargo (EK)')]);

    TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });

    const fixture = TestBed.createComponent(AppComponent);
    await fixture.whenStable();

    fixture.componentInstance.query.set('176');
    await fixture.componentInstance.refreshResults();
    await fixture.whenStable();

    expect(fixture.nativeElement.querySelector('app-facts-card')).not.toBeNull();
  });
});
