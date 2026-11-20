import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

// The node embeds the built client from internal/node/webdist (architecture 8.1). The build
// empties that directory, so the .gitignore that keeps it out of Git is written back each time.
const webdist = '../internal/node/webdist';

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
  ],
  build: { outDir: webdist, emptyOutDir: true },
  // Tests mount components, so Svelte must resolve to its browser build.
  resolve: mode === 'test' ? { conditions: ['browser'] } : {},
  test: { environment: 'happy-dom' },
}));
