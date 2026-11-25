import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ModuleStatus, StarterModuleInfo } from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { VexBridgeService } from '../services/vex-bridge.service';
import { ModulesDialogComponent } from './modules-dialog.component';

describe('ModulesDialogComponent', () => {
  const installedMod: ModuleStatus = {
    id: 'air-carriers',
    name: 'Air Carrier IATA Prefix Index',
    category: 'Air Freight',
    description: '48 global air cargo carriers with hubs and official tracking links.',
    icon: '✈️',
    version: '2.0.0',
    enabled: true,
    defaultEnabled: true,
    cardCount: 48,
    overlayCount: 0,
    patternCount: 0,
  };

  const starterInstalled: StarterModuleInfo = {
    id: 'air-carriers',
    name: 'Air Carrier IATA Prefix Index',
    category: 'Air Freight',
    description: 'Already installed pack.',
    icon: '✈️',
    version: '2.0.0',
    defaultEnabled: true,
    installed: true,
    cardCount: 48,
    overlayCount: 0,
    patternCount: 0,
  };

  const starterAvailable: StarterModuleInfo = {
    id: 'containers',
    name: 'ISO Container Sizes & Blueprints',
    category: 'Equipment',
    description: 'ISO 6346 container dimensions and pallet capacities.',
    icon: '📦',
    version: '2.0.0',
    defaultEnabled: true,
    installed: false,
    cardCount: 16,
    overlayCount: 0,
    patternCount: 0,
  };

  let bridgeMock: {
    listModules: ReturnType<typeof vi.fn>;
    listStarterModules: ReturnType<typeof vi.fn>;
    installStarterModules: ReturnType<typeof vi.fn>;
    setEnabledModules: ReturnType<typeof vi.fn>;
    deleteModule: ReturnType<typeof vi.fn>;
    exportModuleJSON: ReturnType<typeof vi.fn>;
    getUserPrefs: ReturnType<typeof vi.fn>;
    setUserPrefs: ReturnType<typeof vi.fn>;
    getLogInfo: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    localStorage.clear();
    bridgeMock = {
      listModules: vi.fn().mockResolvedValue([installedMod]),
      listStarterModules: vi.fn().mockResolvedValue([starterInstalled, starterAvailable]),
      installStarterModules: vi.fn().mockResolvedValue([installedMod]),
      setEnabledModules: vi.fn().mockResolvedValue([{ ...installedMod, enabled: false }]),
      deleteModule: vi.fn().mockResolvedValue([]),
      exportModuleJSON: vi.fn().mockResolvedValue('{"id":"air-carriers"}'),
      getUserPrefs: vi.fn().mockResolvedValue({ onboardingCompleted: true, powerUserMode: false }),
      setUserPrefs: vi.fn().mockResolvedValue(undefined),
      getLogInfo: vi.fn().mockResolvedValue({
        filePath: '/tmp/vex.jsonl',
        level: 'info',
        sizeBytes: 120,
        metricsAddr: 'http://127.0.0.1:17380/metrics',
      }),
    };
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('separates Installed Modules from Starter Catalog (Install button only, zero checkboxes) and uses 3-row cards', async () => {
    TestBed.configureTestingModule({
      imports: [ModulesDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(ModulesDialogComponent);
    await fixture.componentInstance.refresh();
    await fixture.whenStable();

    // Zero confusing checkboxes anywhere in the dialog
    expect(fixture.nativeElement.querySelectorAll('input[type="checkbox"]').length).toBe(0);

    // Installed section has 1 card in 3-row structure
    const installedSection = fixture.nativeElement.querySelector('section[aria-label="Installed Modules"]') as HTMLElement;
    const installedCards = installedSection.querySelectorAll('.module-card');
    expect(installedCards.length).toBe(1);
    expect(installedCards[0].querySelector('.mod-row-1')?.textContent).toContain('Air Carrier IATA Prefix Index');
    expect(installedCards[0].querySelector('.mod-desc')?.textContent).toContain('48 global air cargo carriers');
    expect(installedCards[0].querySelector('.mod-row-3')?.textContent).toContain('48 cards');

    // Starter Catalog shows ONLY uninstalled starter packs with an [Install] button
    const starterSection = fixture.nativeElement.querySelector('section[aria-label="Starter Catalog"]') as HTMLElement;
    const starterCards = starterSection.querySelectorAll('.starter-card');
    expect(starterCards.length).toBe(1);
    expect(starterCards[0].textContent).toContain('ISO Container Sizes & Blueprints');

    const installBtn = starterCards[0].querySelector('button.primary-btn') as HTMLButtonElement;
    expect(installBtn.textContent?.trim()).toBe('Install');
    installBtn.click();
    await fixture.whenStable();
    expect(bridgeMock.installStarterModules).toHaveBeenCalledWith(['containers']);
  });

  it('focuses modal title (tabindex="-1") on open rather than the Close button', async () => {
    TestBed.configureTestingModule({
      imports: [ModulesDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(ModulesDialogComponent);
    const dialogEl = fixture.nativeElement.querySelector('dialog') as HTMLDialogElement;
    // Stub showModal for happy-dom compatibility
    dialogEl.showModal = vi.fn(() => {
      dialogEl.setAttribute('open', '');
    });

    const titleEl = fixture.nativeElement.querySelector('#modules-dialog-title') as HTMLElement;
    expect(titleEl.getAttribute('tabindex')).toBe('-1');
    const focusSpy = vi.spyOn(titleEl, 'focus');

    fixture.componentRef.setInput('open', true);
    await fixture.whenStable();

    expect(dialogEl.showModal).toHaveBeenCalled();
    expect(focusSpy).toHaveBeenCalledWith({ preventScroll: true });
  });

  it('displays live MCP Listening/Off status pill and copies valid MCP Client JSON', async () => {
    TestBed.configureTestingModule({
      imports: [ModulesDialogComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });
    const fixture = TestBed.createComponent(ModulesDialogComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    await fixture.componentInstance.refresh();
    await fixture.whenStable();

    const pill = fixture.nativeElement.querySelector('.mcp-status-pill') as HTMLElement;
    expect(pill.textContent?.trim()).toBe('● Listening');
    expect(pill.classList.contains('listening')).toBe(true);

    const copyClientBtn = fixture.nativeElement.querySelector('.mcp-header-row button.accent') as HTMLButtonElement;
    expect(copyClientBtn.textContent?.trim()).toBe('Copy Client JSON');
    copyClientBtn.click();
    expect(copySpy).toHaveBeenCalledWith(
      'MCP Client JSON',
      expect.stringContaining('"url": "http://127.0.0.1:17380/mcp"')
    );

    // When metricsAddr is absent -> ○ Off
    bridgeMock.getLogInfo.mockResolvedValueOnce({ filePath: '', level: 'info', sizeBytes: 0, metricsAddr: '' });
    await fixture.componentInstance.refresh();
    await fixture.whenStable();
    expect(fixture.nativeElement.querySelector('.mcp-status-pill')?.textContent?.trim()).toBe('○ Off');
  });
});
