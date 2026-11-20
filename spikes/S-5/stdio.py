"""Interrupt a cooperative structured child over anonymous pipes, not a PTY."""

import asyncio
import json
import sys
import threading


def worker():
    stop = threading.Event()

    def turn():
        print("TURN ACTIVE", flush=True)
        print("INTERRUPTED" if stop.wait(30) else "ELAPSED", flush=True)

    task = threading.Thread(target=turn)
    task.start()
    request = json.loads(input())
    if request["request"]["subtype"] != "interrupt":
        raise ValueError("expected interrupt")
    stop.set()
    task.join()
    print("NEXT: " + input().upper(), flush=True)


async def probe():
    p = await asyncio.create_subprocess_exec(
        sys.executable, __file__, "--child",
        stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE,
    )
    try:
        for want, reply in [
            ("TURN ACTIVE", '{"type":"control_request","request_id":"1","request":{"subtype":"interrupt"}}'),
            ("INTERRUPTED", "ping"),
            ("NEXT: PING", None),
        ]:
            got = (await asyncio.wait_for(p.stdout.readline(), 8)).decode().strip()
            if got != want:
                raise ValueError(f"expected {want!r}, got {got!r}")
            print(got)
            if reply is not None:
                p.stdin.write((reply + "\n").encode())
                await p.stdin.drain()
        if await asyncio.wait_for(p.wait(), 8) != 0:
            raise RuntimeError("child failed")
        print("EXIT 0")
    finally:
        p.stdin.close()


if __name__ == "__main__":
    if sys.argv[1:] == ["--child"]:
        worker()
    else:
        asyncio.run(probe())
