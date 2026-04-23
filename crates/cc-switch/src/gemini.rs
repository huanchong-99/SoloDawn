//! Gemini CLI 配置管理
//!
//! Gemini CLI 使用 .env 格式的配置文件：
//! - ~/.gemini/.env - 环境变量配置

use std::{collections::HashMap, hash::BuildHasher};

use crate::{
    atomic_write::atomic_write_text,
    config_path::{ensure_parent_dir_exists, get_gemini_env_path},
    error::Result,
};

/// 解析 .env 文件内容
pub fn parse_env_file(content: &str) -> HashMap<String, String> {
    let mut map = HashMap::new();
    for line in content.lines() {
        let line = line.trim();
        // 跳过空行和注释
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        // 解析 KEY=VALUE
        if let Some((key, value)) = line.split_once('=') {
            let key = key.trim().to_string();
            let value = value.trim();
            // 移除引号
            let value = value.trim_matches('"').trim_matches('\'').to_string();
            map.insert(key, value);
        }
    }
    map
}

/// 序列化为 .env 格式
pub fn serialize_env_file<S: BuildHasher>(map: &HashMap<String, String, S>) -> String {
    let mut lines: Vec<String> = map
        .iter()
        .map(|(k, v)| {
            // 如果值包含空格或特殊字符，使用引号
            if v.contains(' ') || v.contains('=') || v.contains('#') {
                format!("{k}=\"{v}\"")
            } else {
                format!("{k}={v}")
            }
        })
        .collect();
    lines.sort(); // 保持顺序一致
    lines.join("\n") + "\n"
}

/// 读取 Gemini 配置
pub async fn read_gemini_config() -> Result<HashMap<String, String>> {
    let path = get_gemini_env_path()?;
    if !path.exists() {
        return Ok(HashMap::new());
    }
    let content = tokio::fs::read_to_string(&path).await?;
    Ok(parse_env_file(&content))
}

/// 写入 Gemini 配置
pub async fn write_gemini_config<S: BuildHasher>(
    config: &HashMap<String, String, S>,
) -> Result<()> {
    let path = get_gemini_env_path()?;
    ensure_parent_dir_exists(&path).await?;
    let content = serialize_env_file(config);
    atomic_write_text(&path, &content).await
}

/// 更新 Gemini 模型配置
pub async fn update_gemini_model(base_url: Option<&str>, api_key: &str, model: &str) -> Result<()> {
    let mut config = read_gemini_config().await?;

    // 设置 API Key
    config.insert("GEMINI_API_KEY".to_string(), api_key.to_string());

    // 设置 Base URL（如果提供）
    if let Some(url) = base_url {
        config.insert("GOOGLE_GEMINI_BASE_URL".to_string(), url.to_string());
    } else {
        config.remove("GOOGLE_GEMINI_BASE_URL");
    }

    // 设置模型
    config.insert("GEMINI_MODEL".to_string(), model.to_string());

    write_gemini_config(&config).await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_env_file() {
        let content = r#"
# Comment
GEMINI_API_KEY=test-key
GEMINI_MODEL="gemini-pro"
EMPTY=
"#;
        let map = parse_env_file(content);
        assert_eq!(map.get("GEMINI_API_KEY"), Some(&"test-key".to_string()));
        assert_eq!(map.get("GEMINI_MODEL"), Some(&"gemini-pro".to_string()));
        assert_eq!(map.get("EMPTY").map(String::as_str), Some(""));
    }

    #[test]
    fn test_serialize_env_file() {
        let mut map = HashMap::new();
        map.insert("KEY1".to_string(), "value1".to_string());
        map.insert("KEY2".to_string(), "value with space".to_string());

        let content = serialize_env_file(&map);
        assert!(content.contains("KEY1=value1"));
        assert!(content.contains("KEY2=\"value with space\""));
    }
}
