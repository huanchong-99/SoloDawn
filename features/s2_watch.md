# I2-1c acceptance runs

| Scenario | Units |
| --- | --- |
| A9.20 and A9.21 | P-4a, P-4b, T2-1a, T2-1b, T2-2a, HS-19, HS-20 and T2-6. I2-2 exercises the browser with U2-3 and U-11. |
| A9.22 | P-4b and T2-7. |
| A9.23 | P-4a, P-4b, T2-1b and T2-7. I2-2 exercises the browser with U2-3. |
| A9.24 and A9.25 | P-4b and T2-8. I2-2 exercises the A9.24 join dialog with U2-4. |
| A9.26 | P-4b, T2-8 and a live forge. |

## A9.21 selected-pair change

The watcher is a pion peer on 127.0.0.1 with the cast's TURN server from
the start. `moveToRelay` (`s2_watch_peer_test.go`) delays its direct
packets by 3 seconds, waits until ICE has measured that, and cuts direct
UDP; ICE renominates the relayed pair on the same PeerConnection. The
delay is needed: pion rates a pair by its last round trip even when it no
longer answers. The packets to the TURN server take 5 ms, as pion rates a
round trip of 0 as none measured, and on Windows a relay round trip on
127.0.0.1 reads 0. The browser and a real network are in the
[real-machine checklist](../docs/cookbook/i11-real-machine-checklist.md),
section 6.

## A9.26 live run

1. Create a disposable repository on a forge other than GitHub. Give it a
   base branch with an initial commit. Sign in to the forge CLI and set up
   Git authentication for the scratch repository. If the CLI needs a
   config directory, pass it through the CLI's own environment variable.
   The repository must not already contain `alpha.go` or `beta.go`.
2. Write a JSON file with `repo`, `base`, `templates` and `inspect`. Use the scratch
   repository's Git remote for `repo` and its base branch for `base`.
   Supply four argument arrays inside `templates` with the keys `push`,
   `open`, `checks` and `merge` as defined in
   [the forge contract](../planning/contracts/step-2/checks-forge.md#3-running-a-template).
   Configure hosted checks and use a merge command that checks the
   reviewed head. `inspect` is a test-only argument array, not a product
   template. Its executable must query the real forge; `{request}` is
   replaced with each task's request number. It must print exactly one
   JSON object such as `{"request":17,"state":"merged","head":"<reviewed SHA>"}`.
   `head` must be the request's merged source head, not the base tip or a
   squash/merge commit. Point `SOLODAWN_S2_LIVE_FORGE` at the JSON file.
3. Run the scenario. It is the only one with `@live` in this feature, and
   no other run includes it:

   ```sh
   go test -tags acceptance -count=1 ./features -run '^TestS2LiveForgeFeature$'
   ```

4. A9.26 reuses M5's forge-touching two-task brief-to-merge steps
   (A5.1/A5.12/A5.13), replacing the bare remote/fake gh with this live
   configuration. It checks pre-confirm inactivity, both pushed branches,
   local/hosted checks, exact cross-review identities and final commits,
   summaries, approval absence, review-to-merge ordering and cleanup.
   Inspection must report two distinct requests merged at their reviewed
   heads. Other M5 scenarios use forge `none` and do not invoke forge
   templates; permission-level and attack/defence checks are excluded.
   Record the CLI version and request URLs. Delete the scratch repository
   when finished. No live run occurs unless opted in.
