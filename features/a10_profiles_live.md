# A10 live profile checks

`TestA10ProfilesLiveFeature` is opt-in (`SOLODAWN_LIVE=1`). It runs the two
`@live @pending` scenarios once for each of `claude`, `codex`, `opencode` and
`qwen` found on PATH. Missing CLIs skip independently. Sign in to each installed
CLI first. These checks can make a small billable model request.

The native picker and selected-model report require independent observations:
the shared driver events do not expose the selected model. This is an
operator-assisted test, not an assertion against its own launch arguments.
Use a second terminal to inspect the native CLI. Do not use the AI's prose
answer as a model report.

Build and run the test binary directly so stdin reaches the observations:

```sh
go test -c -tags acceptance -o profiles-live.test ./features/
cd features
SOLODAWN_LIVE=1 ../profiles-live.test -test.v -test.run '^TestA10ProfilesLiveFeature$' -test.timeout 30m
```

Select one CLI by adding its subtest name, for example
`-test.run '^TestA10ProfilesLiveFeature$/^claude$'`.

For each installed CLI, supply three JSON values when prompted:

1. Its independently inspected picker list, in order. Each entry contains
   `model`, `name`, `efforts` (an array, including `[]` when empty), and `fast`
   (a boolean). Tier is SoloDawn's classification, not a native picker field,
   so it is not part of this comparison. An empty listing is `[]`.
2. A choice object with `model`, `effort`, and `reportedModel`. The latter is
   the resolved model ID expected for that native-picker choice, recorded
   **before** the new session starts. This allows a CLI alias and its resolved
   model ID to differ. Empty effort is allowed. Qwen has no discovery list;
   enter its model ID manually from its native picker.
3. After the driver starts a new session and completes a short turn, inspect
   that session's native model report using the printed session ID and
   workspace. Enter the reported model ID as a JSON string. It must equal
   the independently recorded `reportedModel`.

For Qwen, discovery still expects `[]` under the no-listing contract. If its
native picker exposes a nonempty list, the literal picker-equality scenario
fails; do not substitute `[]` for what was observed. The scenarios remain
pending until their real-CLI behavior has been checked.

Without opt-in, the ordinary command skips without reading stdin or starting
any CLI:

```sh
go test -v -tags acceptance ./features/ -run '^TestA10ProfilesLiveFeature$'
```
