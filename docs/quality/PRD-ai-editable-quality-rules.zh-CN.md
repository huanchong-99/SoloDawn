# PRD — AI 辅助、多智能体校验、用户可编辑的质量门禁规则

| 字段 | 值 |
|---|---|
| **标题** | AI 辅助、多智能体校验、用户可编辑的质量门禁规则 |
| **状态** | /goal-ready 草稿 |
| **日期** | 2026-06-20 |
| **修订** | rev2（2026-06-20） |
| **模型** | opus 4.8 / ultracode |
| **负责人** | 质量平台 |
| **范围** | `crates/quality`、`crates/services`、`crates/server`、`crates/db`、`crates/local-deployment`、`crates/executors`、`frontend` |

> **rev2 变更（摘要）：** 来自负责人的决策 D1–D9 现已落地，相应的开放问题被标记为 RESOLVED（第 17 节）。创作引擎不再是"一把计量的 key"——它是**所有全局配置的 LLM 来源，由用户自行选择**，统一隐藏在单一的**创作模型 invoker** 抽象之后，配备两个均复用现有基础设施的后端：计量的 `LLMClient`（`create_llm_client`）以及**订阅制原生 OAuth 交互式传输**（货真价实的 `claude` 二进制，不走额度池）。一个新的顶层 **Reuse Map** 章节（位于 §8 与 §9 之间）把"复用优先于重写"的姿态显式化。规则格式采用复用优先的分期：**P1 = 仅受限正则（零新增依赖）**，**P2 = ast-grep AST 格式**。二次校验的**确认对话框是强制的**，绝不可选。

---

## 1. 执行摘要

SoloDawn 当前的质量门禁只能强制执行约 43 个硬编码的数值指标（`MetricKey`，`crates/quality/src/metrics.rs:14-172`），并通过 `GT`/`LT` 条件设定阈值。*产生*这些指标的规则是被编译进二进制文件的 Rust 结构体（`crates/quality/src/rules/`）。项目负责人若要添加某个项目专属的规则（"在我的代码里禁止 `X`"），离不开一个 Rust PR；他们甚至无法*理解*像 `test_file_absence` 或 `runtime_security_smells` 这样的指标究竟意味着什么——编辑器只渲染出裸露的枚举标记，没有任何描述（`QualityGateRulesEditor.tsx:184-188`）。

本特性按用户提出的顺序交付三样东西：

1. **自文档化的提示气泡（tooltip）**——规则编辑器中每个指标旁边都有一个带圈的 "!"，展示它*度量什么*以及*它在项目中的当前值*（读取自最新的已持久化运行，悬停时绝不重新计算——D7）。
2. **自然语言 → 规则生成**——用户输入"我想禁止 X"，然后**他们已经全局配置好的任意 LLM 来源**（他们自己计量的 API key *或*他们的官方 Claude 订阅）起草出一条声明式规则外加一段通俗语言描述，后者反过来又喂给同一个 "!" 提示气泡。用户通过现有的模型选择 UI *自行挑选用哪个已配置来源*进行创作（D1）。
3. **多智能体对抗式校验，配合一项实证性的真值测试和一项无上下文的往返检查**——因为生成的规则可能达不到标准，智能体会*对抗式*地打磨它，候选规则会被*执行*到 must-flag / must-not-flag（必须命中 / 必须不命中）的代码片段上，**用户在一个强制对话框中确认**（D2），随后一个**全新的、被剥夺上下文的**智能体仅凭规则本体逆向推断出意图，再由一名裁判把它的重建结果与原始请求进行比对。匹配 → 可用；不匹配 → 循环，并设有硬上限。

设计上的关键支点是一种**声明式、沙箱化的规则格式**（P1：受限正则；P2：ast-grep YAML）：规则是*数据*，绝不是可执行代码。这就使得核心原则**"用 AI 生成，不用 AI 执行"**得以成立——LLM 只在一次性创作期间出现；一旦经人工确认，规则就是固定数据，由一个确定性的、无 LLM 的引擎在每一次门禁运行中以完全相同的方式运行。这恰恰化解了那个悖论：用 AI 去构建那道本应捕捉 AI 错误的护栏。

**rev2 姿态——复用优先于重写（D9）。** 本特性所需的几乎一切都已存在于代码库中并经实地验证：`LLMClient` trait 与 `create_llm_client`；一个已经构建好、与 trait 兼容的**订阅制交互式传输**（无 `-p` 的原生 OAuth 路径，`crates/local-deployment/src/container.rs:1468`）；严重度封顶机制（`cap_for_advisory`）；指标/门禁机制；模型选择器 hook + 下拉 UI；带 `gates_confirmed_at` 硬阻断的 G2 确认对话框。真正净新增的表面只是少数几个胶水模块（`rule_authoring/`、`provider/declarative.rs`）、四张 DB 表、几个枚举变体，以及前端接线。"Reuse Map" 章节用两列枚举了这种"既有 vs 胶水"的拆分。

---

## 2. 问题与动机

**门禁规则仅限 Rust 且不是数据驱动的。** 规则抽象（`crates/quality/src/rules/mod.rs:18-51`）是 `Rule` + `CommonRule`/`RustRule`/`TsRule` 这一 trait 家族；每条规则都是一个手写的结构体，注册在 `all_rust_rules()`（`rules/rust/mod.rs:22`）、`all_ts_rules` 或 `all_common_rules` 之中。**不存在任何在运行期从数据加载 pattern/severity/metric 的结构体**。添加一条规则意味着编写并编译 Rust。

**Rust↔TS 的不对称。** Rust 规则使用真正的 `syn` AST（例如 `rules/rust/error_handling.rs` 通过 `syn::visit::Visit` 实现了"在 `#[cfg(test)]` 之外使用 .unwrap()/.expect()/panic!"）；TypeScript 规则则仅基于正则/行（`rules/typescript/console_usage.rs`）——这个 crate 里没有 TS AST。P1 受限正则格式逐行地对两种语言都奏效；P2 ast-grep 格式则为形如 AST 的规则弥合了结构性缺口（D5）。

**约 43 个不透明的指标，零文档。** `MetricKey` 是一个封闭枚举（`metrics.rs:14-172`），其 `as_str()` 吐出诸如 `clippy_warnings`、`tsc_errors`、`runtime_security_smells` 这样的标记。`selectable_metric_keys()`（`crates/server/src/routes/quality.rs:207-254`）把这些直接喂给编辑器，而编辑器只展示原始标记、不加任何解释（`QualityGateRulesEditor.tsx:174-194`）。用户压根没法判断该选什么——这正是"我看不懂 / 我不知道该加什么"的抱怨。

**一类真实存在的行为缺陷成为实证测试的动机。** 代码库此前已经发布过过于宽泛的规则——例如 `test_file_absence` 把符合惯用法的内联 `#[cfg(test)]` 模块都标记了出来。一个生成正则的 LLM 会原样复现这种过度命中。一次文本往返检查（"重建出来的请求是否匹配？"）抓不住*行为上*的误报；只有把规则*执行*到精心整理的代码片段上才能抓住。

**持久化机制存在但不透明。** 每个项目的策略是 SQLite 表 `project_quality_policy`（迁移 `20260614120000`），其中保存着一份不透明的 `QualityGateConfig` 的 `config_yaml`，外加一个反范式化的 `mode`（`crates/db/src/models/project_quality_policy.rs`）。规则*不是*一等行——根本没有地方存放 pattern 本体、它的自然语言出处，或它的校验产物。

---

## 3. 目标与非目标

### 目标
- G1. 规则编辑器中每个可选指标都有一个悬停 "!" 提示气泡，含名称、描述、示例，以及该指标在项目中的**当前值**（读取自最新的已持久化 `quality_run`，并标注为"截至 `<timestamp>`"——D7）。
- G2. 用户可以提交一段自然语言请求，并使用**他们已配置好的任意 LLM 来源（由他们自己选择）**，收到一条生成的、声明式的、沙箱化的规则 + 通俗语言描述（他们计量的 API key *或*他们的官方 Claude 订阅——D1）。
- G3. 生成的规则会经过**对抗式**校验，**实证性地执行**到生成的正/负代码片段上，**在一个强制对话框中由人工确认**（D2），随后由一个无上下文智能体 + 裁判进行**往返验证**；失败则循环并设硬上限（`MAX_AUTHORING_ROUNDS = 4`，D6），否则交还给用户。
- G4. 已确认的规则由一个无 LLM 的引擎 provider 在每一次运行中以**确定性且完全相同**的方式强制执行。
- G5. 整个特性是**增量且非破坏性**的（R4 约束）：现有的内置 provider 和策略 YAML 形态原封不动；一切以"暗发"（默认关闭）方式上线，并按项目以 shadow→warn→enforce 逐步推出。
- G6. 自定义规则可以按项目展示、编辑、添加和删除（R4 的"二级弹窗"目标）。优先项目作用域（v1 UI 中 `project_id NOT NULL`），同时把该列保持为可空，以便后续增量地引入全局/组织级步骤（D4）。

### 非目标
- N1. 不执行任何用户/LLM 提供的任意代码。规则是纯数据。
- N2. 扫描/强制执行路径中永远没有 LLM。
- N3. 不新建全局 LLM 设置存储；复用现有的、按实体加密的配置**以及**现有的模型来源枚举/选择器（D1、D9）。
- N4. 不改动 `quality_run`/`quality_issue` 审计表——创作期校验是一个*独立的*产物存储。
- N5. 组织级/全局（无项目）规则在 schema 上是支持的（`project_id` 保持可空），但 v1 的 UI/特性是项目作用域的（`project_id NOT NULL`）；全局作用域是后续的增量步骤（D4）。
- N6. 不新增确认对话框——扩展现有的 G2 `QualityGateConfirmDialog`（保留 `gates_confirmed_at` 的硬阻断）。人工确认是**强制的**，绝不可选（D2）。
- N7. 不新增 LLM 传输。订阅制创作后端复用已经构建好、已经接线好的无 `-p` 交互式原生 OAuth 传输（D1、D9）；创作这一轮**不引入任何 PTY**（见 §7.2 的更正）。

---

## 4. 现状（有据可依）

| 关注点 | 今日实情 | 文件 |
|---|---|---|
| 引擎流程 | `QualityProvider::analyze` → `ProviderReport{metrics, issues}` → 聚合 → 门禁条件**只**读取 `metrics`（不读 `issues`） | `provider/mod.rs:131-170`、`engine.rs` |
| 严重度上限 | `cap_for_advisory` 把 `SeverityOrigin::ProjectConfig` 的分析器封顶到 `Major`；`severity_origin()` 是一个**穷尽式** match（用 `_` 不能编译） | `rule.rs:124-126`、`rule.rs:205-219` |
| 分析器来源 | 封闭枚举 + `Other(String)`；`Other(_) => Tool`（保留完整严重度） | `rule.rs:158-178`、`rule.rs:219` |
| 指标目录 | 封闭枚举，3 处耦合点（变体+重命名、`as_str`、`display_name`）；哨兵 `QualityGateEmptyScan` 标了 `#[ts(skip)]` | `metrics.rs:14-273` |
| Provider 注册 | 单一站点 `build_providers`，每个都受一个 `ProvidersConfig` 布尔值控制 | `engine.rs:81-141`、`config.rs:91-120` |
| Provider 开关 | 全部通过 `default_true()` 默认为 `true` | `config.rs:122-124` |
| 策略存储 | `project_quality_policy`（BLOB `project_id` 主键、不透明 `config_yaml`、反范式化 `mode`）；解析器是 DB 优先级 0，每次运行重新读取 | `models/project_quality_policy.rs`、`quality_policy.rs:26` |
| 编辑器 | 唯一一个共享的 `QualityGateRulesEditor`，被 `RulesDialog`、`ConfirmDialog`、`SettingsNew` 共用；`metricOptions` 已穿线到全部三处；使用原始的 `text-slate-*`/`bg-white` Tailwind（**不是** `.new-design`） | `QualityGateRulesEditor.tsx` |
| 指标目录 API | `GET /api/quality/policy/metrics` → `MetricCatalogResponse { metrics, operators }` | `quality.rs:198-203`、`:277` |
| 计量 LLM 客户端 | `create_llm_client(&OrchestratorConfig)` → `Box<dyn LLMClient>`；一个方法 `chat(Vec<LLMMessage>) -> LLMResponse` | `orchestrator/llm.rs:922`、`:23` |
| **订阅制 LLM 客户端（已构建）** | `InteractiveClaudeClient` 针对货真价实的 `claude` 二进制实现 `LLMClient`（订阅/原生 OAuth 表面，不走 `-p`/Agent-SDK 池）；`create_interactive_claude_client(model)` 在无原生凭据时返回 `None` | `llm.rs:628-703`、`:706`、`:868` |
| **交互式传输（已接线）** | `LocalContainerService::try_spawn_interactive_native_oauth` 把每一次 ClaudeCode 运行都在 `-p` 路径**之前**自动路由到无 `-p` 传输；单轮、stdin 接 null 的 piped spawn + 转录 tailer（**不是** PTY） | `container.rs:1468`、`:1044`、`:2108` |
| 模型枚举 | `GET /api/cli_types/:cli/models` → `Vec<ModelConfig>`（`hasApiKey`/`isOfficial`/`displayName`/`apiType`）；Rust 侧 `ModelConfig::find_user_configured`/`find_all` | `cli_types.rs:156-162`、`cli_type.rs:309/345` |
| 模型选择器（FE） | `useModelConfigForExecutor` 合并自定义+官方、自动选中、暴露 `selectedModelConfigId`/`setSelectedModelConfigId`；渲染为带 Custom/Official 分区的 `ToolbarDropdown` | `useModelConfigForExecutor.ts:56`、`CreateChatBox.tsx:143-201` |
| 计量/订阅判别器 | `InteractiveAuthMode::resolve(api_key, base_url)` → `NativeOauth`（None,_）/ `OfficialKey`（Some,None）/ `Relay`（Some,Some） | `cc_switch.rs:594-604` |
| 阶段模板 | `generate_audit_plan`——自由的 `async fn(&dyn LLMClient,...)`，在 LLM/解析出错时 fail-close 到一个默认值 | `audit_plan.rs:22-70` |
| 裁判形态 | `AuditScoreResult::parse`（各维度 → 总分对比 `PASS_THRESHOLD`），解析失败 = 不及格分 | `types.rs:631`、`:624` |
| 循环上限惯用法 | `FINAL_REPAIR_MAX_ROUNDS = 4` + 守卫 `bail!` | `agent.rs:115`、`:8203` |
| G2 确认硬阻断 | 若 `gates_confirmed_at` 为 null（服务端），物化被拒绝 | `planning_drafts.rs:976`、handler `:436`、mount `:162` |

**声明式缺口：** 既没有数据驱动的规则类型，也没有面向可编辑规则的 DB 表。本特性以增量方式把两者都构建出来。**创作 + 强制执行流水线中的其余一切都是复用**（见 Reuse Map）。

---

## 5. 设计原则

1. **用 AI 生成，不用 AI 执行。** LLM 被限定在创作环节（一次性、经多重校验、经人工确认）。确认之后，规则就是固定数据，确定性引擎永远以完全相同的方式运行它。一条正确的规则无论模型如何漂移都保持正确；强制执行路径从不调用 LLM，也从不把扫描到的源码喂进 prompt。
2. **规则是数据，绝不是代码。** 规则是一个受限正则的 JSON 对象（P1），或一份 ast-grep YAML 文档（P2）。匹配器无法打开文件、套接字或派生进程——这个安全性质是*结构性的*。
3. **实证真值高于 LLM 共识。** 两个意见一致的 LLM 会一起收敛到错误答案上；而一份被执行过的正/负夹具（fixture）无法被"辩"出行为失败。实证测试对任何裁判意见都具有最终权威。
4. **对抗，而非附和。** 第二个智能体的职责是*打破*规则（误报、规避、歧义），而不是随声附和。
5. **被剥夺上下文的独立性。** 往返解释器*只*看得到规则本体——它无法把原始意图偷渡回来，这就使往返成为一项真正独立的检查。
6. **默认仅作建议，门禁需主动选入（D3）。** 自定义规则的严重度被封顶到 `Major`（`AnalyzerSource::CustomRule → SeverityOrigin::ProjectConfig`，经由 `cap_for_advisory`）；门禁只通过一个显式的、主动选入的 `CustomRuleCritical` 计数指标发生。生成的规则无法自我升级到 `Blocker`。
7. **增量且可逆。** 新 provider、新表、新路由、新 MetricKey——任何现有东西的形态都不变。默认关闭；按项目 shadow→enforce。
8. **fail-closed 且有界。** 每个创作阶段都 fail-close 到一个安全默认值；循环有硬上限 4（D6）；在 Enforce 模式下强制执行超时会发出一个 Blocker。
9. **复用优先于重写（D9）。** 在每一层（LLM 调用、严重度机制、持久化模式、选择器 UI、确认对话框）都优先选择既有组件而非新建。任何 v1 提议要构建、却已经存在的东西，都会在 Reuse Map 中被标注出来，并从净新增清单中移除。

---

## 6. 声明式规则格式（关键支点）

### 6.1 选型（分期——D5）
每条自定义规则都携带一个 `rule_format` 判别项，因此两种格式得以共存，而简单的标记/头部封禁绝不会拉起 AST 路径：
- **`regex`（P1，最先上线，零新增依赖）**——一个建立在该 crate 现有 `regex` 1.x（已经是依赖，`crates/quality/Cargo.toml:25`）之上的受限正则声明式 JSON schema，直接以 `rules/typescript/console_usage.rs` 为蓝本。横跨 Rust 与 TS 覆盖封禁标记 / 禁止子串 / 必需头部这类风格的规则。
- **`ast_grep`（P2，增量的表达力升级）**——ast-grep YAML，通过 MIT 许可的 crates **`ast-grep-core` + `ast-grep-config` + `ast-grep-language`**（捆绑的 Rust + TypeScript tree-sitter 语法）进程内嵌入，用于诸如"在 handler 中禁止 `.unwrap()`"这样的结构性规则。**仅在**确认了未经验证的 ast-grep linter-envelope 字段名之后才加入（见 §17，唯一剩余的技术性开放项）。

`rule_format` 判别项从 P1 起就出现在 schema 和 DB 中，因此 P2 是一个纯增量步骤，而非迁移。

### 6.2 理由
P1 受限正则格式是复用优先的最小集：它新增**零**依赖（`regex = "1"` 已经在 `crates/quality/Cargo.toml:25` 中），复用了久经验证的 `console_usage.rs` 的"已编译 `Regex` + 逐行 `find_iter` + `is_test_file` 跳过"模式，并且是线性时间安全的（无回溯/前后查找/反向引用），因此它能立刻覆盖大多数"禁止标记 X"的请求。

对于 P2 结构层，ast-grep 是**唯一**同时满足以下条件的候选：(a) 纯声明式**数据**，绝不作为代码执行；(b) **一套 schema 横跨 Rust 与 TS**，弥合今日 `syn`-与-正则的分裂（TS 没有 AST）；(c) 可作为**许可证干净、无外部二进制/子进程的 Rust crate** 嵌入（不同于 semgrep 的 OCaml 进程 + 限制性规则许可证）；(d) **紧凑且形如代码**，因此 LLM 能可靠地生成它（不同于 tree-sitter 的 `.scm` S-表达式，后者对 LLM 不友好且强迫自建加载器）；以及 (e) 可通过断言 `NodeMatch` 的数量/位置在进程内**对代码片段进行测试**。

**应避免：** semgrep/opengrep（子进程 + 限制性规则许可证 + 最大的沙箱攻击面）以及裸 tree-sitter 查询（对 LLM 不友好 + 自建加载器）。

### 6.3 示例

**受限正则（P1）——禁止标记 / 文件头部存在性（零新增依赖）：**
```json
{
  "kind": "forbidden_token",
  "languages": ["rust", "typescript"],
  "pattern": "\\bdbg!\\s*\\(",
  "message": "dbg! macro left in committed code",
  "exclude_globs": ["**/tests/**"]
}
```

**受限正则（P1）——在已提交的 TS 中封禁 `console.log`（镜像 `console_usage.rs`）：**
```json
{
  "kind": "forbidden_token",
  "languages": ["typescript"],
  "pattern": "console\\.log\\s*\\(",
  "message": "console.log left in committed TS",
  "exclude_globs": ["**/*.test.ts", "**/*.spec.ts"]
}
```

**ast-grep（P2）——在测试之外禁止 `.unwrap()`（结构性；即 `error_handling.rs` 的确切行为，以声明式表达）：**
```yaml
id: no-unwrap-outside-tests
language: rust
severity: warning
message: ".unwrap() outside test code can panic in production"
rule:
  pattern: $E.unwrap()
  not:
    inside:
      any:
        - matches: inside-cfg-test   # util sub-rule for #[cfg(test)] scope
        - matches: inside-test-fn
```

> **[unverified]** 在编写 P2 加载器之前，必须对照锁定的 ast-grep crate 版本确认 ast-grep linter-envelope 的确切字段名（`id`/`message`/`severity`/`note`/`constraints`/`utils`）（这是唯一剩余的技术性开放项，§17）。匹配类别（原子型 `pattern`/`kind`/`regex`；关系型 `inside`/`has`/`follows`/`precedes`；组合型 `all`/`any`/`not`/`matches`）已确认。P1 不依赖于此。

### 6.4 沙箱模型
- **无 FS / 无网络 / 无子进程**——一条 pattern 规则根本不派生任何子进程（不同于 `provider/mod.rs:120` 的 node 子进程 provider），因此天然继承了无 FS/无网络的性质。（注意：这也是为什么创作那一轮**不**使用 PTY——扫描路径是纯进程内匹配；§7.2 中关于 PTY 的讨论只涉及如何调用*订阅制创作*的 LLM，而非强制执行。）
- **正则沙箱**——Rust 的 `regex` 1.x 是线性时间的（无回溯/前后查找/反向引用），因此回溯型 ReDoS 不可能发生。剩余的攻击向量是 DFA 内存膨胀（例如 `a{0,1000000}`）：**在加载时一次性**通过 `RegexBuilder::new(p).size_limit(1<<20).dfa_size_limit(1<<20).build()` 编译；把失败作为面向用户的 400 拒绝，绝不在扫描时拒绝。
- **输入边界**——限制每文件字节数；跳过/截断病态的压缩单行；复用 `analysis::is_excluded`（`analysis/mod.rs:21`），使 `node_modules`/`target`/`dist`/`.git`/`vendor`/`.next`/`build` 永不被遍历。
- **AST 解析错误跳过（P2）**——保留 `syn`/tree-sitter 的解析错误跳过模式（`builtin_rust.rs:77`），使敌意的扫描源码无法令一条 AST 规则 panic。

### 6.5 它如何喂给引擎
一个新 provider（第 8 节）遍历文件、运行每条已编译规则，并为每个命中构建：
```rust
QualityIssue::new_capped(def.rule_id, def.rule_type, def.severity,
    AnalyzerSource::CustomRule, msg).with_location(file, line)
```
（注意：是 `new_capped` + `CustomRule`，**而非** `console_usage.rs` 的 `::new` + `Other("built-in")` 模式——封顶正是 D3 的全部要点。）随后它把计数聚合进两个新的 `MetricKey`。按引擎的解耦，**单凭 issues 永远不会触发门禁——只有被某个条件引用的 metrics 才会**（`engine.rs` 聚合；门禁条件只读 `metrics` map）。因此强制执行要求发布一个数值计数指标，与 `builtin_rust_critical`（`builtin_rust.rs:117-147`）如出一辙。

---

## 7. 特性规格

### 7.1 指标/规则提示气泡（"!" + 当前值——D7）
- 用 `info: MetricInfo[]` 丰富 `MetricCatalogResponse`（`quality.rs:198`），其中 `MetricInfo { key, displayName, description, example, higherIsWorse }` 是一张**编译期静态表**，每个可选 `MetricKey` 一条。（在两次部署之间保持静态 → `staleTime` 设为 1 小时即可。）
- 新增 `GET /api/projects/{id}/quality-metrics/latest` → `ProjectMetricSnapshot { values: Record<MetricKey, MeasureValue>, runId, ranAt }`，数据源自**最新的已持久化 `quality_run.report_json`**（已持久化；复用现有的运行）。**D7：悬停时不要触发一次全新的、昂贵的重新计算**——提示气泡标注为"截至 `<ranAt>`"，并在可为空时优雅降级为"尚无运行"。
- 在 `QualityGateRulesEditor.tsx` 中，新增一个可选的 `metricInfo?: MetricInfo[]` prop，穿线方式与 `metricOptions` 完全一致。在 Metric `<select>` 旁边（174-194 行）渲染一个带圈 "!" 按钮。悬停/聚焦时弹出一个 popover：displayName、description、example、当前项目值及其 `ranAt` 时间戳。
- 对于**自定义**规则，同一个 "!" 显示存储在 `custom_rule` 上、由 LLM 生成的 `description`——这样生成的规则在应用**之后**仍保持自文档化（用户计划第 2 步）。

### 7.2 NL→规则生成——双来源、全局、用户可选的引擎（D1）

**创作引擎是所有全局配置的 LLM 来源，由用户自行选择。** 用户通过现有的模型选择 UI 挑选用哪个已配置来源来创作规则；存在**一个"创作模型 invoker"抽象**（§8.6），它配备**两个后端，二者均复用现有基础设施**。

**对此前草稿以及"PTY"前提的关键更正。** 现有的交互式传输**并不**为单轮驱动一个 PTY，创作这一轮也绝不可以。它使用 `tokio::process::Command`，且 stdin/stdout/stderr = `null`（一个 piped 的一次性调用，`container.rs:1064-1073`）：被关闭的 stdin 使货真价实的 `claude` 恰好运行一轮，在写出磁盘上的转录 JSONL 后以 RC=0 退出；助手回复是通过**对该转录文件做 tail** 读取的，绝不从 stdout 读取。另一套工作流终端 `-p` PTY 机制（`terminal/process.rs ProcessManager::spawn_pty_with_config`）**不是**原生 OAuth 传输所复用的东西，**也不**在此处使用。创作流水线复用的是 **piped 一次性调用 + 转录 tailer** 这道接缝。

**入口点——每个门禁头部一个"用 AI 生成规则"。** 门禁头部（`QualityGateRulesEditor.tsx:142-154`，紧挨"Add condition"）打开新的 `RuleAuthoringDialog.tsx`。一个 textarea 捕获 NL 请求；对话框把 `nlRequest` + `currentRulesContext`（实时的 `value` 条件）**外加用户选择的 `modelConfigId`（+ `cliTypeId`）**POST 到 `POST /api/projects/{id}/custom-rules/author`。

**模型选择——复用现有的选择器（D1、D9）。** 该对话框提起（lift）`CreateChatBox.tsx` 的 `ToolbarDropdown` 选择器（143-201 行），由 `useModelConfigForExecutor(executor, workflowModelLibrary)`（`useModelConfigForExecutor.ts:56`）喂数据。那个 hook 已经枚举出每个 CLI 下**所有**全局配置的模型（经由 `useModelsForCli` → `GET /api/cli_types/:cli/models`），**合并**自定义（来自 `workflow_model_library`）+ 官方（DB 中 `isOfficial && hasApiKey` 的行），把每一项标记为 `ModelOption{ id, displayName, subtitle, isCustom, hasApiKey }`，自动选中，并暴露 `selectedModelConfigId`/`setSelectedModelConfigId`。该对话框绑定 `selectedId`/`onChange` 并提交所选的 `model_config_id`。**无需任何新的枚举或选择器代码。**

**后端分派——两个被复用的 invoker 后端（服务端按所选来源解析）：**

1. **计量的 API-key 来源 → 现有的 `LLMClient`。** 通过 `ModelConfig::resolve_preferred_or_default(pool, Some(chosen_id), cli_type_id)`（`cli_type.rs:387`）解析用户显式选择的配置。如果解析出的行带有 key，则通过 `ModelConfig::get_api_key()`（`cli_type.rs:186`）解密——**绝不**直接读取 `encrypted_api_key`/`orchestrator_api_key`（解密是自动的，AES-256-GCM，`crates/db/src/encryption.rs`）——经由 `OrchestratorConfig::from_workflow(api_type, base_url, api_key, model)`（`config.rs:290`）构建一个 `OrchestratorConfig`，然后调用 `create_llm_client(&config)`（`llm.rs:922`）。这条 HTTP 路径计量在用户自己的 key 上，并且**豁免**于订阅制额度池。**约束：** `OrchestratorConfig::validate()`（`config.rs:320-331`）把 `api_type` 白名单限定为 ∈ {openai, anthropic, openai-compatible, anthropic-compatible}——一个 `google` 的 `ModelConfig`（由 `Step3Models` 提供）在这里会被**拒绝**，无法经由此后端创作；要在选择器中把这一点呈现给用户（禁用/标注 google 行）。

2. **官方订阅 / 原生 OAuth 来源 → 现有的交互式传输（不走额度池）。** 当所选来源**没有 api key**（原生/订阅模型）时，invoker 通过**已经构建好、已经接线好的无 `-p` 交互式原生 OAuth 传输**驱动货真价实的 `claude` 二进制——既非计量 API，也非 PTY，更不走 `-p`/Agent-SDK 池。两个复用层级，挑选流水线能用的最高那一层：

   - **最高（当一对 Workspace+Session 代价不高时优先）：** 用一个 `CodingAgentInitialRequest { prompt = 规则创作指令, executor_profile_id (ClaudeCode), working_dir, allow_user_questions: false }`（`coding_agent_initial.rs:18`）调用 trait 方法 `ContainerService::start_execution(workspace, session, &ExecutorAction, &run_reason)`（实现于 `container.rs` → `start_execution_inner`，`container.rs:2108`）。路由器经由 `try_spawn_interactive_native_oauth`（`container.rs:1468`）在 `-p` 路径**之前**自动选中原生 OAuth；services 层的 `start_execution`（`services/container.rs:1046`）运行那一遍 `normalize_logs` + `spawn_stream_raw_logs_to_db`；助手回复由 `extract_last_assistant_message`（`container.rs:1201`）捕获进 `coding_agent_turn.summary`。流水线从 summary 读取回复（或订阅 `MsgStore` 以获取流式输出）。
   - **最低（当一个 Workspace 太重时）：** 逐字复刻 `crates/local-deployment/tests/interactive_transport_smoke.rs` 中的标准配方：`cc_switch::create_interactive_isolated_home(None, &wd)`（`cc_switch.rs:544`）→ `cc_switch::setup_interactive_auth(&home, /*api_key*/ None, /*base_url*/ None, &model, /*native_src*/ ~/.claude)`（`cc_switch.rs:707`，它会复制 `~/.claude/.credentials.json` 并清除 `ANTHROPIC_API_KEY/AUTH_TOKEN/BASE_URL/CLAUDE_CODE_OAUTH_TOKEN`，使该运行留在订阅计划上并远离计量 API）→ 构建 `ClaudeCode { interactive: Some(true), interactive_session_id: Some(home.session_uuid), model, .. }.build_interactive_command_parts()`（`claude.rs:204`，它发出 `--session-id <uuid>`，**没有** `-p`）→ `.into_resolved().await` → `LocalContainerService::spawn_interactive_claude(...)`（`container.rs:1044`，piped 一次性调用，**不是** PTY）→ `add_child_to_store`（`container.rs:182`）→ 等待 `LogMsg::Finished` → `ClaudeCode::normalize_logs`（`claude.rs:336`）+ 扫描历史中的 `assistant_message`（smoke 测试的 `assistant_texts()` 辅助函数，或复用 `extract_last_assistant_message`）→ **至关重要**地在会话结束时调用 `ProcessManager::cleanup_logical_session_home`（`process.rs:434`）以移除那个承载凭据的交互式 home（RB-37），因为这条底层接缝没有 Workspace 可以驱动 `cleanup_workspace`。

**订阅后端的计费保证。** 为了强制走免费的订阅路径，invoker 向 `InteractiveAuthMode::resolve`（`cc_switch.rs:597` → `NativeOauth`）传入 `api_key = None`/`base_url = None`，确保 `~/.claude/.credentials.json` 存在，并且**绝不设置 `SOLODAWN_NO_POOL`**（那会强制走计量的 `-p` 后备）。因为是*用户显式选择*了来源，`container.rs:1539-1564` 处的优先级歧义（一个穿透下来的 `config_id=None` 可能会给一个同时还存有 key 的订阅用户错误计费）不会发生——选择器总是提交一个显式 id，而无 key 的哨兵正是选中原生路径的依据。

**成本姿态。** 计量来源动用用户自己的 key（豁免于池的 HTTP）；订阅来源运行在用户的官方计划上，**零**计量 API / 额度池成本。如果用户选择了一个没有可用凭据的来源 → `ApiError::BadRequest`，带一条可操作的提示消息。两个后端之间**不存在**任何静默回退——所选来源的模式（key 还是 native）确定性地选定后端。

### 7.3 校验流水线（具名步骤）
模块 `crates/services/src/services/rule_authoring/`。`const MAX_AUTHORING_ROUNDS: usize = 4;`（镜像 `FINAL_REPAIR_MAX_ROUNDS`，`agent.rs:115`；D6——固定，可配置性延后）。每个阶段都是一个自由的、会 fail-close 的 `async fn(&dyn LLMClient, ...) -> StageResult`（即 `audit_plan.rs:22-70` 模板）；JSON 通过 `extract_json_from_mixed_response`（`agent.rs:5511`）+ `normalize_instruction_payload`（`agent.rs:5496`）解析。**因为每个阶段都接收 `&dyn LLMClient`，同一条流水线对两个 invoker 后端原封不动地运行**——计量的 `create_llm_client` box 与订阅制的 `InteractiveClaudeClient` box 是同一个 trait 对象（这正是让 D1 近乎免费的关键复用；唯一的调用点决策是构建哪个 box）。

| 步骤 | 智能体 | 模块函数 | 它做什么 |
|---|---|---|---|
| 1 GENERATE | **Proposer** | `generate::draft_rule` | 从 NL + 当前正在编辑的规则，产出 `CandidateRule {rule_format, rule_body, description, rule_type, severity, mapped_metric}` **外加 2-3 个正例（必须命中）+ 2-3 个负例（必须不命中）**——产出示例是必需的。fail-closed 默认值 = 一条被标记为无效的最小空操作规则。 |
| 2 ADVERSARIAL REVIEW | **Adversary** | `adversary::attack` | 用一个迥异的 system prompt 去*打破*规则：(a) 误报片段（过度命中，例如内联 `#[cfg(test)]`——即真实的 `test_file_absence` 缺陷），(b) 满足意图却绕过 pattern 的规避片段，(c) 歧义/作用域方面的诉求。它的片段被**作为永久夹具追加**。 |
| 3 EMPIRICAL TEST | *（确定性，无 LLM）* | `empirical_test::evaluate` | 编译候选规则，并通过共享的 `quality::run_candidate(compiled_rule, snippet, virtual_path)` 把它**执行**到每个正例/负例/误报/规避片段上。通过 = 所有正例被命中、所有负例 + 误报片段干净、所有规避片段被命中。**具最终权威。** |
| 4 JUDGE | **Judge** | `judge::score` | 形如 `AuditScoreResult`（意图保真度、精确率/无过度命中、召回率/无规避、清晰度）对比 `PASS_THRESHOLD`（`types.rs:624`）。一份不及格的实证报告**强制** `passed=false`，无论 LLM 意见如何。若 `!passed` → `generate::revise(...)` 并循环。 |
| — CAP | — | — | 若循环耗尽 `MAX_AUTHORING_ROUNDS`（=4）→ 返回 `outcome=capped_out`，带上最佳候选 + 所有对话记录（**不** panic——交还给用户；D6）。 |
| 5 USER CONFIRM（强制） | *（人工）* | UI | 完整的 `AuthorRuleResult` 在 `RuleAuthoringDialog` 中展示，并**必须**被确认——确认绝不可选（D2）。**在 Confirm 之前不持久化任何东西。** 确认后 → 写入一行 `status='shadow'` 的 `custom_rule`。 |
| 6 CONTEXT-FREE REVERSE-ENGINEER | **Interpreter** | `reverse_engineer::interpret` | 一次**全新的** `LLMClient` 调用，其全部输入是已确认的 `rule_body` + `description`——**没有** nl_request、**没有**辩论、**没有**示例。输出：`ReconstructedRequest`。 |
| 7 JUDGE-COMPARE | **Matcher** | `reverse_engineer::compare` | 显式的裁判裁决：重建结果在语义上是否与原始请求匹配？`RoundTripVerdict {judgePassed, judgeScore, reconstructedRequest, rationale}`。**匹配** → 持久化 `custom_rule_validation{verdict='pass', roundtrip_ok=1}`；规则保持 shadow。**不匹配** → 携带"重建-对比-原始"的差量作为额外修复指令，重新进入步骤 1/2，计入上限；在到达上限后，持久化 `{verdict='fail', roundtrip_ok=0}` 并交还给用户（回退到手动）。 |

智能体阵容（全部是同一个 `&dyn LLMClient`，prompt 各不相同；步骤 6 使用一个无历史的全新消息向量）：Proposer → Adversary → [确定性的第 3 步] → Judge → [人工的第 5 步，强制] → Interpreter（无上下文）→ Matcher。`MockLLMClient`（`llm.rs:100-155`）确定性地单测每个阶段以及循环/上限。

### 7.4 确定性强制执行（无 AI）
每一次门禁运行，`resolve_quality_config`（`quality_policy.rs:26`）额外加载该项目已启用的 `custom_rule` 行，编译它们（P1：受 size 限制的正则；P2：解析后的 ast-grep YAML），并据此构造 `DeclarativeRuleProvider`。`QualityEngine::run` 以确定性且完全相同的方式执行它们。`custom_rule_critical > 0`（仅当项目选入 enforce 时）像任何内置指标一样触发门禁。该 provider 从不派生子进程，也从不调用 LLM。

### 7.5 自文档化提示气泡
Proposer 产出那段为 "!" 提示气泡提供动力的通俗语言 `description`，因此每条自定义规则都是自文档化的；提示气泡同时还显示该指标截至最新运行的当前项目值（7.1，D7）。内置指标描述来自静态编译目录；自定义规则描述存放在 DB 中。

### 7.6 编辑-重新校验策略（D8）
- 编辑一条规则的**本体**（匹配逻辑：`rule_body`、`rule_format`、`severity`、`mapped_metric`）会**重新运行完整的创作校验流水线**（§7.3 的步骤 1-7，经由 `POST .../{ruleId}/revalidate`），并把规则**退回到 `status='shadow'`**，直到它再次通过。同一条流水线，没有新的代码路径。
- **仅元数据的编辑**（`name`、`description` 文本）**跳过**重新校验且不改变状态——它们只 bump `version` 并写入一行 `custom_rule_audit`。
- 服务端通过把提交的 `CustomRuleInput` 与持久化的行做 diff 来决定走哪条路径：任何对本体字段的改动都会触发重新校验；仅限于 name/description 的改动则不会。

---

## 8. 架构与数据流

### 8.1 按 crate 划分的组件
- **`crates/quality`**——新增 `provider/declarative.rs`（`DeclarativeRuleProvider` 实现 `QualityProvider`）；共享 `run_candidate(compiled_rule, snippet, virtual_path) -> Vec<QualityIssue>`；新增 `AnalyzerSource::CustomRule`；新增 `MetricKey::{CustomRuleViolations, CustomRuleCritical}`；`ProvidersConfig.declarative_rules` 开关；`build_providers` 注册。**P1 不新增 Cargo 依赖**（regex 已存在，`Cargo.toml:25`）；**P2 加入 ast-grep crates。**
- **`crates/services`**——新增同级模块 `services/rule_authoring/`（`mod.rs`、`generate.rs`、`adversary.rs`、`empirical_test.rs`、`judge.rs`、`reverse_engineer.rs`），注册到 `services/mod.rs`；外加那个轻薄的**创作模型 invoker**（§8.6）。`quality_policy.rs` 扩展为加载 + 编译自定义规则。
- **`crates/server`**——用提示气泡目录、当前值端点、自定义规则 CRUD，以及创作/重新校验路由扩展 `routes/quality.rs`（author 路由接收所选的 `model_config_id` + `cli_type_id`）。
- **`crates/local-deployment` / `crates/executors`**——**无改动**；订阅制 invoker 后端*原样复用*现有的 `ContainerService::start_execution` / `try_spawn_interactive_native_oauth` / `spawn_interactive_claude` 以及 `ClaudeCode` 交互式 argv 构建器。
- **`crates/db`**——迁移 `20260620120000_create_custom_rules.sql` + 4 个模型 + `mod.rs` 注册。
- **`frontend`**——在 `QualityGateRulesEditor.tsx` 中加入 `metricInfo` prop + "!" popover + 自定义规则区；新增 `RuleAuthoringDialog.tsx`（提起 `CreateChatBox` 模型选择器 + `useModelConfigForExecutor`）；扩展 G2 `QualityGateConfirmDialog.tsx`；`useQualityPolicy.ts` 的 hooks。

### 8.2 Provider 契约（新 `DeclarativeRuleProvider`）
- 名称 `"declarative-rules"`。在构造时接收一个由已编译自定义规则定义组成的 `Vec`（quality crate 保持**无 DB**——这是已验证的 G3 边界）。
- 通过 `analysis::collect_files`（`analysis/mod.rs:33`）+ `analysis::is_excluded`（`analysis/mod.rs:21`）（即 `builtin_common.rs:90-135` 模板）遍历文件。
- 对于 `regex` 规则（P1）：逐行运行一个受 size 限制的已编译 `Regex`（即 `console_usage.rs` 模板，但用 `new_capped` + `CustomRule`）。对于 `ast_grep` 规则（P2）：按文件扩展名选定捆绑的 Rust/TS 语法，在其上运行 `ast-grep-core`。
- 把每个命中映射到 `QualityIssue::new_capped(...)`（`issue.rs:105`），并把计数聚合进两个新的 MetricKey（即 `builtin_rust.rs:117-150` 发布模板）。
- 当加载了零条规则时，`applicable_metrics()` 返回 `Vec::new()`（即 `rust_analyzer.rs` 的"不适用即为空"模式），因此空规则集永不 fail-close，也不会对无关仓库误报。
- `analyze()` 被包裹在 `tokio::time::timeout` 中；在 Enforce 模式下超时会发出一个 Blocker（fail-closed，与空扫描分支 `engine.rs:317` 一致）。一个 provider 的 `Err` 会降级为一份无指标的失败报告 → fail-closed；当"没有规则运行"应当无害时返回 `Ok` + 哨兵。

### 8.3 严重度权威（D3）
新增 `AnalyzerSource::CustomRule`（`rule.rs:158-178`），在**穷尽式**的 `severity_origin()` match（`rule.rs:205`）中映射到 `SeverityOrigin::ProjectConfig`——因此 LLM/用户创作的严重度会被 `cap_for_advisory`（`rule.rs:124`）封顶到 `Major`（非阻断），这与已经为 `EsLint` 编码的"模型口味"论点（`rule.rs:208`）完全一致。被钉死的 `severity_origin_classification_is_pinned` 测试（`rule.rs:300-358`）以及 `cap_routes_through_severity_origin` 测试（`rule.rs:332`）必须为这个新变体进行扩展。门禁**只**通过操作者主动选入的显式 `CustomRuleCritical` 计数指标发生——**绝不**通过自我声明的 `Blocker`。复用现有的封顶 + 指标-条件机制；不引入任何新的升级路径。

### 8.4 文字流程——创作（双来源）
```
UI NL request + 用户选择的模型（useModelConfigForExecutor 选择器）
  -> POST /api/projects/{id}/custom-rules/author { nlRequest, modelConfigId, cliTypeId, currentRulesContext? }
  -> 创作模型 invoker 解析所选来源：
       若它有 key  -> resolve_preferred_or_default(Some(id),cli) -> get_api_key
                           -> OrchestratorConfig::from_workflow -> create_llm_client      (计量，豁免于池)
       若它没有 key -> 订阅制原生 OAuth 交互式传输（不是 PTY，不走池）：
                           ContainerService::start_execution(ClaudeCode CodingAgentInitialRequest)
                           [或底层：create_interactive_isolated_home -> setup_interactive_auth(None,None,..)
                            -> build_interactive_command_parts -> spawn_interactive_claude -> tail 转录
                            -> normalize_logs -> 提取助手回复 -> cleanup_logical_session_home]
  -> 两者都产出一个 Box<dyn LLMClient>；rule_authoring::author_rules 原封不动地运行：
       Proposer -> [循环：Adversary -> empirical(run_candidate) -> Judge -> revise] (cap = MAX_AUTHORING_ROUNDS = 4)
  -> AuthorRuleResult {candidate, examples, empirical, debate, roundTrip, outcome, roundsUsed}
  -> UI 展示双智能体对话记录 + 实证证据
  -> 用户 Confirm（强制）
  -> POST .../custom-rules (持久化) at status=shadow
  -> Interpreter（全新，无上下文）-> Matcher（裁判比对）
  -> 持久化 custom_rule + _example + _validation + _audit
```

### 8.5 文字流程——强制执行（无 AI）
```
gate run
  -> resolve_quality_config (DB priority-0) loads enabled custom_rule rows
  -> compile (P1 size-limited regex / P2 parsed ast-grep YAML) + construct DeclarativeRuleProvider
  -> QualityEngine::run executes providers concurrently (deterministic)
  -> CustomRuleViolations / CustomRuleCritical published into metrics map
  -> if project opted into enforce: condition `custom_rule_critical GT 0` gates like any builtin metric
```

### 8.6 创作模型 invoker（横跨两个后端的唯一新抽象——D1）
一道轻薄的服务端接缝——`rule_authoring::invoker`——它把用户选择的来源转化为一个 `Box<dyn LLMClient>`，从而让整条 `rule_authoring` 流水线保持后端无关（`&dyn LLMClient`）。它**不**包装一个新传输；它*在两个现有传输之间做选择*：

```
fn build_authoring_client(pool, project_id, model_config_id, cli_type_id)
    -> Result<Box<dyn LLMClient>, ApiError>
  let cfg = ModelConfig::resolve_preferred_or_default(pool, Some(model_config_id), cli_type_id)?; // cli_type.rs:387
  match InteractiveAuthMode::resolve(cfg.get_api_key()?.as_deref(), cfg.base_url.as_deref()) {     // cc_switch.rs:597
    OfficialKey | Relay  => {                 // 计量，豁免于池的 HTTP
        let oc = OrchestratorConfig::from_workflow(cfg.api_type, cfg.base_url, key, cfg.api_model_id); // config.rs:290
        create_llm_client(&oc)                // llm.rs:922  (validate() 拒绝 api_type=google)
    }
    NativeOauth          => {                 // 订阅制，不走额度池，不是 PTY
        // 要么驱动 ContainerService::start_execution（优先）并把它的 summary 作为回复来适配，
        // 要么使用已经构建好的 create_interactive_claude_client(model)（llm.rs:868），它本身
        // 返回一个针对货真价实的 claude 二进制实现 LLMClient 的 InteractiveClaudeClient（llm.rs:628）。
        create_interactive_claude_client(&cfg.api_model_id)
            .ok_or(ApiError::BadRequest("no native subscription credentials"))?
    }
  }
```

两个后端，皆为复用：(a) 经由 `create_llm_client` 的计量 `LLMClient`；(b) 经由现有的 `create_interactive_claude_client`（`llm.rs:868`，它构造已经构建好的 `InteractiveClaudeClient`，`llm.rs:628`）的订阅制原生 OAuth 交互式传输——或者，为了完整的生命周期，用 `ContainerService::start_execution`。判别器是 `InteractiveAuthMode::resolve`（`cc_switch.rs:597`）；用户显式的 `model_config_id` 避开了 `container.rs:1539-1564` 的错误计费穿透。

> **rev2 复用标注（D9）：** 此前的草稿（§7.2）把交互式订阅路径当作一个"应避免的后备"。那**低估并未充分复用**一个既有的、与 trait 兼容的运行器：`InteractiveClaudeClient`（`llm.rs:628`）已经实现了与 `create_llm_client` 相同的 `LLMClient` trait，已经在 `planning_drafts.rs:375/587` 和 `agent.rs:199` 被消费，并且运行在用户的官方订阅上、**不走**额度池。rev2 把它提升为一个一等的、用户可选的创作后端。**没有任何新传输需要构建。**

---

## Reuse Map（既有复用 vs 新胶水——D9）

两列：什么已经存在并被原样复用（file:line 已验证），对比那最小的净新增胶水。指导原则是"优先既有组件；标注任何此前提议要构建、却已经存在的东西"。

| 关注点 | REUSE（既有，file:line） | NEW（最小胶水） |
|---|---|---|
| **计量 LLM 调用** | `LLMClient` trait `llm.rs:23`；`create_llm_client` `llm.rs:922`；`OrchestratorConfig::from_workflow` `config.rs:290`；`validate()` 白名单 `config.rs:320` | 仅调用点（从所选 `ModelConfig` 构建 `OrchestratorConfig`） |
| **订阅制 LLM 调用（不走池）**——*曾被标注为"构建一个 PTY 运行器"；已经存在* | `InteractiveClaudeClient` 实现 `LLMClient` `llm.rs:628-703/:706`；`create_interactive_claude_client` `llm.rs:868`；`ContainerService::start_execution` → `start_execution_inner` `container.rs:2108`；`try_spawn_interactive_native_oauth` `container.rs:1468`；`spawn_interactive_claude` `container.rs:1044`；`create_interactive_isolated_home` `cc_switch.rs:544`；`setup_interactive_auth` `cc_switch.rs:707`；`InteractiveAuthMode::resolve` `cc_switch.rs:597`；`build_interactive_command_parts` `claude.rs:204`；`normalize_logs` `claude.rs:336`；`extract_last_assistant_message` `container.rs:1201`；smoke 配方 `interactive_transport_smoke.rs`；清理 `cleanup_logical_session_home` `process.rs:434` | `build_authoring_client` 分派（§8.6）+（若走底层）一次显式的 `cleanup_logical_session_home` 调用 |
| **创作模型 invoker** | 上述两个后端；判别器 `cc_switch.rs:597` | 一道轻薄的 `invoker` 函数（§8.6），按来源选定后端 |
| **模型枚举** | `GET /api/cli_types/:cli/models` `cli_types.rs:156`；`ModelConfig::find_by_cli_type` `cli_type.rs:205`；`find_user_configured`/`find_all` `cli_type.rs:309/345`；FE `useModelsForCli` `useCliTypes.ts:155` | 无 |
| **模型选择器 UI** | `useModelConfigForExecutor` `useModelConfigForExecutor.ts:56`；`ToolbarDropdown` Custom/Official 分区 `CreateChatBox.tsx:143-201`；原生哨兵 `NATIVE_MODEL_ID` `workflow/types.ts:137` | 把提起的下拉绑定进 `RuleAuthoringDialog` |
| **凭据解析** | `ModelConfig::get_api_key` `cli_type.rs:186`；`resolve_preferred_or_default` `cli_type.rs:387`；`find_with_credentials_for_cli` `cli_type.rs:401`；`Workflow::get_api_key` `workflow.rs:206`；AES-256-GCM `db/src/encryption.rs` | 无（始终传入显式的 `model_config_id`） |
| **创作阶段** | fail-closed 模板 `audit_plan.rs:22-70`；`extract_json_from_mixed_response` `agent.rs:5511`；`normalize_instruction_payload` `agent.rs:5496`；`AuditScoreResult`/`::parse`/`PASS_THRESHOLD` `types.rs:557/631/624`；`MockLLMClient` `llm.rs:100-155` | `rule_authoring/{mod,generate,adversary,empirical_test,judge,reverse_engineer}.rs` + prompts + structs |
| **循环上限** | `FINAL_REPAIR_MAX_ROUNDS=4` `agent.rs:115`；守卫 `bail!` `agent.rs:8203` | `const MAX_AUTHORING_ROUNDS=4` 克隆（D6） |
| **规则 trait/类型** | `Rule`/`RustRule`/`TsRule`/`CommonRule` + `RuleConfig` `rules/mod.rs:18-144` | `DeclarativeRuleProvider` + `run_candidate` |
| **Issue 构建** | `QualityIssue::new_capped` `issue.rs:105`；`.with_location` `issue.rs:117` | 使用 `new_capped`+`CustomRule`（**而非** `console_usage.rs` 的 `::new`+`Other`） |
| **严重度封顶（D3）** | `cap_for_advisory` `rule.rs:124`；`SeverityOrigin` `rule.rs:146`；`AnalyzerSource` `rule.rs:158`；穷尽式 `severity_origin()` `rule.rs:205`；被钉死的测试 `rule.rs:300-358` | 1 个 `CustomRule` 变体 + 1 个 match 臂 + 扩展被钉死的测试 |
| **指标/门禁** | `MetricKey` `metrics.rs:14`；`as_str`/`display_name` `metrics.rs:176/226`；门禁 `Operator(Gt/Lt)`/`Condition` `gate/condition.rs:18`；`selectable_metric_keys` `quality.rs:207`；`MetricCatalogResponse` `quality.rs:198`；`get_metric_catalog` `quality.rs:277` | 2 个 `MetricKey` 变体（3 处耦合点 + selectable + ts 重新生成）+ `info` 字段 |
| **正则格式（P1）** | `regex="1"` 已是依赖 `Cargo.toml:25`；模板 `rules/typescript/console_usage.rs` | 受限正则 JSON schema + 受 size 限制的编译 |
| **AST 格式（P2）** | （尚无） | ast-grep crates + 加载器（在 schema 确认之后，§17） |
| **文件遍历** | `analysis::collect_files` `analysis/mod.rs:33`；`is_excluded` `analysis/mod.rs:21` | 无 |
| **指标发布** | `builtin_rust.rs:117-150` 模板 | 发布 2 个新指标 |
| **DB 模式** | Uuid/BLOB 模型+迁移 `project_quality_policy.rs:15-67`；`insert_batch` `quality_issue.rs:103-138`；迁移模板 `20260614120000_create_project_quality_policy.sql`；`models/mod.rs:25-39`；`SCHEMA_EXPECTATIONS` `lib.rs:31` | 迁移 `20260620120000`（4 张表）+ 4 个模型 + 注册 |
| **共享编辑器** | `QualityGateRulesEditor.tsx`（props 14-23；`addCondition`/`updateCondition` 73-90；门禁头部 142-154；metric select 174-194；原始 `text-slate-*` 类） | `metricInfo` prop + "!" popover + 自定义规则区 |
| **确认对话框（D2）** | G2 `QualityGateConfirmDialog.tsx`（编辑器渲染 177-184；Save&Confirm 111-141）；硬阻断 `planning_drafts.rs:976`（handler `:436`，mount `:162`） | 追加一个只读的 规则/示例/实证/往返 面板；保持强制 |
| **查询/变更** | `useQualityPolicy.ts` keys 9-14 + hooks 39-58；`lib/api.ts` `qualityPolicyApi` 674-704 + `makeRequest`/`handleApiResponse` 119/248 | `useGenerateRule` + `useCustomRules` + `customRules` key |
| **ts-rs 导出** | `MetricKey` `#[derive(TS)]` `metrics.rs:12`；workspace 依赖 `Cargo.toml:23` | 对新 DTO 加 `#[derive(TS)]` + 重新生成 |

**显式的"已经存在，请勿构建"标注（D9）：**
1. **订阅/原生交互式运行器**——已经构建好且与 trait 兼容（`InteractiveClaudeClient` `llm.rs:628`/`:868`）；此前的 §7.2 未充分复用它。请勿构建 PTY 运行器或任何新传输。
2. **regex 1.x**——已经是依赖（`Cargo.toml:25`）；请勿新增。（ast-grep 确实是新的，仅限 P2。）
3. **模型枚举 + 选择器**——已经存在（`useModelConfigForExecutor` + `CreateChatBox` 下拉 + `GET /api/cli_types/:cli/models`）；提起复用，请勿重建。
4. **ts-rs 导出装置**——已经接线（`MetricKey`）；只有新 DTO 需要 `#[derive(TS)]` + 重新生成。
5. **G2 确认对话框 + 硬阻断**——已经存在；扩展，绝不替换（`gates_confirmed_at` 阻断在 `planning_drafts.rs:976`）。
6. **不存在任何预先存在的 `custom_rule`/`rule_authoring`/声明式脚手架**——已在 `crates/**/*.rs` 中确认缺失，因此其余 NEW 项确属净新增，而非重复。

**带入 rev2 的细微引用漂移更正：**
- §11 样式：`QualityGateRulesEditor.tsx` 使用原始的 `text-slate-*`/`bg-white` Tailwind，**而非** `.new-design` tokens；只有 `ConfirmDialog` 外壳使用 ui-new tokens。共享编辑器内部的新 UI 应当匹配它现有的 slate 类。
- `api_type` 白名单（`config.rs:320-331`）排除了 `google`；一个 Google 的 `ModelConfig`（由 `Step3Models` 提供）无法流经 `create_llm_client` 进行创作——在选择器中标注/禁用它。

---

## 9. 数据模型与 Schema

**一个增量迁移** `crates/db/migrations/20260620120000_create_custom_rules.sql`——纯 `CREATE TABLE IF NOT EXISTS` + `CREATE INDEX`，**无重建、无 `PRAGMA foreign_keys` 切换**（这是承重的安全约束：sqlx 0.8.6 把每个迁移包进一个隐式事务，其中裸的 `PRAGMA foreign_keys=OFF` 是一个静默的空操作）。**每个项目作用域的 FK 都是 BLOB**，以匹配 `projects.id` 的 BLOB 主键（即 `project_quality_policy.rs` 的 Uuid 模式——一个 TEXT 子 FK 对一个 BLOB 父表会静默失败，这个 bug 由 `20260202090000` 修复）。

**作用域说明（D4）：** `project_id` 在 schema 中保持**可空**（`NULL` = 全局/组织级规则），因此全局作用域是后续的增量步骤，但 v1 的 UI/特性把它当作必填项（`project_id NOT NULL` 在路由/handler 层强制，而非在列上）。全局作用域上线时无需 schema 变更。

### custom_rule
```
id BLOB PRIMARY KEY NOT NULL
project_id BLOB REFERENCES projects(id) ON DELETE CASCADE   -- NULL = global/org rule (schema-allowed; v1 UI requires non-null, D4)
name TEXT NOT NULL
nl_request TEXT NOT NULL                  -- original NL ask (round-trip compare + reproducibility)
rule_format TEXT NOT NULL CHECK (rule_format IN ('ast_grep','regex'))   -- P1 emits 'regex'; 'ast_grep' is P2 (D5)
rule_body TEXT NOT NULL                   -- regex+scope JSON (P1) or ast-grep YAML (P2)
description TEXT                           -- LLM-generated text powering the '!' tooltip
rule_type TEXT NOT NULL DEFAULT 'CodeSmell' CHECK (rule_type IN ('Bug','Vulnerability','CodeSmell','SecurityHotspot'))
severity TEXT NOT NULL DEFAULT 'MAJOR' CHECK (severity IN ('INFO','MINOR','MAJOR','CRITICAL','BLOCKER'))
mapped_metric TEXT                         -- MetricKey::as_str() token; free text, NOT an FK
enabled INTEGER NOT NULL DEFAULT 1
status TEXT NOT NULL DEFAULT 'shadow' CHECK (status IN ('draft','shadow','warn','enforce','disabled'))
created_by TEXT
version INTEGER NOT NULL DEFAULT 1
created_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
updated_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
UNIQUE(project_id, name)
indexes: idx_custom_rule_project(project_id); idx_custom_rule_enabled(project_id, enabled); idx_custom_rule_metric(mapped_metric)
```

### custom_rule_example（正确性甲骨文 / oracle）
```
id BLOB PK NOT NULL
rule_id BLOB NOT NULL REFERENCES custom_rule(id) ON DELETE CASCADE
kind TEXT NOT NULL CHECK (kind IN ('positive','negative'))   -- positive SHOULD flag; negative MUST NOT
language TEXT                                                 -- 'rust','typescript', NULL = agnostic
snippet TEXT NOT NULL
expected_match INTEGER NOT NULL                              -- 1 = rule expected to fire
note TEXT
created_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
index: idx_custom_rule_example_rule(rule_id, kind)
```

### custom_rule_validation（仅限创作期产物——切勿与 quality_run/quality_issue 混为一谈）
```
id BLOB PK NOT NULL
rule_id BLOB NOT NULL REFERENCES custom_rule(id) ON DELETE CASCADE
rule_version INTEGER NOT NULL
verdict TEXT NOT NULL CHECK (verdict IN ('pass','fail','error','pending'))
roundtrip_ok INTEGER                                         -- judge verdict on reconstructed-NL vs original (NULL until run)
judge_score REAL                                             -- AuditScoreResult-style total
examples_total INTEGER NOT NULL DEFAULT 0
examples_passed INTEGER NOT NULL DEFAULT 0
rounds_used INTEGER NOT NULL DEFAULT 0
results_json TEXT                                            -- per-example {example_id, expected, actual, matched_spans}; + adversary transcript
error_message TEXT
validated_by TEXT
created_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
index: idx_custom_rule_validation_rule(rule_id, created_at DESC)
```

### custom_rule_audit（仅追加，永不 UPDATE → 永不需要重建；故意无 FK，以便规则被删后历史仍存活）
```
id BLOB PK NOT NULL
rule_id BLOB NOT NULL
project_id BLOB
action TEXT NOT NULL CHECK (action IN ('create','update','enable','disable','delete','revalidate','promote'))
actor TEXT
from_version INTEGER
to_version INTEGER
diff_json TEXT
created_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
index: idx_custom_rule_audit_rule(rule_id, created_at DESC)
```

**Rust 模型：** 新文件 `crates/db/src/models/custom_rule.rs`（以及 `_example`/`_validation`/`_audit`），`#[derive(Debug,Clone,FromRow,Serialize,Deserialize,TS)]` `#[serde(rename_all="camelCase")]`，`Uuid` 主键 + `DateTime<Utc>`（即 `project_quality_policy.rs:15-67` 的 BLOB/Uuid 模式，**而非** `quality_run.rs` 的 String 模式）。CRUD：`find_by_project` / `find_enabled_by_project` / `upsert` / `set_enabled` / `delete` + 子表的 insert 辅助函数（即 `quality_issue.rs:103-138` 用于示例的 `insert_batch` 模式）。注册到 `models/mod.rs:25-39`（`pub mod` + `pub use`）。可选地把这些新的 NOT-NULL-带默认值的列加入 `SCHEMA_EXPECTATIONS`（`lib.rs:31-86`），用于 Windows 启动期自愈。

**未来重建提示：** 如果 `custom_rule` 将来需要删除某一列，使用已验证的 sqlx-逃逸三明治（`PRAGMA foreign_keys=OFF; COMMIT; BEGIN; ...12 步重建...; PRAGMA foreign_key_check; COMMIT; PRAGMA foreign_keys=ON; BEGIN;`）——绝不用裸 pragma。审计表是仅追加的，因此完全避开了这一点。

**保持不变：** `project_quality_policy` 中的 `QualityGateConfig` YAML 是一个并行存储；引用 `custom_rule_critical` 的门禁条件仍然住在 YAML 里，但规则本体/示例/描述住在 `custom_rule` 里。内置指标提示气泡描述是静态的（编译目录），不在 DB 中。

---

## 10. API 表面

全部包裹在 `ApiResponse<T>` 中，由 `handleApiResponse` 拆包；所有 DTO 都 `#[derive(TS)]` → 重新生成 `shared/types`（即 `generate_types --check` 门禁，`ci-basic.yml:112`）。路由扩展 `crates/server/src/routes/quality.rs`（挂载于 `/api/quality` + `/api/projects`，`mod.rs:169-171`）。

**现有（不变），复用：**
- `GET /api/quality/policy/default`、`GET /api/quality/policy/metrics`（`quality.rs:277`）、`GET/PUT/DELETE /api/projects/{id}/quality-policy`、`POST /api/planning-drafts/{draftId}/confirm-gates`（`planning_drafts.rs:162`）。
- **模型来源枚举（为创作选择器复用——D1）：** `GET /api/cli_types`（`cli_types.rs:131`）和 `GET /api/cli_types/{cliTypeId}/models` → `Vec<ModelConfig>`（`cli_types.rs:156`），每行携带 `hasApiKey`/`isOfficial`/`displayName`/`apiType`。FE 选择器经由 `useModelConfigForExecutor` 消费它们；**不新增任何枚举端点**——author 路由只是*接收所选的 `model_config_id` + `cli_type_id`*。

**扩展——提示气泡目录：**
```
GET /api/quality/policy/metrics
  -> MetricCatalogResponse { metrics: MetricKey[], operators: string[], info: MetricInfo[] }
  MetricInfo { key: MetricKey, displayName: string, description: string, example: string, higherIsWorse: boolean }
```

**新增——提示气泡的当前值（仅限最新的已持久化运行——D7）：**
```
GET /api/projects/{id}/quality-metrics/latest
  -> ProjectMetricSnapshot { values: Record<MetricKey, MeasureValue>, runId: string|null, ranAt: string|null }
  (reads the latest quality_run.report_json; never recomputes on hover)
```

**新增——自定义规则 CRUD：**
```
GET    /api/projects/{id}/custom-rules                 -> CustomRule[]
POST   /api/projects/{id}/custom-rules                 (CustomRuleInput) -> CustomRule
PUT    /api/projects/{id}/custom-rules/{ruleId}        (CustomRuleInput) -> CustomRule   (D8: body edit -> revalidate+shadow; metadata-only -> bump version + audit)
DELETE /api/projects/{id}/custom-rules/{ruleId}        -> ApiResponse<()>
PATCH  /api/projects/{id}/custom-rules/{ruleId}/status ({status}) -> CustomRule          (shadow->warn->enforce promotion)
GET    /api/projects/{id}/custom-rules/{ruleId}/validations -> CustomRuleValidation[]
```

**新增——AI 创作（核心所在；双来源——D1）：**
```
POST /api/projects/{id}/custom-rules/author
  Request  AuthorRuleRequest {
    nlRequest,
    modelConfigId: string,                  // user-selected source (from the reused picker)
    cliTypeId: string,                      // maps the source to its CLI roster
    ruleFormatPreference?: 'regex' | 'ast_grep',   // P1 honours 'regex'; 'ast_grep' available in P2 (D5)
    currentRulesContext?: ConditionConfig[]
  }
  Response AuthorRuleResult {
    candidate: CustomRuleDraft { ruleFormat, ruleBody, description, ruleType, severity, mappedMetric },
    examples: RuleExample[],                       // 2-3 positive + 2-3 negative
    empirical: EmpiricalReport { total, passed, perExample: ExampleResult[] },
    debate: AdversaryTranscript { proposerNotes, attackerFindings, revisions },
    roundTrip: RoundTripVerdict { reconstructedRequest, judgePassed, judgeScore, rationale },
    outcome: 'passed' | 'capped_out',
    roundsUsed: number,
    engine: { modelConfigId, backend: 'metered' | 'subscription' }   // echoes which invoker backend ran (D1)
  }
POST /api/projects/{id}/custom-rules/{ruleId}/revalidate -> CustomRuleValidation   (D8: full pipeline; drops to shadow)
```

**服务端来源解析（author/revalidate 路由——D1）：** 经由 `ModelConfig::resolve_preferred_or_default(pool, Some(modelConfigId), cliTypeId)`（`cli_type.rs:387`）解析；从 `InteractiveAuthMode::resolve(get_api_key()?, base_url)`（`cc_switch.rs:597`）挑选 invoker 后端：带 key → `create_llm_client`（计量，豁免于池；`validate()` 拒绝 `google`）；无 key/native → 订阅制交互式传输（不走额度池；绝不设置 `SOLODAWN_NO_POOL`）。始终传入**显式**的 `modelConfigId`，以避开 `container.rs:1539-1564` 的错误计费穿透。

**校验不变量（服务端，任何持久化之前）：**
- 任何门禁**条件**都流经 `QualityGateConfig.validate()`（`config.rs:354`）：operator ∈ {GT,LT}、阈值可解析、`MetricKey` 是一个编译期枚举变体（serde 在反序列化时拒绝未知值——一个坏的 metric 会在 validate 之前就返回 400）。
- 一条自定义规则会被**拒绝**持久化（400），除非：`rule_format`/`severity`/`rule_type` 通过 CHECK 枚举；（P1）正则在 `RegexBuilder.size_limit(1<<20).dfa_size_limit(1<<20)` 之内编译通过；**并且**实证夹具运行通过（每个正例命中 ≥1，每个负例命中 0）。这就是**准入门禁**。
- `mapped_metric` 若存在，必须是一个已知的 `MetricKey::as_str()` 标记。

**需新增/重新生成的 ts-rs 类型：** `MetricInfo`、`ProjectMetricSnapshot`、`CustomRule`、`CustomRuleInput`、`CustomRuleDraft`、`RuleExample`、`EmpiricalReport`、`ExampleResult`、`AuthorRuleRequest`、`AuthorRuleResult`、`AdversaryTranscript`、`RoundTripVerdict`、`CustomRuleValidation`。**注意：** `shared/types` 是 ts-rs 生成的——重新生成，绝不手工编辑。

---

## 11. UI/UX

以**唯一**那个共享的 `QualityGateRulesEditor.tsx`（props `value`/`defaults`/`metricOptions`/`onChange`/`readOnly`/`errors`）为锚点，它被 `RulesDialog`、`ConfirmDialog`、`SettingsNew` 共用。**样式更正（rev2）：** 共享编辑器使用原始的 `text-slate-*`/`bg-white` Tailwind，**而非** `.new-design` tokens——编辑器*内部*的新 UI 应当匹配它现有的 slate 类；只有 `ConfirmDialog` 外壳使用 ui-new/.new-design tokens。

1. **提示气泡（D7）**——新增可选的 `metricInfo?: MetricInfo[]`，像 `metricOptions` 一样穿线。在 Metric `<select>` 旁边（174-194 行）渲染一个带圈 "!" 按钮（复用已导入的 lucide `Info`/`AlertTriangle`）。Popover 展示 displayName、description、example，以及**截至 `<ranAt>`** 的当前项目值（来自 `GET /api/projects/{id}/quality-metrics/latest`，可为空 → "尚无运行"）；它绝不触发重新计算。对于自定义规则，展示存储的 `description`。
2. **生成规则的可供性，配合被复用的模型选择器（D1）**——每个门禁头部一个"用 AI 生成规则"按钮（142-154 行）打开 `RuleAuthoringDialog.tsx`：
   - **模型选择器：** 提起由 `useModelConfigForExecutor` 输出（`useModelConfigForExecutor.ts:56`）喂数据的 `CreateChatBox.tsx` `ToolbarDropdown`（143-201 行）——Custom/Official 分区、选中 id 上的 CheckIcon、`displayName` + `subtitle`。绑定 `selectedModelConfigId`/`setSelectedModelConfigId`；提交所选的 `model_config_id` + `cli_type_id`。标注/禁用 `google`-`apiType` 行（被 `validate()` 拒绝）；无 key 的 native 行路由到订阅后端。
   - NL textarea；把 `nlRequest` + `currentRulesContext`（实时 `value` 条件）+ 所选来源发送到 author 路由。
   - 运行中的步进器：Proposer → Adversary → Empirical → Judge（以及一个来自 `AuthorRuleResult.engine.backend` 的"subscription" vs "metered" 徽章）。
   - 结果渲染三个面板：(a) 候选规则 + 通俗语言描述 + "!" 提示气泡；(b) 实证证据表（正例/负例/误报片段，实际命中/不命中 对比 期望，不匹配处标红）；(c) 对抗辩论记录。若 `outcome=capped_out`，显示一条横幅"无法收敛——请手动编辑"，并预填最佳候选（D6 交还给用户）。
   - 主操作"确认并添加" → 持久化 `custom_rule`（status shadow）**并且**，当映射到某个门禁指标时，通过现有的 `addCondition`/`updateCondition` 辅助函数（73-90 行）把一个 `ConditionConfig` 拼接进编辑器。随后步骤 6/7 运行；一个"往返检查"徽章显示通过/失败，并带上重建出的请求。失败时，对话框返回到编辑步骤（即循环），而不是关闭。
   - 在 `useQualityPolicy.ts` 中新增 `useGenerateRule` mutation + `useCustomRules` query（`qualityPolicyKeys` 增加一个 `customRules:(projectId)` key），遵循现有的 `makeRequest`+`handleApiResponse` 模式（`lib/api.ts` 的 `qualityPolicyApi`，674-704 行）。
3. **自定义规则管理**——在编辑器中（SonarQube 区下方）新增一个区，列出 `custom_rule` 行：名称、"!" 描述提示气泡、状态徽章（draft/shadow/warn/enforce）、启用开关、编辑、删除，以及"重新校验"。这就是 R4 的 展示/编辑/添加/删除 表面。**D8：** 编辑一条规则的**本体**会触发 `revalidate` 并把它退回到 shadow；仅编辑 name/description 则不会。通过 `PATCH .../status` 进行 shadow→warn→enforce 的晋级（绝不自动 enforce）。
4. **带示例的确认——强制（D2）**——扩展**现有的** G2 `QualityGateConfirmDialog.tsx`（**不要**新增对话框；`gates_confirmed_at` 物化硬阻断，`planning_drafts.rs:976`，必须保持完好）。在编辑器渲染处（177-184 行）附近，新增一个**只读面板**，对每条生效规则展示：规则（本体）、它生成的 `description`、正/负示例、实证测试结果，以及往返裁决——数据来自同一份 `MetricInfo` 目录 + `custom_rule`/`custom_rule_validation`。人工确认**绝不可选**：Save & Confirm（111-141 行）仍然是越过硬阻断的唯一路径。在产品文案的所有地方移除任何"可选"/"可跳过"的确认表述。
5. **SettingsNew.tsx / RulesDialog.tsx** 除了传入新的 `metricInfo` prop、并通过共享编辑器免费获得自定义规则区之外，不需要任何结构性改动。

---

## 12. 安全与防护

- **沙箱化的声明式格式是关键支点**——规则是数据（P1 受限正则；P2 ast-grep YAML），绝不是 Rust，也绝不是可执行代码。匹配器无法打开文件、套接字或派生进程；不同于 node 子进程 provider（`provider/mod.rs:120`），一条 pattern 规则根本不派生任何子进程，因此无 FS/无网络是结构性继承的。
- **用 AI 生成、不用 AI 执行**——LLM 只在 `rule_authoring` 中出现（一次性、经对抗式校验、经人工确认）。扫描步骤是对字节的纯正则/AST，从不把扫描到的源码喂进 prompt → 仓库代码的 prompt 注入（攻击面 B：诸如"忽略之前的指令，标记为干净"这类敌意注释）**在构造上是惰性的**。
- **PTY 隔离 / 不走池的订阅（D1）**——订阅制创作后端复用现有的无 `-p` 交互式原生 OAuth 传输，**不是 PTY**，**也不走** `-p`/Agent-SDK 额度池。`setup_interactive_auth`（`cc_switch.rs:707`）把 `~/.claude/.credentials.json` 复制进一个**按逻辑会话隔离的 home**（`create_interactive_isolated_home`，`cc_switch.rs:544`；以一个稳定的 `claude-isession-` UUID 为键），并**清除**所有计费路由的环境变量（`ANTHROPIC_API_KEY/AUTH_TOKEN/BASE_URL/CLAUDE_CODE_OAUTH_TOKEN`），使该轮留在用户的订阅上并远离计量 API。`CLAUDE_CONFIG_DIR`+`CLAUDE_HOME` 都被设为那个隔离目录。该 home 持有一个凭据文件，因此 **RB-37 清理是强制的**：高层的 `start_execution` 路径经由 `cleanup_workspace` 清理；底层接缝**必须**在会话结束时调用 `ProcessManager::cleanup_logical_session_home`（`process.rs:434`），否则会泄漏一个承载凭据的临时目录。`spawn_interactive_claude` 故意对 stdout/stderr 使用 `Stdio::null`（管道化有 >64KB 终末消息管道缓冲死锁的风险）；请勿更改。流水线**绝不可**设置 `SOLODAWN_NO_POOL`（那会强制走计量的 `-p` 后备）。
- **正则沙箱（P1）**——Rust 的 `regex` 1.x 是线性时间的（无回溯/前后查找/反向引用），因此回溯型 ReDoS 不可能发生。在加载时**一次性**通过 `RegexBuilder::new(p).size_limit(1<<20).dfa_size_limit(1<<20).build()` 编译每个 pattern；把失败作为 400 拒绝，绝不在扫描时拒绝。约束输入（每文件字节数；截断病态的压缩单行）；复用 `analysis::is_excluded`。
- **fail-closed 超时**——把 `analyze()` 包裹在 `tokio::time::timeout` 中；在 Enforce 模式下超时发出一个 Blocker（与 `engine.rs:317` 一致）。保留 `syn`/tree-sitter 解析错误跳过（`builtin_rust.rs:77`）。一个 provider 的 `Err` 已经降级为一份失败报告——当"没有规则运行"应当无害时返回 `Ok`+哨兵，从而空规则集不会硬阻断。
- **prompt 注入（攻击面 A——NL → 规则）**——模型只返回符合严格 schema 的 DATA，在持久化**之前**由 serde + `RegexBuilder` 编译 + 实证夹具运行进行校验。模型**无法**铸造一个自我升级的 Blocker：`AnalyzerSource::CustomRule → SeverityOrigin::ProjectConfig`（`rule.rs:205`）把自定义严重度封顶到 `Major`（D3）；门禁仅通过显式的 `CustomRuleCritical` 计数指标主动选入。持久化的是规则（数据），而非自由文本 prompt，因此重跑可复现。**攻击面 A 同时适用于两个 invoker 后端**——一把计量 key 和订阅制传输都喂给同一套严格 schema + 准入门禁，因此两个后端都不会拓宽注入面。
- **Key 处理（D1）**——通过模型上的 `get_api_key()` 辅助方法（`cli_type.rs:186` / `workflow.rs:206`）复用用户已配置的凭据；绝不直接读取 `encrypted_api_key`/`orchestrator_api_key`（解密是自动的，AES-256-GCM）。对于计量来源，构建 `OrchestratorConfig`（validate() 把 `api_type` 列入白名单）并走 `create_llm_client`（计量，豁免于池）。对于订阅来源，根本不设置任何 key 环境变量（原生 OAuth）。始终传入**显式**的 `model_config_id`，使 `container.rs:1539-1564` 的优先级穿透无法给订阅用户错误计费。绝不把任何 key 存进 `system_settings`（明文 K/V）。
- **强制人工确认（D2）**——没有任何 AI 创作的规则会在用户未越过强制的 G2 确认对话框的情况下被持久化或晋级；`gates_confirmed_at` 硬阻断（`planning_drafts.rs:976`）仍然是物化的唯一路径。不存在任何"可选确认"的代码路径。
- **准入门禁**——拒绝持久化任何 未通过其自身正/负夹具，**或者** 命中了一个精心整理的已知干净语料库 的规则（这抓住了主导性失败模式——会把整个仓库误报的、过于宽泛的 LLM 正则）。

---

## 13. 测试策略

- **内联单元测试守护 CI。** quality crate **没有 `tests/` 目录**——每个 quality 测试都是内联 `#[cfg(test)]`，因此 `cargo nextest run --workspace ... --lib`（`ci-basic.yml:109` 后端门禁）会运行它们。把以下内容**内联**放进 `crates/quality`：规则编译、正则 `size_limit` 拒绝、超时 fail-closed、针对 `CustomRule` 的 `severity_origin`/`cap_for_advisory`、更新后被钉死的 `severity_origin` 测试（`rule.rs:300-358`），以及**实证正/负夹具测试**（镜像 `secret_detection.rs:209` 的 `analyze_text`——内存内、确定性、无 IO/时钟/网络）。
- **实证夹具作为准入门禁。** `run_candidate(compiled_rule, snippet, virtual_path)` 断言每个正例产生一个 issue、每个负例不产生任何 issue。这既门禁了一条新规则的接纳，又把它回归锁定。它抓住了过度命中这一失败模式，而这正是 LLM 生成正则的头号风险（例如把内联 `#[cfg(test)]` 标记出来）。
- **创作流水线测试。** 使用 `MockLLMClient`（`llm.rs:100-155`）确定性地单测每个阶段（`generate`/`adversary`/`empirical_test`/`judge`/`reverse_engineer`）以及循环/上限——验证 fail-closed 默认值、一份不及格的实证报告强制 `passed=false`、以及上限在 4 轮时返回 `capped_out`（而非 panic）。**因为两个 invoker 后端都解析为 `&dyn LLMClient`，流水线测试用一个 mock 就覆盖了两个后端**——无需任何传输 mock（计量/订阅的拆分在 `build_authoring_client` 分派边界单独演练）。
- **invoker 分派测试。** 单测 `build_authoring_client`（§8.6）：一个带 key 的 `ModelConfig` 选中计量后端；一个无 key/native 的配置选中订阅后端；一个 `google`-`apiType` 的配置被拒绝（`validate()`）；一个显式 id 绝不会穿透到 `find_with_credentials_for_cli` 的错误计费。订阅传输本身已经由 `crates/local-deployment/tests/interactive_transport_smoke.rs` 端到端覆盖（复用，请勿重复）。
- **集成测试在 nextest 下运行，而非 `cargo test`。** DB 持久化（通过新表的自定义规则 CRUD）、解析器加载已启用规则、编辑-重新校验退回到 shadow（D8），以及完整引擎运行，都放进 `crates/services/tests` / `crates/server/tests`，镜像 `quality_policy_resolver_test.rs`（内存 `SqlitePool` + `sqlx::migrate!("../db/migrations")`）和 `quality_gates_test.rs`（axum `oneshot`）。任何触及 `DeploymentImpl` 的测试**必须**在 nextest 下运行（按进程隔离）+ `#[serial]`——绝不用 `cargo test`（即 `6facb481a` 修复的迁移竞态：每个 `DeploymentImpl::new()` 都打开同一个 `db.sqlite`）。
- **CI 覆盖缺口（已标注）。** `--lib` 门禁运行内联测试，但**不**运行 `crates/*/tests` 集成测试。把实证夹具 + 编译/超时/严重度封顶测试**内联**（自动门禁）；对于 DB/端到端覆盖，要么在可行处添加内联，要么添加一个**不带** `--lib` 的独立 nextest 步骤。
- **CI 卫生。** 每个新的 `#[derive(TS)]` DTO 和新的 `MetricKey` 变体都需要把重新生成的 `shared/types` 提交，否则 `generate_types --check` 会失败（`ci-basic.yml:112`）；clippy 运行 `--all-targets --all-features`（`:90`），因此测试夹具必须 clippy-clean。用一个内存内测试验证 `custom_rule_example`/`_validation` 上的 CASCADE 确实触发（运行期连接设 `PRAGMA foreign_keys=ON`，`lib.rs:244/291`）。

---

## 14. 推出与分期

**P0 — 提示气泡（D7）**（纯前端 + 静态目录，零引擎风险，最先上线）：
- 用 `MetricInfo[]` 丰富 `MetricCatalogResponse`（每个可选 `MetricKey` 一条静态 描述/示例 表，`quality.rs:207`）；新增 `GET /api/projects/{id}/quality-metrics/latest`（读取**最新的**现有 `quality_run.report_json`；绝不重新计算——D7）。
- 加入 `metricInfo` prop + 带圈 "!" popover；穿线经过 `useQualityMetricKeys`。重新生成 ts-rs（`MetricInfo`、`ProjectMetricSnapshot`）。无 DB 迁移。直接解决"我看不懂它"。

**P1 — 受限正则声明式格式 + 确定性强制执行（D5：仅正则，零新增依赖）**（引擎的那一半；尚无 AI）：
- **不新增 Cargo 依赖**——复用 `regex = "1"`（`Cargo.toml:25`）。实现 `provider/declarative.rs`（即 `console_usage.rs` 模板，但用 `new_capped`+`CustomRule`）+ 共享 `run_candidate` + 内联实证夹具。**P1 不加入 ast-grep。**
- 上线**受限正则**的 `rule_body` 格式，并从创作路径发出 `rule_format='regex'`；保留 `rule_format` 判别项，以便 `ast_grep` 能在 P2 无迁移地加入。
- 加入 `AnalyzerSource::CustomRule → SeverityOrigin::ProjectConfig`（更新穷尽式 match + 被钉死的测试 `rule.rs:300-358`；D3）；加入 `MetricKey::{CustomRuleViolations, CustomRuleCritical}`（3 处耦合点 + `selectable_metric_keys` + ts-rs 重新生成）。加入 `ProvidersConfig.declarative_rules=false` + `build_providers` 注册，并使 `applicable_metrics` 在无规则时为空。
- 加入 4 张 `custom_rule` 表（迁移 `20260620120000`）+ db 模型 + CRUD 路由 + 自定义规则管理区。`resolve_quality_config` 加载已启用规则，并以 受 size 限制的编译 + tokio 超时 构造 provider。**UI 中作用域仅限项目（D4）**，尽管该列保持可空。
- **通过 `QualityGateMode`/每规则 `status` 推出：** 规则落地于 `shadow`（运行 + 记录，绝不阻断——已验证的引擎 shadow 路径），按项目晋级 shadow→warn→enforce；**仅在**夹具 + 干净语料库通过后才允许 enforce；严重度保持封顶。每个项目的策略每次运行都重新读取，因此晋级无需重新部署。

**P2 — NL 生成 + 对抗式校验 + ast-grep 结构性规则（D1、D5）**（AI 的那一半 + 结构性格式升级，最后，藏在如今已稳定的强制执行之后）：
- 构建 `crates/services/src/services/rule_authoring/` + 那个**双来源的创作模型 invoker**（§8.6），复用 `create_llm_client`（计量）**以及**已经构建好的订阅制交互式传输（`create_interactive_claude_client`/`ContainerService::start_execution`）——**没有新传输，没有 PTY**。复用 `AuditScoreResult` + 有界循环上限（`MAX_AUTHORING_ROUNDS=4`，D6）；用 `MockLLMClient` 单测每个阶段 + 循环 + 分派。
- 加入 `POST .../custom-rules/author`（接收所选的 `modelConfigId`+`cliTypeId`）+ `/revalidate`（D8：完整流水线 → shadow）。加入 `RuleAuthoringDialog.tsx`，提起 `CreateChatBox` 模型选择器 + `useModelConfigForExecutor`（D1）。接通无上下文逆向工程（步骤 6）+ 裁判比对（步骤 7）。
- 用**强制的**只读 规则/示例/实证/往返 面板扩展 G2 `QualityGateConfirmDialog`（D2）。
- **ast-grep 结构性格式：** 在**确认了未经验证的 ast-grep linter-envelope 字段名**（§17）之后，于现有的 `rule_format` 判别项之后加入 ast-grep MIT crates 和 `ast_grep` 加载器——增量、无迁移。这是 P2 的表达力升级（例如 handler 中的 `.unwrap()`），而非 P1 的依赖。

每一期都可独立发布：P0 交付即时的 UX 价值；P1 在没有任何 AI、也没有 ast-grep 的情况下可手动使用（正则规则）；P2 在一个已验证的确定性基底之上叠加 AI 创作（双来源）和结构性规则。一个 `ProvidersConfig.declarative_rules=false` 默认值 + 可选的 `SOLODAWN_NO_*` 环境变量 kill-switch 让该特性在就绪之前保持暗发状态。（注意：绝不从创作路径设置 `SOLODAWN_NO_POOL`——它会为订阅后端强制走计量的 `-p` 后备。）

---

## 15. 风险与缓解

**风险 1 — "AI 构建 AI 护栏"（核心张力）。** 用一个 LLM 去创作那条本应捕捉 LLM 错误的规则，有继承同样盲点的风险。**解决（设计内置）：** (a) 用 AI 生成 / 不用 AI 执行——AI 只触及创作；确认后的规则是确定性数据，永远以完全相同的方式运行；(b) 实证测试是**确定性真值**——它无法被"辩"出行为失败；(c) 逆向工程智能体被刻意设为**无上下文**，是一项真正独立的检查；(d) `CustomRule` 严重度被封顶到 `Major`（D3），因此一条坏规则无法自我升级到 Blocker；(e) **强制**人工确认（D2）+ 按项目 shadow→enforce 意味着在有人晋级之前，没有任何 AI 创作的规则会阻断交付。

**风险 2 — 两个意见一致的 LLM 收敛到错误答案。** **解决：** 步骤 2 是显式**对抗式**的（迥异的攻击者 prompt 产出误报/规避/歧义）；它的片段成为永久夹具，喂给确定性的步骤 3——分歧被强制制造并对照真值检查。

**风险 3 — 过于宽泛的生成正则把整个仓库误报**（即真实的 `test_file_absence`/内联-`#[cfg(test)]` 缺陷类）。**解决：** 准入门禁拒绝持久化，除非 负例 + 对抗者误报夹具通过 **并且** 一个精心整理的干净语料库保持干净；默认 Shadow 模式在任何门禁之前收集真实的误报率。

**风险 4 — DFA 内存膨胀 / 病态输入。** **解决：** 一次性编译时的 `size_limit`+`dfa_size_limit`、每文件/每行输入上限、tokio 超时 fail-closed。

**风险 5 — ast-grep 体积占用 + 未验证的 schema 字段（仅 P2——D5）。** **解决：** P1 上线零新增依赖的受限正则（`rule_format='regex'`），因此整个特性无需 ast-grep 即可使用；`rule_format` 判别项让 `ast_grep` 在 P2 增量地到来。**在编写 P2 加载器之前，对照锁定的 crate 版本确认 ast-grep YAML linter-envelope 字段名**（唯一剩余的技术性开放项，§17）。如果 AST 依赖将来被否决，后备正则 schema 完全可行。

**风险 6 — CI 覆盖缺口**（`ci-basic.yml` 后端门禁是 nextest `--lib`，它运行内联测试但不运行 `crates/*/tests`）。**解决：** 把实证夹具 + 编译/超时/严重度封顶测试**内联**；在可行处添加内联 DB 覆盖，或添加一个不带 `--lib` 的独立 nextest 步骤。`DeploymentImpl` 集成测试必须在 nextest + `#[serial]` 下运行，绝不用 `cargo test`。

**风险 7 — ts-rs 漂移。** 每个新的 `#[derive(TS)]` DTO 和新的 `MetricKey` 变体都需要把重新生成的 `shared/types` 提交，否则 `generate_types --check` 会失败；clippy `--all-targets` 意味着测试夹具必须 clippy-clean。

**风险 8 — 成本意外（仅计量后端——D1）。** 在一个**计量**来源上创作最多运行 `MAX_AUTHORING_ROUNDS × 若干次 LLM 调用`，计在用户的 key 上。**解决：** 把轮次上限定为 4（D6），显示 token 用量（`LLMResponse.usage`），并呈现每次运行的后端（`AuthorRuleResult.engine.backend`）。**订阅**后端（原生 OAuth）有**零**计量/额度池成本；选择它就是免费路径。把"所选来源没有可用凭据"呈现出来，而不是静默切换后端。

**风险 9 — 订阅后端错误计费 / 凭据泄漏（D1）。** 一个穿透下来的 resolve（`config_id=None`）进入 `find_with_credentials_for_cli`，可能会把一个同时还存有 key 的订阅用户路由到计量计费（`container.rs:1539-1564`）；而底层交互式接缝持有一个复制来的 `.credentials.json`。**解决：** 始终提交用户的**显式** `model_config_id`；绝不设置 `SOLODAWN_NO_POOL`；并在底层路径上**始终**于会话结束时调用 `ProcessManager::cleanup_logical_session_home`（`process.rs:434`）（高层的 `start_execution` 路径经由 `cleanup_workspace` 做这件事）。

**风险 10 — 订阅制 PTY/交互式并发（开放——§17）。** 交互式传输为每一轮针对一个按逻辑会话隔离的 home 派生一个真正的 `claude` 子进程；通过订阅后端的**并发**创作轮次的安全上限尚未设定。**解决（待定）：** 给并发的订阅创作运行设上限（例如一个小信号量），使并行的创作请求不会派生无界数量的 `claude` 子进程；计量运行是 HTTP，不受此约束。

---

## 16. 成功指标

- **可理解性：** ≥90% 的可选指标在编辑器中显示非空的 描述 + 示例；"!" popover 为每个指标渲染截至 `<ranAt>` 的当前项目值（或"尚无运行"）——不做任何重新计算（D7）。
- **创作产出率：** ≥70% 的 NL 请求在 `MAX_AUTHORING_ROUNDS`（=4）之内产出一个通过实证夹具运行的候选；`capped_out` 结果总是交还一个预填的最佳候选（绝非死胡同）。在**两个** invoker 后端上都成立（D1）。
- **往返完整性：** 在测试夹具中，无上下文的 Matcher 拒绝 ≥X% 的故意不匹配候选（在 P2 期间标定）。
- **强制执行安全：** 零个 AI 创作的规则在未通过其夹具 + 干净语料库检查**以及**强制人工确认（D2）的情况下到达 Enforce；默认 Shadow 误报率在晋级之前按项目可观测。
- **确定性：** 同一条已确认规则在多次门禁运行中产生逐字节相同的结果（由内联夹具回归锁定）。
- **成本完整性：** 订阅支撑的创作运行动用**零**计量/额度池成本；计量运行报告 token 用量；无静默后端切换（D1）。
- **非破坏性：** 在 `declarative_rules=false`（默认）下，现有门禁行为与上线前逐比特相同（由现有引擎测试保持绿色来验证）。

---

## 17. 给用户的开放问题

负责人的决策 D1–D9 现已落地。下列各项先把每个决策记录为 RESOLVED，然后列出唯一仍然真正开放的事项。

### 已解决（rev2 中已落地的决策）
- **D1 — 创作引擎（曾为 OQ-3）：RESOLVED。** 引擎是**所有全局配置的 LLM 来源，由用户自行选择**。一个**创作模型 invoker**（§8.6）配两个后端，二者均复用现有基础设施：(a) 计量的 API-key 来源 → 现有的 `create_llm_client`（`llm.rs:922`）；(b) 官方订阅 / 原生 OAuth 来源 → 现有的无 `-p` 交互式传输，驱动货真价实的 `claude` 二进制、不走额度池（`create_interactive_claude_client` `llm.rs:868` / `ContainerService::start_execution` `container.rs:2108`；**不是 PTY**）。用户通过被复用的 `useModelConfigForExecutor` + `CreateChatBox` 下拉选择来源。没有新传输，没有新选择器，没有新枚举。
- **D2 — 确认对话框（曾为 OQ-7）：RESOLVED——强制。** 二次校验确认**绝不可选**。用一个只读面板（规则、生成的描述、正/负示例、实证结果、往返裁决）扩展现有的 G2 `QualityGateConfirmDialog`；保留 `gates_confirmed_at` 硬阻断（`planning_drafts.rs:976`）。移除所有"可选确认"的表述。
- **D3 — 严重度（曾为 OQ-1）：RESOLVED——默认仅作建议。** `AnalyzerSource::CustomRule → SeverityOrigin::ProjectConfig`，被 `cap_for_advisory`（`rule.rs:124`）封顶到 `Major`；门禁**只**通过主动选入的 `CustomRuleCritical` 计数指标，绝不通过自我声明的 `Blocker`。复用现有的封顶 + 指标-条件机制。
- **D4 — 作用域（曾为 OQ-2）：RESOLVED——优先项目作用域。** `project_id` 在 v1 UI/特性中是必填的（在路由层强制），同时该列保持**可空**，以便全局/组织级作用域成为后续的、无迁移的增量步骤。
- **D5 — 规则格式分期（曾为 OQ-4）：RESOLVED——正则优先，ast-grep 随后。** P1 仅上线**受限正则格式**（零新增依赖；`regex = "1"` `Cargo.toml:25`；`console_usage.rs` 模板）。P2 在同一个 `rule_format` 判别项之后**加入** ast-grep AST 格式（MIT crates），**在**确认了未经验证的 ast-grep schema 字段名之后（见下文仍开放项）。
- **D6 — 创作轮次上限（曾为 OQ-5）：RESOLVED——固定为 4。** `const MAX_AUTHORING_ROUNDS = 4`（镜像 `FINAL_REPAIR_MAX_ROUNDS` `agent.rs:115`）；`capped_out` 交还给用户（不 panic）。可配置性延后。
- **D7 — 提示气泡当前值（曾为 OQ-6）：RESOLVED——最新的已持久化运行。** 读取最新的 `quality_run.report_json`，标注为"截至 `<ranAt>`"；悬停时**不**做全新的重新计算。
- **D8 — 编辑-重新校验（曾为 OQ-8）：RESOLVED。** 编辑一条规则的**本体**会重新运行完整的创作校验并把规则退回到 **shadow**；仅元数据的编辑（name/description）跳过重新校验（只 bump version + 审计）。复用同一条流水线。
- **D9 — 复用优先于重写（横切）：RESOLVED。** 新增了 **Reuse Map** 章节（既有复用 vs 新胶水），并标注了此前草稿提议要构建、却已经存在的一切（主要是订阅制交互式运行器、模型枚举/选择器、regex 依赖、ts-rs 装置，以及 G2 确认对话框）。净新增代码被最小化为：`rule_authoring/` + invoker、`provider/declarative.rs` + `run_candidate`、四张 DB 表 + 模型、三处枚举编辑点，以及前端接线。

### 仍然真正开放
1. **ast-grep linter-envelope 字段名（P2 阻断项——D5/风险 5）。** 在编写 P2 的 `ast_grep` 加载器**之前**，确认 ast-grep YAML 的确切字段名（`id`/`message`/`severity`/`note`/`constraints`/`utils`，以及锁定的 crate 版本）。匹配类别已确认；只有 envelope 是 `[unverified]`。P1（正则）不依赖于此。
2. **订阅后端并发上限（D1/风险 10）。** 为通过订阅制原生 OAuth 传输的**并发**创作轮次设定安全上限（每一轮都针对一个隔离的 home 派生一个真正的 `claude` 子进程）。建议默认值：一个小信号量（例如 1–2 个并发的订阅创作运行）；计量的 HTTP 运行豁免。确认这个数字，以及它是全局的还是按用户/按项目的。
