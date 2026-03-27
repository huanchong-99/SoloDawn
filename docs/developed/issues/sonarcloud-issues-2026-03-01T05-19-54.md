# SonarCloud Issues 报告

**生成时间**: 2026/03/01 13:19
**问题总数**: 3
**已加载**: 3
**收集数量**: 3

---

## 统计信息

### 按严重程度分类

- **Major**: 2 个
- **Minor**: 1 个

### 按类型分类

- **Code Smell**: 3 个

### 按影响分类

- **Maintainability**: 3 个

### 按属性分类

- **Consistency**: 3 个

### 按文件统计 (Top 20)

- **frontend/src/components/workflow/steps/Step2Tasks.tsx**: 1 个问题
- **scripts/run-dev.js**: 1 个问题
- **scripts/setup-dev-environment.js**: 1 个问题

---

## 问题列表（按文件分组）

## 1. frontend/src/components/workflow/steps/Step2Tasks.tsx

> 该文件共有 **1** 个问题

### 1.1 Prefer `.at(…)` over `[….length - index]`.

- **问题ID**: `AZyn1L8j_TisDgi1p14j`
- **项目**: huanchong-99
- **行号**: L475
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 475min effort
- **标签**: es2022, performance, ...

**问题代码片段**:
```
1: import React, { useState, useEffect, useRef } from 'react';
2: import { ChevronLeft, ChevronRight } from 'lucide-react';
3: import { Field, FieldLabel, FieldError } from '../../ui-new/primitives/Field';
4: import { cn } from '@/lib/utils';
5: import type { TaskConfig } from '../types';
6: import { useTranslation } from 'react-i18next';
8: /** Quick select options for terminal count */
9: const TERMINAL_COUNT_QUICK_OPTIONS = [1, 2, 3, 4, 5];
10: /** Maximum allowed terminal count */
11: const MAX_TERMINAL_COUNT = 10;
12: /** Minimum allowed terminal count */
13: const MIN_TERMINAL_COUNT = 1;
15: function slugify(text: string): string {
16: const normalized = text.toLowerCase().trim();
17: const slugChars: string[] = [];
18: let lastWasSeparator = false;
20: for (const char of normalized) {
21: const codePoint = char.codePointAt(0);
22: if (codePoint === undefined) {
23: continue;
24: }
26: const isAsciiAlphaNumeric =
27: (codePoint >= 97 && codePoint <= 122) ||
28: (codePoint >= 48 && codePoint <= 57);
30: if (isAsciiAlphaNumeric) {
31: slugChars.push(char);
32: lastWasSeparator = false;
33: continue;
34: }
36: const isSeparator =
37: codePoint === 45 || // -
38: codePoint === 95 || // _
39: char.trim().length === 0;
41: if (isSeparator && !lastWasSeparator && slugChars.length > 0) {
42: slugChars.push('-');
43: lastWasSeparator = true;
44: }
45: }
47: if (slugChars[] === '-') {
48: slugChars.pop();
49: }
51: return slugChars.join('');
52: }
54: interface Step2TasksProps {
55: config: TaskConfig[];
56: taskCount: number;
57: onChange: (tasks: TaskConfig[]) => void;
58: errors: Record<string, string>;
59: }
61: /**
```

---

## 2. scripts/run-dev.js

> 该文件共有 **1** 个问题

### 2.1 Prefer top-level await over an async function `start` call.

- **问题ID**: `AZyn1Men_TisDgi1p14k`
- **项目**: huanchong-99
- **行号**: L5125
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 5125min effort
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
15: function getPathKey(env) {
16: return Object.keys(env).find((name) => name.toLowerCase() === "path") ?? "PATH";
17: }
19: function resolveExecutable(command, env = process.env) {
20: if (typeof command !== "string" || command.length === 0) {
21: return command;
22: }
23: if (path.isAbsolute(command) || command.includes("/") || command.includes("\\")) {
24: return command;
25: }
27: const pathValue = env[getPathKey(env)];
28: if (typeof pathValue !== "string" || pathValue.length === 0) {
29: return command;
30: }
32: const extensions =
33: process.platform === "win32"
34: ? (env.PATHEXT ?? process.env.PATHEXT ?? ".EXE;.CMD;.BAT;.COM")
35: .split(";")
36: .filter(Boolean)
37: : [""];
38: const names =
39: process.platform === "win32" && path.extname(command) === ""
40: ? extensions.map((ext) => `${command}${ext}`)
41: : [command];
43: for (const dir of pathValue.split(path.delimiter).filter(Boolean)) {
44: for (const name of names) {
45: const candidate = path.join(dir, name);
46: try {
47: fs.accessSync(candidate, fs.constants.X_OK);
48: return candidate;
49: } catch {
50: // Ignore and continue checking other PATH entries.
51: }
52: }
53: }
55: return command;
56: }
58: function isProcessAlive(pid) {
59: if (!Number.isInteger(pid) || pid <= 0) return false;
```

---

## 3. scripts/setup-dev-environment.js

> 该文件共有 **1** 个问题

### 3.1 Prefer top-level await over an async IIFE.

- **问题ID**: `AZyn1Me-_TisDgi1p14l`
- **项目**: huanchong-99
- **行号**: L1365
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1365min effort
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

