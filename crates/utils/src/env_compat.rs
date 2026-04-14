//! Environment variable helpers with backward compatibility for GITCORTEX_ → SOLODAWN_ migration.
//
// W2-31-07: Backward compatibility for the deprecated `GITCORTEX_*` env var
// names is intentionally retained. A `tracing::warn!` is emitted whenever the
// old name is used so operators can migrate incrementally.
// TODO(W2-31-07): Remove `GITCORTEX_*` fallbacks once all deployment configs,
// Docker images, CI pipelines, and user docs have been audited and updated
// to the `SOLODAWN_*` names. Track a deprecation window (e.g. two minor
// releases) before deletion.

use std::env::VarError;

/// Read an environment variable, trying the new SOLODAWN_ name first,
/// then falling back to the deprecated GITCORTEX_ name with a warning.
pub fn var_with_compat(new_name: &str, old_name: &str) -> Result<String, VarError> {
    match std::env::var(new_name) {
        Ok(val) => Ok(val),
        Err(_) => match std::env::var(old_name) {
            Ok(val) => {
                tracing::warn!(
                    new = new_name,
                    old = old_name,
                    "Deprecated env var used; please switch to the new name"
                );
                Ok(val)
            }
            Err(e) => Err(e),
        },
    }
}

/// Check if an environment variable is set (trying new name, then old name).
pub fn var_is_set(new_name: &str, old_name: &str) -> bool {
    std::env::var(new_name).is_ok() || std::env::var(old_name).is_ok()
}

/// Read an optional environment variable with compat fallback.
/// Returns None if neither name is set.
pub fn var_opt_with_compat(new_name: &str, old_name: &str) -> Option<String> {
    var_with_compat(new_name, old_name).ok()
}
