//! CLI-specific submit-keystroke policy.
//!
//! When SoloDawn dispatches a terminal's initial instruction it pastes the text
//! into the CLI's TUI composer. Different CLIs need different follow-up to make
//! the composer actually *submit* that text:
//!
//! - **Codex** keeps pasted text in its composer until it receives explicit
//!   Enter keystroke(s); cold-start TUI frames can also swallow the first one or
//!   two, so several spaced Enters are required.
//! - **Claude Code** normally submits on the instruction's own trailing carriage
//!   return, but occasionally leaves the *first* pasted prompt in the composer on
//!   cold start, so one delayed Enter is sent on the initial dispatch as a safety.
//! - **Other CLIs** submit on the instruction payload's own trailing carriage
//!   return; injecting extra synthetic Enters can race the startup TUI and submit
//!   partial/empty input.
//!
//! This policy is the single source of truth shared by both dispatch paths — the
//! orchestrator (agent-planned mode) and the DIY/manual-workflow dispatcher — so
//! a given CLI is driven identically regardless of execution mode. Prior to this
//! module the DIY path hard-coded a single generic Enter, which was enough for
//! Claude Code but left Codex's prompt sitting unsubmitted in the composer (the
//! terminal "initialised, then did nothing").

/// Submit-keystroke delays (ms) for Codex — three spaced Enters.
const CODEX_SUBMIT_SCHEDULE_MS: &[u64] = &[120, 360, 900];

/// Submit-keystroke delay (ms) for Claude Code's initial dispatch — one Enter.
const CLAUDE_INITIAL_SUBMIT_SCHEDULE_MS: &[u64] = &[420];

/// No follow-up submit keystrokes.
const NO_SUBMIT_SCHEDULE_MS: &[u64] = &[];

/// True if this CLI's TUI keeps pasted text in the composer until an explicit
/// Enter keystroke is sent (currently: Codex).
pub fn cli_needs_explicit_submit(cli_type_id: &str) -> bool {
    cli_type_id.to_ascii_lowercase().contains("codex")
}

/// True if this CLI is Claude Code.
pub fn cli_is_claude_code(cli_type_id: &str) -> bool {
    cli_type_id.to_ascii_lowercase().contains("claude-code")
}

/// Submit-keystroke schedule (ms delays between successive Enters) to send after
/// the **initial** instruction dispatch, keyed on the CLI type id.
///
/// See the module docs for the rationale behind each schedule.
pub fn initial_submit_keystroke_schedule_ms(cli_type_id: &str) -> &'static [u64] {
    if cli_needs_explicit_submit(cli_type_id) {
        CODEX_SUBMIT_SCHEDULE_MS
    } else if cli_is_claude_code(cli_type_id) {
        CLAUDE_INITIAL_SUBMIT_SCHEDULE_MS
    } else {
        NO_SUBMIT_SCHEDULE_MS
    }
}

/// Submit-keystroke schedule for a **non-initial** dispatch (e.g. a follow-up
/// instruction or a slash-command prompt typed after the first one). Only CLIs
/// that need explicit submission (Codex) get keystrokes here; Claude Code's
/// cold-start safety Enter applies to the initial dispatch only.
pub fn followup_submit_keystroke_schedule_ms(cli_type_id: &str) -> &'static [u64] {
    if cli_needs_explicit_submit(cli_type_id) {
        CODEX_SUBMIT_SCHEDULE_MS
    } else {
        NO_SUBMIT_SCHEDULE_MS
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn codex_variants_need_explicit_submit() {
        assert!(cli_needs_explicit_submit("codex"));
        assert!(cli_needs_explicit_submit("Codex"));
        assert!(cli_needs_explicit_submit("cli-codex"));
        assert!(cli_needs_explicit_submit("codex-gpt-5"));
        assert!(!cli_needs_explicit_submit("claude-code"));
        assert!(!cli_needs_explicit_submit("droid"));
    }

    #[test]
    fn claude_code_detected_case_insensitively() {
        assert!(cli_is_claude_code("claude-code"));
        assert!(cli_is_claude_code("Claude-Code"));
        assert!(cli_is_claude_code("cli-claude-code-sonnet"));
        assert!(!cli_is_claude_code("codex"));
        assert!(!cli_is_claude_code("claude")); // must be the full "claude-code" marker
    }

    #[test]
    fn initial_schedule_matches_cli() {
        // Codex: three spaced Enters — the key fix for the composer-not-submitting bug.
        assert_eq!(initial_submit_keystroke_schedule_ms("codex"), &[120, 360, 900]);
        assert_eq!(
            initial_submit_keystroke_schedule_ms("cli-codex-123"),
            &[120, 360, 900]
        );
        // Claude Code: one cold-start safety Enter.
        assert_eq!(initial_submit_keystroke_schedule_ms("claude-code"), &[420]);
        // Others: none — the instruction's own trailing CR submits it.
        assert_eq!(
            initial_submit_keystroke_schedule_ms("droid"),
            &[] as &[u64]
        );
        assert_eq!(
            initial_submit_keystroke_schedule_ms("opencode"),
            &[] as &[u64]
        );
    }

    #[test]
    fn followup_schedule_only_for_codex() {
        assert_eq!(
            followup_submit_keystroke_schedule_ms("codex"),
            &[120, 360, 900]
        );
        // Claude Code gets no follow-up Enter (its safety Enter is initial-only).
        assert_eq!(
            followup_submit_keystroke_schedule_ms("claude-code"),
            &[] as &[u64]
        );
        assert_eq!(
            followup_submit_keystroke_schedule_ms("droid"),
            &[] as &[u64]
        );
    }
}
