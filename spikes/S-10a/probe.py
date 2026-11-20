"""Run from the disposable checkout root with the same LAB and PATH as run.sh."""

import errno
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile


def denies(expected, operation):
    try:
        operation()
    except OSError as exc:
        if exc.errno == expected:
            return errno.errorcode[expected]
        raise
    raise AssertionError("operation unexpectedly succeeded")


def open_for_write(path):
    # No truncation, including on the host during the positive control.
    os.close(os.open(path, os.O_WRONLY))


def connect(host, port):
    with socket.create_connection((host, port), timeout=2):
        pass


def child(private, toolfile, port):
    with tempfile.TemporaryFile(dir=".s10a") as file:
        file.write(b"check")
        file.seek(0)
        assert file.read() == b"check"
    assert "S10A_FAKE_TOKEN" not in os.environ
    print("worktree read/write OK; parent environment marker absent")
    print("private file read:", denies(errno.ENOENT, Path(private).read_bytes))
    print("create at hidden host path:",
          denies(errno.EROFS, lambda: Path(private).touch(exist_ok=False)))
    print("toolchain write:", denies(errno.EROFS, lambda: open_for_write(toolfile)))
    interfaces = [name for _, name in socket.if_nameindex()]
    assert interfaces == ["lo"], interfaces
    print("interfaces:", interfaces)
    print("host listener:", denies(errno.ECONNREFUSED, lambda: connect("127.0.0.1", int(port))))
    print("off-link TCP:", denies(errno.ENETUNREACH, lambda: connect("192.0.2.1", 443)))


def controls():
    goroot = subprocess.check_output(["go", "env", "GOROOT"], text=True).strip()
    toolfile = Path(goroot) / "VERSION"
    open_for_write(toolfile)
    with tempfile.NamedTemporaryFile(dir=Path.home()) as private, socket.socket() as listener:
        private.write(b"private fixture")
        private.flush()
        assert Path(private.name).read_bytes() == b"private fixture"
        listener.bind(("127.0.0.1", 0))
        listener.listen()
        port = listener.getsockname()[1]
        connect("127.0.0.1", port)
        print("host controls: private file readable, toolchain writable, listener reachable",
              flush=True)
        subprocess.run(
            ["bash", "spikes/S-10a/run.sh", "sandbox", "probe", private.name, toolfile, str(port)],
            env={**os.environ, "S10A_FAKE_TOKEN": "fixture"},
            check=True,
        )


if __name__ == "__main__":
    if sys.argv[1:2] == ["child"]:
        child(*sys.argv[2:])
    else:
        controls()
