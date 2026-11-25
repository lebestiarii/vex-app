---
name: vex-frontend-testing
description: Offline Angular 22 zoneless unit & component testing guide for Vex using Vitest 5, happy-dom, Angular Ivy Linker, and Deno 2.9.8 (plus Node.js 22+ ng test compatibility). Use whenever adding or updating frontend specs in frontend/src/**/*.spec.ts.
---

# Vex — Offline Frontend Testing Guide (Angular 22 + Vitest + Deno)

## 1. Test Commands (100% Offline, Sandbox-Safe)

- **macOS (Homebrew Deno)**:
  ```bash
  cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task test
  ```
- **Windows / Standard Deno**:
  ```powershell
  cd frontend; deno task test
  ```
- **Node.js `22.22+` / Angular CLI**:
  ```bash
  cd frontend && npm run ng:test
  ```

## 2. How the Offline Harness Works

1. `frontend/scripts/test-aot.mjs` compiles app + `*.spec.ts` files with Angular 22's `ngc` (`tsconfig.spec.json`) into `frontend/out-tsc/spec/` and cleans up `out-tsc/spec/` in a `finally` block.
2. `frontend/vitest.config.mts` runs **Vitest 5** (`environment: 'happy-dom'`, `watch: false`, `api: false` so zero loopback ports are opened) with an in-memory `@angular/compiler-cli/linker/babel` plugin transforming partial `ɵɵngDeclare` definitions inside `node_modules/@angular/*.mjs`.
3. `frontend/src/test-setup.ts` initializes `BrowserTestingModule` + `platformBrowserTesting()` once and calls `getTestBed().resetTestingModule()` in a global `afterEach`.

## 3. Zoneless Angular 22 Testing Guardrails

- **Never use `vi.useFakeTimers()` in component specs that await `fixture.whenStable()`**: Angular 22's zoneless change-detection scheduler relies on real timer/microtask ticks; freezing timers causes `fixture.whenStable()` to hang until Vitest's 5s timeout.
  - Keep `vi.useFakeTimers()` strictly in pure service specs (e.g., `RolodexStateService` toast/shake/clock timer tests) where `fixture.whenStable()` is not used.
  - When a component's `async ngOnInit()` chains multiple `await` hops, explicitly await its public refresh method (e.g. `await fixture.componentInstance.refreshResults(); await fixture.whenStable();`) before asserting async DOM state.
- **Clean Up Service Timers with `DestroyRef`**: Services that register `setInterval`, `setTimeout`, or `matchMedia` listeners (such as `RolodexStateService`) must register cleanup via `inject(DestroyRef).onDestroy(...)` so `TestBed` resets never leak open handles.
- **Mock Wails RPC (`VexBridgeService`)**: Provide a `vi.fn()` mock for `VexBridgeService` in dialog/root component specs — never call `/wails/runtime.js` or bind network sockets.
- **Zero Hardcoded Demo Data in Production**: Test fixtures belong strictly inside `*.spec.ts` files, never in `frontend/src/app/` runtime code.
