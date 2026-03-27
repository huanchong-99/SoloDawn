# Phase 11: 单项目结构迁移（一次性迁移计划）

> **状态:** ✅ 已完成
> **完成时间:** 2026-01-21
> **前置条件:** 设计/实施文档已完整审计
> **原则:** 一次性迁移，迁移完成后再修复
> **范围:** 仅保留运行/构建/测试必需内容；不保留上游文档类文件

> **📋 代码审查报告 (2026-01-21):**
> - **综合评分:** 7.4/10 - 计划基本合理，但风险被低估
> - **关键发现:** `crates/remote` 被共享任务（Share）功能依赖，需同步删除
> - **修正建议:** 采用 Git worktree 隔离 + 分阶段验证策略
> - **依赖清单:** 已识别 6 个文件使用 `use remote::`，全部为上游功能

---

## 目标

- 将 `vibe-kanban-main` 与 `cc-switch-main` 融合为单一项目结构。
- 删除原项目中在本项目内不使用的内容。
- 保留并迁入已被 SoloDawn 集成的必要模块（尤其是 cc-switch 相关）。
- 迁移完成后再进入修复与验证阶段。

---

## 迁移范围（冻结清单）

### 保留（从 `vibe-kanban-main` 迁入）

- 代码与运行核心：`crates/`, `frontend/`, `shared/`, `assets/`, `scripts/`, `tests/`
- 构建与工具链：`.cargo/`, `Cargo.toml`, `Cargo.lock`, `pnpm-workspace.yaml`,
  `package.json`, `pnpm-lock.yaml`, `.npmrc`, `rust-toolchain.toml`,
  `rustfmt.toml`, `clippy.toml`

### 删除（不迁入）

- 上游文档类文件：`vibe-kanban-main/README.md`, `vibe-kanban-main/LICENSE`,
  `vibe-kanban-main/.gitignore`, `vibe-kanban-main/AGENTS.md`, `vibe-kanban-main/docs`,
  `vibe-kanban-main/CODE-OF-CONDUCT.md`
- 远程部署/发布相关：`vibe-kanban-main/remote-frontend`, `vibe-kanban-main/crates/remote`,
  `vibe-kanban-main/npx-cli`
- **共享任务功能（上游远程部署依赖）:**
  - `crates/services/src/services/share/` (整个目录)
  - `crates/services/src/services/remote_client.rs`
  - `crates/server/src/routes/shared_tasks.rs`
  - `crates/server/src/bin/generate_types.rs` 中的 remote 类型引用
  - `crates/server/src/main.rs` 中的 `share_publisher` 调用
- 非 MVP 必需：`vibe-kanban-main/dev_assets_seed`, `vibe-kanban-main/Dockerfile`,
  `vibe-kanban-main/local-build.sh`, `vibe-kanban-main/package-lock.json`,
  `vibe-kanban-main/test_security_fix.sh`

### cc-switch-main 补齐（按需迁入）

如 `crates/cc-switch` 缺失以下能力，则从 `cc-switch-main/src-tauri/src` 迁入并整合：

- `provider.rs`（Provider 数据模型）
- `config.rs`（Claude 配置读写）
- `codex_config.rs`（Codex 配置读写）
- `gemini_config.rs`（Gemini 配置读写）
- `services/provider/*`（Provider 服务与 live 写入）

---

## 迁移前检查清单（必须在 Task 11.1 开始前完成）

### 检查项 1: Git worktree 环境准备

```bash
# 创建隔离的迁移工作树
git worktree add ../solodawn-migration -b single-project-migration
cd ../solodawn-migration/vibe-kanban-main

# 验证工作树创建成功
git status
```

**验收标准:** 成功创建 worktree，且不影响主分支

---

### 检查项 2: 依赖关系全量扫描

```bash
# 搜索所有 remote:: 引用
rg "use remote::" --type rust

# 搜索所有 shared_task 相关引用
rg "shared_task|share_publisher|SharePublisher" --type rust

# 验证 SoloDawn 新增模块不使用共享任务功能
rg "shared_task" --type-null crates/orchestrator/ crates/terminal/ crates/git_watcher/ crates/cc-switch/
```

**验收标准:**
- 所有 `use remote::` 引用已记录
- 确认共享任务功能仅被上游代码使用
- SoloDawn 新增模块无依赖

---

### 检查项 3: 基线构建验证

```bash
# 在 vibe-kanban-main 目录执行
cargo check --workspace
```

**验收标准:** 当前代码可编译通过（记录基线，用于迁移后对比）

---

## 任务拆分（一次性迁移）

### Task 11.0: 创建 Git worktree 隔离环境

**目标:** 创建独立的迁移分支，避免污染主分支
**步骤:**
1. 执行 `git worktree add ../solodawn-migration -b single-project-migration`
2. 验证 worktree 创建成功
3. 在 worktree 中执行后续所有迁移任务
**交付物:** 独立的迁移工作目录
**验收标准:** worktree 创建成功，主分支不受影响

---

### Task 11.1: 冻结迁移清单（Keep/Drop/补齐）

**目标:** 输出最终迁移映射表与保留/删除清单  
**步骤:**
1. 全量扫描 `vibe-kanban-main` 与 `cc-switch-main` 的引用关系（`rg`）。
2. 确认保留与删除目录清单，标注 cc-switch 必需模块。
3. 形成迁移映射（来源 -> 目标）。
**交付物:** 迁移映射表、最终清单  
**验收标准:** 清单可直接执行且通过确认

---

### Task 11.2: 远程部署依赖清理（新增）

> **代码审查发现 (2026-01-21):**
> - `crates/remote` 被共享任务（Share）功能依赖
> - 共享任务是 Vibe Kanban 上游的远程部署特性，SoloDawn 不需要
> - 直接删除相关模块，无需重构

**目标:** 删除所有与 `crates/remote` 相关的上游代码
**涉及文件:**
- `crates/services/src/services/share/` (整个目录)
- `crates/services/src/services/remote_client.rs`
- `crates/server/src/routes/shared_tasks.rs`
- `crates/server/src/bin/generate_types.rs`
- `crates/server/src/main.rs`

**步骤:**
1. 删除 `crates/services/src/services/share/` 整个目录
2. 删除 `crates/services/src/services/remote_client.rs`
3. 删除 `crates/server/src/routes/shared_tasks.rs`
4. 从 `crates/server/src/routes/mod.rs` 移除 `shared_tasks` 模块引用
5. 从 `crates/server/src/main.rs` 移除 `share_publisher` 相关代码（第 96-100 行）
6. 从 `crates/services/src/services/container.rs` 移除 `SharePublisher` 相关接口
7. 从 `crates/services/src/services/mod.rs` 移除 `share` 和 `remote_client` 模块
8. 从 `crates/server/Cargo.toml` 移除 `remote = { path = "../remote" }` 依赖
9. 执行 `cargo check --workspace` 验证编译通过

**交付物:** 清理后的代码，无 `remote` 和 `shared_task` 引用

**验收标准:**
- `rg "use remote::" --type rust` 无结果
- `rg "share_publisher|SharePublisher|shared_task" --type rust` 无结果
- `cargo check --workspace` 编译通过

---

### Task 11.3: cc-switch-main 必要模块补齐迁入

**目标:** 补齐 SoloDawn 已集成但 `crates/cc-switch` 缺失的能力
**涉及路径:** `cc-switch-main/src-tauri/src/*`, `crates/cc-switch/src/*`
**步骤:**
1. 对比 `cc-switch-main/src-tauri/src` 与 `crates/cc-switch/src` 能力覆盖。
2. 迁入缺失模块并完成最小整合（入口、导出、依赖整理）。
3. 确保 `rg -n "cc-switch-main"` 无残留引用。
**交付物:** 补齐后的 `crates/cc-switch`
**验收标准:** 所需模块已迁入且代码可被引用（暂不修复编译错误）

---

### Task 11.4: 一次性迁移核心目录与配置

**目标:** 将保留清单中的目录与文件一次性迁入根目录  
**步骤:**
1. 批量移动 `vibe-kanban-main` 保留目录至根目录。
2. 批量移动必要配置文件至根目录（避免覆盖 SoloDawn 现有文件）。
3. 若冲突，保留 SoloDawn 根目录版本，记录差异（迁移后再处理）。
**交付物:** 根目录形成单项目结构  
**验收标准:** 根目录包含 `crates/`, `frontend/`, `shared/`, `assets/`, `scripts/`, `tests/`

---

### Task 11.5: 路径与工作区配置重写

**目标:** 移除旧路径与被删除模块的配置引用  
**涉及文件:** `Cargo.toml`, `pnpm-workspace.yaml`, `package.json`, `scripts/*`  
**步骤:**
1. 从 `Cargo.toml` workspace 成员中移除 `crates/remote`。
2. 从 `pnpm-workspace.yaml` 移除 `remote-frontend`。
3. 从 `package.json` 移除 `npx-cli` 相关 `bin`/`files` 与脚本引用。
4. 全量搜索清理 `vibe-kanban-main`/`cc-switch-main` 路径残留。
**交付物:** 更新后的工作区与脚本配置  
**验收标准:** `rg -n "vibe-kanban-main|cc-switch-main"` 无结果

---

### Task 11.6: 删除不需要模块/目录

**目标:** 删除非 MVP 目录与上游项目残留  
**步骤:**
1. 删除 `remote-frontend`、`npx-cli`、`dev_assets_seed`、上游 `docs` 与杂项文件。
2. 确认无被运行链路引用的目录残留。
**交付物:** 精简后的单项目目录  
**验收标准:** 删除清单目录全部不存在

---

### Task 11.7: 删除源目录（最终收口）

**目标:** 删除 `vibe-kanban-main` 与 `cc-switch-main`  
**步骤:**
1. 再次确认核心目录已迁入根目录。
2. 删除 `vibe-kanban-main` 与 `cc-switch-main`。
**交付物:** 根目录仅保留单项目结构  
**验收标准:** 根目录无 `vibe-kanban-main`/`cc-switch-main`

---

## 迁移后修复清单（不在本阶段执行）

- 统一修复路径与导入，恢复构建与运行。
- 清理遗留前端页面/后端路由，仅保留 MVP 目标功能。
- 运行 `pnpm run check` 与 `cargo test --workspace` 并集中修复问题。
- 更新根目录 README（仅保留 SoloDawn 自有内容）。

---

## 📋 代码审查报告摘要 (2026-01-21)

### 审查执行

- **审查对象:** 单项目迁移计划 (`2026-01-21-single-project-migration.md`)
- **审查范围:** 完整项目代码审计 + 计划可行性评估
- **审查方法:** 全量依赖搜索 + 架构分析 + 建议验证

### 关键发现

| 维度 | 发现 | 评估 |
|------|------|------|
| **架构理解** | 准确识别了嵌套三层结构 | ✅ 9/10 |
| **计划合理性** | 策略正确，但风险评估不足 | ⚠️ 7/10 |
| **技术可行性** | 存在隐藏依赖，可能导致迁移失败 | 🔴 6/10 |
| **完整性** | 遗漏 CI/CD 和 IDE 配置 | ⚠️ 7/10 |
| **可操作性** | 任务拆分清晰，但缺乏回滚方案 | ✅ 8/10 |

**综合评分:** 7.4/10 - 计划基本合理，但风险被低估

### 风险等级

| 风险等级 | 风险项 | 解决方案 |
|---------|--------|----------|
| 🔴 **高风险** | `crates/remote` 的隐藏依赖 | 新增 Task 11.2 清理共享任务模块 |
| 🟡 **中风险** | pnpm-workspace 配置错误 | Task 11.5 统一重写 |
| 🟢 **低风险** | 文档引用错误 | 迁移后清理 |

### 修正后的迁移策略

```bash
# 阶段 0: 创建隔离环境 (Task 11.0)
git worktree add ../solodawn-migration -b single-project-migration

# 阶段 1: 清理远程部署依赖 (Task 11.2)
# 删除 share/、remote_client.rs、shared_tasks.rs 等

# 阶段 2: 继续原计划迁移 (Task 11.3-11.7)
# 移动文件 → 重写配置 → 删除源目录
```

### 建议采纳情况

| 原建议 | 修正后 | 状态 |
|--------|--------|------|
| 1. 执行 `rg "use remote::"` 搜索 | ✅ 纳入迁移前检查清单 | 已采纳 |
| 2. 删除前重构依赖 | ✅ 改为直接删除 share 模块 | 已修正 |
| 3. 创建 Git worktree | ✅ 纳入 Task 11.0 | 已采纳 |
| 4. 分阶段验证 | ✅ 每个任务执行 `cargo check` | 已采纳 |

### 后续建议

1. **必须执行** (迁移前):
   - ✅ 在 worktree 中执行所有迁移操作
   - ✅ 保留详细日志，记录所有错误

2. **建议执行** (迁移中):
   - ✅ 每个任务完成后执行 `cargo check --workspace`
   - ✅ 使用 `rg` 验证无残留引用

3. **可选执行** (迁移后):
   - ⚪ 更新 CI/CD 配置
   - ⚪ 更新 IDE 配置

