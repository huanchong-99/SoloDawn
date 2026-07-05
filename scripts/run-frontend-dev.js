#!/usr/bin/env node

const path = require("node:path");
const fs = require("node:fs");
const { spawn } = require("node:child_process");

function main() {
  const port = String(process.env.FRONTEND_PORT || "23457");
  const env = {
    ...process.env,
    FRONTEND_PORT: port,
  };
  // Spawn Vite directly with the current Node executable — avoids re-entering
  // the invoking package manager via npm_execpath (see scripts/run-dev.js).
  const frontendDir = path.join(__dirname, "..", "frontend");
  const viteBin = path.join(frontendDir, "node_modules", "vite", "bin", "vite.js");
  if (!fs.existsSync(viteBin)) {
    console.error(`[frontend:dev] Vite is not installed at ${viteBin}. Run \`pnpm install\` first.`);
    process.exit(1);
  }

  const child = spawn(process.execPath, [viteBin, "--port", port, "--host"], {
    cwd: frontendDir,
    stdio: "inherit",
    env,
  });

  child.on("error", (err) => {
    console.error(`[frontend:dev] failed to start: ${err.message}`);
    process.exit(1);
  });

  child.on("exit", (code) => {
    process.exit(code ?? 0);
  });
}

main();
