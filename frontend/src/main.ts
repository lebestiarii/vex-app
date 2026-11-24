import { bootstrapApplication } from '@angular/platform-browser';
import { AppComponent } from './app/app.component';

bootstrapApplication(AppComponent).catch((err) => {
  console.error('Vex bootstrap error:', err);
  const root = document.querySelector('app-root');
  if (root) {
    const pre = document.createElement('pre');
    pre.style.cssText =
      'margin:16px;padding:12px;border-radius:10px;background:#fff1f2;color:#be123c;border:1px solid #fecdd3;font:12px/1.45 ui-monospace,monospace;white-space:pre-wrap;word-break:break-word;';
    pre.textContent = `Vex failed to start:\n${err instanceof Error ? err.stack || err.message : String(err)}`;
    root.replaceChildren(pre);
  }
});
