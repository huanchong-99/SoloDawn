use std::path::PathBuf;

use tokio::fs;

use crate::path::get_solodawn_temp_dir;

/// [G36-008] Uses `get_solodawn_temp_dir()` so the port file location is
/// consistent with the rest of the codebase (respects `SOLODAWN_TEMP_DIR`
/// and the debug/release "solodawn-dev" vs "solodawn" distinction).
pub async fn write_port_file(port: u16) -> std::io::Result<PathBuf> {
    let dir = get_solodawn_temp_dir();
    let path = dir.join("solodawn.port");
    tracing::debug!("Writing port {} to {:?}", port, path);
    fs::create_dir_all(&dir).await?;
    fs::write(&path, port.to_string()).await?;
    Ok(path)
}

pub async fn read_port_file(app_name: &str) -> std::io::Result<u16> {
    let dir = get_solodawn_temp_dir();
    let path = dir.join(format!("{app_name}.port"));
    tracing::debug!("Reading port from {:?}", path);

    let content = fs::read_to_string(&path).await?;
    let port: u16 = content
        .trim()
        .parse()
        .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidData, e))?;

    Ok(port)
}
