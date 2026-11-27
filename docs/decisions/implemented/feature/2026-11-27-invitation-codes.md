# Agent Note: Reusable invitation codes with bounded lifetime and hosted rooms

Status: implemented

## Problem

One-time invitation links ([people.md](../../../../planning/contracts/step-2/people.md) section 2.1
and section 4) expire in 10 minutes and admit a single browser click. Inviting several colleagues or
adding someone who is not available within 10 minutes forced admins to keep issuing links one by
one. At the same time, a reusable invitation needed to work without adding a group database to
rendezvous servers (`R-130`) or storing device-pairing secrets permanently.

## Decision

The owner decided on 2026-11-27 to add reusable **invitation codes** alongside one-time links
([people.md](../../../../planning/contracts/step-2/people.md) sections 2.6 and 4.1,
[product-rules.md](../../../../planning/product-rules.md) `R-140`):

1. **Form:** An invitation code reuses the pairing-secret format
   ([envelope.md](../../../../planning/contracts/envelope.md) section 6), with at most one open
   code per group at a time.
2. **Lifecycle (`code new` / `show` / `off`):** Admins open, inspect, or end a group's code for a
   limited time, replacing any prior open code and ending it automatically on expiry, group move,
   or removal of another member; exact duration bounds, storage rules, and invalidation edges
   live in [people.md](../../../../planning/contracts/step-2/people.md) (section 2.6).
3. **Hosted rendezvous rooms:** The home node hosts a persistent room for each open code on its
   built-in handler and on connected rendezvous servers that advertise hosting, re-hosting across
   reconnects and restarts; hosted room lifetime and per-connection limits live in
   [rendezvous.md](../../../../planning/contracts/rendezvous.md) (sections 2.1–2.2) and
   [admit.go](../../../../internal/node/admit.go).
4. **Joining flow:** Any number of people holding an open code can join via `#/join` without
   enrolling devices into another person's log, while ending the code closes pending non-member
   joining connections; see [people.md](../../../../planning/contracts/step-2/people.md) (sections
   3 and 4.1) and [admit.go](../../../../internal/node/admit.go) for the joining window and
   admission rules.

Origin: originated and approved by the owner on 2026-11-27; coordinator settled room limits,
lifecycle edges, and UI placement.

## Alternatives considered

- **Extending one-time invitation links to 7 days and multi-use:** links encode the home's device id
  and one-time token in URL hashes, making typed entry on another phone awkward and exposing link
  tokens in browser history rather than short human-readable codes.
- **Replacing one-time links completely:** rejected because a single-click 10-minute link remains
  useful when onboarding one person quickly without keeping a multi-use code open.
- **Keeping an invitation code open when a member is removed:** rejected because a removed person
  who already knows the code could otherwise rejoin immediately.

## Consequences

- Accepted risks K-39 and K-40 ([risks.md](../../../../planning/risks.md)): a leaked code admits
  anyone holding it until it ends, and a holder could pre-occupy the code's hosted rendezvous room
  while connected; rendezvous servers older than this change omit `hosts` and cannot carry hosted
  code rooms.
- Groups must revoke/regenerate codes when removing a member or moving the group to another home.

## Verification

- Implemented across commits `54bb46cde` (wire/contracts), `636421612` (rendezvous hosted rooms),
  `4b2668921` (home & store `invites`), `da927dede` (node room hosting & peer cleanup),
  `d47ac277f` (web `#/join`, `Invite`, and `CreateGroup`), `74564d179` (demo simulation),
  `45803ffbd` (real-process acceptance `A9.47`–`A9.55` & `web/e2e/join.spec.ts`), and `1926ba1db`
  (contract sync).

## Related

- [product-rules.md](../../../../planning/product-rules.md) (`R-140`)
- [people.md](../../../../planning/contracts/step-2/people.md) (sections 2.1, 2.6, 3, and 4.1)
- [rendezvous.md](../../../../planning/contracts/rendezvous.md) (sections 2.1–2.2)
- [admit.go](../../../../internal/node/admit.go)
- [risks.md](../../../../planning/risks.md) (`K-39`, `K-40`)
