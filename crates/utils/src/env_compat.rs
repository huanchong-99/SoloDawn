//! Environment variable helpers with backward compatibility for GITCORTEX_ → SOLODAWN_ migration.
//
// NOTE(W2-31-07): Backward compatibility for the deprecated `GITCORTEX_*` env
// var names is intentionally retained for at least two minor release cycles
// from v0.0.153 (i.e. remove no earlier than v0.2.0). A `tracing::warn!` is
// emitted on first fallback use so operators see a deprecation signal in
// logs. When removing: grep for `var_with_compat` / `var_is_set` /
// `var_opt_with_compat` callers, drop the old-name argument, and delete this
// module's compat path.

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
