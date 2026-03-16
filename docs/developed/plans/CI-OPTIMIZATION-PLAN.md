# CI Pipeline 全面优化计划

> **目标**: 在 100% 保留所有审计检查（build、clippy、test、lint、typecheck、docker build、sonar）的前提下，通过消除重复编译、引入精细缓存、优化构建流程，将 CI 总耗时压缩 50-70%。
>
> **核心原则**: 不跳过任何检查，不降低任何质量标准。所有优化手段都是让同样的检查跑得更快，而不是少跑检查。
>
> **并发策略**: 20 个 Agent 分 4 个 Wave 执行，同一 Wave 内的 Agent 互不冲突（各自负责独立文件），Wave 之间有依赖关系。

---

## 当前瓶颈分析

| 问题 | 影响 | 根因 |
|------|------|------|
| `cargo check` + `cargo clippy` 重复编译 | 浪费 5-10 分钟 | clippy 已包含 check 功能，多跑一遍纯属浪费 |
| clippy 和 test 的 feature flags 不一致 | 增量编译复用差 | clippy 用 `--all-targets --all-features`，test 用默认 |
| ci-basic 和 ci-docker 各自从零编译 Rust | 两倍编译时间 | 没有共享编译缓存 |
| ci-quality 也独立编译一遍 | 三倍编译时间 | 独立 cache key |
| actions/cache 缓存 target/ 目录 | 上传下载慢，命中率不稳定 | target/ 太大（几 GB），Cargo.lock 一变全失效 |
| Docker 无 cargo-chef | 任何代码变动全量重编译 | 依赖和代码没有分层 |
| 系统依赖安装重复 | 每个 job 各装一遍 | 没有复用 |
| `--test-threads=1` 串行测试 | 测试执行慢 | 可能因 SQLite 锁或全局状态 |
| GHA runner 只有 2 核 | 编译天花板低 | 免费 runner 限制 |

---

## 优化方案总览

### 方案 A: 消除重复编译（立即生效）
- 删除 `cargo check` 步骤（clippy 完全覆盖）
- 对齐 clippy 和 test 的编译参数，最大化增量复用

### 方案 B: 引入 sccache（编译单元级缓存）
- 替代 actions/cache 对 target/ 的粗粒度缓存
- 以单个 .o/.rlib 为粒度，Cargo.lock 变了也只重编译变化的 crate
- 所有 Rust 编译 job 共享同一个 sccache 缓存池

### 方案 C: Docker 构建引入 cargo-chef
- 将 Rust 编译拆成「依赖层」和「代码层」
- 依赖层缓存命中率接近 100%（第三方依赖极少变）
- 代码层只增量编译项目自身代码

### 方案 D: 提取 Composite Actions（消除重复配置）
- Rust 工具链 + 系统依赖 + sccache 封装为可复用 action
- 前端 Node + pnpm 封装为可复用 action
- 所有 workflow 共享，减少维护成本

### 方案 E: 测试执行优化
- 引入 cargo-nextest 替代 cargo test
- 更好的并行调度，测试隔离
- 评估是否可以放开 `--test-threads=1` 限制

### 方案 F: 跨 Job 缓存共享
- sccache 使用 GHA cache backend，所有 job 自动共享
- ci-basic 编译完的产物，ci-docker 和 ci-quality 直接复用

---

## Agent 任务分配（20 个 Agent，4 个 Wave）

### Wave 1: 基础设施层（Agent 1-6，互不冲突）

每个 Agent 创建独立的文件，不修改任何已有文件。

| Agent | 任务 | 负责文件 | 说明 |
|-------|------|----------|------|
| **Agent 1** | 创建 Rust 环境 Composite Action | `.github/actions/setup-rust/action.yml` | 封装：系统依赖安装、Rust 工具链安装、sccache 安装配置、cargo-nextest 安装。输入参数：sccache 开关、nextest 开关、额外系统包 |
| **Agent 2** | 创建前端环境 Composite Action | `.github/actions/setup-frontend/action.yml` | 封装：pnpm 安装、Node.js 安装、pnpm install。输入参数：node 版本、pnpm 版本、working-directory |
| **Agent 3** | 创建 sccache 配置 Composite Action | `.github/actions/setup-sccache/action.yml` | 封装：sccache 二进制下载安装、环境变量设置（SCCACHE_GHA_ENABLED、RUSTC_WRAPPER）、GHA cache backend 配置、统计输出步骤 |
| **Agent 4** | 创建 cargo-chef 优化的 Dockerfile 片段 | `docker/Dockerfile.chef-stage` | 编写 cargo-chef prepare + cook 阶段模板，作为后续 Dockerfile 重构的输入。包含：chef 安装、recipe.json 生成、依赖预编译、代码编译分离 |
| **Agent 5** | 编写 sccache 集成测试脚本 | `scripts/ci/test-sccache.sh` | 验证 sccache 是否正常工作的测试脚本：检查 RUSTC_WRAPPER 设置、编译一个小 crate、验证缓存命中、输出统计 |
| **Agent 6** | 编写 CI 耗时基准记录脚本 | `scripts/ci/benchmark-ci.sh` | 记录各步骤耗时的脚本，用于优化前后对比。输出 JSON 格式：{step, duration_seconds, cache_hit_rate} |

### Wave 2: 核心优化（Agent 7-13，依赖 Wave 1）

Agent 7-10 各自修改不同的 workflow 文件，互不冲突。Agent 11-13 修改非 workflow 文件。

| Agent | 任务 | 负责文件 | 说明 |
|-------|------|----------|------|
| **Agent 7** | 重构 ci-basic.yml | `.github/workflows/ci-basic.yml` | ① 删除 `cargo check` 步骤（clippy 已覆盖）② 替换手动 Rust 安装为 `setup-rust` composite action ③ 替换手动前端安装为 `setup-frontend` composite action ④ 集成 sccache（通过 composite action）⑤ 对齐 clippy 和 test 的 feature flags ⑥ 将 `cargo test` 替换为 `cargo nextest run` ⑦ 评估并调整 `--test-threads` 参数 ⑧ 添加 sccache 统计输出步骤 |
| **Agent 8** | 重构 ci-quality.yml | `.github/workflows/ci-quality.yml` | ① 替换手动 Rust 安装为 `setup-rust` composite action ② 集成 sccache ③ 利用 sccache 共享缓存（与 ci-basic 同一个 cache backend）④ 移除独立的 cargo cache 步骤（sccache 替代）⑤ 添加 sccache 统计输出 |
| **Agent 9** | 重构 ci-docker.yml | `.github/workflows/ci-docker.yml` | ① 优化 Docker BuildKit 缓存策略 ② 添加更精细的 cache-from/cache-to 配置 ③ 添加构建耗时输出 ④ 确保 GHA cache backend 正确配置 |
| **Agent 10** | 重构 ci-notify.yml（如需适配） | `.github/workflows/ci-notify.yml` | ① 检查是否需要适配新的 job 名称 ② 确保 workflow_run 触发器匹配新的 workflow 结构 ③ 不需要改动则标记为无需修改 |
| **Agent 11** | 重构 Dockerfile 引入 cargo-chef | `docker/Dockerfile` | ① 添加 chef-planner 阶段（cargo chef prepare）② 添加 chef-builder 阶段（cargo chef cook --release）③ 修改 rust-builder 阶段使用 chef 预编译的依赖 ④ 保留所有现有构建参数和镜像支持 ⑤ 保留 docker profile 配置 ⑥ 确保 BuildKit cache mount 正确配置 |
| **Agent 12** | 优化 Cargo workspace 编译配置 | `Cargo.toml`（仅 workspace 级别的 `[profile]` 部分） | ① 检查并优化 CI profile 设置 ② 确认 docker profile 的 `opt-level=1, codegen-units=256` 是否合理 ③ 考虑添加 `[profile.ci]` 专用 profile（如果有意义）④ **不修改依赖声明、不修改 crate 列表** |
| **Agent 13** | 创建 CI 优化文档 | `docs/developed/ops/ci-optimization.md` | 编写 CI 优化说明文档：① 各优化点的原理说明 ② sccache 工作机制 ③ cargo-chef 工作机制 ④ 缓存策略说明 ⑤ 故障排除指南（缓存失效、sccache 故障回退等）|

### Wave 3: 集成验证（Agent 14-18，依赖 Wave 2）

确保所有修改协调一致，不遗漏。

| Agent | 任务 | 负责文件 | 说明 |
|-------|------|----------|------|
| **Agent 14** | ci-basic.yml 集成验证 | 只读验证，不修改文件 | ① 验证 composite action 引用路径正确 ② 验证 sccache 环境变量传递正确 ③ 验证 nextest 命令语法 ④ 验证 feature flags 对齐 ⑤ 输出验证报告 |
| **Agent 15** | ci-quality.yml 集成验证 | 只读验证，不修改文件 | ① 验证 composite action 引用路径正确 ② 验证 sccache 缓存共享配置 ③ 验证 SonarCloud 步骤未受影响 ④ 输出验证报告 |
| **Agent 16** | ci-docker.yml + Dockerfile 集成验证 | 只读验证，不修改文件 | ① 验证 Dockerfile 语法（docker buildx build --check）② 验证 cargo-chef 阶段顺序正确 ③ 验证 BuildKit cache 配置一致 ④ 验证多阶段构建产物传递正确 ⑤ 输出验证报告 |
| **Agent 17** | 全 workflow 交叉验证 | 只读验证，不修改文件 | ① 验证所有 workflow 的 action 版本一致（SHA pinning）② 验证 ci-notify 的 workflow_run 触发器匹配 ③ 验证没有遗漏的 required check ④ 验证所有 secret 引用正确 ⑤ 输出验证报告 |
| **Agent 18** | Composite Action 单元验证 | 只读验证，不修改文件 | ① 验证所有 composite action 的 inputs/outputs 定义完整 ② 验证 action.yml 语法正确 ③ 验证 shell 脚本可执行 ④ 输出验证报告 |

### Wave 4: 修复与收尾（Agent 19-20，依赖 Wave 3）

根据 Wave 3 的验证报告修复问题。

| Agent | 任务 | 负责文件 | 说明 |
|-------|------|----------|------|
| **Agent 19** | 修复验证发现的问题（Workflow 文件） | Wave 3 报告中涉及的 workflow 文件 | 根据 Agent 14-17 的验证报告，修复所有发现的问题。每个问题逐一修复并记录。|
| **Agent 20** | 修复验证发现的问题（非 Workflow 文件） | Wave 3 报告中涉及的其他文件 | 根据 Agent 16-18 的验证报告，修复 Dockerfile、Composite Action、脚本等文件中的问题。|

---

## 文件所有权矩阵（防冲突）

| 文件 | Wave 1 | Wave 2 | Wave 3 | Wave 4 |
|------|--------|--------|--------|--------|
| `.github/actions/setup-rust/action.yml` | Agent 1 创建 | — | Agent 18 验证 | Agent 20 修复 |
| `.github/actions/setup-frontend/action.yml` | Agent 2 创建 | — | Agent 18 验证 | Agent 20 修复 |
| `.github/actions/setup-sccache/action.yml` | Agent 3 创建 | — | Agent 18 验证 | Agent 20 修复 |
| `docker/Dockerfile.chef-stage` | Agent 4 创建 | Agent 11 消费 | — | — |
| `docker/Dockerfile` | — | Agent 11 修改 | Agent 16 验证 | Agent 20 修复 |
| `scripts/ci/test-sccache.sh` | Agent 5 创建 | — | — | — |
| `scripts/ci/benchmark-ci.sh` | Agent 6 创建 | — | — | — |
| `.github/workflows/ci-basic.yml` | — | Agent 7 修改 | Agent 14 验证 | Agent 19 修复 |
| `.github/workflows/ci-quality.yml` | — | Agent 8 修改 | Agent 15 验证 | Agent 19 修复 |
| `.github/workflows/ci-docker.yml` | — | Agent 9 修改 | Agent 16 验证 | Agent 19 修复 |
| `.github/workflows/ci-notify.yml` | — | Agent 10 检查 | Agent 17 验证 | Agent 19 修复 |
| `Cargo.toml`（profile 部分） | — | Agent 12 修改 | — | — |
| `docs/developed/ops/ci-optimization.md` | — | Agent 13 创建 | — | — |

---

## 各优化的预期收益

| 优化项 | 影响范围 | 预期节省时间 | 风险等级 |
|--------|----------|-------------|----------|
| 删除 cargo check | ci-basic | 5-10 分钟 | 极低（clippy 完全覆盖） |
| 对齐 feature flags | ci-basic | 3-5 分钟 | 低（编译参数调整） |
| sccache 替代 actions/cache | 全部 Rust job | 10-20 分钟 | 中（新工具引入） |
| cargo-chef | ci-docker | 15-25 分钟 | 中（Dockerfile 重构） |
| 跨 job 缓存共享 | ci-quality, ci-docker | 10-15 分钟 | 中（缓存一致性） |
| cargo-nextest | ci-basic | 2-5 分钟 | 低（测试运行器替换） |
| Composite Actions | 全部 workflow | 维护成本降低 | 低（重构，不改逻辑） |

**总计预期**: 从目前的 ~45 分钟（backend-check）+ ~60 分钟（docker-build）压缩到 ~15-20 分钟 + ~10-15 分钟。

---

## 回退策略

每个优化都有独立的回退路径：

1. **sccache 故障**: composite action 设计为可禁用（`sccache-enabled: false`），回退到直接编译
2. **cargo-chef 问题**: Dockerfile 保留旧的构建路径注释，可快速还原
3. **nextest 兼容性**: 回退到 `cargo test`，只需改一行命令
4. **composite action 问题**: 每个 workflow 可以内联展开 action 内容

---

## 执行前提

1. 需要确认 GitHub repo 的 Actions cache 配额（免费版 10GB，sccache 需要足够空间）
2. 需要确认是否有 GitHub 付费 runner（方案不依赖，但有的话效果更好）
3. cargo-chef 和 cargo-nextest 需要在 CI 中安装（已包含在 composite action 设计中）

---

## 验收标准

- [ ] 所有现有检查 100% 保留（clippy、test、lint、typecheck、sonar、docker build）
- [ ] 所有 workflow 正常触发且通过
- [ ] ci-basic backend-check 耗时 < 20 分钟
- [ ] ci-docker docker-build 耗时 < 25 分钟（首次），< 15 分钟（缓存命中）
- [ ] ci-quality 耗时 < 15 分钟
- [ ] sccache 缓存命中率 > 70%（第二次及以后的运行）
- [ ] 所有 GitHub Actions 仍然 pinned to full commit SHA
- [ ] SonarCloud 仍然 0 issues
