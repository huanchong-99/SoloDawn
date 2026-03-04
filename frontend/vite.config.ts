// vite.config.ts
import { sentryVitePlugin } from "@sentry/vite-plugin";
import { defineConfig, Plugin } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import fs from "node:fs";

function executorSchemasPlugin(): Plugin {
  const VIRTUAL_ID = "virtual:executor-schemas";
  const RESOLVED_VIRTUAL_ID = "\0" + VIRTUAL_ID;

  return {
    name: "executor-schemas-plugin",
    resolveId(id) {
      if (id === VIRTUAL_ID) return RESOLVED_VIRTUAL_ID; // keep it virtual
      return null;
    },
    load(id) {
      if (id !== RESOLVED_VIRTUAL_ID) return null;

      const schemasDir = path.resolve(__dirname, "../shared/schemas");
      const files = fs.existsSync(schemasDir)
        ? fs.readdirSync(schemasDir).filter((f) => f.endsWith(".json"))
        : [];

      const imports: string[] = [];
      const entries: string[] = [];

      files.forEach((file, i) => {
        const varName = `__schema_${i}`;
        const importPath = `shared/schemas/${file}`; // uses your alias
        const key = file.replace(/\.json$/, "").toUpperCase(); // claude_code -> CLAUDE_CODE
        imports.push(`import ${varName} from "${importPath}";`);
        entries.push(`  "${key}": ${varName}`);
      });

      // IMPORTANT: pure JS (no TS types), and quote keys.
      const code = `
${imports.join("\n")}

export const schemas = {
${entries.join(",\n")}
};

export default schemas;
`;
      return code;
    },
  };
}

function isBenignWsProxyError(error: unknown): boolean {
  const code = (error as NodeJS.ErrnoException | undefined)?.code;
  return code === "ECONNRESET" || code === "ECONNABORTED" || code === "EPIPE";
}

const sentryAuthToken = process.env.SENTRY_AUTH_TOKEN?.trim();
const sentryUploadEnabled = Boolean(sentryAuthToken);

export default defineConfig({
  plugins: [
    react({
      babel: {
        plugins: [
          [
            "babel-plugin-react-compiler",
            {
              target: "18",
              sources: [path.resolve(__dirname, "src")],
              environment: {
                enableResetCacheOnSourceFileChanges: true,
              },
            },
          ],
        ],
      },
    }),
    ...(sentryUploadEnabled
      ? [
          sentryVitePlugin({
            org: "bloop-ai",
            project: "gitcortex",
            authToken: sentryAuthToken,
            telemetry: false,
          }),
        ]
      : []),
    executorSchemasPlugin(),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
      shared: path.resolve(__dirname, "../shared"),
    },
  },
  server: {
    port: Number.parseInt(process.env.FRONTEND_PORT || "23457", 10),
    strictPort: true, // Don't try other ports if 23457 is in use
    proxy: {
      "/api": {
        target: `http://localhost:${process.env.BACKEND_PORT || "23456"}`,
        changeOrigin: true,
        ws: true,
        configure: (proxy) => {
          const existingErrorListeners = proxy.listeners("error");
          proxy.removeAllListeners("error");
          proxy.on("error", (error, req, res, target) => {
            const isWsRequest =
              req?.headers?.upgrade?.toLowerCase?.() === "websocket";

            if (isWsRequest && isBenignWsProxyError(error)) {
              // Ignore noisy WS close races (client-side reconnect/close)
              // to keep dev logs focused on actionable failures.
              return;
            }

            for (const listener of existingErrorListeners) {
              listener.call(proxy, error, req, res, target);
            }
          });
        },
      }
    },
    fs: {
      allow: [path.resolve(__dirname, "."), path.resolve(__dirname, "..")],
    },
    open: process.env.VITE_OPEN === "true",
    allowedHosts: [
      ".trycloudflare.com", // allow all cloudflared tunnels
    ],
  },
  optimizeDeps: {
    exclude: ["wa-sqlite"],
  },
  build: {
    sourcemap: true,
    chunkSizeWarningLimit: 7000,
  },
});
