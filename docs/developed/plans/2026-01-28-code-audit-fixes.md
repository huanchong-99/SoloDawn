# SoloDawn 代码审计修复实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**目标:** 修复所有 P0（编译阻塞）+ P1（严重安全问题），将代码评分从 D 级（45/100）提升到 B 级（70+）

**架构方法:** 分阶段修复：编译阻塞 → 安全加固 → 功能完善 → 代码质量

**技术栈:** Rust + Axum + SQLx + Tower-Websockets + Tokio + AES-GCM加密 + JWT验证

---

## 计划说明

### TDD 方法
- 每个修复先写失败测试
- 实现最小代码使测试通过
- 重构优化
- 频繁提交（每个可验证点单独 commit）

### 依赖与并行策略
- **Phase 1 可并行执行** - 4个 P0 编译错误互相独立
- **Phase 2 依赖顺序** - API Key加密 → DTO脱敏 → 认证中间件 → 路径限制 → JWT校验
- **Phase 3 依赖 Phase 1** - 功能完善需要项目可编译
- **Phase 4 可与 Phase 3 并行** - 但需在 Phase 2 完成后更新 README

### 回滚策略
- 每个任务完成时必须 `cargo test` 或 `cargo check` 通过
- 若破坏其他功能，立即 `git revert <commit>` 回滚

### 工作区设置要求

⚠️ **重要:** 如果使用 `using-git-worktrees` 创建隔离工作区，**必须复制 `e:\SoloDawn\CLAUDE.md` 内容到新工作区**

```bash
# 如果在 worktree 中工作，需要复制项目指令
git worktree add ../solodawn-fixes -b audit-fixes
cd ../solodawn-fixes
cp ../SoloDawn/CLAUDE.md ./
```

**原因:** `CLAUDE.md` 包含项目的核心指令（AGENTS.md），特别是：
- Codex MCP 协作规范
- 代码安全和防御性要求
- 项目特定的开发和审查流程

新工作区如果没有这个文件，AI 助手将无法遵循项目规范，导致：
- ❌ 无法与 Codex MCP 正确协作
- ❌ 代码审查标准不一致
- ❌ 违反项目安全和开发规范

**验证:** 确保新工作区的 `CLAUDE.md` 包含 AGENTS.md 部分（Codex Tool Invocation Specification）

---

## Phase 1: 编译修复（P0）- 预计 85 分钟

**目标:** 修复所有编译阻塞，使项目可编译运行

### Task 1.1: 修复 Slash Commands 的 deployment.pool 错误

**优先级:** P0
**预计时间:** 25 分钟

**涉及文件:**
- 修改: `crates/server/src/routes/slash_commands.rs:51,106,133,162,171,190,205`
- 测试: `crates/server/tests/slash_commands_pool_test.rs`

**背景:** `DeploymentImpl` 不存在 `pool` 字段，导致 `/workflows/presets/commands` 相关路由编译失败

**Step 1: 验证当前问题**
```bash
cargo check -p server 2>&1 | rg "deployment\.pool"
```
预期输出: `error[E0609]: no field 'pool' on type 'DeploymentImpl'`

**Step 2: 编写失败测试**
```rust
// crates/server/tests/slash_commands_pool_test.rs
use server::DeploymentImpl;

#[tokio::test]
async fn test_list_command_presets_uses_db_pool() {
    let deployment = DeploymentImpl::new().await.unwrap();
    // 这应该能访问到 pool
    let _pool = &deployment.db().pool;
}
```

**Step 3: 运行测试确认失败**
```bash
cargo test -p server test_list_command_presets_uses_db_pool
```
预期: 编译失败，提示 `deployment.pool` 不存在

**Step 4: 实现最小修复**
```rust
// crates/server/src/routes/slash_commands.rs

// 在所有使用 deployment.pool 的地方改为 deployment.db().pool

// 示例 - list_command_presets 函数:
pub async fn list_command_presets(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Vec<SlashCommandPreset>>>, ApiError> {
    let presets = SlashCommandPreset::find_all(&deployment.db().pool)
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to fetch command presets: {}", e).into()))?;
    Ok(Json(ApiResponse::success(presets)))
}

// 类似修改 create_command_preset, update_command_preset, delete_command_preset
// 所有 .pool 引用改为 .db().pool
```

**Step 5: 运行测试确认通过**
```bash
cargo test -p server test_list_command_presets_uses_db_pool
```
预期: PASS

**Step 6: 验证没有破坏其他功能**
```bash
cargo test -p server
```
预期: ALL PASS

**Step 7: 提交**
```bash
git add crates/server/src/routes/slash_commands.rs crates/server/tests/slash_commands_pool_test.rs
git commit -m "fix(server): use deployment.db().pool in slash commands routes"
```

---

### Task 1.2: 修复 CLI 检测导入路径和 Arc 类型

**优先级:** P0
**预计时间:** 20 分钟

**涉及文件:**
- 修改: `crates/server/src/routes/cli_types.rs:12,37,38`
- 测试: `crates/server/tests/cli_types_detect_test.rs`

**背景:** `CliDetector` 导入路径错误，且 `Arc<&DBService>` 与 `Arc<DBService>` 类型不匹配

**Step 1: 验证当前问题**
```bash
cargo check -p server 2>&1 | rg "CliDetector|Arc<&DBService>"
```
预期输出: `cannot find type CliDetector` 或 `expected Arc<DBService>, found Arc<&DBService>`

**Step 2: 编写失败测试**
```rust
// crates/server/tests/cli_types_detect_test.rs
use server::DeploymentImpl;

#[tokio::test]
async fn test_cli_detection_uses_arc_dbservice() {
    let deployment = DeploymentImpl::new().await.unwrap();
    let db = Arc::new(deployment.db().clone());
    // 测试能正确构造 Arc<DBService>
    assert_eq!(Arc::strong_count(&db), 1);
}
```

**Step 3: 运行测试确认失败**
```bash
cargo test -p server test_cli_detection_uses_arc_dbservice
```
预期: 编译失败

**Step 4: 实现最小修复**
```rust
// crates/server/src/routes/cli_types.rs

// 修复导入路径
use services::services::terminal::detector::CliDetector;

async fn detect_cli_types(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<Vec<CliDetectionStatus>>, ApiError> {
    // 修复 Arc 类型
    let db = Arc::new(deployment.db().clone());
    let detector = CliDetector::new(db);
    let results = detector.detect_all().await
        .map_err(|e| ApiError::Internal(format!("Failed to detect CLIs: {}", e)))?;
    Ok(ResponseJson(results))
}
```

**Step 5: 运行测试确认通过**
```bash
cargo test -p server test_cli_detection_uses_arc_dbservice
```
预期: PASS

**Step 6: 验证没有破坏其他功能**
```bash
cargo test -p server
```
预期: ALL PASS

**Step 7: 提交**
```bash
git add crates/server/src/routes/cli_types.rs crates/server/tests/cli_types_detect_test.rs
git commit -m "fix(server): correct CliDetector path and Arc<DBService> usage"
```

---

### Task 1.3: 在 DeploymentImpl 中暴露 ProcessManager

**优先级:** P0
**预计时间:** 30 分钟

**涉及文件:**
- 修改: `crates/deployment/src/lib.rs` (trait 定义)
- 修改: `crates/local-deployment/src/lib.rs` (实现)
- 测试: `crates/server/tests/deployment_process_manager_test.rs`

**背景:** `terminal_ws` 调用了不存在的 `process_manager()` 方法

**Step 1: 验证当前问题**
```bash
cargo check -p server 2>&1 | rg "process_manager"
```
预期输出: `no method named process_manager found`

**Step 2: 编写失败测试**
```rust
// crates/server/tests/deployment_process_manager_test.rs
use deployment::Deployment;
use server::DeploymentImpl;

#[tokio::test]
async fn test_deployment_exposes_process_manager() {
    let deployment = DeploymentImpl::new().await.unwrap();
    let _manager = deployment.process_manager();
    assert!(true); // 如果编译通过则成功
}
```

**Step 3: 运行测试确认失败**
```bash
cargo test -p server test_deployment_exposes_process_manager
```
预期: 编译失败

**Step 4: 实现最小修复**
```rust
// crates/deployment/src/lib.rs
use services::services::terminal::process::ProcessManager;

pub trait Deployment: Clone + Send + Sync + 'static {
    // 现有方法...

    fn process_manager(&self) -> &Arc<ProcessManager>;
}

// crates/local-deployment/src/lib.rs
use services::services::terminal::process::ProcessManager;

#[derive(Clone)]
pub struct LocalDeployment {
    // 现有字段...
    process_manager: Arc<ProcessManager>,
}

impl Deployment for LocalDeployment {
    // 现有方法实现...

    fn process_manager(&self) -> &Arc<ProcessManager> {
        &self.process_manager
    }
}

impl LocalDeployment {
    pub async fn new() -> Result<Self, DeploymentError> {
        // 在构造时初始化 process_manager
        let process_manager = Arc::new(ProcessManager::new());

        Ok(Self {
            // 其他字段...
            process_manager,
        })
    }
}
```

**Step 5: 运行测试确认通过**
```bash
cargo test -p server test_deployment_exposes_process_manager
```
预期: PASS

**Step 6: 验证没有破坏其他功能**
```bash
cargo check --workspace
```
预期: `Finished` 无错误

**Step 7: 提交**
```bash
git add crates/deployment/src/lib.rs crates/local-deployment/src/lib.rs crates/server/tests/deployment_process_manager_test.rs
git commit -m "fix(deployment): expose ProcessManager via Deployment trait"
```

---

### Task 1.4: 验证所有编译错误已修复

**优先级:** P0
**预计时间:** 10 分钟

**涉及文件:**
- 验证: 整个 workspace

**背景:** P0 全部修复后必须确保项目可编译

**Step 1: 验证编译成功**
```bash
cargo check --workspace
```
预期输出: `Finished` 无 error

**Step 2: 运行所有测试**
```bash
cargo test --workspace
```
预期: 至少编译通过，测试可以运行

**Step 3: 如果有错误**
回到具体任务修复

**Step 4: 提交验证**
```bash
git commit -m "chore(build): verify P0 compilation fixes" --allow-empty
```

---

## Phase 2: 安全加固（P1）- 预计 220 分钟

**目标:** 修复所有严重安全漏洞

### Task 2.1: 实现 API Key 加密存储与脱敏响应

**优先级:** P1
**预计时间:** 45 分钟

**涉及文件:**
- 修改: `crates/db/src/models/terminal.rs:58` (添加加密方法)
- 修改: `crates/server/src/routes/workflows.rs:447` (使用加密)
- 修改: `crates/server/src/routes/workflows_dto.rs:75,251` (脱敏)
- 修改: `crates/services/src/services/cc_switch.rs:53` (解密使用)
- 测试: `crates/db/src/models/terminal.rs` (加密测试)
- 测试: `crates/server/tests/security_terminal_api_key_test.rs`

**背景:** 终端 API Key 目前明文存储并在 DTO 中回传，存在泄密风险

**Step 1: 验证当前问题**
```bash
rg -n "custom_api_key" crates/db/src/models/terminal.rs crates/server/src/routes/workflows_dto.rs
```
预期输出: `custom_api_key` 在 DTO 中直接暴露

**Step 2: 编写失败测试**
```rust
// crates/db/src/models/terminal.rs (添加 tests 模块)
#[cfg(test)]
mod encryption_tests {
    use super::*;
    use serial_test::serial;

    #[test]
    #[serial]
    fn test_terminal_api_key_encryption_roundtrip() {
        temp_env::with_var("SOLODAWN_ENCRYPTION_KEY", Some("12345678901234567890123456789012"), || {
            let mut terminal = create_test_terminal();

            terminal.set_custom_api_key("sk-test").unwrap();
            assert!(terminal.custom_api_key.is_some());
            assert_ne!(terminal.custom_api_key.as_ref().unwrap(), "sk-test"); // 应该已加密

            let decrypted = terminal.get_custom_api_key().unwrap().unwrap();
            assert_eq!(decrypted, "sk-test");
        });
    }
}

// crates/server/src/routes/workflows_dto.rs (添加测试)
#[test]
fn test_terminal_dto_does_not_expose_api_key() {
    let terminal = create_terminal_with_api_key("sk-secret");
    let dto = TerminalDto::from_terminal(&terminal);
    assert!(dto.custom_api_key.is_none()); // DTO 不应暴露 API key
}
```

**Step 3: 运行测试确认失败**
```bash
cargo test -p db test_terminal_api_key_encryption_roundtrip
cargo test -p server test_terminal_dto_does_not_expose_api_key
```
预期: 测试失败或编译失败（缺少加密方法）

**Step 4: 实现最小修复**
```rust
// crates/db/src/models/terminal.rs
use aes_gcm::{Aes256Gcm, Nonce, aead::{Aead, AeadCore, KeyInit, OsRng}};
use base64::{Engine as _, engine::general_purpose};

impl Terminal {
    const ENCRYPTION_KEY_ENV: &str = "SOLODAWN_ENCRYPTION_KEY";

    fn get_encryption_key() -> anyhow::Result<[u8; 32]> {
        let key_str = std::env::var(Self::ENCRYPTION_KEY_ENV)
            .map_err(|_| anyhow::anyhow!("Encryption key not found: {}", Self::ENCRYPTION_KEY_ENV))?;
        if key_str.len() != 32 {
            return Err(anyhow::anyhow!("Invalid encryption key length: {}", key_str.len()));
        }
        key_str.as_bytes().try_into().map_err(|_| anyhow::anyhow!("Invalid encryption key format"))
    }

    pub fn set_custom_api_key(&mut self, plaintext: &str) -> anyhow::Result<()> {
        let key = Self::get_encryption_key()?;
        let cipher = Aes256Gcm::new_from_slice(&key)?;
        let nonce = Aes256Gcm::generate_nonce(&mut OsRng);
        let ciphertext = cipher.encrypt(&nonce, plaintext.as_bytes())?;
        let mut combined = nonce.to_vec();
        combined.extend_from_slice(&ciphertext);
        self.custom_api_key = Some(general_purpose::STANDARD.encode(&combined));
        Ok(())
    }

    pub fn get_custom_api_key(&self) -> anyhow::Result<Option<String>> {
        match &self.custom_api_key {
            None => Ok(None),
            Some(encoded) => {
                let key = Self::get_encryption_key()?;
                let combined = general_purpose::STANDARD.decode(encoded)?;
                if combined.len() < 12 {
                    return Err(anyhow::anyhow!("Invalid encrypted data length"));
                }
                let (nonce_bytes, ciphertext) = combined.split_at(12);
                #[allow(deprecated)]
                let nonce = Nonce::from_slice(nonce_bytes);
                let cipher = Aes256Gcm::new_from_slice(&key)?;
                let plaintext = cipher.decrypt(nonce, ciphertext)?;
                Ok(Some(String::from_utf8(plaintext)?))
            }
        }
    }
}

// crates/services/src/services/cc_switch.rs (使用解密)
let config = SwitchConfig {
    base_url: terminal.custom_base_url.clone(),
    api_key: terminal
        .get_custom_api_key()?
        .ok_or_else(|| anyhow::anyhow!("API key not configured for terminal"))?,
    model: model_config.api_model_id.clone().unwrap_or_else(|| model_config.name.clone()),
};

// crates/server/src/routes/workflows.rs (创建时加密)
let mut terminal = Terminal {
    // ...
    custom_api_key: None,
    // ...
};
if let Some(key) = &terminal_req.custom_api_key {
    terminal.set_custom_api_key(key)
        .map_err(|e| ApiError::BadRequest(format!("Failed to encrypt terminal API key: {}", e)))?;
}

// crates/server/src/routes/workflows_dto.rs (DTO 脱敏)
impl TerminalDto {
    pub fn from_terminal(terminal: &db::models::Terminal) -> Self {
        Self {
            // ...
            custom_api_key: None, // 永远不返回 API key
            // ...
        }
    }
}
```

**Step 5: 运行测试确认通过**
```bash
cargo test -p db test_terminal_api_key_encryption_roundtrip
cargo test -p server test_terminal_dto_does_not_expose_api_key
```
预期: PASS

**Step 6: 验证没有破坏其他功能**
```bash
cargo test -p server
```
预期: ALL PASS

**Step 7: 提交**
```bash
git add crates/db/src/models/terminal.rs crates/services/src/services/cc_switch.rs crates/server/src/routes/workflows.rs crates/server/src/routes/workflows_dto.rs
git commit -m "fix(security): encrypt terminal api keys and mask responses"
```

---

### Task 2.2: 移除 --dangerously-skip-permissions 强制参数

**优先级:** P1
**预计时间:** 20 分钟

**涉及文件:**
- 修改: `crates/services/src/services/terminal/launcher.rs:263`
- 测试: `crates/services/src/services/terminal/launcher.rs`

**背景:** 默认绕过 CLI 权限保护，属于高风险配置

**Step 1: 验证当前问题**
```bash
rg -n "dangerously-skip-permissions" crates/services/src/services/terminal/launcher.rs
```
预期输出: `--dangerously-skip-permissions` 出现在 `build_launch_command`

**Step 2: 编写失败测试**
```rust
// crates/services/src/services/terminal/launcher.rs (tests module)
#[tokio::test]
async fn test_build_launch_command_does_not_skip_permissions_by_default() {
    let (launcher, _) = setup_launcher().await;
    let cmd = launcher.build_launch_command("claude-code");
    let args: Vec<String> = cmd.get_args().map(|a| a.to_string_lossy().to_string()).collect();
    assert!(!args.iter().any(|a| a == "--dangerously-skip-permissions"));
}
```

**Step 3: 运行测试确认失败**
```bash
cargo test -p services test_build_launch_command_does_not_skip_permissions_by_default
```
预期: FAILED，因为参数存在

**Step 4: 实现最小修复**
```rust
// crates/services/src/services/terminal/launcher.rs
fn build_launch_command(&self, cli_name: &str) -> tokio::process::Command {
    let mut cmd = match cli_name {
        "claude-code" => tokio::process::Command::new("claude"),
        "gemini-cli" => tokio::process::Command::new("gemini"),
        "codex" => tokio::process::Command::new("codex"),
        "amp" => tokio::process::Command::new("amp"),
        "cursor-agent" => tokio::process::Command::new("cursor"),
        _ => tokio::process::Command::new(cli_name),
    };
    cmd.current_dir(&self.working_dir);
    cmd.kill_on_drop(true);
    cmd
}

// 移除所有 .arg("--dangerously-skip-permissions") 调用
```

**Step 5: 运行测试确认通过**
```bash
cargo test -p services test_build_launch_command_does_not_skip_permissions_by_default
```
预期: PASS

**Step 6: 验证没有破坏其他功能**
```bash
cargo test -p services
```
预期: ALL PASS

**Step 7: 提交**
```bash
git add crates/services/src/services/terminal/launcher.rs
git commit -m "fix(security): stop forcing dangerously-skip-permissions"
```

---

### Task 2.3: 实现文件系统路径白名单/根目录限制

**优先级:** P1
**预计时间:** 40 分钟

**涉及文件:**
- 修改: `crates/services/src/services/filesystem.rs:307`
- 修改: `crates/server/src/routes/filesystem.rs:19,23`
- 测试: `crates/services/src/services/filesystem.rs`
- 测试: `crates/server/tests/filesystem_path_guard_test.rs`

**背景:** 文件系统 API 接受任意路径，存在目录遍历风险

**Step 1: 验证当前问题**
```bash
rg -n "list_directory\\(query.path\\)" crates/server/src/routes/filesystem.rs
```
预期输出: 直接传入用户路径

**Step 2: 编写失败测试**
```rust
// crates/services/src/services/filesystem.rs (tests module)
#[cfg(test)]
mod path_guard_tests {
    use super::*;

    #[test]
    fn test_rejects_path_outside_allowed_root() {
        let service = FilesystemService::new_with_roots(vec![
            std::path::PathBuf::from("/allowed")
        ]);
        let result = service.list_directory(Some("/etc".to_string()));
        assert!(result.is_err());
    }
}
```

**Step 3: 运行测试确认失败**
```bash
cargo test -p services test_rejects_path_outside_allowed_root
```
预期: FAILED，因为当前未限制路径

**Step 4: 实现最小修复**
```rust
// crates/services/src/services/filesystem.rs
#[derive(Clone)]
pub struct FilesystemService {
    allowed_roots: Vec<PathBuf>,
}

impl FilesystemService {
    pub fn new() -> Self {
        Self {
            allowed_roots: vec![Self::get_home_directory()],
        }
    }

    pub fn new_with_roots(allowed_roots: Vec<PathBuf>) -> Self {
        Self { allowed_roots }
    }

    fn resolve_path(&self, path: PathBuf) -> Result<PathBuf, FilesystemError> {
        let canonical = path.canonicalize().map_err(FilesystemError::Io)?;
        let allowed = self.allowed_roots.iter().any(|root| canonical.starts_with(root));
        if !allowed {
            return Err(FilesystemError::PathIsNotDirectory);
        }
        Ok(canonical)
    }

    pub fn list_directory(&self, path: Option<String>) -> Result<DirectoryListResponse, FilesystemError> {
        let raw = path.map_or_else(Self::get_home_directory, PathBuf::from);
        let path = self.resolve_path(raw)?;
        Self::verify_directory(&path)?;
        // 现有逻辑...
        Ok(DirectoryListResponse { entries: directory_entries, current_path: path.to_string_lossy().to_string() })
    }
}

// crates/server/src/routes/filesystem.rs
pub async fn list_directory(
    State(deployment): State<DeploymentImpl>,
    Query(query): Query<ListDirectoryQuery>,
) -> Result<ResponseJson<ApiResponse<DirectoryListResponse>>, ApiError> {
    match deployment.filesystem().list_directory(query.path) {
        Ok(response) => Ok(ResponseJson(ApiResponse::success(response))),
        Err(FilesystemError::PathIsNotDirectory) =>
            Ok(ResponseJson(ApiResponse::error("Path not allowed"))),
        Err(e) => Err(ApiError::Internal(format!("Failed to list directory: {}", e).into())),
    }
}
```

**Step 5: 运行测试确认通过**
```bash
cargo test -p services test_rejects_path_outside_allowed_root
```
预期: PASS

**Step 6: 验证没有破坏其他功能**
```bash
cargo test -p server
```
预期: ALL PASS

**Step 7: 提交**
```bash
git add crates/services/src/services/filesystem.rs crates/server/src/routes/filesystem.rs
git commit -m "fix(security): restrict filesystem paths to allowed roots"
```

---

### Task 2.4: 实现 JWT 签名验证

**优先级:** P1
**预计时间:** 30 分钟

**涉及文件:**
- 修改: `crates/utils/src/jwt.rs:2,32,39`
- 测试: `crates/utils/src/jwt.rs`

**背景:** `insecure_decode` 不验证签名，Token 可伪造

**Step 1: 验证当前问题**
```bash
rg -n "insecure_decode" crates/utils/src/jwt.rs
```
预期输出: `insecure_decode` 被使用

**Step 2: 编写失败测试**
```rust
// crates/utils/src/jwt.rs (tests module)
#[cfg(test)]
mod tests {
    use super::*;
    use jsonwebtoken::{encode, EncodingKey, Header};

    #[derive(serde::Serialize)]
    struct Claims { sub: String, exp: usize }

    #[test]
    fn test_verify_rejects_invalid_signature() {
        let claims = Claims {
            sub: "00000000-0000-0000-0000-000000000000".to_string(),
            exp: 9999999999
        };
        let token = encode(&Header::default(), &claims, &EncodingKey::from_secret(b"secret1")).unwrap();
        let result = verify_and_extract_subject(&token, "wrongsecret");
        assert!(result.is_err());
    }
}
```

**Step 3: 运行测试确认失败**
```bash
cargo test -p utils test_verify_rejects_invalid_signature
```
预期: 编译失败或测试失败（缺少验证函数）

**Step 4: 实现最小修复**
```rust
// crates/utils/src/jwt.rs
use jsonwebtoken::{decode, DecodingKey, Validation};

pub fn verify_and_extract_subject(token: &str, secret: &str) -> Result<Uuid, TokenClaimsError> {
    let mut validation = Validation::default();
    validation.validate_exp = true;
    let data = decode::<SubClaim>(
        token,
        &DecodingKey::from_secret(secret.as_bytes()),
        &validation
    )?;
    let sub = data.claims.sub.ok_or(TokenClaimsError::MissingSubject)?;
    Uuid::parse_str(&sub).map_err(|_| TokenClaimsError::InvalidSubject(sub))
}

pub fn verify_and_extract_expiration(token: &str, secret: &str) -> Result<DateTime<Utc>, TokenClaimsError> {
    let mut validation = Validation::default();
    validation.validate_exp = true;
    let data = decode::<ExpClaim>(
        token,
        &DecodingKey::from_secret(secret.as_bytes()),
        &validation
    )?;
    let exp = data.claims.exp.ok_or(TokenClaimsError::MissingExpiration)?;
    DateTime::from_timestamp(exp, 0)
        .ok_or(TokenClaimsError::InvalidExpiration(exp))
}
```

**Step 5: 运行测试确认通过**
```bash
cargo test -p utils test_verify_rejects_invalid_signature
```
预期: PASS

**Step 6: 验证没有破坏其他功能**
```bash
cargo test -p utils
```
预期: ALL PASS

**Step 7: 提交**
```bash
git add crates/utils/src/jwt.rs
git commit -m "fix(security): verify jwt signatures before extracting claims"
```

---

### Task 2.5: 设计并实现基础认证中间件

**优先级:** P1
**预计时间:** 45 分钟

**涉及文件:**
- 新增: `crates/server/src/middleware/auth.rs`
- 修改: `crates/server/src/middleware/mod.rs`
- 修改: `crates/server/src/routes/mod.rs:21`
- 修改: `crates/server/src/main.rs:112`
- 测试: `crates/server/tests/auth_middleware_test.rs`

**背景:** 无认证/授权中间件，远程可触发文件系统扫描与终端执行

**Step 1: 验证当前问题**
```bash
rg -n "middleware" crates/server/src/routes/mod.rs
```
预期输出: 未使用认证中间件

**Step 2: 编写失败测试**
```rust
// crates/server/tests/auth_middleware_test.rs
use axum::{Router, routing::get};
use tower::ServiceExt;

#[tokio::test]
async fn test_requires_auth_header() {
    std::env::set_var("SOLODAWN_API_TOKEN", "test-token");
    let app = server::routes::router(server::DeploymentImpl::new().await.unwrap());
    let response = app.oneshot(
        http::Request::builder()
            .uri("/api/health")
            .body(axum::body::Body::empty())
            .unwrap()
    ).await.unwrap();
    assert_eq!(response.status(), http::StatusCode::UNAUTHORIZED);
}
```

**Step 3: 运行测试确认失败**
```bash
cargo test -p server test_requires_auth_header
```
预期: FAILED（当前返回 200）

**Step 4: 实现最小修复**
```rust
// crates/server/src/middleware/auth.rs
use axum::{http::StatusCode, response::Response, middleware::Next, extract::Request};

pub async fn require_api_token(req: Request, next: Next) -> Result<Response, StatusCode> {
    let token = std::env::var("SOLODAWN_API_TOKEN").ok();
    if token.is_none() {
        // 如果未配置 token，则允许访问（开发模式）
        return Ok(next.run(req).await);
    }
    let auth = req.headers().get("authorization")
        .and_then(|v| v.to_str().ok());
    if auth == Some(&format!("Bearer {}", token.unwrap())) {
        Ok(next.run(req).await)
    } else {
        Err(StatusCode::UNAUTHORIZED)
    }
}

// crates/server/src/middleware/mod.rs
pub mod auth;
pub use auth::*;

// crates/server/src/routes/mod.rs
use crate::middleware::require_api_token;

pub fn router(deployment: DeploymentImpl) -> Router {
    let base_routes = Router::new()
        // ... 现有路由 ...
        .layer(axum::middleware::from_fn(require_api_token))
        .with_state(deployment);
    // ...
}
```

**Step 5: 运行测试确认通过**
```bash
cargo test -p server test_requires_auth_header
```
预期: PASS

**Step 6: 验证没有破坏其他功能**
```bash
cargo test -p server
```
预期: ALL PASS

**Step 7: 提交**
```bash
git add crates/server/src/middleware/auth.rs crates/server/src/middleware/mod.rs crates/server/src/routes/mod.rs crates/server/tests/auth_middleware_test.rs
git commit -m "fix(security): add API token auth middleware"
```

---

## Phase 3: 功能完善（P2）- 预计 245 分钟

**目标:** 修复重要功能缺陷

### Task 3.1: 实现 ProcessManager handle 存储

**优先级:** P2
**预计时间:** 35 分钟

**涉及文件:**
- 修改: `crates/services/src/services/terminal/process.rs:392`
- 测试: `crates/services/src/services/terminal/process.rs`

**背景:** `get_handle` 永远返回 `None`，终端 WS 无法读取输出

**Step 1-7:** （类似前面任务的 TDD 流程）

**核心修复:**
```rust
struct TrackedProcess {
    child: Child,
    handle: ProcessHandle,
}

pub struct ProcessManager {
    processes: Arc<RwLock<HashMap<String, TrackedProcess>>>,
}

pub async fn get_handle(&self, terminal_id: &str) -> Option<ProcessHandle> {
    let mut processes = self.processes.write().await;
    processes.remove(terminal_id).map(|p| p.handle)
}
```

---

### Task 3.2: 实现终端 PTY 真实读写

**优先级:** P2
**预计时间:** 60 分钟

**涉及文件:**
- 修改: `crates/server/src/routes/terminal_ws.rs:136,226,307`
- 修改: `crates/services/src/services/terminal/process.rs`
- 测试: `crates/services/tests/terminal_pty_test.rs`

**背景:** 当前 WS 仅占位模拟输出，无法真实交互

**核心修复:** 使用 `portable-pty` crate 实现真实 PTY

---

### Task 3.3: 移除或修复失败测试

**优先级:** P2
**预计时间:** 20 分钟

**涉及文件:**
- 修改: `crates/services/tests/terminal_timeout_test.rs:8,16`

**背景:** `assert!(false)` 让 CI 永远失败

**核心修复:** 实现真实的超时清理测试逻辑

---

### Task 3.4: 优化 list_workflows N+1 查询

**优先级:** P2
**预计时间:** 40 分钟

**涉及文件:**
- 修改: `crates/server/src/routes/workflows.rs:273,275,281`
- 修改: `crates/db/src/models/workflow.rs`
- 测试: `crates/server/tests/workflows_list_counts_test.rs`

**背景:** 逐个查询 tasks/terminals 导致 N+1

**核心修复:** 使用 SQL 聚合查询：
```sql
SELECT w.*,
       COUNT(DISTINCT wt.id) AS tasks_count,
       COUNT(t.id) AS terminals_count
FROM workflow w
LEFT JOIN workflow_task wt ON wt.workflow_id = w.id
LEFT JOIN terminal t ON t.workflow_task_id = wt.id
WHERE w.project_id = ?
GROUP BY w.id
```

---

### Task 3.5: 改造文件系统扫描为异步

**优先级:** P2
**预计时间:** 30 分钟

**涉及文件:**
- 修改: `crates/services/src/services/filesystem.rs:131,136,307`
- 修改: `crates/server/src/routes/filesystem.rs:19`

**背景:** 同步 IO 在 async 线程中阻塞

**核心修复:** 使用 `tokio::task::spawn_blocking`

---

### Task 3.6: 实现 Orchestrator 状态回滚

**优先级:** P2
**预计时间:** 35 分钟

**涉及文件:**
- 修改: `crates/services/src/services/orchestrator/runtime.rs:142,147`
- 测试: `crates/services/src/services/orchestrator/runtime_test.rs`

**背景:** 在 agent 创建前就标记 running，失败时无法回滚

**核心修复:** 先创建 agent，成功后再更新状态为 running

---

## Phase 4: 代码质量（P3）- 预计 125 分钟

**目标:** 提升可维护性和代码质量

### Task 4.1: 实现日志持久化

**优先级:** P3
**预计时间:** 40 分钟

**涉及文件:**
- 修改: `crates/services/src/services/terminal/process.rs:232,276`
- 测试: `crates/services/tests/terminal_logging_test.rs`

**背景:** 日志缓冲满直接丢弃，无法落盘

---

### Task 4.2: 移除硬编码会话创建

**优先级:** P3
**预计时间:** 20 分钟

**涉及文件:**
- 修改: `crates/services/src/services/terminal/launcher.rs:153`

**背景:** 固定使用 `claude-code`，与实际 CLI 类型不一致

---

### Task 4.3: 改造 asset_dir() 使用 Result 返回

**优先级:** P3
**预计时间:** 30 分钟

**涉及文件:**
- 修改: `crates/utils/src/assets.rs:9,18`
- 修改: `crates/db/src/lib.rs:22`
- 修改: `crates/server/src/main.rs:40`

**背景:** `expect` 直接 panic，异常场景不可恢复

---

### Task 4.4: 统一终端状态语义

**优先级:** P3
**预计时间:** 20 分钟

**涉及文件:**
- 修改: `crates/db/src/models/terminal.rs:373`
- 修改: `crates/services/tests/terminal_lifecycle_test.rs:221`

**背景:** 代码设置 `waiting`，测试期望 `running`

---

### Task 4.5: 更新 README 为真实状态

**优先级:** P3
**预计时间:** 15 分钟

**涉及文件:**
- 修改: `README.md:249,477`
- 测试: `crates/server/tests/readme_status_test.rs`

**背景:** README 仍宣称 "代码审计 100/100"

**核心修复:**
```markdown
- 代码审计评分: 45/100（D 级）— 当前不通过
- 已知问题: P0 编译阻塞、P1 安全缺陷、P2 关键功能缺失
- 修复进度: Phase 1 完成，Phase 2 进行中
```

---

## 验收标准

### Phase 1 完成标准
- ✅ `cargo check --workspace` 无错误
- ✅ `cargo test --workspace` 可以运行（允许部分测试失败）

### Phase 2 完成标准
- ✅ 所有 P1 安全测试通过
- ✅ API Key 加密存储
- ✅ 文件系统路径受限
- ✅ 认证中间件生效
- ✅ JWT 签名验证

### Phase 3 完成标准
- ✅ 终端 PTY 真实读写
- ✅ 无失败测试
- ✅ N+1 查询优化
- ✅ 文件系统异步扫描

### Phase 4 完成标准
- ✅ 日志持久化
- ✅ 无 expect panic
- ✅ 状态语义统一
- ✅ README 状态真实

---

## 预期结果

完成所有 4 个 Phase 后：
- **评分提升:** 45/100 (D) → 75+/100 (B)
- **编译状态:** ❌ 23 个错误 → ✅ 0 个错误
- **安全状态:** ❌ 5 个 P1 漏洞 → ✅ 全部修复
- **功能状态:** ❌ 终端 WS 不可用 → ✅ 完整 PTY 支持

---

## 附录：快速参考

### 依赖管理图
```
Phase 1 (P0) → 可并行
    ↓
Phase 2 (P1) → 2.1 → 2.2 → 2.5 → 2.3 → 2.4
    ↓
Phase 3 (P2) → 依赖 Phase 1
    ↓
Phase 4 (P3) → 可与 Phase 3 并行（除 Task 4.5）
```

### 关键环境变量
```bash
# API Key 加密（32 字节）
export SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"

# API 认证（可选）
export SOLODAWN_API_TOKEN="your-secret-token"

# JWT 密钥
export SOLODAWN_JWT_SECRET="your-jwt-secret"
```

### 测试命令速查
```bash
# 编译检查
cargo check --workspace

# 运行所有测试
cargo test --workspace

# 运行特定测试
cargo test -p server test_name

# 安全审计
bash scripts/audit-security.sh
```

---

**重要提示:**
1. 这是 TDD 驱动的修复计划
2. 每个任务都是独立的、可验证的
3. 考虑 Rust 所有权、借用检查器特性
4. 频繁提交，便于回滚
5. 遇到阻塞问题立即回滚该 commit
