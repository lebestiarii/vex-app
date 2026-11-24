import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, '..');
const outSpecDir = path.join(rootDir, 'out-tsc', 'spec');

fs.rmSync(outSpecDir, { recursive: true, force: true });

try {
  // 1. AOT-compile app + spec files under Angular 22 ngc & TypeScript 6
  execFileSync(
    process.execPath,
    [
      'run',
      '-A',
      path.join(rootDir, 'node_modules/@angular/compiler-cli/bundles/src/bin/ngc.js'),
      '-p',
      path.join(rootDir, 'tsconfig.spec.json'),
    ],
    { cwd: rootDir, stdio: 'inherit' }
  );

  // 2. Run Vitest 5 offline against compiled specs with happy-dom + Angular Ivy Linker
  const extraArgs = process.argv.slice(2);
  execFileSync(
    process.execPath,
    [
      'run',
      '-A',
      path.join(rootDir, 'node_modules/vitest/vitest.mjs'),
      'run',
      '--config',
      path.join(rootDir, 'vitest.config.mts'),
      ...extraArgs,
    ],
    { cwd: rootDir, stdio: 'inherit' }
  );
} finally {
  fs.rmSync(outSpecDir, { recursive: true, force: true });
}
