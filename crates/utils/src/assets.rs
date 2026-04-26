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

    #[test]
    fn test_asset_dir_env_override() {
        let dir = std::env::temp_dir().join("solodawn-asset-dir-test");
        let _ = std::fs::remove_dir_all(&dir);
        unsafe { std::env::set_var("SOLODAWN_ASSET_DIR", &dir) };
        let result = asset_dir().expect("asset_dir should succeed with env override");
        assert_eq!(result, dir);
        assert!(dir.exists());
        unsafe { std::env::remove_var("SOLODAWN_ASSET_DIR") };
        let _ = std::fs::remove_dir_all(&dir);
    }
}
