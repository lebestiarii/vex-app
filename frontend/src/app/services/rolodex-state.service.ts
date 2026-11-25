import { DestroyRef, inject, Injectable, signal } from '@angular/core';
import { CardNode } from '../models/card-node.model';

export interface ToastMessage {
  id: number;
  text: string;
  tone: 'info' | 'warn';
}

export type ThemeMode = 'light' | 'dark';

const PIN_STORAGE_KEY = 'vex.rolodex.pinnedIds.v1';
const PIN_NODES_STORAGE_KEY = 'vex.rolodex.pinnedNodes.v1';
const THEME_STORAGE_KEY = 'vex.rolodex.theme.v1';
export const MAX_PINS = 3;

@Injectable({ providedIn: 'root' })
export class RolodexStateService {
  readonly pinnedIds = signal<string[]>(this.loadInitialPins());
  readonly pinnedCardsMap = signal<Record<string, CardNode>>(this.loadInitialPinnedNodes());
  readonly shakingPinId = signal<string | null>(null);
  readonly toasts = signal<ToastMessage[]>([]);
  readonly liveAnnouncement = signal<string>('');
  readonly clockNow = signal<Date>(new Date());
  readonly theme = signal<ThemeMode>(this.loadInitialTheme());

  private readonly destroyRef = inject(DestroyRef);
  private nextToastId = 1;
  private shakeTimer: ReturnType<typeof setTimeout> | null = null;
  private hasManualTheme = false;

  constructor() {
    this.applyThemeAttr(this.theme());

    if (typeof window !== 'undefined') {
      const clockTimer = setInterval(() => {
        this.clockNow.set(new Date());
      }, 30_000);

      // Follow OS color scheme changes dynamically when user hasn't manually overridden
      const mq = window.matchMedia?.('(prefers-color-scheme: dark)');
      const onThemeMediaChange = (ev: MediaQueryListEvent) => {
        if (!this.hasManualTheme) {
          const next: ThemeMode = ev.matches ? 'dark' : 'light';
          this.theme.set(next);
          this.applyThemeAttr(next);
        }
      };
      mq?.addEventListener?.('change', onThemeMediaChange);

      this.destroyRef.onDestroy(() => {
        clearInterval(clockTimer);
        if (this.shakeTimer) {
          clearTimeout(this.shakeTimer);
        }
        mq?.removeEventListener?.('change', onThemeMediaChange);
      });
    }
  }

  toggleTheme(): void {
    const next: ThemeMode = this.theme() === 'light' ? 'dark' : 'light';
    this.hasManualTheme = true;
    this.theme.set(next);
    this.applyThemeAttr(next);
    try {
      localStorage.setItem(THEME_STORAGE_KEY, next);
    } catch {
      // ignore storage errors
    }
  }

  isPinned(id: string): boolean {
    return this.pinnedIds().includes(id);
  }

  togglePin(target: string | CardNode): boolean {
    const id = typeof target === 'string' ? target : target.id;
    const node = typeof target === 'string' ? null : target;
    const current = this.pinnedIds();

    if (current.includes(id)) {
      const next = current.filter((item) => item !== id);
      this.pinnedIds.set(next);
      this.savePins(next);
      this.pinnedCardsMap.update((prev) => {
        if (!(id in prev)) return prev;
        const copy = { ...prev };
        delete copy[id];
        this.savePinnedNodes(copy);
        return copy;
      });
      this.showToast('Card unpinned', 'info');
      return false;
    }

    if (current.length >= MAX_PINS) {
      this.triggerPinShake(id);
      this.showToast(`Pin limit reached (max ${MAX_PINS} cards). Unpin one first.`, 'warn');
      return false;
    }

    const next = [...current, id];
    this.pinnedIds.set(next);
    this.savePins(next);
    if (node) {
      this.pinnedCardsMap.update((prev) => {
        const updated = { ...prev, [id]: node };
        this.savePinnedNodes(updated);
        return updated;
      });
    }
    this.showToast(`Pinned to top (${next.length}/${MAX_PINS})`, 'info');
    return true;
  }

  syncPinnedCards(nodes: CardNode[]): void {
    const pinnedSet = new Set(this.pinnedIds());
    if (pinnedSet.size === 0 || nodes.length === 0) return;

    let changed = false;
    const next = { ...this.pinnedCardsMap() };
    for (const n of nodes) {
      if (pinnedSet.has(n.id)) {
        next[n.id] = n;
        changed = true;
      }
    }
    if (changed) {
      this.pinnedCardsMap.set(next);
      this.savePinnedNodes(next);
    }
  }

  async copyText(label: string, value: string): Promise<void> {
    const clean = (value ?? '').trim();
    if (!clean) return;
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(clean);
      } else {
        const ta = document.createElement('textarea');
        ta.value = clean;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
      }
      const preview = clean.length > 64 ? `${clean.slice(0, 61)}…` : clean;
      this.showToast(`Copied ${label}: ${preview}`, 'info');
    } catch {
      this.showToast(`Ready to copy ${label}`, 'warn');
    }
  }

  openExternalUrl(url: string): void {
    const clean = (url ?? '').trim();
    if (!clean) return;
    if (typeof window !== 'undefined') {
      window.open(clean, '_blank', 'noopener,noreferrer');
    }
  }

  showToast(text: string, tone: 'info' | 'warn' = 'info'): void {
    const id = this.nextToastId++;
    this.toasts.update((list) => [...list.slice(-2), { id, text, tone }]);
    this.announce(text);

    setTimeout(() => {
      this.dismissToast(id);
    }, 3400);
  }

  dismissToast(id: number): void {
    this.toasts.update((list) => list.filter((t) => t.id !== id));
  }

  isClockChip(label: string): boolean {
    if (!label || !label.includes('/') || label.includes(' ')) {
      return false;
    }
    try {
      new Intl.DateTimeFormat('en-GB', { timeZone: label });
      return true;
    } catch {
      return false;
    }
  }

  formatClockChip(label: string): string {
    if (!label || !label.includes('/') || label.includes(' ')) {
      return label;
    }
    try {
      return new Intl.DateTimeFormat('en-GB', {
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
        timeZone: label,
      }).format(this.clockNow());
    } catch {
      return label;
    }
  }

  private applyThemeAttr(mode: ThemeMode): void {
    if (typeof document !== 'undefined') {
      document.documentElement.setAttribute('data-theme', mode);
    }
  }

  private loadInitialTheme(): ThemeMode {
    if (typeof localStorage !== 'undefined') {
      try {
        const saved = localStorage.getItem(THEME_STORAGE_KEY);
        if (saved === 'light' || saved === 'dark') {
          this.hasManualTheme = true;
          return saved;
        }
      } catch {
        // ignore
      }
    }
    if (typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches) {
      return 'dark';
    }
    return 'light';
  }

  private triggerPinShake(id: string): void {
    if (this.shakeTimer) {
      clearTimeout(this.shakeTimer);
    }
    this.shakingPinId.set(id);
    this.shakeTimer = setTimeout(() => {
      this.shakingPinId.set(null);
    }, 420);
  }

  private announce(message: string): void {
    const doc =
      typeof document !== 'undefined'
        ? (document as Document & { ariaNotify?: (msg: string) => void })
        : null;
    if (doc && typeof doc.ariaNotify === 'function') {
      try {
        doc.ariaNotify(message);
        return;
      } catch {
        // fallback below
      }
    }
    this.liveAnnouncement.set('');
    setTimeout(() => this.liveAnnouncement.set(message), 16);
  }

  private loadInitialPins(): string[] {
    if (typeof localStorage === 'undefined') {
      return [];
    }
    try {
      const raw = localStorage.getItem(PIN_STORAGE_KEY);
      if (!raw) return [];
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return parsed.filter((v): v is string => typeof v === 'string').slice(0, MAX_PINS);
      }
    } catch {
      // ignore
    }
    return [];
  }

  private savePins(ids: string[]): void {
    try {
      localStorage.setItem(PIN_STORAGE_KEY, JSON.stringify(ids));
    } catch {
      // ignore
    }
  }

  private loadInitialPinnedNodes(): Record<string, CardNode> {
    if (typeof localStorage === 'undefined') {
      return {};
    }
    try {
      const raw = localStorage.getItem(PIN_NODES_STORAGE_KEY);
      if (!raw) return {};
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        const clean: Record<string, CardNode> = {};
        for (const [k, v] of Object.entries(parsed)) {
          if (v && typeof v === 'object' && 'id' in v && 'layout' in v) {
            clean[k] = v as CardNode;
          }
        }
        return clean;
      }
    } catch {
      // ignore
    }
    return {};
  }

  private savePinnedNodes(map: Record<string, CardNode>): void {
    try {
      localStorage.setItem(PIN_NODES_STORAGE_KEY, JSON.stringify(map));
    } catch {
      // ignore
    }
  }
}
