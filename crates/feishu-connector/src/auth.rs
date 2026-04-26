use std::sync::Arc;

use anyhow::Result;
use reqwest::Client;
use tokio::sync::{Mutex, RwLock};

use crate::types::{CachedToken, FeishuConfig, WsEndpointResponse};

pub struct FeishuAuth {
    config: FeishuConfig,
    http_client: Client,
    tenant_token: Arc<RwLock<Option<CachedToken>>>,
    /// Serializes refresh calls to prevent TOCTOU races where multiple tasks
    /// see an expired token and all issue concurrent refresh requests.
    refresh_mutex: Mutex<()>,
}

impl FeishuAuth {
    pub fn new(config: FeishuConfig) -> Self {
        Self {
            config,
            http_client: Client::new(),
            tenant_token: Arc::new(RwLock::new(None)),
            refresh_mutex: Mutex::new(()),
        }
    }

    /// Get tenant access token, using cache if valid (refreshes 5 min before expiry)
    pub async fn get_tenant_token(&self) -> Result<String> {
        // Fast path: check cache under read lock
        {
            let cached = self.tenant_token.read().await;
            if let Some(ref token) = *cached {
                if token.expires_at
                    > std::time::Instant::now() + std::time::Duration::from_secs(300)
                {
                    return Ok(token.token.clone());
                }
            }
        }
        // Serialize refresh to prevent TOCTOU race (G32-001)
        let _guard = self.refresh_mutex.lock().await;
        // Double-check: another task may have refreshed while we waited
        {
            let cached = self.tenant_token.read().await;
            if let Some(ref token) = *cached {
                if token.expires_at
                    > std::time::Instant::now() + std::time::Duration::from_secs(300)
                {
                    return Ok(token.token.clone());
                }
            }
        }
        self.refresh_tenant_token().await
    }

    async fn refresh_tenant_token(&self) -> Result<String> {
        let url = format!(
            "{}/open-apis/auth/v3/tenant_access_token/internal",
            self.config.base_url
        );
        let resp = self
            .http_client
            .post(&url)
            .json(&serde_json::json!({
                "app_id": self.config.app_id,
                "app_secret": self.config.app_secret,
            }))
            .send()
            .await?
            .json::<serde_json::Value>()
            .await?;

        let token = resp["tenant_access_token"]
            .as_str()
            .ok_or_else(|| anyhow::anyhow!("Missing tenant_access_token in response"))?
            .to_string();
        let expire = resp["expire"].as_u64().unwrap_or(7200);

        let cached = CachedToken {
            token: token.clone(),
            expires_at: std::time::Instant::now() + std::time::Duration::from_secs(expire),
        };
        *self.tenant_token.write().await = Some(cached);
        Ok(token)
    }

    /// Acquire WebSocket endpoint URL from Feishu.
    ///
    /// Per the official SDK, this endpoint authenticates via AppID/AppSecret
    /// in the request body (not Bearer token).
    pub async fn acquire_ws_endpoint(&self) -> Result<WsEndpointResponse> {
        let url = format!("{}/callback/ws/endpoint", self.config.base_url);
        let resp = self
            .http_client
            .post(&url)
            .json(&serde_json::json!({
                "AppID": self.config.app_id,
                "AppSecret": self.config.app_secret,
            }))
            .send()
            .await?;

        let status = resp.status();
        let body = resp.text().await?;

        let parsed: WsEndpointResponse = serde_json::from_str(&body).map_err(|e| {
            anyhow::anyhow!(
                "Failed to parse Feishu WS endpoint response (HTTP {}): {} — raw: {}",
                status,
                e,
                body
            )
        })?;

        if parsed.code != 0 {
            anyhow::bail!(
                "Feishu WS endpoint error: code={}, msg={}. \
                 Check: 1) App is published 2) Long connection mode enabled \
                 3) Events subscribed (im.message.receive_v1)",
                parsed.code,
                parsed.msg
            );
        }

        Ok(parsed)
    }
}
