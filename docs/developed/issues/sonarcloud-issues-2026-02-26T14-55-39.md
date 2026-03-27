# SonarCloud Issues 报告

**生成时间**: 2026/02/26 22:55
**问题总数**: 352
**已加载**: 352
**收集数量**: 352

---

## 统计信息

### 按严重程度分类

- **Minor**: 159 个
- **Major**: 132 个
- **Critical**: 48 个
- **Info**: 7 个
- **Blocker**: 6 个

### 按类型分类

- **Code Smell**: 323 个
- **Bug**: 28 个
- **Vulnerability**: 1 个

### 按影响分类

- **Maintainability**: 287 个
- **Reliability**: 64 个
- **Security**: 1 个

### 按属性分类

- **Intentionality**: 174 个
- **Consistency**: 131 个
- **Adaptability**: 47 个

### 按文件统计 (Top 20)

- **huanchong-99SoloDawnfrontend/.../components/ui-new/containers/NewDisplayConversationEntry.tsx**: 14 个问题
- **huanchong-99SoloDawnfrontend/src/vscode/bridge.ts**: 10 个问题
- **huanchong-99SoloDawnfrontend/src/components/tasks/TaskFollowUpSection.tsx**: 8 个问题
- **huanchong-99SoloDawnfrontend/src/hooks/useProjectTasks.ts**: 8 个问题
- **huanchong-99SoloDawnfrontend/src/stores/wsStore.ts**: 8 个问题
- **huanchong-99SoloDawncrates/db/migrations/20250730000000_add_executor_action_to_execution_processes.sql**: 7 个问题
- **huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatFileEntry.tsx**: 7 个问题
- **huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/plugins/file-tag-typeahead-plugin.tsx**: 7 个问题
- **huanchong-99SoloDawncrates/db/migrations/20251209000000_add_project_repositories.sql**: 6 个问题
- **huanchong-99SoloDawnfrontend/.../NormalizedConversation/DisplayConversationEntry.tsx**: 6 个问题
- **huanchong-99SoloDawnfrontend/.../components/ui-new/containers/DiffViewCardWithComments.tsx**: 6 个问题
- **huanchong-99SoloDawnfrontend/src/contexts/ClickedElementsProvider.tsx**: 6 个问题
- **huanchong-99SoloDawnfrontend/src/components/DiffCard.tsx**: 5 个问题
- **huanchong-99SoloDawnfrontend/src/components/ui-new/views/PreviewBrowser.tsx**: 5 个问题
- **huanchong-99SoloDawnfrontend/src/hooks/useConversationHistory.ts**: 5 个问题
- **huanchong-99SoloDawncrates/db/migrations/20250716143725_add_default_templates.sql**: 4 个问题
- **huanchong-99SoloDawncrates/db/migrations/20260119000001_add_performance_indexes.sql**: 4 个问题
- **huanchong-99SoloDawnfrontend/src/components/dialogs/scripts/ScriptFixerDialog.tsx**: 4 个问题
- **huanchong-99SoloDawnfrontend/src/components/dialogs/shared/RepoPickerDialog.tsx**: 4 个问题
- **huanchong-99SoloDawnfrontend/src/components/terminal/TerminalDebugView.tsx**: 4 个问题

---

## 问题列表（按文件分组）

## 1. huanchong-99SoloDawncrates/db/migrations/20250617183714_init.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 1.1 Define a constant instead of duplicating this literal 7 times. ✅ 已修复

- **问题ID**: `AZyVwe6BZ9DOUQdEsGpi`
- **项目**: huanchong-99
- **行号**: L104
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 104min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 2. huanchong-99SoloDawncrates/db/migrations/20250620212427_execution_processes.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 2.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe4rZ9DOUQdEsGo7`
- **项目**: huanchong-99
- **行号**: L184
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 184min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 3. huanchong-99SoloDawncrates/db/migrations/20250716143725_add_default_templates.sql ✅ 已修复

> 该文件共有 **4** 个问题

### 3.1 An illegal character with code point 10 was found in this literal. ✅ 已修复

- **问题ID**: `AZyVwe5tZ9DOUQdEsGpd`
- **项目**: huanchong-99
- **行号**: L2110
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2110min effort
- **创建时间**: 1 month ago
- **标签**: pitfall

### 3.2 Define a constant instead of duplicating this literal 6 times. ✅ 已修复

- **问题ID**: `AZyVwe5tZ9DOUQdEsGpg`
- **项目**: huanchong-99
- **行号**: L594
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 594min effort
- **创建时间**: 1 month ago
- **标签**: design

### 3.3 An illegal character with code point 10 was found in this literal. ✅ 已修复

- **问题ID**: `AZyVwe5tZ9DOUQdEsGpe`
- **项目**: huanchong-99
- **行号**: L7610
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 7610min effort
- **创建时间**: 1 month ago
- **标签**: pitfall

### 3.4 An illegal character with code point 10 was found in this literal. ✅ 已修复

- **问题ID**: `AZyVwe5tZ9DOUQdEsGpf`
- **项目**: huanchong-99
- **行号**: L12910
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 12910min effort
- **创建时间**: 1 month ago
- **标签**: pitfall

---

## 4. huanchong-99SoloDawncrates/db/migrations/20250720000000_add_cleanupscript_to_process_type_constraint.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 4.1 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe6ZZ9DOUQdEsGpl`
- **项目**: huanchong-99
- **行号**: L1130
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1130min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 5. huanchong-99SoloDawncrates/db/migrations/20250730000000_add_executor_action_to_execution_processes.sql ✅ 已修复

> 该文件共有 **7** 个问题

### 5.1 Define a constant instead of duplicating this literal 4 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGpC`
- **项目**: huanchong-99
- **行号**: L234
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 234min effort
- **创建时间**: 1 day ago
- **标签**: design

### 5.2 Define a constant instead of duplicating this literal 4 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGo_`
- **项目**: huanchong-99
- **行号**: L254
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 254min effort
- **创建时间**: 1 day ago
- **标签**: design

### 5.3 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGpD`
- **项目**: huanchong-99
- **行号**: L294
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 294min effort
- **创建时间**: 1 day ago
- **标签**: design

### 5.4 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGpA`
- **项目**: huanchong-99
- **行号**: L304
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 304min effort
- **创建时间**: 1 day ago
- **标签**: design

### 5.5 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGpB`
- **项目**: huanchong-99
- **行号**: L314
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 314min effort
- **创建时间**: 1 day ago
- **标签**: design

### 5.6 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGo-`
- **项目**: huanchong-99
- **行号**: L324
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 324min effort
- **创建时间**: 1 day ago
- **标签**: design

### 5.7 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe47Z9DOUQdEsGo9`
- **项目**: huanchong-99
- **行号**: L5810
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 5810min effort
- **创建时间**: 1 day ago
- **标签**: sql

---

## 6. huanchong-99SoloDawncrates/db/migrations/20250815100344_migrate_old_executor_actions.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 6.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe4TZ9DOUQdEsGo4`
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

## 7. huanchong-99SoloDawncrates/db/migrations/20250818150000_refactor_images_to_junction_tables.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 7.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe6JZ9DOUQdEsGpj`
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

## 8. huanchong-99SoloDawncrates/db/migrations/20250819000000_move_merge_commit_to_merges_table.sql ✅ 已修复

> 该文件共有 **3** 个问题

### 8.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe31Z9DOUQdEsGoz`
- **项目**: huanchong-99
- **行号**: L94
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 94min effort
- **创建时间**: 1 month ago
- **标签**: design

### 8.2 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe31Z9DOUQdEsGox`
- **项目**: huanchong-99
- **行号**: L174
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 174min effort
- **创建时间**: 1 month ago
- **标签**: design

### 8.3 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe31Z9DOUQdEsGoy`
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

## 9. huanchong-99SoloDawncrates/db/migrations/20250921222241_unify_drafts_tables.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 9.1 Refactor this SQL query to eliminate the use of EXISTS. ✅ 已修复

- **问题ID**: `AZyVwe4zZ9DOUQdEsGo8`
- **项目**: huanchong-99
- **行号**: L721
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 day ago
- **标签**: performance, sql

---

## 10. huanchong-99SoloDawncrates/db/migrations/20251020120000_convert_templates_to_tags.sql ✅ 已修复

> 该文件共有 **2** 个问题

### 10.1 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5SZ9DOUQdEsGpS`
- **项目**: huanchong-99
- **行号**: L1010
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1010min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 10.2 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5SZ9DOUQdEsGpT`
- **项目**: huanchong-99
- **行号**: L2510
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2510min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 11. huanchong-99SoloDawncrates/db/migrations/20251114000000_create_shared_tasks.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 11.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe4DZ9DOUQdEsGo1`
- **项目**: huanchong-99
- **行号**: L184
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 184min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 12. huanchong-99SoloDawncrates/db/migrations/20251202000000_migrate_to_electric.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 12.1 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe37Z9DOUQdEsGo0`
- **项目**: huanchong-99
- **行号**: L1130
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1130min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 13. huanchong-99SoloDawncrates/db/migrations/20251209000000_add_project_repositories.sql ✅ 已修复

> 该文件共有 **6** 个问题

### 13.1 Define a constant instead of duplicating this literal 8 times. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpc`
- **项目**: huanchong-99
- **行号**: L134
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 134min effort
- **创建时间**: 1 month ago
- **标签**: design

### 13.2 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpX`
- **项目**: huanchong-99
- **行号**: L7910
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 7910min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 13.3 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpY`
- **项目**: huanchong-99
- **行号**: L9210
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 9210min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 13.4 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpZ`
- **项目**: huanchong-99
- **行号**: L10930
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 10930min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 13.5 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpa`
- **项目**: huanchong-99
- **行号**: L13330
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 13330min effort
- **创建时间**: 1 day ago
- **标签**: sql

### 13.6 The number of join conditions 4 exceeds the maximum allowed 3. ✅ 已修复

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpb`
- **项目**: huanchong-99
- **行号**: L1652
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Adaptability
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: brain-overload, performance, ...

---

## 14. huanchong-99SoloDawncrates/db/migrations/20251216142123_refactor_task_attempts_to_workspaces_sessions.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 14.1 Define a constant instead of duplicating this literal 7 times. ✅ 已修复

- **问题ID**: `AZyVwe4cZ9DOUQdEsGo5`
- **项目**: huanchong-99
- **行号**: L204
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 204min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 15. huanchong-99SoloDawncrates/db/migrations/20251219000000_add_agent_working_dir_to_projects.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 15.1 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe5LZ9DOUQdEsGpR`
- **项目**: huanchong-99
- **行号**: L1110
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1110min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 16. huanchong-99SoloDawncrates/db/migrations/20251220134608_fix_session_executor_format.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 16.1 ✅ 已修复 - Refactor this SQL query to prevent doing a full table scan due to the value of the "LIKE" condition

- **问题ID**: `AZyVwe51Z9DOUQdEsGph`
- **项目**: huanchong-99
- **行号**: L103
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: performance, sql
- **修复方式**: 将 LIKE '%:%' 替换为 instr(executor, ':') > 0，避免全表扫描警告

---

## 17. huanchong-99SoloDawncrates/db/migrations/20260107000000_move_scripts_to_repos.sql ✅ 已修复

> 该文件共有 **2** 个问题

### 17.1 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe4KZ9DOUQdEsGo2`
- **项目**: huanchong-99
- **行号**: L1430
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1430min effort
- **创建时间**: 1 month ago
- **标签**: sql

### 17.2 Ensure that the WHERE clause is not missing in this UPDATE query. ✅ 已修复

- **问题ID**: `AZyVwe4KZ9DOUQdEsGo3`
- **项目**: huanchong-99
- **行号**: L5430
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 5430min effort
- **创建时间**: 1 month ago
- **标签**: sql

---

## 18. huanchong-99SoloDawncrates/db/migrations/20260117000001_create_workflow_tables.sql ✅ 已修复

> 该文件共有 **3** 个问题

### 18.1 Define a constant instead of duplicating this literal 4 times. ✅ 已修复

- **问题ID**: `AZyVwe5bZ9DOUQdEsGpW`
- **项目**: huanchong-99
- **行号**: L294
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 294min effort
- **创建时间**: 1 month ago
- **标签**: design

### 18.2 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5bZ9DOUQdEsGpU`
- **项目**: huanchong-99
- **行号**: L304
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 304min effort
- **创建时间**: 1 month ago
- **标签**: design

### 18.3 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5bZ9DOUQdEsGpV`
- **项目**: huanchong-99
- **行号**: L314
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 314min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 19. huanchong-99SoloDawncrates/db/migrations/20260119000001_add_performance_indexes.sql ✅ 已修复

> 该文件共有 **4** 个问题

### 19.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpN`
- **项目**: huanchong-99
- **行号**: L234
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 234min effort
- **创建时间**: 1 month ago
- **标签**: design

### 19.2 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpO`
- **项目**: huanchong-99
- **行号**: L234
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 234min effort
- **创建时间**: 1 month ago
- **标签**: design

### 19.3 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpP`
- **项目**: huanchong-99
- **行号**: L234
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 234min effort
- **创建时间**: 1 month ago
- **标签**: design

### 19.4 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe5DZ9DOUQdEsGpQ`
- **项目**: huanchong-99
- **行号**: L414
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 414min effort
- **创建时间**: 1 month ago
- **标签**: design

---

## 20. huanchong-99SoloDawncrates/db/migrations/20260208010000_backfill_terminal_auto_confirm.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 20.1 Refactor this SQL query to eliminate the use of EXISTS. ✅ 已修复

- **问题ID**: `AZyVwe6RZ9DOUQdEsGpk`
- **项目**: huanchong-99
- **行号**: L131
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **创建时间**: 18 days ago
- **标签**: performance, sql

---

## 21. huanchong-99SoloDawncrates/db/migrations/20260208020000_fix_terminal_old_foreign_keys.sql ✅ 已修复

> 该文件共有 **1** 个问题

### 21.1 Define a constant instead of duplicating this literal 3 times. ✅ 已修复

- **问题ID**: `AZyVwe4kZ9DOUQdEsGo6`
- **项目**: huanchong-99
- **行号**: L334
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 334min effort
- **创建时间**: 17 days ago
- **标签**: design

---

## 22. huanchong-99SoloDawncrates/db/migrations/20260224001000_backfill_workflow_api_key_encrypted.sql ✅ 已修复

> 该文件共有 **2** 个问题

### 22.1 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe6gZ9DOUQdEsGpm`
- **项目**: huanchong-99
- **行号**: L1710
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1710min effort
- **创建时间**: 1 day ago
- **标签**: sql

### 22.2 Use IS NULL and IS NOT NULL instead of direct NULL comparisons. ✅ 已修复

- **问题ID**: `AZyVwe6gZ9DOUQdEsGpn`
- **项目**: huanchong-99
- **行号**: L1910
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1910min effort
- **创建时间**: 1 day ago
- **标签**: sql

---

## 23. huanchong-99SoloDawndocker/Dockerfile ✅ 已修复

> 该文件共有 **1** 个问题

### 23.1 Merge this RUN instruction with the consecutive ones. ✅ 已修复

- **问题ID**: `AZyVwe8EZ9DOUQdEsGqq`
- **项目**: huanchong-99
- **行号**: L375
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 375min effort
- **创建时间**: 1 day ago

---

## 24. huanchong-99SoloDawnfrontend/.../NormalizedConversation/DisplayConversationEntry.tsx

> 该文件共有 **6** 个问题

### 24.1 Refactor this function to reduce its Cognitive Complexity from 18 to the 15 allowed.

- **问题ID**: `AZyVweX9Z9DOUQdEsGeH`
- **项目**: huanchong-99
- **行号**: L608
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 608min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 24.2 Complete the task associated to this "TODO" comment.

- **问题ID**: `AZyVweX9Z9DOUQdEsGeI`
- **项目**: huanchong-99
- **行号**: L800
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 800min effort
- **创建时间**: 1 month ago
- **标签**: cwe

### 24.3 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

- **问题ID**: `AZyZVcMjuNB-_5CPqJgK`
- **项目**: huanchong-99
- **行号**: L2265
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2265min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 24.4 Refactor this function to reduce its Cognitive Complexity from 18 to the 15 allowed.

- **问题ID**: `AZyVweX9Z9DOUQdEsGeL`
- **项目**: huanchong-99
- **行号**: L4638
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 4638min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 24.5 Refactor this function to reduce its Cognitive Complexity from 20 to the 15 allowed.

- **问题ID**: `AZyVweX9Z9DOUQdEsGeN`
- **项目**: huanchong-99
- **行号**: L71210
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 71210min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 24.6 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZyVweX9Z9DOUQdEsGeS`
- **项目**: huanchong-99
- **行号**: L9545
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 9545min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 25. huanchong-99SoloDawnfrontend/.../NormalizedConversation/EditDiffRenderer.tsx

> 该文件共有 **2** 个问题

### 25.1 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

- **问题ID**: `AZyZVcM2uNB-_5CPqJgL`
- **项目**: huanchong-99
- **行号**: L1045
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1045min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 25.2 Non-interactive elements should not be assigned interactive roles.

- **问题ID**: `AZyZVcM2uNB-_5CPqJgM`
- **项目**: huanchong-99
- **行号**: L1055
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1055min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

---

## 26. huanchong-99SoloDawnfrontend/.../NormalizedConversation/FileChangeRenderer.tsx

> 该文件共有 **2** 个问题

### 26.1 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

- **问题ID**: `AZyZVcNGuNB-_5CPqJgN`
- **项目**: huanchong-99
- **行号**: L1405
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1405min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 26.2 Non-interactive elements should not be assigned interactive roles.

- **问题ID**: `AZyZVcNGuNB-_5CPqJgO`
- **项目**: huanchong-99
- **行号**: L1415
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1415min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

---

## 27. huanchong-99SoloDawnfrontend/.../NormalizedConversation/NextActionCard.tsx

> 该文件共有 **1** 个问题

### 27.1 Refactor this function to reduce its Cognitive Complexity from 22 to the 15 allowed.

- **问题ID**: `AZyVweYgZ9DOUQdEsGeh`
- **项目**: huanchong-99
- **行号**: L12312
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 12312min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

---

## 28. huanchong-99SoloDawnfrontend/.../components/dialogs/projects/LinkProjectDialog.tsx

> 该文件共有 **1** 个问题

### 28.1 Unexpected negated condition.

- **问题ID**: `AZyZxP-b4NBSmYbRRYRr`
- **项目**: huanchong-99
- **行号**: L2562
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2562min effort
- **创建时间**: 3 hours ago
- **标签**: readability

---

## 29. huanchong-99SoloDawnfrontend/.../components/tasks/TaskDetails/ProcessLogsViewer.tsx

> 该文件共有 **1** 个问题

### 29.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

---

## 30. huanchong-99SoloDawnfrontend/.../components/tasks/TaskDetails/ProcessesTab.tsx

> 该文件共有 **3** 个问题

### 30.1 Unexpected negated condition.

- **问题ID**: `AZyZXLZ2BQ5dxpgmwNpg`
- **项目**: huanchong-99
- **行号**: L2652
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2652min effort
- **创建时间**: 5 hours ago
- **标签**: readability

### 30.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyZXLZ2BQ5dxpgmwNph`
- **项目**: huanchong-99
- **行号**: L2815
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2815min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 30.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

- **问题ID**: `AZyZXLZ2BQ5dxpgmwNpi`
- **项目**: huanchong-99
- **行号**: L2815
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2815min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

---

## 31. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ContextBarContainer.tsx

> 该文件共有 **2** 个问题

### 31.1 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc2oNp-e-LWbDubNX`
- **项目**: huanchong-99
- **行号**: L425
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 425min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

### 31.2 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc2oNp-e-LWbDubNY`
- **项目**: huanchong-99
- **行号**: L515
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 515min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

---

## 32. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ConversationListContainer.tsx

> 该文件共有 **2** 个问题

### 32.1 Move this component definition out of the parent component and pass data as props.

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

### 32.2 Move this component definition out of the parent component and pass data as props.

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

## 33. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/DiffViewCardWithComments.tsx

> 该文件共有 **6** 个问题

### 33.1 Unexpected negated condition.

- **问题ID**: `AZyVwekXZ9DOUQdEsGh3`
- **项目**: huanchong-99
- **行号**: L2902
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2902min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 33.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVwekXZ9DOUQdEsGh4`
- **项目**: huanchong-99
- **行号**: L3405
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3405min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 33.3 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

- **问题ID**: `AZyZVcV1uNB-_5CPqJgc`
- **项目**: huanchong-99
- **行号**: L3405
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 3405min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 33.4 `tabIndex` should only be declared on interactive elements.

- **问题ID**: `AZyZVcV1uNB-_5CPqJgd`
- **项目**: huanchong-99
- **行号**: L3425
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 3425min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 33.5 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVwekXZ9DOUQdEsGh6`
- **项目**: huanchong-99
- **行号**: L4165
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4165min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 33.6 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

- **问题ID**: `AZyVwekXZ9DOUQdEsGh7`
- **项目**: huanchong-99
- **行号**: L4165
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 4165min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 34. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/NavbarContainer.tsx

> 该文件共有 **2** 个问题

### 34.1 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc2rwp-e-LWbDubNZ`
- **项目**: huanchong-99
- **行号**: L435
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 435min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

### 34.2 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc2rwp-e-LWbDubNa`
- **项目**: huanchong-99
- **行号**: L525
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 525min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

---

## 35. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/NewDisplayConversationEntry.tsx

> 该文件共有 **14** 个问题

### 35.1 Refactor this function to reduce its Cognitive Complexity from 16 to the 15 allowed.

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

### 35.2 Complete the task associated to this "TODO" comment.

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

### 35.3 Mark the props of the component as read-only. ✅ 已修复

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

### 35.4 Mark the props of the component as read-only. ✅ 已修复

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

### 35.5 Mark the props of the component as read-only. ✅ 已修复

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

### 35.6 Mark the props of the component as read-only. ✅ 已修复

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

### 35.7 Mark the props of the component as read-only. ✅ 已修复

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

### 35.8 Mark the props of the component as read-only. ✅ 已修复

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

### 35.9 Complete the task associated to this "TODO" comment.

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

### 35.10 Mark the props of the component as read-only. ✅ 已修复

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

### 35.11 Mark the props of the component as read-only. ✅ 已修复

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

### 35.12 Mark the props of the component as read-only. ✅ 已修复

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

### 35.13 Extract this nested ternary operation into an independent statement.

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

### 35.14 Mark the props of the component as read-only. ✅ 已修复

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

## 36. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ProcessListContainer.tsx

> 该文件共有 **1** 个问题

### 36.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

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

## 37. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ProjectSelectorContainer.tsx

> 该文件共有 **1** 个问题

### 37.1 Move this component definition out of the parent component and pass data as props.

- **问题ID**: `AZyVweirZ9DOUQdEsGhi`
- **项目**: huanchong-99
- **行号**: L2025
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2025min effort
- **创建时间**: 1 month ago
- **标签**: jsx, performance, ...

---

## 38. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/SessionChatBoxContainer.tsx

> 该文件共有 **4** 个问题

### 38.1 This assertion is unnecessary since it does not change the type of the expression.

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

### 38.2 Remove this use of the "void" operator. ✅ 已修复

- **问题ID**: `AZyZVcVEuNB-_5CPqJgb`
- **项目**: huanchong-99
- **行号**: L6105
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6105min effort
- **创建时间**: 5 hours ago
- **标签**: confusing, type-dependent

### 38.3 Remove this use of the "void" operator.

- **问题ID**: `AZyZkrqw1b9TkhNHR2Lo`
- **项目**: huanchong-99
- **行号**: L6135
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6135min effort
- **创建时间**: 4 hours ago
- **标签**: confusing, type-dependent

### 38.4 Promise-returning function provided to property where a void return was expected.

- **问题ID**: `AZyVwei1Z9DOUQdEsGhl`
- **项目**: huanchong-99
- **行号**: L6155
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 6155min effort
- **创建时间**: 1 month ago
- **标签**: async, promise, ...

---

## 39. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/ContextBar.tsx

> 该文件共有 **1** 个问题

### 39.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

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

---

## 40. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/SearchableDropdown.tsx

> 该文件共有 **1** 个问题

### 40.1 Move this component definition out of the parent component and pass data as props.

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

## 41. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/SessionChatBox.tsx

> 该文件共有 **1** 个问题

### 41.1 Complete the task associated to this "TODO" comment.

- **问题ID**: `AZyVwegaZ9DOUQdEsGg_`
- **项目**: huanchong-99
- **行号**: L5670
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5670min effort
- **创建时间**: 1 month ago
- **标签**: cwe

---

## 42. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/WorkspaceSummary.tsx

> 该文件共有 **1** 个问题

### 42.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

## 43. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/nodes/image-node.tsx

> 该文件共有 **1** 个问题

### 43.1 Mark the props of the component as read-only.

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

---

## 44. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/nodes/pr-comment-node.tsx

> 该文件共有 **1** 个问题

### 44.1 Mark the props of the component as read-only.

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

---

## 45. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/plugins/file-tag-typeahead-plugin.tsx

> 该文件共有 **7** 个问题

### 45.1 Mark the props of the component as read-only.

- **问题ID**: `AZyZVcAquNB-_5CPqJf_`
- **项目**: huanchong-99
- **行号**: L1335
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1335min effort
- **创建时间**: 5 hours ago
- **标签**: react, type-dependent

### 45.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweNzZ9DOUQdEsGar`
- **项目**: huanchong-99
- **行号**: L1525
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1525min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 45.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

- **问题ID**: `AZyVweNzZ9DOUQdEsGas`
- **项目**: huanchong-99
- **行号**: L1525
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1525min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 45.4 Mark the props of the component as read-only.

- **问题ID**: `AZyZVcAquNB-_5CPqJgA`
- **项目**: huanchong-99
- **行号**: L1735
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1735min effort
- **创建时间**: 5 hours ago
- **标签**: react, type-dependent

### 45.5 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweNzZ9DOUQdEsGap`
- **项目**: huanchong-99
- **行号**: L1925
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1925min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 45.6 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

- **问题ID**: `AZyVweNzZ9DOUQdEsGaq`
- **项目**: huanchong-99
- **行号**: L1925
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1925min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 45.7 Mark the props of the component as read-only.

- **问题ID**: `AZyVweNzZ9DOUQdEsGan`
- **项目**: huanchong-99
- **行号**: L2315
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2315min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 46. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/plugins/toolbar-plugin.tsx

> 该文件共有 **1** 个问题

### 46.1 Mark the props of the component as read-only.

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

---

## 47. huanchong-99SoloDawnfrontend/.../components/workflow/validators/step5Commands.ts

> 该文件共有 **1** 个问题

### 47.1 Remove this use of the "void" operator.

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

## 48. huanchong-99SoloDawnfrontend/.../tasks/TaskDetails/preview/PreviewToolbar.tsx

> 该文件共有 **1** 个问题

### 48.1 Unexpected negated condition.

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

## 49. huanchong-99SoloDawnfrontend/.../ui-new/dialogs/commandBar/useCommandBarState.ts

> 该文件共有 **2** 个问题

### 49.1 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc2r_p-e-LWbDubNb`
- **项目**: huanchong-99
- **行号**: L515
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 515min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

### 49.2 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyZVcWnuNB-_5CPqJge`
- **项目**: huanchong-99
- **行号**: L1031
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1031min effort
- **创建时间**: 5 hours ago
- **标签**: redundant, type-dependent

---

## 50. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatEntryContainer.tsx

> 该文件共有 **3** 个问题

### 50.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

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

### 50.2 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

- **问题ID**: `AZyZVcPwuNB-_5CPqJgQ`
- **项目**: huanchong-99
- **行号**: L925
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 925min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 50.3 `tabIndex` should only be declared on interactive elements.

- **问题ID**: `AZyZVcPwuNB-_5CPqJgR`
- **项目**: huanchong-99
- **行号**: L945
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 945min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

---

## 51. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatFileEntry.tsx

> 该文件共有 **7** 个问题

### 51.1 Refactor this function to reduce its Cognitive Complexity from 23 to the 15 allowed.

- **问题ID**: `AZyZVcRluNB-_5CPqJgS`
- **项目**: huanchong-99
- **行号**: L2513
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 2513min effort
- **创建时间**: 5 hours ago
- **标签**: brain-overload

### 51.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

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

### 51.3 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

- **问题ID**: `AZyZVcRluNB-_5CPqJgT`
- **项目**: huanchong-99
- **行号**: L605
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 605min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 51.4 `tabIndex` should only be declared on interactive elements.

- **问题ID**: `AZyZVcRluNB-_5CPqJgU`
- **项目**: huanchong-99
- **行号**: L625
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 625min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 51.5 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweb2Z9DOUQdEsGfw`
- **项目**: huanchong-99
- **行号**: L1385
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1385min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 51.6 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

- **问题ID**: `AZyZVcRluNB-_5CPqJgV`
- **项目**: huanchong-99
- **行号**: L1385
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1385min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 51.7 `tabIndex` should only be declared on interactive elements.

- **问题ID**: `AZyZVcRluNB-_5CPqJgW`
- **项目**: huanchong-99
- **行号**: L1405
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1405min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

---

## 52. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatToolSummary.tsx

> 该文件共有 **3** 个问题

### 52.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

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

### 52.2 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

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

### 52.3 `tabIndex` should only be declared on interactive elements.

- **问题ID**: `AZyZVcR0uNB-_5CPqJgX`
- **项目**: huanchong-99
- **行号**: L615
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 615min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

---

## 53. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/DiffViewCard.tsx

> 该文件共有 **3** 个问题

### 53.1 Mark the props of the component as read-only.

- **问题ID**: `AZyZVcPfuNB-_5CPqJgP`
- **项目**: huanchong-99
- **行号**: L1705
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1705min effort
- **创建时间**: 5 hours ago
- **标签**: react, type-dependent

### 53.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVwebUZ9DOUQdEsGfl`
- **项目**: huanchong-99
- **行号**: L2085
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2085min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 53.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

- **问题ID**: `AZyVwebUZ9DOUQdEsGfm`
- **项目**: huanchong-99
- **行号**: L2085
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2085min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 54. huanchong-99SoloDawnfrontend/src/components/DiffCard.tsx

> 该文件共有 **5** 个问题

### 54.1 Refactor this function to reduce its Cognitive Complexity from 17 to the 15 allowed.

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

### 54.2 Extract this nested ternary operation into an independent statement.

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

### 54.3 Unexpected negated condition.

- **问题ID**: `AZyZVcY3uNB-_5CPqJgi`
- **项目**: huanchong-99
- **行号**: L3282
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3282min effort
- **创建时间**: 5 hours ago
- **标签**: readability

### 54.4 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZyZVcY3uNB-_5CPqJgj`
- **项目**: huanchong-99
- **行号**: L3305
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3305min effort
- **创建时间**: 5 hours ago
- **标签**: confusing

### 54.5 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZyVweomZ9DOUQdEsGjR`
- **项目**: huanchong-99
- **行号**: L3325
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3325min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 55. huanchong-99SoloDawnfrontend/src/components/ExecutorConfigForm.tsx

> 该文件共有 **1** 个问题

### 55.1 Do not use Array index in keys

- **问题ID**: `AZyZ21thdO4WpiZegSza`
- **项目**: huanchong-99
- **行号**: L1585
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1585min effort
- **创建时间**: 2 hours ago
- **标签**: jsx, performance, ...

---

## 56. huanchong-99SoloDawnfrontend/src/components/ThemeProvider.tsx

> 该文件共有 **1** 个问题

### 56.1 useState call is not destructured into value + setter pair

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

---

## 57. huanchong-99SoloDawnfrontend/src/components/board/TerminalActivityPanel.tsx

> 该文件共有 **2** 个问题

### 57.1 Mark the props of the component as read-only.

- **问题ID**: `AZyVwenDZ9DOUQdEsGiw`
- **项目**: huanchong-99
- **行号**: L485
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 485min effort
- **创建时间**: 26 days ago
- **标签**: react, type-dependent

### 57.2 Do not use Array index in keys

- **问题ID**: `AZyZ21nsdO4WpiZegSzZ`
- **项目**: huanchong-99
- **行号**: L745
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 745min effort
- **创建时间**: 2 hours ago
- **标签**: jsx, performance, ...

---

## 58. huanchong-99SoloDawnfrontend/src/components/common/RawLogText.tsx

> 该文件共有 **1** 个问题

### 58.1 Refactor this code to not use nested template literals.

- **问题ID**: `AZyZVcapuNB-_5CPqJgk`
- **项目**: huanchong-99
- **行号**: L4010
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4010min effort
- **创建时间**: 5 hours ago
- **标签**: brain-overload, confusing

---

## 59. huanchong-99SoloDawnfrontend/src/components/dialogs/global/FeatureShowcaseDialog.tsx

> 该文件共有 **1** 个问题

### 59.1 Do not use Array index in keys ✅ 已修复

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

## 60. huanchong-99SoloDawnfrontend/src/components/dialogs/scripts/ScriptFixerDialog.tsx

> 该文件共有 **4** 个问题

### 60.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc2`
- **项目**: huanchong-99
- **行号**: L1295
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1295min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 60.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc9`
- **项目**: huanchong-99
- **行号**: L3445
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3445min effort
- **创建时间**: 27 days ago
- **标签**: confusing

### 60.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc-`
- **项目**: huanchong-99
- **行号**: L3515
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3515min effort
- **创建时间**: 27 days ago
- **标签**: confusing

### 60.4 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweTEZ9DOUQdEsGc_`
- **项目**: huanchong-99
- **行号**: L3605
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3605min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 61. huanchong-99SoloDawnfrontend/src/components/dialogs/shared/FolderPickerDialog.tsx

> 该文件共有 **3** 个问题

### 61.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element. ✅ 已修复

- **问题ID**: `AZyVweSzZ9DOUQdEsGcv`
- **项目**: huanchong-99
- **行号**: L2595
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2595min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 61.2 Visible, non-interactive elements with click handlers must have at least one keyboard listener. ✅ 已修复

- **问题ID**: `AZyVweSzZ9DOUQdEsGcw`
- **项目**: huanchong-99
- **行号**: L2595
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 2595min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 61.3 Unexpected negated condition.

- **问题ID**: `AZyVweSzZ9DOUQdEsGcy`
- **项目**: huanchong-99
- **行号**: L2622
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2622min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 62. huanchong-99SoloDawnfrontend/src/components/dialogs/shared/RepoPickerDialog.tsx

> 该文件共有 **4** 个问题

### 62.1 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyZVcHZuNB-_5CPqJgG`
- **项目**: huanchong-99
- **行号**: L1965
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1965min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 62.2 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyZVcHZuNB-_5CPqJgH`
- **项目**: huanchong-99
- **行号**: L2215
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2215min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 62.3 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyZVcHZuNB-_5CPqJgI`
- **项目**: huanchong-99
- **行号**: L2855
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 2855min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 62.4 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices. ✅ 已修复

- **问题ID**: `AZyZVcHZuNB-_5CPqJgJ`
- **项目**: huanchong-99
- **行号**: L3515
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 3515min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

---

## 63. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/CreatePRDialog.tsx

> 该文件共有 **2** 个问题

### 63.1 A fragment with only one child is redundant.

- **问题ID**: `AZyVweSCZ9DOUQdEsGcG`
- **项目**: huanchong-99
- **行号**: L3345
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3345min effort
- **创建时间**: 1 month ago
- **标签**: react

### 63.2 Unexpected negated condition.

- **问题ID**: `AZyVweSCZ9DOUQdEsGcH`
- **项目**: huanchong-99
- **行号**: L3462
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3462min effort
- **创建时间**: 1 month ago
- **标签**: readability

---

## 64. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/PrCommentsDialog.tsx

> 该文件共有 **3** 个问题

### 64.1 Refactor this function to reduce its Cognitive Complexity from 17 to the 15 allowed.

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

### 64.2 Extract this nested ternary operation into an independent statement.

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

### 64.3 Extract this nested ternary operation into an independent statement.

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

## 65. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/RestoreLogsDialog.tsx

> 该文件共有 **2** 个问题

### 65.1 Refactor this function to reduce its Cognitive Complexity from 31 to the 15 allowed. ✅ 已修复

- **问题ID**: `AZyVweSYZ9DOUQdEsGcS`
- **项目**: huanchong-99
- **行号**: L8121
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 8121min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 65.2 Passing a fragment to an HTML element is useless.

- **问题ID**: `AZyVweSYZ9DOUQdEsGcT`
- **项目**: huanchong-99
- **行号**: L2355
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 2355min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 66. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/ShareDialog.tsx

> 该文件共有 **1** 个问题

### 66.1 Unexpected negated condition.

- **问题ID**: `AZyZVcF1uNB-_5CPqJgF`
- **项目**: huanchong-99
- **行号**: L1442
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1442min effort
- **创建时间**: 5 hours ago
- **标签**: readability

---

## 67. huanchong-99SoloDawnfrontend/src/components/dialogs/tasks/TaskFormDialog.tsx

> 该文件共有 **2** 个问题

### 67.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweSNZ9DOUQdEsGcQ`
- **项目**: huanchong-99
- **行号**: L7275
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 7275min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 67.2 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

- **问题ID**: `AZyVweSNZ9DOUQdEsGcR`
- **项目**: huanchong-99
- **行号**: L7275
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 7275min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 68. huanchong-99SoloDawnfrontend/src/components/layout/NewDesignLayout.tsx

> 该文件共有 **1** 个问题

### 68.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVwerqZ9DOUQdEsGkO`
- **项目**: huanchong-99
- **行号**: L1085
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1085min effort
- **创建时间**: 26 days ago
- **标签**: confusing

---

## 69. huanchong-99SoloDawnfrontend/src/components/panels/DiffsPanel.tsx

> 该文件共有 **1** 个问题

### 69.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweXjZ9DOUQdEsGeB`
- **项目**: huanchong-99
- **行号**: L2355
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2355min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 70. huanchong-99SoloDawnfrontend/src/components/panels/PreviewPanel.tsx

> 该文件共有 **1** 个问题

### 70.1 Ambiguous spacing after previous element a

- **问题ID**: `AZyVweXTZ9DOUQdEsGd4`
- **项目**: huanchong-99
- **行号**: L2615
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2615min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 71. huanchong-99SoloDawnfrontend/src/components/panels/TaskPanel.tsx

> 该文件共有 **3** 个问题

### 71.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

### 71.2 Unexpected negated condition.

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

### 71.3 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweXbZ9DOUQdEsGd8`
- **项目**: huanchong-99
- **行号**: L1365
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1365min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 72. huanchong-99SoloDawnfrontend/src/components/pipeline/TerminalNode.tsx

> 该文件共有 **2** 个问题

### 72.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweoGZ9DOUQdEsGjG`
- **项目**: huanchong-99
- **行号**: L535
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 535min effort
- **创建时间**: 22 days ago
- **标签**: confusing

### 72.2 Extract this nested ternary operation into an independent statement. ✅ 已修复

- **问题ID**: `AZyVweoGZ9DOUQdEsGjH`
- **项目**: huanchong-99
- **行号**: L545
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 545min effort
- **创建时间**: 22 days ago
- **标签**: confusing

---

## 73. huanchong-99SoloDawnfrontend/src/components/settings/ExecutorProfileSelector.tsx

> 该文件共有 **1** 个问题

### 73.1 Mark the props of the component as read-only. ✅ 已修复

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

## 74. huanchong-99SoloDawnfrontend/src/components/tasks/BranchSelector.tsx

> 该文件共有 **2** 个问题

### 74.1 Mark the props of the component as read-only. ✅ 已修复

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

### 74.2 Move this component definition out of the parent component and pass data as props.

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

## 75. huanchong-99SoloDawnfrontend/src/components/tasks/ConfigSelector.tsx

> 该文件共有 **1** 个问题

### 75.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

## 76. huanchong-99SoloDawnfrontend/src/components/tasks/RepoBranchSelector.tsx

> 该文件共有 **1** 个问题

### 76.1 Mark the props of the component as read-only. ✅ 已修复

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

## 77. huanchong-99SoloDawnfrontend/src/components/tasks/RepoSelector.tsx

> 该文件共有 **1** 个问题

### 77.1 Mark the props of the component as read-only. ✅ 已修复

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

## 78. huanchong-99SoloDawnfrontend/src/components/tasks/TaskFollowUpSection.tsx

> 该文件共有 **8** 个问题

### 78.1 'shared/types' imported multiple times.

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

### 78.2 '@/lib/api' imported multiple times.

- **问题ID**: `AZyVweaUZ9DOUQdEsGfJ`
- **项目**: huanchong-99
- **行号**: L561
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 561min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 78.3 'shared/types' imported multiple times.

- **问题ID**: `AZyVweaUZ9DOUQdEsGfK`
- **项目**: huanchong-99
- **行号**: L571
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 571min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 78.4 '@/lib/api' imported multiple times.

- **问题ID**: `AZyVweaUZ9DOUQdEsGfL`
- **项目**: huanchong-99
- **行号**: L581
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 581min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 78.5 'shared/types' imported multiple times.

- **问题ID**: `AZyVweaUZ9DOUQdEsGfM`
- **项目**: huanchong-99
- **行号**: L611
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 611min effort
- **创建时间**: 1 month ago
- **标签**: es2015

### 78.6 Refactor this function to reduce its Cognitive Complexity from 26 to the 15 allowed.

- **问题ID**: `AZyVweaUZ9DOUQdEsGfN`
- **项目**: huanchong-99
- **行号**: L10616
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 10616min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 78.7 Unexpected negated condition.

- **问题ID**: `AZyVweaUZ9DOUQdEsGfP`
- **项目**: huanchong-99
- **行号**: L6712
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6712min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 78.8 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweaUZ9DOUQdEsGfQ`
- **项目**: huanchong-99
- **行号**: L8415
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 8415min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 79. huanchong-99SoloDawnfrontend/src/components/tasks/Toolbar/GitOperations.tsx

> 该文件共有 **2** 个问题

### 79.1 Extract this nested ternary operation into an independent statement.

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

### 79.2 Extract this nested ternary operation into an independent statement.

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

## 80. huanchong-99SoloDawnfrontend/src/components/tasks/follow-up/FollowUpConflictSection.tsx

> 该文件共有 **1** 个问题

### 80.1 Mark the props of the component as read-only. ✅ 已修复

- **问题ID**: `AZyVweZpZ9DOUQdEsGe8`
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

## 81. huanchong-99SoloDawnfrontend/src/components/terminal/TerminalDebugView.tsx

> 该文件共有 **4** 个问题

### 81.1 Prefer `String#replaceAll()` over `String#replace()`.

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

### 81.2 Mark the props of the component as read-only.

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

### 81.3 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZyaDtVxyTAFHPcjycf9`
- **项目**: huanchong-99
- **行号**: L1335
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1335min effort
- **创建时间**: 1 hour ago
- **标签**: confusing

### 81.4 Unexpected negated condition.

- **问题ID**: `AZyVweqKZ9DOUQdEsGj1`
- **项目**: huanchong-99
- **行号**: L1952
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1952min effort
- **创建时间**: 23 days ago
- **标签**: readability

---

## 82. huanchong-99SoloDawnfrontend/src/components/terminal/TerminalEmulator.test.tsx

> 该文件共有 **1** 个问题

### 82.1 Make this public static property readonly.

- **问题ID**: `AZyVwep-Z9DOUQdEsGjm`
- **项目**: huanchong-99
- **行号**: L3520
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 3520min effort
- **创建时间**: 16 days ago
- **标签**: cwe

---

## 83. huanchong-99SoloDawnfrontend/src/components/ui-new/actions/index.ts

> 该文件共有 **1** 个问题

### 83.1 Unexpected negated condition.

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

## 84. huanchong-99SoloDawnfrontend/src/components/ui-new/dialogs/RebaseDialog.tsx

> 该文件共有 **1** 个问题

### 84.1 'msg' will use Object's default stringification format ('[object Object]') when stringified.

- **问题ID**: `AZyaJqrWbTTkCKzmc8Mq`
- **项目**: huanchong-99
- **行号**: L565
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 565min effort
- **创建时间**: 1 hour ago
- **标签**: object, string, ...

---

## 85. huanchong-99SoloDawnfrontend/src/components/ui-new/dialogs/WorkspacesGuideDialog.tsx

> 该文件共有 **2** 个问题

### 85.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

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

### 85.2 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

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

## 86. huanchong-99SoloDawnfrontend/src/components/ui-new/hooks/usePreviewUrl.ts

> 该文件共有 **3** 个问题

### 86.1 Simplify this regular expression to reduce its complexity from 22 to the 20 allowed.

- **问题ID**: `AZyZkrvL1b9TkhNHR2Lp`
- **项目**: huanchong-99
- **行号**: L1312
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1312min effort
- **创建时间**: 4 hours ago
- **标签**: regex, type-dependent

### 86.2 Compare with `undefined` directly instead of using `typeof`.

- **问题ID**: `AZyZVcXruNB-_5CPqJgh`
- **项目**: huanchong-99
- **行号**: L202
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 202min effort
- **创建时间**: 5 hours ago
- **标签**: readability, style

### 86.3 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwempZ9DOUQdEsGiq`
- **项目**: huanchong-99
- **行号**: L611
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 611min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 87. huanchong-99SoloDawnfrontend/src/components/ui-new/views/FileTreeNode.tsx

> 该文件共有 **2** 个问题

### 87.1 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

- **问题ID**: `AZyZVcUDuNB-_5CPqJgY`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 635min effort
- **创建时间**: 5 hours ago
- **标签**: accessibility, react

### 87.2 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZyVweheZ9DOUQdEsGhO`
- **项目**: huanchong-99
- **行号**: L1145
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1145min effort
- **创建时间**: 1 month ago
- **标签**: confusing

---

## 88. huanchong-99SoloDawnfrontend/src/components/ui-new/views/PreviewBrowser.tsx

> 该文件共有 **5** 个问题

### 88.1 Mark the props of the component as read-only.

- **问题ID**: `AZyZVcUcuNB-_5CPqJgZ`
- **项目**: huanchong-99
- **行号**: L1005
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1005min effort
- **创建时间**: 5 hours ago
- **标签**: react, type-dependent

### 88.2 Refactor this function to reduce its Cognitive Complexity from 20 to the 15 allowed.

- **问题ID**: `AZyVweiAZ9DOUQdEsGhS`
- **项目**: huanchong-99
- **行号**: L18210
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 18210min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 88.3 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweiAZ9DOUQdEsGhW`
- **项目**: huanchong-99
- **行号**: L4365
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4365min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 88.4 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweiAZ9DOUQdEsGhX`
- **项目**: huanchong-99
- **行号**: L4425
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4425min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 88.5 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweiAZ9DOUQdEsGhY`
- **项目**: huanchong-99
- **行号**: L4485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4485min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 89. huanchong-99SoloDawnfrontend/src/components/ui-new/views/PreviewControls.tsx

> 该文件共有 **1** 个问题

### 89.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

## 90. huanchong-99SoloDawnfrontend/src/components/ui-new/views/WorkspacesMain.tsx

> 该文件共有 **2** 个问题

### 90.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

### 90.2 Unexpected negated condition.

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

## 91. huanchong-99SoloDawnfrontend/src/components/ui/breadcrumb.tsx

> 该文件共有 **1** 个问题

### 91.1 Comments inside children section of tag should be placed inside braces

- **问题ID**: `AZyVwePhZ9DOUQdEsGbS`
- **项目**: huanchong-99
- **行号**: L851
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 851min effort
- **创建时间**: 1 month ago
- **标签**: react

---

## 92. huanchong-99SoloDawnfrontend/src/components/ui/dialog.tsx

> 该文件共有 **3** 个问题

### 92.1 This assertion is unnecessary since it does not change the type of the expression.

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

### 92.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

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

### 92.3 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

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

---

## 93. huanchong-99SoloDawnfrontend/src/components/ui/json-editor.tsx

> 该文件共有 **1** 个问题

### 93.1 Compare with `undefined` directly instead of using `typeof`.

- **问题ID**: `AZyZVcEtuNB-_5CPqJgD`
- **项目**: huanchong-99
- **行号**: L432
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 432min effort
- **创建时间**: 5 hours ago
- **标签**: readability, style

---

## 94. huanchong-99SoloDawnfrontend/src/components/ui/multi-file-search-textarea.tsx

> 该文件共有 **2** 个问题

### 94.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

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

### 94.2 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

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

## 95. huanchong-99SoloDawnfrontend/src/components/ui/pr-comment-card.tsx

> 该文件共有 **1** 个问题

### 95.1 Mark the props of the component as read-only.

- **问题ID**: `AZyVweO7Z9DOUQdEsGbH`
- **项目**: huanchong-99
- **行号**: L1835
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1835min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 96. huanchong-99SoloDawnfrontend/src/components/ui/table/data-table.tsx

> 该文件共有 **2** 个问题

### 96.1 Mark the props of the component as read-only.

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

### 96.2 Extract this nested ternary operation into an independent statement.

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

## 97. huanchong-99SoloDawnfrontend/src/components/ui/wysiwyg.tsx

> 该文件共有 **3** 个问题

### 97.1 Remove this redundant type alias and replace its occurrences with "string".

- **问题ID**: `AZyVweQRZ9DOUQdEsGbd`
- **项目**: huanchong-99
- **行号**: L485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 485min effort
- **创建时间**: 1 month ago

### 97.2 `String.raw` should be used to avoid escaping `\`.

- **问题ID**: `AZyZVcFFuNB-_5CPqJgE`
- **项目**: huanchong-99
- **行号**: L1105
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1105min effort
- **创建时间**: 5 hours ago
- **标签**: readability

### 97.3 The array passed as the value prop to the Context provider changes every render. To fix this consider wrapping it in a useMemo hook.

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

## 98. huanchong-99SoloDawnfrontend/src/components/ui/wysiwyg/lib/create-decorator-node.tsx

> 该文件共有 **1** 个问题

### 98.1 Mark the props of the component as read-only.

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

## 99. huanchong-99SoloDawnfrontend/src/components/wizard/WorkflowConfigureStep.tsx

> 该文件共有 **1** 个问题

### 99.1 Mark the props of the component as read-only.

- **问题ID**: `AZyVweNHZ9DOUQdEsGac`
- **项目**: huanchong-99
- **行号**: L55
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 55min effort
- **创建时间**: 28 days ago
- **标签**: react, type-dependent

---

## 100. huanchong-99SoloDawnfrontend/src/components/wizard/WorkflowWizard.tsx

> 该文件共有 **1** 个问题

### 100.1 Mark the props of the component as read-only.

- **问题ID**: `AZyVweM_Z9DOUQdEsGab`
- **项目**: huanchong-99
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 155min effort
- **创建时间**: 28 days ago
- **标签**: react, type-dependent

---

## 101. huanchong-99SoloDawnfrontend/src/components/workflow/PipelineView.tsx

> 该文件共有 **1** 个问题

### 101.1 Mark the props of the component as read-only.

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

## 102. huanchong-99SoloDawnfrontend/src/components/workflow/StepIndicator.tsx

> 该文件共有 **1** 个问题

### 102.1 Mark the props of the component as read-only.

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

## 103. huanchong-99SoloDawnfrontend/src/components/workflow/TerminalCard.tsx

> 该文件共有 **1** 个问题

### 103.1 Mark the props of the component as read-only.

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

---

## 104. huanchong-99SoloDawnfrontend/src/components/workflow/WorkflowPromptDialog.tsx

> 该文件共有 **1** 个问题

### 104.1 Mark the props of the component as read-only.

- **问题ID**: `AZyVweWRZ9DOUQdEsGdl`
- **项目**: huanchong-99
- **行号**: L995
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 995min effort
- **创建时间**: 18 days ago
- **标签**: react, type-dependent

---

## 105. huanchong-99SoloDawnfrontend/src/components/workflow/WorkflowWizard.tsx

> 该文件共有 **1** 个问题

### 105.1 Mark the props of the component as read-only.

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

---

## 106. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step0Project.tsx

> 该文件共有 **1** 个问题

### 106.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

## 107. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step2Tasks.tsx

> 该文件共有 **1** 个问题

### 107.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

## 108. huanchong-99SoloDawnfrontend/src/components/workflow/steps/Step4Terminals.test.tsx

> 该文件共有 **1** 个问题

### 108.1 `String.raw` should be used to avoid escaping `\`.

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

## 109. huanchong-99SoloDawnfrontend/src/contexts/ActionsContext.tsx

> 该文件共有 **1** 个问题

### 109.1 Mark the props of the component as read-only.

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

## 110. huanchong-99SoloDawnfrontend/src/contexts/ApprovalFeedbackContext.tsx

> 该文件共有 **1** 个问题

### 110.1 Mark the props of the component as read-only.

- **问题ID**: `AZyVwezzZ9DOUQdEsGnF`
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

## 111. huanchong-99SoloDawnfrontend/src/contexts/ChangesViewContext.tsx

> 该文件共有 **1** 个问题

### 111.1 Mark the props of the component as read-only.

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

## 112. huanchong-99SoloDawnfrontend/src/contexts/ClickedElementsProvider.tsx

> 该文件共有 **6** 个问题

### 112.1 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc25kp-e-LWbDubNi`
- **项目**: huanchong-99
- **行号**: L955
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 955min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

### 112.2 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc25kp-e-LWbDubNj`
- **项目**: huanchong-99
- **行号**: L995
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 995min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

### 112.3 Prefer negative index over length minus index for `slice`.

- **问题ID**: `AZyVwezbZ9DOUQdEsGm3`
- **项目**: huanchong-99
- **行号**: L1045
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1045min effort
- **创建时间**: 1 month ago
- **标签**: performance, readability

### 112.4 Unexpected negated condition.

- **问题ID**: `AZyVwezbZ9DOUQdEsGm5`
- **项目**: huanchong-99
- **行号**: L1302
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1302min effort
- **创建时间**: 1 month ago
- **标签**: readability

### 112.5 Refactor this code to not use nested template literals.

- **问题ID**: `AZyVwezbZ9DOUQdEsGm_`
- **项目**: huanchong-99
- **行号**: L35010
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 35010min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload, confusing

### 112.6 Mark the props of the component as read-only.

- **问题ID**: `AZyVwezbZ9DOUQdEsGnA`
- **项目**: huanchong-99
- **行号**: L3595
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 3595min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 113. huanchong-99SoloDawnfrontend/src/contexts/CreateModeContext.tsx

> 该文件共有 **1** 个问题

### 113.1 Mark the props of the component as read-only.

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

## 114. huanchong-99SoloDawnfrontend/src/contexts/EntriesContext.tsx

> 该文件共有 **1** 个问题

### 114.1 useState call is not destructured into value + setter pair

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

## 115. huanchong-99SoloDawnfrontend/src/contexts/ExecutionProcessesContext.tsx

> 该文件共有 **1** 个问题

### 115.1 Consider removing 'undefined' type or '?' specifier, one of them is redundant.

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

## 116. huanchong-99SoloDawnfrontend/src/contexts/LogsPanelContext.tsx

> 该文件共有 **1** 个问题

### 116.1 Mark the props of the component as read-only.

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

---

## 117. huanchong-99SoloDawnfrontend/src/contexts/MessageEditContext.tsx

> 该文件共有 **1** 个问题

### 117.1 Mark the props of the component as read-only.

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

## 118. huanchong-99SoloDawnfrontend/src/contexts/ProcessSelectionContext.tsx

> 该文件共有 **1** 个问题

### 118.1 Mark the props of the component as read-only.

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

## 119. huanchong-99SoloDawnfrontend/src/contexts/ProjectContext.tsx

> 该文件共有 **1** 个问题

### 119.1 Mark the props of the component as read-only.

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

## 120. huanchong-99SoloDawnfrontend/src/contexts/RetryUiContext.tsx

> 该文件共有 **1** 个问题

### 120.1 Mark the props of the component as read-only.

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

---

## 121. huanchong-99SoloDawnfrontend/src/contexts/ReviewProvider.tsx

> 该文件共有 **1** 个问题

### 121.1 Mark the props of the component as read-only.

- **问题ID**: `AZyVwezSZ9DOUQdEsGmw`
- **项目**: huanchong-99
- **行号**: L595
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 595min effort
- **创建时间**: 1 month ago
- **标签**: react, type-dependent

---

## 122. huanchong-99SoloDawnfrontend/src/contexts/SearchContext.tsx

> 该文件共有 **1** 个问题

### 122.1 Mark the props of the component as read-only.

- **问题ID**: `AZyVwe0JZ9DOUQdEsGnK`
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

## 123. huanchong-99SoloDawnfrontend/src/contexts/WorkspaceContext.tsx

> 该文件共有 **1** 个问题

### 123.1 Mark the props of the component as read-only.

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

---

## 124. huanchong-99SoloDawnfrontend/src/hooks/useCommandBarShortcut.ts

> 该文件共有 **1** 个问题

### 124.1 'platform' is deprecated.

- **问题ID**: `AZyZ21zSdO4WpiZegSzb`
- **项目**: huanchong-99
- **行号**: L1515
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1515min effort
- **创建时间**: 2 hours ago
- **标签**: cwe, obsolete, ...

---

## 125. huanchong-99SoloDawnfrontend/src/hooks/useContextBarPosition.ts

> 该文件共有 **1** 个问题

### 125.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

## 126. huanchong-99SoloDawnfrontend/src/hooks/useConversationHistory.ts

> 该文件共有 **5** 个问题

### 126.1 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZyVwewpZ9DOUQdEsGl7`
- **项目**: huanchong-99
- **行号**: L1605
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1605min effort
- **创建时间**: 1 month ago
- **标签**: confusing

### 126.2 Using `join()` for entries.map((line) => line.content) may use Object's default stringification format ('[object Object]') when stringified.

- **问题ID**: `AZyZVchPuNB-_5CPqJgq`
- **项目**: huanchong-99
- **行号**: L1645
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1645min effort
- **创建时间**: 5 hours ago
- **标签**: object, string, ...

### 126.3 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwewpZ9DOUQdEsGl5`
- **项目**: huanchong-99
- **行号**: L2531
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2531min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 126.4 Refactor this code to not nest functions more than 4 levels deep.

- **问题ID**: `AZyVwewpZ9DOUQdEsGl9`
- **项目**: huanchong-99
- **行号**: L53620
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 53620min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

### 126.5 Refactor this code to not nest functions more than 4 levels deep.

- **问题ID**: `AZyVwewpZ9DOUQdEsGl-`
- **项目**: huanchong-99
- **行号**: L53920
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 53920min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

---

## 127. huanchong-99SoloDawnfrontend/src/hooks/useDevserverPreview.ts

> 该文件共有 **1** 个问题

### 127.1 Do not use an object literal as default for parameter `options`.

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

## 128. huanchong-99SoloDawnfrontend/src/hooks/useDevserverUrl.ts

> 该文件共有 **3** 个问题

### 128.1 Simplify this regular expression to reduce its complexity from 22 to the 20 allowed.

- **问题ID**: `AZyZkr6a1b9TkhNHR2Lq`
- **项目**: huanchong-99
- **行号**: L712
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 712min effort
- **创建时间**: 4 hours ago
- **标签**: regex, type-dependent

### 128.2 Compare with `undefined` directly instead of using `typeof`.

- **问题ID**: `AZyZVcgAuNB-_5CPqJgp`
- **项目**: huanchong-99
- **行号**: L202
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 202min effort
- **创建时间**: 5 hours ago
- **标签**: readability, style

### 128.3 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwewEZ9DOUQdEsGlu`
- **项目**: huanchong-99
- **行号**: L581
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 581min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

---

## 129. huanchong-99SoloDawnfrontend/src/hooks/useLogStream.ts

> 该文件共有 **2** 个问题

### 129.1 Prefer using an optional chain expression instead, as it's more concise and easier to read.

- **问题ID**: `AZyVwevTZ9DOUQdEsGlb`
- **项目**: huanchong-99
- **行号**: L575
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 575min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 129.2 Refactor this code to not nest functions more than 4 levels deep.

- **问题ID**: `AZyVwevTZ9DOUQdEsGlc`
- **项目**: huanchong-99
- **行号**: L12220
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 12220min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

---

## 130. huanchong-99SoloDawnfrontend/src/hooks/useMediaQuery.ts

> 该文件共有 **3** 个问题

### 130.1 Unexpected negated condition.

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

### 130.2 The signature '(callback: ((this: MediaQueryList, ev: MediaQueryListEvent) => any) | null): void' of 'mql.addListener' is deprecated.

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

### 130.3 The signature '(callback: ((this: MediaQueryList, ev: MediaQueryListEvent) => any) | null): void' of 'mql.removeListener' is deprecated.

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

## 131. huanchong-99SoloDawnfrontend/src/hooks/useNavigateWithSearch.ts

> 该文件共有 **1** 个问题

### 131.1 Extract this nested ternary operation into an independent statement.

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

## 132. huanchong-99SoloDawnfrontend/src/hooks/usePreviewSettings.ts

> 该文件共有 **1** 个问题

### 132.1 This assertion is unnecessary since it does not change the type of the expression.

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

## 133. huanchong-99SoloDawnfrontend/src/hooks/usePreviousPath.ts

> 该文件共有 **1** 个问题

### 133.1 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc20Wp-e-LWbDubNc`
- **项目**: huanchong-99
- **行号**: L645
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 645min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

---

## 134. huanchong-99SoloDawnfrontend/src/hooks/useProjectTasks.ts

> 该文件共有 **8** 个问题

### 134.1 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwewWZ9DOUQdEsGlw`
- **项目**: huanchong-99
- **行号**: L1481
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1481min effort
- **创建时间**: 27 days ago
- **标签**: redundant, type-dependent

### 134.2 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwewWZ9DOUQdEsGlx`
- **项目**: huanchong-99
- **行号**: L1491
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1491min effort
- **创建时间**: 27 days ago
- **标签**: redundant, type-dependent

### 134.3 This assertion is unnecessary since it does not change the type of the expression.

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

### 134.4 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwewWZ9DOUQdEsGlz`
- **项目**: huanchong-99
- **行号**: L1551
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1551min effort
- **创建时间**: 27 days ago
- **标签**: redundant, type-dependent

### 134.5 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwewWZ9DOUQdEsGl0`
- **项目**: huanchong-99
- **行号**: L1561
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1561min effort
- **创建时间**: 27 days ago
- **标签**: redundant, type-dependent

### 134.6 This assertion is unnecessary since it does not change the type of the expression.

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

### 134.7 This assertion is unnecessary since it does not change the type of the expression.

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

### 134.8 This assertion is unnecessary since it does not change the type of the expression.

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

## 135. huanchong-99SoloDawnfrontend/src/hooks/useRebase.ts

> 该文件共有 **1** 个问题

### 135.1 Prefer `throw error` over `return Promise.reject(error)`.

- **问题ID**: `AZyZn0LRgmDo_BFy_eYW`
- **项目**: huanchong-99
- **行号**: L405
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 405min effort
- **创建时间**: 3 hours ago
- **标签**: async, confusing, ...

---

## 136. huanchong-99SoloDawnfrontend/src/hooks/useTodos.ts

> 该文件共有 **4** 个问题

### 136.1 Complete the task associated to this "TODO" comment.

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

### 136.2 Complete the task associated to this "TODO" comment.

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

### 136.3 This assertion is unnecessary since it does not change the type of the expression.

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

### 136.4 Complete the task associated to this "TODO" comment.

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

## 137. huanchong-99SoloDawnfrontend/src/hooks/useVariant.ts

> 该文件共有 **3** 个问题

### 137.1 Unexpected negated condition.

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

### 137.2 useState call is not destructured into value + setter pair

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

### 137.3 Unexpected negated condition.

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

## 138. huanchong-99SoloDawnfrontend/src/hooks/useWorkflows.test.tsx

> 该文件共有 **1** 个问题

### 138.1 Remove this useless assignment to variable "url".

- **问题ID**: `AZyZmLS0sTBU_SG-64wF`
- **项目**: huanchong-99
- **行号**: L2811
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2811min effort
- **创建时间**: 3 hours ago
- **标签**: cwe, unused

---

## 139. huanchong-99SoloDawnfrontend/src/i18n/languages.ts

> 该文件共有 **1** 个问题

### 139.1 This assertion is unnecessary since it does not change the type of the expression.

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

## 140. huanchong-99SoloDawnfrontend/src/keyboard/useSemanticKey.ts

> 该文件共有 **2** 个问题

### 140.1 Prefer using nullish coalescing operator (`??`) instead of a ternary expression, as it is simpler to read.

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

### 140.2 Unexpected negated condition.

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

## 141. huanchong-99SoloDawnfrontend/src/lib/devServerUtils.ts

> 该文件共有 **2** 个问题

### 141.1 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVweyUZ9DOUQdEsGmh`
- **项目**: huanchong-99
- **行号**: L291
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 291min effort
- **创建时间**: 27 days ago
- **标签**: redundant, type-dependent

### 141.2 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVweyUZ9DOUQdEsGmi`
- **项目**: huanchong-99
- **行号**: L291
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 291min effort
- **创建时间**: 27 days ago
- **标签**: redundant, type-dependent

---

## 142. huanchong-99SoloDawnfrontend/src/lib/mcpStrategies.ts

> 该文件共有 **4** 个问题

### 142.1 This assertion is unnecessary since it does not change the type of the expression.

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

### 142.2 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc24kp-e-LWbDubNg`
- **项目**: huanchong-99
- **行号**: L255
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 255min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

### 142.3 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVweycZ9DOUQdEsGmm`
- **项目**: huanchong-99
- **行号**: L931
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 931min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 142.4 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyZc24kp-e-LWbDubNh`
- **项目**: huanchong-99
- **行号**: L1045
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1045min effort
- **创建时间**: 4 hours ago
- **标签**: es2022, performance, ...

---

## 143. huanchong-99SoloDawnfrontend/src/main.tsx

> 该文件共有 **1** 个问题

### 143.1 Don't use a zero fraction in the number.

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

## 144. huanchong-99SoloDawnfrontend/src/pages/SlashCommands.e2e.test.tsx

> 该文件共有 **1** 个问题

### 144.1 Remove this useless assignment to variable "user".

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

## 145. huanchong-99SoloDawnfrontend/src/pages/SlashCommands.tsx

> 该文件共有 **1** 个问题

### 145.1 Mark the props of the component as read-only.

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

---

## 146. huanchong-99SoloDawnfrontend/src/pages/WorkflowDebug.tsx

> 该文件共有 **1** 个问题

### 146.1 Move function 'mapTerminalStatus' to the outer scope.

- **问题ID**: `AZyVweu8Z9DOUQdEsGlV`
- **项目**: huanchong-99
- **行号**: L1505
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1505min effort
- **创建时间**: 1 month ago
- **标签**: javascript, optimization, ...

---

## 147. huanchong-99SoloDawnfrontend/src/pages/Workflows.test.tsx

> 该文件共有 **1** 个问题

### 147.1 'init.body' may use Object's default stringification format ('[object Object]') when stringified.

- **问题ID**: `AZyVweuzZ9DOUQdEsGlQ`
- **项目**: huanchong-99
- **行号**: L12245
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 12245min effort
- **创建时间**: 17 days ago
- **标签**: object, string, ...

---

## 148. huanchong-99SoloDawnfrontend/src/pages/Workflows.tsx

> 该文件共有 **2** 个问题

### 148.1 Refactor this function to reduce its Cognitive Complexity from 23 to the 15 allowed.

- **问题ID**: `AZyVwetGZ9DOUQdEsGkd`
- **项目**: huanchong-99
- **行号**: L11513
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 11513min effort
- **创建时间**: 17 days ago
- **标签**: brain-overload

### 148.2 Prefer `.find(…)` over `.filter(…)`.

- **问题ID**: `AZyVwetGZ9DOUQdEsGki`
- **项目**: huanchong-99
- **行号**: L5885
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 5885min effort
- **创建时间**: 17 days ago
- **标签**: performance, readability

---

## 149. huanchong-99SoloDawnfrontend/src/pages/settings/AgentSettings.tsx

> 该文件共有 **4** 个问题

### 149.1 Prefer using an optional chain expression instead, as it's more concise and easier to read.

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

### 149.2 Prefer using an optional chain expression instead, as it's more concise and easier to read.

- **问题ID**: `AZyVwetlZ9DOUQdEsGks`
- **项目**: huanchong-99
- **行号**: L3545
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3545min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 149.3 Prefer using an optional chain expression instead, as it's more concise and easier to read.

- **问题ID**: `AZyVwetlZ9DOUQdEsGkt`
- **项目**: huanchong-99
- **行号**: L3765
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3765min effort
- **创建时间**: 1 month ago
- **标签**: type-dependent

### 149.4 'profilesError' will use Object's default stringification format ('[object Object]') when stringified.

- **问题ID**: `AZyVwetlZ9DOUQdEsGku`
- **项目**: huanchong-99
- **行号**: L4345
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4345min effort
- **创建时间**: 1 month ago
- **标签**: object, string, ...

---

## 150. huanchong-99SoloDawnfrontend/src/pages/settings/GeneralSettings.tsx

> 该文件共有 **1** 个问题

### 150.1 Prefer `String#codePointAt()` over `String#charCodeAt()`.

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

---

## 151. huanchong-99SoloDawnfrontend/src/pages/settings/McpSettings.tsx

> 该文件共有 **1** 个问题

### 151.1 This assertion is unnecessary since the receiver accepts the original type of the expression.

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

## 152. huanchong-99SoloDawnfrontend/src/pages/settings/OrganizationSettings.tsx

> 该文件共有 **4** 个问题

### 152.1 Mark the props of the component as read-only.

- **问题ID**: `AZyZVccGuNB-_5CPqJgl`
- **项目**: huanchong-99
- **行号**: L495
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 495min effort
- **创建时间**: 5 hours ago
- **标签**: react, type-dependent

### 152.2 Mark the props of the component as read-only.

- **问题ID**: `AZyZVccGuNB-_5CPqJgm`
- **项目**: huanchong-99
- **行号**: L945
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 945min effort
- **创建时间**: 5 hours ago
- **标签**: react, type-dependent

### 152.3 Mark the props of the component as read-only.

- **问题ID**: `AZyZVccGuNB-_5CPqJgn`
- **项目**: huanchong-99
- **行号**: L1515
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1515min effort
- **创建时间**: 5 hours ago
- **标签**: react, type-dependent

### 152.4 Promise-returning function provided to property where a void return was expected.

- **问题ID**: `AZyVwet5Z9DOUQdEsGky`
- **项目**: huanchong-99
- **行号**: L3255
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 3255min effort
- **创建时间**: 1 month ago
- **标签**: async, promise, ...

---

## 153. huanchong-99SoloDawnfrontend/src/pages/settings/ProjectSettings.tsx

> 该文件共有 **2** 个问题

### 153.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweuDZ9DOUQdEsGk_`
- **项目**: huanchong-99
- **行号**: L5055
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 5055min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

### 153.2 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

- **问题ID**: `AZyVweuDZ9DOUQdEsGlA`
- **项目**: huanchong-99
- **行号**: L5055
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 5055min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 154. huanchong-99SoloDawnfrontend/src/stores/__tests__/wsStore.test.ts

> 该文件共有 **1** 个问题

### 154.1 Type literal has only a call signature, you should use a function type instead.

- **问题ID**: `AZyVwe2uZ9DOUQdEsGoj`
- **项目**: huanchong-99
- **行号**: L635
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 635min effort
- **创建时间**: 18 days ago
- **标签**: function, type

---

## 155. huanchong-99SoloDawnfrontend/src/stores/useUiPreferencesStore.ts

> 该文件共有 **1** 个问题

### 155.1 Extract this nested ternary operation into an independent statement. ✅ 已修复

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

## 156. huanchong-99SoloDawnfrontend/src/stores/wsStore.ts

> 该文件共有 **8** 个问题

### 156.1 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoP`
- **项目**: huanchong-99
- **行号**: L4605
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4605min effort
- **创建时间**: 18 days ago
- **标签**: confusing

### 156.2 `statuses` should be a `Set`, and use `statuses.has()` to check existence or non-existence.

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoS`
- **项目**: huanchong-99
- **行号**: L6195
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6195min effort
- **创建时间**: 18 days ago
- **标签**: optimization, performance

### 156.3 Prefer using an optional chain expression instead, as it's more concise and easier to read.

- **问题ID**: `AZyZg5dw331ZRqy8UBd3`
- **项目**: huanchong-99
- **行号**: L7895
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 7895min effort
- **创建时间**: 4 hours ago
- **标签**: type-dependent

### 156.4 Prefer using an optional chain expression instead, as it's more concise and easier to read.

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoU`
- **项目**: huanchong-99
- **行号**: L8775
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 8775min effort
- **创建时间**: 18 days ago
- **标签**: type-dependent

### 156.5 Prefer using an optional chain expression instead, as it's more concise and easier to read.

- **问题ID**: `AZyZxQj24NBSmYbRRYRt`
- **项目**: huanchong-99
- **行号**: L9695
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 9695min effort
- **创建时间**: 3 hours ago
- **标签**: type-dependent

### 156.6 Prefer using an optional chain expression instead, as it's more concise and easier to read.

- **问题ID**: `AZyVwe2OZ9DOUQdEsGoZ`
- **项目**: huanchong-99
- **行号**: L10125
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 10125min effort
- **创建时间**: 18 days ago
- **标签**: type-dependent

### 156.7 Prefer using an optional chain expression instead, as it's more concise and easier to read.

- **问题ID**: `AZyZxQj24NBSmYbRRYRu`
- **项目**: huanchong-99
- **行号**: L10495
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 10495min effort
- **创建时间**: 3 hours ago
- **标签**: type-dependent

### 156.8 Prefer using an optional chain expression instead, as it's more concise and easier to read.

- **问题ID**: `AZyZg5dw331ZRqy8UBd4`
- **项目**: huanchong-99
- **行号**: L12405
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 12405min effort
- **创建时间**: 4 hours ago
- **标签**: type-dependent

---

## 157. huanchong-99SoloDawnfrontend/src/utils/StyleOverride.tsx

> 该文件共有 **1** 个问题

### 157.1 Mark the props of the component as read-only.

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

---

## 158. huanchong-99SoloDawnfrontend/src/utils/TruncatePath.tsx

> 该文件共有 **1** 个问题

### 158.1 Move this array "reverse" operation to a separate statement or replace it with "toReversed".

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

---

## 159. huanchong-99SoloDawnfrontend/src/utils/fileTreeUtils.ts

> 该文件共有 **1** 个问题

### 159.1 Refactor this function to reduce its Cognitive Complexity from 16 to the 15 allowed.

- **问题ID**: `AZyVwe1fZ9DOUQdEsGnf`
- **项目**: huanchong-99
- **行号**: L706
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 706min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

---

## 160. huanchong-99SoloDawnfrontend/src/utils/previewBridge.ts

> 该文件共有 **1** 个问题

### 160.1 Prefer using an optional chain expression instead, as it's more concise and easier to read.

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

---

## 161. huanchong-99SoloDawnfrontend/src/utils/string.ts

> 该文件共有 **1** 个问题

### 161.1 Prefer `.findLast(…)` over `.filter(…).pop()`.

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

---

## 162. huanchong-99SoloDawnfrontend/src/vscode/ContextMenu.tsx

> 该文件共有 **4** 个问题

### 162.1 Review this redundant assignment: "cut" already holds the assigned value along all execution paths.

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

### 162.2 Review this redundant assignment: "paste" already holds the assigned value along all execution paths.

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

### 162.3 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoE`
- **项目**: huanchong-99
- **行号**: L2061
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2061min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 162.4 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVwe2AZ9DOUQdEsGoJ`
- **项目**: huanchong-99
- **行号**: L2515
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2515min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

---

## 163. huanchong-99SoloDawnfrontend/src/vscode/bridge.ts

> 该文件共有 **10** 个问题

### 163.1 'platform' is deprecated.

- **问题ID**: `AZyZ21-kdO4WpiZegSzc`
- **项目**: huanchong-99
- **行号**: L5315
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 5315min effort
- **创建时间**: 2 hours ago
- **标签**: cwe, obsolete, ...

### 163.2 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyZksEs1b9TkhNHR2Lr`
- **项目**: huanchong-99
- **行号**: L1331
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1331min effort
- **创建时间**: 4 hours ago
- **标签**: redundant, type-dependent

### 163.3 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyZksEs1b9TkhNHR2Ls`
- **项目**: huanchong-99
- **行号**: L2091
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2091min effort
- **创建时间**: 4 hours ago
- **标签**: redundant, type-dependent

### 163.4 This assertion is unnecessary since it does not change the type of the expression.

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

### 163.5 Replace this union type with a type alias.

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

### 163.6 Unexpected negated condition.

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

### 163.7 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwe14Z9DOUQdEsGnr`
- **项目**: huanchong-99
- **行号**: L2471
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2471min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 163.8 Verify the origin of the received message.

- **问题ID**: `AZyZVcoWuNB-_5CPqJgu`
- **项目**: huanchong-99
- **行号**: L31410
- **类型**: Vulnerability
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Security
- **工作量**: 31410min effort
- **创建时间**: 5 hours ago
- **标签**: cwe, html5, ...

### 163.9 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyVwe14Z9DOUQdEsGn0`
- **项目**: huanchong-99
- **行号**: L3281
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 3281min effort
- **创建时间**: 1 month ago
- **标签**: redundant, type-dependent

### 163.10 This assertion is unnecessary since it does not change the type of the expression.

- **问题ID**: `AZyZksEs1b9TkhNHR2Lt`
- **项目**: huanchong-99
- **行号**: L4311
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4311min effort
- **创建时间**: 4 hours ago
- **标签**: redundant, type-dependent

---

## 164. huanchong-99SoloDawnscripts/check-i18n.sh ✅ 已修复

> 该文件共有 **3** 个问题

### 164.1 Add an explicit return statement at the end of the function. ✅ 已修复

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

### 164.2 Define a constant instead of using the literal '   - %s\n' 7 times. ✅ 已修复

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

### 164.3 Add an explicit return statement at the end of the function. ✅ 已修复

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

## 165. huanchong-99SoloDawnscripts/docker/e2e-smoke.sh ✅ 已修复

> 该文件共有 **2** 个问题

### 165.1 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe6yZ9DOUQdEsGps`
- **项目**: huanchong-99
- **行号**: L542
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 542min effort
- **创建时间**: 2 days ago
- **标签**: bash, best-practices, ...

### 165.2 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich. ✅ 已修复

- **问题ID**: `AZyVwe6yZ9DOUQdEsGpt`
- **项目**: huanchong-99
- **行号**: L622
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 622min effort
- **创建时间**: 2 days ago
- **标签**: bash, best-practices, ...

---

## 166. huanchong-99SoloDawnscripts/run-dev.js ✅ 已修复

> 该文件共有 **1** 个问题

### 166.1 Prefer top-level await over an async function `main` call. ✅ 已修复

- **问题ID**: `AZyVwe7rZ9DOUQdEsGqe`
- **项目**: huanchong-99
- **行号**: L4665
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4665min effort
- **创建时间**: 28 days ago
- **标签**: async, es2022, ...

---

## 167. huanchong-99SoloDawnscripts/setup-dev-environment.js ✅ 已修复

> 该文件共有 **1** 个问题

### 167.1 Prefer top-level await over an async IIFE. ✅ 已修复

- **问题ID**: `AZyZVcxvuNB-_5CPqJgv`
- **项目**: huanchong-99
- **行号**: L945
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 945min effort
- **创建时间**: 5 hours ago
- **标签**: async, es2022, ...

---

