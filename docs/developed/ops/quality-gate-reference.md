# Quality Gate Quick Reference / 质量门速查手册

## Environment Variables / 环境变量

| Variable | Description | 说明 | Default |
|----------|-------------|------|---------|
| `QUALITY_GATE_MODE` | Override quality gate mode | 覆盖质量门模式 | (from YAML) |
| `SONAR_TOKEN` | SonarQube/SonarCloud auth token | SonarQube/SonarCloud 认证令牌 | (none) |
| `SONAR_HOST_URL` | SonarQube server URL | SonarQube 服务器地址 | `http://localhost:9000` |
| `GITCORTEX_FEISHU_ENABLED` | Enable Feishu connector | 启用飞书连接器 | `false` |
| `GITCORTEX_ENCRYPTION_KEY` | AES-256-GCM key (32 bytes) | AES-256-GCM 加密密钥（32 字节） | (required) |

## Quality Gate Modes / 质量门模式

| Mode | EN Description | 中文说明 |
|------|---------------|---------|
| `off` | Disabled, legacy workflow | 关闭，走旧流程 |
| `shadow` | Analyze + log, never block | 分析并记录，不阻断（默认） |
| `warn` | Analyze + notify UI, don't block | 分析并通知 UI，不阻断合并 |
| `enforce` | Analyze + block on failure | 分析并在失败时阻断 |

## Scripts / 脚本

| Script | Purpose | 用途 |
|--------|---------|------|
| `scripts/quality/run-quality-gate.sh` | Run quality engine (Linux/macOS) | 运行质量引擎（Linux/macOS） |
| `scripts/quality/run-quality-gate.ps1` | Run quality engine (Windows) | 运行质量引擎（Windows） |
| `scripts/quality/run-sonar-scanner.sh` | Run SonarCloud scanner (Linux/macOS) | 运行 SonarCloud 扫描器（Linux/macOS） |
| `scripts/quality/run-sonar-scanner.ps1` | Run SonarCloud scanner (Windows) | 运行 SonarCloud 扫描器（Windows） |
| `scripts/quality/setup-sonarqube.sh` | SonarQube setup helper (Linux/macOS) | SonarQube 安装辅助（Linux/macOS） |
| `scripts/quality/setup-sonarqube.ps1` | SonarQube setup helper (Windows) | SonarQube 安装辅助（Windows） |
| `scripts/quality/sync-quality-profile.sh` | Sync Sonar quality profile (Linux/macOS) | 同步 Sonar 质量配置（Linux/macOS） |
| `scripts/quality/sync-quality-profile.ps1` | Sync Sonar quality profile (Windows) | 同步 Sonar 质量配置（Windows） |

## Package.json Commands / 包命令

| Command | Purpose | 用途 |
|---------|---------|------|
| `pnpm run quality` | Full quality gate (repo level) | 完整质量门（仓库级） |
| `pnpm run quality:check` | Dry-run / shadow mode | 试运行 / shadow 模式 |
| `pnpm run quality:sonar` | SonarCloud scanner only | 仅 SonarCloud 扫描 |

## CI Workflows / CI 工作流

| Workflow | File | Purpose | 用途 |
|----------|------|---------|------|
| Basic Checks | `.github/workflows/ci-basic.yml` | cargo check/clippy/test + frontend test/lint/tsc | 基础检查 |
| Quality Gate | `.github/workflows/ci-quality.yml` | SonarCloud + quality engine (shadow) | 质量门（shadow 模式） |
| Docker Build | `.github/workflows/ci-docker.yml` | Docker image build verification | Docker 镜像构建验证 |

## API Endpoints / API 端点

| Endpoint | Method | Purpose | 用途 |
|----------|--------|---------|------|
| `/api/terminals/:id/quality/latest` | GET | Latest quality run for terminal | 终端最新质量运行 |
| `/api/workflows/:id/quality/runs` | GET | All quality runs for workflow | 工作流所有质量运行 |
| `/api/quality/runs/:id` | GET | Quality run detail with issues | 质量运行详情含问题列表 |
| `/api/integrations/feishu/status` | GET | Feishu connector status | 飞书连接器状态 |
| `/api/integrations/feishu/config` | PUT | Update Feishu config | 更新飞书配置 |
| `/api/integrations/feishu/reconnect` | POST | Trigger Feishu reconnect | 触发飞书重连 |

## WebSocket Events / WebSocket 事件

| Event Type | Payload | Description | 说明 |
|------------|---------|-------------|------|
| `quality.gate_result` | `{terminalId, status, summary}` | Quality gate completed | 质量门完成 |
| `terminal.status_changed` | `{terminalId, status}` | Terminal status update | 终端状态变更 |
| `workflow.status_changed` | `{workflowId, status}` | Workflow status update | 工作流状态变更 |

## UI States / UI 状态

| Badge | Color | Meaning | 含义 |
|-------|-------|---------|------|
| Passed | Green | Quality gate passed | 质量门通过 |
| Failed | Red | Quality gate failed | 质量门失败 |
| Warning | Yellow | Issues found but not blocking | 发现问题但不阻断 |
| Skipped | Gray | Quality gate not run (off mode) | 未运行质量门（off 模式） |
| Pending | Blue | Quality analysis in progress | 质量分析进行中 |

## Configuration Files / 配置文件

| File | Purpose | 用途 |
|------|---------|------|
| `quality/quality-gate.yaml` | Gate modes, tiers, conditions, providers | 门模式、层级、条件、提供者 |
| `quality/sonar/sonar-project.properties` | SonarQube project settings | SonarQube 项目设置 |
