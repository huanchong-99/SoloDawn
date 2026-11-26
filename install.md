# Install and pair SoloDawn

From the [SoloDawn releases](https://github.com/huanchong-99/SoloDawn/releases),
download `SHA256SUMS` and the binary for your computer **from the same release**.
Save both in an empty directory. Each binary is named
`solodawn_<version>_<os>_<arch>`, with an extra `.exe` on Windows.
`<version>` is the release's tag as the releases page shows it, including its
leading `v`. A tag ending in `-rc.N` is a release candidate, a pre-release for testing.
For example, a Linux x86-64 binary for `v1.10.2` is named
`solodawn_v1.10.2_linux_amd64`.

| System | `<os>` | `<arch>` |
| --- | --- | --- |
| Linux | `linux` | `amd64` for x86-64, `arm64` for AArch64 |
| macOS | `darwin` | `amd64` for Intel, `arm64` for Apple silicon |
| Windows | `windows` | `amd64` for x64, `arm64` for ARM64 |

## Verify and install

In the download directory, set `asset` to the downloaded binary's filename.
The commands below check the SHA-256 digest and filename against the manifest
before installing the executable.

**Linux and macOS** (Bash or Zsh)

```sh
asset='REPLACE_WITH_BINARY_FILENAME'
if [ "$(uname -s)" = Darwin ]; then
  shasum -a 256 "$asset"
else
  sha256sum "$asset"
fi | grep -Fxf SHA256SUMS &&
  mkdir -p "$HOME/.local/bin" &&
  install -m 755 "$asset" "$HOME/.local/bin/solodawn" &&
  if [ "$(uname -s)" = Darwin ]; then
    xattr -d com.apple.quarantine "$HOME/.local/bin/solodawn" 2>/dev/null || true
  fi
```

On success the matching line appears. On macOS the last step clears the
quarantine mark that a browser download carries, which would otherwise
stop the ad-hoc-signed binary from running; it runs only after the checksum
matched. On macOS with the Application Firewall enabled, macOS asks once
whether `solodawn` may accept incoming connections: allow it, or direct
connections to it fail and only the relay works. If it never asks and direct
connections fail, remove a blocked `solodawn` or other Go program (often a
`.test` binary) under System Settings, Network, Firewall, Options, then
restart the node. macOS may ask again after an update: allow it again. Add
`$HOME/.local/bin` to your `PATH` if needed.

**Windows** (PowerShell)

```powershell
$asset = 'REPLACE_WITH_BINARY_FILENAME.exe'
$digest = (Get-FileHash -Algorithm SHA256 -LiteralPath $asset -ErrorAction Stop).Hash
if ((Get-Content -LiteralPath SHA256SUMS -ErrorAction Stop) -notcontains "$digest  $asset") {
    throw "SHA256SUMS has no matching digest and filename"
}
$bin = Join-Path $env:LOCALAPPDATA 'Programs\SoloDawn'
New-Item -ItemType Directory -Force -Path $bin | Out-Null
Copy-Item -LiteralPath $asset -Destination (Join-Path $bin 'solodawn.exe')
```

Add the installation directory to your user `Path` or run the executable
by its full path.

## What the node needs

SoloDawn runs no Git program itself: it needs neither `git` nor `gh`, and it
makes no clone, branch, merge or pull request. What it needs on a computer is
the AI command-line tools you want to use (`claude`, `codex`, `opencode`,
`qwen`), found on your `PATH` and signed in.

The AIs of a group work in folders you choose, on your own computer, at full
permission: an AI can read and change everything in its folder. The folder
is not a copy. If your project uses Git, commit or set aside your own
changes before you give the folder to an AI. If the AI uses Git, it does so
by the project's own rules, so `git` must be installed for that.

## Pair with the browser

Run `solodawn version` and confirm it prints the tag you downloaded.
Then use the pairing secret from the
onboarding prompt within its ten-minute window. Replace the placeholders
with that secret and a label for this computer.

```text
solodawn pair <secret> --name "<label>"
```

Check the pairing result against the device label and fingerprint shown
in the browser.

When pairing succeeds, the node registers itself to start at login:
`~/Library/LaunchAgents/org.solodawn.node.plist` on macOS, the systemd user
unit `solodawn.service` on Linux, and the scheduled task `SoloDawn-<user>` on
Windows.

Give an AI only folders of projects you trust, because their `CLAUDE.md`,
`AGENTS.md` and hooks are used by the AIs that work in them. A Qwen Code AI
may load a project's own configuration (`.qwen/settings.json`, `QWEN.md`)
even in a group session: Qwen Code has no known switch to keep it out.

## Update

Version 1 does not update itself. The node keeps its pairing and groups;
they are not in the binary. To update a computer, download the new release's
`SHA256SUMS` and binary into an empty directory, then for your system do three
things in this order: stop the node, replace the binary with the commands of
"Verify and install" above (set `asset` to the new filename in that block),
and start the node. Confirm the new tag with `solodawn version`.

### From `v3.0.0-rc.1`

A node of `v3.0.0-rc.1` cannot talk to one of a later release candidate:
update every node and every browser together. At its first start the new
version deletes every group on the node, with its messages, tasks, cards,
notes, knowledge-base documents and sessions. Pairing, your own devices,
account types and peers stay. The people who were in your groups are
forgotten and must be invited again by link. A file made by
`solodawn group export` on rc.1 cannot be imported. The old `projects/`
folder in the data directory is not touched: it may hold task branches and
worktrees that were never pushed, so copy what you need, then delete it.

If the node runs in a terminal, press Ctrl+C there first, replace the binary,
then start `solodawn node` again. If it runs as the login service, use the
commands below.

**Linux**

```sh
systemctl --user stop solodawn.service
```

Replace the binary, then:

```sh
systemctl --user start solodawn.service
```

**macOS**

```sh
launchctl bootout "gui/$(id -u)/org.solodawn.node" 2>/dev/null || true
```

Replace the binary, then:

```sh
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/org.solodawn.node.plist"
```

**Windows** (PowerShell). A running `solodawn.exe` cannot be overwritten, so
end the task and wait until no `solodawn` process is left:

```powershell
$name = 'SoloDawn-' + [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
Stop-ScheduledTask -TaskPath '\' -TaskName $name
for ($i = 0; $i -lt 10 -and (Get-Process -Name solodawn -ErrorAction SilentlyContinue); $i++) {
    Start-Sleep -Seconds 1
}
Get-Process -Name solodawn -ErrorAction SilentlyContinue | ForEach-Object { Stop-Process -Id $_.Id -Force }
```

Replace the binary, then:

```powershell
$name = 'SoloDawn-' + [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
Start-ScheduledTask -TaskPath '\' -TaskName $name
```

## Back up a group

`solodawn group export <group id> <file>` without `--to` writes a copy of a
group that this node is the home of, and replaces `<file>` if it exists.
The node must be running. SoloDawn schedules nothing; run the command from
the system's timer.

**Linux and macOS**: add one line with `crontab -e`, here every day at 03:00
(adjust the two paths):

```text
0 3 * * * "$HOME/.local/bin/solodawn" group export <group id> "$HOME/solodawn-backup.db"
```

**Windows** (PowerShell), every day at 03:00:

```powershell
$exe = "$env:LOCALAPPDATA\Programs\SoloDawn\solodawn.exe"
$action = New-ScheduledTaskAction -Execute $exe -Argument "group export <group id> `"$env:USERPROFILE\solodawn-backup.db`""
Register-ScheduledTask -TaskName SoloDawnBackup -Action $action -Trigger (New-ScheduledTaskTrigger -Daily -At 3am)
```

To restore on another computer of the same owner, run
`solodawn group import <file>` there (`planning/contracts/step-2/export.md`
section 4).

## Run your own rendezvous

Skip this section unless you were asked to run a server. The pairing steps
above use the rendezvous of the page that gave you the prompt. To run your own
server and protect your installation from the outside, see
[Self-host a rendezvous and protect it](https://github.com/huanchong-99/SoloDawn/blob/main/docs/cookbook/self-hosting.md).

## After a failure

Run the command for your system and send the archive it writes, with the time
of the failure; the times in the log are UTC. Do it soon: the node keeps only
`node.log` and the file before it, 10 MB each. On macOS and Linux the node
need not be stopped; on Windows it must be (below).
The lines hold ids, counts and states, not messages, file contents, tokens or
keys; an error line can name a file, and a `turn.stray` line the network address
that sent a stray packet to the node's relay socket.

**macOS**

```sh
tar czf ~/Desktop/solodawn-logs.tgz -C "$HOME/Library/Application Support/solodawn" logs
```

**Linux**

```sh
tar czf ~/solodawn-logs.tgz -C "${XDG_DATA_HOME:-$HOME/.local/share}/solodawn" logs
```

**Windows** (PowerShell). Older Windows `tar` cannot open a file the node is
writing, so stop the node first with the commands of "Update" above (Ctrl+C
in its terminal, or the `Stop-ScheduledTask` block), then:

```powershell
tar -a -cf "$([Environment]::GetFolderPath('Desktop'))\solodawn-logs.zip" -C "$env:LocalAppData\solodawn" logs
```
