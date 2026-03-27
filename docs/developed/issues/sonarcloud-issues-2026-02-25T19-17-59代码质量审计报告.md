# SonarCloud Issues 报告

**生成时间**: 2026/02/26 03:17
**问题总数**: 1026
**已加载**: 1026
**收集数量**: 1026

---

## 统计信息

### 按严重程度分类

- **Minor**: 583 个
- **Major**: 327 个
- **Critical**: 101 个
- **Info**: 9 个
- **Blocker**: 6 个

### 按类型分类

- **Code Smell**: 964 个
- **Bug**: 56 个
- **Vulnerability**: 6 个

### 按影响分类

- **Maintainability**: 852 个
- **Reliability**: 168 个
- **Security**: 6 个

### 按属性分类

- **Consistency**: 487 个
- **Intentionality**: 442 个
- **Adaptability**: 97 个

### 按文件统计 (Top 20)

- **huanchong-99SoloDawnfrontend/src/vscode/bridge.ts**: 26 个问题
- **huanchong-99SoloDawnfrontend/src/components/terminal/TerminalDebugView.tsx**: 21 个问题
- **huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/RestoreLogsDialog.tsx**: 17 个问题
- **huanchong-99SoloDawnfrontend/.../components/ui-new/containers/NewDisplayConversationEntry.tsx**: 17 个问题
- **huanchong-99SoloDawnfrontend/src/stores/wsStore.ts**: 16 个问题
- **huanchong-99SoloDawnfrontend/src/components/ui/pr-comment-card.tsx**: 15 个问题
- **huanchong-99SoloDawnscripts/check-i18n.sh**: 15 个问题
- **huanchong-99SoloDawncrates/db/migrations/20260119000001_add_performance_indexes.sql**: 13 个问题
- **huanchong-99SoloDawnfrontend/.../NormalizedConversation/DisplayConversationEntry.tsx**: 13 个问题
- **huanchong-99SoloDawnfrontend/src/vscode/ContextMenu.tsx**: 13 个问题
- **huanchong-99SoloDawnfrontend/src/components/dialogs/scripts/ScriptFixerDialog.tsx**: 11 个问题
- **huanchong-99SoloDawnfrontend/src/contexts/ClickedElementsProvider.tsx**: 11 个问题
- **huanchong-99SoloDawnfrontend/src/pages/settings/OrganizationSettings.tsx**: 11 个问题
- **huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/TaskFormDialog.tsx**: 10 个问题
- **huanchong-99SoloDawnfrontend/src/components/tasks/TaskFollowUpSection.tsx**: 10 个问题
- **huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/SessionChatBox.tsx**: 10 个问题
- **huanchong-99SoloDawnfrontend/.../components/tasks/TaskDetails/ProcessesTab.tsx**: 9 个问题
- **huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/RepoCard.tsx**: 9 个问题
- **huanchong-99SoloDawnfrontend/src/pages/Workflows.test.tsx**: 9 个问题
- **huanchong-99SoloDawnfrontend/src/stores/__tests__/wsStore.test.ts**: 9 个问题

---

## 问题列表（按文件分组）

## 1. huanchong-99SoloDawncrates/db/migrations/20250617183714_init.sql

> 该文件共有 **1** 个问题

### 1.1 Define a constant instead of duplicating this literal 7 times. ✅ 已修复

- **问题ID**: `AZyVwe6BZ9DOUQdEsGpi`
- **项目**: huanchong-99
- **行号**: L84
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 84min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 2. huanchong-99SoloDawncrates/db/migrations/20250620212427_execution_processes.sql

> 该文件共有 **1** 个问题

### 2.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe4rZ9DOUQdEsGo7`
- **项目**: huanchong-99
- **行号**: L164
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 164min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 3. huanchong-99SoloDawncrates/db/migrations/20250716143725_add_default_templates.sql

> 该文件共有 **4** 个问题

### 3.1 An illegal character with code point 10 was found in this literal. ✅ 已修复

- **问题ID**: `AZyVwe5tZ9DOUQdEsGpd`
- **项目**: huanchong-99
- **行号**: L1610
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1610min effort
- **创建时间**: 1 month ago
- **标签**: pitfall

### 3.2 Define a constant instead of duplicating this literal 6 times. ✅ 已修复

- **问题ID**: `AZyVwe5tZ9DOUQdEsGpg`
- **项目**: huanchong-99
- **行号**: L544
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 544min effort
- **创建时间**: 1 month ago
- **标签**: design

### 3.3 An illegal character with code point 10 was found in this literal. ✅ 已修复

- **问题ID**: `AZyVwe5tZ9DOUQdEsGpe`
- **项目**: huanchong-99
- **行号**: L7110
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 7110min effort
- **创建时间**: 1 month ago
- **标签**: pitfall

### 3.4 An illegal character with code point 10 was found in this literal. ✅ 已修复

- **问题ID**: `AZyVwe5tZ9DOUQdEsGpf`
- **项目**: huanchong-99
- **行号**: L12410
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 12410min effort
- **创建时间**: 1 month ago
- **标签**: pitfall

---

## 4. huanchong-99SoloDawncrates/db/migrations/20250720000000_add_cleanupscript_to_process_type_constraint.sql

> 该文件共有 **1** 个问题

### 4.1 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe6ZZ9DOUQdEsGpl`
- **项目**: huanchong-99
- **行号**: L1030
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1030min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 5. huanchong-99SoloDawncrates/db/migrations/20250730000000_add_executor_action_to_execution_processes.sql

> 该文件共有 **7** 个问题

### 5.1 Define a constant instead of duplicating this literal 4 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGpC`
- **项目**: huanchong-99
- **行号**: L184
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 184min effort
- **创建时间**: 22 hours ago
- **标签**: design

### 5.2 Define a constant instead of duplicating this literal 4 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGo_`
- **项目**: huanchong-99
- **行号**: L204
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 204min effort
- **创建时间**: 22 hours ago
- **标签**: design

### 5.3 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGpD`
- **项目**: huanchong-99
- **行号**: L244
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 244min effort
- **创建时间**: 22 hours ago
- **标签**: design

### 5.4 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGpA`
- **项目**: huanchong-99
- **行号**: L254
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 254min effort
- **创建时间**: 22 hours ago
- **标签**: design

### 5.5 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGpB`
- **项目**: huanchong-99
- **行号**: L264
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 264min effort
- **创建时间**: 22 hours ago
- **标签**: design

### 5.6 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGo-`
- **项目**: huanchong-99
- **行号**: L274
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 274min effort
- **创建时间**: 22 hours ago
- **标签**: design

### 5.7 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGo9`
- **项目**: huanchong-99
- **行号**: L5310
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 5310min effort
- **创建时间**: 22 hours ago
- **标签**: sql

---

## 6. huanchong-99SoloDawncrates/db/migrations/20250815100344_migrate_old_executor_actions.sql

> 该文件共有 **1** 个问题

### 6.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe4TZ9DOUQdEsGo4`
- **项目**: huanchong-99
- **行号**: L54
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 54min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 7. huanchong-99SoloDawncrates/db/migrations/20250818150000_refactor_images_to_junction_tables.sql

> 该文件共有 **1** 个问题

### 7.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe6JZ9DOUQdEsGpj`
- **项目**: huanchong-99
- **行号**: L144
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 144min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 8. huanchong-99SoloDawncrates/db/migrations/20250819000000_move_merge_commit_to_merges_table.sql

> 该文件共有 **3** 个问题

### 8.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe31Z9DOUQdEsGoz`
- **项目**: huanchong-99
- **行号**: L54
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 54min effort
- **创建时间**: 1 month ago
- **标签**: design

### 8.2 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe31Z9DOUQdEsGox`
- **项目**: huanchong-99
- **行号**: L134
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 134min effort
- **创建时间**: 1 month ago
- **标签**: design

### 8.3 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe31Z9DOUQdEsGoy`
- **项目**: huanchong-99
- **行号**: L134
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 134min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 9. huanchong-99SoloDawncrates/db/migrations/20250921222241_unify_drafts_tables.sql

> 该文件共有 **1** 个问题

### 9.1 Refactor this SQL query to eliminate the use of EXISTS. ✅ 已修复

- **问题ID**: `AZyVwe4zZ9DOUQdEsGo8`
- **项目**: huanchong-99
- **行号**: L711
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 22 hours ago
- **标签**: performance, sql

---

## 10. huanchong-99SoloDawncrates/db/migrations/20251020120000_convert_templates_to_tags.sql

> 该文件共有 **2** 个问题

### 10.1 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5SZ9DOUQdEsGpS`
- **项目**: huanchong-99
- **行号**: L710
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 710min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 10.2 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5SZ9DOUQdEsGpT`
- **项目**: huanchong-99
- **行号**: L2210
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2210min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 11. huanchong-99SoloDawncrates/db/migrations/20251114000000_create_shared_tasks.sql

> 该文件共有 **1** 个问题

### 11.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe4DZ9DOUQdEsGo1`
- **项目**: huanchong-99
- **行号**: L164
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 164min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 12. huanchong-99SoloDawncrates/db/migrations/20251202000000_migrate_to_electric.sql

> 该文件共有 **1** 个问题

### 12.1 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe37Z9DOUQdEsGo0`
- **项目**: huanchong-99
- **行号**: L1030
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1030min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 13. huanchong-99SoloDawncrates/db/migrations/20251209000000_add_project_repositories.sql

> 该文件共有 **6** 个问题

### 13.1 Define a constant instead of duplicating this literal 8 times. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpc`
- **项目**: huanchong-99
- **行号**: L74
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 74min effort
- **创建时间**: 1 month ago
- **标签**: design

### 13.2 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpX`
- **项目**: huanchong-99
- **行号**: L7310
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 7310min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 13.3 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpY`
- **项目**: huanchong-99
- **行号**: L8610
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 8610min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 13.4 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpZ`
- **项目**: huanchong-99
- **行号**: L10330
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 10330min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 13.5 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpa`
- **项目**: huanchong-99
- **行号**: L12730
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 12730min effort
- **创建时间**: 22 hours ago
- **标签**: sql

### 13.6 The number of join conditions 4 exceeds the maximum allowed 3. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpb`
- **项目**: huanchong-99
- **行号**: L1592
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Adaptability
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: brain-overload, performance, ...

---

## 14. huanchong-99SoloDawncrates/db/migrations/20251216142123_refactor_task_attempts_to_workspaces_sessions.sql

> 该文件共有 **1** 个问题

### 14.1 Define a constant instead of duplicating this literal 7 times. ✅ 已修复

- **问题ID**: `AZyVwe4cZ9DOUQdEsGo5`
- **项目**: huanchong-99
- **行号**: L174
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 174min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 15. huanchong-99SoloDawncrates/db/migrations/20251219000000_add_agent_working_dir_to_projects.sql

> 该文件共有 **1** 个问题

### 15.1 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5LZ9DOUQdEsGpR`
- **项目**: huanchong-99
- **行号**: L810
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 810min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 16. huanchong-99SoloDawncrates/db/migrations/20251220134608_fix_session_executor_format.sql

> 该文件共有 **1** 个问题

### 16.1 Refactor this SQL query to prevent doing a full table scan due to the value of the "LIKE" condition ✅ 已修复

- **问题ID**: `AZyVwe51Z9DOUQdEsGph`
- **项目**: huanchong-99
- **行号**: L73
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: performance, sql

---

## 17. huanchong-99SoloDawncrates/db/migrations/20260107000000_move_scripts_to_repos.sql

> 该文件共有 **2** 个问题

### 17.1 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe4KZ9DOUQdEsGo2`
- **项目**: huanchong-99
- **行号**: L1130
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1130min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 17.2 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe4KZ9DOUQdEsGo3`
- **项目**: huanchong-99
- **行号**: L5130
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 5130min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 18. huanchong-99SoloDawncrates/db/migrations/20260117000001_create_workflow_tables.sql

> 该文件共有 **3** 个问题

### 18.1 Define a constant instead of duplicating this literal 4 times. ✅ 已修复

- **问题ID**: `AZyVwe5bZ9DOUQdEsGpW`
- **项目**: huanchong-99
- **行号**: L254
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 254min effort
- **创建时间**: 1 month ago
- **标签**: design

### 18.2 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5bZ9DOUQdEsGpU`
- **项目**: huanchong-99
- **行号**: L264
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 264min effort
- **创建时间**: 1 month ago
- **标签**: design

### 18.3 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5bZ9DOUQdEsGpV`
- **项目**: huanchong-99
- **行号**: L274
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 274min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 19. huanchong-99SoloDawncrates/db/migrations/20260119000001_add_performance_indexes.sql

> 该文件共有 **13** 个问题

### 19.1 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpE`
- **项目**: huanchong-99
- **行号**: L125
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 125min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 19.2 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpF`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 19.3 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpN`
- **项目**: huanchong-99
- **行号**: L214
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 214min effort
- **创建时间**: 1 month ago
- **标签**: design

### 19.4 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpO`
- **项目**: huanchong-99
- **行号**: L214
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 214min effort
- **创建时间**: 1 month ago
- **标签**: design

### 19.5 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpP`
- **项目**: huanchong-99
- **行号**: L214
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 214min effort
- **创建时间**: 1 month ago
- **标签**: design

### 19.6 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpG`
- **项目**: huanchong-99
- **行号**: L345
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 345min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 19.7 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpH`
- **项目**: huanchong-99
- **行号**: L395
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 395min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 19.8 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpQ`
- **项目**: huanchong-99
- **行号**: L424
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 424min effort
- **创建时间**: 1 month ago
- **标签**: design

### 19.9 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpI`
- **项目**: huanchong-99
- **行号**: L495
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 495min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 19.10 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpJ`
- **项目**: huanchong-99
- **行号**: L545
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 545min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 19.11 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpK`
- **项目**: huanchong-99
- **行号**: L705
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 705min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 19.12 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpL`
- **项目**: huanchong-99
- **行号**: L765
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 765min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 19.13 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpM`
- **项目**: huanchong-99
- **行号**: L925
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 925min effort
- **创建时间**: 27 days ago
- **标签**: unused

---

## 20. huanchong-99SoloDawncrates/db/migrations/20260208010000_backfill_terminal_auto_confirm.sql

> 该文件共有 **1** 个问题

### 20.1 Refactor this SQL query to eliminate the use of EXISTS. ✅ 已修复

- **问题ID**: `AZyVwe6RZ9DOUQdEsGpk`
- **项目**: huanchong-99
- **行号**: L121
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 17 days ago
- **标签**: performance, sql

---

## 21. huanchong-99SoloDawncrates/db/migrations/20260208020000_fix_terminal_old_foreign_keys.sql

> 该文件共有 **1** 个问题

### 21.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe4kZ9DOUQdEsGo6`
- **项目**: huanchong-99
- **行号**: L314
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 314min effort
- **创建时间**: 17 days ago
- **标签**: design

---

## 22. huanchong-99SoloDawncrates/db/migrations/20260224001000_backfill_workflow_api_key_encrypted.sql

> 该文件共有 **2** 个问题

### 22.1 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe6gZ9DOUQdEsGpm`
- **项目**: huanchong-99
- **行号**: L1410
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1410min effort
- **创建时间**: 22 hours ago
- **标签**: sql

### 22.2 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe6gZ9DOUQdEsGpn`
- **项目**: huanchong-99
- **行号**: L1610
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1610min effort
- **创建时间**: 22 hours ago
- **标签**: sql

---

## 23. huanchong-99SoloDawndocker/Dockerfile

> 该文件共有 **2** 个问题

### 23.1 Merge this RUN instruction with the consecutive ones. ✅ 已修复

- **问题ID**: `AZyVwe8EZ9DOUQdEsGqq`
- **项目**: huanchong-99
- **行号**: L375
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 375min effort
- **创建时间**: 22 hours ago

### 23.2 Merge this RUN instruction with the consecutive ones. ✅ 已修复

- **问题ID**: `AZyVwe8EZ9DOUQdEsGqr`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 635min effort
- **创建时间**: 2 days ago

---

## 24. huanchong-99SoloDawnfrontend/.../NormalizedConversation/DisplayConversationEntry.tsx

> 该文件共有 **13** 个问题

### 24.1 'diffDeletable' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeG`
- **项目**: huanchong-99
- **行号**: L495
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 495min effort
- **创建时间**: 1 month ago
- **标签**: react

### 24.2 Refactor this function to reduce its Cognitive Complexity from 18 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeH`
- **项目**: huanchong-99
- **行号**: L618
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 618min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 24.3 Complete the task associated to this "TODO" comment. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeI`
- **项目**: huanchong-99
- **行号**: L810
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 810min effort
- **创建时间**: 1 month ago
- **标签**: cwe

### 24.4 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeJ`
- **项目**: huanchong-99
- **行号**: L2275
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2275min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 24.5 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeK`
- **项目**: huanchong-99
- **行号**: L2275
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2275min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 24.6 Refactor this function to reduce its Cognitive Complexity from 18 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeL`
- **项目**: huanchong-99
- **行号**: L4488
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 4488min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 24.7 `SCRIPT_TOOL_NAMES` should be a `Set`, and use `SCRIPT_TOOL_NAMES.has()` to check existence or non-existence. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeM`
- **项目**: huanchong-99
- **行号**: L5945
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5945min effort
- **创建时间**: 1 month ago
- **标签**: optimization, performance

### 24.8 Refactor this function to reduce its Cognitive Complexity from 20 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeN`
- **项目**: huanchong-99
- **行号**: L69710
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 69710min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 24.9 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeO`
- **项目**: huanchong-99
- **行号**: L6975
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 6975min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 24.10 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeP`
- **项目**: huanchong-99
- **行号**: L7501
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 7501min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 24.11 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeQ`
- **项目**: huanchong-99
- **行号**: L7891
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 7891min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 24.12 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeR`
- **项目**: huanchong-99
- **行号**: L7945
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 7945min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

### 24.13 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweX9Z9DOUQdEsGeS`
- **项目**: huanchong-99
- **行号**: L9315
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 9315min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 25. huanchong-99SoloDawnfrontend/.../NormalizedConversation/EditDiffRenderer.tsx

> 该文件共有 **4** 个问题

### 25.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweYOZ9DOUQdEsGeY`
- **项目**: huanchong-99
- **行号**: L645
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 645min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 25.2 Non-interactive elements should not be assigned mouse or keyboard event listeners. ✅ 已修复

- **问题ID**: `AZyVweYOZ9DOUQdEsGeZ`
- **项目**: huanchong-99
- **行号**: L1045
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1045min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 25.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweYOZ9DOUQdEsGea`
- **项目**: huanchong-99
- **行号**: L1045
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1045min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 25.4 A fragment with only one child is redundant. ✅ 已修复

- **问题ID**: `AZyVweYOZ9DOUQdEsGeb`
- **项目**: huanchong-99
- **行号**: L1305
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 1305min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 26. huanchong-99SoloDawnfrontend/.../NormalizedConversation/FileChangeRenderer.tsx

> 该文件共有 **3** 个问题

### 26.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweYXZ9DOUQdEsGec`
- **项目**: huanchong-99
- **行号**: L605
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 605min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 26.2 Non-interactive elements should not be assigned mouse or keyboard event listeners. ✅ 已修复

- **问题ID**: `AZyVweYXZ9DOUQdEsGed`
- **项目**: huanchong-99
- **行号**: L1375
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1375min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 26.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweYXZ9DOUQdEsGee`
- **项目**: huanchong-99
- **行号**: L1375
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1375min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 27. huanchong-99SoloDawnfrontend/.../NormalizedConversation/NextActionCard.tsx

> 该文件共有 **6** 个问题

### 27.1 '@/components/ide/IdeIcon' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweYgZ9DOUQdEsGef`
- **项目**: huanchong-99
- **行号**: L221
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 221min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 27.2 '@/components/ide/IdeIcon' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweYgZ9DOUQdEsGeg`
- **项目**: huanchong-99
- **行号**: L241
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 241min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 27.3 Refactor this function to reduce its Cognitive Complexity from 32 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweYgZ9DOUQdEsGeh`
- **项目**: huanchong-99
- **行号**: L5022
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 5022min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 27.4 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweYgZ9DOUQdEsGei`
- **项目**: huanchong-99
- **行号**: L505
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 505min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 27.5 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweYgZ9DOUQdEsGej`
- **项目**: huanchong-99
- **行号**: L3322
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3322min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 27.6 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweYgZ9DOUQdEsGek`
- **项目**: huanchong-99
- **行号**: L3345
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3345min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 28. huanchong-99SoloDawnfrontend/.../NormalizedConversation/PendingApprovalEntry.tsx

> 该文件共有 **5** 个问题

### 28.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweYHZ9DOUQdEsGeT`
- **项目**: huanchong-99
- **行号**: L562
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 562min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 28.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweYHZ9DOUQdEsGeU`
- **项目**: huanchong-99
- **行号**: L602
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 602min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 28.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweYHZ9DOUQdEsGeV`
- **项目**: huanchong-99
- **行号**: L632
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 632min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 28.4 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweYHZ9DOUQdEsGeW`
- **项目**: huanchong-99
- **行号**: L755
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 755min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 28.5 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweYHZ9DOUQdEsGeX`
- **项目**: huanchong-99
- **行号**: L1275
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1275min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 29. huanchong-99SoloDawnfrontend/.../NormalizedConversation/RetryEditorInline.tsx

> 该文件共有 **1** 个问题

### 29.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweXzZ9DOUQdEsGeF`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 30. huanchong-99SoloDawnfrontend/.../components/dialogs/projects/LinkProjectDialog.tsx

> 该文件共有 **6** 个问题

### 30.1 Refactor this function to reduce its Cognitive Complexity from 26 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweRDZ9DOUQdEsGbv`
- **项目**: huanchong-99
- **行号**: L4516
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 4516min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 30.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweRDZ9DOUQdEsGbx`
- **项目**: huanchong-99
- **行号**: L2095
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2095min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 30.3 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweRDZ9DOUQdEsGbw`
- **项目**: huanchong-99
- **行号**: L2092
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2092min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 30.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweRDZ9DOUQdEsGbz`
- **项目**: huanchong-99
- **行号**: L2155
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2155min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 30.5 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweRDZ9DOUQdEsGby`
- **项目**: huanchong-99
- **行号**: L2152
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2152min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 30.6 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweRDZ9DOUQdEsGb0`
- **项目**: huanchong-99
- **行号**: L2785
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2785min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 31. huanchong-99SoloDawnfrontend/.../components/dialogs/projects/ProjectEditorSelectionDialog.tsx

> 该文件共有 **1** 个问题

### 31.1 A form label must be associated with a control. ✅ 已修复

- **问题ID**: `AZyVweQ5Z9DOUQdEsGbu`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 635min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 32. huanchong-99SoloDawnfrontend/.../components/dialogs/settings/DeleteConfigurationDialog.tsx

> 该文件共有 **1** 个问题

### 32.1 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVweS7Z9DOUQdEsGc0`
- **项目**: huanchong-99
- **行号**: L381
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

---

## 33. huanchong-99SoloDawnfrontend/.../components/tasks/TaskDetails/ProcessLogsViewer.tsx

> 该文件共有 **3** 个问题

### 33.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweZXZ9DOUQdEsGe1`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 33.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweZXZ9DOUQdEsGe2`
- **项目**: huanchong-99
- **行号**: L755
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 755min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 33.3 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweZXZ9DOUQdEsGe3`
- **项目**: huanchong-99
- **行号**: L995
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 995min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 34. huanchong-99SoloDawnfrontend/.../components/tasks/TaskDetails/ProcessesTab.tsx

> 该文件共有 **9** 个问题

### 34.1 Refactor this function to reduce its Cognitive Complexity from 21 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweZPZ9DOUQdEsGes`
- **项目**: huanchong-99
- **行号**: L2611
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 2611min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 34.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweZPZ9DOUQdEsGet`
- **项目**: huanchong-99
- **行号**: L265
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 265min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 34.3 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweZPZ9DOUQdEsGeu`
- **项目**: huanchong-99
- **行号**: L1682
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1682min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 34.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweZPZ9DOUQdEsGev`
- **项目**: huanchong-99
- **行号**: L1805
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1805min effort
- **创建时间**: 26 days ago
- **标签**: confusing

### 34.5 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweZPZ9DOUQdEsGew`
- **项目**: huanchong-99
- **行号**: L1905
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1905min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 34.6 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweZPZ9DOUQdEsGex`
- **项目**: huanchong-99
- **行号**: L1905
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1905min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 34.7 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweZPZ9DOUQdEsGey`
- **项目**: huanchong-99
- **行号**: L1955
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1955min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 34.8 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweZPZ9DOUQdEsGez`
- **项目**: huanchong-99
- **行号**: L2935
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2935min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 34.9 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweZPZ9DOUQdEsGe0`
- **项目**: huanchong-99
- **行号**: L3125
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3125min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 35. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/BrowseRepoButtonContainer.tsx

> 该文件共有 **1** 个问题

### 35.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwekfZ9DOUQdEsGh9`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 36. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ChangesPanelContainer.tsx

> 该文件共有 **4** 个问题

### 36.1 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwejOZ9DOUQdEsGho`
- **项目**: huanchong-99
- **行号**: L6320
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 6320min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 36.2 Prefer `.dataset` over `getAttribute(…)`. ✅ 已修复

- **问题ID**: `AZyVwejOZ9DOUQdEsGhp`
- **项目**: huanchong-99
- **行号**: L645
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 645min effort
- **创建时间**: 1 month ago
- **标签**: api, dom, ...

### 36.3 Prefer `.dataset` over `setAttribute(…)`. ✅ 已修复

- **问题ID**: `AZyVwejOZ9DOUQdEsGhq`
- **项目**: huanchong-99
- **行号**: L1235
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1235min effort
- **创建时间**: 1 month ago
- **标签**: api, dom, ...

### 36.4 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwejOZ9DOUQdEsGhr`
- **项目**: huanchong-99
- **行号**: L1395
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1395min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 37. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/CommentWidgetLine.tsx

> 该文件共有 **1** 个问题

### 37.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweiKZ9DOUQdEsGha`
- **项目**: huanchong-99
- **行号**: L165
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 165min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 38. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ContextBarContainer.tsx

> 该文件共有 **1** 个问题

### 38.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweiSZ9DOUQdEsGhb`
- **项目**: huanchong-99
- **行号**: L625
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 625min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 39. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ConversationListContainer.tsx

> 该文件共有 **3** 个问题

### 39.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwekwZ9DOUQdEsGh_`
- **项目**: huanchong-99
- **行号**: L875
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 875min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 39.2 Move this component definition out of the parent component and pass data as props. ✅ 已修复

- **问题ID**: `AZyVwekwZ9DOUQdEsGiA`
- **项目**: huanchong-99
- **行号**: L1795
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1795min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

### 39.3 Move this component definition out of the parent component and pass data as props. ✅ 已修复

- **问题ID**: `AZyVwekwZ9DOUQdEsGiB`
- **项目**: huanchong-99
- **行号**: L1805
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1805min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 40. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/CopyButton.tsx

> 该文件共有 **1** 个问题

### 40.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwekoZ9DOUQdEsGh-`
- **项目**: huanchong-99
- **行号**: L165
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 165min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 41. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/CreateChatBoxContainer.tsx

> 该文件共有 **2** 个问题

### 41.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwejmZ9DOUQdEsGhu`
- **项目**: huanchong-99
- **行号**: L1905
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1905min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 41.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwejmZ9DOUQdEsGhv`
- **项目**: huanchong-99
- **行号**: L1915
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1915min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 42. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/CreateRepoButtonContainer.tsx

> 该文件共有 **1** 个问题

### 42.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwei-Z9DOUQdEsGhm`
- **项目**: huanchong-99
- **行号**: L125
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 125min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 43. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/DiffViewCardWithComments.tsx

> 该文件共有 **8** 个问题

### 43.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwekXZ9DOUQdEsGh1`
- **项目**: huanchong-99
- **行号**: L1715
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1715min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 43.2 'If' statement should not be the only statement in 'else' block ✅ 已修复

- **问题ID**: `AZyVwekXZ9DOUQdEsGh2`
- **项目**: huanchong-99
- **行号**: L2705
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2705min effort
- **创建时间**: 1 month ago

### 43.3 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVwekXZ9DOUQdEsGh3`
- **项目**: huanchong-99
- **行号**: L2922
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2922min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 43.4 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVwekXZ9DOUQdEsGh4`
- **项目**: huanchong-99
- **行号**: L3425
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3425min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 43.5 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVwekXZ9DOUQdEsGh5`
- **项目**: huanchong-99
- **行号**: L3425
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 3425min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 43.6 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVwekXZ9DOUQdEsGh6`
- **项目**: huanchong-99
- **行号**: L4105
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4105min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 43.7 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVwekXZ9DOUQdEsGh7`
- **项目**: huanchong-99
- **行号**: L4105
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 4105min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 43.8 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwekXZ9DOUQdEsGh8`
- **项目**: huanchong-99
- **行号**: L4485
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4485min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 44. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/FileTreeContainer.tsx

> 该文件共有 **1** 个问题

### 44.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwejGZ9DOUQdEsGhn`
- **项目**: huanchong-99
- **行号**: L215
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 215min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 45. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/GitHubCommentRenderer.tsx

> 该文件共有 **1** 个问题

### 45.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwejXZ9DOUQdEsGhs`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 46. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/GitPanelContainer.tsx

> 该文件共有 **2** 个问题

### 46.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelCZ9DOUQdEsGiD`
- **项目**: huanchong-99
- **行号**: L285
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 285min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 46.2 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwelCZ9DOUQdEsGiE`
- **项目**: huanchong-99
- **行号**: L665
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 665min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

---

## 47. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/GitPanelCreateContainer.tsx

> 该文件共有 **1** 个问题

### 47.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwej-Z9DOUQdEsGhy`
- **项目**: huanchong-99
- **行号**: L125
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 125min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 48. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/LogsContentContainer.tsx

> 该文件共有 **1** 个问题

### 48.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwej2Z9DOUQdEsGhx`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 49. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/NewDisplayConversationEntry.tsx

> 该文件共有 **17** 个问题

### 49.1 Refactor this function to reduce its Cognitive Complexity from 16 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwelVZ9DOUQdEsGiH`
- **项目**: huanchong-99
- **行号**: L556
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 556min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 49.2 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiI`
- **项目**: huanchong-99
- **行号**: L1701
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1701min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 49.3 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiJ`
- **项目**: huanchong-99
- **行号**: L1755
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1755min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

### 49.4 Complete the task associated to this "TODO" comment. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiK`
- **项目**: huanchong-99
- **行号**: L1980
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1980min effort
- **创建时间**: 1 month ago
- **标签**: cwe

### 49.5 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiL`
- **项目**: huanchong-99
- **行号**: L2615
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2615min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.6 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiM`
- **项目**: huanchong-99
- **行号**: L3405
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 3405min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.7 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiN`
- **项目**: huanchong-99
- **行号**: L4175
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4175min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.8 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiO`
- **项目**: huanchong-99
- **行号**: L4595
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4595min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.9 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiP`
- **项目**: huanchong-99
- **行号**: L4925
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4925min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.10 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiQ`
- **项目**: huanchong-99
- **行号**: L5325
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 5325min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.11 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiR`
- **项目**: huanchong-99
- **行号**: L5455
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 5455min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.12 Complete the task associated to this "TODO" comment. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiS`
- **项目**: huanchong-99
- **行号**: L5970
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5970min effort
- **创建时间**: 1 month ago
- **标签**: cwe

### 49.13 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiT`
- **项目**: huanchong-99
- **行号**: L5995
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 5995min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.14 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiU`
- **项目**: huanchong-99
- **行号**: L6175
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 6175min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.15 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiV`
- **项目**: huanchong-99
- **行号**: L6415
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 6415min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 49.16 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiW`
- **项目**: huanchong-99
- **行号**: L6775
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6775min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 49.17 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelWZ9DOUQdEsGiX`
- **项目**: huanchong-99
- **行号**: L7075
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 7075min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 50. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/PreviewBrowserContainer.tsx

> 该文件共有 **2** 个问题

### 50.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweibZ9DOUQdEsGhc`
- **项目**: huanchong-99
- **行号**: L345
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 345min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 50.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweibZ9DOUQdEsGhd`
- **项目**: huanchong-99
- **行号**: L2945
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2945min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 51. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/PreviewControlsContainer.tsx

> 该文件共有 **1** 个问题

### 51.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwekOZ9DOUQdEsGh0`
- **项目**: huanchong-99
- **行号**: L175
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 175min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 52. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ProcessListContainer.tsx

> 该文件共有 **1** 个问题

### 52.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVwejeZ9DOUQdEsGht`
- **项目**: huanchong-99
- **行号**: L755
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 755min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 53. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ProjectSelectorContainer.tsx

> 该文件共有 **4** 个问题

### 53.1 'react-virtuoso' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweirZ9DOUQdEsGhf`
- **项目**: huanchong-99
- **行号**: L31
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 31min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 53.2 'react-virtuoso' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweirZ9DOUQdEsGhg`
- **项目**: huanchong-99
- **行号**: L141
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 141min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 53.3 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweirZ9DOUQdEsGhh`
- **项目**: huanchong-99
- **行号**: L255
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 255min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 53.4 Move this component definition out of the parent component and pass data as props. ✅ 已修复

- **问题ID**: `AZyVweirZ9DOUQdEsGhi`
- **项目**: huanchong-99
- **行号**: L2035
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2035min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 54. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/RecentReposListContainer.tsx

> 该文件共有 **1** 个问题

### 54.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwekGZ9DOUQdEsGhz`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 55. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ReviewCommentRenderer.tsx

> 该文件共有 **1** 个问题

### 55.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweleZ9DOUQdEsGiY`
- **项目**: huanchong-99
- **行号**: L135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 135min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 56. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/RightSidebar.tsx

> 该文件共有 **1** 个问题

### 56.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweijZ9DOUQdEsGhe`
- **项目**: huanchong-99
- **行号**: L235
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 235min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 57. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/SearchableDropdownContainer.tsx

> 该文件共有 **1** 个问题

### 57.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwek5Z9DOUQdEsGiC`
- **项目**: huanchong-99
- **行号**: L355
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 355min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 58. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/SessionChatBoxContainer.tsx

> 该文件共有 **3** 个问题

### 58.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwei1Z9DOUQdEsGhj`
- **项目**: huanchong-99
- **行号**: L865
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 865min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 58.2 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwei1Z9DOUQdEsGhk`
- **项目**: huanchong-99
- **行号**: L1281
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1281min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 58.3 Promise-returning function provided to property where a void return was expected. ✅ 已修复

- **问题ID**: `AZyVwei1Z9DOUQdEsGhl`
- **项目**: huanchong-99
- **行号**: L6115
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 6115min effort
- **创建时间**: 1 month ago
- **标签**: async, promise, ...

---

## 59. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/VirtualizedProcessLogs.tsx

> 该文件共有 **1** 个问题

### 59.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwejvZ9DOUQdEsGhw`
- **项目**: huanchong-99
- **行号**: L755
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 755min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 60. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/WorkspacesLayout.tsx

> 该文件共有 **2** 个问题

### 60.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelLZ9DOUQdEsGiF`
- **项目**: huanchong-99
- **行号**: L425
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 425min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 60.2 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVwelLZ9DOUQdEsGiG`
- **项目**: huanchong-99
- **行号**: L995
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 995min effort
- **创建时间**: 1 month ago
- **标签**: confusing, type-dependent

---

## 61. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/WorkspacesMainContainer.tsx

> 该文件共有 **1** 个问题

### 61.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwelmZ9DOUQdEsGiZ`
- **项目**: huanchong-99
- **行号**: L205
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 205min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 62. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/Card.tsx

> 该文件共有 **1** 个问题

### 62.1 Headings must have content and the content must be accessible by a screen reader. ✅ 已修复

- **问题ID**: `AZyVweeQZ9DOUQdEsGgV`
- **项目**: huanchong-99
- **行号**: L365
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 365min effort
- **创建时间**: 27 days ago
- **标签**: accessibility, react

---

## 63. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/ChatBoxBase.tsx

> 该文件共有 **1** 个问题

### 63.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwednZ9DOUQdEsGgO`
- **项目**: huanchong-99
- **行号**: L805
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 805min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 64. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/CollapsibleSection.tsx

> 该文件共有 **1** 个问题

### 64.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwedXZ9DOUQdEsGgM`
- **项目**: huanchong-99
- **行号**: L175
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 175min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 65. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/CollapsibleSectionHeader.tsx

> 该文件共有 **2** 个问题

### 65.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweeYZ9DOUQdEsGgW`
- **项目**: huanchong-99
- **行号**: L205
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 205min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 65.2 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweeYZ9DOUQdEsGgX`
- **项目**: huanchong-99
- **行号**: L495
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 495min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 66. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/Command.tsx

> 该文件共有 **2** 个问题

### 66.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwee3Z9DOUQdEsGgd`
- **项目**: huanchong-99
- **行号**: L265
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 265min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 66.2 Unknown property 'cmdk-input-wrapper' found ✅ 已修复

- **问题ID**: `AZyVwee3Z9DOUQdEsGge`
- **项目**: huanchong-99
- **行号**: L505
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 505min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 67. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/CommandBar.tsx

> 该文件共有 **2** 个问题

### 67.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwec-Z9DOUQdEsGgA`
- **项目**: huanchong-99
- **行号**: L205
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 205min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 67.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwec-Z9DOUQdEsGgB`
- **项目**: huanchong-99
- **行号**: L585
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 585min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 68. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/CommentCard.tsx

> 该文件共有 **1** 个问题

### 68.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwedGZ9DOUQdEsGgC`
- **项目**: huanchong-99
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 69. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/ContextBar.tsx

> 该文件共有 **3** 个问题

### 69.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwefZZ9DOUQdEsGgm`
- **项目**: huanchong-99
- **行号**: L725
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 725min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 69.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVwefZZ9DOUQdEsGgn`
- **项目**: huanchong-99
- **行号**: L805
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 805min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 69.3 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwefZZ9DOUQdEsGgo`
- **项目**: huanchong-99
- **行号**: L1365
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1365min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 70. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/CreateChatBox.tsx

> 该文件共有 **1** 个问题

### 70.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweeIZ9DOUQdEsGgU`
- **项目**: huanchong-99
- **行号**: L505
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 505min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 71. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/ErrorAlert.tsx

> 该文件共有 **2** 个问题

### 71.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweeAZ9DOUQdEsGgS`
- **项目**: huanchong-99
- **行号**: L95
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 95min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 71.2 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVweeAZ9DOUQdEsGgT`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 72. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/Field.tsx

> 该文件共有 **2** 个问题

### 72.1 Use <details>, <fieldset>, <optgroup>, or <address> instead of the "group" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwefRZ9DOUQdEsGgk`
- **项目**: huanchong-99
- **行号**: L855
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 855min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 72.2 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVwefRZ9DOUQdEsGgl`
- **项目**: huanchong-99
- **行号**: L2095
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2095min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 73. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/FormField.tsx

> 该文件共有 **1** 个问题

### 73.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwec1Z9DOUQdEsGf_`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 185min effort
- **创建时间**: 27 days ago
- **标签**: react, type-dependent

---

## 74. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/IconButton.tsx

> 该文件共有 **2** 个问题

### 74.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwef5Z9DOUQdEsGgw`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 74.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwef5Z9DOUQdEsGgx`
- **项目**: huanchong-99
- **行号**: L255
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 255min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 75. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/IconButtonGroup.tsx

> 该文件共有 **2** 个问题

### 75.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwed4Z9DOUQdEsGgQ`
- **项目**: huanchong-99
- **行号**: L105
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 105min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 75.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwed4Z9DOUQdEsGgR`
- **项目**: huanchong-99
- **行号**: L465
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 465min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 76. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/IconListItem.tsx

> 该文件共有 **1** 个问题

### 76.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwedfZ9DOUQdEsGgN`
- **项目**: huanchong-99
- **行号**: L135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 135min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 77. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/InputField.tsx

> 该文件共有 **1** 个问题

### 77.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwectZ9DOUQdEsGf-`
- **项目**: huanchong-99
- **行号**: L215
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 215min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 78. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/PrimaryButton.tsx

> 该文件共有 **4** 个问题

### 78.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwefhZ9DOUQdEsGgp`
- **项目**: huanchong-99
- **行号**: L135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 135min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 78.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwefhZ9DOUQdEsGgq`
- **项目**: huanchong-99
- **行号**: L235
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 235min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 78.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwefhZ9DOUQdEsGgr`
- **项目**: huanchong-99
- **行号**: L255
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 255min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 78.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwefhZ9DOUQdEsGgs`
- **项目**: huanchong-99
- **行号**: L415
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 415min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 79. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/ProcessListItem.tsx

> 该文件共有 **1** 个问题

### 79.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwee_Z9DOUQdEsGgf`
- **项目**: huanchong-99
- **行号**: L465
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 465min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 80. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/RecentReposList.tsx

> 该文件共有 **1** 个问题

### 80.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwedwZ9DOUQdEsGgP`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 81. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/RepoCard.tsx

> 该文件共有 **9** 个问题

### 81.1 Refactor this function to reduce its Cognitive Complexity from 23 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwedPZ9DOUQdEsGgD`
- **项目**: huanchong-99
- **行号**: L7313
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 7313min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 81.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwedPZ9DOUQdEsGgE`
- **项目**: huanchong-99
- **行号**: L735
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 735min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 81.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwedPZ9DOUQdEsGgF`
- **项目**: huanchong-99
- **行号**: L2245
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2245min effort
- **创建时间**: 22 hours ago
- **标签**: confusing

### 81.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwedPZ9DOUQdEsGgG`
- **项目**: huanchong-99
- **行号**: L2395
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2395min effort
- **创建时间**: 22 hours ago
- **标签**: confusing

### 81.5 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwedPZ9DOUQdEsGgH`
- **项目**: huanchong-99
- **行号**: L2655
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2655min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 81.6 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwedPZ9DOUQdEsGgI`
- **项目**: huanchong-99
- **行号**: L2725
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2725min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 81.7 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwedPZ9DOUQdEsGgJ`
- **项目**: huanchong-99
- **行号**: L2745
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2745min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 81.8 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwedPZ9DOUQdEsGgK`
- **项目**: huanchong-99
- **行号**: L2815
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2815min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 81.9 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwedPZ9DOUQdEsGgL`
- **项目**: huanchong-99
- **行号**: L2835
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2835min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 82. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/RepoCardSimple.tsx

> 该文件共有 **1** 个问题

### 82.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwegiZ9DOUQdEsGhA`
- **项目**: huanchong-99
- **行号**: L175
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 175min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 83. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/SearchableDropdown.tsx

> 该文件共有 **2** 个问题

### 83.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwegQZ9DOUQdEsGg0`
- **项目**: huanchong-99
- **行号**: L595
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 595min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 83.2 Move this component definition out of the parent component and pass data as props. ✅ 已修复

- **问题ID**: `AZyVwegQZ9DOUQdEsGg1`
- **项目**: huanchong-99
- **行号**: L1025
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1025min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 84. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/SectionHeader.tsx

> 该文件共有 **1** 个问题

### 84.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweefZ9DOUQdEsGgY`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 115min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 85. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/SelectedReposList.tsx

> 该文件共有 **1** 个问题

### 85.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwefpZ9DOUQdEsGgt`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 86. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/SessionChatBox.tsx

> 该文件共有 **10** 个问题

### 86.1 Refactor this function to reduce its Cognitive Complexity from 28 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg2`
- **项目**: huanchong-99
- **行号**: L14418
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 14418min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 86.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg3`
- **项目**: huanchong-99
- **行号**: L1445
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1445min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 86.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg4`
- **项目**: huanchong-99
- **行号**: L1765
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1765min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 86.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg5`
- **项目**: huanchong-99
- **行号**: L1785
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1785min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 86.5 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg6`
- **项目**: huanchong-99
- **行号**: L2025
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2025min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 86.6 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg7`
- **项目**: huanchong-99
- **行号**: L2045
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2045min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 86.7 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg8`
- **项目**: huanchong-99
- **行号**: L2065
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2065min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 86.8 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg9`
- **项目**: huanchong-99
- **行号**: L2585
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2585min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 86.9 Refactor this function to reduce its Cognitive Complexity from 20 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg-`
- **项目**: huanchong-99
- **行号**: L26810
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 26810min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 86.10 Complete the task associated to this "TODO" comment. ✅ 已修复

- **问题ID**: `AZyVwegaZ9DOUQdEsGg_`
- **项目**: huanchong-99
- **行号**: L5330
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5330min effort
- **创建时间**: 1 month ago
- **标签**: cwe

---

## 87. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/SplitButton.tsx

> 该文件共有 **1** 个问题

### 87.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwegIZ9DOUQdEsGgz`
- **项目**: huanchong-99
- **行号**: L245
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 245min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 88. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/StatusPill.tsx

> 该文件共有 **4** 个问题

### 88.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwefHZ9DOUQdEsGgg`
- **项目**: huanchong-99
- **行号**: L385
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 385min effort
- **创建时间**: 27 days ago
- **标签**: react, type-dependent

### 88.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVwefHZ9DOUQdEsGgh`
- **项目**: huanchong-99
- **行号**: L605
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 605min effort
- **创建时间**: 27 days ago
- **标签**: accessibility, react

### 88.3 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwefHZ9DOUQdEsGgi`
- **项目**: huanchong-99
- **行号**: L605
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 605min effort
- **创建时间**: 27 days ago
- **标签**: accessibility, react

### 88.4 `tabIndex` should only be declared on interactive elements. ✅ 已修复

- **问题ID**: `AZyVwefHZ9DOUQdEsGgj`
- **项目**: huanchong-99
- **行号**: L625
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 625min effort
- **创建时间**: 27 days ago
- **标签**: accessibility, react

---

## 89. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/Toolbar.tsx

> 该文件共有 **3** 个问题

### 89.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweenZ9DOUQdEsGgZ`
- **项目**: huanchong-99
- **行号**: L245
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 245min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 89.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweenZ9DOUQdEsGga`
- **项目**: huanchong-99
- **行号**: L375
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 375min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 89.3 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweenZ9DOUQdEsGgb`
- **项目**: huanchong-99
- **行号**: L665
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 665min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 90. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/Tooltip.tsx

> 该文件共有 **1** 个问题

### 90.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwegBZ9DOUQdEsGgy`
- **项目**: huanchong-99
- **行号**: L125
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 125min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 91. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/ViewHeader.tsx

> 该文件共有 **1** 个问题

### 91.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweevZ9DOUQdEsGgc`
- **项目**: huanchong-99
- **行号**: L135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 135min effort
- **创建时间**: 27 days ago
- **标签**: react, type-dependent

---

## 92. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/WorkspaceSummary.tsx

> 该文件共有 **2** 个问题

### 92.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwefxZ9DOUQdEsGgu`
- **项目**: huanchong-99
- **行号**: L395
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 395min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 92.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwefyZ9DOUQdEsGgv`
- **项目**: huanchong-99
- **行号**: L1755
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1755min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 93. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/nodes/image-node.tsx

> 该文件共有 **6** 个问题

### 93.1 Refactor this function to reduce its Cognitive Complexity from 18 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweOZZ9DOUQdEsGaw`
- **项目**: huanchong-99
- **行号**: L378
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 378min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 93.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweOZZ9DOUQdEsGax`
- **项目**: huanchong-99
- **行号**: L375
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 375min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 93.3 'nodeKey' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweOZZ9DOUQdEsGay`
- **项目**: huanchong-99
- **行号**: L425
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 425min effort
- **创建时间**: 1 month ago
- **标签**: react

### 93.4 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweOZZ9DOUQdEsGaz`
- **项目**: huanchong-99
- **行号**: L1302
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1302min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 93.5 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweOZZ9DOUQdEsGa0`
- **项目**: huanchong-99
- **行号**: L1495
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1495min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 93.6 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweOZZ9DOUQdEsGa1`
- **项目**: huanchong-99
- **行号**: L1495
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1495min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 94. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/nodes/pr-comment-node.tsx

> 该文件共有 **3** 个问题

### 94.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweOOZ9DOUQdEsGat`
- **项目**: huanchong-99
- **行号**: L325
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 325min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 94.2 'nodeKey' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweOOZ9DOUQdEsGau`
- **项目**: huanchong-99
- **行号**: L375
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 375min effort
- **创建时间**: 1 month ago
- **标签**: react

### 94.3 Prefer `.dataset` over `setAttribute(…)`. ✅ 已修复

- **问题ID**: `AZyVweOOZ9DOUQdEsGav`
- **项目**: huanchong-99
- **行号**: L825
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 825min effort
- **创建时间**: 1 month ago
- **标签**: api, dom, ...

---

## 95. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/plugins/code-block-shortcut-plugin.tsx

> 该文件共有 **2** 个问题

### 95.1 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVweNaZ9DOUQdEsGai`
- **项目**: huanchong-99
- **行号**: L6820
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 6820min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 95.2 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVweNaZ9DOUQdEsGaj`
- **项目**: huanchong-99
- **行号**: L8120
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 8120min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

---

## 96. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/plugins/file-tag-typeahead-plugin.tsx

> 该文件共有 **6** 个问题

### 96.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweNzZ9DOUQdEsGan`
- **项目**: huanchong-99
- **行号**: L655
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 655min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 96.2 Refactor this function to reduce its Cognitive Complexity from 22 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweNzZ9DOUQdEsGao`
- **项目**: huanchong-99
- **行号**: L11312
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 11312min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 96.3 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweNzZ9DOUQdEsGap`
- **项目**: huanchong-99
- **行号**: L2115
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2115min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 96.4 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweNzZ9DOUQdEsGaq`
- **项目**: huanchong-99
- **行号**: L2115
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2115min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 96.5 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweNzZ9DOUQdEsGar`
- **项目**: huanchong-99
- **行号**: L2555
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2555min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 96.6 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweNzZ9DOUQdEsGas`
- **项目**: huanchong-99
- **行号**: L2555
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2555min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 97. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/plugins/read-only-link-plugin.tsx

> 该文件共有 **1** 个问题

### 97.1 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVweNhZ9DOUQdEsGak`
- **项目**: huanchong-99
- **行号**: L10920
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 10920min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

---

## 98. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/plugins/toolbar-plugin.tsx

> 该文件共有 **2** 个问题

### 98.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweNpZ9DOUQdEsGal`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 98.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweNpZ9DOUQdEsGam`
- **项目**: huanchong-99
- **行号**: L962
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 962min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 99. huanchong-99SoloDawnfrontend/.../components/workflow/validators/index.ts

> 该文件共有 **7** 个问题

### 99.1 Use `export…from` to re-export `validateStep0Project`. ✅ 已修复

- **问题ID**: `AZyVweUzZ9DOUQdEsGdI`
- **项目**: huanchong-99
- **行号**: L325
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 325min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 99.2 Use `export…from` to re-export `validateStep1Basic`. ✅ 已修复

- **问题ID**: `AZyVweUzZ9DOUQdEsGdJ`
- **项目**: huanchong-99
- **行号**: L335
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 335min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 99.3 Use `export…from` to re-export `validateStep2Tasks`. ✅ 已修复

- **问题ID**: `AZyVweUzZ9DOUQdEsGdK`
- **项目**: huanchong-99
- **行号**: L345
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 345min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 99.4 Use `export…from` to re-export `validateStep3Models`. ✅ 已修复

- **问题ID**: `AZyVweUzZ9DOUQdEsGdL`
- **项目**: huanchong-99
- **行号**: L355
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 355min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 99.5 Use `export…from` to re-export `validateStep4Terminals`. ✅ 已修复

- **问题ID**: `AZyVweUzZ9DOUQdEsGdM`
- **项目**: huanchong-99
- **行号**: L365
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 365min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 99.6 Use `export…from` to re-export `validateStep5Commands`. ✅ 已修复

- **问题ID**: `AZyVweUzZ9DOUQdEsGdN`
- **项目**: huanchong-99
- **行号**: L375
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 375min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 99.7 Use `export…from` to re-export `validateStep6Advanced`. ✅ 已修复

- **问题ID**: `AZyVweUzZ9DOUQdEsGdO`
- **项目**: huanchong-99
- **行号**: L385
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 385min effort
- **创建时间**: 1 month ago
- **标签**: convention

---

## 100. huanchong-99SoloDawnfrontend/.../components/workflow/validators/step5Commands.ts

> 该文件共有 **1** 个问题

### 100.1 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweU7Z9DOUQdEsGdP`
- **项目**: huanchong-99
- **行号**: L75
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 75min effort
- **创建时间**: 1 month ago
- **标签**: confusing, type-dependent

---

## 101. huanchong-99SoloDawnfrontend/.../tasks/TaskDetails/preview/DevServerLogsView.tsx

> 该文件共有 **2** 个问题

### 101.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweY_Z9DOUQdEsGep`
- **项目**: huanchong-99
- **行号**: L175
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 175min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 101.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweY_Z9DOUQdEsGeq`
- **项目**: huanchong-99
- **行号**: L645
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 645min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 102. huanchong-99SoloDawnfrontend/.../tasks/TaskDetails/preview/NoServerContent.tsx

> 该文件共有 **1** 个问题

### 102.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweZHZ9DOUQdEsGer`
- **项目**: huanchong-99
- **行号**: L325
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 325min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 103. huanchong-99SoloDawnfrontend/.../tasks/TaskDetails/preview/PreviewToolbar.tsx

> 该文件共有 **2** 个问题

### 103.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweY4Z9DOUQdEsGen`
- **项目**: huanchong-99
- **行号**: L265
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 265min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 103.2 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweY4Z9DOUQdEsGeo`
- **项目**: huanchong-99
- **行号**: L782
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 782min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 104. huanchong-99SoloDawnfrontend/.../tasks/TaskDetails/preview/ReadyContent.tsx

> 该文件共有 **1** 个问题

### 104.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweYwZ9DOUQdEsGem`
- **项目**: huanchong-99
- **行号**: L95
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 95min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 105. huanchong-99SoloDawnfrontend/.../ui-new/dialogs/commandBar/useCommandBarState.ts

> 该文件共有 **2** 个问题

### 105.1 Refactor this function to reduce its Cognitive Complexity from 32 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweluZ9DOUQdEsGia`
- **项目**: huanchong-99
- **行号**: L3922
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3922min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 105.2 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVweluZ9DOUQdEsGib`
- **项目**: huanchong-99
- **行号**: L951
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 951min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 106. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatApprovalCard.tsx

> 该文件共有 **1** 个问题

### 106.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwebEZ9DOUQdEsGfe`
- **项目**: huanchong-99
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 155min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 107. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatAssistantMessage.tsx

> 该文件共有 **1** 个问题

### 107.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwecNZ9DOUQdEsGf2`
- **项目**: huanchong-99
- **行号**: L85
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 85min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 108. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatEntryContainer.tsx

> 该文件共有 **3** 个问题

### 108.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwebcZ9DOUQdEsGfo`
- **项目**: huanchong-99
- **行号**: L615
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 615min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 108.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVwebcZ9DOUQdEsGfp`
- **项目**: huanchong-99
- **行号**: L925
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 925min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 108.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVwebcZ9DOUQdEsGfq`
- **项目**: huanchong-99
- **行号**: L925
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 925min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 109. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatErrorMessage.tsx

> 该文件共有 **4** 个问题

### 109.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwecUZ9DOUQdEsGf3`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 115min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 109.2 Elements with the 'button' interactive role must be focusable. ✅ 已修复

- **问题ID**: `AZyVwecUZ9DOUQdEsGf4`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 109.3 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwecUZ9DOUQdEsGf5`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 109.4 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVwecUZ9DOUQdEsGf6`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 110. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatFileEntry.tsx

> 该文件共有 **5** 个问题

### 110.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweb2Z9DOUQdEsGft`
- **项目**: huanchong-99
- **行号**: L255
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 255min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 110.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweb2Z9DOUQdEsGfu`
- **项目**: huanchong-99
- **行号**: L605
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 605min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 110.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweb2Z9DOUQdEsGfv`
- **项目**: huanchong-99
- **行号**: L605
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 605min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 110.4 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweb2Z9DOUQdEsGfw`
- **项目**: huanchong-99
- **行号**: L1305
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1305min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 110.5 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweb2Z9DOUQdEsGfx`
- **项目**: huanchong-99
- **行号**: L1305
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1305min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 111. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatMarkdown.tsx

> 该文件共有 **1** 个问题

### 111.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweblZ9DOUQdEsGfr`
- **项目**: huanchong-99
- **行号**: L125
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 125min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 112. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatScriptEntry.tsx

> 该文件共有 **2** 个问题

### 112.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweckZ9DOUQdEsGf8`
- **项目**: huanchong-99
- **行号**: L175
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 175min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 112.2 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweckZ9DOUQdEsGf9`
- **项目**: huanchong-99
- **行号**: L545
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 545min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 113. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatSystemMessage.tsx

> 该文件共有 **4** 个问题

### 113.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwebLZ9DOUQdEsGff`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 115min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 113.2 Elements with the 'button' interactive role must be focusable. ✅ 已修复

- **问题ID**: `AZyVwebLZ9DOUQdEsGfg`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 113.3 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwebLZ9DOUQdEsGfh`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 113.4 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVwebLZ9DOUQdEsGfi`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 114. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatThinkingMessage.tsx

> 该文件共有 **1** 个问题

### 114.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweccZ9DOUQdEsGf7`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 115min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 115. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatTodoList.tsx

> 该文件共有 **4** 个问题

### 115.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwea8Z9DOUQdEsGfa`
- **项目**: huanchong-99
- **行号**: L245
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 245min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 115.2 Elements with the 'button' interactive role must be focusable. ✅ 已修复

- **问题ID**: `AZyVwea8Z9DOUQdEsGfb`
- **项目**: huanchong-99
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 295min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 115.3 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwea8Z9DOUQdEsGfc`
- **项目**: huanchong-99
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 115.4 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVwea8Z9DOUQdEsGfd`
- **项目**: huanchong-99
- **行号**: L295
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 295min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 116. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatToolSummary.tsx

> 该文件共有 **3** 个问题

### 116.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweb9Z9DOUQdEsGfy`
- **项目**: huanchong-99
- **行号**: L535
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 535min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 116.2 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweb9Z9DOUQdEsGfz`
- **项目**: huanchong-99
- **行号**: L535
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 535min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 116.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweb9Z9DOUQdEsGf0`
- **项目**: huanchong-99
- **行号**: L535
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 535min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 117. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatUserMessage.tsx

> 该文件共有 **1** 个问题

### 117.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwecFZ9DOUQdEsGf1`
- **项目**: huanchong-99
- **行号**: L165
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 165min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 118. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/DiffViewCard.tsx

> 该文件共有 **5** 个问题

### 118.1 Refactor this function to reduce its Cognitive Complexity from 18 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwebUZ9DOUQdEsGfj`
- **项目**: huanchong-99
- **行号**: L688
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 688min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 118.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwebUZ9DOUQdEsGfk`
- **项目**: huanchong-99
- **行号**: L1635
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1635min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 118.3 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVwebUZ9DOUQdEsGfl`
- **项目**: huanchong-99
- **行号**: L1885
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1885min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 118.4 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVwebUZ9DOUQdEsGfm`
- **项目**: huanchong-99
- **行号**: L1885
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1885min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 118.5 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwebUZ9DOUQdEsGfn`
- **项目**: huanchong-99
- **行号**: L2485
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2485min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 119. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ToolStatusDot.tsx

> 该文件共有 **1** 个问题

### 119.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwebsZ9DOUQdEsGfs`
- **项目**: huanchong-99
- **行号**: L95
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 95min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 120. huanchong-99SoloDawnfrontend/.../ui/wysiwyg/transformers/table-transformer.ts

> 该文件共有 **3** 个问题

### 120.1 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweNRZ9DOUQdEsGaf`
- **项目**: huanchong-99
- **行号**: L235
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 235min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 120.2 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweNRZ9DOUQdEsGag`
- **项目**: huanchong-99
- **行号**: L575
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 575min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 120.3 `String.raw` should be used to avoid escaping `\`. ✅ 已修复

- **问题ID**: `AZyVweNRZ9DOUQdEsGah`
- **项目**: huanchong-99
- **行号**: L575
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 575min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 121. huanchong-99SoloDawnfrontend/src/App.tsx

> 该文件共有 **1** 个问题

### 121.1 Remove this redundant jump. ✅ 已修复

- **问题ID**: `AZyVwe3TZ9DOUQdEsGoq`
- **项目**: huanchong-99
- **行号**: L1051
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1051min effort
- **创建时间**: 1 month ago
- **标签**: clumsy, redundant

---

## 122. huanchong-99SoloDawnfrontend/src/components/AgentAvailabilityIndicator.tsx

> 该文件共有 **1** 个问题

### 122.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwerOZ9DOUQdEsGkL`
- **项目**: huanchong-99
- **行号**: L95
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 95min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 123. huanchong-99SoloDawnfrontend/src/components/ConfigProvider.tsx

> 该文件共有 **3** 个问题

### 123.1 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweqoZ9DOUQdEsGkE`
- **项目**: huanchong-99
- **行号**: L161
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 161min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 123.2 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweqoZ9DOUQdEsGkF`
- **项目**: huanchong-99
- **行号**: L171
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 171min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 123.3 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweqoZ9DOUQdEsGkG`
- **项目**: huanchong-99
- **行号**: L675
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 675min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 124. huanchong-99SoloDawnfrontend/src/components/DiffCard.tsx

> 该文件共有 **6** 个问题

### 124.1 Refactor this function to reduce its Cognitive Complexity from 17 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweomZ9DOUQdEsGjM`
- **项目**: huanchong-99
- **行号**: L777
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 777min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 124.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweomZ9DOUQdEsGjN`
- **项目**: huanchong-99
- **行号**: L775
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 775min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 124.3 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweomZ9DOUQdEsGjO`
- **项目**: huanchong-99
- **行号**: L1842
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1842min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 124.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweomZ9DOUQdEsGjP`
- **项目**: huanchong-99
- **行号**: L3285
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3285min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 124.5 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweomZ9DOUQdEsGjQ`
- **项目**: huanchong-99
- **行号**: L3295
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3295min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 124.6 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweomZ9DOUQdEsGjR`
- **项目**: huanchong-99
- **行号**: L3315
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3315min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 125. huanchong-99SoloDawnfrontend/src/components/DiffViewSwitch.tsx

> 该文件共有 **1** 个问题

### 125.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwer9Z9DOUQdEsGkQ`
- **项目**: huanchong-99
- **行号**: L225
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 225min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 126. huanchong-99SoloDawnfrontend/src/components/EditorAvailabilityIndicator.tsx

> 该文件共有 **1** 个问题

### 126.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwesiZ9DOUQdEsGkX`
- **项目**: huanchong-99
- **行号**: L135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 135min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 127. huanchong-99SoloDawnfrontend/src/components/ExecutorConfigForm.tsx

> 该文件共有 **2** 个问题

### 127.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwesHZ9DOUQdEsGkR`
- **项目**: huanchong-99
- **行号**: L285
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 285min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 127.2 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVwesHZ9DOUQdEsGkS`
- **项目**: huanchong-99
- **行号**: L1585
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1585min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 128. huanchong-99SoloDawnfrontend/src/components/SearchBar.tsx

> 该文件共有 **1** 个问题

### 128.1 'onClear' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVwesqZ9DOUQdEsGkY`
- **项目**: huanchong-99
- **行号**: L125
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 125min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 129. huanchong-99SoloDawnfrontend/src/components/TagManager.tsx

> 该文件共有 **1** 个问题

### 129.1 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwerFZ9DOUQdEsGkK`
- **项目**: huanchong-99
- **行号**: L401
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

---

## 130. huanchong-99SoloDawnfrontend/src/components/ThemeProvider.tsx

> 该文件共有 **5** 个问题

### 130.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwepIZ9DOUQdEsGjV`
- **项目**: huanchong-99
- **行号**: L215
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 215min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 130.2 useState call is not destructured into value + setter pair ✅ 已修复

- **问题ID**: `AZyVwepIZ9DOUQdEsGjW`
- **项目**: huanchong-99
- **行号**: L265
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 265min effort
- **创建时间**: 1 month ago
- **标签**: react

### 130.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwepIZ9DOUQdEsGjX`
- **项目**: huanchong-99
- **行号**: L342
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 342min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 130.4 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwepIZ9DOUQdEsGjY`
- **项目**: huanchong-99
- **行号**: L392
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 392min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 130.5 The 'value' object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwepIZ9DOUQdEsGjZ`
- **项目**: huanchong-99
- **行号**: L555
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 555min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 131. huanchong-99SoloDawnfrontend/src/components/agents/AgentIcon.tsx

> 该文件共有 **2** 个问题

### 131.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwem7Z9DOUQdEsGit`
- **项目**: huanchong-99
- **行号**: L112
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 112min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 131.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwem7Z9DOUQdEsGiu`
- **项目**: huanchong-99
- **行号**: L445
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 445min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 132. huanchong-99SoloDawnfrontend/src/components/board/StatusBar.tsx

> 该文件共有 **1** 个问题

### 132.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwenUZ9DOUQdEsGi3`
- **项目**: huanchong-99
- **行号**: L85
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 85min effort
- **创建时间**: 21 days ago
- **标签**: react, type-dependent

---

## 133. huanchong-99SoloDawnfrontend/src/components/board/TaskCard.tsx

> 该文件共有 **1** 个问题

### 133.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwenMZ9DOUQdEsGi2`
- **项目**: huanchong-99
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 155min effort
- **创建时间**: 22 days ago
- **标签**: react, type-dependent

---

## 134. huanchong-99SoloDawnfrontend/src/components/board/TerminalActivityPanel.tsx

> 该文件共有 **7** 个问题

### 134.1 `ACTIVE_STATUSES` should be a `Set`, and use `ACTIVE_STATUSES.has()` to check existence or non-existence. ✅ 已修复

- **问题ID**: `AZyVwenDZ9DOUQdEsGiv`
- **项目**: huanchong-99
- **行号**: L235
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 235min effort
- **创建时间**: 25 days ago
- **标签**: optimization, performance

### 134.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwenDZ9DOUQdEsGiw`
- **项目**: huanchong-99
- **行号**: L485
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 485min effort
- **创建时间**: 25 days ago
- **标签**: react, type-dependent

### 134.3 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVwenDZ9DOUQdEsGix`
- **项目**: huanchong-99
- **行号**: L745
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 745min effort
- **创建时间**: 25 days ago
- **标签**: jsx, performance, ...

### 134.4 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwenDZ9DOUQdEsGiy`
- **项目**: huanchong-99
- **行号**: L855
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 855min effort
- **创建时间**: 25 days ago
- **标签**: react, type-dependent

### 134.5 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwenDZ9DOUQdEsGiz`
- **项目**: huanchong-99
- **行号**: L945
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 945min effort
- **创建时间**: 25 days ago
- **标签**: confusing

### 134.6 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwenDZ9DOUQdEsGi0`
- **项目**: huanchong-99
- **行号**: L955
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 955min effort
- **创建时间**: 25 days ago
- **标签**: confusing

### 134.7 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwenDZ9DOUQdEsGi1`
- **项目**: huanchong-99
- **行号**: L1035
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1035min effort
- **创建时间**: 27 days ago
- **标签**: react, type-dependent

---

## 135. huanchong-99SoloDawnfrontend/src/components/board/TerminalDots.tsx

> 该文件共有 **8** 个问题

### 135.1 "working" is overridden by string in this union type. ✅ 已修复

- **问题ID**: `AZyVwentZ9DOUQdEsGi-`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 25 days ago
- **标签**: redundant, type-dependent

### 135.2 "not_started" is overridden by string in this union type. ✅ 已修复

- **问题ID**: `AZyVwentZ9DOUQdEsGi7`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 25 days ago
- **标签**: redundant, type-dependent

### 135.3 "starting" is overridden by string in this union type. ✅ 已修复

- **问题ID**: `AZyVwentZ9DOUQdEsGi8`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 25 days ago
- **标签**: redundant, type-dependent

### 135.4 "waiting" is overridden by string in this union type. ✅ 已修复

- **问题ID**: `AZyVwentZ9DOUQdEsGi9`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 25 days ago
- **标签**: redundant, type-dependent

### 135.5 "completed" is overridden by string in this union type. ✅ 已修复

- **问题ID**: `AZyVwentZ9DOUQdEsGi_`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 25 days ago
- **标签**: redundant, type-dependent

### 135.6 "failed" is overridden by string in this union type. ✅ 已修复

- **问题ID**: `AZyVwentZ9DOUQdEsGjA`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 25 days ago
- **标签**: redundant, type-dependent

### 135.7 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwentZ9DOUQdEsGjB`
- **项目**: huanchong-99
- **行号**: L415
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 415min effort
- **创建时间**: 25 days ago
- **标签**: react, type-dependent

### 135.8 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVwentZ9DOUQdEsGjC`
- **项目**: huanchong-99
- **行号**: L645
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 645min effort
- **创建时间**: 28 days ago
- **标签**: jsx, performance, ...

---

## 136. huanchong-99SoloDawnfrontend/src/components/board/WorkflowCard.tsx

> 该文件共有 **1** 个问题

### 136.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwen1Z9DOUQdEsGjD`
- **项目**: huanchong-99
- **行号**: L105
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 105min effort
- **创建时间**: 28 days ago
- **标签**: react, type-dependent

---

## 137. huanchong-99SoloDawnfrontend/src/components/board/WorkflowKanbanBoard.tsx

> 该文件共有 **2** 个问题

### 137.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwenkZ9DOUQdEsGi5`
- **项目**: huanchong-99
- **行号**: L385
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 385min effort
- **创建时间**: 22 days ago
- **标签**: react, type-dependent

### 137.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwenkZ9DOUQdEsGi6`
- **项目**: huanchong-99
- **行号**: L645
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 645min effort
- **创建时间**: 28 days ago
- **标签**: react, type-dependent

---

## 138. huanchong-99SoloDawnfrontend/src/components/board/WorkflowSidebar.tsx

> 该文件共有 **1** 个问题

### 138.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwenbZ9DOUQdEsGi4`
- **项目**: huanchong-99
- **行号**: L135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 135min effort
- **创建时间**: 28 days ago
- **标签**: react, type-dependent

---

## 139. huanchong-99SoloDawnfrontend/src/components/common/ProfileVariantBadge.tsx

> 该文件共有 **1** 个问题

### 139.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwesQZ9DOUQdEsGkT`
- **项目**: huanchong-99
- **行号**: L95
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 95min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 140. huanchong-99SoloDawnfrontend/src/components/common/RawLogText.tsx

> 该文件共有 **3** 个问题

### 140.1 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVwesZZ9DOUQdEsGkU`
- **项目**: huanchong-99
- **行号**: L405
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 405min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 140.2 `String.raw` should be used to avoid escaping `\`. ✅ 已修复

- **问题ID**: `AZyVwesZZ9DOUQdEsGkV`
- **项目**: huanchong-99
- **行号**: L405
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 405min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 140.3 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVwesZZ9DOUQdEsGkW`
- **项目**: huanchong-99
- **行号**: L695
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 695min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 141. huanchong-99SoloDawnfrontend/src/components/debug/TerminalDebugView.tsx

> 该文件共有 **1** 个问题

### 141.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweo_Z9DOUQdEsGjU`
- **项目**: huanchong-99
- **行号**: L105
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 105min effort
- **创建时间**: 27 days ago
- **标签**: react, type-dependent

---

## 142. huanchong-99SoloDawnfrontend/src/components/debug/TerminalSidebar.tsx

> 该文件共有 **1** 个问题

### 142.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweo4Z9DOUQdEsGjT`
- **项目**: huanchong-99
- **行号**: L105
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 105min effort
- **创建时间**: 27 days ago
- **标签**: react, type-dependent

---

## 143. huanchong-99SoloDawnfrontend/src/components/dialogs/global/FeatureShowcaseDialog.tsx

> 该文件共有 **1** 个问题

### 143.1 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVweUQZ9DOUQdEsGdB`
- **项目**: huanchong-99
- **行号**: L1045
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1045min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 144. huanchong-99SoloDawnfrontend/src/components/dialogs/global/OAuthDialog.tsx

> 该文件共有 **1** 个问题

### 144.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweTNZ9DOUQdEsGdA`
- **项目**: huanchong-99
- **行号**: L1192
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1192min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 145. huanchong-99SoloDawnfrontend/src/components/dialogs/global/OnboardingDialog.tsx

> 该文件共有 **1** 个问题

### 145.1 Provide a compare function to avoid sorting elements alphabetically. ✅ 已修复

- **问题ID**: `AZyVweUYZ9DOUQdEsGdC`
- **项目**: huanchong-99
- **行号**: L11210
- **类型**: Bug
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 11210min effort
- **创建时间**: 1 month ago
- **标签**: bad-practice, type-dependent

---

## 146. huanchong-99SoloDawnfrontend/src/components/dialogs/org/CreateOrganizationDialog.tsx

> 该文件共有 **4** 个问题

### 146.1 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweUiZ9DOUQdEsGdD`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 635min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 146.2 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweUiZ9DOUQdEsGdE`
- **项目**: huanchong-99
- **行号**: L645
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 645min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 146.3 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweUiZ9DOUQdEsGdF`
- **项目**: huanchong-99
- **行号**: L655
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 655min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 146.4 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweUiZ9DOUQdEsGdG`
- **项目**: huanchong-99
- **行号**: L665
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 665min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

---

## 147. huanchong-99SoloDawnfrontend/src/components/dialogs/scripts/ScriptFixerDialog.tsx

> 该文件共有 **11** 个问题

### 147.1 Refactor this function to reduce its Cognitive Complexity from 22 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc1`
- **项目**: huanchong-99
- **行号**: L4812
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 4812min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 147.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc2`
- **项目**: huanchong-99
- **行号**: L725
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 725min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 147.3 Move this array "sort" operation to a separate statement or replace it with "toSorted". ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc3`
- **项目**: huanchong-99
- **行号**: L795
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 795min effort
- **创建时间**: 26 days ago
- **标签**: type-dependent

### 147.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc4`
- **项目**: huanchong-99
- **行号**: L1285
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1285min effort
- **创建时间**: 26 days ago
- **标签**: confusing

### 147.5 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc5`
- **项目**: huanchong-99
- **行号**: L1745
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1745min effort
- **创建时间**: 26 days ago
- **标签**: confusing

### 147.6 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc6`
- **项目**: huanchong-99
- **行号**: L2075
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2075min effort
- **创建时间**: 26 days ago
- **标签**: confusing

### 147.7 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc7`
- **项目**: huanchong-99
- **行号**: L2425
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2425min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 147.8 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc8`
- **项目**: huanchong-99
- **行号**: L2985
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2985min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 147.9 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc9`
- **项目**: huanchong-99
- **行号**: L3255
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3255min effort
- **创建时间**: 26 days ago
- **标签**: confusing

### 147.10 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc-`
- **项目**: huanchong-99
- **行号**: L3325
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3325min effort
- **创建时间**: 26 days ago
- **标签**: confusing

### 147.11 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc_`
- **项目**: huanchong-99
- **行号**: L3415
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3415min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 148. huanchong-99SoloDawnfrontend/src/components/dialogs/shared/FolderPickerDialog.tsx

> 该文件共有 **7** 个问题

### 148.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSzZ9DOUQdEsGct`
- **项目**: huanchong-99
- **行号**: L2375
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2375min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 148.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSzZ9DOUQdEsGcu`
- **项目**: huanchong-99
- **行号**: L2425
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2425min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 148.3 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweSzZ9DOUQdEsGcv`
- **项目**: huanchong-99
- **行号**: L2515
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2515min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 148.4 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSzZ9DOUQdEsGcw`
- **项目**: huanchong-99
- **行号**: L2515
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2515min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 148.5 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVweSzZ9DOUQdEsGcx`
- **项目**: huanchong-99
- **行号**: L2525
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2525min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

### 148.6 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweSzZ9DOUQdEsGcy`
- **项目**: huanchong-99
- **行号**: L2542
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2542min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 148.7 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSzZ9DOUQdEsGcz`
- **项目**: huanchong-99
- **行号**: L2645
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2645min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 149. huanchong-99SoloDawnfrontend/src/components/dialogs/shared/LoginRequiredPrompt.tsx

> 该文件共有 **2** 个问题

### 149.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweSfZ9DOUQdEsGcj`
- **项目**: huanchong-99
- **行号**: L225
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 225min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 149.2 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweSfZ9DOUQdEsGck`
- **项目**: huanchong-99
- **行号**: L405
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 405min effort
- **创建时间**: 1 month ago
- **标签**: confusing, type-dependent

---

## 150. huanchong-99SoloDawnfrontend/src/components/dialogs/shared/RepoPickerDialog.tsx

> 该文件共有 **8** 个问题

### 150.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweSqZ9DOUQdEsGcl`
- **项目**: huanchong-99
- **行号**: L1965
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1965min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 150.2 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSqZ9DOUQdEsGcm`
- **项目**: huanchong-99
- **行号**: L1965
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1965min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 150.3 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweSqZ9DOUQdEsGcn`
- **项目**: huanchong-99
- **行号**: L2135
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2135min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 150.4 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSqZ9DOUQdEsGco`
- **项目**: huanchong-99
- **行号**: L2135
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2135min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 150.5 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweSqZ9DOUQdEsGcp`
- **项目**: huanchong-99
- **行号**: L2695
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2695min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 150.6 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSqZ9DOUQdEsGcq`
- **项目**: huanchong-99
- **行号**: L2695
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2695min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 150.7 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweSqZ9DOUQdEsGcr`
- **项目**: huanchong-99
- **行号**: L3275
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3275min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 150.8 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSqZ9DOUQdEsGcs`
- **项目**: huanchong-99
- **行号**: L3275
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 3275min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 151. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/ChangeTargetBranchDialog.tsx

> 该文件共有 **1** 个问题

### 151.1 Destructuring assignment isChangingTargetBranch unnecessarily renamed. ✅ 已修复

- **问题ID**: `AZyVweRoZ9DOUQdEsGb-`
- **项目**: huanchong-99
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 1 month ago

---

## 152. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/CreatePRDialog.tsx

> 该文件共有 **4** 个问题

### 152.1 Refactor this function to reduce its Cognitive Complexity from 30 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweSCZ9DOUQdEsGcE`
- **项目**: huanchong-99
- **行号**: L11120
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 11120min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 152.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSCZ9DOUQdEsGcF`
- **项目**: huanchong-99
- **行号**: L1905
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1905min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 152.3 A fragment with only one child is redundant. ✅ 已修复

- **问题ID**: `AZyVweSCZ9DOUQdEsGcG`
- **项目**: huanchong-99
- **行号**: L2615
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 2615min effort
- **创建时间**: 1 month ago
- **标签**: react

### 152.4 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweSCZ9DOUQdEsGcH`
- **项目**: huanchong-99
- **行号**: L2732
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2732min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 153. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/EditorSelectionDialog.tsx

> 该文件共有 **1** 个问题

### 153.1 A form label must be associated with a control. ✅ 已修复

- **问题ID**: `AZyVweRXZ9DOUQdEsGb7`
- **项目**: huanchong-99
- **行号**: L645
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 645min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 154. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/GitActionsDialog.tsx

> 该文件共有 **2** 个问题

### 154.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweRfZ9DOUQdEsGb8`
- **项目**: huanchong-99
- **行号**: L345
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 345min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 154.2 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVweRfZ9DOUQdEsGb9`
- **项目**: huanchong-99
- **行号**: L575
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 575min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

---

## 155. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/PrCommentsDialog.tsx

> 该文件共有 **3** 个问题

### 155.1 Refactor this function to reduce its Cognitive Complexity from 17 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweR5Z9DOUQdEsGcB`
- **项目**: huanchong-99
- **行号**: L367
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 367min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 155.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweR5Z9DOUQdEsGcC`
- **项目**: huanchong-99
- **行号**: L1235
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1235min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 155.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweR5Z9DOUQdEsGcD`
- **项目**: huanchong-99
- **行号**: L1275
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1275min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 156. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/RestoreLogsDialog.tsx

> 该文件共有 **17** 个问题

### 156.1 Refactor this function to reduce its Cognitive Complexity from 46 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcS`
- **项目**: huanchong-99
- **行号**: L4936
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 4936min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 156.2 Passing a fragment to an HTML element is useless. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcT`
- **项目**: huanchong-99
- **行号**: L2035
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 2035min effort
- **创建时间**: 1 month ago
- **标签**: react

### 156.3 Elements with the 'switch' interactive role must be focusable. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcU`
- **项目**: huanchong-99
- **行号**: L2805
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2805min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 156.4 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcV`
- **项目**: huanchong-99
- **行号**: L2805
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2805min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 156.5 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcW`
- **项目**: huanchong-99
- **行号**: L3172
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3172min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 156.6 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcX`
- **项目**: huanchong-99
- **行号**: L3195
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3195min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 156.7 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcY`
- **项目**: huanchong-99
- **行号**: L3262
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3262min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 156.8 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcZ`
- **项目**: huanchong-99
- **行号**: L3285
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3285min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 156.9 Elements with the 'switch' interactive role must be focusable. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGca`
- **项目**: huanchong-99
- **行号**: L3385
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3385min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 156.10 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcb`
- **项目**: huanchong-99
- **行号**: L3385
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 3385min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 156.11 Elements with the 'switch' interactive role must be focusable. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcc`
- **项目**: huanchong-99
- **行号**: L4355
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4355min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 156.12 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcd`
- **项目**: huanchong-99
- **行号**: L4355
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 4355min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 156.13 Elements with the ARIA role "switch" must have the following attributes defined: aria-checked ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGce`
- **项目**: huanchong-99
- **行号**: L4375
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4375min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 156.14 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcf`
- **项目**: huanchong-99
- **行号**: L4485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4485min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 156.15 Elements with the 'switch' interactive role must be focusable. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcg`
- **项目**: huanchong-99
- **行号**: L4745
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4745min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 156.16 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGch`
- **项目**: huanchong-99
- **行号**: L4745
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 4745min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 156.17 Elements with the ARIA role "switch" must have the following attributes defined: aria-checked ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGci`
- **项目**: huanchong-99
- **行号**: L4765
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4765min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 157. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/ShareDialog.tsx

> 该文件共有 **6** 个问题

### 157.1 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweROZ9DOUQdEsGb1`
- **项目**: huanchong-99
- **行号**: L845
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 845min effort
- **创建时间**: 1 month ago
- **标签**: confusing, type-dependent

### 157.2 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweROZ9DOUQdEsGb2`
- **项目**: huanchong-99
- **行号**: L965
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 965min effort
- **创建时间**: 1 month ago
- **标签**: confusing, type-dependent

### 157.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweROZ9DOUQdEsGb4`
- **项目**: huanchong-99
- **行号**: L1335
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1335min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 157.4 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweROZ9DOUQdEsGb3`
- **项目**: huanchong-99
- **行号**: L1332
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1332min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 157.5 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweROZ9DOUQdEsGb6`
- **项目**: huanchong-99
- **行号**: L1395
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1395min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 157.6 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweROZ9DOUQdEsGb5`
- **项目**: huanchong-99
- **行号**: L1392
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1392min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 158. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/TaskFormDialog.tsx

> 该文件共有 **10** 个问题

### 158.1 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcI`
- **项目**: huanchong-99
- **行号**: L3265
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3265min effort
- **创建时间**: 1 month ago
- **标签**: confusing, type-dependent

### 158.2 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcJ`
- **项目**: huanchong-99
- **行号**: L57620
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 57620min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 158.3 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcK`
- **项目**: huanchong-99
- **行号**: L58320
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 58320min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 158.4 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcM`
- **项目**: huanchong-99
- **行号**: L58720
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 58720min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 158.5 Prefer `.some(…)` over `.find(…)`. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcL`
- **项目**: huanchong-99
- **行号**: L5875
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5875min effort
- **创建时间**: 1 month ago
- **标签**: performance, readability

### 158.6 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcN`
- **项目**: huanchong-99
- **行号**: L6615
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6615min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 158.7 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcO`
- **项目**: huanchong-99
- **行号**: L6645
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6645min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 158.8 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcP`
- **项目**: huanchong-99
- **行号**: L6655
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6655min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 158.9 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcQ`
- **项目**: huanchong-99
- **行号**: L6835
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 6835min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 158.10 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSNZ9DOUQdEsGcR`
- **项目**: huanchong-99
- **行号**: L6835
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 6835min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 159. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/ViewRelatedTasksDialog.tsx

> 该文件共有 **2** 个问题

### 159.1 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweRwZ9DOUQdEsGb_`
- **项目**: huanchong-99
- **行号**: L151
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 151min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 159.2 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweRwZ9DOUQdEsGcA`
- **项目**: huanchong-99
- **行号**: L161
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 161min effort
- **创建时间**: 1 month ago
- **标签**: es2015

---

## 160. huanchong-99SoloDawnfrontend/src/components/diff/CommentWidgetLine.tsx

> 该文件共有 **1** 个问题

### 160.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweqUZ9DOUQdEsGkC`
- **项目**: huanchong-99
- **行号**: L165
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 165min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 161. huanchong-99SoloDawnfrontend/src/components/diff/ReviewCommentRenderer.tsx

> 该文件共有 **1** 个问题

### 161.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweqeZ9DOUQdEsGkD`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 115min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 162. huanchong-99SoloDawnfrontend/src/components/ide/IdeIcon.tsx

> 该文件共有 **2** 个问题

### 162.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweq8Z9DOUQdEsGkI`
- **项目**: huanchong-99
- **行号**: L122
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 122min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 162.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweq8Z9DOUQdEsGkJ`
- **项目**: huanchong-99
- **行号**: L415
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 415min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 163. huanchong-99SoloDawnfrontend/src/components/ide/OpenInIdeButton.tsx

> 该文件共有 **1** 个问题

### 163.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweqyZ9DOUQdEsGkH`
- **项目**: huanchong-99
- **行号**: L125
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 125min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 164. huanchong-99SoloDawnfrontend/src/components/layout/Navbar.tsx

> 该文件共有 **1** 个问题

### 164.1 Use <hr> instead of the "separator" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwerYZ9DOUQdEsGkM`
- **项目**: huanchong-99
- **行号**: L625
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 625min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 165. huanchong-99SoloDawnfrontend/src/components/layout/NewDesignLayout.tsx

> 该文件共有 **1** 个问题

### 165.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwerqZ9DOUQdEsGkO`
- **项目**: huanchong-99
- **行号**: L1085
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1085min effort
- **创建时间**: 25 days ago
- **标签**: confusing

---

## 166. huanchong-99SoloDawnfrontend/src/components/layout/NormalLayout.tsx

> 该文件共有 **1** 个问题

### 166.1 A fragment with only one child is redundant. ✅ 已修复

- **问题ID**: `AZyVwerhZ9DOUQdEsGkN`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 115min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 167. huanchong-99SoloDawnfrontend/src/components/legacy-design/LegacyDesignScope.tsx

> 该文件共有 **1** 个问题

### 167.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwes6Z9DOUQdEsGka`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 115min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 168. huanchong-99SoloDawnfrontend/src/components/org/MemberListItem.tsx

> 该文件共有 **1** 个问题

### 168.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwepoZ9DOUQdEsGje`
- **项目**: huanchong-99
- **行号**: L265
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 265min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 169. huanchong-99SoloDawnfrontend/src/components/org/PendingInvitationItem.tsx

> 该文件共有 **2** 个问题

### 169.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwepdZ9DOUQdEsGjc`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 169.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwepdZ9DOUQdEsGjd`
- **项目**: huanchong-99
- **行号**: L222
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 222min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 170. huanchong-99SoloDawnfrontend/src/components/org/RemoteProjectItem.tsx

> 该文件共有 **2** 个问题

### 170.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwepSZ9DOUQdEsGja`
- **项目**: huanchong-99
- **行号**: L265
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 265min effort
- **创建时间**: 17 days ago
- **标签**: react, type-dependent

### 170.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwepSZ9DOUQdEsGjb`
- **项目**: huanchong-99
- **行号**: L412
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 412min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 171. huanchong-99SoloDawnfrontend/src/components/panels/AttemptHeaderActions.tsx

> 该文件共有 **3** 个问题

### 171.1 Compare with `undefined` directly instead of using `typeof`. ✅ 已修复

- **问题ID**: `AZyVweXrZ9DOUQdEsGeC`
- **项目**: huanchong-99
- **行号**: L412
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 412min effort
- **创建时间**: 1 month ago
- **标签**: readability, style

### 171.2 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVweXrZ9DOUQdEsGeD`
- **项目**: huanchong-99
- **行号**: L1065
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1065min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 171.3 Compare with `undefined` directly instead of using `typeof`. ✅ 已修复

- **问题ID**: `AZyVweXrZ9DOUQdEsGeE`
- **项目**: huanchong-99
- **行号**: L1282
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1282min effort
- **创建时间**: 1 month ago
- **标签**: readability, style

---

## 172. huanchong-99SoloDawnfrontend/src/components/panels/DiffsPanel.tsx

> 该文件共有 **5** 个问题

### 172.1 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweXjZ9DOUQdEsGd9`
- **项目**: huanchong-99
- **行号**: L171
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 171min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 172.2 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweXjZ9DOUQdEsGd-`
- **项目**: huanchong-99
- **行号**: L181
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 181min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 172.3 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweXjZ9DOUQdEsGd_`
- **项目**: huanchong-99
- **行号**: L515
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 515min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 172.4 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweXjZ9DOUQdEsGeA`
- **项目**: huanchong-99
- **行号**: L1615
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1615min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 172.5 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweXjZ9DOUQdEsGeB`
- **项目**: huanchong-99
- **行号**: L2365
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2365min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 173. huanchong-99SoloDawnfrontend/src/components/panels/PreviewPanel.tsx

> 该文件共有 **4** 个问题

### 173.1 Refactor this function to reduce its Cognitive Complexity from 16 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweXTZ9DOUQdEsGd1`
- **项目**: huanchong-99
- **行号**: L226
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 226min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 173.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweXTZ9DOUQdEsGd2`
- **项目**: huanchong-99
- **行号**: L1505
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1505min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 173.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweXTZ9DOUQdEsGd3`
- **项目**: huanchong-99
- **行号**: L1525
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1525min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 173.4 Ambiguous spacing after previous element a ✅ 已修复

- **问题ID**: `AZyVweXTZ9DOUQdEsGd4`
- **项目**: huanchong-99
- **行号**: L2575
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2575min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 174. huanchong-99SoloDawnfrontend/src/components/panels/TaskPanel.tsx

> 该文件共有 **4** 个问题

### 174.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweXbZ9DOUQdEsGd6`
- **项目**: huanchong-99
- **行号**: L505
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 505min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 174.2 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweXbZ9DOUQdEsGd5`
- **项目**: huanchong-99
- **行号**: L502
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 502min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 174.3 A fragment with only one child is redundant. ✅ 已修复

- **问题ID**: `AZyVweXbZ9DOUQdEsGd7`
- **项目**: huanchong-99
- **行号**: L1035
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 1035min effort
- **创建时间**: 26 days ago
- **标签**: react

### 174.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweXbZ9DOUQdEsGd8`
- **项目**: huanchong-99
- **行号**: L1375
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1375min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 175. huanchong-99SoloDawnfrontend/src/components/pipeline/MergeTerminalNode.tsx

> 该文件共有 **1** 个问题

### 175.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweoVZ9DOUQdEsGjK`
- **项目**: huanchong-99
- **行号**: L75
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 75min effort
- **创建时间**: 28 days ago
- **标签**: react, type-dependent

---

## 176. huanchong-99SoloDawnfrontend/src/components/pipeline/OrchestratorHeader.tsx

> 该文件共有 **1** 个问题

### 176.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwen-Z9DOUQdEsGjE`
- **项目**: huanchong-99
- **行号**: L235
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 235min effort
- **创建时间**: 21 days ago
- **标签**: react, type-dependent

---

## 177. huanchong-99SoloDawnfrontend/src/components/pipeline/TaskPipeline.tsx

> 该文件共有 **2** 个问题

### 177.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweoOZ9DOUQdEsGjI`
- **项目**: huanchong-99
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 155min effort
- **创建时间**: 21 days ago
- **标签**: react, type-dependent

### 177.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweoOZ9DOUQdEsGjJ`
- **项目**: huanchong-99
- **行号**: L585
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 585min effort
- **创建时间**: 28 days ago
- **标签**: react, type-dependent

---

## 178. huanchong-99SoloDawnfrontend/src/components/pipeline/TerminalDetailPanel.tsx

> 该文件共有 **1** 个问题

### 178.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweodZ9DOUQdEsGjL`
- **项目**: huanchong-99
- **行号**: L75
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 75min effort
- **创建时间**: 28 days ago
- **标签**: react, type-dependent

---

## 179. huanchong-99SoloDawnfrontend/src/components/pipeline/TerminalNode.tsx

> 该文件共有 **3** 个问题

### 179.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweoGZ9DOUQdEsGjF`
- **项目**: huanchong-99
- **行号**: L325
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 325min effort
- **创建时间**: 21 days ago
- **标签**: react, type-dependent

### 179.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweoGZ9DOUQdEsGjG`
- **项目**: huanchong-99
- **行号**: L535
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 535min effort
- **创建时间**: 21 days ago
- **标签**: confusing

### 179.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweoGZ9DOUQdEsGjH`
- **项目**: huanchong-99
- **行号**: L545
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 545min effort
- **创建时间**: 21 days ago
- **标签**: confusing

---

## 180. huanchong-99SoloDawnfrontend/src/components/rjsf/templates/FieldTemplate.tsx

> 该文件共有 **1** 个问题

### 180.1 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVweouZ9DOUQdEsGjS`
- **项目**: huanchong-99
- **行号**: L505
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 505min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 181. huanchong-99SoloDawnfrontend/src/components/settings/ExecutorProfileSelector.tsx

> 该文件共有 **1** 个问题

### 181.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwerzZ9DOUQdEsGkP`
- **项目**: huanchong-99
- **行号**: L165
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 165min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 182. huanchong-99SoloDawnfrontend/src/components/showcase/ShowcaseStageMedia.tsx

> 该文件共有 **1** 个问题

### 182.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweszZ9DOUQdEsGkZ`
- **项目**: huanchong-99
- **行号**: L245
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 245min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 183. huanchong-99SoloDawnfrontend/src/components/tasks/AgentSelector.tsx

> 该文件共有 **2** 个问题

### 183.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweatZ9DOUQdEsGfX`
- **项目**: huanchong-99
- **行号**: L215
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 215min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 183.2 Provide a compare function that depends on "String.localeCompare", to reliably sort elements alphabetically. ✅ 已修复

- **问题ID**: `AZyVweatZ9DOUQdEsGfY`
- **项目**: huanchong-99
- **行号**: L3010
- **类型**: Bug
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 3010min effort
- **创建时间**: 1 month ago
- **标签**: bad-practice, type-dependent

---

## 184. huanchong-99SoloDawnfrontend/src/components/tasks/BranchSelector.tsx

> 该文件共有 **3** 个问题

### 184.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweZgZ9DOUQdEsGe4`
- **项目**: huanchong-99
- **行号**: L1035
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1035min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 184.2 Expected a `for-of` loop instead of a `for` loop with this simple iteration. ✅ 已修复

- **问题ID**: `AZyVweZgZ9DOUQdEsGe5`
- **项目**: huanchong-99
- **行号**: L1675
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1675min effort
- **创建时间**: 1 month ago
- **标签**: clumsy

### 184.3 Move this component definition out of the parent component and pass data as props. ✅ 已修复

- **问题ID**: `AZyVweZgZ9DOUQdEsGe6`
- **项目**: huanchong-99
- **行号**: L2775
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2775min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 185. huanchong-99SoloDawnfrontend/src/components/tasks/ConfigSelector.tsx

> 该文件共有 **3** 个问题

### 185.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweZxZ9DOUQdEsGe9`
- **项目**: huanchong-99
- **行号**: L215
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 215min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 185.2 Provide a compare function that depends on "String.localeCompare", to reliably sort elements alphabetically. ✅ 已修复

- **问题ID**: `AZyVweZxZ9DOUQdEsGe-`
- **项目**: huanchong-99
- **行号**: L3110
- **类型**: Bug
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 3110min effort
- **创建时间**: 1 month ago
- **标签**: bad-practice, type-dependent

### 185.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweZxZ9DOUQdEsGe_`
- **项目**: huanchong-99
- **行号**: L765
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 765min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 186. huanchong-99SoloDawnfrontend/src/components/tasks/RepoBranchSelector.tsx

> 该文件共有 **1** 个问题

### 186.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweaBZ9DOUQdEsGfD`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 187. huanchong-99SoloDawnfrontend/src/components/tasks/RepoSelector.tsx

> 该文件共有 **1** 个问题

### 187.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweYoZ9DOUQdEsGel`
- **项目**: huanchong-99
- **行号**: L225
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 225min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 188. huanchong-99SoloDawnfrontend/src/components/tasks/SharedTaskCard.tsx

> 该文件共有 **1** 个问题

### 188.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwea1Z9DOUQdEsGfZ`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 189. huanchong-99SoloDawnfrontend/src/components/tasks/TaskCard.tsx

> 该文件共有 **3** 个问题

### 189.1 '@/hooks' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweZ6Z9DOUQdEsGfA`
- **项目**: huanchong-99
- **行号**: L71
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 71min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 189.2 '@/hooks' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweZ6Z9DOUQdEsGfB`
- **项目**: huanchong-99
- **行号**: L131
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 131min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 189.3 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweZ6Z9DOUQdEsGfC`
- **项目**: huanchong-99
- **行号**: L285
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 285min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 190. huanchong-99SoloDawnfrontend/src/components/tasks/TaskCardHeader.tsx

> 该文件共有 **1** 个问题

### 190.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweacZ9DOUQdEsGfR`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 191. huanchong-99SoloDawnfrontend/src/components/tasks/TaskFollowUpSection.tsx

> 该文件共有 **10** 个问题

### 191.1 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfH`
- **项目**: huanchong-99
- **行号**: L281
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 281min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 191.2 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfI`
- **项目**: huanchong-99
- **行号**: L501
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 501min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 191.3 '@/lib/api' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfJ`
- **项目**: huanchong-99
- **行号**: L571
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 571min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 191.4 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfK`
- **项目**: huanchong-99
- **行号**: L581
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 581min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 191.5 '@/lib/api' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfL`
- **项目**: huanchong-99
- **行号**: L591
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 591min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 191.6 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfM`
- **项目**: huanchong-99
- **行号**: L621
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 621min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 191.7 Refactor this function to reduce its Cognitive Complexity from 26 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfN`
- **项目**: huanchong-99
- **行号**: L6916
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 6916min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 191.8 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfO`
- **项目**: huanchong-99
- **行号**: L695
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 695min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 191.9 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfP`
- **项目**: huanchong-99
- **行号**: L6392
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6392min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 191.10 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweaUZ9DOUQdEsGfQ`
- **项目**: huanchong-99
- **行号**: L8085
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 8085min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 192. huanchong-99SoloDawnfrontend/src/components/tasks/Toolbar/GitOperations.tsx

> 该文件共有 **5** 个问题

### 192.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwealZ9DOUQdEsGfS`
- **项目**: huanchong-99
- **行号**: L465
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 465min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 192.2 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwealZ9DOUQdEsGfT`
- **项目**: huanchong-99
- **行号**: L911
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

### 192.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwealZ9DOUQdEsGfU`
- **项目**: huanchong-99
- **行号**: L1665
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1665min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 192.4 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwealZ9DOUQdEsGfV`
- **项目**: huanchong-99
- **行号**: L2431
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

### 192.5 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwealZ9DOUQdEsGfW`
- **项目**: huanchong-99
- **行号**: L4735
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4735min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 193. huanchong-99SoloDawnfrontend/src/components/tasks/UserAvatar.tsx

> 该文件共有 **3** 个问题

### 193.1 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVweaJZ9DOUQdEsGfE`
- **项目**: huanchong-99
- **行号**: L345
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 345min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 193.2 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVweaJZ9DOUQdEsGfF`
- **项目**: huanchong-99
- **行号**: L415
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 415min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 193.3 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVweaJZ9DOUQdEsGfG`
- **项目**: huanchong-99
- **行号**: L591
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

---

## 194. huanchong-99SoloDawnfrontend/src/components/tasks/follow-up/FollowUpConflictSection.tsx

> 该文件共有 **2** 个问题

### 194.1 'isEditable' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweZpZ9DOUQdEsGe7`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 115min effort
- **创建时间**: 1 month ago
- **标签**: react

### 194.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweZpZ9DOUQdEsGe8`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 195. huanchong-99SoloDawnfrontend/src/components/terminal/TerminalDebugView.tsx

> 该文件共有 **21** 个问题

### 195.1 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGjt`
- **项目**: huanchong-99
- **行号**: L325
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 325min effort
- **创建时间**: 14 days ago
- **标签**: es2021, readability

### 195.2 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGju`
- **项目**: huanchong-99
- **行号**: L335
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 335min effort
- **创建时间**: 14 days ago
- **标签**: es2021, readability

### 195.3 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGjv`
- **项目**: huanchong-99
- **行号**: L345
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 345min effort
- **创建时间**: 14 days ago
- **标签**: es2021, readability

### 195.4 Refactor this function to reduce its Cognitive Complexity from 23 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGjw`
- **项目**: huanchong-99
- **行号**: L3913
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3913min effort
- **创建时间**: 14 days ago
- **标签**: brain-overload

### 195.5 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGjx`
- **项目**: huanchong-99
- **行号**: L395
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 395min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 195.6 useState call is not destructured into value + setter pair ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGjy`
- **项目**: huanchong-99
- **行号**: L445
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 445min effort
- **创建时间**: 22 days ago
- **标签**: react

### 195.7 Unnecessary use of conditional expression for default assignment. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGjz`
- **项目**: huanchong-99
- **行号**: L615
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 615min effort
- **创建时间**: 1 month ago

### 195.8 '(payload as { message?: unknown }).message ?? ''' will use Object's default stringification format ('[object Object]') when stringified. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj0`
- **项目**: huanchong-99
- **行号**: L1315
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1315min effort
- **创建时间**: 14 days ago
- **标签**: object, string, ...

### 195.9 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj1`
- **项目**: huanchong-99
- **行号**: L1922
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1922min effort
- **创建时间**: 23 days ago
- **标签**: readability

### 195.10 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj2`
- **项目**: huanchong-99
- **行号**: L2815
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2815min effort
- **创建时间**: 22 days ago
- **标签**: confusing, type-dependent

### 195.11 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj3`
- **项目**: huanchong-99
- **行号**: L2955
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2955min effort
- **创建时间**: 22 days ago
- **标签**: confusing, type-dependent

### 195.12 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj4`
- **项目**: huanchong-99
- **行号**: L3285
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3285min effort
- **创建时间**: 14 days ago
- **标签**: confusing, type-dependent

### 195.13 Use <menu>, <ol>, or <ul> instead of the "list" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj5`
- **项目**: huanchong-99
- **行号**: L3565
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 3565min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 195.14 Use <li> instead of the "listitem" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj6`
- **项目**: huanchong-99
- **行号**: L3625
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 3625min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 195.15 The attribute aria-pressed is not supported by the role listitem. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj7`
- **项目**: huanchong-99
- **行号**: L3625
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3625min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 195.16 Interactive elements should not be assigned non-interactive roles. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj8`
- **项目**: huanchong-99
- **行号**: L3645
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3645min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 195.17 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj9`
- **项目**: huanchong-99
- **行号**: L4175
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4175min effort
- **创建时间**: 14 days ago
- **标签**: confusing

### 195.18 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj-`
- **项目**: huanchong-99
- **行号**: L4315
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4315min effort
- **创建时间**: 14 days ago
- **标签**: confusing

### 195.19 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGj_`
- **项目**: huanchong-99
- **行号**: L4455
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4455min effort
- **创建时间**: 14 days ago
- **标签**: confusing, type-dependent

### 195.20 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGkA`
- **项目**: huanchong-99
- **行号**: L4535
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4535min effort
- **创建时间**: 14 days ago
- **标签**: confusing

### 195.21 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweqKZ9DOUQdEsGkB`
- **项目**: huanchong-99
- **行号**: L4845
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4845min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 196. huanchong-99SoloDawnfrontend/src/components/terminal/TerminalEmulator.test.tsx

> 该文件共有 **8** 个问题

### 196.1 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwep-Z9DOUQdEsGjl`
- **项目**: huanchong-99
- **行号**: L3420
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3420min effort
- **创建时间**: 15 days ago
- **标签**: cwe

### 196.2 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwep-Z9DOUQdEsGjm`
- **项目**: huanchong-99
- **行号**: L3520
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3520min effort
- **创建时间**: 15 days ago
- **标签**: cwe

### 196.3 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwep-Z9DOUQdEsGjn`
- **项目**: huanchong-99
- **行号**: L3620
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3620min effort
- **创建时间**: 3 days ago
- **标签**: cwe

### 196.4 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwep-Z9DOUQdEsGjo`
- **项目**: huanchong-99
- **行号**: L3720
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3720min effort
- **创建时间**: 3 days ago
- **标签**: cwe

### 196.5 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwep-Z9DOUQdEsGjp`
- **项目**: huanchong-99
- **行号**: L3820
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3820min effort
- **创建时间**: 3 days ago
- **标签**: cwe

### 196.6 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwep-Z9DOUQdEsGjq`
- **项目**: huanchong-99
- **行号**: L3920
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3920min effort
- **创建时间**: 3 days ago
- **标签**: cwe

### 196.7 `String.raw` should be used to avoid escaping `\`. ✅ 已修复

- **问题ID**: `AZyVwep-Z9DOUQdEsGjr`
- **项目**: huanchong-99
- **行号**: L1025
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1025min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 196.8 `String.raw` should be used to avoid escaping `\`. ✅ 已修复

- **问题ID**: `AZyVwep-Z9DOUQdEsGjs`
- **项目**: huanchong-99
- **行号**: L1085
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1085min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 197. huanchong-99SoloDawnfrontend/src/components/terminal/TerminalEmulator.tsx

> 该文件共有 **6** 个问题

### 197.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwepzZ9DOUQdEsGjf`
- **项目**: huanchong-99
- **行号**: L782
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 782min effort
- **创建时间**: 3 days ago
- **标签**: es2020, portability

### 197.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwepzZ9DOUQdEsGjg`
- **项目**: huanchong-99
- **行号**: L1002
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1002min effort
- **创建时间**: 12 days ago
- **标签**: es2020, portability

### 197.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwepzZ9DOUQdEsGjh`
- **项目**: huanchong-99
- **行号**: L2502
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2502min effort
- **创建时间**: 3 days ago
- **标签**: es2020, portability

### 197.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwepzZ9DOUQdEsGjj`
- **项目**: huanchong-99
- **行号**: L3085
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3085min effort
- **创建时间**: 15 days ago
- **标签**: confusing

### 197.5 Unnecessary use of conditional expression for default assignment. ✅ 已修复

- **问题ID**: `AZyVwepzZ9DOUQdEsGji`
- **项目**: huanchong-99
- **行号**: L3085
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3085min effort
- **创建时间**: 15 days ago

### 197.6 Elements with ARIA roles must use a valid, non-abstract ARIA role. ✅ 已修复

- **问题ID**: `AZyVwepzZ9DOUQdEsGjk`
- **项目**: huanchong-99
- **行号**: L3485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 3485min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 198. huanchong-99SoloDawnfrontend/src/components/ui-new/actions/index.ts

> 该文件共有 **1** 个问题

### 198.1 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVwemhZ9DOUQdEsGim`
- **项目**: huanchong-99
- **行号**: L2872
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2872min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 199. huanchong-99SoloDawnfrontend/src/components/ui-new/actions/useActionVisibility.ts

> 该文件共有 **2** 个问题

### 199.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwemTZ9DOUQdEsGik`
- **项目**: huanchong-99
- **行号**: L515
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 515min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 199.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwemTZ9DOUQdEsGil`
- **项目**: huanchong-99
- **行号**: L535
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 535min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 200. huanchong-99SoloDawnfrontend/src/components/ui-new/dialogs/ChangeTargetDialog.tsx

> 该文件共有 **1** 个问题

### 200.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwel3Z9DOUQdEsGic`
- **项目**: huanchong-99
- **行号**: L315
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 315min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 201. huanchong-99SoloDawnfrontend/src/components/ui-new/dialogs/RebaseDialog.tsx

> 该文件共有 **3** 个问题

### 201.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwemBZ9DOUQdEsGid`
- **项目**: huanchong-99
- **行号**: L375
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 375min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 201.2 Refactor this function to reduce its Cognitive Complexity from 19 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwemBZ9DOUQdEsGie`
- **项目**: huanchong-99
- **行号**: L649
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 649min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 201.3 'err.message' will use Object's default stringification format ('[object Object]') when stringified. ✅ 已修复

- **问题ID**: `AZyVwemBZ9DOUQdEsGif`
- **项目**: huanchong-99
- **行号**: L1175
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1175min effort
- **创建时间**: 1 month ago
- **标签**: object, string, ...

---

## 202. huanchong-99SoloDawnfrontend/src/components/ui-new/dialogs/WorkspacesGuideDialog.tsx

> 该文件共有 **4** 个问题

### 202.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwemJZ9DOUQdEsGig`
- **项目**: huanchong-99
- **行号**: L542
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 542min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 202.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwemJZ9DOUQdEsGih`
- **项目**: huanchong-99
- **行号**: L552
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 552min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 202.3 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVwemJZ9DOUQdEsGii`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 635min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 202.4 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVwemJZ9DOUQdEsGij`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 635min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 203. huanchong-99SoloDawnfrontend/src/components/ui-new/hooks/usePreviewUrl.ts

> 该文件共有 **4** 个问题

### 203.1 Simplify this regular expression to reduce its complexity from 21 to the 20 allowed. ✅ 已修复

- **问题ID**: `AZyVwempZ9DOUQdEsGin`
- **项目**: huanchong-99
- **行号**: L1210
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1210min effort
- **创建时间**: 1 month ago
- **标签**: regex, type-dependent

### 203.2 Prefer `globalThis.window` over `window`. ✅ 已修复

- **问题ID**: `AZyVwempZ9DOUQdEsGio`
- **项目**: huanchong-99
- **行号**: L192
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 192min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 203.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwempZ9DOUQdEsGip`
- **项目**: huanchong-99
- **行号**: L202
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 202min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 203.4 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwempZ9DOUQdEsGiq`
- **项目**: huanchong-99
- **行号**: L601
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 601min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 204. huanchong-99SoloDawnfrontend/src/components/ui-new/scope/NewDesignScope.tsx

> 该文件共有 **2** 个问题

### 204.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwemyZ9DOUQdEsGir`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 204.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwemyZ9DOUQdEsGis`
- **项目**: huanchong-99
- **行号**: L355
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 355min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 205. huanchong-99SoloDawnfrontend/src/components/ui-new/views/FileTree.tsx

> 该文件共有 **1** 个问题

### 205.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwehFZ9DOUQdEsGhF`
- **项目**: huanchong-99
- **行号**: L335
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 335min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 206. huanchong-99SoloDawnfrontend/src/components/ui-new/views/FileTreeNode.tsx

> 该文件共有 **4** 个问题

### 206.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweheZ9DOUQdEsGhL`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 635min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 206.2 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweheZ9DOUQdEsGhM`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 635min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 206.3 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVweheZ9DOUQdEsGhN`
- **项目**: huanchong-99
- **行号**: L775
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 775min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

### 206.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweheZ9DOUQdEsGhO`
- **项目**: huanchong-99
- **行号**: L1065
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1065min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 207. huanchong-99SoloDawnfrontend/src/components/ui-new/views/FileTreePlaceholder.tsx

> 该文件共有 **1** 个问题

### 207.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweh2Z9DOUQdEsGhR`
- **项目**: huanchong-99
- **行号**: L85
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 85min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 208. huanchong-99SoloDawnfrontend/src/components/ui-new/views/FileTreeSearchBar.tsx

> 该文件共有 **1** 个问题

### 208.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweg9Z9DOUQdEsGhE`
- **项目**: huanchong-99
- **行号**: L135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 135min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 209. huanchong-99SoloDawnfrontend/src/components/ui-new/views/GitPanel.tsx

> 该文件共有 **2** 个问题

### 209.1 'onAddRepo' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVwegqZ9DOUQdEsGhB`
- **项目**: huanchong-99
- **行号**: L415
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 415min effort
- **创建时间**: 1 month ago
- **标签**: react

### 209.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwegqZ9DOUQdEsGhC`
- **项目**: huanchong-99
- **行号**: L465
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 465min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 210. huanchong-99SoloDawnfrontend/src/components/ui-new/views/GitPanelCreate.tsx

> 该文件共有 **1** 个问题

### 210.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwehuZ9DOUQdEsGhQ`
- **项目**: huanchong-99
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 211. huanchong-99SoloDawnfrontend/src/components/ui-new/views/Navbar.tsx

> 该文件共有 **1** 个问题

### 211.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwehnZ9DOUQdEsGhP`
- **项目**: huanchong-99
- **行号**: L735
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 735min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 212. huanchong-99SoloDawnfrontend/src/components/ui-new/views/PreviewBrowser.tsx

> 该文件共有 **8** 个问题

### 212.1 Refactor this function to reduce its Cognitive Complexity from 41 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweiAZ9DOUQdEsGhS`
- **项目**: huanchong-99
- **行号**: L6531
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 6531min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 212.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweiAZ9DOUQdEsGhT`
- **项目**: huanchong-99
- **行号**: L655
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 655min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 212.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweiAZ9DOUQdEsGhU`
- **项目**: huanchong-99
- **行号**: L2325
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2325min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 212.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweiAZ9DOUQdEsGhV`
- **项目**: huanchong-99
- **行号**: L2355
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2355min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 212.5 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweiAZ9DOUQdEsGhW`
- **项目**: huanchong-99
- **行号**: L3255
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3255min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 212.6 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweiAZ9DOUQdEsGhX`
- **项目**: huanchong-99
- **行号**: L3315
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3315min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 212.7 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweiAZ9DOUQdEsGhY`
- **项目**: huanchong-99
- **行号**: L3375
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3375min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 212.8 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweiAZ9DOUQdEsGhZ`
- **项目**: huanchong-99
- **行号**: L3585
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3585min effort
- **创建时间**: 25 days ago
- **标签**: confusing

---

## 213. huanchong-99SoloDawnfrontend/src/components/ui-new/views/PreviewControls.tsx

> 该文件共有 **2** 个问题

### 213.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwehWZ9DOUQdEsGhJ`
- **项目**: huanchong-99
- **行号**: L245
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 245min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 213.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwehWZ9DOUQdEsGhK`
- **项目**: huanchong-99
- **行号**: L905
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 905min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 214. huanchong-99SoloDawnfrontend/src/components/ui-new/views/WorkspacesMain.tsx

> 该文件共有 **3** 个问题

### 214.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwehPZ9DOUQdEsGhG`
- **项目**: huanchong-99
- **行号**: L345
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 345min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 214.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwehPZ9DOUQdEsGhI`
- **项目**: huanchong-99
- **行号**: L685
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 685min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 214.3 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVwehPZ9DOUQdEsGhH`
- **项目**: huanchong-99
- **行号**: L682
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 682min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 215. huanchong-99SoloDawnfrontend/src/components/ui-new/views/WorkspacesSidebar.tsx

> 该文件共有 **1** 个问题

### 215.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweg1Z9DOUQdEsGhD`
- **项目**: huanchong-99
- **行号**: L285
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 285min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 216. huanchong-99SoloDawnfrontend/src/components/ui/actions-dropdown.tsx

> 该文件共有 **2** 个问题

### 216.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweQbZ9DOUQdEsGbi`
- **项目**: huanchong-99
- **行号**: L385
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 385min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 216.2 A fragment with only one child is redundant. ✅ 已修复

- **问题ID**: `AZyVweQbZ9DOUQdEsGbj`
- **项目**: huanchong-99
- **行号**: L1765
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 1765min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 217. huanchong-99SoloDawnfrontend/src/components/ui/alert.tsx

> 该文件共有 **1** 个问题

### 217.1 Headings must have content and the content must be accessible by a screen reader. ✅ 已修复

- **问题ID**: `AZyVweP-Z9DOUQdEsGba`
- **项目**: huanchong-99
- **行号**: L415
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 415min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 218. huanchong-99SoloDawnfrontend/src/components/ui/auto-expanding-textarea.tsx

> 该文件共有 **4** 个问题

### 218.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwePYZ9DOUQdEsGbM`
- **项目**: huanchong-99
- **行号**: L352
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 352min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 218.2 Prefer `Number.parseInt` over `parseInt`. ✅ 已修复

- **问题ID**: `AZyVwePYZ9DOUQdEsGbN`
- **项目**: huanchong-99
- **行号**: L362
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 362min effort
- **创建时间**: 1 month ago
- **标签**: convention, es2015

### 218.3 Prefer `Number.parseInt` over `parseInt`. ✅ 已修复

- **问题ID**: `AZyVwePYZ9DOUQdEsGbO`
- **项目**: huanchong-99
- **行号**: L372
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 372min effort
- **创建时间**: 1 month ago
- **标签**: convention, es2015

### 218.4 Prefer `Number.parseInt` over `parseInt`. ✅ 已修复

- **问题ID**: `AZyVwePYZ9DOUQdEsGbP`
- **项目**: huanchong-99
- **行号**: L382
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 382min effort
- **创建时间**: 1 month ago
- **标签**: convention, es2015

---

## 219. huanchong-99SoloDawnfrontend/src/components/ui/breadcrumb.tsx

> 该文件共有 **4** 个问题

### 219.1 Use <a href=...>, or <area href=...> instead of the "link" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwePhZ9DOUQdEsGbQ`
- **项目**: huanchong-99
- **行号**: L655
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 655min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 219.2 Use <img alt=...> instead of the "presentation" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwePhZ9DOUQdEsGbR`
- **项目**: huanchong-99
- **行号**: L815
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 815min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 219.3 Comments inside children section of tag should be placed inside braces ✅ 已修复

- **问题ID**: `AZyVwePhZ9DOUQdEsGbS`
- **项目**: huanchong-99
- **行号**: L871
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 871min effort
- **创建时间**: 1 month ago
- **标签**: react

### 219.4 Use <img alt=...> instead of the "presentation" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwePhZ9DOUQdEsGbT`
- **项目**: huanchong-99
- **行号**: L985
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 985min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 220. huanchong-99SoloDawnfrontend/src/components/ui/carousel.tsx

> 该文件共有 **3** 个问题

### 220.1 The object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwePFZ9DOUQdEsGbI`
- **项目**: huanchong-99
- **行号**: L1235
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1235min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

### 220.2 Use <section aria-label=...>, or <section aria-labelledby=...> instead of the "region" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwePFZ9DOUQdEsGbJ`
- **项目**: huanchong-99
- **行号**: L1355
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1355min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 220.3 Use <details>, <fieldset>, <optgroup>, or <address> instead of the "group" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVwePFZ9DOUQdEsGbK`
- **项目**: huanchong-99
- **行号**: L1805
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1805min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 221. huanchong-99SoloDawnfrontend/src/components/ui/checkbox.tsx

> 该文件共有 **1** 个问题

### 221.1 Use <input type="checkbox"> instead of the "checkbox" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweP1Z9DOUQdEsGbZ`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 222. huanchong-99SoloDawnfrontend/src/components/ui/dialog.tsx

> 该文件共有 **4** 个问题

### 222.1 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVweQkZ9DOUQdEsGbk`
- **项目**: huanchong-99
- **行号**: L851
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 851min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 222.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweQkZ9DOUQdEsGbl`
- **项目**: huanchong-99
- **行号**: L1125
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1125min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 222.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweQkZ9DOUQdEsGbm`
- **项目**: huanchong-99
- **行号**: L1125
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1125min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 222.4 Headings must have content and the content must be accessible by a screen reader. ✅ 已修复

- **问题ID**: `AZyVweQkZ9DOUQdEsGbn`
- **项目**: huanchong-99
- **行号**: L1585
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1585min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 223. huanchong-99SoloDawnfrontend/src/components/ui/json-editor.tsx

> 该文件共有 **2** 个问题

### 223.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweQHZ9DOUQdEsGbb`
- **项目**: huanchong-99
- **行号**: L352
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 352min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 223.2 Prefer `globalThis.window` over `window`. ✅ 已修复

- **问题ID**: `AZyVweQHZ9DOUQdEsGbc`
- **项目**: huanchong-99
- **行号**: L432
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 432min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 224. huanchong-99SoloDawnfrontend/src/components/ui/multi-file-search-textarea.tsx

> 该文件共有 **6** 个问题

### 224.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweQvZ9DOUQdEsGbo`
- **项目**: huanchong-99
- **行号**: L285
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 285min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 224.2 Use `.includes()`, rather than `.indexOf()`, when checking for existence. ✅ 已修复

- **问题ID**: `AZyVweQvZ9DOUQdEsGbp`
- **项目**: huanchong-99
- **行号**: L1505
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1505min effort
- **创建时间**: 1 month ago
- **标签**: es6, readability

### 224.3 Use `.includes()`, rather than `.indexOf()`, when checking for existence. ✅ 已修复

- **问题ID**: `AZyVweQvZ9DOUQdEsGbq`
- **项目**: huanchong-99
- **行号**: L1535
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1535min effort
- **创建时间**: 1 month ago
- **标签**: es6, readability

### 224.4 Remove this useless assignment to variable "maxHeight". ✅ 已修复

- **问题ID**: `AZyVweQvZ9DOUQdEsGbr`
- **项目**: huanchong-99
- **行号**: L2761
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2761min effort
- **创建时间**: 1 month ago
- **标签**: cwe, unused

### 224.5 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweQvZ9DOUQdEsGbs`
- **项目**: huanchong-99
- **行号**: L3765
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3765min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 224.6 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweQvZ9DOUQdEsGbt`
- **项目**: huanchong-99
- **行号**: L3765
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 3765min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 225. huanchong-99SoloDawnfrontend/src/components/ui/pr-comment-card.tsx

> 该文件共有 **15** 个问题

### 225.1 'createdAt' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweO6Z9DOUQdEsGa5`
- **项目**: huanchong-99
- **行号**: L85
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 85min effort
- **创建时间**: 1 month ago
- **标签**: react

### 225.2 'url' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGa6`
- **项目**: huanchong-99
- **行号**: L95
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 95min effort
- **创建时间**: 1 month ago
- **标签**: react

### 225.3 'line' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGa7`
- **项目**: huanchong-99
- **行号**: L135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 135min effort
- **创建时间**: 1 month ago
- **标签**: react

### 225.4 'diffHunk' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGa8`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react

### 225.5 'variant' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGa9`
- **项目**: huanchong-99
- **行号**: L165
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 165min effort
- **创建时间**: 1 month ago
- **标签**: react

### 225.6 'onDoubleClick' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGa-`
- **项目**: huanchong-99
- **行号**: L185
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 185min effort
- **创建时间**: 1 month ago
- **标签**: react

### 225.7 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGa_`
- **项目**: huanchong-99
- **行号**: L385
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 385min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 225.8 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGbA`
- **项目**: huanchong-99
- **行号**: L545
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 545min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

### 225.9 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGbB`
- **项目**: huanchong-99
- **行号**: L665
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 665min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 225.10 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGbC`
- **项目**: huanchong-99
- **行号**: L815
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 815min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 225.11 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGbD`
- **项目**: huanchong-99
- **行号**: L815
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 815min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 225.12 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGbE`
- **项目**: huanchong-99
- **行号**: L1045
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1045min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 225.13 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGbF`
- **项目**: huanchong-99
- **行号**: L1225
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1225min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 225.14 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGbG`
- **项目**: huanchong-99
- **行号**: L1225
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1225min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 225.15 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweO7Z9DOUQdEsGbH`
- **项目**: huanchong-99
- **行号**: L1855
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1855min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 226. huanchong-99SoloDawnfrontend/src/components/ui/shadcn-io/kanban.tsx

> 该文件共有 **1** 个问题

### 226.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwePOZ9DOUQdEsGbL`
- **项目**: huanchong-99
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 155min effort
- **创建时间**: 26 days ago
- **标签**: react, type-dependent

---

## 227. huanchong-99SoloDawnfrontend/src/components/ui/table/data-table.tsx

> 该文件共有 **2** 个问题

### 227.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweOwZ9DOUQdEsGa3`
- **项目**: huanchong-99
- **行号**: L305
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 305min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 227.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweOwZ9DOUQdEsGa4`
- **项目**: huanchong-99
- **行号**: L625
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 625min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 228. huanchong-99SoloDawnfrontend/src/components/ui/toast.tsx

> 该文件共有 **4** 个问题

### 228.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwePrZ9DOUQdEsGbU`
- **项目**: huanchong-99
- **行号**: L345
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 345min effort
- **创建时间**: 23 days ago
- **标签**: react, type-dependent

### 228.2 The object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwePrZ9DOUQdEsGbW`
- **项目**: huanchong-99
- **行号**: L475
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 475min effort
- **创建时间**: 23 days ago
- **标签**: jsx, performance, ...

### 228.3 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwePrZ9DOUQdEsGbX`
- **项目**: huanchong-99
- **行号**: L595
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 595min effort
- **创建时间**: 23 days ago
- **标签**: react, type-dependent

### 228.4 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwePrZ9DOUQdEsGbY`
- **项目**: huanchong-99
- **行号**: L765
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 765min effort
- **创建时间**: 23 days ago
- **标签**: react, type-dependent

---

## 229. huanchong-99SoloDawnfrontend/src/components/ui/wysiwyg.tsx

> 该文件共有 **5** 个问题

### 229.1 Remove this redundant type alias and replace its occurrences with "string". ✅ 已修复

- **问题ID**: `AZyVweQRZ9DOUQdEsGbd`
- **项目**: huanchong-99
- **行号**: L485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 485min effort
- **创建时间**: 1 month ago

### 229.2 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweQRZ9DOUQdEsGbe`
- **项目**: huanchong-99
- **行号**: L835
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 835min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 229.3 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweQRZ9DOUQdEsGbf`
- **项目**: huanchong-99
- **行号**: L1105
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1105min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 229.4 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweQRZ9DOUQdEsGbg`
- **项目**: huanchong-99
- **行号**: L1132
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1132min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 229.5 The array passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVweQRZ9DOUQdEsGbh`
- **项目**: huanchong-99
- **行号**: L2235
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2235min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 230. huanchong-99SoloDawnfrontend/src/components/ui/wysiwyg/lib/create-decorator-node.tsx

> 该文件共有 **1** 个问题

### 230.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweOnZ9DOUQdEsGa2`
- **项目**: huanchong-99
- **行号**: L1935
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1935min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 231. huanchong-99SoloDawnfrontend/src/components/wizard/StepIndicator.tsx

> 该文件共有 **1** 个问题

### 231.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweK7Z9DOUQdEsGaa`
- **项目**: huanchong-99
- **行号**: L85
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 85min effort
- **创建时间**: 27 days ago
- **标签**: react, type-dependent

---

## 232. huanchong-99SoloDawnfrontend/src/components/wizard/WorkflowConfigureStep.tsx

> 该文件共有 **3** 个问题

### 232.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweNHZ9DOUQdEsGac`
- **项目**: huanchong-99
- **行号**: L55
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 55min effort
- **创建时间**: 27 days ago
- **标签**: react, type-dependent

### 232.2 A form label must be associated with a control. ✅ 已修复

- **问题ID**: `AZyVweNHZ9DOUQdEsGad`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 115min effort
- **创建时间**: 27 days ago
- **标签**: accessibility, react

### 232.3 A form label must be associated with a control. ✅ 已修复

- **问题ID**: `AZyVweNHZ9DOUQdEsGae`
- **项目**: huanchong-99
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 155min effort
- **创建时间**: 27 days ago
- **标签**: accessibility, react

---

## 233. huanchong-99SoloDawnfrontend/src/components/wizard/WorkflowWizard.tsx

> 该文件共有 **1** 个问题

### 233.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweM_Z9DOUQdEsGab`
- **项目**: huanchong-99
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 155min effort
- **创建时间**: 27 days ago
- **标签**: react, type-dependent

---

## 234. huanchong-99SoloDawnfrontend/src/components/workflow/PipelineView.test.tsx

> 该文件共有 **3** 个问题

### 234.1 Remove this unused import of 'WorkflowDetailDto'. ✅ 已修复

- **问题ID**: `AZyVweWrZ9DOUQdEsGdp`
- **项目**: huanchong-99
- **行号**: L51
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 51min effort
- **创建时间**: 1 month ago
- **标签**: es2015, type-dependent, ...

### 234.2 `String.raw` should be used to avoid escaping `\`. ✅ 已修复

- **问题ID**: `AZyVweWrZ9DOUQdEsGdq`
- **项目**: huanchong-99
- **行号**: L1935
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1935min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 234.3 `String.raw` should be used to avoid escaping `\`. ✅ 已修复

- **问题ID**: `AZyVweWrZ9DOUQdEsGdr`
- **项目**: huanchong-99
- **行号**: L3125
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3125min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 235. huanchong-99SoloDawnfrontend/src/components/workflow/PipelineView.tsx

> 该文件共有 **1** 个问题

### 235.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweW6Z9DOUQdEsGdt`
- **项目**: huanchong-99
- **行号**: L655
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 655min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 236. huanchong-99SoloDawnfrontend/src/components/workflow/StepIndicator.test.tsx

> 该文件共有 **1** 个问题

### 236.1 `String.raw` should be used to avoid escaping `\`. ✅ 已修复

- **问题ID**: `AZyVweWZZ9DOUQdEsGdm`
- **项目**: huanchong-99
- **行号**: L615
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 615min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 237. huanchong-99SoloDawnfrontend/src/components/workflow/StepIndicator.tsx

> 该文件共有 **1** 个问题

### 237.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweWzZ9DOUQdEsGds`
- **项目**: huanchong-99
- **行号**: L145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 145min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 238. huanchong-99SoloDawnfrontend/src/components/workflow/TerminalCard.tsx

> 该文件共有 **6** 个问题

### 238.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweXLZ9DOUQdEsGdv`
- **项目**: huanchong-99
- **行号**: L845
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 845min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 238.2 Use 'Object.hasOwn()' instead of 'Object.prototype.hasOwnProperty.call()'. ✅ 已修复

- **问题ID**: `AZyVweXLZ9DOUQdEsGdw`
- **项目**: huanchong-99
- **行号**: L875
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 875min effort
- **创建时间**: 1 month ago
- **标签**: es2022

### 238.3 Unnecessary use of conditional expression for default assignment. ✅ 已修复

- **问题ID**: `AZyVweXLZ9DOUQdEsGdx`
- **项目**: huanchong-99
- **行号**: L925
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 925min effort
- **创建时间**: 1 month ago

### 238.4 Elements with the 'button' interactive role must be focusable. ✅ 已修复

- **问题ID**: `AZyVweXLZ9DOUQdEsGdy`
- **项目**: huanchong-99
- **行号**: L955
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 955min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 238.5 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyVweXLZ9DOUQdEsGdz`
- **项目**: huanchong-99
- **行号**: L955
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 955min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 238.6 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweXLZ9DOUQdEsGd0`
- **项目**: huanchong-99
- **行号**: L955
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 955min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 239. huanchong-99SoloDawnfrontend/src/components/workflow/WorkflowPromptDialog.tsx

> 该文件共有 **1** 个问题

### 239.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweWRZ9DOUQdEsGdl`
- **项目**: huanchong-99
- **行号**: L995
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 995min effort
- **创建时间**: 17 days ago
- **标签**: react, type-dependent

---

## 240. huanchong-99SoloDawnfrontend/src/components/workflow/WorkflowWizard.tsx

> 该文件共有 **2** 个问题

### 240.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweWiZ9DOUQdEsGdn`
- **项目**: huanchong-99
- **行号**: L285
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 285min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 240.2 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVweWiZ9DOUQdEsGdo`
- **项目**: huanchong-99
- **行号**: L2935
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2935min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 241. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step0Project.tsx

> 该文件共有 **1** 个问题

### 241.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweViZ9DOUQdEsGdW`
- **项目**: huanchong-99
- **行号**: L1825
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1825min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 242. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step1Basic.tsx

> 该文件共有 **2** 个问题

### 242.1 Prefer `Number.parseInt` over `parseInt`. ✅ 已修复

- **问题ID**: `AZyVweVpZ9DOUQdEsGdX`
- **项目**: huanchong-99
- **行号**: L372
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 372min effort
- **创建时间**: 1 month ago
- **标签**: convention, es2015

### 242.2 Prefer `Number.isNaN` over `isNaN`. ✅ 已修复

- **问题ID**: `AZyVweVpZ9DOUQdEsGdY`
- **项目**: huanchong-99
- **行号**: L382
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 382min effort
- **创建时间**: 1 month ago
- **标签**: convention, es2015

---

## 243. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step2Tasks.tsx

> 该文件共有 **7** 个问题

### 243.1 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweV8Z9DOUQdEsGdc`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 195min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 243.2 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweV8Z9DOUQdEsGdd`
- **项目**: huanchong-99
- **行号**: L205
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 205min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 243.3 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVweV8Z9DOUQdEsGde`
- **项目**: huanchong-99
- **行号**: L215
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 215min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 243.4 Prefer `Number.parseInt` over `parseInt`. ✅ 已修复

- **问题ID**: `AZyVweV8Z9DOUQdEsGdg`
- **项目**: huanchong-99
- **行号**: L992
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 992min effort
- **创建时间**: 25 days ago
- **标签**: convention, es2015

### 243.5 Prefer `Number.isNaN` over `isNaN`. ✅ 已修复

- **问题ID**: `AZyVweV8Z9DOUQdEsGdh`
- **项目**: huanchong-99
- **行号**: L1002
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1002min effort
- **创建时间**: 25 days ago
- **标签**: convention, es2015

### 243.6 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVweV8Z9DOUQdEsGdi`
- **项目**: huanchong-99
- **行号**: L1405
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1405min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

### 243.7 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweV8Z9DOUQdEsGdj`
- **项目**: huanchong-99
- **行号**: L1455
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1455min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 244. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step3Models.tsx

> 该文件共有 **3** 个问题

### 244.1 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVweVzZ9DOUQdEsGdZ`
- **项目**: huanchong-99
- **行号**: L1611
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 25 days ago
- **标签**: cwe, error-handling, ...

### 244.2 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVweVzZ9DOUQdEsGda`
- **项目**: huanchong-99
- **行号**: L2051
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 25 days ago
- **标签**: cwe, error-handling, ...

### 244.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweVzZ9DOUQdEsGdb`
- **项目**: huanchong-99
- **行号**: L2642
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2642min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 245. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step4Terminals.test.tsx

> 该文件共有 **1** 个问题

### 245.1 `String.raw` should be used to avoid escaping `\`. ✅ 已修复

- **问题ID**: `AZyVweVZZ9DOUQdEsGdV`
- **项目**: huanchong-99
- **行号**: L4705
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4705min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 246. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step4Terminals.tsx

> 该文件共有 **3** 个问题

### 246.1 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVweVGZ9DOUQdEsGdQ`
- **项目**: huanchong-99
- **行号**: L13520
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 13520min effort
- **创建时间**: 17 days ago
- **标签**: brain-overload

### 246.2 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweVGZ9DOUQdEsGdR`
- **项目**: huanchong-99
- **行号**: L1935
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1935min effort
- **创建时间**: 21 days ago
- **标签**: confusing, type-dependent

### 246.3 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVweVGZ9DOUQdEsGdS`
- **项目**: huanchong-99
- **行号**: L1975
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1975min effort
- **创建时间**: 21 days ago
- **标签**: confusing, type-dependent

---

## 247. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step5Commands.tsx

> 该文件共有 **1** 个问题

### 247.1 Provide multiple methods instead of using "enabled" to determine which action to take. ✅ 已修复

- **问题ID**: `AZyVweWHZ9DOUQdEsGdk`
- **项目**: huanchong-99
- **行号**: L36215
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 36215min effort
- **创建时间**: 1 month ago
- **标签**: design, type-dependent

---

## 248. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step6Advanced.tsx

> 该文件共有 **2** 个问题

### 248.1 Provide multiple methods instead of using "enabled" to determine which action to take. ✅ 已修复

- **问题ID**: `AZyVweVQZ9DOUQdEsGdU`
- **项目**: huanchong-99
- **行号**: L6715
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 6715min effort
- **创建时间**: 1 month ago
- **标签**: design, type-dependent

### 248.2 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweVQZ9DOUQdEsGdT`
- **项目**: huanchong-99
- **行号**: L672
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 672min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 249. huanchong-99SoloDawnfrontend/src/components/workflow/types.test.ts

> 该文件共有 **1** 个问题

### 249.1 Remove this unused import of 'CreateWorkflowRequest'. ✅ 已修复

- **问题ID**: `AZyVweXCZ9DOUQdEsGdu`
- **项目**: huanchong-99
- **行号**: L91
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 91min effort
- **创建时间**: 1 month ago
- **标签**: es2015, type-dependent, ...

---

## 250. huanchong-99SoloDawnfrontend/src/contexts/ActionsContext.tsx

> 该文件共有 **1** 个问题

### 250.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwe0RZ9DOUQdEsGnM`
- **项目**: huanchong-99
- **行号**: L475
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 475min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 251. huanchong-99SoloDawnfrontend/src/contexts/ApprovalFeedbackContext.tsx

> 该文件共有 **2** 个问题

### 251.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwezzZ9DOUQdEsGnF`
- **项目**: huanchong-99
- **行号**: L455
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 455min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 251.2 The object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwezzZ9DOUQdEsGnG`
- **项目**: huanchong-99
- **行号**: L935
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 935min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 252. huanchong-99SoloDawnfrontend/src/contexts/ApprovalFormContext.tsx

> 该文件共有 **2** 个问题

### 252.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwey8Z9DOUQdEsGms`
- **项目**: huanchong-99
- **行号**: L645
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 645min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 252.2 The object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwey8Z9DOUQdEsGmt`
- **项目**: huanchong-99
- **行号**: L955
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 955min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 253. huanchong-99SoloDawnfrontend/src/contexts/ChangesViewContext.tsx

> 该文件共有 **1** 个问题

### 253.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwe0hZ9DOUQdEsGnO`
- **项目**: huanchong-99
- **行号**: L495
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 495min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 254. huanchong-99SoloDawnfrontend/src/contexts/ClickedElementsProvider.tsx

> 该文件共有 **11** 个问题

### 254.1 Prefer negative index over length minus index for `slice`. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGm3`
- **项目**: huanchong-99
- **行号**: L1035
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1035min effort
- **创建时间**: 1 month ago
- **标签**: performance, readability

### 254.2 Refactor this code to not use nested template literals. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGm6`
- **项目**: huanchong-99
- **行号**: L12910
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 12910min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload, confusing

### 254.3 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGm5`
- **项目**: huanchong-99
- **行号**: L1292
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1292min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 254.4 Provide a compare function that depends on "String.localeCompare", to reliably sort elements alphabetically. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGm7`
- **项目**: huanchong-99
- **行号**: L14410
- **类型**: Bug
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 14410min effort
- **创建时间**: 1 month ago
- **标签**: bad-practice, type-dependent

### 254.5 Refactor this function to reduce its Cognitive Complexity from 18 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGm8`
- **项目**: huanchong-99
- **行号**: L1738
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 1738min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 254.6 'value' will use Object's default stringification format ('[object Object]') when stringified. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGm9`
- **项目**: huanchong-99
- **行号**: L1895
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1895min effort
- **创建时间**: 1 month ago
- **标签**: object, string, ...

### 254.7 'value' will use Object's default stringification format ('[object Object]') when stringified. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGm-`
- **项目**: huanchong-99
- **行号**: L1905
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1905min effort
- **创建时间**: 1 month ago
- **标签**: object, string, ...

### 254.8 Refactor this code to not use nested template literals. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGm_`
- **项目**: huanchong-99
- **行号**: L31810
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 31810min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload, confusing

### 254.9 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGnA`
- **项目**: huanchong-99
- **行号**: L3275
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 3275min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 254.10 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGnB`
- **项目**: huanchong-99
- **行号**: L3455
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3455min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 254.11 The object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwezbZ9DOUQdEsGnC`
- **项目**: huanchong-99
- **行号**: L3885
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3885min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 255. huanchong-99SoloDawnfrontend/src/contexts/CreateModeContext.tsx

> 该文件共有 **1** 个问题

### 255.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwe0CZ9DOUQdEsGnJ`
- **项目**: huanchong-99
- **行号**: L325
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 325min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 256. huanchong-99SoloDawnfrontend/src/contexts/EntriesContext.tsx

> 该文件共有 **1** 个问题

### 256.1 useState call is not destructured into value + setter pair ✅ 已修复

- **问题ID**: `AZyVwezDZ9DOUQdEsGmu`
- **项目**: huanchong-99
- **行号**: L245
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 245min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 257. huanchong-99SoloDawnfrontend/src/contexts/ExecutionProcessesContext.tsx

> 该文件共有 **1** 个问题

### 257.1 Consider removing 'undefined' type or '?' specifier, one of them is redundant. ✅ 已修复

- **问题ID**: `AZyVwezjZ9DOUQdEsGnD`
- **项目**: huanchong-99
- **行号**: L241
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 241min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 258. huanchong-99SoloDawnfrontend/src/contexts/GitOperationsContext.tsx

> 该文件共有 **1** 个问题

### 258.1 The object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwezKZ9DOUQdEsGmv`
- **项目**: huanchong-99
- **行号**: L235
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 235min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 259. huanchong-99SoloDawnfrontend/src/contexts/LogsPanelContext.tsx

> 该文件共有 **2** 个问题

### 259.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwez6Z9DOUQdEsGnH`
- **项目**: huanchong-99
- **行号**: L535
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 535min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 259.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwez6Z9DOUQdEsGnI`
- **项目**: huanchong-99
- **行号**: L685
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 685min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 260. huanchong-99SoloDawnfrontend/src/contexts/MessageEditContext.tsx

> 该文件共有 **1** 个问题

### 260.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwe0vZ9DOUQdEsGnS`
- **项目**: huanchong-99
- **行号**: L305
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 305min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 261. huanchong-99SoloDawnfrontend/src/contexts/ProcessSelectionContext.tsx

> 该文件共有 **1** 个问题

### 261.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwe0ZZ9DOUQdEsGnN`
- **项目**: huanchong-99
- **行号**: L165
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 165min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 262. huanchong-99SoloDawnfrontend/src/contexts/ProjectContext.tsx

> 该文件共有 **1** 个问题

### 262.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwezrZ9DOUQdEsGnE`
- **项目**: huanchong-99
- **行号**: L265
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 265min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 263. huanchong-99SoloDawnfrontend/src/contexts/RetryUiContext.tsx

> 该文件共有 **3** 个问题

### 263.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwe0oZ9DOUQdEsGnP`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 263.2 'attemptId' PropType is defined but prop is never used ✅ 已修复

- **问题ID**: `AZyVwe0oZ9DOUQdEsGnQ`
- **项目**: huanchong-99
- **行号**: L225
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 225min effort
- **创建时间**: 1 month ago
- **标签**: react

### 263.3 The 'value' object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwe0oZ9DOUQdEsGnR`
- **项目**: huanchong-99
- **行号**: L515
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 515min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 264. huanchong-99SoloDawnfrontend/src/contexts/ReviewProvider.tsx

> 该文件共有 **3** 个问题

### 264.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwezSZ9DOUQdEsGmw`
- **项目**: huanchong-99
- **行号**: L585
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 585min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 264.2 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVwezSZ9DOUQdEsGmx`
- **项目**: huanchong-99
- **行号**: L1285
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1285min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 264.3 The object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwezSZ9DOUQdEsGmz`
- **项目**: huanchong-99
- **行号**: L1415
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1415min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 265. huanchong-99SoloDawnfrontend/src/contexts/SearchContext.tsx

> 该文件共有 **2** 个问题

### 265.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwe0JZ9DOUQdEsGnK`
- **项目**: huanchong-99
- **行号**: L275
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 275min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 265.2 The 'value' object passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook. ✅ 已修复

- **问题ID**: `AZyVwe0JZ9DOUQdEsGnL`
- **项目**: huanchong-99
- **行号**: L605
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 605min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 266. huanchong-99SoloDawnfrontend/src/contexts/WorkspaceContext.tsx

> 该文件共有 **2** 个问题

### 266.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwey0Z9DOUQdEsGmq`
- **项目**: huanchong-99
- **行号**: L845
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 845min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 266.2 Complete the task associated to this "TODO" comment. ✅ 已修复

- **问题ID**: `AZyVwey0Z9DOUQdEsGmr`
- **项目**: huanchong-99
- **行号**: L1240
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1240min effort
- **创建时间**: 1 month ago
- **标签**: cwe

---

## 267. huanchong-99SoloDawnfrontend/src/hooks/auth/useAuthStatus.ts

> 该文件共有 **1** 个问题

### 267.1 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVwexBZ9DOUQdEsGmN`
- **项目**: huanchong-99
- **行号**: L235
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 235min effort
- **创建时间**: 22 hours ago
- **标签**: confusing, type-dependent

---

## 268. huanchong-99SoloDawnfrontend/src/hooks/useCommandBarShortcut.ts

> 该文件共有 **4** 个问题

### 268.1 'platform' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwevrZ9DOUQdEsGli`
- **项目**: huanchong-99
- **行号**: L1515
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1515min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 268.2 Use `.includes()`, rather than `.indexOf()`, when checking for existence. ✅ 已修复

- **问题ID**: `AZyVwevrZ9DOUQdEsGlj`
- **项目**: huanchong-99
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 155min effort
- **创建时间**: 1 month ago
- **标签**: es6, readability

### 268.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwevrZ9DOUQdEsGlk`
- **项目**: huanchong-99
- **行号**: L312
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 312min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 268.4 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwevrZ9DOUQdEsGll`
- **项目**: huanchong-99
- **行号**: L342
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 342min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 269. huanchong-99SoloDawnfrontend/src/hooks/useContextBarPosition.ts

> 该文件共有 **1** 个问题

### 269.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwewNZ9DOUQdEsGlv`
- **项目**: huanchong-99
- **行号**: L705
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 705min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 270. huanchong-99SoloDawnfrontend/src/hooks/useConversationHistory.ts

> 该文件共有 **8** 个问题

### 270.1 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewpZ9DOUQdEsGl5`
- **项目**: huanchong-99
- **行号**: L1421
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1421min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 270.2 Refactor this function to reduce its Cognitive Complexity from 29 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwewpZ9DOUQdEsGl6`
- **项目**: huanchong-99
- **行号**: L22519
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 22519min effort
- **创建时间**: 26 days ago
- **标签**: brain-overload

### 270.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwewpZ9DOUQdEsGl7`
- **项目**: huanchong-99
- **行号**: L3655
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3655min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 270.4 Using `join()` for p.entries.map((line) => line.content) may use Object's default stringification format ('[object Object]') when stringified. ✅ 已修复

- **问题ID**: `AZyVwewpZ9DOUQdEsGl8`
- **项目**: huanchong-99
- **行号**: L3695
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3695min effort
- **创建时间**: 1 month ago
- **标签**: object, string, ...

### 270.5 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwewpZ9DOUQdEsGl9`
- **项目**: huanchong-99
- **行号**: L45920
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 45920min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 270.6 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwewpZ9DOUQdEsGl-`
- **项目**: huanchong-99
- **行号**: L46220
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 46220min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 270.7 Expected the Promise rejection reason to be an Error. ✅ 已修复

- **问题ID**: `AZyVwewpZ9DOUQdEsGl_`
- **项目**: huanchong-99
- **行号**: L4775
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4775min effort
- **创建时间**: 1 month ago

### 270.8 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwewpZ9DOUQdEsGmA`
- **项目**: huanchong-99
- **行号**: L4921
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

---

## 271. huanchong-99SoloDawnfrontend/src/hooks/useDevserverPreview.ts

> 该文件共有 **1** 个问题

### 271.1 Do not use an object literal as default for parameter `options`. ✅ 已修复

- **问题ID**: `AZyVwewfZ9DOUQdEsGl4`
- **项目**: huanchong-99
- **行号**: L235
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 235min effort
- **创建时间**: 1 month ago
- **标签**: confusing, pitfall

---

## 272. huanchong-99SoloDawnfrontend/src/hooks/useDevserverUrl.ts

> 该文件共有 **4** 个问题

### 272.1 Simplify this regular expression to reduce its complexity from 21 to the 20 allowed. ✅ 已修复

- **问题ID**: `AZyVwewEZ9DOUQdEsGlr`
- **项目**: huanchong-99
- **行号**: L510
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 510min effort
- **创建时间**: 1 month ago
- **标签**: regex, type-dependent

### 272.2 Prefer `globalThis.window` over `window`. ✅ 已修复

- **问题ID**: `AZyVwewEZ9DOUQdEsGls`
- **项目**: huanchong-99
- **行号**: L172
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 172min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 272.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwewEZ9DOUQdEsGlt`
- **项目**: huanchong-99
- **行号**: L182
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 182min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 272.4 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewEZ9DOUQdEsGlu`
- **项目**: huanchong-99
- **行号**: L551
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 551min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 273. huanchong-99SoloDawnfrontend/src/hooks/useJsonPatchWsStream.ts

> 该文件共有 **4** 个问题

### 273.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwexlZ9DOUQdEsGmR`
- **项目**: huanchong-99
- **行号**: L572
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 572min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 273.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwexlZ9DOUQdEsGmS`
- **项目**: huanchong-99
- **行号**: L962
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 962min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 273.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwexlZ9DOUQdEsGmT`
- **项目**: huanchong-99
- **行号**: L1342
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1342min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 273.4 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwexlZ9DOUQdEsGmU`
- **项目**: huanchong-99
- **行号**: L2082
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2082min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 274. huanchong-99SoloDawnfrontend/src/hooks/useLogStream.test.tsx

> 该文件共有 **1** 个问题

### 274.1 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwexKZ9DOUQdEsGmO`
- **项目**: huanchong-99
- **行号**: L620
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 620min effort
- **创建时间**: 22 hours ago
- **标签**: cwe

---

## 275. huanchong-99SoloDawnfrontend/src/hooks/useLogStream.ts

> 该文件共有 **6** 个问题

### 275.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwevTZ9DOUQdEsGlX`
- **项目**: huanchong-99
- **行号**: L372
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 372min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 275.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwevTZ9DOUQdEsGlY`
- **项目**: huanchong-99
- **行号**: L382
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 382min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 275.3 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwevTZ9DOUQdEsGlZ`
- **项目**: huanchong-99
- **行号**: L6220
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 6220min effort
- **创建时间**: 22 hours ago
- **标签**: brain-overload

### 275.4 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwevTZ9DOUQdEsGla`
- **项目**: huanchong-99
- **行号**: L7920
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 7920min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 275.5 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwevTZ9DOUQdEsGlb`
- **项目**: huanchong-99
- **行号**: L815
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 815min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 275.6 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwevTZ9DOUQdEsGlc`
- **项目**: huanchong-99
- **行号**: L12120
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 12120min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

---

## 276. huanchong-99SoloDawnfrontend/src/hooks/useMediaQuery.ts

> 该文件共有 **7** 个问题

### 276.1 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVwew4Z9DOUQdEsGmG`
- **项目**: huanchong-99
- **行号**: L52
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 52min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 276.2 Prefer `globalThis.window` over `window`. ✅ 已修复

- **问题ID**: `AZyVwew4Z9DOUQdEsGmH`
- **项目**: huanchong-99
- **行号**: L52
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 52min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 276.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwew4Z9DOUQdEsGmI`
- **项目**: huanchong-99
- **行号**: L52
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 52min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 276.4 Prefer `globalThis.window` over `window`. ✅ 已修复

- **问题ID**: `AZyVwew4Z9DOUQdEsGmJ`
- **项目**: huanchong-99
- **行号**: L102
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 102min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 276.5 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwew4Z9DOUQdEsGmK`
- **项目**: huanchong-99
- **行号**: L122
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 122min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 276.6 The signature '(callback: ((this: MediaQueryList, ev: MediaQueryListEvent) => any) | null): void' of 'mql.addListener' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwew4Z9DOUQdEsGmL`
- **项目**: huanchong-99
- **行号**: L1815
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1815min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 276.7 The signature '(callback: ((this: MediaQueryList, ev: MediaQueryListEvent) => any) | null): void' of 'mql.removeListener' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwew4Z9DOUQdEsGmM`
- **项目**: huanchong-99
- **行号**: L2715
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2715min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

---

## 277. huanchong-99SoloDawnfrontend/src/hooks/useMessageEditRetry.ts

> 该文件共有 **1** 个问题

### 277.1 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVweyCZ9DOUQdEsGme`
- **项目**: huanchong-99
- **行号**: L485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 485min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

---

## 278. huanchong-99SoloDawnfrontend/src/hooks/useNavigateWithSearch.ts

> 该文件共有 **1** 个问题

### 278.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwexdZ9DOUQdEsGmQ`
- **项目**: huanchong-99
- **行号**: L1015
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1015min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 279. huanchong-99SoloDawnfrontend/src/hooks/usePreviewSettings.ts

> 该文件共有 **1** 个问题

### 279.1 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwexVZ9DOUQdEsGmP`
- **项目**: huanchong-99
- **行号**: L571
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 571min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 280. huanchong-99SoloDawnfrontend/src/hooks/usePreviousPath.test.tsx

> 该文件共有 **1** 个问题

### 280.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwevbZ9DOUQdEsGld`
- **项目**: huanchong-99
- **行号**: L472
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 472min effort
- **创建时间**: 22 hours ago
- **标签**: es2020, portability

---

## 281. huanchong-99SoloDawnfrontend/src/hooks/usePreviousPath.ts

> 该文件共有 **3** 个问题

### 281.1 Prefer `globalThis.window` over `window`. ✅ 已修复

- **问题ID**: `AZyVwevjZ9DOUQdEsGlf`
- **项目**: huanchong-99
- **行号**: L142
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 142min effort
- **创建时间**: 22 hours ago
- **标签**: es2020, portability

### 281.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwevjZ9DOUQdEsGlg`
- **项目**: huanchong-99
- **行号**: L192
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 192min effort
- **创建时间**: 22 hours ago
- **标签**: es2020, portability

### 281.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwevjZ9DOUQdEsGlh`
- **项目**: huanchong-99
- **行号**: L252
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 252min effort
- **创建时间**: 22 hours ago
- **标签**: es2020, portability

---

## 282. huanchong-99SoloDawnfrontend/src/hooks/useProjectTasks.ts

> 该文件共有 **8** 个问题

### 282.1 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewWZ9DOUQdEsGlw`
- **项目**: huanchong-99
- **行号**: L1481
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1481min effort
- **创建时间**: 26 days ago
- **标签**: redundant, type-dependent

### 282.2 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewWZ9DOUQdEsGlx`
- **项目**: huanchong-99
- **行号**: L1491
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1491min effort
- **创建时间**: 26 days ago
- **标签**: redundant, type-dependent

### 282.3 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewWZ9DOUQdEsGly`
- **项目**: huanchong-99
- **行号**: L1521
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1521min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 282.4 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewWZ9DOUQdEsGlz`
- **项目**: huanchong-99
- **行号**: L1551
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1551min effort
- **创建时间**: 26 days ago
- **标签**: redundant, type-dependent

### 282.5 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewWZ9DOUQdEsGl0`
- **项目**: huanchong-99
- **行号**: L1561
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1561min effort
- **创建时间**: 26 days ago
- **标签**: redundant, type-dependent

### 282.6 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewWZ9DOUQdEsGl1`
- **项目**: huanchong-99
- **行号**: L1831
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1831min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 282.7 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewWZ9DOUQdEsGl2`
- **项目**: huanchong-99
- **行号**: L1861
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1861min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 282.8 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwewWZ9DOUQdEsGl3`
- **项目**: huanchong-99
- **行号**: L1871
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1871min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 283. huanchong-99SoloDawnfrontend/src/hooks/useRebase.ts

> 该文件共有 **5** 个问题

### 283.1 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVwewxZ9DOUQdEsGmB`
- **项目**: huanchong-99
- **行号**: L31
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 31min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 283.2 'shared/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVwewxZ9DOUQdEsGmC`
- **项目**: huanchong-99
- **行号**: L41
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 41min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 283.3 Expected the Promise rejection reason to be an Error. ✅ 已修复 ✅ 已修复

- **问题ID**: `AZyVwewxZ9DOUQdEsGmD`
- **项目**: huanchong-99
- **行号**: L255
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 255min effort
- **创建时间**: 22 hours ago

### 283.4 Expected the Promise rejection reason to be an Error. ✅ 已修复

- **问题ID**: `AZyVwewxZ9DOUQdEsGmE`
- **项目**: huanchong-99
- **行号**: L425
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 425min effort
- **创建时间**: 1 month ago

### 283.5 Prefer `throw error` over `return Promise.reject(error)`. ✅ 已修复

- **问题ID**: `AZyVwewxZ9DOUQdEsGmF`
- **项目**: huanchong-99
- **行号**: L425
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 425min effort
- **创建时间**: 1 month ago
- **标签**: async, confusing, ...

---

## 284. huanchong-99SoloDawnfrontend/src/hooks/useRetryProcess.ts

> 该文件共有 **1** 个问题

### 284.1 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwev0Z9DOUQdEsGlm`
- **项目**: huanchong-99
- **行号**: L485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 485min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

---

## 285. huanchong-99SoloDawnfrontend/src/hooks/useTodos.ts

> 该文件共有 **4** 个问题

### 285.1 Complete the task associated to this "TODO" comment. ✅ 已修复

- **问题ID**: `AZyVwev8Z9DOUQdEsGln`
- **项目**: huanchong-99
- **行号**: L120
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 120min effort
- **创建时间**: 1 month ago
- **标签**: cwe

### 285.2 Complete the task associated to this "TODO" comment. ✅ 已修复

- **问题ID**: `AZyVwev8Z9DOUQdEsGlo`
- **项目**: huanchong-99
- **行号**: L140
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 140min effort
- **创建时间**: 1 month ago
- **标签**: cwe

### 285.3 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwev8Z9DOUQdEsGlp`
- **项目**: huanchong-99
- **行号**: L231
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 231min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 285.4 Complete the task associated to this "TODO" comment. ✅ 已修复

- **问题ID**: `AZyVwev8Z9DOUQdEsGlq`
- **项目**: huanchong-99
- **行号**: L550
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 550min effort
- **创建时间**: 1 month ago
- **标签**: cwe

---

## 286. huanchong-99SoloDawnfrontend/src/hooks/useVariant.ts

> 该文件共有 **3** 个问题

### 286.1 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVwex6Z9DOUQdEsGmb`
- **项目**: huanchong-99
- **行号**: L202
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 202min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 286.2 useState call is not destructured into value + setter pair ✅ 已修复

- **问题ID**: `AZyVwex6Z9DOUQdEsGmc`
- **项目**: huanchong-99
- **行号**: L225
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 225min effort
- **创建时间**: 1 month ago
- **标签**: react

### 286.3 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVwex6Z9DOUQdEsGmd`
- **项目**: huanchong-99
- **行号**: L312
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 312min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 287. huanchong-99SoloDawnfrontend/src/hooks/useWorkflows.ts

> 该文件共有 **6** 个问题

### 287.1 Use `export…from` to re-export `Workflow`. ✅ 已修复

- **问题ID**: `AZyVwexwZ9DOUQdEsGmV`
- **项目**: huanchong-99
- **行号**: L7135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 7135min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 287.2 Use `export…from` to re-export `WorkflowListItemDto`. ✅ 已修复

- **问题ID**: `AZyVwexwZ9DOUQdEsGmW`
- **项目**: huanchong-99
- **行号**: L7145
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 7145min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 287.3 Use `export…from` to re-export `WorkflowTaskDto`. ✅ 已修复

- **问题ID**: `AZyVwexwZ9DOUQdEsGmX`
- **项目**: huanchong-99
- **行号**: L7155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 7155min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 287.4 Use `export…from` to re-export `TerminalDto`. ✅ 已修复

- **问题ID**: `AZyVwexwZ9DOUQdEsGmY`
- **项目**: huanchong-99
- **行号**: L7165
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 7165min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 287.5 Use `export…from` to re-export `WorkflowCommandDto`. ✅ 已修复

- **问题ID**: `AZyVwexwZ9DOUQdEsGmZ`
- **项目**: huanchong-99
- **行号**: L7175
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 7175min effort
- **创建时间**: 1 month ago
- **标签**: convention

### 287.6 Use `export…from` to re-export `SlashCommandPresetDto`. ✅ 已修复

- **问题ID**: `AZyVwexwZ9DOUQdEsGma`
- **项目**: huanchong-99
- **行号**: L7185
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 7185min effort
- **创建时间**: 1 month ago
- **标签**: convention

---

## 288. huanchong-99SoloDawnfrontend/src/i18n/languages.ts

> 该文件共有 **1** 个问题

### 288.1 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwevLZ9DOUQdEsGlW`
- **项目**: huanchong-99
- **行号**: L731
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 731min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 289. huanchong-99SoloDawnfrontend/src/keyboard/useSemanticKey.ts

> 该文件共有 **2** 个问题

### 289.1 Prefer using nullish coalescing operator (`??`) instead of a ternary expression, as it is simpler to read. ✅ 已修复

- **问题ID**: `AZyVweyMZ9DOUQdEsGmf`
- **项目**: huanchong-99
- **行号**: L355
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 355min effort
- **创建时间**: 1 month ago
- **标签**: es2020, nullish-coalescing, ...

### 289.2 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVweyMZ9DOUQdEsGmg`
- **项目**: huanchong-99
- **行号**: L352
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 352min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 290. huanchong-99SoloDawnfrontend/src/lib/conflicts.ts

> 该文件共有 **1** 个问题

### 290.1 Refactor this code to not use nested template literals. ✅ 已修复

- **问题ID**: `AZyVweykZ9DOUQdEsGmn`
- **项目**: huanchong-99
- **行号**: L4810
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4810min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload, confusing

---

## 291. huanchong-99SoloDawnfrontend/src/lib/devServerUtils.ts

> 该文件共有 **2** 个问题

### 291.1 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVweyUZ9DOUQdEsGmh`
- **项目**: huanchong-99
- **行号**: L291
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 291min effort
- **创建时间**: 26 days ago
- **标签**: redundant, type-dependent

### 291.2 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVweyUZ9DOUQdEsGmi`
- **项目**: huanchong-99
- **行号**: L291
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 291min effort
- **创建时间**: 26 days ago
- **标签**: redundant, type-dependent

---

## 292. huanchong-99SoloDawnfrontend/src/lib/mcpStrategies.ts

> 该文件共有 **4** 个问题

### 292.1 Prefer `structuredClone(…)` over `JSON.parse(JSON.stringify(…))` to create a deep clone. ✅ 已修复

- **问题ID**: `AZyVweycZ9DOUQdEsGmj`
- **项目**: huanchong-99
- **行号**: L115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 115min effort
- **创建时间**: 1 month ago
- **标签**: es2021, modernize, ...

### 292.2 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVweycZ9DOUQdEsGmk`
- **项目**: huanchong-99
- **行号**: L181
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 181min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 292.3 Prefer `structuredClone(…)` over `JSON.parse(JSON.stringify(…))` to create a deep clone. ✅ 已修复

- **问题ID**: `AZyVweycZ9DOUQdEsGml`
- **项目**: huanchong-99
- **行号**: L865
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 865min effort
- **创建时间**: 1 month ago
- **标签**: es2021, modernize, ...

### 292.4 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVweycZ9DOUQdEsGmm`
- **项目**: huanchong-99
- **行号**: L951
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 951min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 293. huanchong-99SoloDawnfrontend/src/lib/utils.ts

> 该文件共有 **2** 个问题

### 293.1 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVweyrZ9DOUQdEsGmo`
- **项目**: huanchong-99
- **行号**: L25
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 25min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 293.2 Complete the task associated to this "TODO" comment. ✅ 已修复

- **问题ID**: `AZyVweyrZ9DOUQdEsGmp`
- **项目**: huanchong-99
- **行号**: L50
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 50min effort
- **创建时间**: 1 month ago
- **标签**: cwe

---

## 294. huanchong-99SoloDawnfrontend/src/main.tsx

> 该文件共有 **1** 个问题

### 294.1 Don't use a zero fraction in the number. ✅ 已修复

- **问题ID**: `AZyVwe3bZ9DOUQdEsGor`
- **项目**: huanchong-99
- **行号**: L231
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 231min effort
- **创建时间**: 1 month ago
- **标签**: consistency, formatting

---

## 295. huanchong-99SoloDawnfrontend/src/pages/SlashCommands.e2e.test.tsx

> 该文件共有 **2** 个问题

### 295.1 Remove this unused import of 'within'. ✅ 已修复

- **问题ID**: `AZyVweuWZ9DOUQdEsGlD`
- **项目**: huanchong-99
- **行号**: L131
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 131min effort
- **创建时间**: 1 month ago
- **标签**: es2015, type-dependent, ...

### 295.2 Remove this useless assignment to variable "user". ✅ 已修复

- **问题ID**: `AZyVweuWZ9DOUQdEsGlE`
- **项目**: huanchong-99
- **行号**: L4281
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4281min effort
- **创建时间**: 1 month ago
- **标签**: cwe, unused

---

## 296. huanchong-99SoloDawnfrontend/src/pages/SlashCommands.tsx

> 该文件共有 **2** 个问题

### 296.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweuNZ9DOUQdEsGlB`
- **项目**: huanchong-99
- **行号**: L1885
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1885min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 296.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweuNZ9DOUQdEsGlC`
- **项目**: huanchong-99
- **行号**: L3095
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3095min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 297. huanchong-99SoloDawnfrontend/src/pages/WorkflowDebug.test.tsx

> 该文件共有 **1** 个问题

### 297.1 Remove this unused import of 'userEvent'. ✅ 已修复

- **问题ID**: `AZyVweunZ9DOUQdEsGlH`
- **项目**: huanchong-99
- **行号**: L121
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 121min effort
- **创建时间**: 1 month ago
- **标签**: es2015, type-dependent, ...

---

## 298. huanchong-99SoloDawnfrontend/src/pages/WorkflowDebug.tsx

> 该文件共有 **5** 个问题

### 298.1 'react-router-dom' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweu8Z9DOUQdEsGlR`
- **项目**: huanchong-99
- **行号**: L11
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 11min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 298.2 'react-router-dom' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVweu8Z9DOUQdEsGlS`
- **项目**: huanchong-99
- **行号**: L211
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 211min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 298.3 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweu8Z9DOUQdEsGlT`
- **项目**: huanchong-99
- **行号**: L595
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 595min effort
- **创建时间**: 21 days ago
- **标签**: react, type-dependent

### 298.4 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweu8Z9DOUQdEsGlU`
- **项目**: huanchong-99
- **行号**: L1272
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1272min effort
- **创建时间**: 17 days ago
- **标签**: es2020, portability

### 298.5 Move function 'mapTerminalStatus' to the outer scope. ✅ 已修复

- **问题ID**: `AZyVweu8Z9DOUQdEsGlV`
- **项目**: huanchong-99
- **行号**: L1515
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1515min effort
- **创建时间**: 1 month ago
- **标签**: javascript, optimization, ...

---

## 299. huanchong-99SoloDawnfrontend/src/pages/WorkflowDebugPage.tsx

> 该文件共有 **2** 个问题

### 299.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweufZ9DOUQdEsGlF`
- **项目**: huanchong-99
- **行号**: L462
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 462min effort
- **创建时间**: 25 days ago
- **标签**: es2020, portability

### 299.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweufZ9DOUQdEsGlG`
- **项目**: huanchong-99
- **行号**: L472
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 472min effort
- **创建时间**: 25 days ago
- **标签**: es2020, portability

---

## 300. huanchong-99SoloDawnfrontend/src/pages/Workflows.test.tsx

> 该文件共有 **9** 个问题

### 300.1 `String.raw` should be used to avoid escaping `\`. ✅ 已修复

- **问题ID**: `AZyVweuzZ9DOUQdEsGlI`
- **项目**: huanchong-99
- **行号**: L265
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 265min effort
- **创建时间**: 16 days ago
- **标签**: readability

### 300.2 This assertion is unnecessary since the receiver accepts the original type of the expression. ✅ 已修复

- **问题ID**: `AZyVweuzZ9DOUQdEsGlJ`
- **项目**: huanchong-99
- **行号**: L5671
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5671min effort
- **创建时间**: 17 days ago
- **标签**: redundant, type-dependent

### 300.3 This assertion is unnecessary since the receiver accepts the original type of the expression. ✅ 已修复

- **问题ID**: `AZyVweuzZ9DOUQdEsGlK`
- **项目**: huanchong-99
- **行号**: L7271
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 7271min effort
- **创建时间**: 17 days ago
- **标签**: redundant, type-dependent

### 300.4 This assertion is unnecessary since the receiver accepts the original type of the expression. ✅ 已修复

- **问题ID**: `AZyVweuzZ9DOUQdEsGlL`
- **项目**: huanchong-99
- **行号**: L8011
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 8011min effort
- **创建时间**: 17 days ago
- **标签**: redundant, type-dependent

### 300.5 This assertion is unnecessary since the receiver accepts the original type of the expression. ✅ 已修复

- **问题ID**: `AZyVweuzZ9DOUQdEsGlM`
- **项目**: huanchong-99
- **行号**: L9461
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 9461min effort
- **创建时间**: 17 days ago
- **标签**: redundant, type-dependent

### 300.6 This assertion is unnecessary since the receiver accepts the original type of the expression. ✅ 已修复

- **问题ID**: `AZyVweuzZ9DOUQdEsGlN`
- **项目**: huanchong-99
- **行号**: L10221
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 10221min effort
- **创建时间**: 17 days ago
- **标签**: redundant, type-dependent

### 300.7 This assertion is unnecessary since the receiver accepts the original type of the expression. ✅ 已修复

- **问题ID**: `AZyVweuzZ9DOUQdEsGlO`
- **项目**: huanchong-99
- **行号**: L11081
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 11081min effort
- **创建时间**: 15 days ago
- **标签**: redundant, type-dependent

### 300.8 This assertion is unnecessary since the receiver accepts the original type of the expression. ✅ 已修复

- **问题ID**: `AZyVweuzZ9DOUQdEsGlP`
- **项目**: huanchong-99
- **行号**: L11821
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 11821min effort
- **创建时间**: 14 days ago
- **标签**: redundant, type-dependent

### 300.9 'init.body' may use Object's default stringification format ('[object Object]') when stringified. ✅ 已修复

- **问题ID**: `AZyVweuzZ9DOUQdEsGlQ`
- **项目**: huanchong-99
- **行号**: L12245
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 12245min effort
- **创建时间**: 16 days ago
- **标签**: object, string, ...

---

## 301. huanchong-99SoloDawnfrontend/src/pages/Workflows.tsx

> 该文件共有 **8** 个问题

### 301.1 '@/components/workflow/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVwetGZ9DOUQdEsGkb`
- **项目**: huanchong-99
- **行号**: L451
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 451min effort
- **创建时间**: 26 days ago
- **标签**: es2015

### 301.2 '@/components/workflow/types' imported multiple times. ✅ 已修复

- **问题ID**: `AZyVwetGZ9DOUQdEsGkc`
- **项目**: huanchong-99
- **行号**: L461
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 461min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 301.3 Refactor this function to reduce its Cognitive Complexity from 25 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwetGZ9DOUQdEsGkd`
- **项目**: huanchong-99
- **行号**: L11615
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 11615min effort
- **创建时间**: 16 days ago
- **标签**: brain-overload

### 301.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwetGZ9DOUQdEsGke`
- **项目**: huanchong-99
- **行号**: L1435
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1435min effort
- **创建时间**: 17 days ago
- **标签**: confusing

### 301.5 This assertion is unnecessary since the receiver accepts the original type of the expression. ✅ 已修复

- **问题ID**: `AZyVwetGZ9DOUQdEsGkf`
- **项目**: huanchong-99
- **行号**: L1511
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1511min effort
- **创建时间**: 23 days ago
- **标签**: redundant, type-dependent

### 301.6 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwetGZ9DOUQdEsGkg`
- **项目**: huanchong-99
- **行号**: L2485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2485min effort
- **创建时间**: 17 days ago
- **标签**: type-dependent

### 301.7 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyVwetGZ9DOUQdEsGkh`
- **项目**: huanchong-99
- **行号**: L4355
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4355min effort
- **创建时间**: 17 days ago
- **标签**: confusing, type-dependent

### 301.8 Prefer `.find(…)` over `.filter(…)`. ✅ 已修复

- **问题ID**: `AZyVwetGZ9DOUQdEsGki`
- **项目**: huanchong-99
- **行号**: L5625
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5625min effort
- **创建时间**: 16 days ago
- **标签**: performance, readability

---

## 302. huanchong-99SoloDawnfrontend/src/pages/settings/AgentSettings.tsx

> 该文件共有 **8** 个问题

### 302.1 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwetlZ9DOUQdEsGko`
- **项目**: huanchong-99
- **行号**: L1751
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

### 302.2 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwetlZ9DOUQdEsGkp`
- **项目**: huanchong-99
- **行号**: L1865
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1865min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 302.3 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwetlZ9DOUQdEsGkq`
- **项目**: huanchong-99
- **行号**: L2231
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

### 302.4 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwetlZ9DOUQdEsGkr`
- **项目**: huanchong-99
- **行号**: L3151
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

### 302.5 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwetlZ9DOUQdEsGks`
- **项目**: huanchong-99
- **行号**: L3555
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3555min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 302.6 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwetlZ9DOUQdEsGkt`
- **项目**: huanchong-99
- **行号**: L3775
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3775min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 302.7 'profilesError' will use Object's default stringification format ('[object Object]') when stringified. ✅ 已修复

- **问题ID**: `AZyVwetlZ9DOUQdEsGku`
- **项目**: huanchong-99
- **行号**: L4355
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4355min effort
- **创建时间**: 1 month ago
- **标签**: object, string, ...

### 302.8 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwetlZ9DOUQdEsGkv`
- **项目**: huanchong-99
- **行号**: L6215
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6215min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

---

## 303. huanchong-99SoloDawnfrontend/src/pages/settings/GeneralSettings.tsx

> 该文件共有 **2** 个问题

### 303.1 Prefer `String#codePointAt()` over `String#charCodeAt()`. ✅ 已修复

- **问题ID**: `AZyVwetbZ9DOUQdEsGkm`
- **项目**: huanchong-99
- **行号**: L855
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 855min effort
- **创建时间**: 1 month ago
- **标签**: internationalization, unicode

### 303.2 'returnValue' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwetbZ9DOUQdEsGkn`
- **项目**: huanchong-99
- **行号**: L12915
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 12915min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

---

## 304. huanchong-99SoloDawnfrontend/src/pages/settings/McpSettings.tsx

> 该文件共有 **1** 个问题

### 304.1 This assertion is unnecessary since the receiver accepts the original type of the expression. ✅ 已修复

- **问题ID**: `AZyVwetvZ9DOUQdEsGkw`
- **项目**: huanchong-99
- **行号**: L2171
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2171min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 305. huanchong-99SoloDawnfrontend/src/pages/settings/OrganizationSettings.tsx

> 该文件共有 **11** 个问题

### 305.1 Refactor this function to reduce its Cognitive Complexity from 29 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGkx`
- **项目**: huanchong-99
- **行号**: L4819
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 4819min effort
- **创建时间**: 17 days ago
- **标签**: brain-overload

### 305.2 Promise-returning function provided to property where a void return was expected. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGky`
- **项目**: huanchong-99
- **行号**: L1285
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1285min effort
- **创建时间**: 1 month ago
- **标签**: async, promise, ...

### 305.3 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGkz`
- **项目**: huanchong-99
- **行号**: L2301
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

### 305.4 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGk0`
- **项目**: huanchong-99
- **行号**: L2481
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

### 305.5 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGk1`
- **项目**: huanchong-99
- **行号**: L2632
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2632min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 305.6 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGk2`
- **项目**: huanchong-99
- **行号**: L2802
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2802min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 305.7 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGk3`
- **项目**: huanchong-99
- **行号**: L4375
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4375min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 305.8 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGk4`
- **项目**: huanchong-99
- **行号**: L4835
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4835min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 305.9 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGk5`
- **项目**: huanchong-99
- **行号**: L5215
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5215min effort
- **创建时间**: 17 days ago
- **标签**: confusing

### 305.10 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGk6`
- **项目**: huanchong-99
- **行号**: L5275
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5275min effort
- **创建时间**: 17 days ago
- **标签**: confusing

### 305.11 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwet5Z9DOUQdEsGk7`
- **项目**: huanchong-99
- **行号**: L5355
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5355min effort
- **创建时间**: 17 days ago
- **标签**: confusing

---

## 306. huanchong-99SoloDawnfrontend/src/pages/settings/ProjectSettings.tsx

> 该文件共有 **5** 个问题

### 306.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweuDZ9DOUQdEsGk8`
- **项目**: huanchong-99
- **行号**: L902
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 902min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 306.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVweuDZ9DOUQdEsGk9`
- **项目**: huanchong-99
- **行号**: L1272
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1272min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 306.3 'returnValue' is deprecated. ✅ 已修复

- **问题ID**: `AZyVweuDZ9DOUQdEsGk-`
- **项目**: huanchong-99
- **行号**: L18315
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 18315min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 306.4 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweuDZ9DOUQdEsGk_`
- **项目**: huanchong-99
- **行号**: L5065
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 5065min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 306.5 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweuDZ9DOUQdEsGlA`
- **项目**: huanchong-99
- **行号**: L5065
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 5065min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 307. huanchong-99SoloDawnfrontend/src/pages/settings/ReposSettings.tsx

> 该文件共有 **3** 个问题

### 307.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwetQZ9DOUQdEsGkj`
- **项目**: huanchong-99
- **行号**: L932
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 932min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 307.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwetQZ9DOUQdEsGkk`
- **项目**: huanchong-99
- **行号**: L1182
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1182min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 307.3 'returnValue' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwetQZ9DOUQdEsGkl`
- **项目**: huanchong-99
- **行号**: L16315
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 16315min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

---

## 308. huanchong-99SoloDawnfrontend/src/stores/__tests__/wsStore.test.ts

> 该文件共有 **9** 个问题

### 308.1 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwe2uZ9DOUQdEsGod`
- **项目**: huanchong-99
- **行号**: L520
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 520min effort
- **创建时间**: 25 days ago
- **标签**: cwe

### 308.2 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwe2uZ9DOUQdEsGoe`
- **项目**: huanchong-99
- **行号**: L620
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 620min effort
- **创建时间**: 25 days ago
- **标签**: cwe

### 308.3 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwe2uZ9DOUQdEsGof`
- **项目**: huanchong-99
- **行号**: L720
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 720min effort
- **创建时间**: 25 days ago
- **标签**: cwe

### 308.4 Make this public static property readonly. ✅ 已修复

- **问题ID**: `AZyVwe2uZ9DOUQdEsGog`
- **项目**: huanchong-99
- **行号**: L820
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 820min effort
- **创建时间**: 25 days ago
- **标签**: cwe

### 308.5 Prefer `globalThis` over `global`. ✅ 已修复

- **问题ID**: `AZyVwe2uZ9DOUQdEsGoh`
- **项目**: huanchong-99
- **行号**: L512
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 512min effort
- **创建时间**: 25 days ago
- **标签**: es2020, portability

### 308.6 Prefer `globalThis` over `global`. ✅ 已修复

- **问题ID**: `AZyVwe2uZ9DOUQdEsGoi`
- **项目**: huanchong-99
- **行号**: L592
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 592min effort
- **创建时间**: 25 days ago
- **标签**: es2020, portability

### 308.7 Type literal has only a call signature, you should use a function type instead. ✅ 已修复

- **问题ID**: `AZyVwe2uZ9DOUQdEsGoj`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 635min effort
- **创建时间**: 17 days ago
- **标签**: function, type

### 308.8 Prefer `globalThis` over `global`. ✅ 已修复

- **问题ID**: `AZyVwe2uZ9DOUQdEsGok`
- **项目**: huanchong-99
- **行号**: L662
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 662min effort
- **创建时间**: 17 days ago
- **标签**: es2020, portability

### 308.9 Prefer `globalThis` over `global`. ✅ 已修复

- **问题ID**: `AZyVwe2uZ9DOUQdEsGol`
- **项目**: huanchong-99
- **行号**: L992
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 992min effort
- **创建时间**: 25 days ago
- **标签**: es2020, portability

---

## 309. huanchong-99SoloDawnfrontend/src/stores/useUiPreferencesStore.ts

> 该文件共有 **1** 个问题

### 309.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwe2gZ9DOUQdEsGoc`
- **项目**: huanchong-99
- **行号**: L2235
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2235min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 310. huanchong-99SoloDawnfrontend/src/stores/workflowStore.ts

> 该文件共有 **1** 个问题

### 310.1 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwe2XZ9DOUQdEsGob`
- **项目**: huanchong-99
- **行号**: L11020
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 11020min effort
- **创建时间**: 25 days ago
- **标签**: brain-overload

---

## 311. huanchong-99SoloDawnfrontend/src/stores/wsStore.ts

> 该文件共有 **16** 个问题

### 311.1 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoL`
- **项目**: huanchong-99
- **行号**: L1645
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1645min effort
- **创建时间**: 17 days ago
- **标签**: es2021, readability

### 311.2 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoM`
- **项目**: huanchong-99
- **行号**: L1655
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1655min effort
- **创建时间**: 17 days ago
- **标签**: es2021, readability

### 311.3 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoN`
- **项目**: huanchong-99
- **行号**: L1665
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1665min effort
- **创建时间**: 17 days ago
- **标签**: es2021, readability

### 311.4 Refactor this function to reduce its Cognitive Complexity from 18 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoO`
- **项目**: huanchong-99
- **行号**: L4498
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 4498min effort
- **创建时间**: 17 days ago
- **标签**: brain-overload

### 311.5 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoP`
- **项目**: huanchong-99
- **行号**: L4695
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4695min effort
- **创建时间**: 17 days ago
- **标签**: confusing

### 311.6 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoQ`
- **项目**: huanchong-99
- **行号**: L5642
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 5642min effort
- **创建时间**: 17 days ago
- **标签**: es2020, portability

### 311.7 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoR`
- **项目**: huanchong-99
- **行号**: L5652
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 5652min effort
- **创建时间**: 17 days ago
- **标签**: es2020, portability

### 311.8 `statuses` should be a `Set`, and use `statuses.has()` to check existence or non-existence. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoS`
- **项目**: huanchong-99
- **行号**: L5725
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5725min effort
- **创建时间**: 17 days ago
- **标签**: optimization, performance

### 311.9 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoT`
- **项目**: huanchong-99
- **行号**: L83520
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 83520min effort
- **创建时间**: 17 days ago
- **标签**: brain-overload

### 311.10 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoU`
- **项目**: huanchong-99
- **行号**: L8795
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 8795min effort
- **创建时间**: 17 days ago
- **标签**: type-dependent

### 311.11 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoV`
- **项目**: huanchong-99
- **行号**: L93220
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 93220min effort
- **创建时间**: 17 days ago
- **标签**: brain-overload

### 311.12 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoW`
- **项目**: huanchong-99
- **行号**: L94520
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 94520min effort
- **创建时间**: 17 days ago
- **标签**: brain-overload

### 311.13 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoX`
- **项目**: huanchong-99
- **行号**: L9855
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 9855min effort
- **创建时间**: 17 days ago
- **标签**: type-dependent

### 311.14 Refactor this code to not nest functions more than 4 levels deep. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoY`
- **项目**: huanchong-99
- **行号**: L101820
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 101820min effort
- **创建时间**: 17 days ago
- **标签**: brain-overload

### 311.15 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoZ`
- **项目**: huanchong-99
- **行号**: L10315
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 10315min effort
- **创建时间**: 17 days ago
- **标签**: type-dependent

### 311.16 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoa`
- **项目**: huanchong-99
- **行号**: L10705
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 10705min effort
- **创建时间**: 17 days ago
- **标签**: type-dependent

---

## 312. huanchong-99SoloDawnfrontend/src/styles/diff-style-overrides.css

> 该文件共有 **2** 个问题

### 312.1 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe3KZ9DOUQdEsGoo`
- **项目**: huanchong-99
- **行号**: L4315
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4315min effort
- **创建时间**: 1 month ago
- **标签**: unused

### 312.2 Remove this commented out code. ✅ 已修复

- **问题ID**: `AZyVwe3KZ9DOUQdEsGop`
- **项目**: huanchong-99
- **行号**: L4825
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4825min effort
- **创建时间**: 1 month ago
- **标签**: unused

---

## 313. huanchong-99SoloDawnfrontend/src/test/setup.ts

> 该文件共有 **1** 个问题

### 313.1 export statement without specifiers is not allowed. ✅ 已修复

- **问题ID**: `AZyVwe1vZ9DOUQdEsGnh`
- **项目**: huanchong-99
- **行号**: L322
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 322min effort
- **创建时间**: 1 month ago
- **标签**: confusing, es6, ...

---

## 314. huanchong-99SoloDawnfrontend/src/types/__tests__/websocket.test.ts

> 该文件共有 **1** 个问题

### 314.1 Remove this unused import of 'WsMessage'. ✅ 已修复

- **问题ID**: `AZyVwe22Z9DOUQdEsGom`
- **项目**: huanchong-99
- **行号**: L11
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 11min effort
- **创建时间**: 1 month ago
- **标签**: es2015, type-dependent, ...

---

## 315. huanchong-99SoloDawnfrontend/src/types/modals.ts

> 该文件共有 **1** 个问题

### 315.1 export statement without specifiers is not allowed. ✅ 已修复

- **问题ID**: `AZyVwe2-Z9DOUQdEsGon`
- **项目**: huanchong-99
- **行号**: L422
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 422min effort
- **创建时间**: 1 month ago
- **标签**: confusing, es6, ...

---

## 316. huanchong-99SoloDawnfrontend/src/utils/StyleOverride.tsx

> 该文件共有 **2** 个问题

### 316.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwe04Z9DOUQdEsGnT`
- **项目**: huanchong-99
- **行号**: L815
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 815min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 316.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe04Z9DOUQdEsGnU`
- **项目**: huanchong-99
- **行号**: L1652
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1652min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 317. huanchong-99SoloDawnfrontend/src/utils/TruncatePath.tsx

> 该文件共有 **3** 个问题

### 317.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVwe1HZ9DOUQdEsGnY`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

### 317.2 Move this array "reverse" operation to a separate statement or replace it with "toReversed". ✅ 已修复

- **问题ID**: `AZyVwe1HZ9DOUQdEsGnZ`
- **项目**: huanchong-99
- **行号**: L215
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 215min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 317.3 Do not use Array index in keys ✅ 已修复

- **问题ID**: `AZyVwe1HZ9DOUQdEsGna`
- **项目**: huanchong-99
- **行号**: L225
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 225min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 318. huanchong-99SoloDawnfrontend/src/utils/fileTreeUtils.ts

> 该文件共有 **1** 个问题

### 318.1 Refactor this function to reduce its Cognitive Complexity from 55 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwe1fZ9DOUQdEsGnf`
- **项目**: huanchong-99
- **行号**: L745
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 745min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

---

## 319. huanchong-99SoloDawnfrontend/src/utils/previewBridge.ts

> 该文件共有 **3** 个问题

### 319.1 Prefer using an optional chain expression instead, as it's more concise and easier to read. ✅ 已修复

- **问题ID**: `AZyVwe1WZ9DOUQdEsGnc`
- **项目**: huanchong-99
- **行号**: L795
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 795min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 319.2 Specify a target origin for this message. ✅ 已修复

- **问题ID**: `AZyVwe1WZ9DOUQdEsGnd`
- **项目**: huanchong-99
- **行号**: L9110
- **类型**: Vulnerability
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Security
- **工作量**: 9110min effort
- **创建时间**: 1 month ago
- **标签**: cwe, html5, ...

### 319.3 Specify a target origin for this message. ✅ 已修复

- **问题ID**: `AZyVwe1WZ9DOUQdEsGne`
- **项目**: huanchong-99
- **行号**: L13210
- **类型**: Vulnerability
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Security
- **工作量**: 13210min effort
- **创建时间**: 1 month ago
- **标签**: cwe, html5, ...

---

## 320. huanchong-99SoloDawnfrontend/src/utils/string.ts

> 该文件共有 **3** 个问题

### 320.1 Prefer `.findLast(…)` over `.filter(…).pop()`. ✅ 已修复

- **问题ID**: `AZyVwe1AZ9DOUQdEsGnV`
- **项目**: huanchong-99
- **行号**: L205
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 205min effort
- **创建时间**: 1 month ago
- **标签**: performance, readability

### 320.2 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVwe1AZ9DOUQdEsGnW`
- **项目**: huanchong-99
- **行号**: L215
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 215min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

### 320.3 Prefer `String#replaceAll()` over `String#replace()`. ✅ 已修复

- **问题ID**: `AZyVwe1AZ9DOUQdEsGnX`
- **项目**: huanchong-99
- **行号**: L215
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 215min effort
- **创建时间**: 1 month ago
- **标签**: es2021, readability

---

## 321. huanchong-99SoloDawnfrontend/src/utils/theme.ts

> 该文件共有 **1** 个问题

### 321.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe1nZ9DOUQdEsGng`
- **项目**: huanchong-99
- **行号**: L162
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 162min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 322. huanchong-99SoloDawnfrontend/src/vscode/ContextMenu.tsx

> 该文件共有 **13** 个问题

### 322.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGn9`
- **项目**: huanchong-99
- **行号**: L112
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 112min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 322.2 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGn-`
- **项目**: huanchong-99
- **行号**: L382
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 382min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 322.3 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGn_`
- **项目**: huanchong-99
- **行号**: L962
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 962min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 322.4 Review this redundant assignment: "cut" already holds the assigned value along all execution paths. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoA`
- **项目**: huanchong-99
- **行号**: L1005
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1005min effort
- **创建时间**: 1 month ago
- **标签**: redundant

### 322.5 Review this redundant assignment: "paste" already holds the assigned value along all execution paths. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoB`
- **项目**: huanchong-99
- **行号**: L1015
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1015min effort
- **创建时间**: 1 month ago
- **标签**: redundant

### 322.6 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoC`
- **项目**: huanchong-99
- **行号**: L16215
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 16215min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 322.7 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoD`
- **项目**: huanchong-99
- **行号**: L18515
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 18515min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 322.8 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoE`
- **项目**: huanchong-99
- **行号**: L2021
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2021min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 322.9 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoF`
- **项目**: huanchong-99
- **行号**: L20615
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 20615min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 322.10 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoG`
- **项目**: huanchong-99
- **行号**: L21315
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 21315min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 322.11 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoH`
- **项目**: huanchong-99
- **行号**: L22115
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 22115min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 322.12 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoI`
- **项目**: huanchong-99
- **行号**: L22915
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 22915min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 322.13 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoJ`
- **项目**: huanchong-99
- **行号**: L2395
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2395min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 323. huanchong-99SoloDawnfrontend/src/vscode/bridge.ts

> 该文件共有 **26** 个问题

### 323.1 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGni`
- **项目**: huanchong-99
- **行号**: L132
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 132min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 323.2 'platform' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnj`
- **项目**: huanchong-99
- **行号**: L4815
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4815min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 323.3 Replace this union type with a type alias. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnk`
- **项目**: huanchong-99
- **行号**: L865
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 865min effort
- **创建时间**: 1 month ago
- **标签**: proficiency

### 323.4 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnl`
- **项目**: huanchong-99
- **行号**: L10715
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 10715min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 323.5 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnm`
- **项目**: huanchong-99
- **行号**: L1362
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1362min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 323.6 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnn`
- **项目**: huanchong-99
- **行号**: L2101
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2101min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 323.7 Replace this union type with a type alias. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGno`
- **项目**: huanchong-99
- **行号**: L2125
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 2125min effort
- **创建时间**: 1 month ago
- **标签**: proficiency

### 323.8 Unexpected negated condition. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnp`
- **项目**: huanchong-99
- **行号**: L2142
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2142min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 323.9 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnq`
- **项目**: huanchong-99
- **行号**: L21815
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 21815min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 323.10 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnr`
- **项目**: huanchong-99
- **行号**: L2441
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2441min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 323.11 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGns`
- **项目**: huanchong-99
- **行号**: L2542
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2542min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 323.12 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnt`
- **项目**: huanchong-99
- **行号**: L2602
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2602min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 323.13 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnu`
- **项目**: huanchong-99
- **行号**: L2642
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2642min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 323.14 Specify a target origin for this message. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnv`
- **项目**: huanchong-99
- **行号**: L27310
- **类型**: Vulnerability
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Security
- **工作量**: 27310min effort
- **创建时间**: 1 month ago
- **标签**: cwe, html5, ...

### 323.15 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnw`
- **项目**: huanchong-99
- **行号**: L2771
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

### 323.16 Specify a target origin for this message. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGny`
- **项目**: huanchong-99
- **行号**: L28810
- **类型**: Vulnerability
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Security
- **工作量**: 28810min effort
- **创建时间**: 1 month ago
- **标签**: cwe, html5, ...

### 323.17 Verify the origin of the received message. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGnz`
- **项目**: huanchong-99
- **行号**: L30710
- **类型**: Vulnerability
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Security
- **工作量**: 30710min effort
- **创建时间**: 1 month ago
- **标签**: cwe, html5, ...

### 323.18 This assertion is unnecessary since it does not change the type of the expression. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGn0`
- **项目**: huanchong-99
- **行号**: L3211
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3211min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 323.19 Specify a target origin for this message. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGn1`
- **项目**: huanchong-99
- **行号**: L33610
- **类型**: Vulnerability
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Security
- **工作量**: 33610min effort
- **创建时间**: 1 month ago
- **标签**: cwe, html5, ...

### 323.20 Handle this exception or don't catch it at all. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGn2`
- **项目**: huanchong-99
- **行号**: L3371
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: cwe, error-handling, ...

### 323.21 Refactor this function to reduce its Cognitive Complexity from 21 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGn3`
- **项目**: huanchong-99
- **行号**: L34211
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 34211min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 323.22 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGn4`
- **项目**: huanchong-99
- **行号**: L36815
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 36815min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 323.23 The signature '(commandId: string, showUI?: boolean | undefined, value?: string | undefined): boolean' of 'document.execCommand' is deprecated. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGn5`
- **项目**: huanchong-99
- **行号**: L37715
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 37715min effort
- **创建时间**: 1 month ago
- **标签**: cwe, obsolete, ...

### 323.24 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGn6`
- **项目**: huanchong-99
- **行号**: L4052
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4052min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 323.25 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGn7`
- **项目**: huanchong-99
- **行号**: L4062
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4062min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

### 323.26 Prefer `globalThis` over `window`. ✅ 已修复

- **问题ID**: `AZyVwe14Z9DOUQdEsGn8`
- **项目**: huanchong-99
- **行号**: L4072
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4072min effort
- **创建时间**: 1 month ago
- **标签**: es2020, portability

---

## 324. huanchong-99SoloDawnfrontend/vite.config.ts

> 该文件共有 **3** 个问题

### 324.1 Prefer `node:path` over `path`. ✅ 已修复

- **问题ID**: `AZyVwe3jZ9DOUQdEsGos`
- **项目**: huanchong-99
- **行号**: L55
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 55min effort
- **创建时间**: 1 month ago
- **标签**: convention, import, ...

### 324.2 Prefer `node:fs` over `fs`. ✅ 已修复

- **问题ID**: `AZyVwe3jZ9DOUQdEsGot`
- **项目**: huanchong-99
- **行号**: L65
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 65min effort
- **创建时间**: 1 month ago
- **标签**: convention, import, ...

### 324.3 Prefer `Number.parseInt` over `parseInt`. ✅ 已修复

- **问题ID**: `AZyVwe3jZ9DOUQdEsGou`
- **项目**: huanchong-99
- **行号**: L852
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 852min effort
- **创建时间**: 24 days ago
- **标签**: convention, es2015

---

## 325. huanchong-99SoloDawnfrontend/vitest.config.ts

> 该文件共有 **2** 个问题

### 325.1 Prefer `node:path` over `path`. ✅ 已修复

- **问题ID**: `AZyVwe3rZ9DOUQdEsGov`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 1 month ago
- **标签**: convention, import, ...

### 325.2 Prefer `node:fs` over `fs`. ✅ 已修复

- **问题ID**: `AZyVwe3rZ9DOUQdEsGow`
- **项目**: huanchong-99
- **行号**: L45
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 45min effort
- **创建时间**: 27 days ago
- **标签**: convention, import, ...

---

## 326. huanchong-99SoloDawnscripts/audit-security.sh

> 该文件共有 **9** 个问题

### 326.1 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7ZZ9DOUQdEsGqE`
- **项目**: huanchong-99
- **行号**: L92
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 92min effort
- **创建时间**: 28 days ago
- **标签**: best-practice, clarity, ...

### 326.2 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7ZZ9DOUQdEsGqF`
- **项目**: huanchong-99
- **行号**: L132
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 132min effort
- **创建时间**: 28 days ago
- **标签**: best-practice, clarity, ...

### 326.3 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7ZZ9DOUQdEsGqG`
- **项目**: huanchong-99
- **行号**: L232
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 232min effort
- **创建时间**: 28 days ago
- **标签**: best-practice, clarity, ...

### 326.4 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7ZZ9DOUQdEsGp-`
- **项目**: huanchong-99
- **行号**: L262
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 262min effort
- **创建时间**: 28 days ago
- **标签**: bash, best-practices, ...

### 326.5 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7ZZ9DOUQdEsGp_`
- **项目**: huanchong-99
- **行号**: L522
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 522min effort
- **创建时间**: 28 days ago
- **标签**: bash, best-practices, ...

### 326.6 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7ZZ9DOUQdEsGqA`
- **项目**: huanchong-99
- **行号**: L632
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 632min effort
- **创建时间**: 28 days ago
- **标签**: bash, best-practices, ...

### 326.7 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7ZZ9DOUQdEsGqB`
- **项目**: huanchong-99
- **行号**: L702
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 702min effort
- **创建时间**: 28 days ago
- **标签**: bash, best-practices, ...

### 326.8 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7ZZ9DOUQdEsGqC`
- **项目**: huanchong-99
- **行号**: L892
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 892min effort
- **创建时间**: 28 days ago
- **标签**: bash, best-practices, ...

### 326.9 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7ZZ9DOUQdEsGqD`
- **项目**: huanchong-99
- **行号**: L1062
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1062min effort
- **创建时间**: 28 days ago
- **标签**: bash, best-practices, ...

---

## 327. huanchong-99SoloDawnscripts/check-i18n.sh

> 该文件共有 **15** 个问题

### 327.1 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqU`
- **项目**: huanchong-99
- **行号**: L102
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 102min effort
- **创建时间**: 1 month ago
- **标签**: best-practice, clarity, ...

### 327.2 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqI`
- **项目**: huanchong-99
- **行号**: L372
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 372min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.3 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqJ`
- **项目**: huanchong-99
- **行号**: L492
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 492min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.4 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqK`
- **项目**: huanchong-99
- **行号**: L692
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 692min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.5 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqL`
- **项目**: huanchong-99
- **行号**: L982
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 982min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.6 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqM`
- **项目**: huanchong-99
- **行号**: L1302
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1302min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.7 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqN`
- **项目**: huanchong-99
- **行号**: L1492
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1492min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.8 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqO`
- **项目**: huanchong-99
- **行号**: L1512
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1512min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.9 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqP`
- **项目**: huanchong-99
- **行号**: L1572
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1572min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.10 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqQ`
- **项目**: huanchong-99
- **行号**: L1642
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1642min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.11 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqR`
- **项目**: huanchong-99
- **行号**: L1652
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1652min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.12 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqS`
- **项目**: huanchong-99
- **行号**: L1672
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1672min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.13 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqT`
- **项目**: huanchong-99
- **行号**: L1712
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1712min effort
- **创建时间**: 1 month ago
- **标签**: bash, best-practices, ...

### 327.14 Define a constant instead of using the literal '   - %s\n' 7 times. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqH`
- **项目**: huanchong-99
- **行号**: L1714
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 1714min effort
- **创建时间**: 1 month ago
- **标签**: design

### 327.15 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqV`
- **项目**: huanchong-99
- **行号**: L1882
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1882min effort
- **创建时间**: 1 month ago
- **标签**: best-practice, clarity, ...

---

## 328. huanchong-99SoloDawnscripts/docker/e2e-smoke.sh

> 该文件共有 **6** 个问题

### 328.1 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe6yZ9DOUQdEsGpu`
- **项目**: huanchong-99
- **行号**: L102
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 102min effort
- **创建时间**: 1 day ago
- **标签**: best-practice, clarity, ...

### 328.2 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复 ✅ 已修复

- **问题ID**: `AZyVwe6yZ9DOUQdEsGpp`
- **项目**: huanchong-99
- **行号**: L182
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 182min effort
- **创建时间**: 1 day ago
- **标签**: bash, best-practices, ...

### 328.3 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复 ✅ 已修复

- **问题ID**: `AZyVwe6yZ9DOUQdEsGpq`
- **项目**: huanchong-99
- **行号**: L262
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 262min effort
- **创建时间**: 1 day ago
- **标签**: bash, best-practices, ...

### 328.4 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复 ✅ 已修复

- **问题ID**: `AZyVwe6yZ9DOUQdEsGpr`
- **项目**: huanchong-99
- **行号**: L442
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 442min effort
- **创建时间**: 1 day ago
- **标签**: bash, best-practices, ...

### 328.5 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复 ✅ 已修复

- **问题ID**: `AZyVwe6yZ9DOUQdEsGps`
- **项目**: huanchong-99
- **行号**: L542
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 542min effort
- **创建时间**: 1 day ago
- **标签**: bash, best-practices, ...

### 328.6 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复 ✅ 已修复

- **问题ID**: `AZyVwe6yZ9DOUQdEsGpt`
- **项目**: huanchong-99
- **行号**: L622
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 622min effort
- **创建时间**: 1 day ago
- **标签**: bash, best-practices, ...

---

## 329. huanchong-99SoloDawnscripts/docker/install/lib/common.sh

> 该文件共有 **7** 个问题

### 329.1 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe66Z9DOUQdEsGpx`
- **项目**: huanchong-99
- **行号**: L122
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 122min effort
- **创建时间**: 2 days ago
- **标签**: best-practice, clarity, ...

### 329.2 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe66Z9DOUQdEsGpy`
- **项目**: huanchong-99
- **行号**: L132
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 132min effort
- **创建时间**: 2 days ago
- **标签**: best-practice, clarity, ...

### 329.3 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe66Z9DOUQdEsGpz`
- **项目**: huanchong-99
- **行号**: L142
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 142min effort
- **创建时间**: 2 days ago
- **标签**: best-practice, clarity, ...

### 329.4 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe66Z9DOUQdEsGp0`
- **项目**: huanchong-99
- **行号**: L182
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 182min effort
- **创建时间**: 2 days ago
- **标签**: best-practice, clarity, ...

### 329.5 Assign this positional parameter to a local variable. ✅ 已修复

- **问题ID**: `AZyVwe66Z9DOUQdEsGpv`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 2 days ago
- **标签**: readability, shell

### 329.6 Assign this positional parameter to a local variable. ✅ 已修复

- **问题ID**: `AZyVwe66Z9DOUQdEsGpw`
- **项目**: huanchong-99
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 195min effort
- **创建时间**: 2 days ago
- **标签**: readability, shell

### 329.7 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe66Z9DOUQdEsGp1`
- **项目**: huanchong-99
- **行号**: L242
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 242min effort
- **创建时间**: 22 hours ago
- **标签**: best-practice, clarity, ...

---

## 330. huanchong-99SoloDawnscripts/docker/install/verify-all-clis.sh

> 该文件共有 **2** 个问题

### 330.1 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7BZ9DOUQdEsGp2`
- **项目**: huanchong-99
- **行号**: L112
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 112min effort
- **创建时间**: 2 days ago
- **标签**: best-practice, clarity, ...

### 330.2 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7BZ9DOUQdEsGp3`
- **项目**: huanchong-99
- **行号**: L212
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 212min effort
- **创建时间**: 2 days ago
- **标签**: best-practice, clarity, ...

---

## 331. huanchong-99SoloDawnscripts/migrate_auto_confirm.sh

> 该文件共有 **4** 个问题

### 331.1 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7KZ9DOUQdEsGp4`
- **项目**: huanchong-99
- **行号**: L252
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 252min effort
- **创建时间**: 19 days ago
- **标签**: best-practice, clarity, ...

### 331.2 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7KZ9DOUQdEsGp5`
- **项目**: huanchong-99
- **行号**: L1052
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1052min effort
- **创建时间**: 19 days ago
- **标签**: best-practice, clarity, ...

### 331.3 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7KZ9DOUQdEsGp6`
- **项目**: huanchong-99
- **行号**: L1142
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1142min effort
- **创建时间**: 19 days ago
- **标签**: best-practice, clarity, ...

### 331.4 Add an explicit return statement at the end of the function. ✅ 已修复

- **问题ID**: `AZyVwe7KZ9DOUQdEsGp7`
- **项目**: huanchong-99
- **行号**: L1192
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1192min effort
- **创建时间**: 19 days ago
- **标签**: best-practice, clarity, ...

---

## 332. huanchong-99SoloDawnscripts/prepare-db.js

> 该文件共有 **3** 个问题

### 332.1 Prefer `node:child_process` over `child_process`. ✅ 已修复

- **问题ID**: `AZyVwe70Z9DOUQdEsGqf`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 1 month ago
- **标签**: convention, import, ...

### 332.2 Prefer `node:fs` over `fs`. ✅ 已修复

- **问题ID**: `AZyVwe70Z9DOUQdEsGqg`
- **项目**: huanchong-99
- **行号**: L45
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 45min effort
- **创建时间**: 1 month ago
- **标签**: convention, import, ...

### 332.3 Prefer `node:path` over `path`. ✅ 已修复

- **问题ID**: `AZyVwe70Z9DOUQdEsGqh`
- **项目**: huanchong-99
- **行号**: L55
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 55min effort
- **创建时间**: 1 month ago
- **标签**: convention, import, ...

---

## 333. huanchong-99SoloDawnscripts/run-dev.js

> 该文件共有 **7** 个问题

### 333.1 Prefer `node:fs` over `fs`. ✅ 已修复

- **问题ID**: `AZyVwe7rZ9DOUQdEsGqW`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 2 days ago
- **标签**: convention, import, ...

### 333.2 Prefer `node:os` over `os`. ✅ 已修复

- **问题ID**: `AZyVwe7rZ9DOUQdEsGqX`
- **项目**: huanchong-99
- **行号**: L45
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 45min effort
- **创建时间**: 2 days ago
- **标签**: convention, import, ...

### 333.3 Prefer `node:path` over `path`. ✅ 已修复

- **问题ID**: `AZyVwe7rZ9DOUQdEsGqY`
- **项目**: huanchong-99
- **行号**: L55
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 55min effort
- **创建时间**: 27 days ago
- **标签**: convention, import, ...

### 333.4 Prefer `node:net` over `net`. ✅ 已修复

- **问题ID**: `AZyVwe7rZ9DOUQdEsGqZ`
- **项目**: huanchong-99
- **行号**: L65
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 65min effort
- **创建时间**: 2 days ago
- **标签**: convention, import, ...

### 333.5 Prefer `node:child_process` over `child_process`. ✅ 已修复

- **问题ID**: `AZyVwe7rZ9DOUQdEsGqa`
- **项目**: huanchong-99
- **行号**: L75
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 75min effort
- **创建时间**: 2 days ago
- **标签**: convention, import, ...

### 333.6 Prefer `node:readline` over `readline`. ✅ 已修复

- **问题ID**: `AZyVwe7rZ9DOUQdEsGqd`
- **项目**: huanchong-99
- **行号**: L3885
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 3885min effort
- **创建时间**: 27 days ago
- **标签**: convention, import, ...

### 333.7 Prefer top-level await over an async function `main` call. ✅ 已修复

- **问题ID**: `AZyVwe7rZ9DOUQdEsGqe`
- **项目**: huanchong-99
- **行号**: L4665
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4665min effort
- **创建时间**: 27 days ago
- **标签**: async, es2022, ...

---

## 334. huanchong-99SoloDawnscripts/run-frontend-dev.js

> 该文件共有 **2** 个问题

### 334.1 Prefer `node:path` over `path`. ✅ 已修复

- **问题ID**: `AZyVwe7SZ9DOUQdEsGp8`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 12 days ago
- **标签**: convention, import, ...

### 334.2 Prefer `node:child_process` over `child_process`. ✅ 已修复

- **问题ID**: `AZyVwe7SZ9DOUQdEsGp9`
- **项目**: huanchong-99
- **行号**: L45
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 45min effort
- **创建时间**: 12 days ago
- **标签**: convention, import, ...

---

## 335. huanchong-99SoloDawnscripts/setup-dev-environment.js

> 该文件共有 **6** 个问题

### 335.1 Prefer `node:fs` over `fs`. ✅ 已修复

- **问题ID**: `AZyVwe78Z9DOUQdEsGqk`
- **项目**: huanchong-99
- **行号**: L35
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 35min effort
- **创建时间**: 1 month ago
- **标签**: convention, import, ...

### 335.2 Prefer `node:path` over `path`. ✅ 已修复

- **问题ID**: `AZyVwe78Z9DOUQdEsGql`
- **项目**: huanchong-99
- **行号**: L45
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 45min effort
- **创建时间**: 1 month ago
- **标签**: convention, import, ...

### 335.3 Prefer `node:net` over `net`. ✅ 已修复

- **问题ID**: `AZyVwe78Z9DOUQdEsGqm`
- **项目**: huanchong-99
- **行号**: L55
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 55min effort
- **创建时间**: 1 month ago
- **标签**: convention, import, ...

### 335.4 Prefer top-level await over using a promise chain. ✅ 已修复

- **问题ID**: `AZyVwe78Z9DOUQdEsGqn`
- **项目**: huanchong-99
- **行号**: L1005
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1005min effort
- **创建时间**: 1 month ago
- **标签**: async, es2022, ...

### 335.5 Prefer top-level await over using a promise chain. ✅ 已修复

- **问题ID**: `AZyVwe78Z9DOUQdEsGqo`
- **项目**: huanchong-99
- **行号**: L1125
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1125min effort
- **创建时间**: 1 month ago
- **标签**: async, es2022, ...

### 335.6 Prefer top-level await over using a promise chain. ✅ 已修复

- **问题ID**: `AZyVwe78Z9DOUQdEsGqp`
- **项目**: huanchong-99
- **行号**: L1205
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1205min effort
- **创建时间**: 1 month ago
- **标签**: async, es2022, ...

---

## 336. huanchong-99SoloDawnscripts/verify-baseline.sh

> 该文件共有 **1** 个问题

### 336.1 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复 ✅ 已修复

- **问题ID**: `AZyVwe6rZ9DOUQdEsGpo`
- **项目**: huanchong-99
- **行号**: L142
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 142min effort
- **创建时间**: 29 days ago
- **标签**: bash, best-practices, ...

---

