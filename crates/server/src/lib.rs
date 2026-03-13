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
    clippy::uninlined_format_args,
    clippy::format_push_string,
    clippy::redundant_pattern_matching,
    clippy::map_unwrap_or,
    clippy::match_wildcard_for_single_variants,
    clippy::wildcard_imports,
    clippy::struct_field_names,
    clippy::redundant_closure_for_method_calls,
    clippy::incompatible_msrv,
    clippy::needless_continue
)]

pub mod error;
pub mod feishu_handle;
pub mod mcp;
pub mod middleware;
pub mod routes;

#[cfg(test)]
pub mod tests;

// Re-export Deployment trait for tests
pub use deployment::Deployment;

// #[cfg(feature = "cloud")]
// type DeploymentImpl = gitcortex_cloud::deployment::CloudDeployment;
// #[cfg(not(feature = "cloud"))]
pub type DeploymentImpl = local_deployment::LocalDeployment;
