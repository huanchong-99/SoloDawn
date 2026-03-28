#!/usr/bin/env node

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const net = require("node:net");
const { spawn, spawnSync } = require("node:child_process");
const { getPorts } = require("./setup-dev-environment");

const children = new Set();
let shuttingDown = false;
const devLockPath = path.join(os.tmpdir(), "solodawn", "run-dev.lock");
let lockFd = null;

function getPathKey(env) {
  return Object.keys(env).find((name) => name.toLowerCase() === "path") ?? "PATH";
}

function resolveExecutable(command, env = process.env) {
  if (typeof command !== "string" || command.length === 0) {
    return command;
  }
  if (path.isAbsolute(command) || command.includes("/") || command.includes("\\")) {
    return command;
  }

  const pathValue = env[getPathKey(env)];
  if (typeof pathValue !== "string" || pathValue.length === 0) {
    return command;
  }

  const extensions =
    process.platform === "win32"
      ? (env.PATHEXT ?? process.env.PATHEXT ?? ".EXE;.CMD;.BAT;.COM")
          .split(";")
          .filter(Boolean)
      : [""];
  const names =
    process.platform === "win32" && path.extname(command) === ""
      ? extensions.map((ext) => `${command}${ext}`)
      : [command];

  for (const dir of pathValue.split(path.delimiter).filter(Boolean)) {
    for (const name of names) {
      const candidate = path.join(dir, name);
      try {
        fs.accessSync(candidate, fs.constants.X_OK);
        return candidate;
      } catch {
        // Ignore and continue checking other PATH entries.
      }
    }
  }

  return command;
}

function isProcessAlive(pid) {
  if (!Number.isInteger(pid) || pid <= 0) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function acquireDevLock() {
  fs.mkdirSync(path.dirname(devLockPath), { recursive: true });

  const tryAcquire = () => {
    lockFd = fs.openSync(devLockPath, "wx");
    fs.writeFileSync(lockFd, `${process.pid}\n`, { encoding: "utf8" });
  };

  try {
    tryAcquire();
    return;
  } catch (error) {
    if (error?.code !== "EEXIST") {
      throw error;
    }
  }

  let existingPid = null;
  try {
    const content = fs.readFileSync(devLockPath, "utf8").trim();
    const parsed = Number(content);
    if (Number.isInteger(parsed) && parsed > 0) {
      existingPid = parsed;
    }
  } catch {
    // Ignore stale/unreadable lock file content and attempt cleanup.
  }

  if (existingPid && isProcessAlive(existingPid)) {
    throw new Error(
      `Another dev environment is already running (pid ${existingPid}). Stop it before starting a new one.`
    );
  }

  // Stale lock: remove once and retry.
  try {
    fs.unlinkSync(devLockPath);
  } catch {
    // Ignore remove errors; retry will surface a real failure if any.
  }
  tryAcquire();
}

function releaseDevLock() {
  if (lockFd !== null) {
    try {
      fs.closeSync(lockFd);
    } catch {
      // ignore
    }
    lockFd = null;
  }
  try {
    fs.unlinkSync(devLockPath);
  } catch {
    // ignore
  }
}

function envFlag(name, defaultValue = false) {
  const raw = process.env[name];
  if (raw == null || raw === "") return defaultValue;
  return /^(1|true|yes|on)$/i.test(raw.trim());
}

function windowsPortCleanupMode() {
  const raw = (process.env.DEV_WINDOWS_PORT_CLEANUP_MODE ?? "off")
    .trim()
    .toLowerCase();
  if (raw === "off" || raw === "none") return "off";
  if (raw === "force") return "force";
  return "graceful";
}

function taskkillPidWindows(pid, { force = false } = {}) {
  if (!Number.isInteger(pid) || pid <= 0) return false;
  const args = ["/pid", String(pid)];
  if (envFlag("DEV_WINDOWS_TASKKILL_TREE", false)) {
    args.push("/t");
  }
  if (force) {
    args.push("/f");
  }
  const result = spawnSync(resolveExecutable("taskkill"), args, { stdio: "ignore" });
  return result.status === 0;
}

/**
 * Stop all child processes and exit
 */
function stop(code) {
  if (shuttingDown) return;
  shuttingDown = true;

  console.log("\n[dev] Shutting down...");

  for (const child of children) {
    if (!child.killed) {
      if (process.platform === "win32") {
        // Default to non-forceful shutdown; allow force fallback via env.
        taskkillPidWindows(child.pid, {
          force: envFlag("DEV_WINDOWS_FORCE_KILL_ON_STOP", false),
        });
      } else {
        child.kill("SIGTERM");
      }
    }
  }

  // Force exit after 5 seconds if processes don't stop
  setTimeout(() => {
    console.log("[dev] Force exit");
    releaseDevLock();
    process.exit(code ?? 0);
  }, 5000);
}

/**
 * Resolve command and args for Windows compatibility
 */
function resolveCommand(command, args) {
  if (process.platform !== "win32") {
    return { command, args };
  }

  if (command === "npm") {
    const npmExecPath = process.env.npm_execpath;
    if (npmExecPath) {
      return { command: process.execPath, args: [npmExecPath, ...args] };
    }
    return { command: "cmd.exe", args: ["/d", "/s", "/c", "npm", ...args] };
  }

  return { command, args };
}

/**
 * Run a command with proper error handling
 */
function run(name, command, args, options) {
  const resolved = resolveCommand(command, args);
  const spawnOptions = {
    stdio: "inherit",
    ...options
  };
  const executable = resolveExecutable(resolved.command, spawnOptions.env ?? process.env);
  console.log(
    `[dev] Starting ${name}: ${resolved.command} ${resolved.args.join(" ")}`
  );

  const child = spawn(executable, resolved.args, spawnOptions);

  children.add(child);

  child.on("error", (err) => {
    console.error(`[dev] Failed to start ${name}: ${err.message}`);
    stop(1);
  });

  child.on("exit", (code, signal) => {
    children.delete(child);
    if (!shuttingDown) {
      console.error(`[dev] ${name} exited unexpectedly (code: ${code}, signal: ${signal})`);
      stop(code ?? 1);
    }
  });

  return child;
}

/**
 * Wait for a TCP port to start accepting connections.
 */
function waitForPort(port, options = {}) {
  const host = options.host ?? "127.0.0.1";
  const timeoutMs = options.timeoutMs ?? 300000;
  const retryDelayMs = options.retryDelayMs ?? 1000;
  const connectTimeoutMs = options.connectTimeoutMs ?? 1000;
  const name = options.name ?? "service";
  const startedAt = Date.now();

  return new Promise((resolve, reject) => {
    const tryConnect = () => {
      if (Date.now() - startedAt > timeoutMs) {
        reject(
          new Error(
            `${name} did not become ready on ${host}:${port} within ${timeoutMs}ms`
          )
        );
        return;
      }

      const socket = new net.Socket();
      let settled = false;

      const finalize = (ok) => {
        if (settled) return;
        settled = true;
        socket.destroy();

        if (ok) {
          resolve();
          return;
        }

        setTimeout(tryConnect, retryDelayMs);
      };

      socket.setTimeout(connectTimeoutMs);
      socket.once("connect", () => finalize(true));
      socket.once("timeout", () => finalize(false));
      socket.once("error", () => finalize(false));
      socket.connect(port, host);
    };

    tryConnect();
  });
}

/**
 * Check whether a TCP port can be bound on the given host.
 * Returns false for expected "port already in use" style failures.
 */
function canBindPortOnHost(port, host) {
  return new Promise((resolve, reject) => {
    const server = net.createServer();

    server.once("error", (error) => {
      if (error?.code === "EAFNOSUPPORT") {
        // Host family is not supported in current environment; ignore.
        resolve(true);
        return;
      }
      if (error?.code === "EADDRINUSE" || error?.code === "EACCES") {
        resolve(false);
        return;
      }
      reject(error);
    });

    server.once("listening", () => {
      server.close((closeError) => {
        if (closeError) {
          reject(closeError);
          return;
        }
        resolve(true);
      });
    });

    server.listen(port, host);
  });
}

/**
 * Check whether a TCP port can be bound for both IPv4 and IPv6 listeners.
 */
async function canBindPort(port) {
  const hosts =
    process.platform === "win32"
      ? ["127.0.0.1", "0.0.0.0", "::1", "::"]
      : ["127.0.0.1", "::1"];

  for (const host of hosts) {
    const available = await canBindPortOnHost(port, host);
    if (!available) {
      return false;
    }
  }

  return true;
}

/**
 * Find TCP LISTEN PIDs for a port (Windows only).
 */
function findListeningPidsWindows(port) {
  const result = spawnSync(resolveExecutable("netstat"), ["-ano", "-p", "tcp"], {
    encoding: "utf8",
  });

  if (result.status !== 0) {
    return [];
  }

  const pids = new Set();
  const lines = result.stdout.split(/\r?\n/);
  for (const line of lines) {
    // Example:
    // TCP    0.0.0.0:23457   0.0.0.0:0   LISTENING   12345
    const match = line.match(
      /^\s*TCP\s+(\S+):(\d+)\s+\S+\s+LISTENING\s+(\d+)\s*$/i
    );
    if (!match) continue;
    const linePort = Number(match[2]);
    const pid = Number(match[3]);
    if (linePort === port && Number.isFinite(pid) && pid > 0) {
      pids.add(pid);
    }
  }

  return [...pids];
}

/**
 * Best-effort cleanup for stale listeners on dev ports.
 */
async function ensurePortAvailable(port, label) {
  if (await canBindPort(port)) {
    return;
  }

  if (process.platform !== "win32") {
    throw new Error(`${label} port ${port} is already in use.`);
  }

  const pids = findListeningPidsWindows(port).filter((pid) => pid !== process.pid);
  if (!pids.length) {
    throw new Error(
      `${label} port ${port} is already in use. Could not resolve owning PID.`
    );
  }

  const cleanupMode = windowsPortCleanupMode();
  if (cleanupMode === "off") {
    throw new Error(
      `${label} port ${port} is occupied by PID(s): ${pids.join(", ")}. Auto cleanup is disabled (DEV_WINDOWS_PORT_CLEANUP_MODE=off).`
    );
  }

  console.warn(
    `[dev] ${label} port ${port} is occupied by PID(s): ${pids.join(", ")}. Attempting ${cleanupMode} cleanup...`
  );

  for (const pid of pids) {
    taskkillPidWindows(pid);
  }

  // Give Windows a short moment to release sockets.
  await new Promise((resolve) => setTimeout(resolve, 800));

  if (!(await canBindPort(port)) && cleanupMode === "force") {
    console.warn(
      `[dev] ${label} port ${port} still in use. Escalating to force cleanup...`
    );
    for (const pid of pids) {
      taskkillPidWindows(pid, { force: true });
    }
    await new Promise((resolve) => setTimeout(resolve, 800));
  }

  if (!(await canBindPort(port))) {
    throw new Error(
      `${label} port ${port} is still in use after cleanup attempt. Set DEV_WINDOWS_PORT_CLEANUP_MODE=force for legacy behavior.`
    );
  }
}

// Handle termination signals
process.on("SIGINT", () => stop(0));
process.on("SIGTERM", () => stop(0));
process.on("exit", () => releaseDevLock());

// Handle Windows Ctrl+C
if (process.platform === "win32") {
  require("node:readline")
    .createInterface({
      input: process.stdin,
      output: process.stdout,
    })
    .on("SIGINT", () => stop(0));
}

async function main() {
  acquireDevLock();
  console.log("[dev] Setting up development environment...");

  const ports = await getPorts();

  console.log(`[dev] Frontend port: ${ports.frontend}`);
  console.log(`[dev] Backend port: ${ports.backend}`);

  // Clean stale listeners up-front to avoid partial startup followed by
  // cascading shutdown (frontend failure causing backend to exit).
  await ensurePortAvailable(ports.backend, "Backend");
  await ensurePortAvailable(ports.frontend, "Frontend");

  // Auto-detect protoc if not already set (required by feishu-connector gRPC build)
  let protoc = process.env.PROTOC;
  if (!protoc) {
    const candidates = [
      String.raw`C:\protoc\bin\protoc.exe`,
      String.raw`C:\tools\protoc\bin\protoc.exe`,
      path.join(os.homedir(), "protoc", "bin", "protoc.exe"),
    ];
    for (const c of candidates) {
      if (fs.existsSync(c)) { protoc = c; break; }
    }
  }

  const env = {
    ...process.env,
    FRONTEND_PORT: String(ports.frontend),
    BACKEND_PORT: String(ports.backend),
    DISABLE_WORKTREE_ORPHAN_CLEANUP: "1",
    RUST_LOG: "debug",
    ...(protoc ? { PROTOC: protoc } : {}),
  };

  // Start backend
  run(
    "backend",
    "cargo",
    ["watch", "-w", "crates", "--", "cargo", "run", "--bin", "server"],
    { env }
  );

  console.log(
    `[dev] Waiting for backend readiness on 127.0.0.1:${ports.backend}...`
  );
  await waitForPort(ports.backend, {
    host: "127.0.0.1",
    timeoutMs: 300000,
    retryDelayMs: 1000,
    connectTimeoutMs: 1000,
    name: "backend",
  });
  console.log("[dev] Backend is ready");

  // Re-check frontend port just before launch to handle races from late
  // terminating/stale previous vite processes.
  await ensurePortAvailable(ports.frontend, "Frontend");

  // Start frontend
  run(
    "frontend",
    "npm",
    ["run", "dev", "--", "--port", String(ports.frontend), "--host"],
    {
      env,
      cwd: path.join(__dirname, "..", "frontend"),
    }
  );

  console.log("[dev] Development servers started successfully");
  console.log("[dev] Press Ctrl+C to stop");
}

function handleStartupFailure(err) {
  releaseDevLock();
  console.error("[dev] Failed to start development environment:", err);
  process.exit(1);
}

function start() {
  // CommonJS entrypoint: keep startup async handling inside a sync launcher.
  const startup = main();
  startup.catch(handleStartupFailure);
}

start();
