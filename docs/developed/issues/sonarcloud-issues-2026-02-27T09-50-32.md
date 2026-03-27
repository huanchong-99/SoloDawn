# SonarCloud Issues 报告

**生成时间**: 2026/02/27 17:50
**问题总数**: 92
**已加载**: 92
**收集数量**: 92

---

## 统计信息

### 按严重程度分类

- **Major**: 38 个
- **Critical**: 32 个
- **Minor**: 12 个
- **Info**: 6 个
- **Blocker**: 4 个

### 按类型分类

- **Code Smell**: 73 个
- **Bug**: 18 个
- **Vulnerability**: 1 个

### 按影响分类

- **Maintainability**: 67 个
- **Reliability**: 24 个
- **Security**: 1 个

### 按属性分类

- **Intentionality**: 41 个
- **Adaptability**: 33 个
- **Consistency**: 18 个

### 按文件统计 (Top 20)

- **huanchong-99SoloDawncrates/db/migrations/20250730000000_add_executor_action_to_execution_processes.sql**: 7 个问题
- **huanchong-99SoloDawnfrontend/src/pages/Workflows.tsx**: 7 个问题
- **huanchong-99SoloDawncrates/db/migrations/20251209000000_add_project_repositories.sql**: 6 个问题
- **huanchong-99SoloDawnfrontend/.../components/ui-new/containers/NewDisplayConversationEntry.tsx**: 5 个问题
- **huanchong-99SoloDawncrates/db/migrations/20260119000001_add_performance_indexes.sql**: 4 个问题
- **huanchong-99SoloDawncrates/db/migrations/20250819000000_move_merge_commit_to_merges_table.sql**: 3 个问题
- **huanchong-99SoloDawncrates/db/migrations/20260117000001_create_workflow_tables.sql**: 3 个问题
- **huanchong-99SoloDawnfrontend/.../NormalizedConversation/DisplayConversationEntry.tsx**: 3 个问题
- **huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatFileEntry.tsx**: 3 个问题
- **huanchong-99SoloDawnfrontend/src/components/ui-new/views/PreviewBrowser.tsx**: 3 个问题
- **huanchong-99SoloDawnfrontend/src/hooks/useConversationHistory.ts**: 3 个问题
- **huanchong-99SoloDawnfrontend/src/hooks/useTodos.ts**: 3 个问题
- **huanchong-99SoloDawnfrontend/src/vscode/bridge.ts**: 3 个问题
- **huanchong-99SoloDawncrates/db/migrations/20251020120000_convert_templates_to_tags.sql**: 2 个问题
- **huanchong-99SoloDawncrates/db/migrations/20260107000000_move_scripts_to_repos.sql**: 2 个问题
- **huanchong-99SoloDawncrates/db/migrations/20260224001000_backfill_workflow_api_key_encrypted.sql**: 2 个问题
- **huanchong-99SoloDawnfrontend/src/components/tasks/Toolbar/GitOperations.tsx**: 2 个问题
- **huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/plugins/file-tag-typeahead-plugin.tsx**: 2 个问题
- **huanchong-99SoloDawnfrontend/src/hooks/useProjectTasks.ts**: 2 个问题
- **huanchong-99SoloDawnfrontend/src/pages/settings/AgentSettings.tsx**: 2 个问题

---

## 问题列表（按文件分组）

## 1. huanchong-99SoloDawncrates/db/migrations/20250617183714_init.sql

> 该文件共有 **1** 个问题

### 1.1 Define a constant instead of duplicating this literal 7 times.

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

**问题代码片段**:
```
5: CREATE TABLE projects (
6: id BLOB PRIMARY KEY,
7: name TEXT NOT NULL,
8: git_repo_path TEXT NOT NULL DEFAULT '' UNIQUE,
9: setup_script TEXT DEFAULT '',
10: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
11: updated_at TEXT NOT NULL DEFAULT (datetime('now',))
12: );
14: CREATE TABLE tasks (
15: id BLOB PRIMARY KEY,
16: project_id BLOB NOT NULL,
17: title TEXT NOT NULL,
18: description TEXT,
19: status TEXT NOT NULL DEFAULT 'todo'
20: CHECK (status IN ('todo','inprogress','done','cancelled','inreview')),
21: created_at TEXT NOT NULL DEFAULT (datetime('now',)),
22: updated_at TEXT NOT NULL DEFAULT (datetime('now',)),
23: FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
24: );
26: CREATE TABLE task_attempts (
27: id BLOB PRIMARY KEY,
28: task_id BLOB NOT NULL,
29: worktree_path TEXT NOT NULL,
30: merge_commit TEXT,
31: executor TEXT,
32: stdout TEXT,
33: stderr TEXT,
34: created_at TEXT NOT NULL DEFAULT (datetime('now',)),
35: updated_at TEXT NOT NULL DEFAULT (datetime('now',)),
36: FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
37: );
39: CREATE TABLE task_attempt_activities (
40: id BLOB PRIMARY KEY,
41: task_attempt_id BLOB NOT NULL,
42: status TEXT NOT NULL DEFAULT 'init'
43: CHECK (status IN ('init','setuprunning','setupcomplete','setupfailed','executorrunning','executorcomplete','executorfailed','paused')), note TEXT,
44: created_at TEXT NOT NULL DEFAULT (datetime('now',)),
45: FOREIGN KEY (task_attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE
46: );
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 2. huanchong-99SoloDawncrates/db/migrations/20250620212427_execution_processes.sql

> 该文件共有 **1** 个问题

### 2.1 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
3: PRAGMA foreign_keys = ON;
5: CREATE TABLE execution_processes (
6: id BLOB PRIMARY KEY,
7: task_attempt_id BLOB NOT NULL,
8: process_type TEXT NOT NULL DEFAULT 'setupscript'
9: CHECK (process_type IN ('setupscript','codingagent','devserver')),
10: status TEXT NOT NULL DEFAULT 'running'
11: CHECK (status IN ('running','completed','failed','killed')),
12: command TEXT NOT NULL,
13: args TEXT, -- JSON array of arguments
14: working_directory TEXT NOT NULL,
15: stdout TEXT,
16: stderr TEXT,
17: exit_code INTEGER,
18: started_at TEXT NOT NULL DEFAULT (datetime('now', )),
19: completed_at TEXT,
20: created_at TEXT NOT NULL DEFAULT (datetime('now',)),
21: updated_at TEXT NOT NULL DEFAULT (datetime('now',)),
22: FOREIGN KEY (task_attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE
23: );
25: CREATE INDEX idx_execution_processes_task_attempt_id ON execution_processes(task_attempt_id);
26: CREATE INDEX idx_execution_processes_status ON execution_processes(status);
27: CREATE INDEX idx_execution_processes_type ON execution_processes(process_type);
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 3. huanchong-99SoloDawncrates/db/migrations/20250716143725_add_default_templates.sql

> 该文件共有 **1** 个问题

### 3.1 Define a constant instead of duplicating this literal 6 times.

- **问题ID**: `AZyVwe5tZ9DOUQdEsGpg`
- **项目**: huanchong-99
- **行号**: L524
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 524min effort
- **创建时间**: 1 month ago
- **标签**: design

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags "illegal character with code point 10" (newlines) in string literals below.
2: -- This is intentional - template content contains multi-line markdown text that requires embedded newlines.
3: -- NOTE: SonarCloud flags duplicate string literals in this migration.
4: -- This is acceptable for SQL DDL migrations where repeated INSERT patterns share common column names.
6: -- Add default global templates
8: -- 1. Bug Analysis template
9: INSERT INTO task_templates (
10: id,
11: project_id,
12: title,
13: description,
14: template_name,
15: created_at,
16: updated_at
17: ) VALUES (
18: randomblob(16),
19: NULL, -- Global template
20: 'Analyze codebase for potential bugs and issues',
21: 'Perform a comprehensive analysis of the project codebase to identify potential bugs, code smells, and areas of improvement.' || char(10) || char(10) ||
22: '## Analysis Checklist:' || char(10) || char(10) ||
23: '### 1. Static Code Analysis' || char(10) ||
24: '- [ ] Run linting tools to identify syntax and style issues' || char(10) ||
25: '- [ ] Check for unused variables, imports, and dead code' || char(10) ||
26: '- [ ] Identify potential type errors or mismatches' || char(10) ||
27: '- [ ] Look for deprecated API usage' || char(10) || char(10) ||
28: '### 2. Common Bug Patterns' || char(10) ||
29: '- [ ] Check for null/undefined reference errors' || char(10) ||
30: '- [ ] Identify potential race conditions' || char(10) ||
31: '- [ ] Look for improper error handling' || char(10) ||
32: '- [ ] Check for resource leaks (memory, file handles, connections)' || char(10) ||
33: '- [ ] Identify potential security vulnerabilities (XSS, SQL injection, etc.)' || char(10) || char(10) ||
34: '### 3. Code Quality Issues' || char(10) ||
35: '- [ ] Identify overly complex functions (high cyclomatic complexity)' || char(10) ||
36: '- [ ] Look for code duplication' || char(10) ||
37: '- [ ] Check for missing or inadequate input validation' || char(10) ||
38: '- [ ] Identify hardcoded values that should be configurable' || char(10) || char(10) ||
39: '### 4. Testing Gaps' || char(10) ||
40: '- [ ] Identify untested code paths' || char(10) ||
41: '- [ ] Check for missing edge case tests' || char(10) ||
42: '- [ ] Look for inadequate error scenario testing' || char(10) || char(10) ||
43: '### 5. Performance Concerns' || char(10) ||
44: '- [ ] Identify potential performance bottlenecks' || char(10) ||
45: '- [ ] Check for inefficient algorithms or data structures' || char(10) ||
46: '- [ ] Look for unnecessary database queries or API calls' || char(10) || char(10) ||
47: '## Deliverables:' || char(10) ||
48: '1. Prioritized list of identified issues' || char(10) ||
49: '2. Recommendations for fixes' || char(10) ||
50: '3. Estimated effort for addressing each issue',
51: 'Bug Analysis',
52: datetime('now', ),
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 4. huanchong-99SoloDawncrates/db/migrations/20250730000000_add_executor_action_to_execution_processes.sql

> 该文件共有 **7** 个问题

### 4.1 Define a constant instead of duplicating this literal 4 times.

- **问题ID**: `AZyVwe47Z9DOUQdEsGpC`
- **项目**: huanchong-99
- **行号**: L234
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 234min effort
- **创建时间**: 2 days ago
- **标签**: design

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'ScriptRequest', 'Bash', json_object patterns)
2: -- in this migration. This is acceptable for SQL migration CASE expressions where each branch requires
3: -- its own complete JSON structure with repeated field names.
4: -- NOTE: Direct NULL comparison (e.g. 'variant', NULL) is intentional here for json_object() calls
5: -- which require explicit NULL values to produce valid JSON output.
6: PRAGMA foreign_keys = ON;
8: -- Add executor_action column to execution_processes table for storing full ExecutorActions JSON
9: ALTER TABLE execution_processes ADD COLUMN executor_action TEXT NOT NULL DEFAULT '{}';
11: -- Backfill legacy rows with placeholder-but-valid ExecutorAction JSON to preserve
12: -- execution history and avoid cascading deletes in related tables.
13: UPDATE execution_processes
14: SET executor_action = CASE process_type
15: WHEN 'codingagent' THEN json_object(
16: 'typ', json_object(
17: 'type', 'CodingAgentInitialRequest',
18: 'prompt', '[legacy execution process migrated without original prompt]',
19: 'executor_profile_id', json_object(
20: 'executor', COALESCE(NULLIF(upper(replace(executor_type, '-', '_')), ''), 'CLAUDE_CODE'),
21: 'variant', NULL
22: ),
23: , NULL
24: ),
25: 'next_action', NULL
26: )
27: WHEN 'cleanupscript' THEN json_object(
28: 'typ', json_object(
29: 'type', 'ScriptRequest',
30: 'script', '',
31: 'language', 'Bash',
32: 'context', 'CleanupScript',
33: , NULL
34: ),
35: 'next_action', NULL
36: )
37: WHEN 'devserver' THEN json_object(
38: 'typ', json_object(
39: 'type', 'ScriptRequest',
40: 'script', '',
41: 'language', 'Bash',
42: 'context', 'DevServer',
43: , NULL
44: ),
45: 'next_action', NULL
46: )
47: ELSE json_object(
48: 'typ', json_object(
49: 'type', 'ScriptRequest',
50: 'script', '',
51: 'language', 'Bash',
52: 'context', 'SetupScript',
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 4.2 Define a constant instead of duplicating this literal 4 times.

- **问题ID**: `AZyVwe47Z9DOUQdEsGo_`
- **项目**: huanchong-99
- **行号**: L254
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 254min effort
- **创建时间**: 2 days ago
- **标签**: design

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'ScriptRequest', 'Bash', json_object patterns)
2: -- in this migration. This is acceptable for SQL migration CASE expressions where each branch requires
3: -- its own complete JSON structure with repeated field names.
4: -- NOTE: Direct NULL comparison (e.g. 'variant', NULL) is intentional here for json_object() calls
5: -- which require explicit NULL values to produce valid JSON output.
6: PRAGMA foreign_keys = ON;
8: -- Add executor_action column to execution_processes table for storing full ExecutorActions JSON
9: ALTER TABLE execution_processes ADD COLUMN executor_action TEXT NOT NULL DEFAULT '{}';
11: -- Backfill legacy rows with placeholder-but-valid ExecutorAction JSON to preserve
12: -- execution history and avoid cascading deletes in related tables.
13: UPDATE execution_processes
14: SET executor_action = CASE process_type
15: WHEN 'codingagent' THEN json_object(
16: 'typ', json_object(
17: 'type', 'CodingAgentInitialRequest',
18: 'prompt', '[legacy execution process migrated without original prompt]',
19: 'executor_profile_id', json_object(
20: 'executor', COALESCE(NULLIF(upper(replace(executor_type, '-', '_')), ''), 'CLAUDE_CODE'),
21: 'variant', NULL
22: ),
23: 'working_dir', NULL
24: ),
25: , NULL
26: )
27: WHEN 'cleanupscript' THEN json_object(
28: 'typ', json_object(
29: 'type', 'ScriptRequest',
30: 'script', '',
31: 'language', 'Bash',
32: 'context', 'CleanupScript',
33: 'working_dir', NULL
34: ),
35: , NULL
36: )
37: WHEN 'devserver' THEN json_object(
38: 'typ', json_object(
39: 'type', 'ScriptRequest',
40: 'script', '',
41: 'language', 'Bash',
42: 'context', 'DevServer',
43: 'working_dir', NULL
44: ),
45: , NULL
46: )
47: ELSE json_object(
48: 'typ', json_object(
49: 'type', 'ScriptRequest',
50: 'script', '',
51: 'language', 'Bash',
52: 'context', 'SetupScript',
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 4.3 Define a constant instead of duplicating this literal 3 times.

- **问题ID**: `AZyVwe47Z9DOUQdEsGpD`
- **项目**: huanchong-99
- **行号**: L294
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 294min effort
- **创建时间**: 2 days ago
- **标签**: design

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'ScriptRequest', 'Bash', json_object patterns)
2: -- in this migration. This is acceptable for SQL migration CASE expressions where each branch requires
3: -- its own complete JSON structure with repeated field names.
4: -- NOTE: Direct NULL comparison (e.g. 'variant', NULL) is intentional here for json_object() calls
5: -- which require explicit NULL values to produce valid JSON output.
6: PRAGMA foreign_keys = ON;
8: -- Add executor_action column to execution_processes table for storing full ExecutorActions JSON
9: ALTER TABLE execution_processes ADD COLUMN executor_action TEXT NOT NULL DEFAULT '{}';
11: -- Backfill legacy rows with placeholder-but-valid ExecutorAction JSON to preserve
12: -- execution history and avoid cascading deletes in related tables.
13: UPDATE execution_processes
14: SET executor_action = CASE process_type
15: WHEN 'codingagent' THEN json_object(
16: 'typ', json_object(
17: 'type', 'CodingAgentInitialRequest',
18: 'prompt', '[legacy execution process migrated without original prompt]',
19: 'executor_profile_id', json_object(
20: 'executor', COALESCE(NULLIF(upper(replace(executor_type, '-', '_')), ''), 'CLAUDE_CODE'),
21: 'variant', NULL
22: ),
23: 'working_dir', NULL
24: ),
25: 'next_action', NULL
26: )
27: WHEN 'cleanupscript' THEN json_object(
28: 'typ', json_object(
29: 'type', ,
30: 'script', '',
31: 'language', 'Bash',
32: 'context', 'CleanupScript',
33: 'working_dir', NULL
34: ),
35: 'next_action', NULL
36: )
37: WHEN 'devserver' THEN json_object(
38: 'typ', json_object(
39: 'type',,
40: 'script', '',
41: 'language', 'Bash',
42: 'context', 'DevServer',
43: 'working_dir', NULL
44: ),
45: 'next_action', NULL
46: )
47: ELSE json_object(
48: 'typ', json_object(
49: 'type',,
50: 'script', '',
51: 'language', 'Bash',
52: 'context', 'SetupScript',
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 4.4 Define a constant instead of duplicating this literal 3 times.

- **问题ID**: `AZyVwe47Z9DOUQdEsGpA`
- **项目**: huanchong-99
- **行号**: L304
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 304min effort
- **创建时间**: 2 days ago
- **标签**: design

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'ScriptRequest', 'Bash', json_object patterns)
2: -- in this migration. This is acceptable for SQL migration CASE expressions where each branch requires
3: -- its own complete JSON structure with repeated field names.
4: -- NOTE: Direct NULL comparison (e.g. 'variant', NULL) is intentional here for json_object() calls
5: -- which require explicit NULL values to produce valid JSON output.
6: PRAGMA foreign_keys = ON;
8: -- Add executor_action column to execution_processes table for storing full ExecutorActions JSON
9: ALTER TABLE execution_processes ADD COLUMN executor_action TEXT NOT NULL DEFAULT '{}';
11: -- Backfill legacy rows with placeholder-but-valid ExecutorAction JSON to preserve
12: -- execution history and avoid cascading deletes in related tables.
13: UPDATE execution_processes
14: SET executor_action = CASE process_type
15: WHEN 'codingagent' THEN json_object(
16: 'typ', json_object(
17: 'type', 'CodingAgentInitialRequest',
18: 'prompt', '[legacy execution process migrated without original prompt]',
19: 'executor_profile_id', json_object(
20: 'executor', COALESCE(NULLIF(upper(replace(executor_type, '-', '_')), ''), 'CLAUDE_CODE'),
21: 'variant', NULL
22: ),
23: 'working_dir', NULL
24: ),
25: 'next_action', NULL
26: )
27: WHEN 'cleanupscript' THEN json_object(
28: 'typ', json_object(
29: 'type', 'ScriptRequest',
30: , '',
31: 'language', 'Bash',
32: 'context', 'CleanupScript',
33: 'working_dir', NULL
34: ),
35: 'next_action', NULL
36: )
37: WHEN 'devserver' THEN json_object(
38: 'typ', json_object(
39: 'type', 'ScriptRequest',
40: , '',
41: 'language', 'Bash',
42: 'context', 'DevServer',
43: 'working_dir', NULL
44: ),
45: 'next_action', NULL
46: )
47: ELSE json_object(
48: 'typ', json_object(
49: 'type', 'ScriptRequest',
50: , '',
51: 'language', 'Bash',
52: 'context', 'SetupScript',
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 4.5 Define a constant instead of duplicating this literal 3 times.

- **问题ID**: `AZyVwe47Z9DOUQdEsGpB`
- **项目**: huanchong-99
- **行号**: L314
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 314min effort
- **创建时间**: 2 days ago
- **标签**: design

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'ScriptRequest', 'Bash', json_object patterns)
2: -- in this migration. This is acceptable for SQL migration CASE expressions where each branch requires
3: -- its own complete JSON structure with repeated field names.
4: -- NOTE: Direct NULL comparison (e.g. 'variant', NULL) is intentional here for json_object() calls
5: -- which require explicit NULL values to produce valid JSON output.
6: PRAGMA foreign_keys = ON;
8: -- Add executor_action column to execution_processes table for storing full ExecutorActions JSON
9: ALTER TABLE execution_processes ADD COLUMN executor_action TEXT NOT NULL DEFAULT '{}';
11: -- Backfill legacy rows with placeholder-but-valid ExecutorAction JSON to preserve
12: -- execution history and avoid cascading deletes in related tables.
13: UPDATE execution_processes
14: SET executor_action = CASE process_type
15: WHEN 'codingagent' THEN json_object(
16: 'typ', json_object(
17: 'type', 'CodingAgentInitialRequest',
18: 'prompt', '[legacy execution process migrated without original prompt]',
19: 'executor_profile_id', json_object(
20: 'executor', COALESCE(NULLIF(upper(replace(executor_type, '-', '_')), ''), 'CLAUDE_CODE'),
21: 'variant', NULL
22: ),
23: 'working_dir', NULL
24: ),
25: 'next_action', NULL
26: )
27: WHEN 'cleanupscript' THEN json_object(
28: 'typ', json_object(
29: 'type', 'ScriptRequest',
30: 'script', '',
31: , 'Bash',
32: 'context', 'CleanupScript',
33: 'working_dir', NULL
34: ),
35: 'next_action', NULL
36: )
37: WHEN 'devserver' THEN json_object(
38: 'typ', json_object(
39: 'type', 'ScriptRequest',
40: 'script', '',
41: , 'Bash',
42: 'context', 'DevServer',
43: 'working_dir', NULL
44: ),
45: 'next_action', NULL
46: )
47: ELSE json_object(
48: 'typ', json_object(
49: 'type', 'ScriptRequest',
50: 'script', '',
51: , 'Bash',
52: 'context', 'SetupScript',
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 4.6 Define a constant instead of duplicating this literal 3 times.

- **问题ID**: `AZyVwe47Z9DOUQdEsGo-`
- **项目**: huanchong-99
- **行号**: L324
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 324min effort
- **创建时间**: 2 days ago
- **标签**: design

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'ScriptRequest', 'Bash', json_object patterns)
2: -- in this migration. This is acceptable for SQL migration CASE expressions where each branch requires
3: -- its own complete JSON structure with repeated field names.
4: -- NOTE: Direct NULL comparison (e.g. 'variant', NULL) is intentional here for json_object() calls
5: -- which require explicit NULL values to produce valid JSON output.
6: PRAGMA foreign_keys = ON;
8: -- Add executor_action column to execution_processes table for storing full ExecutorActions JSON
9: ALTER TABLE execution_processes ADD COLUMN executor_action TEXT NOT NULL DEFAULT '{}';
11: -- Backfill legacy rows with placeholder-but-valid ExecutorAction JSON to preserve
12: -- execution history and avoid cascading deletes in related tables.
13: UPDATE execution_processes
14: SET executor_action = CASE process_type
15: WHEN 'codingagent' THEN json_object(
16: 'typ', json_object(
17: 'type', 'CodingAgentInitialRequest',
18: 'prompt', '[legacy execution process migrated without original prompt]',
19: 'executor_profile_id', json_object(
20: 'executor', COALESCE(NULLIF(upper(replace(executor_type, '-', '_')), ''), 'CLAUDE_CODE'),
21: 'variant', NULL
22: ),
23: 'working_dir', NULL
24: ),
25: 'next_action', NULL
26: )
27: WHEN 'cleanupscript' THEN json_object(
28: 'typ', json_object(
29: 'type', 'ScriptRequest',
30: 'script', '',
31: 'language', 'Bash',
32: , 'CleanupScript',
33: 'working_dir', NULL
34: ),
35: 'next_action', NULL
36: )
37: WHEN 'devserver' THEN json_object(
38: 'typ', json_object(
39: 'type', 'ScriptRequest',
40: 'script', '',
41: 'language', 'Bash',
42: , 'DevServer',
43: 'working_dir', NULL
44: ),
45: 'next_action', NULL
46: )
47: ELSE json_object(
48: 'typ', json_object(
49: 'type', 'ScriptRequest',
50: 'script', '',
51: 'language', 'Bash',
52: , 'SetupScript',
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 4.7 Use IS NULL and IS NOT NULL instead of direct NULL comparisons.

- **问题ID**: `AZyVwe47Z9DOUQdEsGo9`
- **项目**: huanchong-99
- **行号**: L5810
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 5810min effort
- **创建时间**: 2 days ago
- **标签**: sql

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'ScriptRequest', 'Bash', json_object patterns)
2: -- in this migration. This is acceptable for SQL migration CASE expressions where each branch requires
3: -- its own complete JSON structure with repeated field names.
4: -- NOTE: Direct NULL comparison (e.g. 'variant', NULL) is intentional here for json_object() calls
5: -- which require explicit NULL values to produce valid JSON output.
6: PRAGMA foreign_keys = ON;
8: -- Add executor_action column to execution_processes table for storing full ExecutorActions JSON
9: ALTER TABLE execution_processes ADD COLUMN executor_action TEXT NOT NULL DEFAULT '{}';
11: -- Backfill legacy rows with placeholder-but-valid ExecutorAction JSON to preserve
12: -- execution history and avoid cascading deletes in related tables.
13: UPDATE execution_processes
14: SET executor_action = CASE process_type
15: WHEN 'codingagent' THEN json_object(
16: 'typ', json_object(
17: 'type', 'CodingAgentInitialRequest',
18: 'prompt', '[legacy execution process migrated without original prompt]',
19: 'executor_profile_id', json_object(
20: 'executor', COALESCE(NULLIF(upper(replace(executor_type, '-', '_')), ''), 'CLAUDE_CODE'),
21: 'variant', NULL
22: ),
23: , NULL
24: ),
25: , NULL
26: )
27: WHEN 'cleanupscript' THEN json_object(
28: 'typ', json_object(
29: 'type', ,
30: , '',
31: , 'Bash',
32: , 'CleanupScript',
33: 'working_dir', NULL
34: ),
35: 'next_action', NULL
36: )
37: WHEN 'devserver' THEN json_object(
38: 'typ', json_object(
39: 'type', 'ScriptRequest',
40: 'script', '',
41: 'language', 'Bash',
42: 'context', 'DevServer',
43: 'working_dir', NULL
44: ),
45: 'next_action', NULL
46: )
47: ELSE json_object(
48: 'typ', json_object(
49: 'type', 'ScriptRequest',
50: 'script', '',
51: 'language', 'Bash',
52: 'context', 'SetupScript',
```

---

## 5. huanchong-99SoloDawncrates/db/migrations/20250815100344_migrate_old_executor_actions.sql

> 该文件共有 **1** 个问题

### 5.1 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals in this migration.
2: -- This is acceptable for SQL migration scripts where json_extract/json_set patterns repeat field paths.
4: -- JSON format changed, means you can access logs from old execution_processes
6: UPDATE execution_processes
7: SET executor_action = json_set(
8: json_remove(executor_action, ),
9: '$.typ.profile_variant_label',
10: json_object(
11: 'profile', json_extract(executor_action,),
12: 'variant', json('null')
13: )
14: )
15: WHERE json_type(executor_action, '$.typ') IS NOT NULL
16: AND json_type(executor_action,) = 'text';
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 6. huanchong-99SoloDawncrates/db/migrations/20250818150000_refactor_images_to_junction_tables.sql

> 该文件共有 **1** 个问题

### 6.1 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
3: PRAGMA foreign_keys = ON;
5: -- Refactor images table to use junction tables for many-to-many relationships
6: -- This allows images to be associated with multiple tasks and execution processes
7: -- No data migration needed as there are no existing users of the image system
9: CREATE TABLE images (
10: id BLOB PRIMARY KEY,
11: file_path TEXT NOT NULL, -- relative path within cache/images/
12: original_name TEXT NOT NULL,
13: mime_type TEXT,
14: size_bytes INTEGER,
15: hash TEXT NOT NULL UNIQUE, -- SHA256 for deduplication
16: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
17: updated_at TEXT NOT NULL DEFAULT (datetime('now',))
18: );
20: -- Create junction table for task-image associations
21: CREATE TABLE task_images (
22: id BLOB PRIMARY KEY,
23: task_id BLOB NOT NULL,
24: image_id BLOB NOT NULL,
25: created_at TEXT NOT NULL DEFAULT (datetime('now',)),
26: FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
27: FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE CASCADE,
28: UNIQUE(task_id, image_id) -- Prevent duplicate associations
29: );
32: -- Create indexes for efficient querying
33: CREATE INDEX idx_images_hash ON images(hash);
34: CREATE INDEX idx_task_images_task_id ON task_images(task_id);
35: CREATE INDEX idx_task_images_image_id ON task_images(image_id);
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 7. huanchong-99SoloDawncrates/db/migrations/20250819000000_move_merge_commit_to_merges_table.sql

> 该文件共有 **3** 个问题

### 7.1 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'direct', 'pr', datetime('now', 'subsec'))
2: -- in this migration. This is acceptable for SQL DDL migrations where CHECK constraints and INSERT
3: -- statements necessarily repeat enum values and default expressions.
5: -- Create enhanced merges table with type-specific columns
6: CREATE TABLE merges (
7: id BLOB PRIMARY KEY,
8: task_attempt_id BLOB NOT NULL,
9: merge_type TEXT NOT NULL CHECK (merge_type IN (, 'pr')),
11: -- Direct merge fields (NULL for PR merges)
12: merge_commit TEXT,
14: -- PR merge fields (NULL for direct merges)
15: pr_number INTEGER,
16: pr_url TEXT,
17: pr_status TEXT CHECK (pr_status IN ('open', 'merged', 'closed')),
18: pr_merged_at TEXT,
19: pr_merge_commit_sha TEXT,
21: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
22: target_branch_name TEXT NOT NULL,
24: -- Data integrity constraints
25: CHECK (
26: (merge_type =AND merge_commit IS NOT NULL
27: AND pr_number IS NULL AND pr_url IS NULL)
28: OR
29: (merge_type = 'pr' AND pr_number IS NOT NULL AND pr_url IS NOT NULL
30: AND pr_status IS NOT NULL AND merge_commit IS NULL)
31: ),
33: FOREIGN KEY (task_attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE
34: );
36: -- Create general index for all task_attempt_id queries
37: CREATE INDEX idx_merges_task_attempt_id ON merges(task_attempt_id);
39: -- Create index for finding open PRs quickly
40: CREATE INDEX idx_merges_open_pr ON merges(task_attempt_id, pr_status)
41: WHERE merge_type = 'pr' AND pr_status = 'open';
43: -- Migrate existing merge_commit data to new table as direct merges
44: INSERT INTO merges (id, task_attempt_id, merge_type, merge_commit, created_at, target_branch_name)
45: SELECT
46: randomblob(16),
47: id,
48: ,
49: merge_commit,
50: updated_at,
51: base_branch
52: FROM task_attempts
53: WHERE merge_commit IS NOT NULL;
55: -- Migrate existing PR data from task_attempts to merges
56: INSERT INTO merges (id, task_attempt_id, merge_type, pr_number, pr_url, pr_status, pr_merged_at, pr_merge_commit_sha, created_at, target_branch_name)
57: SELECT
58: randomblob(16),
59: id,
60: 'pr',
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 7.2 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'direct', 'pr', datetime('now', 'subsec'))
2: -- in this migration. This is acceptable for SQL DDL migrations where CHECK constraints and INSERT
3: -- statements necessarily repeat enum values and default expressions.
5: -- Create enhanced merges table with type-specific columns
6: CREATE TABLE merges (
7: id BLOB PRIMARY KEY,
8: task_attempt_id BLOB NOT NULL,
9: merge_type TEXT NOT NULL CHECK (merge_type IN ('direct', 'pr')),
11: -- Direct merge fields (NULL for PR merges)
12: merge_commit TEXT,
14: -- PR merge fields (NULL for direct merges)
15: pr_number INTEGER,
16: pr_url TEXT,
17: pr_status TEXT CHECK (pr_status IN ('open', 'merged', )),
18: pr_merged_at TEXT,
19: pr_merge_commit_sha TEXT,
21: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
22: target_branch_name TEXT NOT NULL,
24: -- Data integrity constraints
25: CHECK (
26: (merge_type = 'direct' AND merge_commit IS NOT NULL
27: AND pr_number IS NULL AND pr_url IS NULL)
28: OR
29: (merge_type = 'pr' AND pr_number IS NOT NULL AND pr_url IS NOT NULL
30: AND pr_status IS NOT NULL AND merge_commit IS NULL)
31: ),
33: FOREIGN KEY (task_attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE
34: );
36: -- Create general index for all task_attempt_id queries
37: CREATE INDEX idx_merges_task_attempt_id ON merges(task_attempt_id);
39: -- Create index for finding open PRs quickly
40: CREATE INDEX idx_merges_open_pr ON merges(task_attempt_id, pr_status)
41: WHERE merge_type = 'pr' AND pr_status = 'open';
43: -- Migrate existing merge_commit data to new table as direct merges
44: INSERT INTO merges (id, task_attempt_id, merge_type, merge_commit, created_at, target_branch_name)
45: SELECT
46: randomblob(16),
47: id,
48: 'direct',
49: merge_commit,
50: updated_at,
51: base_branch
52: FROM task_attempts
53: WHERE merge_commit IS NOT NULL;
55: -- Migrate existing PR data from task_attempts to merges
56: INSERT INTO merges (id, task_attempt_id, merge_type, pr_number, pr_url, pr_status, pr_merged_at, pr_merge_commit_sha, created_at, target_branch_name)
57: SELECT
58: randomblob(16),
59: id,
60: 'pr',
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 7.3 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. 'direct', 'pr', datetime('now', 'subsec'))
2: -- in this migration. This is acceptable for SQL DDL migrations where CHECK constraints and INSERT
3: -- statements necessarily repeat enum values and default expressions.
5: -- Create enhanced merges table with type-specific columns
6: CREATE TABLE merges (
7: id BLOB PRIMARY KEY,
8: task_attempt_id BLOB NOT NULL,
9: merge_type TEXT NOT NULL CHECK (merge_type IN ('direct', 'pr')),
11: -- Direct merge fields (NULL for PR merges)
12: merge_commit TEXT,
14: -- PR merge fields (NULL for direct merges)
15: pr_number INTEGER,
16: pr_url TEXT,
17: pr_status TEXT CHECK (pr_status IN ('open', , 'closed')),
18: pr_merged_at TEXT,
19: pr_merge_commit_sha TEXT,
21: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
22: target_branch_name TEXT NOT NULL,
24: -- Data integrity constraints
25: CHECK (
26: (merge_type = 'direct' AND merge_commit IS NOT NULL
27: AND pr_number IS NULL AND pr_url IS NULL)
28: OR
29: (merge_type = 'pr' AND pr_number IS NOT NULL AND pr_url IS NOT NULL
30: AND pr_status IS NOT NULL AND merge_commit IS NULL)
31: ),
33: FOREIGN KEY (task_attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE
34: );
36: -- Create general index for all task_attempt_id queries
37: CREATE INDEX idx_merges_task_attempt_id ON merges(task_attempt_id);
39: -- Create index for finding open PRs quickly
40: CREATE INDEX idx_merges_open_pr ON merges(task_attempt_id, pr_status)
41: WHERE merge_type = 'pr' AND pr_status = 'open';
43: -- Migrate existing merge_commit data to new table as direct merges
44: INSERT INTO merges (id, task_attempt_id, merge_type, merge_commit, created_at, target_branch_name)
45: SELECT
46: randomblob(16),
47: id,
48: 'direct',
49: merge_commit,
50: updated_at,
51: base_branch
52: FROM task_attempts
53: WHERE merge_commit IS NOT NULL;
55: -- Migrate existing PR data from task_attempts to merges
56: INSERT INTO merges (id, task_attempt_id, merge_type, pr_number, pr_url, pr_status, pr_merged_at, pr_merge_commit_sha, created_at, target_branch_name)
57: SELECT
58: randomblob(16),
59: id,
60: 'pr',
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 8. huanchong-99SoloDawncrates/db/migrations/20251020120000_convert_templates_to_tags.sql

> 该文件共有 **2** 个问题

### 8.1 Use IS NULL and IS NOT NULL instead of direct NULL comparisons.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags direct NULL comparisons (IS NOT NULL) in the WHERE clause below.
2: -- The usage of "IS NOT NULL" here is already correct SQL syntax; this is a false positive.
4: -- Convert task_templates to tags
5: -- Migrate ALL templates with snake_case conversion
7: CREATE TABLE tags (
8: id BLOB PRIMARY KEY,
9: tag_name TEXT NOT NULL CHECK(INSTR(tag_name, ' ') = 0),
10: content TEXT NOT NULL CHECK(content ''),
11: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
12: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec'))
13: );
15: -- Only migrate templates that have non-empty descriptions
16: -- Templates with empty/null descriptions are skipped
17: INSERT INTO tags (id, tag_name, content, created_at, updated_at)
18: SELECT
19: id,
20: LOWER(REPLACE(template_name, ' ', '_')) as tag_name,
21: description,
22: created_at,
23: updated_at
24: FROM task_templates
25: WHERE description IS NOT NULL AND description '';
27: DROP INDEX idx_task_templates_project_id;
28: DROP INDEX idx_task_templates_unique_name_project;
29: DROP INDEX idx_task_templates_unique_name_global;
30: DROP TABLE task_templates;
```

### 8.2 Use IS NULL and IS NOT NULL instead of direct NULL comparisons.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags direct NULL comparisons (IS NOT NULL) in the WHERE clause below.
2: -- The usage of "IS NOT NULL" here is already correct SQL syntax; this is a false positive.
4: -- Convert task_templates to tags
5: -- Migrate ALL templates with snake_case conversion
7: CREATE TABLE tags (
8: id BLOB PRIMARY KEY,
9: tag_name TEXT NOT NULL CHECK(INSTR(tag_name, ' ') = 0),
10: content TEXT NOT NULL CHECK(content ''),
11: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
12: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec'))
13: );
15: -- Only migrate templates that have non-empty descriptions
16: -- Templates with empty/null descriptions are skipped
17: INSERT INTO tags (id, tag_name, content, created_at, updated_at)
18: SELECT
19: id,
20: LOWER(REPLACE(template_name, ' ', '_')) as tag_name,
21: description,
22: created_at,
23: updated_at
24: FROM task_templates
25: WHERE description IS NOT NULL AND description '';
27: DROP INDEX idx_task_templates_project_id;
28: DROP INDEX idx_task_templates_unique_name_project;
29: DROP INDEX idx_task_templates_unique_name_global;
30: DROP TABLE task_templates;
```

---

## 9. huanchong-99SoloDawncrates/db/migrations/20251114000000_create_shared_tasks.sql

> 该文件共有 **1** 个问题

### 9.1 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
3: PRAGMA foreign_keys = ON;
5: CREATE TABLE IF NOT EXISTS shared_tasks (
6: id BLOB PRIMARY KEY,
7: remote_project_id BLOB NOT NULL,
8: title TEXT NOT NULL,
9: description TEXT,
10: status TEXT NOT NULL DEFAULT 'todo'
11: CHECK (status IN ('todo','inprogress','done','cancelled','inreview')),
12: assignee_user_id BLOB,
13: assignee_first_name TEXT,
14: assignee_last_name TEXT,
15: assignee_username TEXT,
16: version INTEGER NOT NULL DEFAULT 1,
17: last_event_seq INTEGER,
18: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
19: updated_at TEXT NOT NULL DEFAULT (datetime('now',))
20: );
22: CREATE INDEX IF NOT EXISTS idx_shared_tasks_remote_project
23: ON shared_tasks (remote_project_id);
25: CREATE INDEX IF NOT EXISTS idx_shared_tasks_status
26: ON shared_tasks (status);
28: CREATE TABLE IF NOT EXISTS shared_activity_cursors (
29: remote_project_id BLOB PRIMARY KEY,
30: last_seq INTEGER NOT NULL CHECK (last_seq >= 0),
31: updated_at TEXT NOT NULL DEFAULT (datetime('now',))
32: );
34: ALTER TABLE tasks
35: ADD COLUMN shared_task_id BLOB REFERENCES shared_tasks(id) ON DELETE SET NULL;
37: CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_shared_task_unique
38: ON tasks(shared_task_id)
39: WHERE shared_task_id IS NOT NULL;
41: ALTER TABLE projects
42: ADD COLUMN remote_project_id BLOB;
44: CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_remote_project_id
45: ON projects(remote_project_id)
46: WHERE remote_project_id IS NOT NULL;
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 10. huanchong-99SoloDawncrates/db/migrations/20251209000000_add_project_repositories.sql

> 该文件共有 **6** 个问题

### 10.1 Define a constant instead of duplicating this literal 8 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
3: -- NOTE: Direct NULL comparisons in this migration use correct IS NOT NULL / IS NULL syntax.
4: -- NOTE: UPDATE statements without WHERE clauses intentionally affect all rows as part of the migration.
5: -- NOTE: Join conditions exceeding 3 tables are necessary for this multi-table data migration.
7: -- Step 1: Create global repos registry
8: CREATE TABLE repos (
9: id BLOB PRIMARY KEY,
10: path TEXT NOT NULL UNIQUE,
11: name TEXT NOT NULL,
12: display_name TEXT NOT NULL,
13: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
14: updated_at TEXT NOT NULL DEFAULT (datetime('now',))
15: );
17: -- Step 2: Create project_repos junction with per-repo script fields
18: CREATE TABLE project_repos (
19: id BLOB PRIMARY KEY,
20: project_id BLOB NOT NULL,
21: repo_id BLOB NOT NULL,
22: setup_script TEXT,
23: cleanup_script TEXT,
24: copy_files TEXT,
25: parallel_setup_script INTEGER NOT NULL DEFAULT 0,
26: FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
27: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
28: UNIQUE (project_id, repo_id)
29: );
30: CREATE INDEX idx_project_repos_project_id ON project_repos(project_id);
31: CREATE INDEX idx_project_repos_repo_id ON project_repos(repo_id);
33: -- Step 3: Create attempt_repos
34: CREATE TABLE attempt_repos (
35: id BLOB PRIMARY KEY,
36: attempt_id BLOB NOT NULL,
37: repo_id BLOB NOT NULL,
38: target_branch TEXT NOT NULL,
39: created_at TEXT NOT NULL DEFAULT (datetime('now',)),
40: updated_at TEXT NOT NULL DEFAULT (datetime('now',)),
41: FOREIGN KEY (attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE,
42: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
43: UNIQUE (attempt_id, repo_id)
44: );
45: CREATE INDEX idx_attempt_repos_attempt_id ON attempt_repos(attempt_id);
46: CREATE INDEX idx_attempt_repos_repo_id ON attempt_repos(repo_id);
48: -- Step 4: Execution process repo states
49: CREATE TABLE execution_process_repo_states (
50: id BLOB PRIMARY KEY,
51: execution_process_id BLOB NOT NULL,
52: repo_id BLOB NOT NULL,
53: before_head_commit TEXT,
54: after_head_commit TEXT,
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 10.2 Use IS NULL and IS NOT NULL instead of direct NULL comparisons.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
3: -- NOTE: Direct NULL comparisons in this migration use correct IS NOT NULL / IS NULL syntax.
4: -- NOTE: UPDATE statements without WHERE clauses intentionally affect all rows as part of the migration.
5: -- NOTE: Join conditions exceeding 3 tables are necessary for this multi-table data migration.
7: -- Step 1: Create global repos registry
8: CREATE TABLE repos (
9: id BLOB PRIMARY KEY,
10: path TEXT NOT NULL UNIQUE,
11: name TEXT NOT NULL,
12: display_name TEXT NOT NULL,
13: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
14: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec'))
15: );
17: -- Step 2: Create project_repos junction with per-repo script fields
18: CREATE TABLE project_repos (
19: id BLOB PRIMARY KEY,
20: project_id BLOB NOT NULL,
21: repo_id BLOB NOT NULL,
22: setup_script TEXT,
23: cleanup_script TEXT,
24: copy_files TEXT,
25: parallel_setup_script INTEGER NOT NULL DEFAULT 0,
26: FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
27: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
28: UNIQUE (project_id, repo_id)
29: );
30: CREATE INDEX idx_project_repos_project_id ON project_repos(project_id);
31: CREATE INDEX idx_project_repos_repo_id ON project_repos(repo_id);
33: -- Step 3: Create attempt_repos
34: CREATE TABLE attempt_repos (
35: id BLOB PRIMARY KEY,
36: attempt_id BLOB NOT NULL,
37: repo_id BLOB NOT NULL,
38: target_branch TEXT NOT NULL,
39: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
40: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
41: FOREIGN KEY (attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE,
42: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
43: UNIQUE (attempt_id, repo_id)
44: );
45: CREATE INDEX idx_attempt_repos_attempt_id ON attempt_repos(attempt_id);
46: CREATE INDEX idx_attempt_repos_repo_id ON attempt_repos(repo_id);
48: -- Step 4: Execution process repo states
49: CREATE TABLE execution_process_repo_states (
50: id BLOB PRIMARY KEY,
51: execution_process_id BLOB NOT NULL,
52: repo_id BLOB NOT NULL,
53: before_head_commit TEXT,
54: after_head_commit TEXT,
```

### 10.3 Use IS NULL and IS NOT NULL instead of direct NULL comparisons.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
3: -- NOTE: Direct NULL comparisons in this migration use correct IS NOT NULL / IS NULL syntax.
4: -- NOTE: UPDATE statements without WHERE clauses intentionally affect all rows as part of the migration.
5: -- NOTE: Join conditions exceeding 3 tables are necessary for this multi-table data migration.
7: -- Step 1: Create global repos registry
8: CREATE TABLE repos (
9: id BLOB PRIMARY KEY,
10: path TEXT NOT NULL UNIQUE,
11: name TEXT NOT NULL,
12: display_name TEXT NOT NULL,
13: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
14: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec'))
15: );
17: -- Step 2: Create project_repos junction with per-repo script fields
18: CREATE TABLE project_repos (
19: id BLOB PRIMARY KEY,
20: project_id BLOB NOT NULL,
21: repo_id BLOB NOT NULL,
22: setup_script TEXT,
23: cleanup_script TEXT,
24: copy_files TEXT,
25: parallel_setup_script INTEGER NOT NULL DEFAULT 0,
26: FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
27: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
28: UNIQUE (project_id, repo_id)
29: );
30: CREATE INDEX idx_project_repos_project_id ON project_repos(project_id);
31: CREATE INDEX idx_project_repos_repo_id ON project_repos(repo_id);
33: -- Step 3: Create attempt_repos
34: CREATE TABLE attempt_repos (
35: id BLOB PRIMARY KEY,
36: attempt_id BLOB NOT NULL,
37: repo_id BLOB NOT NULL,
38: target_branch TEXT NOT NULL,
39: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
40: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
41: FOREIGN KEY (attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE,
42: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
43: UNIQUE (attempt_id, repo_id)
44: );
45: CREATE INDEX idx_attempt_repos_attempt_id ON attempt_repos(attempt_id);
46: CREATE INDEX idx_attempt_repos_repo_id ON attempt_repos(repo_id);
48: -- Step 4: Execution process repo states
49: CREATE TABLE execution_process_repo_states (
50: id BLOB PRIMARY KEY,
51: execution_process_id BLOB NOT NULL,
52: repo_id BLOB NOT NULL,
53: before_head_commit TEXT,
54: after_head_commit TEXT,
```

### 10.4 Ensure that the WHERE clause is not missing in this UPDATE query.

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpZ`
- **项目**: huanchong-99
- **行号**: L11030
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 11030min effort
- **创建时间**: 1 month ago
- **标签**: sql

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
3: -- NOTE: Direct NULL comparisons in this migration use correct IS NOT NULL / IS NULL syntax.
4: -- NOTE: UPDATE statements without WHERE clauses intentionally affect all rows as part of the migration.
5: -- NOTE: Join conditions exceeding 3 tables are necessary for this multi-table data migration.
7: -- Step 1: Create global repos registry
8: CREATE TABLE repos (
9: id BLOB PRIMARY KEY,
10: path TEXT NOT NULL UNIQUE,
11: name TEXT NOT NULL,
12: display_name TEXT NOT NULL,
13: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
14: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec'))
15: );
17: -- Step 2: Create project_repos junction with per-repo script fields
18: CREATE TABLE project_repos (
19: id BLOB PRIMARY KEY,
20: project_id BLOB NOT NULL,
21: repo_id BLOB NOT NULL,
22: setup_script TEXT,
23: cleanup_script TEXT,
24: copy_files TEXT,
25: parallel_setup_script INTEGER NOT NULL DEFAULT 0,
26: FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
27: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
28: UNIQUE (project_id, repo_id)
29: );
30: CREATE INDEX idx_project_repos_project_id ON project_repos(project_id);
31: CREATE INDEX idx_project_repos_repo_id ON project_repos(repo_id);
33: -- Step 3: Create attempt_repos
34: CREATE TABLE attempt_repos (
35: id BLOB PRIMARY KEY,
36: attempt_id BLOB NOT NULL,
37: repo_id BLOB NOT NULL,
38: target_branch TEXT NOT NULL,
39: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
40: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
41: FOREIGN KEY (attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE,
42: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
43: UNIQUE (attempt_id, repo_id)
44: );
45: CREATE INDEX idx_attempt_repos_attempt_id ON attempt_repos(attempt_id);
46: CREATE INDEX idx_attempt_repos_repo_id ON attempt_repos(repo_id);
48: -- Step 4: Execution process repo states
49: CREATE TABLE execution_process_repo_states (
50: id BLOB PRIMARY KEY,
51: execution_process_id BLOB NOT NULL,
52: repo_id BLOB NOT NULL,
53: before_head_commit TEXT,
54: after_head_commit TEXT,
```

### 10.5 Ensure that the WHERE clause is not missing in this UPDATE query.

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpa`
- **项目**: huanchong-99
- **行号**: L13530
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 13530min effort
- **创建时间**: 2 days ago
- **标签**: sql

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
3: -- NOTE: Direct NULL comparisons in this migration use correct IS NOT NULL / IS NULL syntax.
4: -- NOTE: UPDATE statements without WHERE clauses intentionally affect all rows as part of the migration.
5: -- NOTE: Join conditions exceeding 3 tables are necessary for this multi-table data migration.
7: -- Step 1: Create global repos registry
8: CREATE TABLE repos (
9: id BLOB PRIMARY KEY,
10: path TEXT NOT NULL UNIQUE,
11: name TEXT NOT NULL,
12: display_name TEXT NOT NULL,
13: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
14: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec'))
15: );
17: -- Step 2: Create project_repos junction with per-repo script fields
18: CREATE TABLE project_repos (
19: id BLOB PRIMARY KEY,
20: project_id BLOB NOT NULL,
21: repo_id BLOB NOT NULL,
22: setup_script TEXT,
23: cleanup_script TEXT,
24: copy_files TEXT,
25: parallel_setup_script INTEGER NOT NULL DEFAULT 0,
26: FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
27: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
28: UNIQUE (project_id, repo_id)
29: );
30: CREATE INDEX idx_project_repos_project_id ON project_repos(project_id);
31: CREATE INDEX idx_project_repos_repo_id ON project_repos(repo_id);
33: -- Step 3: Create attempt_repos
34: CREATE TABLE attempt_repos (
35: id BLOB PRIMARY KEY,
36: attempt_id BLOB NOT NULL,
37: repo_id BLOB NOT NULL,
38: target_branch TEXT NOT NULL,
39: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
40: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
41: FOREIGN KEY (attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE,
42: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
43: UNIQUE (attempt_id, repo_id)
44: );
45: CREATE INDEX idx_attempt_repos_attempt_id ON attempt_repos(attempt_id);
46: CREATE INDEX idx_attempt_repos_repo_id ON attempt_repos(repo_id);
48: -- Step 4: Execution process repo states
49: CREATE TABLE execution_process_repo_states (
50: id BLOB PRIMARY KEY,
51: execution_process_id BLOB NOT NULL,
52: repo_id BLOB NOT NULL,
53: before_head_commit TEXT,
54: after_head_commit TEXT,
```

### 10.6 The number of join conditions 4 exceeds the maximum allowed 3.

- **问题ID**: `AZyVwe5jZ9DOUQdEsGpb`
- **项目**: huanchong-99
- **行号**: L1672
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Adaptability
- **影响**: Maintainability
- **创建时间**: 1 month ago
- **标签**: brain-overload, performance, ...

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
3: -- NOTE: Direct NULL comparisons in this migration use correct IS NOT NULL / IS NULL syntax.
4: -- NOTE: UPDATE statements without WHERE clauses intentionally affect all rows as part of the migration.
5: -- NOTE: Join conditions exceeding 3 tables are necessary for this multi-table data migration.
7: -- Step 1: Create global repos registry
8: CREATE TABLE repos (
9: id BLOB PRIMARY KEY,
10: path TEXT NOT NULL UNIQUE,
11: name TEXT NOT NULL,
12: display_name TEXT NOT NULL,
13: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
14: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec'))
15: );
17: -- Step 2: Create project_repos junction with per-repo script fields
18: CREATE TABLE project_repos (
19: id BLOB PRIMARY KEY,
20: project_id BLOB NOT NULL,
21: repo_id BLOB NOT NULL,
22: setup_script TEXT,
23: cleanup_script TEXT,
24: copy_files TEXT,
25: parallel_setup_script INTEGER NOT NULL DEFAULT 0,
26: FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
27: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
28: UNIQUE (project_id, repo_id)
29: );
30: CREATE INDEX idx_project_repos_project_id ON project_repos(project_id);
31: CREATE INDEX idx_project_repos_repo_id ON project_repos(repo_id);
33: -- Step 3: Create attempt_repos
34: CREATE TABLE attempt_repos (
35: id BLOB PRIMARY KEY,
36: attempt_id BLOB NOT NULL,
37: repo_id BLOB NOT NULL,
38: target_branch TEXT NOT NULL,
39: created_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
40: updated_at TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
41: FOREIGN KEY (attempt_id) REFERENCES task_attempts(id) ON DELETE CASCADE,
42: FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
43: UNIQUE (attempt_id, repo_id)
44: );
45: CREATE INDEX idx_attempt_repos_attempt_id ON attempt_repos(attempt_id);
46: CREATE INDEX idx_attempt_repos_repo_id ON attempt_repos(repo_id);
48: -- Step 4: Execution process repo states
49: CREATE TABLE execution_process_repo_states (
50: id BLOB PRIMARY KEY,
51: execution_process_id BLOB NOT NULL,
52: repo_id BLOB NOT NULL,
53: before_head_commit TEXT,
54: after_head_commit TEXT,
```

---

## 11. huanchong-99SoloDawncrates/db/migrations/20251216142123_refactor_task_attempts_to_workspaces_sessions.sql

> 该文件共有 **1** 个问题

### 11.1 Define a constant instead of duplicating this literal 7 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now', 'subsec')) in this migration.
2: -- This is acceptable for SQL DDL migrations where each table definition requires its own DEFAULT clause.
4: -- Refactor task_attempts into workspaces and sessions
5: -- - Rename task_attempts -> workspaces (keeps workspace-related fields)
6: -- - Create sessions table (executor moves here)
7: -- - Update execution_processes.task_attempt_id -> session_id
8: -- - Rename executor_sessions -> coding_agent_turns (drop redundant task_attempt_id)
9: -- - Rename merges.task_attempt_id -> workspace_id
10: -- - Rename tasks.parent_task_attempt -> parent_workspace_id
12: -- 1. Rename task_attempts to workspaces (FK refs auto-update in schema)
13: ALTER TABLE task_attempts RENAME TO workspaces;
15: -- 2. Create sessions table
16: CREATE TABLE sessions (
17: id BLOB PRIMARY KEY,
18: workspace_id BLOB NOT NULL,
19: executor TEXT,
20: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
21: updated_at TEXT NOT NULL DEFAULT (datetime('now',)),
22: FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
23: );
25: CREATE INDEX idx_sessions_workspace_id ON sessions(workspace_id);
27: -- 3. Migrate data: create one session per workspace
28: INSERT INTO sessions (id, workspace_id, executor, created_at, updated_at)
29: SELECT randomblob(16), id, executor, created_at, updated_at FROM workspaces;
31: -- 4. Drop executor column from workspaces
32: ALTER TABLE workspaces DROP COLUMN executor;
34: -- 5. Rename merges.task_attempt_id to workspace_id
35: DROP INDEX idx_merges_task_attempt_id;
36: DROP INDEX idx_merges_open_pr;
37: ALTER TABLE merges RENAME COLUMN task_attempt_id TO workspace_id;
38: CREATE INDEX idx_merges_workspace_id ON merges(workspace_id);
39: CREATE INDEX idx_merges_open_pr ON merges(workspace_id, pr_status)
40: WHERE merge_type = 'pr' AND pr_status = 'open';
42: -- 6. Rename tasks.parent_task_attempt to parent_workspace_id
43: DROP INDEX IF EXISTS idx_tasks_parent_task_attempt;
44: ALTER TABLE tasks RENAME COLUMN parent_task_attempt TO parent_workspace_id;
45: CREATE INDEX idx_tasks_parent_workspace_id ON tasks(parent_workspace_id);
47: -- Steps 7-8 need FK disabled to avoid cascade deletes during DROP TABLE
48: -- sqlx workaround: end auto-transaction to allow PRAGMA to take effect
49: -- https://github.com/launchbadge/sqlx/issues/2085#issuecomment-1499859906
50: COMMIT;
52: PRAGMA foreign_keys = OFF;
54: BEGIN TRANSACTION;
56: -- 7. Update execution_processes to reference session_id instead of task_attempt_id
57: -- (needs rebuild because FK target changes from workspaces to sessions)
58: DROP INDEX IF EXISTS idx_execution_processes_task_attempt_created_at;
59: DROP INDEX IF EXISTS idx_execution_processes_task_attempt_type_created;
61: CREATE TABLE execution_processes_new (
62: id BLOB PRIMARY KEY,
63: session_id BLOB NOT NULL,
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 12. huanchong-99SoloDawncrates/db/migrations/20251219000000_add_agent_working_dir_to_projects.sql

> 该文件共有 **1** 个问题

### 12.1 Use IS NULL and IS NOT NULL instead of direct NULL comparisons.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags direct NULL comparisons in this migration.
2: -- The IS NOT NULL usage here is correct SQL syntax for filtering rows during migration.
4: -- Add column with empty default first (named default_ because it's the default for new workspaces)
5: ALTER TABLE projects ADD COLUMN default_agent_working_dir TEXT DEFAULT '';
7: -- Copy existing dev_script_working_dir values to default_agent_working_dir
8: -- ONLY for single-repo projects (multi-repo projects should default to None/empty)
9: UPDATE projects SET default_agent_working_dir = dev_script_working_dir
10: WHERE dev_script_working_dir IS NOT NULL
11: AND dev_script_working_dir ''
12: AND (SELECT COUNT(*) FROM project_repos WHERE project_repos.project_id = projects.id) = 1;
14: -- Add agent_working_dir to workspaces (snapshot of project's default at workspace creation)
15: ALTER TABLE workspaces ADD COLUMN agent_working_dir TEXT DEFAULT '';
```

---

## 13. huanchong-99SoloDawncrates/db/migrations/20260107000000_move_scripts_to_repos.sql

> 该文件共有 **2** 个问题

### 13.1 Ensure that the WHERE clause is not missing in this UPDATE query.

- **问题ID**: `AZyVwe4KZ9DOUQdEsGo2`
- **项目**: huanchong-99
- **行号**: L1530
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1530min effort
- **创建时间**: 1 month ago
- **标签**: sql

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags UPDATE statements without WHERE clauses in this migration.
2: -- These UPDATEs intentionally affect all rows to migrate script data from project_repos to repos.
4: -- Add script columns to repos
5: ALTER TABLE repos ADD COLUMN setup_script TEXT;
6: ALTER TABLE repos ADD COLUMN cleanup_script TEXT;
7: ALTER TABLE repos ADD COLUMN copy_files TEXT;
8: ALTER TABLE repos ADD COLUMN parallel_setup_script INTEGER NOT NULL DEFAULT 0;
9: ALTER TABLE repos ADD COLUMN dev_server_script TEXT;
11: -- Migrate scripts only when repo-level value is unambiguous.
12: -- Avoid LIMIT 1 so shared repos with diverging per-project scripts are not
13: -- silently overwritten by an arbitrary row.
14: -- intentional: update all rows
15: repos
16: SET
17: setup_script = (
18: SELECT CASE
19: WHEN COUNT(DISTINCT NULLIF(pr.setup_script, '')) <= 1
20: THEN MAX(NULLIF(pr.setup_script, ''))
21: ELSE NULL
22: END
23: FROM project_repos pr
24: WHERE pr.repo_id = repos.id
25: ),
26: cleanup_script = (
27: SELECT CASE
28: WHEN COUNT(DISTINCT NULLIF(pr.cleanup_script, '')) <= 1
29: THEN MAX(NULLIF(pr.cleanup_script, ''))
30: ELSE NULL
31: END
32: FROM project_repos pr
33: WHERE pr.repo_id = repos.id
34: ),
35: copy_files = (
36: SELECT CASE
37: WHEN COUNT(DISTINCT NULLIF(pr.copy_files, '')) <= 1
38: THEN MAX(NULLIF(pr.copy_files, ''))
39: ELSE NULL
40: END
41: FROM project_repos pr
42: WHERE pr.repo_id = repos.id
43: ),
44: parallel_setup_script = (
45: SELECT CASE
46: WHEN COUNT(DISTINCT pr.parallel_setup_script) <= 1
47: THEN COALESCE(MAX(pr.parallel_setup_script), 0)
48: ELSE 0
49: END
50: FROM project_repos pr
51: WHERE pr.repo_id = repos.id
52: );
```

### 13.2 Ensure that the WHERE clause is not missing in this UPDATE query.

- **问题ID**: `AZyVwe4KZ9DOUQdEsGo3`
- **项目**: huanchong-99
- **行号**: L5630
- **类型**: Bug
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 5630min effort
- **创建时间**: 1 month ago
- **标签**: sql

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags UPDATE statements without WHERE clauses in this migration.
2: -- These UPDATEs intentionally affect all rows to migrate script data from project_repos to repos.
4: -- Add script columns to repos
5: ALTER TABLE repos ADD COLUMN setup_script TEXT;
6: ALTER TABLE repos ADD COLUMN cleanup_script TEXT;
7: ALTER TABLE repos ADD COLUMN copy_files TEXT;
8: ALTER TABLE repos ADD COLUMN parallel_setup_script INTEGER NOT NULL DEFAULT 0;
9: ALTER TABLE repos ADD COLUMN dev_server_script TEXT;
11: -- Migrate scripts only when repo-level value is unambiguous.
12: -- Avoid LIMIT 1 so shared repos with diverging per-project scripts are not
13: -- silently overwritten by an arbitrary row.
14: -- intentional: update all rows
15: repos
16: SET
17: setup_script = (
18: SELECT CASE
19: WHEN COUNT(DISTINCT NULLIF(pr.setup_script, '')) <= 1
20: THEN MAX(NULLIF(pr.setup_script, ''))
21: ELSE NULL
22: END
23: FROM project_repos pr
24: WHERE pr.repo_id = repos.id
25: ),
26: cleanup_script = (
27: SELECT CASE
28: WHEN COUNT(DISTINCT NULLIF(pr.cleanup_script, '')) <= 1
29: THEN MAX(NULLIF(pr.cleanup_script, ''))
30: ELSE NULL
31: END
32: FROM project_repos pr
33: WHERE pr.repo_id = repos.id
34: ),
35: copy_files = (
36: SELECT CASE
37: WHEN COUNT(DISTINCT NULLIF(pr.copy_files, '')) <= 1
38: THEN MAX(NULLIF(pr.copy_files, ''))
39: ELSE NULL
40: END
41: FROM project_repos pr
42: WHERE pr.repo_id = repos.id
43: ),
44: parallel_setup_script = (
45: SELECT CASE
46: WHEN COUNT(DISTINCT pr.parallel_setup_script) <= 1
47: THEN COALESCE(MAX(pr.parallel_setup_script), 0)
48: ELSE 0
49: END
50: FROM project_repos pr
51: WHERE pr.repo_id = repos.id
52: );
```

---

## 14. huanchong-99SoloDawncrates/db/migrations/20260117000001_create_workflow_tables.sql

> 该文件共有 **3** 个问题

### 14.1 Define a constant instead of duplicating this literal 4 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now'), cli_type_id references)
2: -- in this migration. This is acceptable for SQL DDL migrations where each table definition
3: -- requires its own DEFAULT clause and foreign key references.
5: -- ============================================================================
6: -- SoloDawn Workflow Tables Migration
7: -- Created: 2026-01-17
8: -- Description: Add workflow coordination tables for multi-terminal orchestration
9: -- ============================================================================
11: -- ----------------------------------------------------------------------------
12: -- 1. CLI Type Table (cli_type)
13: -- Stores supported AI coding agent CLI information
14: -- ----------------------------------------------------------------------------
15: CREATE TABLE IF NOT EXISTS cli_type (
16: id TEXT PRIMARY KEY,
17: name TEXT NOT NULL UNIQUE, -- Internal name, e.g., 'claude-code'
18: display_name TEXT NOT NULL, -- Display name, e.g., 'Claude Code'
19: detect_command TEXT NOT NULL, -- Detection command, e.g., 'claude --version'
20: install_command TEXT, -- Installation command (optional)
21: install_guide_url TEXT, -- Installation guide URL
22: config_file_path TEXT, -- Config file path template
23: is_system INTEGER NOT NULL DEFAULT 1, -- Is system built-in
24: created_at TEXT NOT NULL DEFAULT (datetime('now'))
25: );
27: -- Insert system built-in CLI types
28: INSERT INTO cli_type (id, name, display_name, detect_command, install_guide_url, config_file_path, is_system) VALUES
29: (, 'claude-code', 'Claude Code', 'claude --version', 'https://docs.anthropic.com/en/docs/claude-code', '~/.claude/settings.json', 1),
30: ('cli-gemini', 'gemini-cli', 'Gemini CLI', 'gemini --version', 'https://github.com/google-gemini/gemini-cli', '~/.gemini/.env', 1),
31: ('cli-codex', 'codex', 'Codex', 'codex --version', 'https://github.com/openai/codex', '~/.codex/auth.json', 1),
32: ('cli-amp', 'amp', 'Amp', 'amp --version', 'https://ampcode.com', NULL, 1),
33: ('cli-cursor', 'cursor-agent', 'Cursor Agent', 'cursor --version', 'https://cursor.sh', NULL, 1),
34: ('cli-qwen', 'qwen-code', 'Qwen Code', 'qwen --version', 'https://qwen.ai', NULL, 1),
35: ('cli-copilot', 'copilot', 'GitHub Copilot', 'gh copilot --version', 'https://github.com/features/copilot', NULL, 1),
36: ('cli-droid', 'droid', 'Droid', 'droid --version', 'https://droid.dev', NULL, 1),
37: ('cli-opencode', 'opencode', 'Opencode', 'opencode --version', 'https://opencode.dev', NULL, 1);
39: -- ----------------------------------------------------------------------------
40: -- 2. Model Config Table (model_config)
41: -- Stores model configurations for each CLI
42: -- ----------------------------------------------------------------------------
43: CREATE TABLE IF NOT EXISTS model_config (
44: id TEXT PRIMARY KEY,
45: cli_type_id TEXT NOT NULL REFERENCES cli_type(id) ON DELETE CASCADE,
46: name TEXT NOT NULL, -- Model internal name, e.g., 'sonnet'
47: display_name TEXT NOT NULL, -- Display name, e.g., 'Claude Sonnet'
48: api_model_id TEXT, -- API model ID, e.g., 'claude-sonnet-4-20250514'
49: is_default INTEGER NOT NULL DEFAULT 0, -- Is default model
50: is_official INTEGER NOT NULL DEFAULT 0, -- Is official model
51: created_at TEXT NOT NULL DEFAULT (datetime('now')),
52: updated_at TEXT NOT NULL DEFAULT (datetime('now')),
53: UNIQUE(cli_type_id, name)
54: );
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 14.2 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now'), cli_type_id references)
2: -- in this migration. This is acceptable for SQL DDL migrations where each table definition
3: -- requires its own DEFAULT clause and foreign key references.
5: -- ============================================================================
6: -- SoloDawn Workflow Tables Migration
7: -- Created: 2026-01-17
8: -- Description: Add workflow coordination tables for multi-terminal orchestration
9: -- ============================================================================
11: -- ----------------------------------------------------------------------------
12: -- 1. CLI Type Table (cli_type)
13: -- Stores supported AI coding agent CLI information
14: -- ----------------------------------------------------------------------------
15: CREATE TABLE IF NOT EXISTS cli_type (
16: id TEXT PRIMARY KEY,
17: name TEXT NOT NULL UNIQUE, -- Internal name, e.g., 'claude-code'
18: display_name TEXT NOT NULL, -- Display name, e.g., 'Claude Code'
19: detect_command TEXT NOT NULL, -- Detection command, e.g., 'claude --version'
20: install_command TEXT, -- Installation command (optional)
21: install_guide_url TEXT, -- Installation guide URL
22: config_file_path TEXT, -- Config file path template
23: is_system INTEGER NOT NULL DEFAULT 1, -- Is system built-in
24: created_at TEXT NOT NULL DEFAULT (datetime('now'))
25: );
27: -- Insert system built-in CLI types
28: INSERT INTO cli_type (id, name, display_name, detect_command, install_guide_url, config_file_path, is_system) VALUES
29: ('cli-claude-code', 'claude-code', 'Claude Code', 'claude --version', 'https://docs.anthropic.com/en/docs/claude-code', '~/.claude/settings.json', 1),
30: (, 'gemini-cli', 'Gemini CLI', 'gemini --version', 'https://github.com/google-gemini/gemini-cli', '~/.gemini/.env', 1),
31: ('cli-codex', 'codex', 'Codex', 'codex --version', 'https://github.com/openai/codex', '~/.codex/auth.json', 1),
32: ('cli-amp', 'amp', 'Amp', 'amp --version', 'https://ampcode.com', NULL, 1),
33: ('cli-cursor', 'cursor-agent', 'Cursor Agent', 'cursor --version', 'https://cursor.sh', NULL, 1),
34: ('cli-qwen', 'qwen-code', 'Qwen Code', 'qwen --version', 'https://qwen.ai', NULL, 1),
35: ('cli-copilot', 'copilot', 'GitHub Copilot', 'gh copilot --version', 'https://github.com/features/copilot', NULL, 1),
36: ('cli-droid', 'droid', 'Droid', 'droid --version', 'https://droid.dev', NULL, 1),
37: ('cli-opencode', 'opencode', 'Opencode', 'opencode --version', 'https://opencode.dev', NULL, 1);
39: -- ----------------------------------------------------------------------------
40: -- 2. Model Config Table (model_config)
41: -- Stores model configurations for each CLI
42: -- ----------------------------------------------------------------------------
43: CREATE TABLE IF NOT EXISTS model_config (
44: id TEXT PRIMARY KEY,
45: cli_type_id TEXT NOT NULL REFERENCES cli_type(id) ON DELETE CASCADE,
46: name TEXT NOT NULL, -- Model internal name, e.g., 'sonnet'
47: display_name TEXT NOT NULL, -- Display name, e.g., 'Claude Sonnet'
48: api_model_id TEXT, -- API model ID, e.g., 'claude-sonnet-4-20250514'
49: is_default INTEGER NOT NULL DEFAULT 0, -- Is default model
50: is_official INTEGER NOT NULL DEFAULT 0, -- Is official model
51: created_at TEXT NOT NULL DEFAULT (datetime('now')),
52: updated_at TEXT NOT NULL DEFAULT (datetime('now')),
53: UNIQUE(cli_type_id, name)
54: );
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 14.3 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now'), cli_type_id references)
2: -- in this migration. This is acceptable for SQL DDL migrations where each table definition
3: -- requires its own DEFAULT clause and foreign key references.
5: -- ============================================================================
6: -- SoloDawn Workflow Tables Migration
7: -- Created: 2026-01-17
8: -- Description: Add workflow coordination tables for multi-terminal orchestration
9: -- ============================================================================
11: -- ----------------------------------------------------------------------------
12: -- 1. CLI Type Table (cli_type)
13: -- Stores supported AI coding agent CLI information
14: -- ----------------------------------------------------------------------------
15: CREATE TABLE IF NOT EXISTS cli_type (
16: id TEXT PRIMARY KEY,
17: name TEXT NOT NULL UNIQUE, -- Internal name, e.g., 'claude-code'
18: display_name TEXT NOT NULL, -- Display name, e.g., 'Claude Code'
19: detect_command TEXT NOT NULL, -- Detection command, e.g., 'claude --version'
20: install_command TEXT, -- Installation command (optional)
21: install_guide_url TEXT, -- Installation guide URL
22: config_file_path TEXT, -- Config file path template
23: is_system INTEGER NOT NULL DEFAULT 1, -- Is system built-in
24: created_at TEXT NOT NULL DEFAULT (datetime('now'))
25: );
27: -- Insert system built-in CLI types
28: INSERT INTO cli_type (id, name, display_name, detect_command, install_guide_url, config_file_path, is_system) VALUES
29: ('cli-claude-code', 'claude-code', 'Claude Code', 'claude --version', 'https://docs.anthropic.com/en/docs/claude-code', '~/.claude/settings.json', 1),
30: ('cli-gemini', 'gemini-cli', 'Gemini CLI', 'gemini --version', 'https://github.com/google-gemini/gemini-cli', '~/.gemini/.env', 1),
31: (, 'codex', 'Codex', 'codex --version', 'https://github.com/openai/codex', '~/.codex/auth.json', 1),
32: ('cli-amp', 'amp', 'Amp', 'amp --version', 'https://ampcode.com', NULL, 1),
33: ('cli-cursor', 'cursor-agent', 'Cursor Agent', 'cursor --version', 'https://cursor.sh', NULL, 1),
34: ('cli-qwen', 'qwen-code', 'Qwen Code', 'qwen --version', 'https://qwen.ai', NULL, 1),
35: ('cli-copilot', 'copilot', 'GitHub Copilot', 'gh copilot --version', 'https://github.com/features/copilot', NULL, 1),
36: ('cli-droid', 'droid', 'Droid', 'droid --version', 'https://droid.dev', NULL, 1),
37: ('cli-opencode', 'opencode', 'Opencode', 'opencode --version', 'https://opencode.dev', NULL, 1);
39: -- ----------------------------------------------------------------------------
40: -- 2. Model Config Table (model_config)
41: -- Stores model configurations for each CLI
42: -- ----------------------------------------------------------------------------
43: CREATE TABLE IF NOT EXISTS model_config (
44: id TEXT PRIMARY KEY,
45: cli_type_id TEXT NOT NULL REFERENCES cli_type(id) ON DELETE CASCADE,
46: name TEXT NOT NULL, -- Model internal name, e.g., 'sonnet'
47: display_name TEXT NOT NULL, -- Display name, e.g., 'Claude Sonnet'
48: api_model_id TEXT, -- API model ID, e.g., 'claude-sonnet-4-20250514'
49: is_default INTEGER NOT NULL DEFAULT 0, -- Is default model
50: is_official INTEGER NOT NULL DEFAULT 0, -- Is official model
51: created_at TEXT NOT NULL DEFAULT (datetime('now')),
52: updated_at TEXT NOT NULL DEFAULT (datetime('now')),
53: UNIQUE(cli_type_id, name)
54: );
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 15. huanchong-99SoloDawncrates/db/migrations/20260119000001_add_performance_indexes.sql

> 该文件共有 **4** 个问题

### 15.1 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. status values like 'completed', 'failed', 'cancelled')
2: -- in this migration. This is acceptable for SQL index definitions where partial index WHERE clauses
3: -- necessarily repeat the same status enum values across different tables.
5: -- ============================================================================
6: -- SoloDawn Performance Indexes Migration
7: -- Created: 2026-01-19
8: -- Description: Add composite and partial indexes for workflow query optimization
9: -- ============================================================================
11: -- ----------------------------------------------------------------------------
12: -- Workflow Table Indexes
13: -- ----------------------------------------------------------------------------
15: -- Index for finding active workflows by project (most common query)
16: CREATE INDEX IF NOT EXISTS idx_workflow_project_status
17: ON workflow(project_id, status)
18: WHERE status IN ('created', 'ready', 'running');
20: -- Index for listing active workflows sorted by creation time
21: CREATE INDEX IF NOT EXISTS idx_workflow_active
22: ON workflow(status, created_at DESC)
23: WHERE status NOT IN ('completed', 'failed', );
25: -- Index for cleanup operations on completed workflows
26: CREATE INDEX IF NOT EXISTS idx_workflow_completed_cleanup
27: ON workflow(project_id, completed_at)
28: WHERE status IN ('completed', 'failed',) AND completed_at IS NOT NULL;
30: -- ----------------------------------------------------------------------------
31: -- Workflow Task Table Indexes
32: -- ----------------------------------------------------------------------------
34: -- Index for finding tasks by workflow with status filtering
35: CREATE INDEX IF NOT EXISTS idx_workflow_task_workflow_status
36: ON workflow_task(workflow_id, status, order_index);
38: -- Index for finding active tasks across all workflows
39: CREATE INDEX IF NOT EXISTS idx_workflow_task_active
40: ON workflow_task(status, created_at)
41: WHERE status IN ('pending', 'running', 'review_pending');
43: -- ----------------------------------------------------------------------------
44: -- Terminal Table Indexes
45: -- ----------------------------------------------------------------------------
47: -- Index for finding terminals by task with status filtering
48: CREATE INDEX IF NOT EXISTS idx_terminal_task_status
49: ON terminal(workflow_task_id, status, order_index);
51: -- Index for finding active terminals across all tasks
52: CREATE INDEX IF NOT EXISTS idx_terminal_active
53: ON terminal(status, started_at)
54: WHERE status IN ('starting', 'waiting', 'working');
56: -- Index for cleanup operations on completed terminals
57: CREATE INDEX IF NOT EXISTS idx_terminal_cleanup
58: ON terminal(workflow_task_id, completed_at)
59: WHERE status IN ('completed', 'failed',) AND completed_at IS NOT NULL;
61: -- ----------------------------------------------------------------------------
62: -- Git Event Table Indexes
63: -- ----------------------------------------------------------------------------
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 15.2 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. status values like 'completed', 'failed', 'cancelled')
2: -- in this migration. This is acceptable for SQL index definitions where partial index WHERE clauses
3: -- necessarily repeat the same status enum values across different tables.
5: -- ============================================================================
6: -- SoloDawn Performance Indexes Migration
7: -- Created: 2026-01-19
8: -- Description: Add composite and partial indexes for workflow query optimization
9: -- ============================================================================
11: -- ----------------------------------------------------------------------------
12: -- Workflow Table Indexes
13: -- ----------------------------------------------------------------------------
15: -- Index for finding active workflows by project (most common query)
16: CREATE INDEX IF NOT EXISTS idx_workflow_project_status
17: ON workflow(project_id, status)
18: WHERE status IN ('created', 'ready', 'running');
20: -- Index for listing active workflows sorted by creation time
21: CREATE INDEX IF NOT EXISTS idx_workflow_active
22: ON workflow(status, created_at DESC)
23: WHERE status NOT IN ('completed', , 'cancelled');
25: -- Index for cleanup operations on completed workflows
26: CREATE INDEX IF NOT EXISTS idx_workflow_completed_cleanup
27: ON workflow(project_id, completed_at)
28: WHERE status IN ('completed',, 'cancelled') AND completed_at IS NOT NULL;
30: -- ----------------------------------------------------------------------------
31: -- Workflow Task Table Indexes
32: -- ----------------------------------------------------------------------------
34: -- Index for finding tasks by workflow with status filtering
35: CREATE INDEX IF NOT EXISTS idx_workflow_task_workflow_status
36: ON workflow_task(workflow_id, status, order_index);
38: -- Index for finding active tasks across all workflows
39: CREATE INDEX IF NOT EXISTS idx_workflow_task_active
40: ON workflow_task(status, created_at)
41: WHERE status IN ('pending', 'running', 'review_pending');
43: -- ----------------------------------------------------------------------------
44: -- Terminal Table Indexes
45: -- ----------------------------------------------------------------------------
47: -- Index for finding terminals by task with status filtering
48: CREATE INDEX IF NOT EXISTS idx_terminal_task_status
49: ON terminal(workflow_task_id, status, order_index);
51: -- Index for finding active terminals across all tasks
52: CREATE INDEX IF NOT EXISTS idx_terminal_active
53: ON terminal(status, started_at)
54: WHERE status IN ('starting', 'waiting', 'working');
56: -- Index for cleanup operations on completed terminals
57: CREATE INDEX IF NOT EXISTS idx_terminal_cleanup
58: ON terminal(workflow_task_id, completed_at)
59: WHERE status IN ('completed',, 'cancelled') AND completed_at IS NOT NULL;
61: -- ----------------------------------------------------------------------------
62: -- Git Event Table Indexes
63: -- ----------------------------------------------------------------------------
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 15.3 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. status values like 'completed', 'failed', 'cancelled')
2: -- in this migration. This is acceptable for SQL index definitions where partial index WHERE clauses
3: -- necessarily repeat the same status enum values across different tables.
5: -- ============================================================================
6: -- SoloDawn Performance Indexes Migration
7: -- Created: 2026-01-19
8: -- Description: Add composite and partial indexes for workflow query optimization
9: -- ============================================================================
11: -- ----------------------------------------------------------------------------
12: -- Workflow Table Indexes
13: -- ----------------------------------------------------------------------------
15: -- Index for finding active workflows by project (most common query)
16: CREATE INDEX IF NOT EXISTS idx_workflow_project_status
17: ON workflow(project_id, status)
18: WHERE status IN ('created', 'ready', 'running');
20: -- Index for listing active workflows sorted by creation time
21: CREATE INDEX IF NOT EXISTS idx_workflow_active
22: ON workflow(status, created_at DESC)
23: WHERE status NOT IN (, 'failed', 'cancelled');
25: -- Index for cleanup operations on completed workflows
26: CREATE INDEX IF NOT EXISTS idx_workflow_completed_cleanup
27: ON workflow(project_id, completed_at)
28: WHERE status IN (, 'failed', 'cancelled') AND completed_at IS NOT NULL;
30: -- ----------------------------------------------------------------------------
31: -- Workflow Task Table Indexes
32: -- ----------------------------------------------------------------------------
34: -- Index for finding tasks by workflow with status filtering
35: CREATE INDEX IF NOT EXISTS idx_workflow_task_workflow_status
36: ON workflow_task(workflow_id, status, order_index);
38: -- Index for finding active tasks across all workflows
39: CREATE INDEX IF NOT EXISTS idx_workflow_task_active
40: ON workflow_task(status, created_at)
41: WHERE status IN ('pending', 'running', 'review_pending');
43: -- ----------------------------------------------------------------------------
44: -- Terminal Table Indexes
45: -- ----------------------------------------------------------------------------
47: -- Index for finding terminals by task with status filtering
48: CREATE INDEX IF NOT EXISTS idx_terminal_task_status
49: ON terminal(workflow_task_id, status, order_index);
51: -- Index for finding active terminals across all tasks
52: CREATE INDEX IF NOT EXISTS idx_terminal_active
53: ON terminal(status, started_at)
54: WHERE status IN ('starting', 'waiting', 'working');
56: -- Index for cleanup operations on completed terminals
57: CREATE INDEX IF NOT EXISTS idx_terminal_cleanup
58: ON terminal(workflow_task_id, completed_at)
59: WHERE status IN (, 'failed', 'cancelled') AND completed_at IS NOT NULL;
61: -- ----------------------------------------------------------------------------
62: -- Git Event Table Indexes
63: -- ----------------------------------------------------------------------------
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

### 15.4 Define a constant instead of duplicating this literal 3 times.

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

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. status values like 'completed', 'failed', 'cancelled')
2: -- in this migration. This is acceptable for SQL index definitions where partial index WHERE clauses
3: -- necessarily repeat the same status enum values across different tables.
5: -- ============================================================================
6: -- SoloDawn Performance Indexes Migration
7: -- Created: 2026-01-19
8: -- Description: Add composite and partial indexes for workflow query optimization
9: -- ============================================================================
11: -- ----------------------------------------------------------------------------
12: -- Workflow Table Indexes
13: -- ----------------------------------------------------------------------------
15: -- Index for finding active workflows by project (most common query)
16: CREATE INDEX IF NOT EXISTS idx_workflow_project_status
17: ON workflow(project_id, status)
18: WHERE status IN ('created', 'ready', 'running');
20: -- Index for listing active workflows sorted by creation time
21: CREATE INDEX IF NOT EXISTS idx_workflow_active
22: ON workflow(status, created_at DESC)
23: WHERE status NOT IN ('completed', 'failed', 'cancelled');
25: -- Index for cleanup operations on completed workflows
26: CREATE INDEX IF NOT EXISTS idx_workflow_completed_cleanup
27: ON workflow(project_id, completed_at)
28: WHERE status IN ('completed', 'failed', 'cancelled') AND completed_at IS NOT NULL;
30: -- ----------------------------------------------------------------------------
31: -- Workflow Task Table Indexes
32: -- ----------------------------------------------------------------------------
34: -- Index for finding tasks by workflow with status filtering
35: CREATE INDEX IF NOT EXISTS idx_workflow_task_workflow_status
36: ON workflow_task(workflow_id, status, order_index);
38: -- Index for finding active tasks across all workflows
39: CREATE INDEX IF NOT EXISTS idx_workflow_task_active
40: ON workflow_task(status, created_at)
41: WHERE status IN (, 'running', 'review_pending');
43: -- ----------------------------------------------------------------------------
44: -- Terminal Table Indexes
45: -- ----------------------------------------------------------------------------
47: -- Index for finding terminals by task with status filtering
48: CREATE INDEX IF NOT EXISTS idx_terminal_task_status
49: ON terminal(workflow_task_id, status, order_index);
51: -- Index for finding active terminals across all tasks
52: CREATE INDEX IF NOT EXISTS idx_terminal_active
53: ON terminal(status, started_at)
54: WHERE status IN ('starting', 'waiting', 'working');
56: -- Index for cleanup operations on completed terminals
57: CREATE INDEX IF NOT EXISTS idx_terminal_cleanup
58: ON terminal(workflow_task_id, completed_at)
59: WHERE status IN ('completed', 'failed', 'cancelled') AND completed_at IS NOT NULL;
61: -- ----------------------------------------------------------------------------
62: -- Git Event Table Indexes
63: -- ----------------------------------------------------------------------------
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 16. huanchong-99SoloDawncrates/db/migrations/20260208020000_fix_terminal_old_foreign_keys.sql

> 该文件共有 **1** 个问题

### 16.1 Define a constant instead of duplicating this literal 3 times.

- **问题ID**: `AZyVwe4kZ9DOUQdEsGo6`
- **项目**: huanchong-99
- **行号**: L334
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 334min effort
- **创建时间**: 18 days ago
- **标签**: design

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags duplicate string literals (e.g. datetime('now'), table/column names) in this migration.
2: -- This is acceptable for SQL DDL migrations where table rebuild requires repeating column definitions.
3: PRAGMA foreign_keys = OFF;
5: CREATE TABLE terminal_log_new (
6: id TEXT PRIMARY KEY,
7: terminal_id TEXT NOT NULL REFERENCES terminal(id) ON DELETE CASCADE,
8: log_type TEXT NOT NULL,
9: content TEXT NOT NULL,
10: created_at TEXT NOT NULL DEFAULT (datetime('now'))
11: );
13: INSERT INTO terminal_log_new (id, terminal_id, log_type, content, created_at)
14: SELECT id, terminal_id, log_type, content, created_at
15: FROM terminal_log;
17: DROP TABLE terminal_log;
18: ALTER TABLE terminal_log_new RENAME TO terminal_log;
20: CREATE INDEX idx_terminal_log_terminal_id ON terminal_log(terminal_id);
21: CREATE INDEX idx_terminal_log_created_at ON terminal_log(created_at);
22: CREATE INDEX idx_terminal_log_streaming ON terminal_log(terminal_id, created_at DESC);
23: CREATE INDEX idx_terminal_log_cleanup ON terminal_log(created_at);
25: CREATE TABLE git_event_new (
26: id TEXT PRIMARY KEY,
27: workflow_id TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
28: terminal_id TEXT REFERENCES terminal(id),
29: commit_hash TEXT NOT NULL,
30: branch TEXT NOT NULL,
31: commit_message TEXT NOT NULL,
32: metadata TEXT,
33: process_status TEXT NOT NULL DEFAULT ,
34: agent_response TEXT,
35: created_at TEXT NOT NULL DEFAULT (datetime('now')),
36: processed_at TEXT
37: );
39: INSERT INTO git_event_new (
40: id,
41: workflow_id,
42: terminal_id,
43: commit_hash,
44: branch,
45: commit_message,
46: metadata,
47: process_status,
48: agent_response,
49: created_at,
50: processed_at
51: )
52: SELECT
53: id,
54: workflow_id,
55: terminal_id,
56: commit_hash,
```

**错误示例 (Noncompliant)**:
```
BEGIN
  prepare('action1');
  execute('action1');
  release('action1');
END;
/
```

**正确示例 (Compliant)**:
```
DECLARE
  co_action CONSTANT VARCHAR2(7) := 'action1';
BEGIN
  prepare(co_action);
  execute(co_action);
  release(co_action);
END;
/
```

---

## 17. huanchong-99SoloDawncrates/db/migrations/20260224001000_backfill_workflow_api_key_encrypted.sql

> 该文件共有 **2** 个问题

### 17.1 Use IS NULL and IS NOT NULL instead of direct NULL comparisons.

- **问题ID**: `AZyVwe6gZ9DOUQdEsGpm`
- **项目**: huanchong-99
- **行号**: L1710
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1710min effort
- **创建时间**: 2 days ago
- **标签**: sql

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags direct NULL comparisons in this migration.
2: -- The IS NULL / IS NOT NULL usage here is correct SQL syntax for conditional backfill logic.
4: -- Complete API key encryption migration by backfilling data into the new column
5: -- introduced in 20260119000000_encrypt_api_keys.sql.
6: --
7: -- Notes:
8: -- 1) Existing `workflow.orchestrator_api_key` values are already encrypted by app logic.
9: -- 2) We mirror those encrypted payloads into `orchestrator_api_key_encrypted` so future
10: -- readers can switch columns safely without data loss.
12: ALTER TABLE workflow ADD COLUMN orchestrator_api_key_encrypted TEXT;
14: -- Backfill existing rows once.
15: UPDATE workflow
16: SET orchestrator_api_key_encrypted = orchestrator_api_key
17: WHERE (orchestrator_api_key_encrypted IS NULL OR orchestrator_api_key_encrypted '')
18: AND orchestrator_api_key IS NOT NULL
19: AND orchestrator_api_key '';
21: -- Keep both columns synchronized during the transition window.
22: CREATE TRIGGER IF NOT EXISTS trg_workflow_api_key_mirror_insert
23: AFTER INSERT ON workflow
24: FOR EACH ROW
25: WHEN (NEW.orchestrator_api_key_encrypted IS NULL OR NEW.orchestrator_api_key_encrypted = '')
26: AND NEW.orchestrator_api_key IS NOT NULL
27: AND NEW.orchestrator_api_key != ''
28: BEGIN
29: UPDATE workflow
30: SET orchestrator_api_key_encrypted = NEW.orchestrator_api_key
31: WHERE id = NEW.id;
32: END;
34: CREATE TRIGGER IF NOT EXISTS trg_workflow_api_key_mirror_update
35: AFTER UPDATE OF orchestrator_api_key ON workflow
36: FOR EACH ROW
37: WHEN NEW.orchestrator_api_key IS NOT NULL
38: AND NEW.orchestrator_api_key != ''
39: AND (NEW.orchestrator_api_key_encrypted IS NULL OR NEW.orchestrator_api_key_encrypted = '')
40: BEGIN
41: UPDATE workflow
42: SET orchestrator_api_key_encrypted = NEW.orchestrator_api_key
43: WHERE id = NEW.id;
44: END;
```

### 17.2 Use IS NULL and IS NOT NULL instead of direct NULL comparisons.

- **问题ID**: `AZyVwe6gZ9DOUQdEsGpn`
- **项目**: huanchong-99
- **行号**: L1910
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 1910min effort
- **创建时间**: 2 days ago
- **标签**: sql

**问题代码片段**:
```
1: -- NOTE: SonarCloud flags direct NULL comparisons in this migration.
2: -- The IS NULL / IS NOT NULL usage here is correct SQL syntax for conditional backfill logic.
4: -- Complete API key encryption migration by backfilling data into the new column
5: -- introduced in 20260119000000_encrypt_api_keys.sql.
6: --
7: -- Notes:
8: -- 1) Existing `workflow.orchestrator_api_key` values are already encrypted by app logic.
9: -- 2) We mirror those encrypted payloads into `orchestrator_api_key_encrypted` so future
10: -- readers can switch columns safely without data loss.
12: ALTER TABLE workflow ADD COLUMN orchestrator_api_key_encrypted TEXT;
14: -- Backfill existing rows once.
15: UPDATE workflow
16: SET orchestrator_api_key_encrypted = orchestrator_api_key
17: WHERE (orchestrator_api_key_encrypted IS NULL OR orchestrator_api_key_encrypted '')
18: AND orchestrator_api_key IS NOT NULL
19: AND orchestrator_api_key '';
21: -- Keep both columns synchronized during the transition window.
22: CREATE TRIGGER IF NOT EXISTS trg_workflow_api_key_mirror_insert
23: AFTER INSERT ON workflow
24: FOR EACH ROW
25: WHEN (NEW.orchestrator_api_key_encrypted IS NULL OR NEW.orchestrator_api_key_encrypted = '')
26: AND NEW.orchestrator_api_key IS NOT NULL
27: AND NEW.orchestrator_api_key != ''
28: BEGIN
29: UPDATE workflow
30: SET orchestrator_api_key_encrypted = NEW.orchestrator_api_key
31: WHERE id = NEW.id;
32: END;
34: CREATE TRIGGER IF NOT EXISTS trg_workflow_api_key_mirror_update
35: AFTER UPDATE OF orchestrator_api_key ON workflow
36: FOR EACH ROW
37: WHEN NEW.orchestrator_api_key IS NOT NULL
38: AND NEW.orchestrator_api_key != ''
39: AND (NEW.orchestrator_api_key_encrypted IS NULL OR NEW.orchestrator_api_key_encrypted = '')
40: BEGIN
41: UPDATE workflow
42: SET orchestrator_api_key_encrypted = NEW.orchestrator_api_key
43: WHERE id = NEW.id;
44: END;
```

---

## 18. huanchong-99SoloDawnfrontend/.../NormalizedConversation/DisplayConversationEntry.tsx

> 该文件共有 **3** 个问题

### 18.1 Refactor this function to reduce its Cognitive Complexity from 19 to the 15 allowed.

- **问题ID**: `AZyVweX9Z9DOUQdEsGeL`
- **项目**: huanchong-99
- **行号**: L5349
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 5349min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

**问题代码片段**:
```
1: import { useCallback } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import WYSIWYGEditor from '@/components/ui/wysiwyg';
4: import {
5: ActionType,
6: NormalizedEntry,
7: ToolStatus,
8: type NormalizedEntryType,
9: type TaskWithAttemptStatus,
10: type JsonValue,
11: } from 'shared/types.ts';
12: import type { WorkspaceWithSession } from '@/types/attempt';
13: import type { ProcessStartPayload } from '@/types/logs';
14: import FileChangeRenderer from './FileChangeRenderer';
15: import { useExpandable } from '@/stores/useExpandableStore';
16: import {
17: AlertCircle,
18: Bot,
19: Brain,
20: CheckSquare,
21: ChevronDown,
22: Hammer,
23: Edit,
24: Eye,
25: Globe,
26: Plus,
27: Search,
28: Settings,
29: Terminal,
30: User,
31: Wrench,
32: } from 'lucide-react';
33: import RawLogText from '../common/RawLogText';
34: import UserMessage from './UserMessage';
35: import PendingApprovalEntry from './PendingApprovalEntry';
36: import { NextActionCard } from './NextActionCard';
37: import { cn } from '@/lib/utils';
38: import { useRetryUi } from '@/contexts/RetryUiContext';
39: import { Button } from '@/components/ui/button';
40: import {
41: ScriptFixerDialog,
42: type ScriptType,
43: } from '@/components/dialogs/scripts/ScriptFixerDialog';
44: import { useAttemptRepo } from '@/hooks/useAttemptRepo';
46: type Props = Readonly<{
47: entry: NormalizedEntry | ProcessStartPayload;
48: expansionKey: string;
49: executionProcessId?: string;
50: taskAttempt?: WorkspaceWithSession;
51: task?: TaskWithAttemptStatus;
```

**错误示例 (Noncompliant)**:
```
function calculateFinalPrice(user, cart) {
  let total = calculateTotal(cart);
  if (user.hasMembership                       // +1 (if)
    && user.orders > 10                        // +1 (more than one condition)
    && user.accountActive
    && !user.hasDiscount
    || user.orders === 1) {                    // +1 (change of operator in condition)
      total = applyDiscount(user, total);
  }
  return total;
}
```

**正确示例 (Compliant)**:
```
function calculateFinalPrice(user, cart) {
  let total = calculateTotal(cart);
  if (isEligibleForDiscount(user)) {       // +1 (if)
    total = applyDiscount(user, total);
  }
  return total;
}

function isEligibleForDiscount(user) {
  return user.hasMembership
    && user.orders > 10                     // +1 (more than one condition)
    && user.accountActive
    && !user.hasDiscount
    || user.orders === 1                    // +1 (change of operator in condition)
}
```

### 18.2 Refactor this function to reduce its Cognitive Complexity from 25 to the 15 allowed.

- **问题ID**: `AZyVweX9Z9DOUQdEsGeN`
- **项目**: huanchong-99
- **行号**: L80315
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 80315min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

**问题代码片段**:
```
1: import { useCallback } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import WYSIWYGEditor from '@/components/ui/wysiwyg';
4: import {
5: ActionType,
6: NormalizedEntry,
7: ToolStatus,
8: type NormalizedEntryType,
9: type TaskWithAttemptStatus,
10: type JsonValue,
11: } from 'shared/types.ts';
12: import type { WorkspaceWithSession } from '@/types/attempt';
13: import type { ProcessStartPayload } from '@/types/logs';
14: import FileChangeRenderer from './FileChangeRenderer';
15: import { useExpandable } from '@/stores/useExpandableStore';
16: import {
17: AlertCircle,
18: Bot,
19: Brain,
20: CheckSquare,
21: ChevronDown,
22: Hammer,
23: Edit,
24: Eye,
25: Globe,
26: Plus,
27: Search,
28: Settings,
29: Terminal,
30: User,
31: Wrench,
32: } from 'lucide-react';
33: import RawLogText from '../common/RawLogText';
34: import UserMessage from './UserMessage';
35: import PendingApprovalEntry from './PendingApprovalEntry';
36: import { NextActionCard } from './NextActionCard';
37: import { cn } from '@/lib/utils';
38: import { useRetryUi } from '@/contexts/RetryUiContext';
39: import { Button } from '@/components/ui/button';
40: import {
41: ScriptFixerDialog,
42: type ScriptType,
43: } from '@/components/dialogs/scripts/ScriptFixerDialog';
44: import { useAttemptRepo } from '@/hooks/useAttemptRepo';
46: type Props = Readonly<{
47: entry: NormalizedEntry | ProcessStartPayload;
48: expansionKey: string;
49: executionProcessId?: string;
50: taskAttempt?: WorkspaceWithSession;
51: task?: TaskWithAttemptStatus;
```

**错误示例 (Noncompliant)**:
```
function calculateFinalPrice(user, cart) {
  let total = calculateTotal(cart);
  if (user.hasMembership                       // +1 (if)
    && user.orders > 10                        // +1 (more than one condition)
    && user.accountActive
    && !user.hasDiscount
    || user.orders === 1) {                    // +1 (change of operator in condition)
      total = applyDiscount(user, total);
  }
  return total;
}
```

**正确示例 (Compliant)**:
```
function calculateFinalPrice(user, cart) {
  let total = calculateTotal(cart);
  if (isEligibleForDiscount(user)) {       // +1 (if)
    total = applyDiscount(user, total);
  }
  return total;
}

function isEligibleForDiscount(user) {
  return user.hasMembership
    && user.orders > 10                     // +1 (more than one condition)
    && user.accountActive
    && !user.hasDiscount
    || user.orders === 1                    // +1 (change of operator in condition)
}
```

### 18.3 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZyVweX9Z9DOUQdEsGeS`
- **项目**: huanchong-99
- **行号**: L9555
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 9555min effort
- **创建时间**: 1 month ago
- **标签**: confusing

**问题代码片段**:
```
1: import { useCallback } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import WYSIWYGEditor from '@/components/ui/wysiwyg';
4: import {
5: ActionType,
6: NormalizedEntry,
7: ToolStatus,
8: type NormalizedEntryType,
9: type TaskWithAttemptStatus,
10: type JsonValue,
11: } from 'shared/types.ts';
12: import type { WorkspaceWithSession } from '@/types/attempt';
13: import type { ProcessStartPayload } from '@/types/logs';
14: import FileChangeRenderer from './FileChangeRenderer';
15: import { useExpandable } from '@/stores/useExpandableStore';
16: import {
17: AlertCircle,
18: Bot,
19: Brain,
20: CheckSquare,
21: ChevronDown,
22: Hammer,
23: Edit,
24: Eye,
25: Globe,
26: Plus,
27: Search,
28: Settings,
29: Terminal,
30: User,
31: Wrench,
32: } from 'lucide-react';
33: import RawLogText from '../common/RawLogText';
34: import UserMessage from './UserMessage';
35: import PendingApprovalEntry from './PendingApprovalEntry';
36: import { NextActionCard } from './NextActionCard';
37: import { cn } from '@/lib/utils';
38: import { useRetryUi } from '@/contexts/RetryUiContext';
39: import { Button } from '@/components/ui/button';
40: import {
41: ScriptFixerDialog,
42: type ScriptType,
43: } from '@/components/dialogs/scripts/ScriptFixerDialog';
44: import { useAttemptRepo } from '@/hooks/useAttemptRepo';
46: type Props = Readonly<{
47: entry: NormalizedEntry | ProcessStartPayload;
48: expansionKey: string;
49: executionProcessId?: string;
50: taskAttempt?: WorkspaceWithSession;
51: task?: TaskWithAttemptStatus;
```

---

## 19. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/DiffViewCardWithComments.tsx

> 该文件共有 **1** 个问题

### 19.1 Use <img alt=...> instead of the "presentation" role to ensure accessibility across all devices.

- **问题ID**: `AZybSJntEFps_QDQ6-pn`
- **项目**: huanchong-99
- **行号**: L4115
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4115min effort
- **创建时间**: 14 hours ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import { useMemo, useCallback } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import {
4: CaretDownIcon,
5: ChatCircleIcon,
6: GithubLogoIcon,
7: } from '@phosphor-icons/react';
8: import { DiffView, DiffModeEnum, SplitSide } from '@git-diff-view/react';
9: import { generateDiffFile, type DiffFile } from '@git-diff-view/file';
10: import { cn } from '@/lib/utils';
11: import { getFileIcon } from '@/utils/fileTypeIcon';
12: import { getHighLightLanguageFromPath } from '@/utils/extToLanguage';
13: import { useTheme } from '@/components/ThemeProvider';
14: import { getActualTheme } from '@/utils/theme';
15: import { useDiffViewMode } from '@/stores/useDiffViewStore';
16: import { stripLineEnding } from '@/utils/string';
17: import {
18: useReview,
19: type ReviewDraft,
20: type ReviewComment,
21: } from '@/contexts/ReviewProvider';
22: import {
23: useWorkspaceContext,
24: type NormalizedGitHubComment,
25: } from '@/contexts/WorkspaceContext';
26: import { CommentWidgetLine } from './CommentWidgetLine';
27: import { ReviewCommentRenderer } from './ReviewCommentRenderer';
28: import { GitHubCommentRenderer } from './GitHubCommentRenderer';
29: import type { ToolStatus, DiffChangeKind } from 'shared/types';
30: import { ToolStatusDot } from '../primitives/conversation/ToolStatusDot';
31: import { OpenInIdeButton } from '@/components/ide/OpenInIdeButton';
32: import { useOpenInEditor } from '@/hooks/useOpenInEditor';
33: import '@/styles/diff-style-overrides.css';
34: import { DisplayTruncatedPath } from '@/utils/TruncatePath';
36: /** Discriminated union for comment data in extendData */
37: type ExtendLineData =
38: | { type: 'review'; comment: ReviewComment }
39: | { type: 'github'; comment: NormalizedGitHubComment };
41: // Discriminated union for input format flexibility
42: export type DiffInput =
43: | {
44: type: 'content';
45: oldContent: string;
46: newContent: string;
47: oldPath?: string;
48: newPath: string;
49: changeKind?: DiffChangeKind;
50: }
51: | {
52: type: 'unified';
```

---

## 20. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/NewDisplayConversationEntry.tsx

> 该文件共有 **5** 个问题

### 20.1 Refactor this function to reduce its Cognitive Complexity from 16 to the 15 allowed.

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

**问题代码片段**:
```
1: import { useMemo, useCallback, useLayoutEffect, useRef, useState } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import type { TFunction } from 'i18next';
4: import {
5: ActionType,
6: NormalizedEntry,
7: ToolStatus,
8: TodoItem,
9: type TaskWithAttemptStatus,
10: type RepoWithTargetBranch,
11: } from 'shared/types';
12: import type { WorkspaceWithSession } from '@/types/attempt';
13: import { DiffLineType, parseInstance } from '@git-diff-view/react';
14: import {
15: usePersistedExpanded,
16: type PersistKey,
17: } from '@/stores/useUiPreferencesStore';
18: import DisplayConversationEntry from '@/components/NormalizedConversation/DisplayConversationEntry';
19: import { useMessageEditContext } from '@/contexts/MessageEditContext';
20: import { useChangesView } from '@/contexts/ChangesViewContext';
21: import { useLogsPanel } from '@/contexts/LogsPanelContext';
22: import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
23: import { cn } from '@/lib/utils';
24: import {
25: ScriptFixerDialog,
26: type ScriptType,
27: } from '@/components/dialogs/scripts/ScriptFixerDialog';
28: import {
29: ChatToolSummary,
30: ChatTodoList,
31: ChatFileEntry,
32: ChatApprovalCard,
33: ChatUserMessage,
34: ChatAssistantMessage,
35: ChatSystemMessage,
36: ChatThinkingMessage,
37: ChatErrorMessage,
38: ChatScriptEntry,
39: } from '../primitives/conversation';
40: import type { DiffInput } from '../primitives/conversation/DiffViewCard';
42: type Props = Readonly<{
43: entry: NormalizedEntry;
44: expansionKey: string;
45: executionProcessId?: string;
46: taskAttempt?: WorkspaceWithSession;
47: task?: TaskWithAttemptStatus;
48: }>;
50: type FileEditAction = Extract<ActionType, { action: 'file_edit' }>;
52: /**
53: * Parse unified diff to extract addition/deletion counts
```

**错误示例 (Noncompliant)**:
```
function calculateFinalPrice(user, cart) {
  let total = calculateTotal(cart);
  if (user.hasMembership                       // +1 (if)
    && user.orders > 10                        // +1 (more than one condition)
    && user.accountActive
    && !user.hasDiscount
    || user.orders === 1) {                    // +1 (change of operator in condition)
      total = applyDiscount(user, total);
  }
  return total;
}
```

**正确示例 (Compliant)**:
```
function calculateFinalPrice(user, cart) {
  let total = calculateTotal(cart);
  if (isEligibleForDiscount(user)) {       // +1 (if)
    total = applyDiscount(user, total);
  }
  return total;
}

function isEligibleForDiscount(user) {
  return user.hasMembership
    && user.orders > 10                     // +1 (more than one condition)
    && user.accountActive
    && !user.hasDiscount
    || user.orders === 1                    // +1 (change of operator in condition)
}
```

### 20.2 `SCRIPT_TOOL_NAMES` should be a `Set`, and use `SCRIPT_TOOL_NAMES.has()` to check existence or non-existence.

- **问题ID**: `AZybSJoFEFps_QDQ6-po`
- **项目**: huanchong-99
- **行号**: L1575
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1575min effort
- **创建时间**: 14 hours ago
- **标签**: optimization, performance

**问题代码片段**:
```
1: import { useMemo, useCallback, useLayoutEffect, useRef, useState } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import type { TFunction } from 'i18next';
4: import {
5: ActionType,
6: NormalizedEntry,
7: ToolStatus,
8: TodoItem,
9: type TaskWithAttemptStatus,
10: type RepoWithTargetBranch,
11: } from 'shared/types';
12: import type { WorkspaceWithSession } from '@/types/attempt';
13: import { DiffLineType, parseInstance } from '@git-diff-view/react';
14: import {
15: usePersistedExpanded,
16: type PersistKey,
17: } from '@/stores/useUiPreferencesStore';
18: import DisplayConversationEntry from '@/components/NormalizedConversation/DisplayConversationEntry';
19: import { useMessageEditContext } from '@/contexts/MessageEditContext';
20: import { useChangesView } from '@/contexts/ChangesViewContext';
21: import { useLogsPanel } from '@/contexts/LogsPanelContext';
22: import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
23: import { cn } from '@/lib/utils';
24: import {
25: ScriptFixerDialog,
26: type ScriptType,
27: } from '@/components/dialogs/scripts/ScriptFixerDialog';
28: import {
29: ChatToolSummary,
30: ChatTodoList,
31: ChatFileEntry,
32: ChatApprovalCard,
33: ChatUserMessage,
34: ChatAssistantMessage,
35: ChatSystemMessage,
36: ChatThinkingMessage,
37: ChatErrorMessage,
38: ChatScriptEntry,
39: } from '../primitives/conversation';
40: import type { DiffInput } from '../primitives/conversation/DiffViewCard';
42: type Props = Readonly<{
43: entry: NormalizedEntry;
44: expansionKey: string;
45: executionProcessId?: string;
46: taskAttempt?: WorkspaceWithSession;
47: task?: TaskWithAttemptStatus;
48: }>;
50: type FileEditAction = Extract<ActionType, { action: 'file_edit' }>;
52: /**
53: * Parse unified diff to extract addition/deletion counts
```

### 20.3 Complete the task associated to this "TODO" comment.

- **问题ID**: `AZyVwelWZ9DOUQdEsGiK`
- **项目**: huanchong-99
- **行号**: L2110
- **类型**: Code Smell
- **严重程度**: Info
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2110min effort
- **创建时间**: 1 month ago
- **标签**: cwe

**问题代码片段**:
```
1: import { useMemo, useCallback, useLayoutEffect, useRef, useState } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import type { TFunction } from 'i18next';
4: import {
5: ActionType,
6: NormalizedEntry,
7: ToolStatus,
8: TodoItem,
9: type TaskWithAttemptStatus,
10: type RepoWithTargetBranch,
11: } from 'shared/types';
12: import type { WorkspaceWithSession } from '@/types/attempt';
13: import { DiffLineType, parseInstance } from '@git-diff-view/react';
14: import {
15: usePersistedExpanded,
16: type PersistKey,
17: } from '@/stores/useUiPreferencesStore';
18: import DisplayConversationEntry from '@/components/NormalizedConversation/DisplayConversationEntry';
19: import { useMessageEditContext } from '@/contexts/MessageEditContext';
20: import { useChangesView } from '@/contexts/ChangesViewContext';
21: import { useLogsPanel } from '@/contexts/LogsPanelContext';
22: import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
23: import { cn } from '@/lib/utils';
24: import {
25: ScriptFixerDialog,
26: type ScriptType,
27: } from '@/components/dialogs/scripts/ScriptFixerDialog';
28: import {
29: ChatToolSummary,
30: ChatTodoList,
31: ChatFileEntry,
32: ChatApprovalCard,
33: ChatUserMessage,
34: ChatAssistantMessage,
35: ChatSystemMessage,
36: ChatThinkingMessage,
37: ChatErrorMessage,
38: ChatScriptEntry,
39: } from '../primitives/conversation';
40: import type { DiffInput } from '../primitives/conversation/DiffViewCard';
42: type Props = Readonly<{
43: entry: NormalizedEntry;
44: expansionKey: string;
45: executionProcessId?: string;
46: taskAttempt?: WorkspaceWithSession;
47: task?: TaskWithAttemptStatus;
48: }>;
50: type FileEditAction = Extract<ActionType, { action: 'file_edit' }>;
52: /**
53: * Parse unified diff to extract addition/deletion counts
```

### 20.4 Complete the task associated to this "TODO" comment.

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

**问题代码片段**:
```
1: import { useMemo, useCallback, useLayoutEffect, useRef, useState } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import type { TFunction } from 'i18next';
4: import {
5: ActionType,
6: NormalizedEntry,
7: ToolStatus,
8: TodoItem,
9: type TaskWithAttemptStatus,
10: type RepoWithTargetBranch,
11: } from 'shared/types';
12: import type { WorkspaceWithSession } from '@/types/attempt';
13: import { DiffLineType, parseInstance } from '@git-diff-view/react';
14: import {
15: usePersistedExpanded,
16: type PersistKey,
17: } from '@/stores/useUiPreferencesStore';
18: import DisplayConversationEntry from '@/components/NormalizedConversation/DisplayConversationEntry';
19: import { useMessageEditContext } from '@/contexts/MessageEditContext';
20: import { useChangesView } from '@/contexts/ChangesViewContext';
21: import { useLogsPanel } from '@/contexts/LogsPanelContext';
22: import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
23: import { cn } from '@/lib/utils';
24: import {
25: ScriptFixerDialog,
26: type ScriptType,
27: } from '@/components/dialogs/scripts/ScriptFixerDialog';
28: import {
29: ChatToolSummary,
30: ChatTodoList,
31: ChatFileEntry,
32: ChatApprovalCard,
33: ChatUserMessage,
34: ChatAssistantMessage,
35: ChatSystemMessage,
36: ChatThinkingMessage,
37: ChatErrorMessage,
38: ChatScriptEntry,
39: } from '../primitives/conversation';
40: import type { DiffInput } from '../primitives/conversation/DiffViewCard';
42: type Props = Readonly<{
43: entry: NormalizedEntry;
44: expansionKey: string;
45: executionProcessId?: string;
46: taskAttempt?: WorkspaceWithSession;
47: task?: TaskWithAttemptStatus;
48: }>;
50: type FileEditAction = Extract<ActionType, { action: 'file_edit' }>;
52: /**
53: * Parse unified diff to extract addition/deletion counts
```

### 20.5 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZybSJoFEFps_QDQ6-pp`
- **项目**: huanchong-99
- **行号**: L6765
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 6765min effort
- **创建时间**: 14 hours ago
- **标签**: confusing

**问题代码片段**:
```
1: import { useMemo, useCallback, useLayoutEffect, useRef, useState } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import type { TFunction } from 'i18next';
4: import {
5: ActionType,
6: NormalizedEntry,
7: ToolStatus,
8: TodoItem,
9: type TaskWithAttemptStatus,
10: type RepoWithTargetBranch,
11: } from 'shared/types';
12: import type { WorkspaceWithSession } from '@/types/attempt';
13: import { DiffLineType, parseInstance } from '@git-diff-view/react';
14: import {
15: usePersistedExpanded,
16: type PersistKey,
17: } from '@/stores/useUiPreferencesStore';
18: import DisplayConversationEntry from '@/components/NormalizedConversation/DisplayConversationEntry';
19: import { useMessageEditContext } from '@/contexts/MessageEditContext';
20: import { useChangesView } from '@/contexts/ChangesViewContext';
21: import { useLogsPanel } from '@/contexts/LogsPanelContext';
22: import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
23: import { cn } from '@/lib/utils';
24: import {
25: ScriptFixerDialog,
26: type ScriptType,
27: } from '@/components/dialogs/scripts/ScriptFixerDialog';
28: import {
29: ChatToolSummary,
30: ChatTodoList,
31: ChatFileEntry,
32: ChatApprovalCard,
33: ChatUserMessage,
34: ChatAssistantMessage,
35: ChatSystemMessage,
36: ChatThinkingMessage,
37: ChatErrorMessage,
38: ChatScriptEntry,
39: } from '../primitives/conversation';
40: import type { DiffInput } from '../primitives/conversation/DiffViewCard';
42: type Props = Readonly<{
43: entry: NormalizedEntry;
44: expansionKey: string;
45: executionProcessId?: string;
46: taskAttempt?: WorkspaceWithSession;
47: task?: TaskWithAttemptStatus;
48: }>;
50: type FileEditAction = Extract<ActionType, { action: 'file_edit' }>;
52: /**
53: * Parse unified diff to extract addition/deletion counts
```

---

## 21. huanchong-99SoloDawnfrontend/.../components/ui-new/containers/ProcessListContainer.tsx

> 该文件共有 **1** 个问题

### 21.1 Use <img alt=...> instead of the "presentation" role to ensure accessibility across all devices.

- **问题ID**: `AZybSJnWEFps_QDQ6-pm`
- **项目**: huanchong-99
- **行号**: L755
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 755min effort
- **创建时间**: 14 hours ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import { useEffect, useMemo, useCallback } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import { useExecutionProcessesContext } from '@/contexts/ExecutionProcessesContext';
4: import { useLogsPanel } from '@/contexts/LogsPanelContext';
5: import { ProcessListItem } from '../primitives/ProcessListItem';
6: import { CollapsibleSectionHeader } from '../primitives/CollapsibleSectionHeader';
7: import { InputField } from '../primitives/InputField';
8: import { CaretUpIcon, CaretDownIcon } from '@phosphor-icons/react';
9: import { PERSIST_KEYS } from '@/stores/useUiPreferencesStore';
11: export function ProcessListContainer() {
12: const {
13: logsPanelContent,
14: logSearchQuery: searchQuery,
15: logMatchIndices,
16: logCurrentMatchIdx: currentMatchIdx,
17: setLogSearchQuery: onSearchQueryChange,
18: handleLogPrevMatch: onPrevMatch,
19: handleLogNextMatch: onNextMatch,
20: viewProcessInPanel: onSelectProcess,
21: } = useLogsPanel();
23: const selectedProcessId =
24: logsPanelContent?.type === 'process' ? logsPanelContent.processId : null;
25: const disableAutoSelect = logsPanelContent?.type === 'tool';
26: const matchCount = logMatchIndices.length;
27: const { t } = useTranslation('common');
28: const { executionProcessesVisible } = useExecutionProcessesContext();
30: // Sort processes by createdAt descending (newest first)
31: const sortedProcesses = useMemo(() => {
32: return [...executionProcessesVisible].sort((a, b) => {
33: return (
34: new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
35: );
36: });
37: }, [executionProcessesVisible]);
39: // Auto-select latest process if none selected (unless disabled)
40: useEffect(() => {
41: if (
42: !disableAutoSelect &&
43: !selectedProcessId &&
44: sortedProcesses.length > 0
45: ) {
46: onSelectProcess(sortedProcesses[0].id);
47: }
48: }, [disableAutoSelect, selectedProcessId, sortedProcesses, onSelectProcess]);
50: const handleSelectProcess = useCallback(
51: (processId: string) => {
52: onSelectProcess(processId);
53: },
54: [onSelectProcess]
55: );
```

---

## 22. huanchong-99SoloDawnfrontend/.../components/ui-new/primitives/SessionChatBox.tsx

> 该文件共有 **1** 个问题

### 22.1 Complete the task associated to this "TODO" comment.

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

**问题代码片段**:
```
1: import { useRef } from 'react';
2: import {
3: PaperclipIcon,
4: CheckIcon,
5: ClockIcon,
6: XIcon,
7: PlusIcon,
8: SpinnerIcon,
9: ChatCircleIcon,
10: TrashIcon,
11: WarningIcon,
12: } from '@phosphor-icons/react';
13: import { useTranslation } from 'react-i18next';
14: import type { Session, BaseCodingAgent, TodoItem } from 'shared/types';
15: import type { LocalImageMetadata } from '@/components/ui/wysiwyg/context/task-attempt-context';
16: import { formatDateShortWithTime } from '@/utils/date';
17: import { toPrettyCase } from '@/utils/string';
18: import { AgentIcon } from '@/components/agents/AgentIcon';
19: import {
20: ChatBoxBase,
21: VisualVariant,
22: type EditorProps,
23: type VariantProps,
24: } from './ChatBoxBase';
25: import { PrimaryButton } from './PrimaryButton';
26: import { ToolbarIconButton, ToolbarDropdown } from './Toolbar';
27: import {
28: type ActionDefinition,
29: type ActionVisibilityContext,
30: isSpecialIcon,
31: } from '../actions';
32: import { isActionEnabled } from '../actions/useActionVisibility';
33: import {
34: DropdownMenuItem,
35: DropdownMenuLabel,
36: DropdownMenuSeparator,
37: } from './Dropdown';
38: import { type ExecutorProps } from './CreateChatBox';
40: // Re-export shared types
41: export type { EditorProps, VariantProps } from './ChatBoxBase';
43: // Status enum - single source of truth for execution state
44: export type ExecutionStatus =
45: | 'idle'
46: | 'sending'
47: | 'running'
48: | 'queued'
49: | 'stopping'
50: | 'queue-loading'
51: | 'feedback'
52: | 'edit';
```

---

## 23. huanchong-99SoloDawnfrontend/.../components/ui/wysiwyg/plugins/file-tag-typeahead-plugin.tsx

> 该文件共有 **2** 个问题

### 23.1 Use <option> instead of the "option" role to ensure accessibility across all devices.

- **问题ID**: `AZybSJeWEFps_QDQ6-pj`
- **项目**: huanchong-99
- **行号**: L1525
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1525min effort
- **创建时间**: 14 hours ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import { useState, useCallback, useRef } from 'react';
2: import { createPortal } from 'react-dom';
3: import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
4: import {
5: LexicalTypeaheadMenuPlugin,
6: MenuOption,
7: } from '@lexical/react/LexicalTypeaheadMenuPlugin';
8: import {
9: $createTextNode,
10: $getRoot,
11: $createParagraphNode,
12: $isParagraphNode,
13: } from 'lexical';
14: import { Tag as TagIcon, FileText } from 'lucide-react';
15: import { usePortalContainer } from '@/contexts/PortalContainerContext';
16: import {
17: searchTagsAndFiles,
18: type SearchResultItem,
19: } from '@/lib/searchTagsAndFiles';
21: class FileTagOption extends MenuOption {
22: item: SearchResultItem;
24: constructor(item: SearchResultItem) {
25: const key =
26: item.type === 'tag' ? `tag-${item.tag!.id}` : `file-${item.file!.path}`;
27: super(key);
28: this.item = item;
29: }
30: }
32: const VIEWPORT_MARGIN = 8;
33: const VERTICAL_GAP = 4;
34: const VERTICAL_GAP_ABOVE = 24;
35: const MIN_WIDTH = 320;
37: // Helper to handle mouse move with position tracking
38: function createMouseMoveHandler(
39: lastMousePositionRef: React.MutableRefObject<{ x: number; y: number } | null>,
40: setHighlightedIndex: (index: number) => void,
41: index: number
42: ) {
43: return (e: React.MouseEvent) => {
44: const pos = { x: e.clientX, y: e.clientY };
45: const last = lastMousePositionRef.current;
46: if (!last || last.x !== pos.x || last.y !== pos.y) {
47: lastMousePositionRef.current = pos;
48: setHighlightedIndex(index);
49: }
50: };
51: }
53: // Helper to get item class names based on selection state
54: function getItemClassName(isSelected: boolean): string {
55: return `px-3 py-2 cursor-pointer text-sm border-l-2 ${
```

### 23.2 Use <option> instead of the "option" role to ensure accessibility across all devices.

- **问题ID**: `AZybSJeWEFps_QDQ6-pk`
- **项目**: huanchong-99
- **行号**: L1965
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1965min effort
- **创建时间**: 14 hours ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import { useState, useCallback, useRef } from 'react';
2: import { createPortal } from 'react-dom';
3: import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
4: import {
5: LexicalTypeaheadMenuPlugin,
6: MenuOption,
7: } from '@lexical/react/LexicalTypeaheadMenuPlugin';
8: import {
9: $createTextNode,
10: $getRoot,
11: $createParagraphNode,
12: $isParagraphNode,
13: } from 'lexical';
14: import { Tag as TagIcon, FileText } from 'lucide-react';
15: import { usePortalContainer } from '@/contexts/PortalContainerContext';
16: import {
17: searchTagsAndFiles,
18: type SearchResultItem,
19: } from '@/lib/searchTagsAndFiles';
21: class FileTagOption extends MenuOption {
22: item: SearchResultItem;
24: constructor(item: SearchResultItem) {
25: const key =
26: item.type === 'tag' ? `tag-${item.tag!.id}` : `file-${item.file!.path}`;
27: super(key);
28: this.item = item;
29: }
30: }
32: const VIEWPORT_MARGIN = 8;
33: const VERTICAL_GAP = 4;
34: const VERTICAL_GAP_ABOVE = 24;
35: const MIN_WIDTH = 320;
37: // Helper to handle mouse move with position tracking
38: function createMouseMoveHandler(
39: lastMousePositionRef: React.MutableRefObject<{ x: number; y: number } | null>,
40: setHighlightedIndex: (index: number) => void,
41: index: number
42: ) {
43: return (e: React.MouseEvent) => {
44: const pos = { x: e.clientX, y: e.clientY };
45: const last = lastMousePositionRef.current;
46: if (!last || last.x !== pos.x || last.y !== pos.y) {
47: lastMousePositionRef.current = pos;
48: setHighlightedIndex(index);
49: }
50: };
51: }
53: // Helper to get item class names based on selection state
54: function getItemClassName(isSelected: boolean): string {
55: return `px-3 py-2 cursor-pointer text-sm border-l-2 ${
```

---

## 24. huanchong-99SoloDawnfrontend/.../ui-new/primitives/conversation/ChatFileEntry.tsx

> 该文件共有 **3** 个问题

### 24.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweb2Z9DOUQdEsGfw`
- **项目**: huanchong-99
- **行号**: L1575
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1575min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import { useTranslation } from 'react-i18next';
2: import { CaretDownIcon, ArrowSquareUpRightIcon } from '@phosphor-icons/react';
3: import { cn } from '@/lib/utils';
4: import { getFileIcon } from '@/utils/fileTypeIcon';
5: import { useTheme } from '@/components/ThemeProvider';
6: import { getActualTheme } from '@/utils/theme';
7: import { ToolStatus } from 'shared/types';
8: import { ToolStatusDot } from './ToolStatusDot';
9: import { DiffViewBody, useDiffData, type DiffInput } from './DiffViewCard';
11: interface ChatFileEntryProps {
12: readonly filename: string;
13: readonly additions?: number;
14: readonly deletions?: number;
15: readonly expanded?: boolean;
16: readonly onToggle?: () => void;
17: readonly className?: string;
18: readonly status?: ToolStatus;
19: /** Optional diff content for expanded view */
20: readonly diffContent?: DiffInput;
21: /** Optional callback to open file in changes panel */
22: readonly onOpenInChanges?: () => void;
23: }
25: function DiffStats({ additions, deletions }: Readonly<{ additions?: number; deletions?: number }>) {
26: const hasStats = additions !== undefined || deletions !== undefined;
27: if (!hasStats) return null;
28: return (
29: <span className="text-sm shrink-0">
30: {additions !== undefined && additions > 0 && (
31: <span className="text-success">+{additions}</span>
32: )}
33: {additions !== undefined && deletions !== undefined && ' '}
34: {deletions !== undefined && deletions > 0 && (
35: <span className="text-error">-{deletions}</span>
36: )}
37: </span>
38: );
39: }
41: function FileHeaderContent({
42: filename,
43: FileIcon,
44: status,
45: onOpenInChanges,
46: additions,
47: deletions,
48: onToggle,
49: expanded,
50: viewInChangesLabel,
51: }: Readonly<{
52: filename: string;
53: FileIcon: React.ComponentType<{ className?: string }>;
```

**错误示例 (Noncompliant)**:
```
<div onClick={() => {}} />; // Noncompliant
```

**正确示例 (Compliant)**:
```
<div onClick={() => {}} role="button" />;
```

### 24.2 Use <input type="button">, <input type="image">, <input type="reset">, <input type="submit">, or <button> instead of the "button" role to ensure accessibility across all devices.

- **问题ID**: `AZyZVcRluNB-_5CPqJgV`
- **项目**: huanchong-99
- **行号**: L1575
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1575min effort
- **创建时间**: 23 hours ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import { useTranslation } from 'react-i18next';
2: import { CaretDownIcon, ArrowSquareUpRightIcon } from '@phosphor-icons/react';
3: import { cn } from '@/lib/utils';
4: import { getFileIcon } from '@/utils/fileTypeIcon';
5: import { useTheme } from '@/components/ThemeProvider';
6: import { getActualTheme } from '@/utils/theme';
7: import { ToolStatus } from 'shared/types';
8: import { ToolStatusDot } from './ToolStatusDot';
9: import { DiffViewBody, useDiffData, type DiffInput } from './DiffViewCard';
11: interface ChatFileEntryProps {
12: readonly filename: string;
13: readonly additions?: number;
14: readonly deletions?: number;
15: readonly expanded?: boolean;
16: readonly onToggle?: () => void;
17: readonly className?: string;
18: readonly status?: ToolStatus;
19: /** Optional diff content for expanded view */
20: readonly diffContent?: DiffInput;
21: /** Optional callback to open file in changes panel */
22: readonly onOpenInChanges?: () => void;
23: }
25: function DiffStats({ additions, deletions }: Readonly<{ additions?: number; deletions?: number }>) {
26: const hasStats = additions !== undefined || deletions !== undefined;
27: if (!hasStats) return null;
28: return (
29: <span className="text-sm shrink-0">
30: {additions !== undefined && additions > 0 && (
31: <span className="text-success">+{additions}</span>
32: )}
33: {additions !== undefined && deletions !== undefined && ' '}
34: {deletions !== undefined && deletions > 0 && (
35: <span className="text-error">-{deletions}</span>
36: )}
37: </span>
38: );
39: }
41: function FileHeaderContent({
42: filename,
43: FileIcon,
44: status,
45: onOpenInChanges,
46: additions,
47: deletions,
48: onToggle,
49: expanded,
50: viewInChangesLabel,
51: }: Readonly<{
52: filename: string;
53: FileIcon: React.ComponentType<{ className?: string }>;
```

### 24.3 `tabIndex` should only be declared on interactive elements.

- **问题ID**: `AZyZVcRluNB-_5CPqJgW`
- **项目**: huanchong-99
- **行号**: L1595
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1595min effort
- **创建时间**: 23 hours ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import { useTranslation } from 'react-i18next';
2: import { CaretDownIcon, ArrowSquareUpRightIcon } from '@phosphor-icons/react';
3: import { cn } from '@/lib/utils';
4: import { getFileIcon } from '@/utils/fileTypeIcon';
5: import { useTheme } from '@/components/ThemeProvider';
6: import { getActualTheme } from '@/utils/theme';
7: import { ToolStatus } from 'shared/types';
8: import { ToolStatusDot } from './ToolStatusDot';
9: import { DiffViewBody, useDiffData, type DiffInput } from './DiffViewCard';
11: interface ChatFileEntryProps {
12: readonly filename: string;
13: readonly additions?: number;
14: readonly deletions?: number;
15: readonly expanded?: boolean;
16: readonly onToggle?: () => void;
17: readonly className?: string;
18: readonly status?: ToolStatus;
19: /** Optional diff content for expanded view */
20: readonly diffContent?: DiffInput;
21: /** Optional callback to open file in changes panel */
22: readonly onOpenInChanges?: () => void;
23: }
25: function DiffStats({ additions, deletions }: Readonly<{ additions?: number; deletions?: number }>) {
26: const hasStats = additions !== undefined || deletions !== undefined;
27: if (!hasStats) return null;
28: return (
29: <span className="text-sm shrink-0">
30: {additions !== undefined && additions > 0 && (
31: <span className="text-success">+{additions}</span>
32: )}
33: {additions !== undefined && deletions !== undefined && ' '}
34: {deletions !== undefined && deletions > 0 && (
35: <span className="text-error">-{deletions}</span>
36: )}
37: </span>
38: );
39: }
41: function FileHeaderContent({
42: filename,
43: FileIcon,
44: status,
45: onOpenInChanges,
46: additions,
47: deletions,
48: onToggle,
49: expanded,
50: viewInChangesLabel,
51: }: Readonly<{
52: filename: string;
53: FileIcon: React.ComponentType<{ className?: string }>;
```

**错误示例 (Noncompliant)**:
```
<div tabIndex="0" />
```

**正确示例 (Compliant)**:
```
<div />
```

---

## 25. huanchong-99SoloDawnfrontend/src/components/ThemeProvider.tsx

> 该文件共有 **1** 个问题

### 25.1 useState call is not destructured into value + setter pair

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

**问题代码片段**:
```
1: import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
2: import { ThemeMode } from 'shared/types';
4: type ThemeProviderProps = Readonly<{
5: children: React.ReactNode;
6: initialTheme?: ThemeMode;
7: }>;
9: type ThemeProviderState = {
10: theme: ThemeMode;
11: setTheme: (theme: ThemeMode) => void;
12: };
14: const initialState: ThemeProviderState = {
15: theme: ThemeMode.SYSTEM,
16: setTheme: () => null,
17: };
19: const ThemeProviderContext = createContext<ThemeProviderState>(initialState);
21: export function ThemeProvider({
22: children,
23: initialTheme = ThemeMode.SYSTEM,
24: ...props
25: }: Readonly<ThemeProviderProps>) {
26: const = useState<ThemeMode>(initialTheme);
28: // Update theme when initialTheme changes
29: useEffect(() => {
30: setThemeState(initialTheme);
31: }, [initialTheme]);
33: useEffect(() => {
34: const root = globalThis.document.documentElement;
36: root.classList.remove('light', 'dark');
38: if (theme === ThemeMode.SYSTEM) {
39: const systemTheme = globalThis.matchMedia('(prefers-color-scheme: dark)')
40: .matches
41: ? 'dark'
42: : 'light';
44: root.classList.add(systemTheme);
45: return;
46: }
48: root.classList.add(theme.toLowerCase());
49: }, [theme]);
51: const setTheme = useCallback((newTheme: ThemeMode) => {
52: setThemeState(newTheme);
53: }, []);
55: const value = useMemo(() => ({
56: theme,
57: setTheme,
58: }), [theme, setTheme]);
60: return (
61: <ThemeProviderContext.Provider {...props} value={value}>
62: {children}
63: </ThemeProviderContext.Provider>
64: );
```

---

## 26. huanchong-99SoloDawnfrontend/src/components/layout/NewDesignLayout.tsx

> 该文件共有 **1** 个问题

### 26.1 Provide multiple methods instead of using "isDisabled" to determine which action to take.

- **问题ID**: `AZya6BbshNI1bV4F5DXP`
- **项目**: huanchong-99
- **行号**: L8015
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 8015min effort
- **创建时间**: 16 hours ago
- **标签**: design, type-dependent

**问题代码片段**:
```
1: import {
2: Outlet,
3: useLocation,
4: useNavigate,
5: useParams,
6: useSearchParams,
7: } from 'react-router-dom';
8: import { LayoutGrid, GitBranch, Bug, Settings } from 'lucide-react';
9: import { useTranslation } from 'react-i18next';
10: import { cn } from '@/lib/utils';
12: type ViewType = 'kanban' | 'pipeline' | 'debug';
14: interface ViewOption {
15: id: ViewType;
16: labelKey: string;
17: icon: React.ComponentType<{ className?: string }>;
18: path: (workflowId?: string) => string;
19: requiresWorkflow: boolean;
20: }
22: /**
23: * View options matching actual route structure:
24: * - /board (kanban, no workflow required)
25: * - /pipeline/:workflowId (requires workflow)
26: * - /debug/:workflowId (requires workflow)
27: */
28: const VIEW_OPTIONS: ViewOption[] = [
29: {
30: id: 'kanban',
31: labelKey: 'viewSwitcher.kanban',
32: icon: LayoutGrid,
33: path: (workflowId) =>
34: workflowId ? `/board?workflowId=${encodeURIComponent(workflowId)}` : '/board',
35: requiresWorkflow: false,
36: },
37: {
38: id: 'pipeline',
39: labelKey: 'viewSwitcher.pipeline',
40: icon: GitBranch,
41: path: (workflowId) =>
42: workflowId ? `/pipeline/${encodeURIComponent(workflowId)}` : '/board',
43: requiresWorkflow: true,
44: },
45: {
46: id: 'debug',
47: labelKey: 'viewSwitcher.debug',
48: icon: Bug,
49: path: (workflowId) =>
50: workflowId ? `/debug/${encodeURIComponent(workflowId)}` : '/board',
51: requiresWorkflow: true,
52: },
53: ];
```

---

## 27. huanchong-99SoloDawnfrontend/src/components/tasks/TaskFollowUpSection.tsx

> 该文件共有 **1** 个问题

### 27.1 Use <details>, <fieldset>, <optgroup>, or <address> instead of the "group" role to ensure accessibility across all devices.

- **问题ID**: `AZybSJknEFps_QDQ6-pl`
- **项目**: huanchong-99
- **行号**: L9975
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 9975min effort
- **创建时间**: 14 hours ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import {
2: Loader2,
3: Send,
4: StopCircle,
5: AlertCircle,
6: Clock,
7: X,
8: Paperclip,
9: Terminal,
10: MessageSquare,
11: } from 'lucide-react';
12: import { Button } from '@/components/ui/button';
13: import { Alert, AlertDescription } from '@/components/ui/alert';
14: import {
15: DropdownMenu,
16: DropdownMenuContent,
17: DropdownMenuItem,
18: DropdownMenuTrigger,
19: } from '@/components/ui/dropdown-menu';
20: import {
21: Tooltip,
22: TooltipContent,
23: TooltipProvider,
24: TooltipTrigger,
25: } from '@/components/ui/tooltip';
26: //
27: import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
28: import {
29: ScratchType,
30: type TaskWithAttemptStatus,
31: type DraftFollowUpData,
32: ExecutorProfileId,
33: type QueueStatus,
34: type Session
35: } from 'shared/types';
36: import { useBranchStatus } from '@/hooks';
37: import { useAttemptRepo } from '@/hooks/useAttemptRepo';
38: import { useAttemptExecution } from '@/hooks/useAttemptExecution';
39: import { useUserSystem } from '@/components/ConfigProvider';
40: import { cn } from '@/lib/utils';
41: //
42: import { useReview } from '@/contexts/ReviewProvider';
43: import { useClickedElements } from '@/contexts/ClickedElementsProvider';
44: import { useEntries } from '@/contexts/EntriesContext';
45: import { useKeySubmitFollowUp, Scope } from '@/keyboard';
46: import { useHotkeysContext } from 'react-hotkeys-hook';
47: import { useProject } from '@/contexts/ProjectContext';
48: //
49: import { VariantSelector } from '@/components/tasks/VariantSelector';
50: import { useAttemptBranch } from '@/hooks/useAttemptBranch';
```

---

## 28. huanchong-99SoloDawnfrontend/src/components/tasks/Toolbar/GitOperations.tsx

> 该文件共有 **2** 个问题

### 28.1 Extract this nested ternary operation into an independent statement.

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

**问题代码片段**:
```
1: import {
2: ArrowRight,
3: GitBranch as GitBranchIcon,
4: GitPullRequest,
5: RefreshCw,
6: Settings,
7: AlertTriangle,
8: CheckCircle,
9: ExternalLink,
10: } from 'lucide-react';
11: import { Button } from '@/components/ui/button.tsx';
12: import {
13: Tooltip,
14: TooltipContent,
15: TooltipProvider,
16: TooltipTrigger,
17: } from '@/components/ui/tooltip.tsx';
18: import { useCallback, useMemo, useState } from 'react';
19: import type {
20: RepoBranchStatus,
21: Merge,
22: TaskWithAttemptStatus,
23: Workspace,
24: } from 'shared/types';
25: import { ChangeTargetBranchDialog } from '@/components/dialogs/tasks/ChangeTargetBranchDialog';
26: import RepoSelector from '@/components/tasks/RepoSelector';
27: import { RebaseDialog } from '@/components/dialogs/tasks/RebaseDialog';
28: import { CreatePRDialog } from '@/components/dialogs/tasks/CreatePRDialog';
29: import { useTranslation } from 'react-i18next';
30: import { useAttemptRepo } from '@/hooks/useAttemptRepo';
31: import { useGitOperations } from '@/hooks/useGitOperations';
32: import { useRepoBranches } from '@/hooks';
34: interface GitOperationsProps {
35: selectedAttempt: Workspace;
36: task: TaskWithAttemptStatus;
37: branchStatus: RepoBranchStatus[] | null;
38: branchStatusError?: Error | null;
39: isAttemptRunning: boolean;
40: selectedBranch: string | null;
41: layout?: 'horizontal' | 'vertical';
42: }
44: export type GitOperationsInputs = Omit<GitOperationsProps, 'selectedAttempt'>;
46: function GitOperations({
47: selectedAttempt,
48: task,
49: branchStatus,
50: branchStatusError,
51: isAttemptRunning,
52: selectedBranch,
53: layout = 'horizontal',
```

### 28.2 Extract this nested ternary operation into an independent statement.

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

**问题代码片段**:
```
1: import {
2: ArrowRight,
3: GitBranch as GitBranchIcon,
4: GitPullRequest,
5: RefreshCw,
6: Settings,
7: AlertTriangle,
8: CheckCircle,
9: ExternalLink,
10: } from 'lucide-react';
11: import { Button } from '@/components/ui/button.tsx';
12: import {
13: Tooltip,
14: TooltipContent,
15: TooltipProvider,
16: TooltipTrigger,
17: } from '@/components/ui/tooltip.tsx';
18: import { useCallback, useMemo, useState } from 'react';
19: import type {
20: RepoBranchStatus,
21: Merge,
22: TaskWithAttemptStatus,
23: Workspace,
24: } from 'shared/types';
25: import { ChangeTargetBranchDialog } from '@/components/dialogs/tasks/ChangeTargetBranchDialog';
26: import RepoSelector from '@/components/tasks/RepoSelector';
27: import { RebaseDialog } from '@/components/dialogs/tasks/RebaseDialog';
28: import { CreatePRDialog } from '@/components/dialogs/tasks/CreatePRDialog';
29: import { useTranslation } from 'react-i18next';
30: import { useAttemptRepo } from '@/hooks/useAttemptRepo';
31: import { useGitOperations } from '@/hooks/useGitOperations';
32: import { useRepoBranches } from '@/hooks';
34: interface GitOperationsProps {
35: selectedAttempt: Workspace;
36: task: TaskWithAttemptStatus;
37: branchStatus: RepoBranchStatus[] | null;
38: branchStatusError?: Error | null;
39: isAttemptRunning: boolean;
40: selectedBranch: string | null;
41: layout?: 'horizontal' | 'vertical';
42: }
44: export type GitOperationsInputs = Omit<GitOperationsProps, 'selectedAttempt'>;
46: function GitOperations({
47: selectedAttempt,
48: task,
49: branchStatus,
50: branchStatusError,
51: isAttemptRunning,
52: selectedBranch,
53: layout = 'horizontal',
```

---

## 29. huanchong-99SoloDawnfrontend/src/components/terminal/TerminalDebugView.tsx

> 该文件共有 **1** 个问题

### 29.1 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZyaDtVxyTAFHPcjycf9`
- **项目**: huanchong-99
- **行号**: L1335
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1335min effort
- **创建时间**: 20 hours ago
- **标签**: confusing

**问题代码片段**:
```
1: import { useState, useRef, useEffect, useCallback } from 'react';
2: import { TerminalEmulator, type TerminalEmulatorRef } from './TerminalEmulator';
3: import { Button } from '@/components/ui/button';
4: import { cn } from '@/lib/utils';
5: import type { Terminal } from '@/components/workflow/TerminalCard';
6: import type { WorkflowTask } from '@/components/workflow/PipelineView';
7: import { useTranslation } from 'react-i18next';
8: import { stripAnsi } from 'fancy-ansi';
10: interface Props {
11: tasks: (WorkflowTask & { terminals: Terminal[] })[];
12: wsUrl: string;
13: }
15: interface TerminalLogEntry {
16: id: string;
17: content: string;
18: }
20: interface TerminalHistoryState {
21: loading: boolean;
22: loaded: boolean;
23: lines: string[];
24: error: string | null;
25: }
27: const TERMINAL_HISTORY_LIMIT = 1000;
28: const CONTROL_CHARACTERS_REGEX = /[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g;
30: const sanitizeTerminalHistoryContent = (content: string) =>
31: stripAnsi(content)
32: .replaceAll('\r\n', '\n')
33: .replaceAll('\r', '\n')
34: .replaceAll(CONTROL_CHARACTERS_REGEX, '');
36: /**
37: * Renders the terminal debugging UI with a terminal list and active emulator.
38: */
39: export function TerminalDebugView({ tasks, wsUrl }: Readonly<Props>) {
40: const { t } = useTranslation('workflow');
41: const [selectedTerminalId, setSelectedTerminalId] = useState<string | null>(null);
42: const [historyByTerminalId, setHistoryByTerminalId] = useState<Record<string, TerminalHistoryState>>({});
43: const readyTerminalIdsRef = useRef<Set<string>>(new Set());
44: const startingTerminalIdsRef = useRef<Set<string>>(new Set());
45: const terminalRef = useRef<TerminalEmulatorRef>(null);
46: const autoStartedRef = useRef<Set<string>>(new Set());
47: const needsRestartRef = useRef<Set<string>>(new Set());
48: const restartAttemptsRef = useRef<Map<string, number>>(new Map());
49: const MAX_RESTART_ATTEMPTS = 3;
50: const defaultRoleLabel = t('terminalCard.defaultRole');
52: const allTerminals = tasks.flatMap((task) =>
53: task.terminals.map((terminal) => ({ ...terminal, taskName: task.name }))
54: );
56: const selectedTerminal = allTerminals.find((terminal) => terminal.id === selectedTerminalId);
58: const getTerminalLabel = (terminal: Terminal) => {
59: const role = terminal.role?.trim();
```

---

## 30. huanchong-99SoloDawnfrontend/src/components/ui-new/dialogs/RebaseDialog.tsx

> 该文件共有 **1** 个问题

### 30.1 'msg' will use Object's default stringification format ('[object Object]') when stringified.

- **问题ID**: `AZyaJqrWbTTkCKzmc8Mq`
- **项目**: huanchong-99
- **行号**: L565
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 565min effort
- **创建时间**: 20 hours ago
- **标签**: object, string, ...

**问题代码片段**:
```
1: import { useEffect, useState } from 'react';
2: import { CaretRightIcon } from '@phosphor-icons/react';
3: import { useTranslation } from 'react-i18next';
4: import {
5: Dialog,
6: DialogContent,
7: DialogDescription,
8: DialogFooter,
9: DialogHeader,
10: DialogTitle,
11: } from '@/components/ui/dialog';
12: import { Button } from '@/components/ui/button';
13: import BranchSelector from '@/components/tasks/BranchSelector';
14: import type { GitBranch, GitOperationError } from 'shared/types';
15: import NiceModal, { useModal } from '@ebay/nice-modal-react';
16: import { defineModal } from '@/lib/modals';
17: import { GitOperationsProvider } from '@/contexts/GitOperationsContext';
18: import { useGitOperations } from '@/hooks/useGitOperations';
19: import { useAttempt } from '@/hooks/useAttempt';
20: import { attemptsApi, type Result } from '@/lib/api';
21: import { ResolveConflictsDialog } from './ResolveConflictsDialog';
23: // Helper to extract error type from Result
24: function getErrorType(err: unknown): string | undefined {
25: const resultErr = err as Result<void, GitOperationError> | undefined;
26: if (resultErr && !resultErr.success) {
27: return resultErr.error?.type;
28: }
29: return undefined;
30: }
32: // Helper to check if error is a conflict error
33: function isConflictError(errorType: string | undefined): boolean {
34: return errorType === 'merge_conflicts' || errorType === 'rebase_in_progress';
35: }
37: // Helper to extract error message from various error structures
38: function extractErrorMessage(err: unknown): string {
39: if (!err || typeof err !== 'object') {
40: return 'Failed to rebase';
41: }
43: // Handle Result<void, GitOperationError> structure
44: if ('error' in err && err.error && typeof err.error === 'object' && 'message' in err.error) {
45: return String(err.error.message);
46: }
48: if ('message' in err && err.message) {
49: const msg = err.message;
50: if (typeof msg === 'string') {
51: return msg;
52: }
53: if (msg instanceof Error) {
54: return msg.message;
55: }
```

**错误示例 (Noncompliant)**:
```
class Foo {};
const foo = new Foo();

foo + ''; // Noncompliant - evaluates to "[object Object]"
`Foo: ${foo}`; // Noncompliant - evaluates to "Foo: [object Object]"
foo.toString(); // Noncompliant - evaluates to "[object Object]"
```

**正确示例 (Compliant)**:
```
class Foo {
  toString() {
    return 'Foo';
  }
}
const foo = new Foo();

foo + '';
`Foo: ${foo}`;
foo.toString();
```

---

## 31. huanchong-99SoloDawnfrontend/src/components/ui-new/hooks/usePreviewUrl.ts

> 该文件共有 **1** 个问题

### 31.1 Simplify this regular expression to reduce its complexity from 21 to the 20 allowed.

- **问题ID**: `AZybSJpCEFps_QDQ6-pq`
- **项目**: huanchong-99
- **行号**: L1310
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1310min effort
- **创建时间**: 14 hours ago
- **标签**: regex, type-dependent

**问题代码片段**:
```
1: import { useEffect, useRef, useState } from 'react';
2: import { stripAnsi } from 'fancy-ansi';
4: export interface PreviewUrlInfo {
5: url: string;
6: port?: number;
7: scheme: 'http' | 'https';
8: }
10: // Simplified regex patterns to reduce complexity
11: const urlPatterns = [
12: // Full URL pattern (e.g., http://localhost:3000, https://127.0.0.1:8080)
13: +1+1+2 (incl 1 for nesting)+1+1+1+1+2 (incl 1 for nesting)+3 (incl 2 for nesting)+2 (incl 1 for nesting)+2 (incl 1 for nesting)+1+2 (incl 1 for nesting)+1,
14: // Host:port pattern (e.g., localhost:3000, 0.0.0.0:8080)
15: /(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[[0-9a-f:]+\]|\d+(?:\.\d+){3}):(\d{2,5})/i,
16: ];
18: // Get the hostname from the current browser location, falling back to 'localhost'
19: const getBrowserHostname = (): string => {
20: if (globalThis.window !== undefined) {
21: return globalThis.window.location.hostname;
22: }
23: return 'localhost';
24: };
26: export const detectPreviewUrl = (line: string): PreviewUrlInfo | null => {
27: const cleaned = stripAnsi(line);
28: const browserHostname = getBrowserHostname();
30: // Try to match a full URL first
31: const fullUrlMatch = urlPatterns[0].exec(cleaned);
32: if (fullUrlMatch) {
33: try {
34: const parsed = new URL(fullUrlMatch[1]);
35: // Replace 0.0.0.0 or :: with browser hostname
36: if (
37: parsed.hostname === '0.0.0.0' ||
38: parsed.hostname === '::' ||
39: parsed.hostname === '[::]'
40: ) {
41: parsed.hostname = browserHostname;
42: }
43: return {
44: url: parsed.toString(),
45: port: parsed.port ? Number(parsed.port) : undefined,
46: scheme: parsed.protocol === 'https:' ? 'https' : 'http',
47: };
48: } catch {
49: // Ignore invalid URLs and fall through to host:port detection
50: }
51: }
53: // Try to match host:port pattern
54: const hostPortMatch = urlPatterns[1].exec(cleaned);
55: if (hostPortMatch) {
56: const port = Number(hostPortMatch[1]);
```

---

## 32. huanchong-99SoloDawnfrontend/src/components/ui-new/views/PreviewBrowser.tsx

> 该文件共有 **3** 个问题

### 32.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweiAZ9DOUQdEsGhW`
- **项目**: huanchong-99
- **行号**: L2465
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2465min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import type { RefObject } from 'react';
2: import {
3: PlayIcon,
4: SpinnerIcon,
5: WrenchIcon,
6: ArrowSquareOutIcon,
7: ArrowClockwiseIcon,
8: CopyIcon,
9: XIcon,
10: MonitorIcon,
11: DeviceMobileIcon,
12: ArrowsOutCardinalIcon,
13: PauseIcon,
14: } from '@phosphor-icons/react';
15: import { useTranslation } from 'react-i18next';
16: import { cn } from '@/lib/utils';
17: import { PrimaryButton } from '../primitives/PrimaryButton';
18: import {
19: IconButtonGroup,
20: IconButtonGroupItem,
21: } from '../primitives/IconButtonGroup';
22: import type { Repo } from 'shared/types';
23: import type {
24: ScreenSize,
25: ResponsiveDimensions,
26: } from '@/hooks/usePreviewSettings';
28: export const MOBILE_WIDTH = 390;
29: export const MOBILE_HEIGHT = 844;
30: // Phone frame adds padding (p-3 = 12px * 2) and rounded corners
31: export const PHONE_FRAME_PADDING = 24;
33: interface PreviewBrowserProps {
34: url?: string;
35: autoDetectedUrl?: string;
36: urlInputValue: string;
37: urlInputRef: RefObject<HTMLInputElement>;
38: isUsingOverride?: boolean;
39: onUrlInputChange: (value: string) => void;
40: onClearOverride?: () => void;
41: onCopyUrl: () => void;
42: onOpenInNewTab: () => void;
43: onRefresh: () => void;
44: onStart: () => void;
45: onStop: () => void;
46: isStarting: boolean;
47: isStopping: boolean;
48: isServerRunning: boolean;
49: screenSize: ScreenSize;
50: localDimensions: ResponsiveDimensions;
51: onScreenSizeChange: (size: ScreenSize) => void;
52: onResizeStart: (
```

**错误示例 (Noncompliant)**:
```
<div onClick={() => {}} />; // Noncompliant
```

**正确示例 (Compliant)**:
```
<div onClick={() => {}} role="button" />;
```

### 32.2 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweiAZ9DOUQdEsGhX`
- **项目**: huanchong-99
- **行号**: L2515
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2515min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import type { RefObject } from 'react';
2: import {
3: PlayIcon,
4: SpinnerIcon,
5: WrenchIcon,
6: ArrowSquareOutIcon,
7: ArrowClockwiseIcon,
8: CopyIcon,
9: XIcon,
10: MonitorIcon,
11: DeviceMobileIcon,
12: ArrowsOutCardinalIcon,
13: PauseIcon,
14: } from '@phosphor-icons/react';
15: import { useTranslation } from 'react-i18next';
16: import { cn } from '@/lib/utils';
17: import { PrimaryButton } from '../primitives/PrimaryButton';
18: import {
19: IconButtonGroup,
20: IconButtonGroupItem,
21: } from '../primitives/IconButtonGroup';
22: import type { Repo } from 'shared/types';
23: import type {
24: ScreenSize,
25: ResponsiveDimensions,
26: } from '@/hooks/usePreviewSettings';
28: export const MOBILE_WIDTH = 390;
29: export const MOBILE_HEIGHT = 844;
30: // Phone frame adds padding (p-3 = 12px * 2) and rounded corners
31: export const PHONE_FRAME_PADDING = 24;
33: interface PreviewBrowserProps {
34: url?: string;
35: autoDetectedUrl?: string;
36: urlInputValue: string;
37: urlInputRef: RefObject<HTMLInputElement>;
38: isUsingOverride?: boolean;
39: onUrlInputChange: (value: string) => void;
40: onClearOverride?: () => void;
41: onCopyUrl: () => void;
42: onOpenInNewTab: () => void;
43: onRefresh: () => void;
44: onStart: () => void;
45: onStop: () => void;
46: isStarting: boolean;
47: isStopping: boolean;
48: isServerRunning: boolean;
49: screenSize: ScreenSize;
50: localDimensions: ResponsiveDimensions;
51: onScreenSizeChange: (size: ScreenSize) => void;
52: onResizeStart: (
```

**错误示例 (Noncompliant)**:
```
<div onClick={() => {}} />; // Noncompliant
```

**正确示例 (Compliant)**:
```
<div onClick={() => {}} role="button" />;
```

### 32.3 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZyVweiAZ9DOUQdEsGhY`
- **项目**: huanchong-99
- **行号**: L2565
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2565min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import type { RefObject } from 'react';
2: import {
3: PlayIcon,
4: SpinnerIcon,
5: WrenchIcon,
6: ArrowSquareOutIcon,
7: ArrowClockwiseIcon,
8: CopyIcon,
9: XIcon,
10: MonitorIcon,
11: DeviceMobileIcon,
12: ArrowsOutCardinalIcon,
13: PauseIcon,
14: } from '@phosphor-icons/react';
15: import { useTranslation } from 'react-i18next';
16: import { cn } from '@/lib/utils';
17: import { PrimaryButton } from '../primitives/PrimaryButton';
18: import {
19: IconButtonGroup,
20: IconButtonGroupItem,
21: } from '../primitives/IconButtonGroup';
22: import type { Repo } from 'shared/types';
23: import type {
24: ScreenSize,
25: ResponsiveDimensions,
26: } from '@/hooks/usePreviewSettings';
28: export const MOBILE_WIDTH = 390;
29: export const MOBILE_HEIGHT = 844;
30: // Phone frame adds padding (p-3 = 12px * 2) and rounded corners
31: export const PHONE_FRAME_PADDING = 24;
33: interface PreviewBrowserProps {
34: url?: string;
35: autoDetectedUrl?: string;
36: urlInputValue: string;
37: urlInputRef: RefObject<HTMLInputElement>;
38: isUsingOverride?: boolean;
39: onUrlInputChange: (value: string) => void;
40: onClearOverride?: () => void;
41: onCopyUrl: () => void;
42: onOpenInNewTab: () => void;
43: onRefresh: () => void;
44: onStart: () => void;
45: onStop: () => void;
46: isStarting: boolean;
47: isStopping: boolean;
48: isServerRunning: boolean;
49: screenSize: ScreenSize;
50: localDimensions: ResponsiveDimensions;
51: onScreenSizeChange: (size: ScreenSize) => void;
52: onResizeStart: (
```

**错误示例 (Noncompliant)**:
```
<div onClick={() => {}} />; // Noncompliant
```

**正确示例 (Compliant)**:
```
<div onClick={() => {}} role="button" />;
```

---

## 33. huanchong-99SoloDawnfrontend/src/contexts/EntriesContext.tsx

> 该文件共有 **1** 个问题

### 33.1 useState call is not destructured into value + setter pair

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

**问题代码片段**:
```
1: import {
2: createContext,
3: useContext,
4: useState,
5: useMemo,
6: useCallback,
7: ReactNode,
8: } from 'react';
9: import type { PatchTypeWithKey } from '@/hooks/useConversationHistory';
11: interface EntriesContextType {
12: entries: PatchTypeWithKey[];
13: setEntries: (entries: PatchTypeWithKey[]) => void;
14: reset: () => void;
15: }
17: const EntriesContext = createContext<EntriesContextType | null>(null);
19: interface EntriesProviderProps {
20: children: ReactNode;
21: }
23: export const EntriesProvider = ({ children }: EntriesProviderProps) => {
24: const = useState<PatchTypeWithKey[]>([]);
26: const setEntries = useCallback((newEntries: PatchTypeWithKey[]) => {
27: setEntriesState(newEntries);
28: }, []);
30: const reset = useCallback(() => {
31: setEntriesState([]);
32: }, []);
34: const value = useMemo(
35: () => ({
36: entries,
37: setEntries,
38: reset,
39: }),
40: [entries, setEntries, reset]
41: );
43: return (
44: <EntriesContext.Provider value={value}>{children}</EntriesContext.Provider>
45: );
46: };
48: export const useEntries = (): EntriesContextType => {
49: const context = useContext(EntriesContext);
50: if (!context) {
51: throw new Error('useEntries must be used within an EntriesProvider');
52: }
53: return context;
54: };
```

---

## 34. huanchong-99SoloDawnfrontend/src/hooks/useCommandBarShortcut.ts

> 该文件共有 **1** 个问题

### 34.1 'platform' is deprecated.

- **问题ID**: `AZybSJtDEFps_QDQ6-pz`
- **项目**: huanchong-99
- **行号**: L1615
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1615min effort
- **创建时间**: 14 hours ago
- **标签**: cwe, obsolete, ...

**问题代码片段**:
```
1: import { useEffect, useCallback } from 'react';
3: /**
4: * Hook that listens for CMD+K (Mac) or Ctrl+K (Windows/Linux) to open the command bar.
5: * Uses native DOM event listener with capture phase to intercept before other handlers
6: * like Lexical editor.
7: */
8: export function useCommandBarShortcut(
9: onOpen: () => void,
10: enabled: boolean = true
11: ) {
12: const handleKeyDown = useCallback(
13: (event: KeyboardEvent) => {
14: // CMD+K (Mac) or Ctrl+K (Windows/Linux)
15: // eslint-disable-next-line @typescript-eslint/no-deprecated -- fallback for browsers without userAgentData
16: const platform: string = (navigator as any).userAgentData?.platform ?? navigator.;
17: const isMac = platform.toUpperCase().includes('MAC');
18: const modifier = isMac ? event.metaKey : event.ctrlKey;
20: if (modifier && event.key.toLowerCase() === 'k') {
21: event.preventDefault();
22: event.stopPropagation();
23: onOpen();
24: }
25: },
26: [onOpen]
27: );
29: useEffect(() => {
30: if (!enabled) return;
32: // Use capture phase to intercept before other handlers (like Lexical editor)
33: globalThis.addEventListener('keydown', handleKeyDown, { capture: true });
35: return () => {
36: globalThis.removeEventListener('keydown', handleKeyDown, { capture: true });
37: };
38: }, [handleKeyDown, enabled]);
39: }
```

---

## 35. huanchong-99SoloDawnfrontend/src/hooks/useConversationHistory.ts

> 该文件共有 **3** 个问题

### 35.1 Extract this nested ternary operation into an independent statement.

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

**问题代码片段**:
```
1: // useConversationHistory.ts
2: import {
3: CommandExitStatus,
4: ExecutionProcess,
5: ExecutionProcessStatus,
6: ExecutorAction,
7: NormalizedEntry,
8: PatchType,
9: ToolStatus,
10: Workspace,
11: } from 'shared/types';
12: import { useExecutionProcessesContext } from '@/contexts/ExecutionProcessesContext';
13: import { useCallback, useEffect, useMemo, useRef } from 'react';
14: import { streamJsonPatchEntries } from '@/utils/streamJsonPatchEntries';
16: export type PatchTypeWithKey = PatchType & {
17: patchKey: string;
18: executionProcessId: string;
19: };
21: export type AddEntryType = 'initial' | 'running' | 'historic' | 'plan';
23: export type OnEntriesUpdated = (
24: newEntries: PatchTypeWithKey[],
25: addType: AddEntryType,
26: loading: boolean
27: ) => void;
29: type ExecutionProcessStaticInfo = {
30: id: string;
31: createdAt: string;
32: updatedAt: string;
33: executorAction: ExecutorAction;
34: };
36: type ExecutionProcessState = {
37: executionProcess: ExecutionProcessStaticInfo;
38: entries: PatchTypeWithKey[];
39: };
41: type ExecutionProcessStateStore = Record<string, ExecutionProcessState>;
43: interface UseConversationHistoryParams {
44: attempt: Workspace;
45: onEntriesUpdated: OnEntriesUpdated;
46: }
48: interface UseConversationHistoryResult {}
50: const MIN_INITIAL_ENTRIES = 10;
51: const REMAINING_BATCH_SIZE = 50;
53: const makeLoadingPatch = (executionProcessId: string): PatchTypeWithKey => ({
54: type: 'NORMALIZED_ENTRY',
55: content: {
56: entry_type: {
57: type: 'loading',
58: },
59: content: '',
60: timestamp: null,
```

### 35.2 Refactor this code to not nest functions more than 4 levels deep.

- **问题ID**: `AZyVwewpZ9DOUQdEsGl9`
- **项目**: huanchong-99
- **行号**: L53420
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 53420min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

**问题代码片段**:
```
1: // useConversationHistory.ts
2: import {
3: CommandExitStatus,
4: ExecutionProcess,
5: ExecutionProcessStatus,
6: ExecutorAction,
7: NormalizedEntry,
8: PatchType,
9: ToolStatus,
10: Workspace,
11: } from 'shared/types';
12: import { useExecutionProcessesContext } from '@/contexts/ExecutionProcessesContext';
13: import { useCallback, useEffect, useMemo, useRef } from 'react';
14: import { streamJsonPatchEntries } from '@/utils/streamJsonPatchEntries';
16: export type PatchTypeWithKey = PatchType & {
17: patchKey: string;
18: executionProcessId: string;
19: };
21: export type AddEntryType = 'initial' | 'running' | 'historic' | 'plan';
23: export type OnEntriesUpdated = (
24: newEntries: PatchTypeWithKey[],
25: addType: AddEntryType,
26: loading: boolean
27: ) => void;
29: type ExecutionProcessStaticInfo = {
30: id: string;
31: createdAt: string;
32: updatedAt: string;
33: executorAction: ExecutorAction;
34: };
36: type ExecutionProcessState = {
37: executionProcess: ExecutionProcessStaticInfo;
38: entries: PatchTypeWithKey[];
39: };
41: type ExecutionProcessStateStore = Record<string, ExecutionProcessState>;
43: interface UseConversationHistoryParams {
44: attempt: Workspace;
45: onEntriesUpdated: OnEntriesUpdated;
46: }
48: interface UseConversationHistoryResult {}
50: const MIN_INITIAL_ENTRIES = 10;
51: const REMAINING_BATCH_SIZE = 50;
53: const makeLoadingPatch = (executionProcessId: string): PatchTypeWithKey => ({
54: type: 'NORMALIZED_ENTRY',
55: content: {
56: entry_type: {
57: type: 'loading',
58: },
59: content: '',
60: timestamp: null,
```

### 35.3 Refactor this code to not nest functions more than 4 levels deep.

- **问题ID**: `AZyVwewpZ9DOUQdEsGl-`
- **项目**: huanchong-99
- **行号**: L53720
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 53720min effort
- **创建时间**: 1 month ago
- **标签**: brain-overload

**问题代码片段**:
```
1: // useConversationHistory.ts
2: import {
3: CommandExitStatus,
4: ExecutionProcess,
5: ExecutionProcessStatus,
6: ExecutorAction,
7: NormalizedEntry,
8: PatchType,
9: ToolStatus,
10: Workspace,
11: } from 'shared/types';
12: import { useExecutionProcessesContext } from '@/contexts/ExecutionProcessesContext';
13: import { useCallback, useEffect, useMemo, useRef } from 'react';
14: import { streamJsonPatchEntries } from '@/utils/streamJsonPatchEntries';
16: export type PatchTypeWithKey = PatchType & {
17: patchKey: string;
18: executionProcessId: string;
19: };
21: export type AddEntryType = 'initial' | 'running' | 'historic' | 'plan';
23: export type OnEntriesUpdated = (
24: newEntries: PatchTypeWithKey[],
25: addType: AddEntryType,
26: loading: boolean
27: ) => void;
29: type ExecutionProcessStaticInfo = {
30: id: string;
31: createdAt: string;
32: updatedAt: string;
33: executorAction: ExecutorAction;
34: };
36: type ExecutionProcessState = {
37: executionProcess: ExecutionProcessStaticInfo;
38: entries: PatchTypeWithKey[];
39: };
41: type ExecutionProcessStateStore = Record<string, ExecutionProcessState>;
43: interface UseConversationHistoryParams {
44: attempt: Workspace;
45: onEntriesUpdated: OnEntriesUpdated;
46: }
48: interface UseConversationHistoryResult {}
50: const MIN_INITIAL_ENTRIES = 10;
51: const REMAINING_BATCH_SIZE = 50;
53: const makeLoadingPatch = (executionProcessId: string): PatchTypeWithKey => ({
54: type: 'NORMALIZED_ENTRY',
55: content: {
56: entry_type: {
57: type: 'loading',
58: },
59: content: '',
60: timestamp: null,
```

---

## 36. huanchong-99SoloDawnfrontend/src/hooks/useDevserverUrl.ts

> 该文件共有 **1** 个问题

### 36.1 Simplify this regular expression to reduce its complexity from 21 to the 20 allowed.

- **问题ID**: `AZybSJthEFps_QDQ6-p0`
- **项目**: huanchong-99
- **行号**: L710
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 710min effort
- **创建时间**: 14 hours ago
- **标签**: regex, type-dependent

**问题代码片段**:
```
1: import { useEffect, useRef, useState } from 'react';
2: import { stripAnsi } from 'fancy-ansi';
4: // Simplified regex patterns to reduce complexity
5: const urlPatterns = [
6: // Match full URLs with various hostname formats
7: +1+1+2 (incl 1 for nesting)+1+1+1+1+2 (incl 1 for nesting)+3 (incl 2 for nesting)+2 (incl 1 for nesting)+2 (incl 1 for nesting)+1+2 (incl 1 for nesting)+1,
8: // Match host:port patterns
9: /(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[[0-9a-f:]+\]|\d+(?:\.\d+){3}):(\d{2,5})/i,
10: ];
12: export type DevserverUrlInfo = {
13: url: string;
14: port?: number;
15: scheme: 'http' | 'https';
16: };
18: // Get the hostname from the current browser location, falling back to 'localhost'
19: const getBrowserHostname = (): string => {
20: if (globalThis.window !== undefined) {
21: return globalThis.window.location.hostname;
22: }
23: return 'localhost';
24: };
26: export const detectDevserverUrl = (line: string): DevserverUrlInfo | null => {
27: const cleaned = stripAnsi(line);
28: const browserHostname = getBrowserHostname();
30: const fullUrlMatch = urlPatterns[0].exec(cleaned);
31: if (fullUrlMatch) {
32: try {
33: const parsed = new URL(fullUrlMatch[1]);
34: if (
35: parsed.hostname === '0.0.0.0' ||
36: parsed.hostname === '::' ||
37: parsed.hostname === '[::]'
38: ) {
39: parsed.hostname = browserHostname;
40: }
41: return {
42: url: parsed.toString(),
43: port: parsed.port ? Number(parsed.port) : undefined,
44: scheme: parsed.protocol === 'https:' ? 'https' : 'http',
45: };
46: } catch {
47: // Ignore invalid URLs and fall through to host:port detection.
48: }
49: }
51: const hostPortMatch = urlPatterns[1].exec(cleaned);
52: if (hostPortMatch) {
53: const port = Number(hostPortMatch[1]);
54: const scheme = /https/i.test(cleaned) ? 'https' : 'http';
55: return {
56: url: `${scheme}://${browserHostname}:${port}`,
```

---

## 37. huanchong-99SoloDawnfrontend/src/hooks/useProjectTasks.ts

> 该文件共有 **2** 个问题

### 37.1 This assertion is unnecessary since it does not change the type of the expression.

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

**问题代码片段**:
```
1: import { useCallback, useMemo } from 'react';
2: import { useJsonPatchWsStream } from './useJsonPatchWsStream';
3: import { useAuth } from '@/hooks';
4: import { useProject } from '@/contexts/ProjectContext';
5: import { useUserSystem } from '@/components/ConfigProvider';
6: import { useLiveQuery, eq, isNull } from '@tanstack/react-db';
7: import { sharedTasksCollection } from '@/lib/electric/sharedTasksCollection';
8: import { useAssigneeUserNames } from './useAssigneeUserName';
9: import { useAutoLinkSharedTasks } from './useAutoLinkSharedTasks';
10: import type {
11: SharedTask,
12: TaskStatus,
13: TaskWithAttemptStatus,
14: } from 'shared/types';
16: export type SharedTaskRecord = SharedTask & {
17: remote_project_id: string;
18: assignee_first_name?: string | null;
19: assignee_last_name?: string | null;
20: assignee_username?: string | null;
21: };
23: type TasksState = {
24: tasks: Record<string, TaskWithAttemptStatus>;
25: };
27: export interface UseProjectTasksResult {
28: tasks: TaskWithAttemptStatus[];
29: tasksById: Record<string, TaskWithAttemptStatus>;
30: tasksByStatus: Record<TaskStatus, TaskWithAttemptStatus[]>;
31: sharedTasksById: Record<string, SharedTaskRecord>;
32: sharedOnlyByStatus: Record<TaskStatus, SharedTaskRecord[]>;
33: isLoading: boolean;
34: isConnected: boolean;
35: error: string | null;
36: }
38: /**
39: * Stream tasks for a project via WebSocket (JSON Patch) and expose as array + map.
40: * Server sends initial snapshot: replace /tasks with an object keyed by id.
41: * Live updates arrive at /tasks/<id> via add/replace/remove operations.
42: */
43: export const useProjectTasks = (projectId: string): UseProjectTasksResult => {
44: const { project } = useProject();
45: const { isSignedIn } = useAuth();
46: const { remoteFeaturesEnabled } = useUserSystem();
47: const remoteProjectId = project?.remoteProjectId;
48: // Remote shared-task APIs are currently disabled server-side; keep this feature
49: // behind an explicit opt-in so clients don't subscribe to a removed shape.
50: const sharedTasksFeatureEnabled =
51: import.meta.env.VITE_ENABLE_SHARED_TASKS === 'true';
52: const sharedTasksEnabled =
53: sharedTasksFeatureEnabled &&
54: remoteFeaturesEnabled &&
```

### 37.2 This assertion is unnecessary since it does not change the type of the expression.

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

**问题代码片段**:
```
1: import { useCallback, useMemo } from 'react';
2: import { useJsonPatchWsStream } from './useJsonPatchWsStream';
3: import { useAuth } from '@/hooks';
4: import { useProject } from '@/contexts/ProjectContext';
5: import { useUserSystem } from '@/components/ConfigProvider';
6: import { useLiveQuery, eq, isNull } from '@tanstack/react-db';
7: import { sharedTasksCollection } from '@/lib/electric/sharedTasksCollection';
8: import { useAssigneeUserNames } from './useAssigneeUserName';
9: import { useAutoLinkSharedTasks } from './useAutoLinkSharedTasks';
10: import type {
11: SharedTask,
12: TaskStatus,
13: TaskWithAttemptStatus,
14: } from 'shared/types';
16: export type SharedTaskRecord = SharedTask & {
17: remote_project_id: string;
18: assignee_first_name?: string | null;
19: assignee_last_name?: string | null;
20: assignee_username?: string | null;
21: };
23: type TasksState = {
24: tasks: Record<string, TaskWithAttemptStatus>;
25: };
27: export interface UseProjectTasksResult {
28: tasks: TaskWithAttemptStatus[];
29: tasksById: Record<string, TaskWithAttemptStatus>;
30: tasksByStatus: Record<TaskStatus, TaskWithAttemptStatus[]>;
31: sharedTasksById: Record<string, SharedTaskRecord>;
32: sharedOnlyByStatus: Record<TaskStatus, SharedTaskRecord[]>;
33: isLoading: boolean;
34: isConnected: boolean;
35: error: string | null;
36: }
38: /**
39: * Stream tasks for a project via WebSocket (JSON Patch) and expose as array + map.
40: * Server sends initial snapshot: replace /tasks with an object keyed by id.
41: * Live updates arrive at /tasks/<id> via add/replace/remove operations.
42: */
43: export const useProjectTasks = (projectId: string): UseProjectTasksResult => {
44: const { project } = useProject();
45: const { isSignedIn } = useAuth();
46: const { remoteFeaturesEnabled } = useUserSystem();
47: const remoteProjectId = project?.remoteProjectId;
48: // Remote shared-task APIs are currently disabled server-side; keep this feature
49: // behind an explicit opt-in so clients don't subscribe to a removed shape.
50: const sharedTasksFeatureEnabled =
51: import.meta.env.VITE_ENABLE_SHARED_TASKS === 'true';
52: const sharedTasksEnabled =
53: sharedTasksFeatureEnabled &&
54: remoteFeaturesEnabled &&
```

---

## 38. huanchong-99SoloDawnfrontend/src/hooks/useTodos.ts

> 该文件共有 **3** 个问题

### 38.1 Complete the task associated to this "TODO" comment.

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

**问题代码片段**:
```
1: import { useMemo } from 'react';
2: import type { TodoItem } from 'shared/types';
3: import type { PatchTypeWithKey } from '@/hooks/useConversationHistory';
5: interface UseTodosResult {
6: todos: TodoItem[];
7: inProgressTodo: TodoItem | null;
8: lastUpdated: string | null;
9: }
11: /**
12: * Hook that extracts and maintains the latest state from normalized conversation entries.
13: * Filters for TodoManagement ActionType entries and returns the most recent todo list,
14: * along with the currently in-progress item.
15: */
16: export const useTodos = (entries: PatchTypeWithKey[]): UseTodosResult => {
17: return useMemo(() => {
18: let latestTodos: TodoItem[] = [];
19: let lastUpdatedTime: string | null = null;
21: for (const entry of entries) {
22: if (entry.type === 'NORMALIZED_ENTRY' && entry.content) {
23: const normalizedEntry = entry.content;
25: if (
26: normalizedEntry.entry_type?.type === 'tool_use' &&
27: normalizedEntry.entry_type?.action_type?.action === 'todo_management'
28: ) {
29: const actionType = normalizedEntry.entry_type.action_type;
30: const partialTodos = actionType.todos || [];
31: const currentTimestamp = normalizedEntry.timestamp;
33: // Only update latestTodos if we have meaningful content OR this is our first entry
34: const hasMeaningfulTodos =
35: partialTodos.length > 0 &&
36: partialTodos.every(
37: (todo: TodoItem) =>
38: todo.content && todo.content.trim().length > 0 && todo.status
39: );
40: const isNewerThanLatest =
41: !lastUpdatedTime ||
42: (!!currentTimestamp && currentTimestamp >= lastUpdatedTime);
44: if (
45: hasMeaningfulTodos ||
46: (isNewerThanLatest && latestTodos.length === 0)
47: ) {
48: latestTodos = partialTodos;
49: lastUpdatedTime = currentTimestamp;
50: }
51: }
52: }
53: }
55: // Find the currently in-progress
56: const inProgressTodo =
57: latestTodos.find((todo) => {
```

### 38.2 Complete the task associated to this "TODO" comment.

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

**问题代码片段**:
```
1: import { useMemo } from 'react';
2: import type { TodoItem } from 'shared/types';
3: import type { PatchTypeWithKey } from '@/hooks/useConversationHistory';
5: interface UseTodosResult {
6: todos: TodoItem[];
7: inProgressTodo: TodoItem | null;
8: lastUpdated: string | null;
9: }
11: /**
12: * Hook that extracts and maintains the latest state from normalized conversation entries.
13: * Filters for TodoManagement ActionType entries and returns the most recent todo list,
14: * along with the currently in-progress item.
15: */
16: export const useTodos = (entries: PatchTypeWithKey[]): UseTodosResult => {
17: return useMemo(() => {
18: let latestTodos: TodoItem[] = [];
19: let lastUpdatedTime: string | null = null;
21: for (const entry of entries) {
22: if (entry.type === 'NORMALIZED_ENTRY' && entry.content) {
23: const normalizedEntry = entry.content;
25: if (
26: normalizedEntry.entry_type?.type === 'tool_use' &&
27: normalizedEntry.entry_type?.action_type?.action === 'todo_management'
28: ) {
29: const actionType = normalizedEntry.entry_type.action_type;
30: const partialTodos = actionType.todos || [];
31: const currentTimestamp = normalizedEntry.timestamp;
33: // Only update latestTodos if we have meaningful content OR this is our first entry
34: const hasMeaningfulTodos =
35: partialTodos.length > 0 &&
36: partialTodos.every(
37: (todo: TodoItem) =>
38: todo.content && todo.content.trim().length > 0 && todo.status
39: );
40: const isNewerThanLatest =
41: !lastUpdatedTime ||
42: (!!currentTimestamp && currentTimestamp >= lastUpdatedTime);
44: if (
45: hasMeaningfulTodos ||
46: (isNewerThanLatest && latestTodos.length === 0)
47: ) {
48: latestTodos = partialTodos;
49: lastUpdatedTime = currentTimestamp;
50: }
51: }
52: }
53: }
55: // Find the currently in-progress
56: const inProgressTodo =
57: latestTodos.find((todo) => {
```

### 38.3 Complete the task associated to this "TODO" comment.

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

**问题代码片段**:
```
1: import { useMemo } from 'react';
2: import type { TodoItem } from 'shared/types';
3: import type { PatchTypeWithKey } from '@/hooks/useConversationHistory';
5: interface UseTodosResult {
6: todos: TodoItem[];
7: inProgressTodo: TodoItem | null;
8: lastUpdated: string | null;
9: }
11: /**
12: * Hook that extracts and maintains the latest state from normalized conversation entries.
13: * Filters for TodoManagement ActionType entries and returns the most recent todo list,
14: * along with the currently in-progress item.
15: */
16: export const useTodos = (entries: PatchTypeWithKey[]): UseTodosResult => {
17: return useMemo(() => {
18: let latestTodos: TodoItem[] = [];
19: let lastUpdatedTime: string | null = null;
21: for (const entry of entries) {
22: if (entry.type === 'NORMALIZED_ENTRY' && entry.content) {
23: const normalizedEntry = entry.content;
25: if (
26: normalizedEntry.entry_type?.type === 'tool_use' &&
27: normalizedEntry.entry_type?.action_type?.action === 'todo_management'
28: ) {
29: const actionType = normalizedEntry.entry_type.action_type;
30: const partialTodos = actionType.todos || [];
31: const currentTimestamp = normalizedEntry.timestamp;
33: // Only update latestTodos if we have meaningful content OR this is our first entry
34: const hasMeaningfulTodos =
35: partialTodos.length > 0 &&
36: partialTodos.every(
37: (todo: TodoItem) =>
38: todo.content && todo.content.trim().length > 0 && todo.status
39: );
40: const isNewerThanLatest =
41: !lastUpdatedTime ||
42: (!!currentTimestamp && currentTimestamp >= lastUpdatedTime);
44: if (
45: hasMeaningfulTodos ||
46: (isNewerThanLatest && latestTodos.length === 0)
47: ) {
48: latestTodos = partialTodos;
49: lastUpdatedTime = currentTimestamp;
50: }
51: }
52: }
53: }
55: // Find the currently in-progress
56: const inProgressTodo =
57: latestTodos.find((todo) => {
```

---

## 39. huanchong-99SoloDawnfrontend/src/hooks/useVariant.ts

> 该文件共有 **1** 个问题

### 39.1 useState call is not destructured into value + setter pair

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

**问题代码片段**:
```
1: import { useCallback, useEffect, useRef, useState } from 'react';
3: type Args = {
4: processVariant: string | null;
5: scratchVariant?: string | null;
6: };
8: /**
9: * Hook to manage variant selection with priority:
10: * 1. User dropdown selection (current session) - highest priority
11: * 2. Scratch-persisted variant (from previous session)
12: * 3. Last execution process variant (fallback)
13: */
14: export function useVariant({ processVariant, scratchVariant }: Args) {
15: // Track if user has explicitly selected a variant this session
16: const hasUserSelectionRef = useRef(false);
18: // Compute initial value: scratch takes priority over process
19: const getInitialVariant = () =>
20: scratchVariant === undefined ? processVariant : scratchVariant;
22: const = useState<string | null>(
23: getInitialVariant
24: );
26: // Sync state when inputs change (if user hasn't made a selection)
27: useEffect(() => {
28: if (hasUserSelectionRef.current) return;
30: const newVariant =
31: scratchVariant === undefined ? processVariant : scratchVariant;
32: setSelectedVariantState(newVariant);
33: }, [scratchVariant, processVariant]);
35: // When user explicitly selects a variant, mark it and update state
36: const setSelectedVariant = useCallback((variant: string | null) => {
37: hasUserSelectionRef.current = true;
38: setSelectedVariantState(variant);
39: }, []);
41: return { selectedVariant, setSelectedVariant } as const;
42: }
```

---

## 40. huanchong-99SoloDawnfrontend/src/pages/Workflows.tsx

> 该文件共有 **7** 个问题

### 40.1 Refactor this function to reduce its Cognitive Complexity from 19 to the 15 allowed.

- **问题ID**: `AZyVwetGZ9DOUQdEsGkd`
- **项目**: huanchong-99
- **行号**: L2189
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 2189min effort
- **创建时间**: 18 days ago
- **标签**: brain-overload

**问题代码片段**:
```
1: import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
2: import { useQueryClient } from '@tanstack/react-query';
3: import { useNavigate, useSearchParams } from 'react-router-dom';
4: import { Button } from '@/components/ui/button';
5: import { Card, CardContent } from '@/components/ui/card';
6: import {
7: Select,
8: SelectContent,
9: SelectItem,
10: SelectTrigger,
11: SelectValue,
12: } from '@/components/ui/select';
13: import {
14: Plus,
15: Play,
16: Pause,
17: Square,
18: Trash2,
19: Rocket,
20: GitMerge,
21: } from 'lucide-react';
22: import { Loader } from '@/components/ui/loader';
23: import {
24: useWorkflows,
25: useCreateWorkflow,
26: usePrepareWorkflow,
27: useStartWorkflow,
28: usePauseWorkflow,
29: useStopWorkflow,
30: useMergeWorkflow,
31: useDeleteWorkflow,
32: useWorkflow,
33: workflowKeys,
34: getWorkflowActions,
35: useSubmitWorkflowPromptResponse,
36: } from '@/hooks/useWorkflows';
37: import { useProjects } from '@/hooks/useProjects';
38: import type { WorkflowTaskDto } from 'shared/types';
39: import { WorkflowWizard } from '@/components/workflow/WorkflowWizard';
40: import {
41: PipelineView,
42: type WorkflowStatus,
43: type WorkflowTask,
44: } from '@/components/workflow/PipelineView';
45: import { WizardConfig, wizardConfigToCreateRequest } from '@/components/workflow/types';
46: import type { TerminalStatus } from '@/components/workflow/TerminalCard';
47: import { cn } from '@/lib/utils';
48: import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
49: import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
50: import { useTranslation } from 'react-i18next';
```

**错误示例 (Noncompliant)**:
```
function calculateFinalPrice(user, cart) {
  let total = calculateTotal(cart);
  if (user.hasMembership                       // +1 (if)
    && user.orders > 10                        // +1 (more than one condition)
    && user.accountActive
    && !user.hasDiscount
    || user.orders === 1) {                    // +1 (change of operator in condition)
      total = applyDiscount(user, total);
  }
  return total;
}
```

**正确示例 (Compliant)**:
```
function calculateFinalPrice(user, cart) {
  let total = calculateTotal(cart);
  if (isEligibleForDiscount(user)) {       // +1 (if)
    total = applyDiscount(user, total);
  }
  return total;
}

function isEligibleForDiscount(user) {
  return user.hasMembership
    && user.orders > 10                     // +1 (more than one condition)
    && user.accountActive
    && !user.hasDiscount
    || user.orders === 1                    // +1 (change of operator in condition)
}
```

### 40.2 Promise-returning function provided to property where a void return was expected.

- **问题ID**: `AZybSJrDEFps_QDQ6-ps`
- **项目**: huanchong-99
- **行号**: L8795
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 8795min effort
- **创建时间**: 14 hours ago
- **标签**: async, promise, ...

**问题代码片段**:
```
1: import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
2: import { useQueryClient } from '@tanstack/react-query';
3: import { useNavigate, useSearchParams } from 'react-router-dom';
4: import { Button } from '@/components/ui/button';
5: import { Card, CardContent } from '@/components/ui/card';
6: import {
7: Select,
8: SelectContent,
9: SelectItem,
10: SelectTrigger,
11: SelectValue,
12: } from '@/components/ui/select';
13: import {
14: Plus,
15: Play,
16: Pause,
17: Square,
18: Trash2,
19: Rocket,
20: GitMerge,
21: } from 'lucide-react';
22: import { Loader } from '@/components/ui/loader';
23: import {
24: useWorkflows,
25: useCreateWorkflow,
26: usePrepareWorkflow,
27: useStartWorkflow,
28: usePauseWorkflow,
29: useStopWorkflow,
30: useMergeWorkflow,
31: useDeleteWorkflow,
32: useWorkflow,
33: workflowKeys,
34: getWorkflowActions,
35: useSubmitWorkflowPromptResponse,
36: } from '@/hooks/useWorkflows';
37: import { useProjects } from '@/hooks/useProjects';
38: import type { WorkflowTaskDto } from 'shared/types';
39: import { WorkflowWizard } from '@/components/workflow/WorkflowWizard';
40: import {
41: PipelineView,
42: type WorkflowStatus,
43: type WorkflowTask,
44: } from '@/components/workflow/PipelineView';
45: import { WizardConfig, wizardConfigToCreateRequest } from '@/components/workflow/types';
46: import type { TerminalStatus } from '@/components/workflow/TerminalCard';
47: import { cn } from '@/lib/utils';
48: import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
49: import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
50: import { useTranslation } from 'react-i18next';
```

**错误示例 (Noncompliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (promise) {
  // ...
}
```

**正确示例 (Compliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (await promise) {
  // ...
}
```

### 40.3 Promise-returning function provided to property where a void return was expected.

- **问题ID**: `AZybSJrDEFps_QDQ6-pt`
- **项目**: huanchong-99
- **行号**: L8805
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 8805min effort
- **创建时间**: 14 hours ago
- **标签**: async, promise, ...

**问题代码片段**:
```
1: import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
2: import { useQueryClient } from '@tanstack/react-query';
3: import { useNavigate, useSearchParams } from 'react-router-dom';
4: import { Button } from '@/components/ui/button';
5: import { Card, CardContent } from '@/components/ui/card';
6: import {
7: Select,
8: SelectContent,
9: SelectItem,
10: SelectTrigger,
11: SelectValue,
12: } from '@/components/ui/select';
13: import {
14: Plus,
15: Play,
16: Pause,
17: Square,
18: Trash2,
19: Rocket,
20: GitMerge,
21: } from 'lucide-react';
22: import { Loader } from '@/components/ui/loader';
23: import {
24: useWorkflows,
25: useCreateWorkflow,
26: usePrepareWorkflow,
27: useStartWorkflow,
28: usePauseWorkflow,
29: useStopWorkflow,
30: useMergeWorkflow,
31: useDeleteWorkflow,
32: useWorkflow,
33: workflowKeys,
34: getWorkflowActions,
35: useSubmitWorkflowPromptResponse,
36: } from '@/hooks/useWorkflows';
37: import { useProjects } from '@/hooks/useProjects';
38: import type { WorkflowTaskDto } from 'shared/types';
39: import { WorkflowWizard } from '@/components/workflow/WorkflowWizard';
40: import {
41: PipelineView,
42: type WorkflowStatus,
43: type WorkflowTask,
44: } from '@/components/workflow/PipelineView';
45: import { WizardConfig, wizardConfigToCreateRequest } from '@/components/workflow/types';
46: import type { TerminalStatus } from '@/components/workflow/TerminalCard';
47: import { cn } from '@/lib/utils';
48: import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
49: import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
50: import { useTranslation } from 'react-i18next';
```

**错误示例 (Noncompliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (promise) {
  // ...
}
```

**正确示例 (Compliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (await promise) {
  // ...
}
```

### 40.4 Promise-returning function provided to property where a void return was expected.

- **问题ID**: `AZybSJrDEFps_QDQ6-pu`
- **项目**: huanchong-99
- **行号**: L8815
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 8815min effort
- **创建时间**: 14 hours ago
- **标签**: async, promise, ...

**问题代码片段**:
```
1: import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
2: import { useQueryClient } from '@tanstack/react-query';
3: import { useNavigate, useSearchParams } from 'react-router-dom';
4: import { Button } from '@/components/ui/button';
5: import { Card, CardContent } from '@/components/ui/card';
6: import {
7: Select,
8: SelectContent,
9: SelectItem,
10: SelectTrigger,
11: SelectValue,
12: } from '@/components/ui/select';
13: import {
14: Plus,
15: Play,
16: Pause,
17: Square,
18: Trash2,
19: Rocket,
20: GitMerge,
21: } from 'lucide-react';
22: import { Loader } from '@/components/ui/loader';
23: import {
24: useWorkflows,
25: useCreateWorkflow,
26: usePrepareWorkflow,
27: useStartWorkflow,
28: usePauseWorkflow,
29: useStopWorkflow,
30: useMergeWorkflow,
31: useDeleteWorkflow,
32: useWorkflow,
33: workflowKeys,
34: getWorkflowActions,
35: useSubmitWorkflowPromptResponse,
36: } from '@/hooks/useWorkflows';
37: import { useProjects } from '@/hooks/useProjects';
38: import type { WorkflowTaskDto } from 'shared/types';
39: import { WorkflowWizard } from '@/components/workflow/WorkflowWizard';
40: import {
41: PipelineView,
42: type WorkflowStatus,
43: type WorkflowTask,
44: } from '@/components/workflow/PipelineView';
45: import { WizardConfig, wizardConfigToCreateRequest } from '@/components/workflow/types';
46: import type { TerminalStatus } from '@/components/workflow/TerminalCard';
47: import { cn } from '@/lib/utils';
48: import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
49: import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
50: import { useTranslation } from 'react-i18next';
```

**错误示例 (Noncompliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (promise) {
  // ...
}
```

**正确示例 (Compliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (await promise) {
  // ...
}
```

### 40.5 Promise-returning function provided to property where a void return was expected.

- **问题ID**: `AZybSJrDEFps_QDQ6-pv`
- **项目**: huanchong-99
- **行号**: L8825
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 8825min effort
- **创建时间**: 14 hours ago
- **标签**: async, promise, ...

**问题代码片段**:
```
1: import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
2: import { useQueryClient } from '@tanstack/react-query';
3: import { useNavigate, useSearchParams } from 'react-router-dom';
4: import { Button } from '@/components/ui/button';
5: import { Card, CardContent } from '@/components/ui/card';
6: import {
7: Select,
8: SelectContent,
9: SelectItem,
10: SelectTrigger,
11: SelectValue,
12: } from '@/components/ui/select';
13: import {
14: Plus,
15: Play,
16: Pause,
17: Square,
18: Trash2,
19: Rocket,
20: GitMerge,
21: } from 'lucide-react';
22: import { Loader } from '@/components/ui/loader';
23: import {
24: useWorkflows,
25: useCreateWorkflow,
26: usePrepareWorkflow,
27: useStartWorkflow,
28: usePauseWorkflow,
29: useStopWorkflow,
30: useMergeWorkflow,
31: useDeleteWorkflow,
32: useWorkflow,
33: workflowKeys,
34: getWorkflowActions,
35: useSubmitWorkflowPromptResponse,
36: } from '@/hooks/useWorkflows';
37: import { useProjects } from '@/hooks/useProjects';
38: import type { WorkflowTaskDto } from 'shared/types';
39: import { WorkflowWizard } from '@/components/workflow/WorkflowWizard';
40: import {
41: PipelineView,
42: type WorkflowStatus,
43: type WorkflowTask,
44: } from '@/components/workflow/PipelineView';
45: import { WizardConfig, wizardConfigToCreateRequest } from '@/components/workflow/types';
46: import type { TerminalStatus } from '@/components/workflow/TerminalCard';
47: import { cn } from '@/lib/utils';
48: import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
49: import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
50: import { useTranslation } from 'react-i18next';
```

**错误示例 (Noncompliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (promise) {
  // ...
}
```

**正确示例 (Compliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (await promise) {
  // ...
}
```

### 40.6 Promise-returning function provided to property where a void return was expected.

- **问题ID**: `AZybSJrDEFps_QDQ6-pw`
- **项目**: huanchong-99
- **行号**: L8835
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 8835min effort
- **创建时间**: 14 hours ago
- **标签**: async, promise, ...

**问题代码片段**:
```
1: import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
2: import { useQueryClient } from '@tanstack/react-query';
3: import { useNavigate, useSearchParams } from 'react-router-dom';
4: import { Button } from '@/components/ui/button';
5: import { Card, CardContent } from '@/components/ui/card';
6: import {
7: Select,
8: SelectContent,
9: SelectItem,
10: SelectTrigger,
11: SelectValue,
12: } from '@/components/ui/select';
13: import {
14: Plus,
15: Play,
16: Pause,
17: Square,
18: Trash2,
19: Rocket,
20: GitMerge,
21: } from 'lucide-react';
22: import { Loader } from '@/components/ui/loader';
23: import {
24: useWorkflows,
25: useCreateWorkflow,
26: usePrepareWorkflow,
27: useStartWorkflow,
28: usePauseWorkflow,
29: useStopWorkflow,
30: useMergeWorkflow,
31: useDeleteWorkflow,
32: useWorkflow,
33: workflowKeys,
34: getWorkflowActions,
35: useSubmitWorkflowPromptResponse,
36: } from '@/hooks/useWorkflows';
37: import { useProjects } from '@/hooks/useProjects';
38: import type { WorkflowTaskDto } from 'shared/types';
39: import { WorkflowWizard } from '@/components/workflow/WorkflowWizard';
40: import {
41: PipelineView,
42: type WorkflowStatus,
43: type WorkflowTask,
44: } from '@/components/workflow/PipelineView';
45: import { WizardConfig, wizardConfigToCreateRequest } from '@/components/workflow/types';
46: import type { TerminalStatus } from '@/components/workflow/TerminalCard';
47: import { cn } from '@/lib/utils';
48: import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
49: import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
50: import { useTranslation } from 'react-i18next';
```

**错误示例 (Noncompliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (promise) {
  // ...
}
```

**正确示例 (Compliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (await promise) {
  // ...
}
```

### 40.7 Promise-returning function provided to property where a void return was expected.

- **问题ID**: `AZybSJrDEFps_QDQ6-px`
- **项目**: huanchong-99
- **行号**: L8845
- **类型**: Bug
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 8845min effort
- **创建时间**: 14 hours ago
- **标签**: async, promise, ...

**问题代码片段**:
```
1: import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
2: import { useQueryClient } from '@tanstack/react-query';
3: import { useNavigate, useSearchParams } from 'react-router-dom';
4: import { Button } from '@/components/ui/button';
5: import { Card, CardContent } from '@/components/ui/card';
6: import {
7: Select,
8: SelectContent,
9: SelectItem,
10: SelectTrigger,
11: SelectValue,
12: } from '@/components/ui/select';
13: import {
14: Plus,
15: Play,
16: Pause,
17: Square,
18: Trash2,
19: Rocket,
20: GitMerge,
21: } from 'lucide-react';
22: import { Loader } from '@/components/ui/loader';
23: import {
24: useWorkflows,
25: useCreateWorkflow,
26: usePrepareWorkflow,
27: useStartWorkflow,
28: usePauseWorkflow,
29: useStopWorkflow,
30: useMergeWorkflow,
31: useDeleteWorkflow,
32: useWorkflow,
33: workflowKeys,
34: getWorkflowActions,
35: useSubmitWorkflowPromptResponse,
36: } from '@/hooks/useWorkflows';
37: import { useProjects } from '@/hooks/useProjects';
38: import type { WorkflowTaskDto } from 'shared/types';
39: import { WorkflowWizard } from '@/components/workflow/WorkflowWizard';
40: import {
41: PipelineView,
42: type WorkflowStatus,
43: type WorkflowTask,
44: } from '@/components/workflow/PipelineView';
45: import { WizardConfig, wizardConfigToCreateRequest } from '@/components/workflow/types';
46: import type { TerminalStatus } from '@/components/workflow/TerminalCard';
47: import { cn } from '@/lib/utils';
48: import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
49: import { CreateProjectDialog } from '@/components/ui-new/dialogs/CreateProjectDialog';
50: import { useTranslation } from 'react-i18next';
```

**错误示例 (Noncompliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (promise) {
  // ...
}
```

**正确示例 (Compliant)**:
```
const promise = new Promise((resolve, reject) => {
  // ...
  resolve(false)
});
if (await promise) {
  // ...
}
```

---

## 41. huanchong-99SoloDawnfrontend/src/pages/settings/AgentSettings.tsx

> 该文件共有 **2** 个问题

### 41.1 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZybSJrbEFps_QDQ6-py`
- **项目**: huanchong-99
- **行号**: L4345
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4345min effort
- **创建时间**: 14 hours ago
- **标签**: confusing

**问题代码片段**:
```
1: import { useEffect, useState } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import { cloneDeep, isEqual } from 'lodash';
4: import {
5: Card,
6: CardContent,
7: CardDescription,
8: CardHeader,
9: CardTitle,
10: } from '@/components/ui/card';
11: import { Button } from '@/components/ui/button';
12: import {
13: Select,
14: SelectContent,
15: SelectItem,
16: SelectTrigger,
17: SelectValue,
18: } from '@/components/ui/select';
19: import {
20: DropdownMenu,
21: DropdownMenuContent,
22: DropdownMenuItem,
23: DropdownMenuTrigger,
24: } from '@/components/ui/dropdown-menu';
25: import { Label } from '@/components/ui/label';
26: import { Alert, AlertDescription } from '@/components/ui/alert';
27: import { Checkbox } from '@/components/ui/checkbox';
28: import { JSONEditor } from '@/components/ui/json-editor';
29: import { ChevronDown, Loader2 } from 'lucide-react';
31: import { ExecutorConfigForm } from '@/components/ExecutorConfigForm';
32: import { useProfiles } from '@/hooks/useProfiles';
33: import { useUserSystem } from '@/components/ConfigProvider';
34: import { CreateConfigurationDialog } from '@/components/dialogs/settings/CreateConfigurationDialog';
35: import { DeleteConfigurationDialog } from '@/components/dialogs/settings/DeleteConfigurationDialog';
36: import { useAgentAvailability } from '@/hooks/useAgentAvailability';
37: import { AgentAvailabilityIndicator } from '@/components/AgentAvailabilityIndicator';
38: import type {
39: BaseCodingAgent,
40: ExecutorConfigs,
41: ExecutorProfileId,
42: } from 'shared/types';
44: type ExecutorsMap = Record<string, Record<string, Record<string, unknown>>>;
46: export function AgentSettings() {
47: const { t } = useTranslation(['settings', 'common']);
48: // Use profiles hook for server state
49: const {
50: profilesContent: serverProfilesContent,
51: profilesPath,
52: isLoading: profilesLoading,
53: isSaving: profilesSaving,
```

### 41.2 'profilesError' will use Object's default stringification format ('[object Object]') when stringified.

- **问题ID**: `AZyVwetlZ9DOUQdEsGku`
- **项目**: huanchong-99
- **行号**: L4365
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 4365min effort
- **创建时间**: 1 month ago
- **标签**: object, string, ...

**问题代码片段**:
```
1: import { useEffect, useState } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import { cloneDeep, isEqual } from 'lodash';
4: import {
5: Card,
6: CardContent,
7: CardDescription,
8: CardHeader,
9: CardTitle,
10: } from '@/components/ui/card';
11: import { Button } from '@/components/ui/button';
12: import {
13: Select,
14: SelectContent,
15: SelectItem,
16: SelectTrigger,
17: SelectValue,
18: } from '@/components/ui/select';
19: import {
20: DropdownMenu,
21: DropdownMenuContent,
22: DropdownMenuItem,
23: DropdownMenuTrigger,
24: } from '@/components/ui/dropdown-menu';
25: import { Label } from '@/components/ui/label';
26: import { Alert, AlertDescription } from '@/components/ui/alert';
27: import { Checkbox } from '@/components/ui/checkbox';
28: import { JSONEditor } from '@/components/ui/json-editor';
29: import { ChevronDown, Loader2 } from 'lucide-react';
31: import { ExecutorConfigForm } from '@/components/ExecutorConfigForm';
32: import { useProfiles } from '@/hooks/useProfiles';
33: import { useUserSystem } from '@/components/ConfigProvider';
34: import { CreateConfigurationDialog } from '@/components/dialogs/settings/CreateConfigurationDialog';
35: import { DeleteConfigurationDialog } from '@/components/dialogs/settings/DeleteConfigurationDialog';
36: import { useAgentAvailability } from '@/hooks/useAgentAvailability';
37: import { AgentAvailabilityIndicator } from '@/components/AgentAvailabilityIndicator';
38: import type {
39: BaseCodingAgent,
40: ExecutorConfigs,
41: ExecutorProfileId,
42: } from 'shared/types';
44: type ExecutorsMap = Record<string, Record<string, Record<string, unknown>>>;
46: export function AgentSettings() {
47: const { t } = useTranslation(['settings', 'common']);
48: // Use profiles hook for server state
49: const {
50: profilesContent: serverProfilesContent,
51: profilesPath,
52: isLoading: profilesLoading,
53: isSaving: profilesSaving,
```

**错误示例 (Noncompliant)**:
```
class Foo {};
const foo = new Foo();

foo + ''; // Noncompliant - evaluates to "[object Object]"
`Foo: ${foo}`; // Noncompliant - evaluates to "Foo: [object Object]"
foo.toString(); // Noncompliant - evaluates to "[object Object]"
```

**正确示例 (Compliant)**:
```
class Foo {
  toString() {
    return 'Foo';
  }
}
const foo = new Foo();

foo + '';
`Foo: ${foo}`;
foo.toString();
```

---

## 42. huanchong-99SoloDawnfrontend/src/pages/settings/McpSettings.tsx

> 该文件共有 **1** 个问题

### 42.1 This assertion is unnecessary since the receiver accepts the original type of the expression.

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

**问题代码片段**:
```
1: import { useEffect, useState } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import {
4: Card,
5: CardContent,
6: CardDescription,
7: CardHeader,
8: CardTitle,
9: } from '@/components/ui/card';
10: import { Button } from '@/components/ui/button';
11: import {
12: Select,
13: SelectContent,
14: SelectItem,
15: SelectTrigger,
16: SelectValue,
17: } from '@/components/ui/select';
18: import {
19: Carousel,
20: CarouselContent,
21: CarouselItem,
22: CarouselNext,
23: CarouselPrevious,
24: } from '@/components/ui/carousel';
25: import { Label } from '@/components/ui/label';
26: import { Alert, AlertDescription } from '@/components/ui/alert';
27: import { JSONEditor } from '@/components/ui/json-editor';
28: import { Loader2 } from 'lucide-react';
29: import type { BaseCodingAgent } from 'shared/types';
30: import { McpConfig } from 'shared/types';
31: import { useUserSystem } from '@/components/ConfigProvider';
32: import { ApiError, mcpServersApi } from '@/lib/api';
33: import { McpConfigStrategyGeneral } from '@/lib/mcpStrategies';
35: const MCP_NOT_SUPPORTED_ERROR_CODE = 'MCP_NOT_SUPPORTED';
37: interface McpUiError {
38: code: string | null;
39: message: string;
40: }
42: export const buildMcpServersPayload = (
43: editorValue: string,
44: mcpConfig: McpConfig
45: ): McpConfig['servers'] => {
46: if (!editorValue.trim()) {
47: return {};
48: }
50: const fullConfig = JSON.parse(editorValue);
51: McpConfigStrategyGeneral.validateFullConfig(mcpConfig, fullConfig);
52: return McpConfigStrategyGeneral.extractServersForApi(mcpConfig, fullConfig);
53: };
55: export function McpSettings() {
```

---

## 43. huanchong-99SoloDawnfrontend/src/vscode/ContextMenu.tsx

> 该文件共有 **1** 个问题

### 43.1 Elements with the 'menu' interactive role must be focusable.

- **问题ID**: `AZybSJxSEFps_QDQ6-p4`
- **项目**: huanchong-99
- **行号**: L2485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 2485min effort
- **创建时间**: 14 hours ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import React, { useEffect, useRef, useState } from 'react';
2: import {
3: readClipboardViaBridge,
4: writeClipboardViaBridge,
5: } from '@/vscode/bridge';
7: type Point = { x: number; y: number };
9: function inIframe(): boolean {
10: try {
11: return globalThis.self !== globalThis.top;
12: } catch {
13: return true;
14: }
15: }
17: function isEditable(
18: target: EventTarget | null
19: ): target is
20: | HTMLInputElement
21: | HTMLTextAreaElement
22: | (HTMLElement & { isContentEditable: boolean }) {
23: const el = target as HTMLElement | null;
24: if (!el) return false;
25: const tag = el.tagName?.toLowerCase();
26: if (tag === 'input' || tag === 'textarea') return true;
27: return !!el.isContentEditable;
28: }
30: async function readClipboardText(): Promise<string> {
31: return await readClipboardViaBridge();
32: }
33: async function writeClipboardText(text: string): Promise<boolean> {
34: return await writeClipboardViaBridge(text);
35: }
37: function getSelectedText(): string {
38: const sel = globalThis.getSelection();
39: return sel ? sel.toString() : '';
40: }
42: function cutFromInput(el: HTMLInputElement | HTMLTextAreaElement) {
43: const start = el.selectionStart ?? 0;
44: const end = el.selectionEnd ?? 0;
45: if (end > start) {
46: const selected = el.value.slice(start, end);
47: void writeClipboardText(selected);
48: const before = el.value.slice(0, start);
49: const after = el.value.slice(end);
50: el.value = before + after;
51: el.setSelectionRange(start, start);
52: el.dispatchEvent(new Event('input', { bubbles: true }));
53: }
54: }
56: function pasteIntoInput(
57: el: HTMLInputElement | HTMLTextAreaElement,
```

**错误示例 (Noncompliant)**:
```
<!-- Element with mouse/keyboard handler has no tabindex -->
<span onclick="submitForm();" role="button">Submit</span>

<!-- Anchor element without href is not focusable -->
<a onclick="showNextPage();" role="button">Next page</a>
```

**正确示例 (Compliant)**:
```
<!-- Element with mouse handler has tabIndex -->
<span onClick="doSomething();" tabIndex="0" role="button">Submit</span>

<!-- Focusable anchor with mouse handler -->
<a href="javascript:void(0);" onClick="doSomething();"> Next page </a>
```

---

## 44. huanchong-99SoloDawnfrontend/src/vscode/bridge.ts

> 该文件共有 **3** 个问题

### 44.1 'platform' is deprecated.

- **问题ID**: `AZybSJxCEFps_QDQ6-p2`
- **项目**: huanchong-99
- **行号**: L5715
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 5715min effort
- **创建时间**: 14 hours ago
- **标签**: cwe, obsolete, ...

**问题代码片段**:
```
1: // VS Code Webview iframe keyboard bridge
2: //
3: // Purpose
4: // - Make typing, paste/cut/undo/redo inside the iframe feel like a regular browser
5: // input/textarea/contentEditable.
6: // - Still allow VS Code to handle global/editor shortcuts by forwarding non-text
7: // editing keys to the parent webview.
8: // - Bridge clipboard reads/writes when navigator.clipboard is restricted.
10: /** Returns true when running inside an iframe (vs top-level window). */
11: export function inIframe(): boolean {
12: try {
13: return globalThis.self !== globalThis.top;
14: } catch (error) {
15: console.debug('Cannot access window.top, assuming iframe', error);
16: return true;
17: }
18: }
20: /** Minimal serializable keyboard event shape used across the bridge. */
21: type KeyPayload = {
22: key: string;
23: code: string;
24: altKey: boolean;
25: ctrlKey: boolean;
26: shiftKey: boolean;
27: metaKey: boolean;
28: repeat: boolean;
29: isComposing: boolean;
30: location: number;
31: };
33: /** Convert a KeyboardEvent to a serializable payload for postMessage. */
34: function serializeKeyEvent(e: KeyboardEvent): KeyPayload {
35: return {
36: key: e.key,
37: code: e.code,
38: altKey: e.altKey,
39: ctrlKey: e.ctrlKey,
40: shiftKey: e.shiftKey,
41: metaKey: e.metaKey,
42: repeat: e.repeat,
43: isComposing: e.isComposing,
44: location: e.location ?? 0,
45: };
46: }
48: /** Type alias for nullable form input elements */
49: type NullableFormInput = HTMLTextAreaElement | HTMLInputElement | null;
51: /** Type alias for editable elements to reduce union type repetition */
52: type EditableElement = HTMLInputElement | HTMLTextAreaElement | (HTMLElement & { isContentEditable: boolean });
54: /** Platform check used for shortcut detection. */
55: const isMac = () => {
56: // eslint-disable-next-line @typescript-eslint/no-deprecated -- fallback for browsers without userAgentData
```

### 44.2 Verify the origin of the received message.

- **问题ID**: `AZyZVcoWuNB-_5CPqJgu`
- **项目**: huanchong-99
- **行号**: L31510
- **类型**: Vulnerability
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Security
- **工作量**: 31510min effort
- **创建时间**: 23 hours ago
- **标签**: cwe, html5, ...

**问题代码片段**:
```
1: // VS Code Webview iframe keyboard bridge
2: //
3: // Purpose
4: // - Make typing, paste/cut/undo/redo inside the iframe feel like a regular browser
5: // input/textarea/contentEditable.
6: // - Still allow VS Code to handle global/editor shortcuts by forwarding non-text
7: // editing keys to the parent webview.
8: // - Bridge clipboard reads/writes when navigator.clipboard is restricted.
10: /** Returns true when running inside an iframe (vs top-level window). */
11: export function inIframe(): boolean {
12: try {
13: return globalThis.self !== globalThis.top;
14: } catch (error) {
15: console.debug('Cannot access window.top, assuming iframe', error);
16: return true;
17: }
18: }
20: /** Minimal serializable keyboard event shape used across the bridge. */
21: type KeyPayload = {
22: key: string;
23: code: string;
24: altKey: boolean;
25: ctrlKey: boolean;
26: shiftKey: boolean;
27: metaKey: boolean;
28: repeat: boolean;
29: isComposing: boolean;
30: location: number;
31: };
33: /** Convert a KeyboardEvent to a serializable payload for postMessage. */
34: function serializeKeyEvent(e: KeyboardEvent): KeyPayload {
35: return {
36: key: e.key,
37: code: e.code,
38: altKey: e.altKey,
39: ctrlKey: e.ctrlKey,
40: shiftKey: e.shiftKey,
41: metaKey: e.metaKey,
42: repeat: e.repeat,
43: isComposing: e.isComposing,
44: location: e.location ?? 0,
45: };
46: }
48: /** Type alias for nullable form input elements */
49: type NullableFormInput = HTMLTextAreaElement | HTMLInputElement | null;
51: /** Type alias for editable elements to reduce union type repetition */
52: type EditableElement = HTMLInputElement | HTMLTextAreaElement | (HTMLElement & { isContentEditable: boolean });
54: /** Platform check used for shortcut detection. */
55: const isMac = () => {
56: // eslint-disable-next-line @typescript-eslint/no-deprecated -- fallback for browsers without userAgentData
```

**错误示例 (Noncompliant)**:
```
var iframe = document.getElementById("testiframe");
iframe.contentWindow.postMessage("hello", "*"); // Noncompliant: * is used
```

**正确示例 (Compliant)**:
```
var iframe = document.getElementById("testiframe");
iframe.contentWindow.postMessage("hello", "https://secure.example.com");
```

### 44.3 Prefer `globalThis` over `window`.

- **问题ID**: `AZybSJxCEFps_QDQ6-p3`
- **项目**: huanchong-99
- **行号**: L3172
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 3172min effort
- **创建时间**: 14 hours ago
- **标签**: es2020, portability

**问题代码片段**:
```
1: // VS Code Webview iframe keyboard bridge
2: //
3: // Purpose
4: // - Make typing, paste/cut/undo/redo inside the iframe feel like a regular browser
5: // input/textarea/contentEditable.
6: // - Still allow VS Code to handle global/editor shortcuts by forwarding non-text
7: // editing keys to the parent webview.
8: // - Bridge clipboard reads/writes when navigator.clipboard is restricted.
10: /** Returns true when running inside an iframe (vs top-level window). */
11: export function inIframe(): boolean {
12: try {
13: return globalThis.self !== globalThis.top;
14: } catch (error) {
15: console.debug('Cannot access window.top, assuming iframe', error);
16: return true;
17: }
18: }
20: /** Minimal serializable keyboard event shape used across the bridge. */
21: type KeyPayload = {
22: key: string;
23: code: string;
24: altKey: boolean;
25: ctrlKey: boolean;
26: shiftKey: boolean;
27: metaKey: boolean;
28: repeat: boolean;
29: isComposing: boolean;
30: location: number;
31: };
33: /** Convert a KeyboardEvent to a serializable payload for postMessage. */
34: function serializeKeyEvent(e: KeyboardEvent): KeyPayload {
35: return {
36: key: e.key,
37: code: e.code,
38: altKey: e.altKey,
39: ctrlKey: e.ctrlKey,
40: shiftKey: e.shiftKey,
41: metaKey: e.metaKey,
42: repeat: e.repeat,
43: isComposing: e.isComposing,
44: location: e.location ?? 0,
45: };
46: }
48: /** Type alias for nullable form input elements */
49: type NullableFormInput = HTMLTextAreaElement | HTMLInputElement | null;
51: /** Type alias for editable elements to reduce union type repetition */
52: type EditableElement = HTMLInputElement | HTMLTextAreaElement | (HTMLElement & { isContentEditable: boolean });
54: /** Platform check used for shortcut detection. */
55: const isMac = () => {
56: // eslint-disable-next-line @typescript-eslint/no-deprecated -- fallback for browsers without userAgentData
```

---

## 45. huanchong-99SoloDawnscripts/check-i18n.sh

> 该文件共有 **1** 个问题

### 45.1 Add an explicit return statement at the end of the function.

- **问题ID**: `AZyVwe7iZ9DOUQdEsGqV`
- **项目**: huanchong-99
- **行号**: L1922
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1922min effort
- **创建时间**: 1 month ago
- **标签**: best-practice, clarity, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # i18n regression check script
3: # Compares i18next/no-literal-string violations between PR and main branch
4: # Initial implementation: This script will show high violation counts until enforcement is enabled
5: set -eo pipefail
7: RULE="i18next/no-literal-string"
8: INDENT_FORMAT=' - %s\n'
10: # Function that outputs violation count to stdout
11: lint_count() {
12: local dir=$1
13: local tmp
14: tmp=$(mktemp)
16: trap 'rm -f "$tmp"' RETURN
18: (
19: set -eo pipefail
20: cd "$dir/frontend" || exit 1
21: # Lint current directory using ESLint from PR workspace
22: LINT_I18N=true npx --prefix "$REPO_ROOT/frontend" eslint . \
23: --ext ts,tsx \
24: --format json \
25: --output-file "$tmp" \
26: --no-error-on-unmatched-pattern \
27: > /dev/null 2>&1 || true # Don't fail on violations
28: )
30: # Parse the clean JSON file
31: jq --arg RULE "$RULE" \
32: '[.[].messages[] | select(.ruleId == $RULE)] | length' "$tmp" \
33: 2>/dev/null || echo "0"
35: return 0
36: }
38: get_json_keys() {
39: local file=$1
40: if [[ ! -f "$file" ]]; then
41: return 2
42: fi
43: jq -r '
44: paths(scalars) as $p
45: | select(getpath($p) | type == "string")
46: | $p | join(".")
47: ' "$file" 2>/dev/null | LC_ALL=C sort -u
48: return 0
49: }
51: check_duplicate_keys() {
52: local file=$1
53: if [[ ! -f "$file" ]]; then
54: return 2
55: fi
57: # Strategy: Use jq's --stream flag to detect duplicate keys
58: # jq --stream processes JSON before parsing (preserves duplicates)
59: # jq tostream processes JSON after parsing (duplicates already collapsed)
```

**错误示例 (Noncompliant)**:
```
my_function() {
  local param="$1"
  echo "processing $param"
}  # Noncompliant
```

**正确示例 (Compliant)**:
```
my_function() {
  local param="$1"
  echo "processing $param"
  return 0
}
```

---

