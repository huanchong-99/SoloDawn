# Review questions, round 1

To the reviewer (GPT-6.2 Astra). Please review the whole of `planning/` and
answer in `planning/reviews/gpt-6.2-round-1.md`, in the format given in
`README.md`. Open a pull request against `rebuild` from a branch named
`devin/planning-review-1`.

> Written for commit `355da9e`. After it was sent, the coordinator settled
> Q-1 to Q-18 of `risks.md` and the plan was updated. Findings about those
> points are still welcome; read them against the "Settled" table in
> `risks.md`. Questions B4, C3, C4 and F3 below are answered there.

## Ground rules for this review

1. `product-rules.md` and `architecture.md` section 1 are decided. Do not
   propose replacing them. If one of them cannot work, write a finding with
   severity `blocker` and say "needs coordinator".
2. **Do not propose defensive programming.** No extra retries, fallbacks,
   redundant validation, nil checks, wrapper layers, circuit breakers,
   health checks, or "just in case" configuration. Validation belongs at
   system boundaries only; inside, a broken invariant is a crash with a
   clear message. A finding that adds code must name the rule or acceptance
   test that fails without it.
3. Prefer deletion. A finding that removes a mechanism while keeping every
   `[S1]` rule satisfied is worth more than one that adds a mechanism.
4. For every claim about pion, a browser, Claude Code, Codex, Git or `gh`,
   say whether you verified it and how (documentation link, source file,
   or a command you ran with its output). End the file with the section
   `## Unverified` listing everything you did not check. An honest "I do
   not know" is a good answer.
5. Be specific: file, section, the failing sequence of events, the smallest
   change.

## A. Holes in the architecture

- **A1.** Single-writer home with per-row sequence numbers
  (`flows/sync.md`, `contracts/datachannel.md` section 4). Find a sequence
  of events after which a client shows a wrong or stale group: lost update,
  reordering between `rows` pushes and a `synced` response, the reset path,
  two connections of one device during a move.
- **A2.** Trust (`architecture.md` section 4, `contracts/envelope.md`). The
  rules are: every command is a signed envelope; a home takes a command
  only from its signer's own connection; a host acts only on envelopes of
  its owner's devices. Find an action a node would take that none of its
  owner's devices signed. Look especially at inbox items with `why` =
  `assign`, `review`, `resume`, `merge`, at `system` rows, and at
  `member status`.
- **A3.** Device log (`contracts/envelope.md` section 4): versioned
  entries, first accepted wins. Is there something simpler that still
  allows revoking a device and, in step 2, lets another person's home
  verify my devices?
- **A4.** At-most-once delivery from host to session and acknowledgement at
  delivery (`flows/agent-turn.md`). Find a failure after which a task hangs
  with nothing visible in the group. Is there any such case for a late
  approval (`flows/approval.md` D)?
- **A5.** The one-shot grant for late approvals. Is there a simpler way to
  let an AI continue after a decision that arrived when its process was
  gone?
- **A6.** One CLI process per open session for both CLIs. Is the memory cost
  acceptable against the alternative (one Codex app-server for many
  threads)? How would the MCP server know its caller in the alternative?

## B. Hole punching and relay switching

- **B1.** Probe-and-move (`flows/connect.md` B). While relayed, the client
  opens a second connection with no TURN server and, if it opens, moves.
  Does pion or a browser already switch from a relay pair to a direct pair
  on a live connection, making this unnecessary? If not, is a second
  connection the right mechanism, and what does it cost?
- **B2.** During the move, the home may push rows on the old connection
  after `hello` arrived on the new one. State the smallest rule that
  guarantees no row is lost or applied twice.
- **B3.** `contracts/rendezvous.md` section 5 assumes pion offers: one fixed
  UDP port for all connections, a passive ICE-TCP listener, resolution of
  browser `.local` candidates, and a way to advertise a router-mapped
  address. Which of these hold for pion v4 today?
- **B4.** TCP between two NATed nodes (`risks.md` Q-1). Is there a way to
  get it with pion, or a small addition, or should the plan state plainly
  that it is not provided?
- **B5.** Signalling fan-out over every known path with de-duplication by
  `conn` (`contracts/rendezvous.md` 2.3). Any failure mode? Would complete
  (non-trickled) descriptions be simpler and good enough?
- **B6.** Without any rendezvous (`flows/connect.md` D, `risks.md` K-2): is
  there a small addition that lets a remote browser start, given that an
  HTTPS page cannot open `ws://` to another machine?
- **B7.** HTTPS and TURN-over-TLS on one port by server name; the relay
  quota counted inside pion/turn; the credential scheme. Workable as
  described?

## C. Group home and sync

- **C1.** Catch-up of at most 1000 rows, otherwise reset; every thread
  message pushed to every client; hosts sync state rows only. Too much, too
  little, or wrong?
- **C2.** One generic `row` table holding JSON, with state rows mirrored in
  memory (`contracts/storage.md`). Which query or update will hurt first?
- **C3.** Retention without tombstones; notes deleted with messages
  (`risks.md` Q-13).
- **C4.** The home is the only copy of a group (`risks.md` Q-12). Is there a
  nearly free safeguard that does not introduce replication logic?
- **C5.** Behaviour while the home is unreachable (`flows/sync.md` H). Is
  anything missing that a `[S1]` rule requires?

## D. Are the contracts enough to start the backend alone?

- **D1.** List every contradiction or gap between
  `contracts/datachannel.md`, `rows.md`, `commands.md`, `mcp-tools.md`,
  `local.md`, `storage.md` and the flows. Check in particular: task `state`,
  `step` and `wait` transitions; inbox `why` values; error codes; who may
  send each command.
- **D2.** Which commands are missing? (Leaving or deleting a group,
  renaming, changing the secretary, removing a task, anything a `[S1]` rule
  implies.)
- **D3.** For each step-1 acceptance test (now in
  `steps/step-1-acceptance.md`), can it be implemented and automated from
  the contracts as written? Name the ones that cannot.
- **D4.** MCP tools (`contracts/mcp-tools.md`): can any be merged or
  dropped? Is "the model speaks only through `post`" right, or should the
  final message of a turn be posted automatically?
- **D5.** Turn format and the `instruction` / `information` labels
  (`contracts/local.md` section 5). Enough for R-52?
- **D6.** Level mapping (`contracts/local.md` 4.1 and 4.2). What are the
  exact flags and settings in current CLI versions for levels 1, 2 and 3?
  What can level 2 mean for Claude Code on native Windows (`risks.md` Q-4)?

## E. Order of the step 1 backend tasks

- **E1.** Is the order in `steps/step-1-backend.md` right? What should move
  earlier to remove risk sooner?
- **E2.** Which tasks are too large for one pull request, and where would
  you cut them?
- **E3.** Fake CLIs for integration tests: adequate, and what must they
  script to be trustworthy?
- **E4.** Are any spike questions missing from M0?

## F. Simpler alternatives

- **F1.** Go through the table in `architecture.md` section 7. For each
  choice, is the rejected alternative, or a third one, less code while still
  meeting the rules?
- **F2.** Could step 1 skip application-level signatures and rely on
  authenticated channels alone, adding envelopes in step 2? What would be
  lost?
- **F3.** Could the home merge automatically (`risks.md` Q-7)? Could a
  session be per task (`risks.md` Q-8)? Give the trade-off as you see it;
  the coordinator decides.
- **F4.** Name everything in step 1 that could be deleted without breaking a
  `[S1]` rule.

## G. What you would do differently

One short section, at most ten lines: if you were to build step 1 and could
change three things in this plan, which three, and why.

## Unverified

(Your list, as rule 4 asks.)
