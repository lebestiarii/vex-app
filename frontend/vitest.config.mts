import fs from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { ConsoleLogger, LogLevel, NodeJSFileSystem } from '@angular/compiler-cli';
import { createEs2015LinkerPlugin } from '@angular/compiler-cli/linker/babel';
import { defineConfig } from 'vitest/config';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const compilerCliPkg = fs.realpathSync(
  path.join(__dirname, 'node_modules/@angular/compiler-cli/package.json')
);
const req = createRequire(compilerCliPkg);
const babel = req('@babel/core');

const linkerPlugin = createEs2015LinkerPlugin({
  fileSystem: new NodeJSFileSystem(),
  logger: new ConsoleLogger(LogLevel.warn),
  unknownDeclarationVersionHandling: 'error',
  sourceMapping: false,
});

function angularIvyLinkerPlugin() {
  return {
    name: 'vex-angular-ivy-linker',
    enforce: 'pre' as const,
    async transform(code: string, id: string) {
      const cleanId = id.split('?')[0];
      if (!cleanId.includes('node_modules') || !cleanId.endsWith('.mjs')) {
        return null;
      }
      if (!code.includes('ɵɵngDeclare')) {
        return null;
      }
      const result = await babel.transformAsync(code, {
        filename: cleanId,
        plugins: [linkerPlugin],
        compact: false,
        sourceMaps: false,
      });
      if (!result || typeof result.code !== 'string') {
        throw new Error(`Angular Ivy linker failed for ${cleanId}`);
      }
      return { code: result.code, map: null };
    },
  };
}

export default defineConfig({
  plugins: [angularIvyLinkerPlugin()],
  test: {
    environment: 'happy-dom',
    include: ['out-tsc/spec/**/*.spec.js'],
    setupFiles: ['out-tsc/spec/test-setup.js'],
    watch: false,
    api: false,
    server: {
      deps: {
        inline: [/@angular\//],
      },
    },
  },
});
