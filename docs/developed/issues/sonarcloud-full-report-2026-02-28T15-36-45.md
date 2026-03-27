# SonarCloud 代码质量完整报告

**生成时间**: 2026/02/28 23:36
**项目**: huanchong-99_SoloDawn

---

# SonarCloud Issues 报告

**生成时间**: 2026/02/28 23:36
**问题总数**: 2
**已加载**: 2
**收集数量**: 2

---

## 统计信息

### 按严重程度分类

- **Major**: 2 个

### 按类型分类

- **Code Smell**: 2 个

### 按影响分类

- **Maintainability**: 2 个

### 按属性分类

- **Consistency**: 2 个

### 按文件统计 (Top 20)

- **scripts/run-dev.js**: 1 个问题
- **scripts/setup-dev-environment.js**: 1 个问题

---

## 问题列表（按文件分组）

## 1. scripts/run-dev.js

> 该文件共有 **1** 个问题

### 1.1 Prefer top-level await over using a promise chain.

- **问题ID**: `AZygjZRMmRmdv3ynIrVA`
- **项目**: huanchong-99
- **行号**: L4575
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 4575min effort
- **创建时间**: 20 hours ago
- **标签**: async, es2022, ...

**问题代码片段**:
```
1: #!/usr/bin/env node
3: const fs = require("node:fs");
4: const os = require("node:os");
5: const path = require("node:path");
6: const net = require("node:net");
7: const { spawn, spawnSync } = require("node:child_process");
8: const { getPorts } = require("./setup-dev-environment");
10: const children = new Set();
11: let shuttingDown = false;
12: const devLockPath = path.join(os.tmpdir(), "solodawn", "run-dev.lock");
13: let lockFd = null;
15: function isProcessAlive(pid) {
16: if (!Number.isInteger(pid) || pid <= 0) return false;
17: try {
18: process.kill(pid, 0);
19: return true;
20: } catch {
21: return false;
22: }
23: }
25: function acquireDevLock() {
26: fs.mkdirSync(path.dirname(devLockPath), { recursive: true });
28: const tryAcquire = () => {
29: lockFd = fs.openSync(devLockPath, "wx");
30: fs.writeFileSync(lockFd, `${process.pid}\n`, { encoding: "utf8" });
31: };
33: try {
34: tryAcquire();
35: return;
36: } catch (error) {
37: if (error?.code !== "EEXIST") {
38: throw error;
39: }
40: }
42: let existingPid = null;
43: try {
44: const content = fs.readFileSync(devLockPath, "utf8").trim();
45: const parsed = Number(content);
46: if (Number.isInteger(parsed) && parsed > 0) {
47: existingPid = parsed;
48: }
49: } catch {
50: // Ignore stale/unreadable lock file content and attempt cleanup.
51: }
53: if (existingPid && isProcessAlive(existingPid)) {
54: throw new Error(
55: `Another dev environment is already running (pid ${existingPid}). Stop it before starting a new one.`
56: );
57: }
59: // Stale lock: remove once and retry.
```

---

## 2. scripts/setup-dev-environment.js

> 该文件共有 **1** 个问题

### 2.1 Prefer top-level await over using a promise chain.

- **问题ID**: `AZygjZTNmRmdv3ynIrVB`
- **项目**: huanchong-99
- **行号**: L1365
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1365min effort
- **创建时间**: 20 hours ago
- **标签**: async, es2022, ...

**问题代码片段**:
```
1: #!/usr/bin/env node
3: const fs = require("node:fs");
4: const path = require("node:path");
5: const net = require("node:net");
7: const DEV_ASSETS_SEED = path.join(__dirname, "..", "dev_assets_seed");
8: const DEV_ASSETS = path.join(__dirname, "..", "dev_assets");
10: // Fixed development ports - always use these
11: const FIXED_FRONTEND_PORT = 23457;
12: const FIXED_BACKEND_PORT = 23456;
14: /**
15: * Check if a port is available
16: */
17: function isPortAvailable(port) {
18: return new Promise((resolve) => {
19: const sock = net.createConnection({ port, host: "localhost" });
20: sock.on("connect", () => {
21: sock.destroy();
22: resolve(false);
23: });
24: sock.on("error", () => resolve(true));
25: });
26: }
28: /**
29: * Allocate ports for development - always use fixed ports
30: */
31: async function allocatePorts() {
32: const ports = {
33: frontend: FIXED_FRONTEND_PORT,
34: backend: FIXED_BACKEND_PORT,
35: timestamp: new Date().toISOString(),
36: };
38: const frontendAvailable = await isPortAvailable(ports.frontend);
39: const backendAvailable = await isPortAvailable(ports.backend);
41: if (process.argv[2] === "get") {
42: if (!frontendAvailable || !backendAvailable) {
43: console.log(
44: `Port availability check failed: frontend:${ports.frontend}=${frontendAvailable}, backend:${ports.backend}=${backendAvailable}`
45: );
46: }
48: console.log("Using fixed dev ports:");
49: console.log(`Frontend: ${ports.frontend}`);
50: console.log(`Backend: ${ports.backend}`);
51: }
53: return ports;
54: }
56: /**
57: * Get ports (allocate if needed)
58: */
59: async function getPorts() {
60: const ports = await allocatePorts();
```

---



---

# SonarCloud 重复代码报告

**生成时间**: 2026/02/28 23:36
**项目**: huanchong-99_SoloDawn
**问题文件总数**: 7
**重复行总数**: 475
**重复块总数**: 86

---

## 统计信息

### 重复率分布

- **严重 (≥50%)**: 6 个文件
- **较高 (30-50%)**: 1 个文件

---

## 重复文件列表（按路径分组）

## 1. crates/db/migrations

> 该目录共有 **1** 个重复文件

### 1.1 20250716143725_add_default_templates.sql

- **路径**: `crates/db/migrations/20250716143725_add_default_templates.sql`
- **重复率**: 95.1%
- **重复行数**: 98 行
- **重复块数**: 26 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_SoloDawn&metric=new_duplicated_lines_density&selected=huanchong-99_SoloDawn%3Acrates%2Fdb%2Fmigrations%2F20250716143725_add_default_templates.sql)

---

## 2. frontend/src/components/ui-new/containers

> 该目录共有 **2** 个重复文件

### 2.1 NavbarContainer.tsx

- **路径**: `frontend/src/components/ui-new/containers/NavbarContainer.tsx`
- **重复率**: 100%
- **重复行数**: 4 行
- **重复块数**: 1 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_SoloDawn&metric=new_duplicated_lines_density&selected=huanchong-99_SoloDawn%3Afrontend%2Fsrc%2Fcomponents%2Fui-new%2Fcontainers%2FNavbarContainer.tsx)

### 2.2 ContextBarContainer.tsx

- **路径**: `frontend/src/components/ui-new/containers/ContextBarContainer.tsx`
- **重复率**: 66.7%
- **重复行数**: 4 行
- **重复块数**: 1 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_SoloDawn&metric=new_duplicated_lines_density&selected=huanchong-99_SoloDawn%3Afrontend%2Fsrc%2Fcomponents%2Fui-new%2Fcontainers%2FContextBarContainer.tsx)

---

## 3. frontend/src/components/ui-new/hooks

> 该目录共有 **1** 个重复文件

### 3.1 usePreviewUrl.ts

- **路径**: `frontend/src/components/ui-new/hooks/usePreviewUrl.ts`
- **重复率**: 99.4%
- **重复行数**: 169 行
- **重复块数**: 3 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_SoloDawn&metric=new_duplicated_lines_density&selected=huanchong-99_SoloDawn%3Afrontend%2Fsrc%2Fcomponents%2Fui-new%2Fhooks%2FusePreviewUrl.ts)

---

## 4. frontend/src/hooks

> 该目录共有 **2** 个重复文件

### 4.1 useDevserverUrl.ts

- **路径**: `frontend/src/hooks/useDevserverUrl.ts`
- **重复率**: 99.4%
- **重复行数**: 169 行
- **重复块数**: 3 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_SoloDawn&metric=new_duplicated_lines_density&selected=huanchong-99_SoloDawn%3Afrontend%2Fsrc%2Fhooks%2FuseDevserverUrl.ts)

### 4.2 useWorkflows.test.tsx

- **路径**: `frontend/src/hooks/useWorkflows.test.tsx`
- **重复率**: 37.5%
- **重复行数**: 3 行
- **重复块数**: 5 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_SoloDawn&metric=new_duplicated_lines_density&selected=huanchong-99_SoloDawn%3Afrontend%2Fsrc%2Fhooks%2FuseWorkflows.test.tsx)

---

## 5. frontend/src/pages

> 该目录共有 **1** 个重复文件

### 5.1 Workflows.test.tsx

- **路径**: `frontend/src/pages/Workflows.test.tsx`
- **重复率**: 62.2%
- **重复行数**: 28 行
- **重复块数**: 47 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_SoloDawn&metric=new_duplicated_lines_density&selected=huanchong-99_SoloDawn%3Afrontend%2Fsrc%2Fpages%2FWorkflows.test.tsx)

---

