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
    clearRecentCards: ReturnType<typeof vi.fn>;
    listCollections: ReturnType<typeof vi.fn>;
    getLogInfo: ReturnType<typeof vi.fn>;
    listModules: ReturnType<typeof vi.fn>;
    listStarterModules: ReturnType<typeof vi.fn>;
    listPatternRules: ReturnType<typeof vi.fn>;
    setUserPrefs: ReturnType<typeof vi.fn>;
    getStayAwake: ReturnType<typeof vi.fn>;
    setStayAwake: ReturnType<typeof vi.fn>;
    hideWindow: ReturnType<typeof vi.fn>;
    resizeWindow: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute('data-motion');
    bridgeMock = {
      isDesktop: signal<boolean>(true),
      getUserPrefs: vi.fn().mockResolvedValue({
        onboardingCompleted: true,
        powerUserMode: false,
        uiSize: 480,
        theme: 'system',
        animationsEnabled: true,
        stayAwake: false,
      }),
      searchRolodex: vi.fn().mockResolvedValue([]),
      listRecentCards: vi.fn().mockResolvedValue([]),
      recordCardUsed: vi.fn().mockResolvedValue(undefined),
      clearRecentCards: vi.fn().mockResolvedValue(undefined),
      listCollections: vi.fn().mockResolvedValue([
        { collection: 'air-carriers', cardCount: 48 },
        { collection: 'containers', cardCount: 12 },
      ]),
      getLogInfo: vi.fn().mockResolvedValue({ filePath: '', level: 'info', sizeBytes: 0, metricsAddr: '' }),
      listModules: vi.fn().mockResolvedValue([
        { id: 'air-carriers', name: 'Air Carriers', enabled: true, cardCount: 48 },
        { id: 'containers', name: 'Containers', enabled: true, cardCount: 12 },
      ]),
      listStarterModules: vi.fn().mockResolvedValue([]),
      listPatternRules: vi.fn().mockResolvedValue([
        { id: 'awb', name: 'AWB', enabled: true },
        { id: 'disabled-rule', name: 'Disabled', enabled: false },
      ]),
      setUserPrefs: vi.fn().mockResolvedValue(undefined),
      getStayAwake: vi.fn().mockResolvedValue(false),
      setStayAwake: vi.fn().mockResolvedValue(undefined),
      hideWindow: vi.fn().mockResolvedValue(undefined),
      resizeWindow: vi.fn().mockResolvedValue(undefined),
    };
  });

  afterEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute('data-motion');
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

    const hint = fixture.nativeElement.querySelector('.footer-count')?.textContent?.trim();
    expect(hint).toContain('60 indexed');

    const sections = fixture.nativeElement.querySelectorAll('.section-meta');
    expect(sections.length).toBe(2);
    expect(sections[0].textContent?.trim()).toBe('Pinned (1/3)');
    expect(sections[1].querySelector('span')?.textContent?.trim()).toBe('Recent');
    expect(sections[1].querySelector('.section-clear-btn')?.textContent?.trim()).toBe('Clear');

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

  it('clears recent cards via inline Clear button, toggles settings popover, and refocuses search on window focus', async () => {
    bridgeMock.listRecentCards.mockResolvedValue([makeFactsCard('guide-hbl', 'What is an HBL?')]);

    TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });

    const fixture = TestBed.createComponent(AppComponent);
    await fixture.whenStable();
    await fixture.componentInstance.refreshResults();
    await fixture.whenStable();

    // Clear recents via inline Clear button
    const clearRecentsBtn = fixture.nativeElement.querySelector('.section-clear-btn') as HTMLButtonElement | null;
    expect(clearRecentsBtn).not.toBeNull();
    clearRecentsBtn?.click();
    await fixture.whenStable();
    expect(bridgeMock.clearRecentCards).toHaveBeenCalledTimes(1);
    expect(fixture.componentInstance.recentEmptyCards()).toEqual([]);

    // Open in-place settings popover via gear button and close on Escape without hiding window
    const gearBtn = fixture.nativeElement.querySelector('button[aria-label="Settings"]') as HTMLButtonElement | null;
    expect(gearBtn).not.toBeNull();
    gearBtn?.click();
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.settings-popover.is-open')).not.toBeNull();

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.settings-popover.is-open')).toBeNull();
    expect(bridgeMock.hideWindow).not.toHaveBeenCalled();

    // Refocus search input on window focus
    const searchInput = fixture.nativeElement.querySelector('#rolodex-search') as HTMLInputElement;
    const focusSpy = vi.spyOn(searchInput, 'focus');
    window.dispatchEvent(new Event('focus'));
    expect(focusSpy).toHaveBeenCalled();
  });

  it('renders Stay Awake indicator left of Clear X inside search input, Quick Add + Settings popovers, and persists settings', async () => {
    TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });

    const fixture = TestBed.createComponent(AppComponent);
    await fixture.whenStable();

    // Initially no X and no Stay Awake indicator
    expect(fixture.nativeElement.querySelector('.input-clear-btn')).toBeNull();
    expect(fixture.nativeElement.querySelector('.stay-awake-indicator')).toBeNull();

    // Quick Add (+) popover toggles cleanly
    const quickAddBtn = fixture.nativeElement.querySelector('button[aria-label="Quick add"]') as HTMLButtonElement;
    quickAddBtn.click();
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.quick-add-popover.is-open')).not.toBeNull();

    // Opening Settings closes Quick Add and exposes 4 UI sizes (480 default), Theme, Animations, Stay Awake
    const gearBtn = fixture.nativeElement.querySelector('button[aria-label="Settings"]') as HTMLButtonElement;
    gearBtn.click();
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.quick-add-popover.is-open')).toBeNull();
    expect(fixture.nativeElement.querySelector('.settings-popover.is-open')).not.toBeNull();

    const sizeButtons = Array.from(
      fixture.nativeElement.querySelectorAll('.size-control .seg-btn')
    ) as HTMLButtonElement[];
    expect(sizeButtons.map((b) => b.textContent?.trim())).toEqual(['460', '480', '560', '620']);
    expect(fixture.componentInstance.state.uiSize()).toBe(480);

    // Select 560 UI Size
    sizeButtons[2].click();
    await fixture.whenStable();
    expect(fixture.componentInstance.state.uiSize()).toBe(560);
    expect(bridgeMock.setUserPrefs).toHaveBeenCalledWith(expect.objectContaining({ uiSize: 560 }));

    // Select Dark theme
    const themeButtons = Array.from(
      fixture.nativeElement.querySelectorAll('.theme-control .seg-btn')
    ) as HTMLButtonElement[];
    expect(themeButtons.map((b) => b.textContent?.trim())).toEqual(['Light', 'Dark', 'System']);
    themeButtons[1].click();
    await fixture.whenStable();
    expect(fixture.componentInstance.state.themePreference()).toBe('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');

    // Toggle Animations Off -> sets data-motion="off"
    const motionButtons = Array.from(
      fixture.nativeElement.querySelectorAll('.motion-control .seg-btn')
    ) as HTMLButtonElement[];
    motionButtons[1].click();
    await fixture.whenStable();
    expect(document.documentElement.getAttribute('data-motion')).toBe('off');

    // Enable Stay Awake and type a query -> verify coffee cup sits immediately left of Clear X inside .input-trailing
    const stayAwakeSwitch = fixture.nativeElement.querySelector('.stay-awake-row') as HTMLButtonElement;
    stayAwakeSwitch.click();
    fixture.componentInstance.query.set('176');
    await fixture.whenStable();

    expect(bridgeMock.setStayAwake).toHaveBeenCalledWith(true);
    const trailingButtons = Array.from(
      fixture.nativeElement.querySelectorAll('.input-trailing > button')
    ) as HTMLButtonElement[];
    expect(trailingButtons.length).toBe(2);
    expect(trailingButtons[0].classList.contains('stay-awake-indicator')).toBe(true);
    expect(trailingButtons[1].classList.contains('input-clear-btn')).toBe(true);
  });

  it('hides scope pills for <=1 module, shows and filters for 2+ modules, and auto-expands modals to 620x620 before restoring', async () => {
    const airCard: CardNode = { ...makeFactsCard('air-1', 'Emirates SkyCargo'), moduleId: 'air-carriers' };
    const boxCard: CardNode = { ...makeFactsCard('box-1', '40ft High Cube'), moduleId: 'containers' };

    TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });

    const fixture = TestBed.createComponent(AppComponent);
    await fixture.whenStable();

    // 1 module match -> scope pills remain closed
    bridgeMock.searchRolodex.mockResolvedValue([airCard]);
    fixture.componentInstance.query.set('emirates');
    await fixture.componentInstance.refreshResults();
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.scope-strip-wrap.is-open')).toBeNull();

    // 2+ modules match -> scope pills open and filter results
    bridgeMock.searchRolodex.mockResolvedValue([airCard, boxCard]);
    fixture.componentInstance.query.set('cargo');
    await fixture.componentInstance.refreshResults();
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.scope-strip-wrap.is-open')).not.toBeNull();

    const pills = Array.from(fixture.nativeElement.querySelectorAll('.scope-pill')) as HTMLButtonElement[];
    expect(pills.length).toBe(3); // All (2), Air Carriers (1), Containers (1)
    pills[2].click();
    await fixture.whenStable();
    expect(fixture.componentInstance.filteredSearchCards().map((c) => c.id)).toEqual(['box-1']);

    // Clearing query resets activeScope back to 'all'
    fixture.componentInstance.clearQuery();
    await fixture.whenStable();
    expect(fixture.componentInstance.activeScope()).toBe('all');

    // Modal auto-expand to 620x620 and restore to saved preset (460) on close
    fixture.componentInstance.selectUISize(460);
    await fixture.whenStable();
    expect(fixture.componentInstance.activeShellWidth()).toBe(460);

    fixture.componentInstance.openModulesFromMenu();
    await new Promise((resolve) => setTimeout(resolve, 90));
    await fixture.whenStable();
    expect(fixture.componentInstance.isModalExpanded()).toBe(true);
    expect(fixture.componentInstance.activeShellWidth()).toBe(620);
    expect(fixture.nativeElement.querySelector('.widget-frame.is-modal-expanded')).not.toBeNull();

    fixture.componentInstance.closeModules();
    await new Promise((resolve) => setTimeout(resolve, 330));
    await fixture.whenStable();
    expect(fixture.componentInstance.isModalExpanded()).toBe(false);
    expect(fixture.componentInstance.activeShellWidth()).toBe(460);
  });
});
