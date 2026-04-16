use std::{
    fs, io,
    path::{Path, PathBuf},
};

use serde_json::Value;
use uuid::Uuid;

pub fn fork_session(session_id: &str) -> io::Result<String> {
    let root = sessions_root()?;
    let source = find_session_file(&root, &format!("{session_id}.jsonl"))?;
    let contents = fs::read_to_string(&source)?;
    let ends_with_newline = contents.ends_with('\n');

    let new_session_id = Uuid::new_v4().to_string();
    let replaced = contents
        .lines()
        .enumerate()
        .map(|(idx, line)| {
            if idx == 0 {
                replace_session_id(line, &new_session_id)
            } else {
                line.to_string()
            }
        })
        .collect::<Vec<_>>()
        .join("\n");

    let mut output = replaced;
    if ends_with_newline {
        output.push('\n');
    }

    // E34-02: explicit validation rather than a silent `unwrap_or`. A session
    // file with no parent component would be unusual and indicates a
    // misconfigured sessions root; fall back to `root` but log so the
    // fallback is visible.
    let destination_parent = if let Some(parent) = source.parent() {
        parent.to_path_buf()
    } else {
        tracing::warn!(
            source = %source.display(),
            "droid fork_session: source has no parent directory; falling back to sessions root"
        );
        root.clone()
    };
    let destination = destination_parent.join(format!("{new_session_id}.jsonl"));
    fs::write(&destination, output)?;

    if let Ok(settings_source) = find_session_file(&root, &format!("{session_id}.settings.json")) {
        // E34-02: explicit validation for the settings file parent as well.
        let settings_parent = if let Some(parent) = settings_source.parent() {
            parent.to_path_buf()
        } else {
            tracing::warn!(
                source = %settings_source.display(),
                "droid fork_session: settings file has no parent directory; falling back to sessions root"
            );
            root.clone()
        };
        let settings_destination =
            settings_parent.join(format!("{new_session_id}.settings.json"));
        let _ = fs::copy(settings_source, settings_destination);
    }

    Ok(new_session_id)
}

fn sessions_root() -> io::Result<PathBuf> {
    dirs::home_dir()
        .map(|home| home.join(".factory").join("sessions"))
        .ok_or_else(|| io::Error::other("Unable to determine home directory"))
}

fn replace_session_id(line: &str, new_session_id: &str) -> String {
    // E34-10: validate the expected schema explicitly rather than relying on
    // a single chained `if let`. We require: (1) the line parses as a JSON
    // object, (2) `type` is a string equal to "session_start", and (3) `id`
    // is a string field. Any shape mismatch returns the original line and
    // logs a warning so schema drift is observable in the logs.
    let mut meta: Value = match serde_json::from_str(line) {
        Ok(v) => v,
        Err(_) => return line.to_string(),
    };

    if !meta.is_object() {
        tracing::warn!("droid replace_session_id: session header line is not a JSON object");
        return line.to_string();
    }

    let is_session_start = meta
        .get("type")
        .and_then(|value| value.as_str())
        .is_some_and(|value| value == "session_start");
    if !is_session_start {
        return line.to_string();
    }

    match meta.get_mut("id") {
        Some(Value::String(id)) => {
            *id = new_session_id.to_string();
        }
        Some(other) => {
            tracing::warn!(
                actual = ?other,
                "droid replace_session_id: expected `id` to be a string"
            );
            return line.to_string();
        }
        None => {
            tracing::warn!("droid replace_session_id: `id` field missing from session_start line");
            return line.to_string();
        }
    }

    match serde_json::to_string(&meta) {
        Ok(serialized) => serialized,
        Err(_) => line.to_string(),
    }
}

fn find_session_file(root: &Path, filename: &str) -> io::Result<PathBuf> {
    if root.join(filename).exists() {
        return Ok(root.join(filename));
    }

    for entry in fs::read_dir(root)? {
        let entry = entry?;
        let path = entry.path();
        if path.is_dir() {
            let candidate = path.join(filename);
            if candidate.exists() {
                return Ok(candidate);
            }
        }
    }

    Err(io::Error::new(
        io::ErrorKind::NotFound,
        format!("Unable to locate {filename} in {}", root.display()),
    ))
}
