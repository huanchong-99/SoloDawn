use directories::ProjectDirs;
use rust_embed::RustEmbed;

const PROJECT_ROOT: &str = env!("CARGO_MANIFEST_DIR");

pub fn asset_dir() -> std::io::Result<std::path::PathBuf> {
    if let Some(d) =
        crate::env_compat::var_opt_with_compat("SOLODAWN_ASSET_DIR", "GITCORTEX_ASSET_DIR")
    {
        let path = std::path::PathBuf::from(d);
        std::fs::create_dir_all(&path)?;
        return Ok(path);
    }

    // Under nextest, every test runs in its own process. Components that open
    // the on-disk database resolve it through `asset_dir()/db.sqlite`; without
    // per-process isolation, parallel test processes share a single `db.sqlite`
    // and race on first-time migration — surfacing as non-deterministic
    // "duplicate column name" / "no such table" migration errors. Give each
    // nextest process a private, freshly-cleaned asset dir.
    //
    // Gated on nextest's `NEXTEST` env var, so production and `cargo test` are
    // unaffected (`cargo test` runs all tests in one process; DB-touching tests
    // there rely on `#[serial]`). An explicit `SOLODAWN_ASSET_DIR` (above) still
    // wins.
    if std::env::var_os("NEXTEST").is_some() {
        static NEXTEST_ASSET_DIR: std::sync::OnceLock<std::path::PathBuf> =
            std::sync::OnceLock::new();
        let dir = NEXTEST_ASSET_DIR.get_or_init(|| {
            let run = std::env::var("NEXTEST_RUN_ID").unwrap_or_else(|_| "local".to_string());
            let d = std::env::temp_dir()
                .join(format!("solodawn-nextest-{run}-{}", std::process::id()));
            // PIDs can be reused as test processes finish within a run; start clean.
            let _ = std::fs::remove_dir_all(&d);
            d
        });
        std::fs::create_dir_all(dir)?;
        return Ok(dir.clone());
    }

    let path = if cfg!(debug_assertions) {
        std::path::PathBuf::from(PROJECT_ROOT).join("../../dev_assets")
    } else {
        let dirs = ProjectDirs::from("ai", "solodawn", "solodawn").ok_or_else(|| {
            std::io::Error::new(
                std::io::ErrorKind::NotFound,
                "OS didn't give us a home directory",
            )
        })?;
        dirs.data_dir().to_path_buf()
    };

    // Ensure the directory exists
    std::fs::create_dir_all(&path)?;

    Ok(path)
    // ✔ macOS → ~/Library/Application Support/MyApp
    // ✔ Linux → ~/.local/share/myapp   (respects XDG_DATA_HOME)
    // ✔ Windows → %APPDATA%\Example\MyApp
}

pub fn config_path() -> std::io::Result<std::path::PathBuf> {
    Ok(asset_dir()?.join("config.json"))
}

pub fn profiles_path() -> std::io::Result<std::path::PathBuf> {
    Ok(asset_dir()?.join("profiles.json"))
}

pub fn credentials_path() -> std::io::Result<std::path::PathBuf> {
    Ok(asset_dir()?.join("credentials.json"))
}

#[derive(RustEmbed)]
#[folder = "../../assets/sounds"]
pub struct SoundAssets;

#[derive(RustEmbed)]
#[folder = "../../assets/scripts"]
pub struct ScriptAssets;

#[cfg(test)]
mod tests {
    use super::*;

    // W2-27-11: This is the only test in the workspace that mutates
    // `SOLODAWN_ASSET_DIR`, so there is no cross-test race on this specific
    // variable. The `unsafe` blocks are required by Rust 2024's `set_var` /
    // `remove_var` signatures; if any additional test in this module ever
    // reads or writes `SOLODAWN_ASSET_DIR`, introduce a module-level `Mutex`
    // guard (see `services::filesystem` tests for the pattern) or switch to
    // `serial_test` to serialize access.
    #[test]
    fn test_asset_dir_env_override() {
        let dir = std::env::temp_dir().join("solodawn-asset-dir-test");
        let _ = std::fs::remove_dir_all(&dir);
        // SAFETY: No other test in this module touches SOLODAWN_ASSET_DIR,
        // and the crate does not spawn background threads during unit tests
        // that would read env vars concurrently.
        unsafe { std::env::set_var("SOLODAWN_ASSET_DIR", &dir) };
        let result = asset_dir().expect("asset_dir should succeed with env override");
        assert_eq!(result, dir);
        assert!(dir.exists());
        // SAFETY: See set_var above — single-writer, no concurrent readers.
        unsafe { std::env::remove_var("SOLODAWN_ASSET_DIR") };
        let _ = std::fs::remove_dir_all(&dir);
    }
}
