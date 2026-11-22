import { resolve } from 'node:path';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { build as viteBuild } from 'vite';
import { configDefaults, defineConfig } from 'vitest/config';

// The node embeds the built client from internal/node/webdist (architecture 8.1). The build
// empties that directory, so the .gitignore that keeps it out of Git is written back each time.
const webdist = '../internal/node/webdist';

// The wire examples of internal/proto, which the golden-file test and the development mock import
// as `@testdata`. Stryker runs the tests in a copy of web/ at web/.stryker-tmp/sandbox-*/, and the
// examples are not in the copy.
const here = import.meta.dirname;
const repo = here.includes('.stryker-tmp') ? '../../..' : '..';
const testdata = resolve(here, repo, 'internal/proto/testdata');
const targets = ['chrome137', 'edge137', 'firefox136', 'safari17.5', 'ios17.5'];

async function buildServiceWorker(): Promise<string> {
  const output = await viteBuild({
    configFile: false,
    logLevel: 'silent',
    resolve: { alias: { '@testdata': testdata } },
    build: {
      write: false,
      target: targets,
      lib: { entry: resolve(here, 'src/sw.ts'), formats: ['iife'], name: 'sw' },
    },
  });
  const chunk = Array.isArray(output) ? output[0]?.output[0] : undefined;
  if (!chunk || chunk.type !== 'chunk') throw new Error('sw.ts produced no bundle');
  return chunk.code;
}

export default defineConfig(({ mode }) => ({
  base: './',
  plugins: [
    svelte(),
    {
      name: 'webdist-gitignore',
      generateBundle() {
        this.emitFile({ type: 'asset', fileName: '.gitignore', source: '*\n!.gitignore\n' });
      },
    },
    {
      name: 'service-worker',
      configureServer(server) {
        server.middlewares.use((req, res, next) => {
          if (req.url?.split('?')[0] !== '/sw.js') return next();
          buildServiceWorker().then((code) => {
            res.setHeader('Content-Type', 'application/javascript; charset=utf-8');
            res.end(code);
          }, next);
        });
      },
      async generateBundle() {
        this.emitFile({ type: 'asset', fileName: 'sw.js', source: await buildServiceWorker() });
      },
    },
  ],
  // The supported browsers (docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md).
  // CSS is minified for the same targets, so light-dark() and nesting ship as written.
  build: {
    outDir: webdist,
    emptyOutDir: true,
    target: targets,
  },
  server: { fs: { allow: ['.', testdata] } },
  resolve: {
    alias: { '@testdata': testdata },
    // Tests mount components, so Svelte must resolve to its browser build.
    ...(mode === 'test' && { conditions: ['browser'] }),
  },
  // The end-to-end tests need Go, a build and browsers; only e2e/config.ts runs them.
  test: {
    environment: 'happy-dom',
    setupFiles: ['./features/setup.ts'],
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
}));
