# Issues

The issue record of this project: bugs, deviations from a requirement or from the owner's expectation, and known limitations, open and resolved. Read it before implementing or fixing anything. Add an entry for every such finding, not for every step; update the entry when it is resolved, and keep resolved entries.

Entry format: `### YYYY-MM-DD · type · title`, where type is `bug`, `deviation` (a feature does not do what was required or expected) or `limitation` (known, accepted for now). Then the symptom, the cause with its location (`file:line` where it is code), and the status (open, or resolved with the version or commit). Link a lesson, decision or postmortem instead of repeating it.

## Open

### 2026-11-20 · limitation · The gate measures CRAP 6.01 for `deliver`, which Go covers fully

- Symptom: `crap.py` gives `deliver` in `internal/peer/ctl.go` a CRAP score of 6.01, above the limit of 6, while `go test -cover` reports `internal/peer` at 100% of statements.
- Cause: the empty `case <-c.Done():` of the `select` in `deliver` (`internal/peer/ctl.go:91`) is a cover block with no statements and count 0. Go leaves it out of its percentage; the gate's conversion of the cover profile into lines (the last command of `stages.fast` in `.project-standard/gates.toml`) counts its line as an uncovered line. That gives `deliver` 13 of 14 lines, 92.9%, at complexity 6. The same before PR-0 (9c09938f6).
- Effect: none on a change that leaves `deliver` alone, because the gate measures changed functions only. A change to `deliver`, or a CRAP run over the whole repository, reports it.
- Status: open. No code change, by the coordinator's decision of 2026-11-20.

### 2026-11-20 · bug · go-nat v0.2.0 gets four things wrong in port mapping

- Symptom: against NET-1's gateway (miniupnpd 2.3.9) in spike S-8 ([#134](https://github.com/huanchong-99/SoloDawn/pull/134)), go-nat failed four ways. (1) NAT-PMP returns the external port it asked for, not the one the router granted: it reported 55459 where the router mapped 30000, so a node would publish an unreachable port. (2) NAT-PMP `DeletePortMapping` leaves the mapping in place. (3) NAT-PMP calls ignore the context: with the router silent, a call with a 2 s context had not returned after 10 s. (4) UPnP asks only for random external ports and fails with 606 where the router restricts ports. Discovery also always takes 5 s.
- Cause: in go-nat v0.2.0, `natpmp.go:106` and `:115` discard the mapping result; `natpmp.go:125-127` only forgets the port; `natpmp.go:43` uses `natpmp.NewClient`, which retries for about 128 s (go-nat-pmp v1.0.2 `natpmp.go:34-35`); `nat.go:123-125` draws random ports and `upnp.go:250-275` never tries `AddAnyPortMapping`; `upnp.go:152` waits 5 s. Details in [`spikes/S-8/results.md`](https://github.com/huanchong-99/SoloDawn/blob/spike/S-8-portmap/spikes/S-8/results.md), section 1.
- Status: open, external. Avoided by the coordinator's decision of 2026-11-20: PR-3 uses go-nat-pmp (`NewClientWithTimeout`) and goupnp's `internetgateway2` (IGDv2 first, then IGDv1) directly, not go-nat, and replaces the go-nat pin in `deps_test.go` with these two.

### 2026-11-20 · limitation · pion drops `tcptype` from server-reflexive candidates

- Symptom: in spike S-8 ([#134](https://github.com/huanchong-99/SoloDawn/pull/134)), a node's mapped ICE-TCP listener published as `typ srflx ... tcptype passive` was never dialled: two symmetric NATs with UDP between them blocked and no TURN did not connect. The same endpoint published as `typ host ... tcptype passive` connected in 206 ms.
- Cause: pion ice v4.4.8 `UnmarshalCandidate` gives `tcptype` to host candidates only (`candidate_base.go:962-993`), so a srflx candidate has no TCP type and the agent creates no active candidate for it (`agent.go:1313-1323`).
- Workaround: publish a mapped TCP endpoint as `typ host ... tcptype passive` with the external address and the granted port (PR-3).
- Status: open, external; not reported upstream for now.

### 2026-11-20 · limitation · pion can select the relay while a direct pair is valid

- Symptom: in spike S-8 ([#134](https://github.com/huanchong-99/SoloDawn/pull/134)), 5 of 63 runs with TURN, a port-mapped answerer and a restricted-cone offerer selected the relay after 2.0 s, although direct pairs were valid from about 630 ms. None of 160 runs without a mapping, none of 160 with a mapped offerer, and none with another offerer NAT type did so.
- Cause, from the ICE trace of [run 44704969215](https://github.com/huanchong-99/SoloDawn/actions/runs/44704969215): a check relayed through the answerer's TURN allocation reaches the offerer before that relay candidate is trickled, so the offerer files it as peer-reflexive and validates the pair. When the relay candidate arrives, pion ice v4.4.8 replaces the peer-reflexive candidate but keeps the pair's old priority (`agent.go:1247-1262`, `setPriorityOverride`). That pair stays the best valid one, and the controlling agent nominates only the best valid pair once its type's wait has passed, 2 s for relay (`selection.go:74-84`). Details in [`spikes/S-8/results.md`](https://github.com/huanchong-99/SoloDawn/blob/spike/S-8-portmap/spikes/S-8/results.md), section 2.
- Workaround: the connection works over the relay, and the probe of `planning/flows/connect.md` section B moves it to a direct path.
- Status: open, external; not reported upstream for now.

### 2026-11-20 · limitation · Three peer scenarios time out on a Mac with its firewall on

- Symptom: "Both nodes can offer over the same two signalling paths", "A replayed offer is not processed" and "Early and repeated candidates" fail with `context deadline exceeded` after 20 s on macOS with the application firewall enabled, on `rebuild` as merged (815c52aa6) and with the audit fixes; the other 77 scenarios pass. They pass on Linux in CI, and on GitHub's `macos-latest` runner (#145).
- Cause, not verified: `features/peer_test.go` makes each node's sockets with `peer.Listen(0, 0)`, which listens on every interface (`internal/peer/sockets.go:22-23`), and the firewall holds incoming traffic to the unsigned test binary on addresses other than loopback. The package's own tests keep the sockets on loopback (`internal/peer/sockets.go:26-27`).
- Status: open. Bind the acceptance nodes to loopback as the unit tests do, or exclude the scenarios on macOS by tag.

### 2026-11-20 · limitation · Rendezvous steps reserve a port by closing it

- Symptom: not observed. The steps listen on 127.0.0.1:0, close the listener and give its port to `solodawn rendezvous`; another process can bind the port in between, and `rvListening` would take that process for the rendezvous.
- Cause: `features/rendezvous_test.go:162-169` and `features/relay_test.go:89-94`; the command cannot take a listener or report the port it chose.
- Status: open, until the command can listen on port 0 and print its address.

### 2026-11-20 · limitation · Three web unit tests time out on a fully busy machine

- Symptom: with 14 busy loops on 12 cores, `web/src/main.test.ts:4`, `web/src/lib/motion.test.ts:29` and `web/src/lib/prefs.test.ts:26` exceeded Vitest's 5 s test limit; one idle run right after `pnpm install` also failed two tests in two files (output not kept). CI passed.
- Cause: each is the first test of its file to import the client's modules afresh inside the test, so their transformation counts against the test's limit.
- Status: open; the gate can fail spuriously while other work loads the machine.

### 2026-11-20 · limitation · End-to-end tests cannot run a node until the node runtime is wired

- Symptom: `startNet` of `web/e2e/nodes.ts` waits until each node answers on its local socket. Every node exits with status 1 and `node runtime is unavailable` right after it makes its data directory and binds the socket, so a start that awaits nodes fails with that message. The harness's own scenarios start nodes with `awaitNodes: false`. A node is started with no flags, because `solodawn node` takes none yet: it has no signalling or web port and no rendezvous address, and the harness hands over only its HOME, data directory, socket and environment.
- Cause: `startNode` is a stub that returns that error until N-4 assembles the runtime (`internal/node/socket.go:80-84`); `cmd/solodawn/main.go:26-28` registers `node` without arguments.
- Status: open until N-4. What I-8 needs from N-4 is listed in [#132](https://github.com/huanchong-99/SoloDawn/pull/132).

### 2026-11-20 · limitation · A node's socket does not fit under a long HOME on macOS

- Symptom: with a HOME of more than about 60 bytes, `solodawn node` exits with `listen unix <HOME>/Library/Application Support/solodawn/sock: bind: invalid argument`. Seen on macOS with HOME under a session's temporary directory of over 100 bytes.
- Cause: a Unix socket path holds at most 103 bytes on macOS (107 on Linux), and on macOS the socket is 41 bytes below HOME (`internal/node/dir.go:23-24`, `internal/node/socket.go:23-27`). The system's temporary directory there (`/var/folders/…/T/`, 49 bytes) leaves room for a short name only, so `t.TempDir()` as HOME fails. `internal/node/socket_test.go:175-179`, `features/cmd_local_test.go:29-34` and `web/e2e/nodes.ts` create their directories under `/tmp` on macOS for this reason.
- Effect: none for an ordinary home directory such as `/Users/<name>`.
- Status: open, accepted for tests; a test that sets HOME for a node on macOS keeps it under `/tmp`.

### 2026-11-20 · deviation · End-to-end steps are not in a `.spec.ts` file

- Symptom: the feature-file convention (`planning/steps/parallel-plan.md` section 2, `planning/contracts/conventions.md` section 5) puts a web slice's steps in `web/features/<slice>.spec.ts`. The steps of `features/web_e2e.feature` are in `web/features/e2e.ts`, and the acceptance gate's `vitest run features` does not run them; the `Browser end-to-end` workflow does.
- Cause: `pnpm test` and the gates use `web/vite.config.ts`, whose Vitest default pattern would run a `web/features/*.spec.ts` without the global setup, Go and the browsers that end-to-end tests need. F-01c keeps `web/features/browser-crypto.ts` out of that run the same way.
- Status: open, accepted by the coordinator on 2026-11-20. `web/vite.config.ts` excludes `e2e/**` ([#132](https://github.com/huanchong-99/SoloDawn/pull/132)), so `web/e2e/*.spec.ts`, I-8's `connect.spec.ts` included, run only under `web/e2e/config.ts`.

### 2026-11-20 · limitation · Two workflows download the same pinned browsers

- Symptom: `.github/workflows/e2e.yml` repeats the download steps of `.github/workflows/browser-vectors.yml`: Chrome for Testing 137, Edge 137 and Firefox 136 with their URLs and SHA-256 sums. A change of version has to be made in both files.
- Cause: E-0 added the end-to-end workflow beside F-01c's without changing it.
- Status: open. A composite action that both workflows use can replace the steps later (coordinator, 2026-11-20).

### 2026-11-20 · limitation · The end-to-end harness runs on macOS and Linux only

- Symptom: the global setup of `web/e2e/config.ts` throws on Windows.
- Cause: `web/e2e/nodes.ts` starts every process as the leader of its own process group, stops it with `process.kill(-pgid)`, and finds what a dead test process left with `ps`; Windows has neither.
- Status: open. The browser workflows run on Linux and macOS; I-11 decides whether end-to-end tests run on Windows.

### 2026-11-20 · limitation · Fake CLIs lack invalid-login and exhausted-limit captures

- Symptom: neither scripted CLI covers an invalid-login startup error or an exhausted rate limit.
- Cause: the [macOS driver spike](https://github.com/huanchong-99/SoloDawn/pull/57) captured signed-in sessions and rate-limit updates, not these failures.
- Status: open. Add scripts from real captures before B-14 handles either failure.

### 2026-11-20 · limitation · Web client can paint one frame in the wrong theme

- Symptom: when the theme chosen in the browser (`sd-theme` = `light` or `dark`) is the opposite of the system setting, the page can paint one frame in the system theme while it loads, then switches.
- Cause: `data-theme` is set on `<html>` by the module script after the bundle has loaded (`web/src/lib/prefs.svelte.ts:40-47`); until then `tokens.css` follows the system (`web/src/styles/tokens.css:6`). `web/index.html` has no inline script that sets it earlier.
- Status: open. No inline script for now, by the coordinator's decision of 2026-11-20 (one inline line is the usual fix; no gate measures it and a later Content-Security-Policy may forbid it).

### 2026-11-20 · limitation · Older browsers get an unstyled notice or a blank page

- Symptom: a browser below the supported versions (Chrome and Edge 137, Firefox 136, Safari 17.5; [decision](docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md)) sees the upgrade notice. Chrome and Edge 123 to 136 and Firefox 120 to 135 show it styled. A browser without `light-dark()` (Chrome below 123, Firefox below 120, Safari below 17.5) shows it with default colours, and one without nested rules that start with a type selector (Chrome below 120, Firefox below 117, Safari below 17.2) also loses the layout those rules give; a browser that cannot run the bundle shows a blank page.
- Cause: the notice is mounted by the same bundle (`web/src/lib/boot.ts:32-34`) and styled by the same tokens (`web/src/styles/tokens.css`, every colour a `light-dark()` pair) and nested rules (`web/src/styles/base.css`) as the application, and the build targets the supported versions (`web/vite.config.ts:21-25`), so the CSS ships as written.
- History: until the baseline was set, `web/vite.config.ts` had no `build.target`, so Vite 8.3.8's default (`chrome111`, `edge111`, `firefox114`, `safari16.4`, `ios16.4`) applied, and Lightning CSS, which minifies the CSS for the build target, rewrote every `light-dark()` pair into two custom properties and flattened the nesting. The built notice was then styled down to those versions; only the development server served the CSS as written, which is what the first version of this entry described.
- Status: open, accepted by the coordinator on 2026-11-20 as a known limitation; restated the same day when the coordinator chose to ship the CSS as written.

### 2026-11-20 · limitation · Safari deletes the device key after seven days without a visit

- Symptom: in Safari on macOS and iOS, when the person does not interact with the SoloDawn page during seven days of Safari use, Safari deletes the origin's IndexedDB, and with it the device key once the identity module keeps it there (F-01b). The browser then starts as a new device and has to be paired again. A web app added to the home screen keeps its own count and is not affected; Lockdown Mode disables IndexedDB altogether.
- Cause: WebKit's tracking prevention deletes all script-writable storage of a site after seven days of browser use without interaction with it ([WebKit blog](https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/)). Not a SoloDawn defect; [decision](docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md), Consequences.
- Mitigation: F-13 shows the iOS add-to-home-screen hint. F-04 asks for persistent storage (`navigator.storage.persist()`) once the device is enrolled, after pairing; Safari grants it by heuristics such as being opened as a home screen web app ([WebKit blog](https://webkit.org/blog/14403/updates-to-storage-policy/)). `loadDevice` does not ask on first run, because Firefox answers the call with a permission prompt (`dom/quota/StorageManager.cpp`, `PersistentStoragePermissionRequest::Start`, asks through `nsContentPermissionUtils::AskPermission`; MDN, [Storage quotas and eviction criteria](https://developer.mozilla.org/en-US/docs/Web/API/Storage_API/Storage_quotas_and_eviction_criteria)).
- Status: open, accepted as a limitation of the platform.

### 2026-11-20 · limitation · Safari's verify() fails every signature over an empty message

- Symptom: WebKit 27.2 (Playwright build, macOS) returns false from `crypto.subtle.verify` for a valid Ed25519 signature over an empty message, RFC 8032 test 1 included; its own signature of the empty message is valid (OpenSSL verifies it). Chromium 159 and Firefox 159 verify both.
- Cause: in WebKit's `Source/WebCore/PAL/pal/crypto/CryptoKitShim.swift`, `isValidSignature` returns false when the data is empty. No bug report found; the first affected Safari version is unknown.
- Effect: none on the protocol; envelopes and challenges have non-empty context prefixes. F-01c asserts WebKit's false result on RFC 8032 test 1 explicitly, separately from the protocol vectors.
- Status: open, external.

### 2026-11-20 · deviation · Web mock does not answer `host.info` or push `groups`

- Symptom: F-02 in `planning/steps/step-1-frontend.md` asks for a mock that answers `hello`, `sync`, `fetch` and `host.info`; the settled plan (PR #58) adds that it pushes `groups`. `mockCtl` answers `hello`, `sync`, `fetch` and `cmd`, and pushes only `reset` and `rows`. `proto.ts` has no `host.info` type; it has a `Groups` type, written from the contract.
- Cause: neither frame has a Go type or a wire example yet, and the mock answers from wire examples only (`web/src/lib/mock.ts:63-75`, `web/src/lib/proto.ts:94-96`, `web/src/lib/proto.ts:372-377`). Both are frozen by unit P-2 (`planning/contracts/conventions.md` section 5; `planning/steps/parallel-plan-units.md` section 1), which also adds the mock answers for `host.info` and `groups`; B-10 named them before the parallel plan.
- Status: open. `host.info` was left out by the coordinator's decision of 2026-11-20, until it has an example; the `groups` push was found in the review of PR #61 and not changed there.

### 2026-11-20 · limitation · Random ids are checked in `n` and `g` of a signed payload only

- Symptom: `contracts/conventions.md` section 2 makes any text that is no random id a wrong type in every field that holds one. `proto.Decode` and `decode` check it for `n` and `g` of a signed payload, which is what the envelope check reads (`contracts/envelope.md` section 3, check 3). A row, a frame or a command field that holds an id (`g` of `sync` or of a row, `thread`, `re`, `task`, `mentions`, a row's own `id`) still takes any text.
- Cause: only those two fields have the type (`internal/proto/envelope.go:12-23`, `web/src/lib/proto.ts:17-22`). The contracts type `n` and `g` as ids in a field table; for the other fields `contracts/rows.md` says "random ids unless stated" without saying per field which values stand beside the id (the `by` of a message row may be `"system"`, a device row's `id` is `<user id>:<ver>`).
- Status: open. A field gets the type (`proto.ID` in Go, `Id` in `proto.ts`) in the task that first reads it from the wire.

### 2026-11-20 · limitation · A message cut short by a closing transport is forwarded as if it were complete

- Symptom: when a client's transport closes in the middle of a WebSocket message, the rendezvous can read the bytes received so far as one complete message. This happens between two fragments of a fragmented message, and also inside one unfragmented frame when the transport closes at the boundary of a read. If those bytes happen to form a valid frame, it is handled like any other: a `to` is forwarded as `from`. `planning/contracts/rendezvous.md` section 2 says that a frame the server cannot read closes its sender.
- Cause: `github.com/coder/websocket` v1.8.15 takes a plain `io.EOF` for the end of the message (`read.go:447-449`, `msgReader.Read`). Between fragments, the header of the next fragment fails with `io.EOF` when the transport has closed (`readFrameHeader`, `frame.go:52-58`). Inside one frame, `Conn.Read` collects the payload with `io.ReadAll`, and when no byte arrives for its next read, `io.ReadFull` returns `io.EOF`, not `io.ErrUnexpectedEOF` (`readFramePayload`, `read.go:282`). `Conn.Read` then returns the bytes so far without an error, to `internal/rendezvous/server.go:179`.
- Why it is harmless: what is forwarded is a complete, valid frame that the same authenticated sender could have sent on purpose; the sender's connection is already gone; and the receiver checks the signed envelope itself (`planning/contracts/envelope.md` section 3).
- Status: open, accepted by the coordinator on 2026-11-20 with no code change; the in-frame case was added after the rendezvous audit of the same day. Revisit if the library changes, or if the server ever acts on a frame beyond forwarding it.

### 2026-11-20 · limitation · A frame dropped for a slow receiver can leave it with stale state

- Symptom: when a receiver's queue of 128 frames is full, the rendezvous drops the next frame for it and closes neither connection (`planning/contracts/rendezvous.md` section 2). A dropped `from` or `room` is covered by its sender's offer timeout and back-off. A frame the server itself generates has no sender to send it again: after a dropped `presence` a watcher shows the wrong online state until that device's next change or the next `watch`; after a dropped `joined` a pairing member waits until it joins again; a dropped `offline`, `ice` or `quota` leaves that request unanswered.
- Cause: `client.sendFrom` drops a frame when the queue is full, holds 16 frames of its sender, or holds 32 from senders off the receiver's watch list (`internal/rendezvous/server.go:129-167`, limits at `:81-82`, queue size at `:92`).
- Status: open. Coalescing the server's own frames is not built, by the coordinator's decision of 2026-11-20.

### 2026-11-20 · limitation · The rendezvous challenge is not bound to a server

- Symptom: a malicious rendezvous, or any node that serves `/rv`, can hand a device that connects to it the nonce of another server's `challenge`, and answer that server with the device's signature. It is then signed in there as the device: the device's own connection there is replaced and closed, and the attacker receives the frames sent to the device, which shows who signals it and when, and can watch other devices under its id. It also gets the device's TURN credentials in `ready` and can spend the device's daily relay quota, cutting off the device's own relayed connections.
- Cause: the answer signs `"solodawn/rv/1\n"` and the nonce only, nothing that names the server (`planning/contracts/rendezvous.md` section 2.1; `internal/identity/key.go:75-82`, checked at `internal/rendezvous/server.go:200-210`).
- Status: open design issue, owner the coordinator, to be designed after the quota reset. The challenge format is unchanged until then.

### 2026-11-20 · limitation · A late `offline` can retire a newer unanswered offer

- Symptom: a node that offers again soon after an offer to an absent device can retire the new offer at once, as if every path had reported the target offline, and wait out a back-off before the next one.
- Cause: the rendezvous answers each `to` it cannot deliver with `{"t":"offline","k":…}`, which names the target but not the `conn` (`planning/contracts/rendezvous.md` section 2.2). `internal/peer/path.go:140-145` passes every such report to `Peer.Offline`, which takes it for each unanswered offer to that target and removes the reporting path (`internal/peer/unanswered.go:38-43`). A report for an earlier frame (the earlier offer or one of its candidates) still on its way when the next offer leaves therefore counts against the new one. The back-off of at least one second makes this rare; a presence report can end the back-off sooner (`planning/flows/connect.md` step 16).
- Status: open, accepted by the coordinator on 2026-11-20 with no contract change.

### 2026-11-20 · deviation · Nothing resumes hosted checks after a node restart yet

- Symptom: after a restart, a task at step `cloud_check` is not polled again, so it stays there until something else moves it (decision D3 of the host-contract audit).
- Cause: `HostedChecksToResume` (`internal/host/forge.go`) selects the tasks to poll again and gives each its PR, commit, branch and `checks.local.at`, where the 60-minute limit counts from, but no code runs at node start yet to call it and then `WaitHostedChecks` for each.
- Status: open. The node must call it at start; owner N-4, or HS-0 if it lands first.

### 2026-11-20 · deviation · No receiver check or shared vector for a beacon yet

- Symptom: `planning/contracts/envelope.md` section 3, checks 4 and 5, now gives a beacon (`planning/contracts/rendezvous.md` section 4) a third set of receiver checks: no `to`, and a two-minute window. The code and the vectors know two kinds only. `OpenSignal` requires `to`, and `OpenCommand` allows 24 hours. So neither reads a beacon as the contract says.
- Cause: the rule was settled after B-02c and F-01d (#136). The identity code has the two kinds only: `internal/identity/envelope.go:30` and `:37`, and `web/src/lib/identity.ts:185-195`. `planning/contracts/vectors.md` section 3 lets `received` be `command` or `signal` only, and `internal/proto/testdata/crypto.json` has no beacon case.
- Status: open. PR-2 adds a beacon case to `received` in `vectors.md` and `crypto.json`, and a third envelope kind in both identity implementations, before it reads beacons.

### 2026-11-20 · deviation · The web client does not handle `full` yet

- Symptom: a browser that joins a pairing room that already has two members is answered `{"t":"full","room":"<room id>"}` (`planning/contracts/rendezvous.md` section 2.2), but the web client drops the frame without telling the page, so a third browser keeps waiting for `joined`.
- Cause: `full` is not among the frames the web rendezvous client reads (`web/src/lib/rendezvous.ts:20`); a frame that does not parse is ignored (`web/src/lib/rendezvous.ts:55-56`). The server sends it from `internal/rendezvous/room.go` (`join`).
- Status: open, owner W-2.

### 2026-11-20 · limitation · `ReviewedMerge.GitHubRepo` is not filled from the group's repository yet

- Symptom: the merge with a remote runs `gh pr merge <pr> --repo <owner>/<repo>` (`planning/contracts/local.md` section 7, row "merge, with a remote"), but no code sets `<owner>/<repo>` from the group's repository; an empty `GitHubRepo` would make the merge fail.
- Cause: `ReviewedMerge.GitHubRepo` (`internal/host/merge.go`) is set by its caller, and no unit calls the merge yet. The claim that `--repo` makes gh skip every local step (worktree removal, local branch deletion) rests on the gh 2.102.0 source (`pkg/cmd/pr/merge/merge.go`: `CanDeleteLocalBranch = !cmd.Flags().Changed("repo")`), not on a run against GitHub or on older gh versions.
- Status: open. The unit that wires the merge (HS-10 or N-4) sets `GitHubRepo` from the group's repository.

### 2026-11-20 · limitation · A8.4 does not run on Windows

- Symptom: on Windows the A8.4 scenario (`features/rendezvous_acme.feature`) cannot start its rendezvous: the binary exits with `lookup rv.localhost: no such host` (CI run 44716573698, `test (windows-latest)`). The scenario is tagged `@not-windows`, which the acceptance runner leaves out on Windows (`features/acceptance_test.go`, `tags`); Linux and macOS run it.
- Cause: the Windows resolver answers no name under `.localhost` (RFC 6761), and the rendezvous looks up its `--domain` for the relay address at startup (`internal/rendezvous/relay.go`, `relay.generator`, `net.LookupIP`). The certificate library refuses `localhost` itself, a name without a dot, so the test cannot use it instead.
- Status: open. A hosts file entry for `rv.localhost` and `turn.rv.localhost` on the Windows runner would let it run; that changes the machine, not the test.

### 2026-11-20 · limitation · A TURN alias links one device's addresses for the life of the process

- Symptom: the alias in a TURN username stays the same for a device as long as the rendezvous process runs, so whoever sees its TURN traffic (the network path to the server, or a relay peer) can tell that allocations from different IP addresses belong to one pseudonym. The alias does not reveal the device id.
- Cause: the alias is an HMAC-SHA256 of the device id under a secret drawn once per process start (`internal/rendezvous/relay.go:79-86`, secret at `:94`), and the daily quota is kept per alias, so it cannot change within a run.
- Status: open, accepted by the coordinator on 2026-11-20 after the rendezvous security review (F8).

### 2026-11-20 · limitation · Limits by address let one NAT crowd its users and a spoofer fill a victim's places

- Symptom: the relay's 128 allocations and the shared TLS port's 256 connections are counted per IPv4 address or IPv6 /64 (`planning/contracts/rendezvous.md` sections 3 and 6). Devices behind one NAT or carrier-grade NAT share one count, so busy neighbours can leave a device without relay. A sender that can forge UDP source addresses can, with TURN credentials of its own (any device key it makes gets them), create allocations from a victim's address without seeing the answers; each holds one of the victim's places for its lifetime, up to an hour, and blind refreshes can keep it.
- Cause: the server can tell sources apart only by their address (`internal/rendezvous/limits.go:41-50`, `source`; `internal/rendezvous/tls.go:192-205`, `admit`), and pion's nonces are not tied to the client's address (pion/turn v5.1.3 `internal/server/util.go:59`, `NonceHash.Generate`), so a nonce fetched from the sender's own address works for forged ones.
- Status: open, accepted by the coordinator on 2026-11-20 after the rendezvous security review (F9).

## Resolved

### 2026-11-20 · bug · The local merge ran the project's Git hooks (review M5)

- Symptom: once a check's `npm install` had let Husky set `core.hooksPath` in the clone's config, a `prepare-commit-msg` hook that adds a trailer made `Apply` return a conflict ("last commit on base does not record …") although the base held the change, and the next round added a second squash commit; a commitlint-style `commit-msg` hook made every local merge a conflict. A configured signing key could make the commit wait for a person.
- Cause: `git merge --squash`, `git commit` and `git reset --merge` in `squashLocal` (`internal/host/merge.go`) ran with the clone's own hooks and signing settings, and the inspection compared the exact message.
- Status: resolved on branch `fix/host-merge-review`: the three commands, and the reset of an interrupted squash after a restart, run with `-c core.hooksPath=<repo>/.git/solodawn-no-hooks`, an empty folder the host makes; the commit with `--no-verify --no-gpg-sign` (`noHooks`, `squashLocal`, `resetLocal`). Pushes and fetches keep the project's hooks (Git LFS needs `pre-push`). `local.md` section 7, row "merge, no remote". Tests `TestLocalMergeRunsNoProjectHooks`, `TestLocalMergeWithoutHooksFolder`.

### 2026-11-20 · bug · `Submit` could reuse a pull request from a fork (review M6)

- Symptom: an open pull request from a fork whose branch has the task branch's name, into the same base, was taken as the task's pull request.
- Cause: in gh 2.102.0 `gh pr list --head` matches the branch name only; `openPR` (`internal/host/forge.go`) took the first entry.
- Status: resolved on branch `fix/host-merge-review`: `--json` asks for `isCrossRepository` and such entries are skipped; `local.md` section 7, row `submit_task`. Test `TestSubmitForgeSkipsForkPullRequests`, with a fake `gh`; not verified against GitHub.

### 2026-11-20 · bug · A merge queue or auto-merge was reported as a conflict (review M7)

- Symptom: when `gh pr merge` exits 0 and the pull request stays `OPEN` (a merge queue or auto-merge merges it later), `Apply` reported `task conflict` "pull request N is OPEN", and so did `Inspect` after a restart.
- Cause: `Apply` and `Inspect` (`internal/host/merge.go`) read the pull request once.
- Status: resolved on branch `fix/host-merge-review`: `awaitPR` reads it every 30 s until it is `MERGED` or `CLOSED`, or 60 minutes after the merge command (after a restart, after `ReviewedMerge.Taken`, the audit row's time, or the restart), then `task conflict` "merge still pending after 60 minutes (merge queue or auto-merge)". `local.md` section 7, rows "merge, with a remote" and "node restarts with a merge". Tests `TestMergeWaitsWhilePending`, `TestPendingMergeEnds`, `TestInspectResumesPendingMerge`, `TestPendingMergeStops`, with a fake `gh`; not verified against GitHub. A failed read during the wait still ends it with the error (outcome unknown), as before.

### 2026-11-20 · bug · `HostedChecksToResume` panicked on a row without `checks.local` (review M8)

- Symptom: a task row at `cloud_check` with no `checks.local` made the selection dereference a nil pointer.
- Cause: `time.UnixMilli(t.Checks.Local.At)` in `HostedChecksToResume` (`internal/host/forge.go`).
- Status: resolved on branch `fix/host-merge-review`: such a task resumes with its 60 minutes spent (`Since` is the zero time), so it ends with the timed-out result unless its first read has finished. Tests `TestHostedChecksToResumePausedAndWithoutLocalCheck`, `TestHostedChecksWithoutLocalCheckTimeOut`.

### 2026-11-20 · bug · Restart inspection without a remote looked at the last commit only (review L6)

- Symptom: after merging task t1 and then t2, `Inspect` of t1 reported a conflict.
- Cause: `inspectLocal` (`internal/host/merge.go`) compared the message of `git log -1` on the base only.
- Status: resolved on branch `fix/host-merge-review`: merged means a commit on the first-parent history of the base whose message is exactly the merge message; `local.md` section 7, row "node restarts with a merge". Test `TestInspectFindsAnEarlierLocalMerge`.

### 2026-11-20 · deviation · Paused tasks were not resumed at `cloud_check` (review L8)

- Symptom: after a restart, a paused task at `cloud_check` was not polled again, though task-machine row 6 takes `task cloud` while paused and `local.md` section 7 says every task at that step.
- Cause: `HostedChecksToResume` (`internal/host/forge.go`) kept `active` tasks only, and its test asserted the exclusion.
- Status: resolved on branch `fix/host-merge-review`: tasks at `cloud_check` that are active or paused resume, the two states row 6 accepts; the test now expects the paused task.

### 2026-11-20 · bug · `DeleteBranch` could drop unreviewed commits (review L9)

- Symptom: a local task branch that had moved past the merged commit was deleted with `git branch -D`, and its later commits with it.
- Cause: `DeleteBranch` (`internal/host/merge.go`) deleted the branch whatever it pointed at.
- Status: resolved on branch `fix/host-merge-review`: the branch is deleted with `git update-ref -d refs/heads/<br> <commit>` only while it is the merged commit; a moved branch is kept and `DeleteBranch` returns `deleted` false. `local.md` section 7, row "task merged". Tests `TestDeleteBranchOnlyAtTheMergedCommit`, `TestDeleteBranchFailures`. Unlike `branch -D`, `update-ref -d` does not refuse a branch that a worktree still has checked out, so the caller must remove the worktree first, as the row says.

### 2026-11-20 · limitation · The own-address rule knows the interface addresses of the start only

- Symptom: the relay sends a datagram to one of the server's own addresses only when it is for one of its relay allocations (`planning/contracts/rendezvous.md` section 3). The server reads its interface addresses once, when it starts; an address that an interface gets later (a new DHCP lease, an added IPv6 address) is not one of its own addresses for this rule until the service restarts, so a client can relay to a service on the server through that address.
- Cause: `ownAddrs` read `net.InterfaceAddrs` once, from `relay.generator` at startup (`internal/rendezvous/relay.go:111-129` before the fix), with the addresses its name resolves to.
- Status: resolved in PR #151: `generator.ownAddr` reads them again once they are a minute old and keeps the previous set if the read fails (`internal/rendezvous/relay.go:133-160`); a new address counts as the server's own within a minute.

### 2026-11-20 · deviation · The plan had host files register their subcommands and channels

- Symptom: `planning/steps/parallel-plan-units.md` had HS-19 register the terminal channel in the table of PR-0 from `host/pty.go`, and every host unit register its subcommands and channels in the tables of N-0 and PR-0 from its own file. `host` may import neither `peer` nor `node` (`planning/architecture.md` 8.2, `imports_test.go`).
- Status: resolved in #135 by the coordinator's decision of 2026-11-20: `node` registers the subcommands of host features in the table of N-0 and gives each `Peer` the handlers of their channels when it creates it (`peer.New`, `peer.Channels`); a handler gets a `peer.Stream`. The entries of HS-19, N-4, PR-0 and PR-1 and the note of lane HOST say so.

### 2026-11-20 · deviation · The probe needs `peer/signal.go`, which PR-1 did not own

- Symptom: PR-1 owned `peer/probe.go` only. PR-0 hands a probe offer to its handler, but a probe also needs an answer without TURN (`create` takes its ICE servers from `paths`), an ordinary offer that closes a pending probe (`ordinaryOffer`), and a probe that does not take the device's slot before its first `hello` (`answer`), all in `internal/peer/signal.go`.
- Status: resolved in #135 by the coordinator's decision of 2026-11-20: PR-1 also owns those three changes in `peer/signal.go`.

### 2026-11-20 · bug · Go acceptance steps acted beyond their own scenario

Found in the audit of the harness; each was reproduced before the fix (lines are those before it).
- A failing step could end the whole suite: later scenarios and their After hooks did not run, godog printed no summary, go test said "subtest may have called FailNow on a parent test". Steps called `Fatal` on the suite's own test from a scenario's subtest (`features/node_update_test.go:98`, `:104`, `features/host_forge_test.go:26`, and `features/host_checks_test.go:36`, fixed on branch `ci/go127-windows-macos`).
- `go test -tags acceptance -count=2 ./features/` failed with `open …/TestFeatures…/017/v0.3.1: no such file or directory`: the cache of release builds was a package variable that outlived the TempDir of the first run (`features/node_update_test.go:89`).
- The fake node's socket stayed listening through every later scenario, and each node-update scenario left an HTTP server listening (five by the end): cleanups were registered with `f.t.Cleanup` (`features/cmd_local_test.go:38`, `:44`, `features/node_update_test.go:54`).
- Under `go test -run` naming the two login-service scenarios, "Register" failed and "Remove" passed without doing anything: HOME was set in a subtest of the suite's test that `-run` skipped (`features/node_service_test.go:30`).
- A scenario that changed a variable other than HOME, or the working directory, changed every later scenario: the guard of PR #121 checked HOME only and did not restore it (`features/acceptance_test.go:67-80`).
- With `commit.gpgsign = true` in the global Git configuration, a host-check scenario failed with "gpg failed to sign the data": git ran with the user's and the system's configuration.
- A step error X was printed as "after scenario hook failed: X, step error: X, X, step error: X": After hooks returned the step's error as their own (`features/identity_test.go:72`, `features/rendezvous_test.go:101`, `features/peer_test.go:61`).
- Status: resolved in PR #142: steps return errors (the comment on `fixtures.t` states the rule); `afterScenario` undoes per scenario; the build cache is `fixtures.stamped`; the login steps set and restore the variables around each operation; after each scenario the suite compares every variable and the working directory with their state at its start, names what changed (never values) and restores it; `TestFeatures` sets `GIT_CONFIG_GLOBAL` to the null device and `GIT_CONFIG_NOSYSTEM`; the hooks return nil.

### 2026-11-20 · bug · The web acceptance file failed on a busy machine although its tests passed

- Symptom: with every core busy, `features/web.spec.ts` failed 3 of 3 runs with "HookTimeoutError: Hook timed out in 10000ms" at `web/features/motion.ts:22`; all 213 tests passed.
- Cause: `browser.close()` took 10.0 s under that load (20 to 60 ms idle), Vitest's default hook limit.
- Status: resolved in PR #142: both browser hooks have 60 s; 3 of 3 loaded runs passed.

### 2026-11-20 · bug · The node could not start on Windows

- Symptom: on the first Windows runs of CI (Windows Server 2025, and 2022 in a trial), every `ListenLocal` failed, so `solodawn node` could not start: 13 tests of `internal/node` and `cmd/solodawn` failed, first with "remove …\sock: The system cannot find the file specified", then with "Access is denied".
- Cause: Windows refuses a dial to a path where no file is, as it refuses one to a stale socket, so `clearStale` removed a file that was not there (`internal/node/socket.go:45-46` before the fix). Then `ownerOnly` opened the socket with `WRITE_DAC` alone, and `SetSecurityInfo` answers "Access is denied" to such a handle on both versions (`internal/node/socket_windows.go:51` before the fix; found with a diagnostic test in CI).
- Also: five tests encoded Unix answers (the error of a dial to a missing socket, the text of exec on a non-executable, address in use, a binary's mode, the `.old` image the Windows install keeps), and the two DACL tests expected `D:P(…)` with the full SID where Windows writes `D:PAI(…)` and the alias `LA` of the runner's built-in Administrator.
- Status: resolved on branch `ci/go127-windows-macos`: a refused dial removes the file only if one is there, the handle also has `READ_CONTROL`, and the tests take the system's own answers.

### 2026-11-20 · deviation · CI gaps found in the audit of the workflows

- Actions were referenced by movable tags (`pnpm/action-setup` is third party); `release.yml` restored pnpm and Go caches into the build of released files; `sudo ./netsim.test` had no time limit; CI never vetted code behind the `acceptance` tag nor compiled the macOS and Windows tests. Resolved in PR #142: pins to the full commit, no cache in the release job, `-test.timeout=10m`, `go vet` with the tag and for every cross-compile target.
- Every job built with Go 1.26.0, eleven patch releases behind 1.26.11 (Backend run 44697108086), because setup-go v5.6.0 reads `go 1.26.0` in `go.mod:3` as an exact version. Resolved on branch `ci/go127-windows-macos` by the coordinator's decision of 2026-11-20: every setup-go takes `go-version: '1.27.x'` with `check-latest: true`, the newest patch of the line development uses (1.27.4); `go.mod` keeps `go 1.26.0` as the minimum.
- No job had a time limit, so a hung step could hold a runner for six hours (a `pnpm install` held a macOS runner for 30 minutes on 2026-11-20); and no Go code ran on Windows or macOS. Resolved on branch `ci/go127-windows-macos`: `timeout-minutes` on every job, and a Backend job on `windows-latest` and `macos-latest` (vet, `go test -race`, the acceptance suite, coverage of the files built only there).
### 2026-11-20 · deviation · Two host check tests skipped themselves on Windows

- Symptom: `TestChecksRequireBothGitDiffReads` and `TestCheckCancellationStopsForegroundPipeline` in `internal/host/checks_test.go` called `t.Skip` on Windows. `t.Skip(` is a listed directive under `[tamper]` in `.project-standard/gates.toml` with no exception, yet both merged with HS-5 (#94).
- Cause: both tests need a POSIX shell (a fake `git` written as a shell script; a pipeline with a FIFO), and the author guarded them with a runtime skip. Neither CI nor the reviews of #94 ran the directive check.
- Status: resolved. Both tests and their FIFO helper moved to `internal/host/checks_unix_test.go` under `//go:build !windows`, so they are not compiled where they cannot run; no listed directive remains in `cmd/`, `internal/` or `features/`.

### 2026-11-20 · bug · Wire types read seven things differently from the contracts

- Symptom: an audit of the merged wire layer found (1) `proto.ts` typed some 50 integer fields (`seq`, `re`, `id`, every time and count outside a message row) as any number, so the browser read `"seq":805.5` where `proto.Decode` refuses it; (2) `decode(Frame, …)` read a `welcome`, `synced` or `fetched` answer that lacks a field as a `cmd` answer `{re, ok}` instead of refusing it; (3) neither language had the optional `full` of an `approval` body (`contracts/messages.md` 2.3, added in PR #71 after the B-01d freeze), so it was dropped; (4) Go's `MessageRow[ApprovalBody, …]` took `"state":null` for a card, and `MessageRow[EmptyBody, StatusOnly]` took a state for a text message, both of which `proto.ts` refuses (`contracts/rows.md` section 5); (5) `testdata/welcome.json` gave the node version as `0.3.1`, where `contracts/local.md` section 9 and the `welcome` example (PR #112) make it the release tag, `v0.3.1`; (6) Go's `Success` took `"ok":false`, which `proto.ts` refuses (`contracts/conventions.md` section 3); (7) the web read a number beyond the largest double in a fraction field (`"window_pct":1e400`) as `Infinity`, which `proto.Decode` refuses.
- Cause: (1) `num` in place of `int` (`web/src/lib/proto.ts`); (2) the `Frame` union tried `CmdResult`, which has no `t` and drops unknown fields, after the typed answers; (3) the structs were frozen before the field existed; (4) `MessageRow` held its state as `*S` for every kind; (5) the example predates PR #112; (6) `Success.OK` was a plain `bool`; (7) `v.number()` accepts `Infinity`.
- Status: resolved in branch `fix/audit-proto`: integers are `int` except the four `usage` fractions (`web/src/lib/proto.ts:10-12`); the `cmd` answer in `Frame` has no `t` (`web/src/lib/proto.ts:562`); `full` is in both types and the approval examples (`internal/proto/messages.go:41`, `web/src/lib/proto.ts:317`); a card's state is a struct and a non-card's is `NoCard`, which is `null` only (`internal/proto/rows.go:98-115`, `internal/proto/messages.go:198-207`); the `welcome` example has `v0.3.1`; `Success.OK` has the type `True`, which takes `true` only (`internal/proto/session.go`); every number in `proto.ts` is finite.

### 2026-11-20 · bug · The language button was not named by its visible text

- Symptom: the top-bar language button shows "EN" (or "中") but was named "Switch language / 切换语言" by its `aria-label`, so a speech-input user saying "click EN" reached nothing (WCAG 2.2 SC 2.5.3), and the half of the name in the other language had no language of its own (SC 3.1.2). Found in the audit of the web shell.
- Cause: `aria-label={t('bar.lang')}` replaced the button's content as its name (`web/src/App.svelte:49` before the fix).
- Status: resolved on branch `fix/audit-web-shell-2`: no `aria-label`; the name is the visible text, marked with its language, then a visually hidden "Switch language / 切换语言" whose halves carry `en` and `zh-CN` (`web/src/App.svelte`, `textIn` and `otherLang` in `web/src/lib/prefs.svelte.ts`). The look is unchanged. Scenario "The language button is named by its visible text, each part in its own language".

### 2026-11-20 · deviation · The browser bar kept the system colour after a theme was chosen

- Symptom: with a light system and the dark theme chosen, the browser's bar (Android Chrome, Safari) stayed white: `<meta name="theme-color">` in `web/index.html` follows the system scheme only.
- Status: resolved on branch `fix/audit-web-shell-2`: an effect in `web/src/lib/prefs.svelte.ts` gives both tags the colour `index.html` gives the theme shown. The manifest colours stay fixed. Unit tests "the colour of the browser bar".

### 2026-11-20 · bug · Under forced colours a choice was told by the weight of its text alone

- Symptom: in a Windows contrast theme (forced colours), the sliding mark of the navigation and the thumb of each segmented control disappear, because the system drops backgrounds and shadows; the current page and the chosen theme and language were shown only in bold.
- Status: resolved on branch `fix/audit-web-shell-2`: `web/src/styles/base.css` section 8 draws them in `Highlight` and `HighlightText` with `forced-color-adjust: none` (otherwise the system's text backplate hides the text) and hides the hover layer over the current entry. Scenario "Under forced colours the chosen options and the current page are marked in system colours".

### 2026-11-20 · deviation · Defaults were stored on the first visit

- Symptom: the first load wrote the default theme and the language guessed from the browser to `localStorage`, so a person who never chose a language kept it after changing the browser's language.
- Cause: one effect wrote `<html>` and storage on every run, the first included (`web/src/lib/prefs.svelte.ts:40-47` before the fix).
- Status: resolved on branch `fix/audit-web-shell-2`: storage is written only when the person changes a setting (`set` in `web/src/lib/prefs.svelte.ts`). Unit test "writes nothing to this browser until the person changes a setting".

### 2026-11-20 · bug · Web device log refused the entry it held when `p` carried a lone surrogate

- Symptom: a browser holding a device-log entry whose `p` has U+FFFD refused with `fork` the same entry arriving with the escape `\ud800` in that place, and the reverse; a Go node, for the same bytes, does not refuse it (`contracts/envelope.md` section 4: the entry held at a version, the same `p`, `k` and `s`, is not refused). Both spellings are one signed text: the signature covers `p` as `TextEncoder` encodes it, a lone surrogate as U+FFFD, and `encoding/json` reads the escape as U+FFFD. A node passes entries on with U+FFFD, so a browser that wrote the escape and later sends its log again after a `welcome` has its own entry refused by a browser that got it through a node. Found in the audit of the identity code.
- Cause: `same` compared `p` as JavaScript strings (`web/src/lib/devlog.ts:111` before the fix).
- Status: resolved in PR #115: `same` compares `p.toWellFormed()` (`web/src/lib/devlog.ts:110-113`). `crypto.json` holds the case in `devlogs` (its entries are JSON text, so that `p` can carry the escape), and both vector runners pass it; `web/src/lib/devlog.test.ts` and `internal/identity/devlog_test.go` (`TestLogTakesTheSameEntryWithALoneSurrogateAgain`) test it too.

### 2026-11-20 · deviation · A device-log entry of more than 2 KiB is refused before the contract says so

- Symptom: `Log.Apply` and `DeviceLog.apply` refuse an entry whose `p` is more than 2,048 bytes as UTF-8. `planning/contracts/envelope.md` on `rebuild` has no such rule yet.
- Cause: the rule was settled by the coordinator (`planning/risks.md` Q-23) and is written in planning pull request #71, which is not merged: `contracts/envelope.md` section 4, rule 2, and the two cases of `contracts/vectors.md`. The code follows that text (`internal/identity/devlog.go:27-29`, `:89`; `web/src/lib/devlog.ts:118-128`), and `crypto.json` holds the cases.
- Status: resolved: PR #71 (4ac3e10f5) merged the rule into `planning/contracts/envelope.md` section 4, rule 2, so the code now follows the contract.

### 2026-11-20 · bug · Language options were spoken in the language of the page

- Symptom: on the settings page of the English interface, the option "简体中文" had no language of its own, so a screen reader spoke it with the English voice (eSpeak in NVDA reads no Chinese); in the Chinese interface "English" was given the Chinese voice. WCAG 2.2 SC 3.1.2 (Language of Parts, AA) asks that each passage's language be programmatically determined. Found in the audit of the web shell.
- Cause: the radio labels inherited `lang` from `<html>`, which follows the interface language (`web/src/routes/Settings.svelte:39-40` before the fix).
- Status: resolved on branch `fix/audit-web-shell`: each option's `<label>` carries the `lang` of the language it names (`web/src/routes/Settings.svelte:41`); the scenario "Each language option is marked with the language it is written in" of `features/web.feature` covers both interfaces.

### 2026-11-20 · deviation · Identity code was behind the contract settled in PR #69

- Symptom: after PR #69 the contract and both implementations differed: the pairing `mac` covered the role and the device id only, so the rendezvous service could change `kind` and `name` of a `pair` message; a `pair` message of any `kind` was accepted; `n` and `g` of a signed payload took any text, an empty one and the escape of a lone surrogate included, which a node and a browser read as different nonces; `crypto.json` had no `pair_messages` and no `kind` and `name` in `pairing`.
- Cause: the rules were settled after B-02b and F-01b merged (`internal/identity/pairing.go:60-64`, `internal/identity/envelope.go:45-51`, `web/src/lib/pairing.ts:46-56`, `web/src/lib/payload.ts:32-38`, all before the change).
- Status: resolved in PR #72 (B-02c with F-01d): the `mac` covers `role`, `k`, `kind` and `name` as `TextEncoder` encodes them (`internal/identity/pairing.go:57-75`, `web/src/lib/pairing.ts:46-72`); a random id is a type that `proto.Decode` and `proto.ts` check (`internal/proto/decode.go:152-157`, `web/src/lib/proto.ts:17-22`); `crypto.json` holds every case `contracts/vectors.md` names and both test suites pass on it.

### 2026-11-20 · deviation · Go identity and the web client answer differently until B-02b

- Symptom: for the same bytes, `internal/identity` and `web/src/lib/identity.ts` can give different answers. The web client follows the contract as settled in PR #58 and the coordinator's rules of 2026-11-20; the Go package follows the contract as it was. Known differences: weak keys as `k`, `dev`, `user`, pair `k` or signature R (web refuses, Go accepts); an empty `to` (web: a value, so `not_for_me`; Go: no `to`); signalling without `to` (web: `not_for_me`; Go accepts); a payload without `t`, `n` or `ts`, or with `null` in a field read (web: `bad_request`; Go: zero values); a key in another case, such as `TO` (web ignores it; Go reads it); an integer written `2.0` (web: 2; Go refuses) and one above 2^53 (web refuses; Go reads it); an `add` without `kind` `node` or `browser` or without a string `name` (web refuses; Go accepts); a `revoke` of the last valid device (web refuses; Go accepts); a long s in a pairing secret (web refuses; Go reads it as S); a pair message whose `k` is not a device id (web ignores it; Go checks only the mac); a signature whose S is not below L (both refuse, the web client before WebCrypto).
- Cause: the rules changed after B-02 merged; the Go package keeps the old behaviour (`internal/identity/key.go:87-91` signature check, `internal/identity/envelope.go:51-62` payload fields, `internal/identity/devlog.go:63-78` and `:93-104` log rules, `internal/identity/pairing.go:36-37` and `:66-68` secret and pair message). The web side: `web/src/lib/payload.ts`, `web/src/lib/identity.ts:126-136`, `:213-219`, `web/src/lib/devlog.ts:98-101`, `web/src/lib/pairing.ts:22-29`.
- Status: resolved in PR #67: `internal/identity` follows the settled contract, `internal/proto/testdata/crypto.json` holds the matching vectors, and `web/src/lib/vectors.test.ts` passes on them. The line references above are those before the change.

### 2026-11-20 · limitation · An empty `to` in a signed payload is read as no `to`

- Symptom: an envelope whose payload carries `"to":""` passes the `not_for_me` check on every receiver, although `contracts/envelope.md` section 3 says a `to` that is present must equal the receiver's device id.
- Cause: `proto.PayloadHeader.To` is a plain string (`internal/proto/envelope.go:17`), so `addressed` cannot tell an empty `to` from a missing one (`internal/identity/envelope.go:56`). Only a device the receiver already listens to can sign such a payload.
- Status: resolved in PR #67, by the rule the contract settled on (`contracts/envelope.md` section 3, check 4): an empty `to` is a value like any other and is nobody's device id, so it is `not_for_me` for signalling and commands alike, and `null` is `bad_request`. The payload is read into a pointer field, which tells an empty `to` from a missing one (`internal/identity/envelope.go:49`, `addressed` at `internal/identity/envelope.go:82`).

### 2026-11-20 · limitation · Go and browser JSON parsers can disagree on a signed payload

- Symptom: not observed. A log entry written by hand by one of the person's own valid devices could be accepted by a node and refused by a browser, or the reverse, which leaves their logs at different versions. Examples: `"ver":2.0` (refused by Go, read as 2 by `JSON.parse`), a key spelled `"VER"` (matched by Go, ignored by a browser). The common fields that `OpenSignal` and `OpenCommand` read disagree the same way: for `{"to":"A","TO":"B"}` Go reads `to` as B and `JSON.parse` as A, and Go also matches Unicode case folds, so `"tſ"` (long s) sets `ts`. Go and `JSON.parse` both keep the last of two equal keys.
- Cause: `entry` reads the payload with `encoding/json` into `proto.DeviceEntry` (`internal/identity/devlog.go:68`), and `addressed` into `proto.PayloadHeader` (`internal/identity/envelope.go:53`); `encoding/json` matches keys without regard to case and refuses a fraction for an integer, and the contract has no canonical JSON. The shared vectors (`internal/proto/testdata/crypto.json`) hold no such payload.
- Status: resolved in PR #67. `internal/identity` reads every payload with `proto.Decode` (`internal/proto/decode.go:40`), as every later task that reads the wire does; it reads as `contracts/conventions.md` section 2 says: names are exact, a field that is always present must be there, `null` is taken only by a field marked "or `null`", and an integer is a number whose nearest double is a whole number of at most 2^53 − 1, so `2.0` is 2. `internal/proto/testdata/crypto.json` holds such payloads in `received` and `devlogs`. What remains is the depth of nesting each parser accepts (`planning/risks.md` K-16).

### 2026-11-20 · limitation · Go verifies signatures of small-order public keys that OpenSSL refuses

- Symptom: for the device id `AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` (the identity point), the signature made of the identity point and a zero scalar verifies over any text in `crypto/ed25519`, so `VerifyChallenge` and `OpenCommand` (with a `known` that admits the id) accept it without any private key. The other small-order points verify such forgeries for part of all texts. Node 24 (OpenSSL) refuses the same signature, so a Go receiver and another Ed25519 implementation can disagree on whether an envelope or log entry verifies.
- Cause: `ed25519.Verify` (`internal/identity/key.go:90`) follows RFC 8032 without refusing small-order or non-canonical public keys, and `contracts/envelope.md` section 2 does not say whether such a key is a device id.
- Status: resolved in PR #67. The fourteen encodings of small-order points are no device id and no first half of a signature (`contracts/envelope.md` sections 2 and 3); `verify` compares the bytes itself before it asks the library (`internal/identity/key.go:91-137`). As `k` such a value is `bad_sig`, as `dev` it makes a log entry refused, and `Secret.Verify` refuses a `pair` message that carries it. `crypto.json` holds a forged signature under each of the fourteen and one by a real key that begins with the identity point. A real key with a small-order point added to it still passes (`planning/risks.md` K-15).

### 2026-11-20 · deviation · Web note rows refuse `pos` until the Go type has it

- Symptom: the settled row contract (PR #58) gives note rows a `pos`; `decode` refuses a note row that carries one.
- Cause: `NoteRow` mirrors the merged Go `NoteRow` and `internal/proto/testdata/row-note.json`, which have no `pos` (`web/src/lib/proto.ts:261-270`).
- Status: resolved in B-01 slice (d). Both wire types require the creation position and the shared note example includes it.

### 2026-11-20 · bug · Web `verify` threw for a `k` or `s` of some hundred thousand characters

- Symptom: `verify`, `isDevice`, `verifyPair` and `DeviceLog.apply` threw `RangeError: Maximum call stack size exceeded` for a `k`, `s`, `dev` or `mac` of 400 000 base64url characters, where the contract wants `bad_sig`, a refused entry or an ignored `pair` message. Under Node 24 a text of 133 000 characters still passed and one of 267 000 threw; the limit differs by engine and was not measured in the browsers. Nothing was accepted that should have been refused. Found in the review of PR #66, before the code was merged.
- Cause: `decode` encodes the bytes it decoded to compare the spelling, and `encode` spread them all into the arguments of one `String.fromCharCode` call (`web/src/lib/identity.ts:18` before the fix).
- Status: resolved in PR #66: `encode` makes the text byte by byte (`web/src/lib/identity.ts:18-19`). Checked under vitest with 400 000 characters.

### 2026-11-20 · limitation · Browsers accept small-order Ed25519 public keys

- Symptom: `crypto.subtle.verify` returns true for the signature R = identity, S = 0 under the identity point as public key, over any message, in Chromium 159, Firefox 159 and WebKit 27.2 (spike S-7, 2026-11-20). WebCrypto requires false for small-order keys; web-platform-tests `eddsa_small_order_points` passes 8, 9 and 9 of 14 in Chrome 158, Firefox 160 and Safari 27.2 (wpt.fyi, 2026-11-19).
- Cause: the engines' Ed25519 libraries do not make the small-order checks ([Firefox bug 1929022](https://bugzilla.mozilla.org/show_bug.cgi?id=1929022), [WebKit bug 280673](https://bugs.webkit.org/show_bug.cgi?id=280673), both open).
- Effect: relying on WebCrypto alone would admit invalid signers. The envelope contract requires rejection before WebCrypto.
- Status: resolved for the web client in F-01b: the settled contract (PR #58, `contracts/envelope.md` section 2) makes the fourteen weak values no device id, and the client compares the bytes itself before WebCrypto is asked, for `k`, `dev`, `user`, a pair message's `k` and the first half of a signature (`web/src/lib/identity.ts:104-136`). The Go side follows in B-02b.

F-01c measurements. The first two results are native `subtle.verify`; the last is SoloDawn's receiver result for **each** of the fourteen weak-key vectors. CI runs four builds on every pull request; the others ran by hand on macOS 26.6.2.

| Browser | Run by | Identity-point vector | Valid empty message (RFC 8032 §7.1 test 1) | Receiver |
| --- | --- | --- | --- | --- |
| Chrome for Testing 137.0.7151.119 | CI, Ubuntu 24.04; by hand, macOS | true | true | `bad_sig` |
| Edge 137.0.3296.93 | CI, Ubuntu 24.04 | true | true | `bad_sig` |
| Firefox 136.0 (Playwright build 1478) | CI, Ubuntu 24.04; by hand, macOS | true | true | `bad_sig` |
| WebKit 27.2 (Playwright build 2370) | CI, macOS 26; by hand, macOS | true | false | `bad_sig` |
| Chromium 156.0.8078.4 (Playwright) | by hand, macOS | true | true | `bad_sig` |
| Firefox 157.0 (Playwright) | by hand, macOS | true | true | `bad_sig` |
| Safari 26.6.2 | by hand, macOS, once, with Vitest's preview provider, which is not in the repository | true | false | `bad_sig` |

No Safari release runs in CI, and Safari 17.5, the baseline, was not run at all. Playwright's WebKit is a current build of Apple's port, so its row shows that the vectors pass on Apple's Ed25519 implementation of today, not on that of Safari 17.5. For Safari 17.5 the baseline still rests on the sources cited in the browser-baseline decision, not on a run.

### 2026-11-20 · limitation · Firefox 136 cannot export the JWK of a seed imported as PKCS #8

- Symptom: on Firefox 136.0, importing an Ed25519 seed succeeds and it signs, but exporting the imported private key as JWK rejects with `OperationError`. The other builds above export the expected public coordinate.
- Effect: the original shared test used that export to construct a key pair. SoloDawn does not export its private device keys.
- Status: handled in F-01c. The suite does not export: it imports the public key of the vector and proves that **each** seed signs for it. No signature bytes are compared.

### 2026-11-20 · limitation · Web client does not start when the browser blocks site storage

- Symptom: expected blank page when the person has blocked site data for the page's origin. Checked under vitest with a `localStorage` that throws (the import of `prefs` rejects); not reproduced in a real browser.
- Cause: `prefs` reads `localStorage` while its module loads (`web/src/lib/prefs.svelte.ts:12-16` before the fix), and both the application and the upgrade notice import it. A browser that refuses storage access throws on `localStorage` (Chrome raises a `SecurityError`), so the import fails before `boot` mounts anything.
- Status: resolved in 815f0547d (PR #61). Storage is read and written behind a guard (`web/src/lib/prefs.svelte.ts:17-35`): a failed read, or a value the client does not know, gives the default; a failed write is ignored. Checked under vitest with a `localStorage` getter that throws a `SecurityError`; still not tried in a real browser.

### 2026-11-20 · bug · Go acceptance suite ran the web feature

- Symptom: `go test -tags acceptance -count=1 ./features/...` failed with `step is undefined` for seven scenarios of `features/web.feature` (for example `the current page is "群聊"`), so the merge gate failed.
- Cause: `features/acceptance_test.go:44` sets `Paths: []string{"."}`, so godog runs every `.feature` in `features/`, including `web.feature`, whose steps are implemented in `web/features/web.spec.ts` (vitest-cucumber) and not in Go; `Strict: true` fails the undefined steps. Each branch passed on its own (the F-00 branch had no `go.mod`, so Go acceptance was skipped; the B-00 and B-01 branches had no `web.feature`); the conflict appeared only after both were merged.
- Status: resolved in PR #59: `web.feature` is tagged `@web` and the godog run uses `Tags: "~@web"`.
