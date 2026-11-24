import { DestroyRef, inject, Injectable, signal } from '@angular/core';

export interface ToastMessage {
  id: number;
  text: string;
  tone: 'info' | 'warn';
}

export type ThemeMode = 'light' | 'dark';

const PIN_STORAGE_KEY = 'vex.rolodex.pinnedIds.v1';
const THEME_STORAGE_KEY = 'vex.rolodex.theme.v1';
export const MAX_PINS = 3;

@Injectable({ providedIn: 'root' })
export class RolodexStateService {
  readonly pinnedIds = signal<string[]>(this.loadInitialPins());
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

  togglePin(id: string): boolean {
    const current = this.pinnedIds();
    if (current.includes(id)) {
      const next = current.filter((item) => item !== id);
      this.pinnedIds.set(next);
      this.savePins(next);
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
    this.showToast(`Pinned to top (${next.length}/${MAX_PINS})`, 'info');
    return true;
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
      this.showToast(`Copied ${label}: ${clean}`, 'info');
    } catch {
      this.showToast(`Ready to copy ${label}: ${clean}`, 'warn');
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
      return ['gateway-ams'];
    }
    try {
      const raw = localStorage.getItem(PIN_STORAGE_KEY);
      if (!raw) return ['gateway-ams'];
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return parsed.filter((v): v is string => typeof v === 'string').slice(0, MAX_PINS);
      }
    } catch {
      // ignore
    }
    return ['gateway-ams'];
  }

  private savePins(ids: string[]): void {
    try {
      localStorage.setItem(PIN_STORAGE_KEY, JSON.stringify(ids));
    } catch {
      // ignore
    }
  }
}
