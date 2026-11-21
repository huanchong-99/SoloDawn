// The Playwright project file of the end-to-end tests: one Vitest project per browser engine. The
// tests run in Node, start real processes through e2e/nodes.ts and drive the browser with
// Playwright. Run one engine with `pnpm exec vitest run --config e2e/config.ts --project chromium`.
import { execFile } from 'node:child_process';
import { mkdir, readdir, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { join, relative, resolve } from 'node:path';
import { promisify } from 'node:util';
import { chromium, firefox, webkit, type Browser } from 'playwright';
import { defineConfig } from 'vitest/config';

const web = resolve(import.meta.dirname, '..');
const repo = resolve(web, '..');
const webdist = join(repo, 'internal/node/webdist');

const browsers = ['chromium', 'firefox', 'webkit'] as const;
export type BrowserName = (typeof browsers)[number];

declare module 'vitest' {
  export interface ProvidedContext {
    /** The engine of the project that runs the test. */
    browser: BrowserName;
  }
}

/**
 * Launches the engine of a project as the browser-vectors workflow sets it up: CHROMIUM_PATH is
 * Chrome or Edge 137, FIREFOX136_PATH is Firefox 136, which speaks the protocol of the older
 * Playwright driver that firefox136-provider depends on. Without them, Playwright's own builds.
 */
export function launch(name: BrowserName): Promise<Browser> {
  const { CHROMIUM_PATH, FIREFOX136_PATH } = process.env;
  if (name === 'chromium') return chromium.launch({ executablePath: CHROMIUM_PATH });
  if (name === 'webkit') return webkit.launch();
  if (!FIREFOX136_PATH) return firefox.launch();
  const provider = createRequire(import.meta.url).resolve('firefox136-provider');
  const driver: typeof import('playwright') = createRequire(provider)('playwright');
  return driver.firefox.launch({ executablePath: FIREFOX136_PATH });
}

/** Builds the web client into solodawn, and the fake CLIs beside it, in root/bin; returns that. */
export async function build(root: string) {
  const client = join(root, 'web');
  const vite = await import('vite');
  await vite.build({ root: web, logLevel: 'warn', build: { outDir: client, emptyOutDir: true } });
  // The binary embeds internal/node/webdist. The overlay puts this build there for the compiler
  // alone, so a run writes nothing into the working tree.
  const replace: Record<string, string> = {};
  for (const file of await files(webdist)) replace[join(webdist, file)] = '';
  for (const file of await files(client)) replace[join(webdist, file)] = join(client, file);
  const overlay = join(root, 'overlay.json');
  await writeFile(overlay, JSON.stringify({ Replace: replace }));
  const bin = join(root, 'bin');
  await mkdir(bin);
  const fakes = ['./internal/host/fakecli/claude', './internal/host/fakecli/codex'];
  const args = ['build', '-overlay', overlay, '-o', bin + '/', './cmd/solodawn', ...fakes];
  await promisify(execFile)('go', args, { cwd: repo });
  return bin;
}

async function files(dir: string) {
  const entries = await readdir(dir, { recursive: true, withFileTypes: true });
  return entries.filter((e) => e.isFile()).map((e) => relative(dir, join(e.parentPath, e.name)));
}

export default defineConfig({
  test: {
    include: ['e2e/**/*.spec.ts', 'features/e2e.ts'],
    environment: 'node',
    // Builds once for every project of the run; see setup in nodes.ts.
    globalSetup: ['e2e/nodes.ts'],
    // One file runs at a time: on a fresh macOS Keychain, two Playwright WebKit UIProcesses calling
    // defaultWebCryptoMasterKey together race on SecItemAdd (SerializedCryptoKeyWrapCocoa.mm), and
    // errSecDuplicateItem returns std::nullopt, making boot's structuredClone(privateKey) fail.
    fileParallelism: false,
    testTimeout: 60_000,
    hookTimeout: 60_000,
    projects: browsers.map((browser) => ({ test: { name: browser, provide: { browser } } })),
  },
});
