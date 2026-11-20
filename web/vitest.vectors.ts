import { playwright } from '@vitest/browser-playwright';
import { playwright as firefox136 } from 'firefox136-provider';
import { defineConfig, mergeConfig } from 'vitest/config';
import vite from './vite.config.ts';

export default defineConfig((env) =>
  mergeConfig(vite(env), {
    test: {
      include: ['src/lib/vectors.test.ts', 'features/browser-crypto.ts'],
      browser: {
        enabled: true,
        headless: true,
        provider: playwright(),
        instances: [
          {
            browser: 'chromium',
            provider: playwright({ launchOptions: { executablePath: process.env.CHROMIUM_PATH } }),
          },
          {
            browser: 'firefox',
            provider: process.env.FIREFOX136_PATH
              ? firefox136({ launchOptions: { executablePath: process.env.FIREFOX136_PATH } })
              : playwright(),
          },
          // GTK/WPE use a different Ed25519 implementation from Apple's port.
          ...(process.platform === 'darwin' ? [{ browser: 'webkit' }] : []),
        ],
      },
    },
  }),
);
