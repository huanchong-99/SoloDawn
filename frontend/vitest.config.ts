import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'node:path';
import fs from 'node:fs';
import os from 'node:os';
import { Plugin } from 'vite';

process.env.BROWSERSLIST_IGNORE_OLD_DATA = '1';

function executorSchemasPlugin(): Plugin {
  const VIRTUAL_ID = 'virtual:executor-schemas';
  const RESOLVED_VIRTUAL_ID = '\0' + VIRTUAL_ID;

  return {
    name: 'executor-schemas-plugin',
    resolveId(id) {
      if (id === VIRTUAL_ID) return RESOLVED_VIRTUAL_ID;
      return null;
    },
    load(id) {
      if (id !== RESOLVED_VIRTUAL_ID) return null;

      const schemasDir = path.resolve(__dirname, '../shared/schemas');
      const files = fs.existsSync(schemasDir)
        ? fs.readdirSync(schemasDir).filter((f) => f.endsWith('.json'))
        : [];

      const imports: string[] = [];
      const entries: string[] = [];

      files.forEach((file, i) => {
        const varName = `__schema_${i}`;
        const importPath = `shared/schemas/${file}`;
        const key = file.replace(/\.json$/, '').toUpperCase();
        imports.push(`import ${varName} from '${importPath}';`);
        entries.push(`  '${key}': ${varName}`);
      });

      const code = `
${imports.join('\n')}

export const schemas = {
${entries.join(',\n')}
};

export default schemas;
`;
      return code;
    },
  };
}

export default defineConfig({
  plugins: [react(), executorSchemasPlugin()],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.{test,spec}.{js,ts,jsx,tsx}'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
    },
    // Performance: use the 'threads' pool (worker_threads) rather than 'forks'
    // (child processes). With jsdom + React Testing Library the suite is bound
    // by per-file fixed startup cost across many small files, not by CPU
    // contention; worker threads have a lower per-file startup tax than forked
    // processes, so threads is measurably faster here (~24.6s -> ~22.6s local).
    // `singleThread: false` means each worker still runs multiple files
    // sequentially within itself.
    //
    // Quality guarantee: the exact same test files run, with the same
    // environment and setup. Only the *scheduling* changes.
    pool: 'threads',
    poolOptions: {
      threads: {
        // Default is min(available CPUs, 16). Keep it explicit so CI runners
        // (2-core GitHub Actions) and local dev both get sensible parallelism.
        // `availableParallelism()` may not exist on older Node; fall back to 4.
        maxThreads: Math.max(2, (os.availableParallelism?.() ?? 4) - 1),
        singleThread: false,
      },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      shared: path.resolve(__dirname, '../shared'),
    },
  },
});
