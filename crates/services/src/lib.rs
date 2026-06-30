#![warn(clippy::pedantic)]
#![allow(
    clippy::doc_markdown,
    clippy::module_name_repetitions,
    clippy::must_use_candidate,
    clippy::missing_errors_doc,
    clippy::missing_panics_doc,
    clippy::similar_names,
    clippy::too_many_lines,
    clippy::cast_possible_truncation,
    clippy::cast_possible_wrap,
    clippy::cast_sign_loss,
    clippy::needless_pass_by_value,
    clippy::trivially_copy_pass_by_ref,
    clippy::unused_self,
    clippy::unused_async,
    clippy::struct_excessive_bools,
    clippy::too_many_arguments,
    clippy::single_match_else,
    clippy::manual_let_else,
    clippy::match_same_arms,
    clippy::if_not_else,
    clippy::redundant_else,
    clippy::items_after_statements,
    clippy::option_if_let_else,
    clippy::uninlined_format_args,
    clippy::format_push_string,
    clippy::redundant_pattern_matching,
    clippy::map_unwrap_or,
    clippy::match_wildcard_for_single_variants,
    clippy::if_then_some_else_none,
    clippy::incompatible_msrv,
    clippy::return_self_not_must_use,
    clippy::default_trait_access,
    clippy::if_same_then_else,
    // Align with crates/server/src/lib.rs allow-list — pedantic lints the
    // project already chose not to enforce server-side; mirrored here so both
    // core crates share one consistent lint policy.
    clippy::redundant_closure_for_method_calls,
    clippy::wildcard_imports,
    clippy::struct_field_names,
    clippy::needless_continue,
    // Additional pedantic/style lints surfaced by the orchestrator code.
    clippy::many_single_char_names,
    clippy::doc_lazy_continuation
)]

pub mod services;
pub mod utils;

// Re-export commonly used modules for convenience
pub use services::{git, git_watcher, merge_coordinator, orchestrator, terminal};
