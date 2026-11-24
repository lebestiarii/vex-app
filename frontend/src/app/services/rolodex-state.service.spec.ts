import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MAX_PINS, RolodexStateService } from './rolodex-state.service';

describe('RolodexStateService', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    localStorage.clear();
    document.documentElement.removeAttribute('data-theme');
  });

  afterEach(() => {
    vi.useRealTimers();
    localStorage.clear();
  });

  function createService(): RolodexStateService {
    TestBed.configureTestingModule({});
    return TestBed.inject(RolodexStateService);
  }

  it('enforces MAX_PINS (3), shakes on 4th pin, and supports unpinning', () => {
    localStorage.setItem('vex.rolodex.pinnedIds.v1', JSON.stringify([]));
    const svc = createService();

    expect(svc.togglePin('card-1')).toBe(true);
    expect(svc.togglePin('card-2')).toBe(true);
    expect(svc.togglePin('card-3')).toBe(true);
    expect(svc.pinnedIds()).toEqual(['card-1', 'card-2', 'card-3']);
    expect(svc.pinnedIds().length).toBe(MAX_PINS);

    // Attempting a 4th pin must fail, trigger shake, and emit a warn toast
    expect(svc.togglePin('card-4')).toBe(false);
    expect(svc.pinnedIds()).toEqual(['card-1', 'card-2', 'card-3']);
    expect(svc.shakingPinId()).toBe('card-4');
    expect(svc.toasts().some((t) => t.tone === 'warn')).toBe(true);

    // Shake clears after 420ms
    vi.advanceTimersByTime(450);
    expect(svc.shakingPinId()).toBeNull();

    // Unpinning removes the ID and persists to localStorage
    expect(svc.togglePin('card-2')).toBe(false);
    expect(svc.isPinned('card-2')).toBe(false);
    expect(svc.pinnedIds()).toEqual(['card-1', 'card-3']);
    expect(JSON.parse(localStorage.getItem('vex.rolodex.pinnedIds.v1') ?? '[]')).toEqual([
      'card-1',
      'card-3',
    ]);
  });

  it('toggles Light/Dark theme, updates document data-theme, and persists choice', () => {
    const svc = createService();
    const initial = svc.theme();
    expect(document.documentElement.getAttribute('data-theme')).toBe(initial);

    svc.toggleTheme();
    const toggled = initial === 'light' ? 'dark' : 'light';
    expect(svc.theme()).toBe(toggled);
    expect(document.documentElement.getAttribute('data-theme')).toBe(toggled);
    expect(localStorage.getItem('vex.rolodex.theme.v1')).toBe(toggled);
  });

  it('formats valid IANA timezone labels as HH:MM and preserves non-timezone chips', () => {
    const svc = createService();
    svc.clockNow.set(new Date('2026-11-24T14:05:00.000Z'));

    expect(svc.formatClockChip('Etc/UTC')).toBe('14:05');
    expect(svc.formatClockChip('NLAMS · AMS')).toBe('NLAMS · AMS');
    expect(svc.formatClockChip('14°C')).toBe('14°C');
    expect(svc.formatClockChip('Not/A_Real_Zone')).toBe('Not/A_Real_Zone');
  });

  it('caps visible toast stack to 3 items and auto-dismisses after 3400ms', () => {
    const svc = createService();
    svc.showToast('One', 'info');
    svc.showToast('Two', 'info');
    svc.showToast('Three', 'info');
    svc.showToast('Four', 'warn');

    expect(svc.toasts().map((t) => t.text)).toEqual(['Two', 'Three', 'Four']);

    vi.advanceTimersByTime(3500);
    expect(svc.toasts()).toEqual([]);
  });
});
