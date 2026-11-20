"""Run from a prepared SoloDawn worktree with official Go and Node SDKs on PATH."""

from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time


tree = Path.cwd().resolve()
scratch = tree / ".s10b"
profile = Path(__file__).with_name("check.sb").resolve()
go = Path(shutil.which("go")).resolve().parents[1]
node = Path(shutil.which("node")).resolve().parents[1]
sandbox = [
    "/usr/bin/sandbox-exec", "-D", f"TREE={tree}", "-D", f"GO={go}",
    "-D", f"NODE={node}", "-f", str(profile),
]
env = {
    "PATH": f"{go}/bin:{node}/bin:/usr/bin:/bin",
    "HOME": str(scratch / "home"), "TMPDIR": str(scratch / "tmp"),
    "GOMODCACHE": str(scratch / "mod"),
    "GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off",
    "CGO_ENABLED": "0", "CI": "true",
}
for name in ["home", "tmp"]:
    (scratch / name).mkdir(parents=True, exist_ok=True)


def run(command, prefix, *args):
    start = time.monotonic()
    result = subprocess.run([*prefix, "/bin/sh", "-ec", command, "check", *args],
                            env=env, cwd=tree, stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    print(result.stdout, end="", flush=True)
    result.check_returncode()
    return time.monotonic() - start


def controls():
    # Only the parent has the listening socket. The check is a shell grandchild.
    probe = """
    const fs = require('node:fs');
    const net = require('node:net');
    const [file, port, expected] = process.argv.slice(1);
    async function check(name, action) {
      let result = 'ok';
      try { await action(); } catch (err) { result = err.code; }
      console.log(name, result);
      if (result !== expected) process.exitCode = 1;
    }
    (async () => {
      fs.writeFileSync('.s10b/inside', 'ok');
      await check('outside read', () => fs.readFileSync(file));
      await check('outside write', () => fs.appendFileSync(file, 'x'));
      await check('loopback TCP', () => new Promise((resolve, reject) => {
        const s = net.connect(Number(port), '127.0.0.1');
        s.on('error', reject);
        s.on('connect', () => { s.destroy(); resolve(); });
      }));
    })();
    """
    with tempfile.TemporaryDirectory(dir=Path.home()) as d, socket.socket() as listener:
        sentinel = Path(d) / "sentinel"
        sentinel.write_text("control")
        listener.bind(("127.0.0.1", 0))
        listener.listen()
        port = str(listener.getsockname()[1])
        for prefix, expected in [([], "ok"), (sandbox, "EPERM")]:
            run('node -e "$1" "$2" "$3" "$4"; test -f .s10b/inside',
                prefix, probe, str(sentinel), port, expected)
        assert sentinel.read_text() == "controlx"


def builds(prefix):
    commands = {
        "node": "cd web && node node_modules/vite/bin/vite.js build && "
                "node node_modules/vitest/vitest.mjs run --pool=threads src/",
        "go": 'go build -buildvcs=false -o "$GOCACHE/solodawn" ./cmd/solodawn && '
              '"$GOCACHE/solodawn" version && go test -buildvcs=false -count=1 '
              './internal/proto ./internal/identity ./internal/store',
    }
    with tempfile.TemporaryDirectory(dir=scratch) as cache:
        env["GOCACHE"] = cache
        for project, command in commands.items():
            elapsed = run(command, prefix)
            print(f"{'sandbox' if prefix else 'baseline'} {project} {elapsed:.3f}s", flush=True)


if __name__ == "__main__":
    run("go telemetry off", sandbox)
    controls()
    # Alternate execution order within pairs; each Go compile starts with an empty cache.
    for pair in ((sandbox, []), ([], sandbox), (sandbox, [])):
        for prefix in pair:
            builds(prefix)
