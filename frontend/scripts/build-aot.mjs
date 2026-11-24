import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { ConsoleLogger, LogLevel, NodeJSFileSystem } from '@angular/compiler-cli';
import { createEs2015LinkerPlugin } from '@angular/compiler-cli/linker/babel';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, '..');
const outTscDir = path.join(rootDir, 'out-tsc', 'app');
const distDir = path.join(rootDir, 'dist');

// Resolve @babel/core and esbuild reliably under both Deno (.deno store) and Node.js
const compilerCliPkg = fs.realpathSync(
  path.join(rootDir, 'node_modules/@angular/compiler-cli/package.json')
);
const req = createRequire(compilerCliPkg);
const babel = req('@babel/core');

function loadEsbuild() {
  const direct = path.join(rootDir, 'node_modules/esbuild');
  if (fs.existsSync(direct)) {
    return createRequire(import.meta.url)(direct);
  }
  const denoStoreDir = path.join(rootDir, 'node_modules/.deno');
  if (fs.existsSync(denoStoreDir)) {
    const matches = fs
      .readdirSync(denoStoreDir)
      .filter((entry) => entry.startsWith('esbuild@'))
      .sort()
      .reverse();
    for (const entry of matches) {
      const cand = path.join(denoStoreDir, entry, 'node_modules/esbuild');
      if (fs.existsSync(cand)) {
        return createRequire(import.meta.url)(cand);
      }
    }
  }
  return createRequire(import.meta.url)('esbuild');
}

const esbuild = loadEsbuild();

fs.rmSync(outTscDir, { recursive: true, force: true });
fs.rmSync(distDir, { recursive: true, force: true });
fs.mkdirSync(distDir, { recursive: true });

// 1. Run official Angular AOT compiler (ngc) -> emits compiled JS with ɵcmp / ɵfac / ɵprov
execFileSync(
  process.execPath,
  [
    'run',
    '-A',
    path.join(rootDir, 'node_modules/@angular/compiler-cli/bundles/src/bin/ngc.js'),
    '-p',
    path.join(rootDir, 'tsconfig.app.json'),
  ],
  { cwd: rootDir, stdio: 'inherit' }
);

// 2. Angular Ivy Linker esbuild plugin: transforms partial declarations (ɵɵngDeclare*) in
//    node_modules/@angular/* (fesm2022) into full AOT Ivy definitions so zero JIT compiler is needed.
const linkerPlugin = createEs2015LinkerPlugin({
  fileSystem: new NodeJSFileSystem(),
  logger: new ConsoleLogger(LogLevel.warn),
  unknownDeclarationVersionHandling: 'error',
  sourceMapping: false,
});

const angularLinkerEsbuildPlugin = {
  name: 'angular-ivy-linker',
  setup(build) {
    build.onLoad({ filter: /\.m?js$/ }, async (args) => {
      // Only run Babel Ivy linker on npm packages that ship partial Ivy declarations
      if (!args.path.includes('node_modules')) {
        return null;
      }
      const source = await fs.promises.readFile(args.path, 'utf8');
      if (!source.includes('ɵɵngDeclare')) {
        return null;
      }
      const result = await babel.transformAsync(source, {
        filename: args.path,
        plugins: [linkerPlugin],
        compact: false,
        sourceMaps: false,
      });
      if (!result || typeof result.code !== 'string') {
        throw new Error(`Angular Ivy linker failed for ${args.path}`);
      }
      return { contents: result.code, loader: 'js' };
    });
  },
};

// 3. Bundle AOT-compiled Angular app + Ivy-linked Angular runtime libraries (zoneless)
await esbuild.build({
  entryPoints: [path.join(outTscDir, 'main.js')],
  bundle: true,
  format: 'esm',
  target: ['es2022', 'safari16'],
  minify: true,
  define: {
    ngDevMode: 'false',
    ngI18nClosureMode: 'false',
  },
  external: ['/wails/runtime.js'],
  plugins: [angularLinkerEsbuildPlugin],
  outfile: path.join(distDir, 'main.js'),
  logLevel: 'info',
});

// 4. Bundle global styles.css
await esbuild.build({
  entryPoints: [path.join(rootDir, 'src/styles.css')],
  bundle: true,
  minify: true,
  outfile: path.join(distDir, 'styles.css'),
  logLevel: 'info',
});

// 5. Emit dist/index.html linking styles.css and main.js
const rawHtml = fs.readFileSync(path.join(rootDir, 'src/index.html'), 'utf8');
const builtHtml = rawHtml
  .replace('</head>', '  <link rel="stylesheet" href="./styles.css" />\n  </head>')
  .replace('</body>', '  <script type="module" src="./main.js"></script>\n  </body>');
fs.writeFileSync(path.join(distDir, 'index.html'), builtHtml, 'utf8');

// Clean intermediate out-tsc directory
fs.rmSync(path.join(rootDir, 'out-tsc'), { recursive: true, force: true });
console.log('Angular AOT + Ivy Linker build complete -> frontend/dist/');
