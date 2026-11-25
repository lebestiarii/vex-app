import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AppComponent } from './app.component';
import { CardNode } from './models/card-node.model';
import { RolodexStateService } from './services/rolodex-state.service';
import { VexBridgeService } from './services/vex-bridge.service';

describe('AppComponent', () => {
  const makeCard = (id: string, title: string): CardNode => ({
    id,
    template: 'knowledge',
    icon: '📄',
    title,
    chips: [],
    data: {},
  });

  let bridgeMock: {
    isDesktop: ReturnType<typeof signal<boolean>>;
    getUserPrefs: ReturnType<typeof vi.fn>;
    searchRolodex: ReturnType<typeof vi.fn>;
    listModules: ReturnType<typeof vi.fn>;
    listStarterModules: ReturnType<typeof vi.fn>;
    listPatternRules: ReturnType<typeof vi.fn>;
    hideWindow: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    localStorage.clear();
    bridgeMock = {
      isDesktop: signal<boolean>(false),
      getUserPrefs: vi.fn().mockResolvedValue({ onboardingCompleted: true }),
      searchRolodex: vi.fn().mockResolvedValue([makeCard('c1', 'First Card')]),
      listModules: vi.fn().mockResolvedValue([]),
      listStarterModules: vi.fn().mockResolvedValue([]),
      listPatternRules: vi.fn().mockResolvedValue([]),
      hideWindow: vi.fn().mockResolvedValue(undefined),
    };
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('requests limit 24 for empty browse query and limit 10 for active search queries', async () => {
    TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });

    const fixture = TestBed.createComponent(AppComponent);
    await fixture.whenStable();

    expect(bridgeMock.searchRolodex).toHaveBeenCalledWith('', 24);

    fixture.componentInstance.query.set('AMS');
    await fixture.componentInstance.refreshResults();

    expect(bridgeMock.searchRolodex).toHaveBeenLastCalledWith('AMS', 10);
  });

  it('orders pinned cards ahead of unpinned results while preserving relevance order', async () => {
    bridgeMock.searchRolodex.mockResolvedValue([
      makeCard('exact-ams', 'Amsterdam Gateway (AMS)'),
      makeCard('pinned-card', 'Pinned Runbook'),
      makeCard('ctx-edi', 'AMS EDI Servers'),
    ]);

    TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [{ provide: VexBridgeService, useValue: bridgeMock }],
    });

    const state = TestBed.inject(RolodexStateService);
    state.togglePin('pinned-card');

    const fixture = TestBed.createComponent(AppComponent);
    await fixture.whenStable();

    const orderedIds = fixture.componentInstance.orderedCards().map((c) => c.id);
    expect(orderedIds).toEqual(['pinned-card', 'exact-ams', 'ctx-edi']);
  });
});
