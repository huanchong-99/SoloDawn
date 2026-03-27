//! Environment variable helpers with backward compatibility for GITCORTEX_ → SOLODAWN_ migration.

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
