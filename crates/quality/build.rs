//! Bundle the SoloDawn central `quality-gate.yaml` into the crate at compile
//! time, copying it into `OUT_DIR` so `include_str!(concat!(OUT_DIR, ...))`
//! can pick it up regardless of where this crate is built (in-workspace,
//! out-of-tree, vendored, etc.).
//!
//! Resolution priority for the source policy file:
//!   1. `SOLODAWN_QUALITY_POLICY` env var (absolute or relative path)
//!   2. `../../quality/quality-gate.yaml` (workspace layout: crates/quality → repo)
//!   3. `quality/quality-gate.yaml` (cwd-relative, e.g. when CARGO_MANIFEST_DIR
//!      itself sits inside the workspace root)
//!
//! If none of the above resolve, we still emit a syntactically valid minimal
//! enforce-mode YAML so the crate compiles in any tree. This is a hard
//! contract from primary brain rejection v1 (Fix #2 must not break out-of-tree
//! builds).

use std::{env, fs, path::PathBuf};

const OUTPUT_FILE: &str = "quality-gate.yaml";

const FALLBACK_POLICY: &str = r#"# Auto-generated minimal enforce-mode policy used when the source
# `quality/quality-gate.yaml` cannot be located at build time
# (e.g. crate built out-of-tree). Keeps the engine's empty-scan
# fail-closed contract intact.
mode: enforce

terminal_gate:
  name: "Terminal Gate (Bundled Fallback)"
  conditions:
    - metric: cargo_check_errors
      operator: "GT"
      threshold: "0"
    - metric: clippy_errors
      operator: "GT"
      threshold: "0"
    - metric: tsc_errors
      operator: "GT"
      threshold: "0"
    - metric: rust_test_failures
      operator: "GT"
      threshold: "0"
    - metric: frontend_test_failures
      operator: "GT"
      threshold: "0"
    - metric: frontend_test_deps_missing
      operator: "GT"
      threshold: "0"
    - metric: builtin_rust_critical
      operator: "GT"
      threshold: "0"
    - metric: builtin_frontend_critical
      operator: "GT"
      threshold: "0"
    - metric: secrets_detected
      operator: "GT"
      threshold: "0"

branch_gate:
  name: "Branch Gate (Bundled Fallback)"
  conditions:
    - metric: cargo_check_errors
      operator: "GT"
      threshold: "0"
    - metric: clippy_errors
      operator: "GT"
      threshold: "0"
    - metric: eslint_errors
      operator: "GT"
      threshold: "0"
    - metric: tsc_errors
      operator: "GT"
      threshold: "0"
    - metric: test_failures
      operator: "GT"
      threshold: "0"
    - metric: secrets_detected
      operator: "GT"
      threshold: "0"

repo_gate:
  name: "Repo Gate (Bundled Fallback)"
  conditions:
    - metric: cargo_check_errors
      operator: "GT"
      threshold: "0"
    - metric: clippy_errors
      operator: "GT"
      threshold: "0"
    - metric: eslint_errors
      operator: "GT"
      threshold: "0"
    - metric: tsc_errors
      operator: "GT"
      threshold: "0"
    - metric: test_failures
      operator: "GT"
      threshold: "0"
    - metric: secrets_detected
      operator: "GT"
      threshold: "0"

providers:
  rust: true
  frontend: true
  repo: true
  security: true
  sonar: false
  builtin_rust: true
  builtin_frontend: true
  builtin_common: true
  coverage: true

sonar:
  host_url: ""
  project_key: "solodawn"
"#;

fn main() {
    let manifest_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR"));
    let out_dir = PathBuf::from(env::var("OUT_DIR").expect("OUT_DIR"));

    let mut candidates: Vec<PathBuf> = Vec::new();
    if let Ok(p) = env::var("SOLODAWN_QUALITY_POLICY") {
        candidates.push(PathBuf::from(p));
    }
    // crates/quality → ../../quality/quality-gate.yaml
    candidates.push(manifest_dir.join("../../quality/quality-gate.yaml"));
    // Fallback when CARGO_MANIFEST_DIR == repo root (rare)
    candidates.push(manifest_dir.join("quality/quality-gate.yaml"));

    let resolved = candidates.into_iter().find(|p| p.is_file());

    let body = match resolved {
        Some(path) => {
            println!("cargo:rerun-if-changed={}", path.display());
            println!("cargo:rerun-if-env-changed=SOLODAWN_QUALITY_POLICY");
            fs::read_to_string(&path).unwrap_or_else(|e| {
                println!(
                    "cargo:warning=quality build.rs: failed to read {}: {} — falling back to inline policy",
                    path.display(),
                    e
                );
                FALLBACK_POLICY.to_string()
            })
        }
        None => {
            println!(
                "cargo:warning=quality build.rs: no central quality-gate.yaml found; using inline FALLBACK_POLICY"
            );
            FALLBACK_POLICY.to_string()
        }
    };

    let dest = out_dir.join(OUTPUT_FILE);
    fs::write(&dest, body).unwrap_or_else(|e| {
        panic!(
            "quality build.rs: failed to write bundled policy to {}: {}",
            dest.display(),
            e
        )
    });
}
