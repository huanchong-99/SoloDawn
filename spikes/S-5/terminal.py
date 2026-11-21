"""Native Windows: python -m pip install pywinpty==3.0.2; python terminal.py"""

import ctypes
import os
import subprocess
import sys
import time

from winpty import Backend, PTY


def console():
    # Opt into console Ctrl+C even when the VM launcher ignores it.
    if not ctypes.windll.kernel32.SetConsoleCtrlHandler(None, False):
        raise ctypes.WinError()
    initial = os.get_terminal_size()
    print(f"READY {initial.columns}x{initial.lines}", flush=True)
    deadline = time.monotonic() + 8
    while (size := os.get_terminal_size()) == initial:
        if time.monotonic() >= deadline:
            raise TimeoutError("console size did not change")
        time.sleep(0.01)
    print(f"READY {size.columns}x{size.lines}", flush=True)
    while True:
        command = input()
        if command == "quit":
            return 37
        if command == "wait":
            try:
                print("WORKING", flush=True)
                time.sleep(60)
                raise RuntimeError("work finished without interruption")
            except KeyboardInterrupt:
                print("CANCELLED", flush=True)
        else:
            print(f"REPLY {command.upper()}", flush=True)


def probe():
    pty = PTY(80, 24, backend=Backend.ConPTY)
    args = subprocess.list2cmdline([__file__, "--child"])
    if not pty.spawn(sys.executable, cmdline=" " + args):
        raise RuntimeError("spawn failed")

    def expect(marker):
        data = ""
        deadline = time.monotonic() + 8
        while marker not in data:
            if time.monotonic() >= deadline:
                raise TimeoutError(f"expected {marker!r}; got {data!r}")
            data += pty.read(blocking=False)
            time.sleep(0.01)
        print(marker)

    expect("READY 80x24")
    pty.set_size(100, 36)
    expect("READY 100x36")
    pty.write("wait\r")
    expect("WORKING")
    pty.write("\x03")
    expect("CANCELLED")
    pty.write("pong\r")
    expect("REPLY PONG")
    pty.write("quit\r")
    deadline = time.monotonic() + 8
    while pty.isalive() and time.monotonic() < deadline:
        time.sleep(0.01)
    code = pty.get_exitstatus()
    if code != 37:
        raise RuntimeError(f"unexpected exit status {code}")
    print(f"EXIT {code}")


if __name__ == "__main__":
    if sys.argv[1:] == ["--child"]:
        sys.exit(console())
    probe()
