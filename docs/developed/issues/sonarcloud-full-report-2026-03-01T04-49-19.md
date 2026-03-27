# SonarCloud 代码质量完整报告

**生成时间**: 2026/03/01 12:49
**项目**: huanchong-99_SoloDawn

---

# SonarCloud Issues 报告

**生成时间**: 2026/03/01 12:49
**问题总数**: 6
**已加载**: 6
**收集数量**: 6

---

## 统计信息

### 按严重程度分类

- **Minor**: 4 个
- **Major**: 2 个

### 按类型分类

- **Code Smell**: 6 个

### 按影响分类

- **Reliability**: 4 个
- **Maintainability**: 2 个

### 按属性分类

- **Consistency**: 5 个
- **Intentionality**: 1 个

### 按文件统计 (Top 20)

- **frontend/src/contexts/ClickedElementsProvider.tsx**: 2 个问题
- **frontend/src/components/dialogs/org/InviteMemberDialog.tsx**: 1 个问题
- **frontend/src/utils/id.ts**: 1 个问题
- **scripts/run-dev.js**: 1 个问题
- **scripts/setup-dev-environment.js**: 1 个问题

---

## 问题列表（按文件分组）

## 1. frontend/src/components/dialogs/org/InviteMemberDialog.tsx

> 该文件共有 **1** 个问题

### 1.1 Prefer `String#codePointAt()` over `String#charCodeAt()`.

- **问题ID**: `AZynlB3BjMuSUnaHmpeu`
- **项目**: huanchong-99
- **行号**: L725
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 725min effort
- **标签**: internationalization, unicode

**问题代码片段**:
```
1: import { useState, useEffect } from 'react';
2: import { Button } from '@/components/ui/button';
3: import { Input } from '@/components/ui/input';
4: import { Label } from '@/components/ui/label';
5: import {
6: Dialog,
7: DialogContent,
8: DialogDescription,
9: DialogFooter,
10: DialogHeader,
11: DialogTitle,
12: } from '@/components/ui/dialog';
13: import {
14: Select,
15: SelectContent,
16: SelectItem,
17: SelectTrigger,
18: SelectValue,
19: } from '@/components/ui/select';
20: import { Alert, AlertDescription } from '@/components/ui/alert';
21: import NiceModal, { useModal } from '@ebay/nice-modal-react';
22: import { useOrganizationMutations } from '@/hooks/useOrganizationMutations';
23: import { MemberRole } from 'shared/types';
24: import { useTranslation } from 'react-i18next';
25: import { defineModal } from '@/lib/modals';
27: export type InviteMemberResult = {
28: action: 'invited' | 'canceled';
29: };
31: export interface InviteMemberDialogProps {
32: organizationId: string;
33: }
35: const InviteMemberDialogImpl = NiceModal.create<InviteMemberDialogProps>(
36: (props) => {
37: const modal = useModal();
38: const { organizationId } = props;
39: const { t } = useTranslation('organization');
40: const [email, setEmail] = useState('');
41: const [role, setRole] = useState<MemberRole>(MemberRole.MEMBER);
42: const [error, setError] = useState<string | null>(null);
44: const { createInvitation } = useOrganizationMutations({
45: onInviteSuccess: () => {
46: modal.resolve({ action: 'invited' } as InviteMemberResult);
47: modal.hide();
48: },
49: onInviteError: (err) => {
50: setError(
51: err instanceof Error ? err.message : 'Failed to send invitation'
52: );
53: },
54: });
```

---

## 2. frontend/src/contexts/ClickedElementsProvider.tsx

> 该文件共有 **2** 个问题

### 2.1 Prefer `String#codePointAt()` over `String#charCodeAt()`.

- **问题ID**: `AZynlCLxjMuSUnaHmpew`
- **项目**: huanchong-99
- **行号**: L915
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 915min effort
- **标签**: internationalization, unicode

**问题代码片段**:
```
1: import {
2: createContext,
3: useContext,
4: useState,
5: ReactNode,
6: useEffect,
7: useCallback,
8: useMemo,
9: } from 'react';
10: import type {
11: OpenInEditorPayload,
12: ComponentInfo,
13: SelectedComponent,
14: } from '@/utils/previewBridge';
15: import type { Workspace } from 'shared/types';
16: import { genId } from '@/utils/id';
18: export interface ClickedEntry {
19: id: string;
20: payload: OpenInEditorPayload;
21: timestamp: number;
22: dedupeKey: string;
23: selectedDepth?: number; // 0 = innermost (selected), 1 = parent, etc.
24: }
26: interface ClickedElementsContextType {
27: elements: ClickedEntry[];
28: addElement: (payload: OpenInEditorPayload) => void;
29: removeElement: (id: string) => void;
30: clearElements: () => void;
31: selectComponent: (id: string, depthFromInner: number) => void;
32: generateMarkdown: () => string;
33: }
35: const ClickedElementsContext = createContext<ClickedElementsContextType | null>(
36: null
37: );
39: export function useClickedElements() {
40: const context = useContext(ClickedElementsContext);
41: if (!context) {
42: throw new Error(
43: 'useClickedElements must be used within a ClickedElementsProvider'
44: );
45: }
46: return context;
47: }
49: interface ClickedElementsProviderProps {
50: children: ReactNode;
51: attempt?: Workspace | null;
52: }
54: const MAX_ELEMENTS = 20;
55: const MAC_PRIVATE_PREFIX = '/private';
56: const MAC_PRIVATE_ALIAS_ROOTS = new Set(['var', 'tmp']);
```

### 2.2 Prefer `String#codePointAt()` over `String#charCodeAt()`.

- **问题ID**: `AZynlCLxjMuSUnaHmpex`
- **项目**: huanchong-99
- **行号**: L1135
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 1135min effort
- **标签**: internationalization, unicode

**问题代码片段**:
```
1: import {
2: createContext,
3: useContext,
4: useState,
5: ReactNode,
6: useEffect,
7: useCallback,
8: useMemo,
9: } from 'react';
10: import type {
11: OpenInEditorPayload,
12: ComponentInfo,
13: SelectedComponent,
14: } from '@/utils/previewBridge';
15: import type { Workspace } from 'shared/types';
16: import { genId } from '@/utils/id';
18: export interface ClickedEntry {
19: id: string;
20: payload: OpenInEditorPayload;
21: timestamp: number;
22: dedupeKey: string;
23: selectedDepth?: number; // 0 = innermost (selected), 1 = parent, etc.
24: }
26: interface ClickedElementsContextType {
27: elements: ClickedEntry[];
28: addElement: (payload: OpenInEditorPayload) => void;
29: removeElement: (id: string) => void;
30: clearElements: () => void;
31: selectComponent: (id: string, depthFromInner: number) => void;
32: generateMarkdown: () => string;
33: }
35: const ClickedElementsContext = createContext<ClickedElementsContextType | null>(
36: null
37: );
39: export function useClickedElements() {
40: const context = useContext(ClickedElementsContext);
41: if (!context) {
42: throw new Error(
43: 'useClickedElements must be used within a ClickedElementsProvider'
44: );
45: }
46: return context;
47: }
49: interface ClickedElementsProviderProps {
50: children: ReactNode;
51: attempt?: Workspace | null;
52: }
54: const MAX_ELEMENTS = 20;
55: const MAC_PRIVATE_PREFIX = '/private';
56: const MAC_PRIVATE_ALIAS_ROOTS = new Set(['var', 'tmp']);
```

---

## 3. frontend/src/utils/id.ts

> 该文件共有 **1** 个问题

### 3.1 Prefer `String#replaceAll()` over `String#replace()`.

- **问题ID**: `AZynlCMujMuSUnaHmpey`
- **项目**: huanchong-99
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 155min effort
- **标签**: es2021, readability

**问题代码片段**:
```
1: let seq = 0;
2: let fallbackCounter = 0;
4: function bytesToHex(bytes: Uint8Array): string {
5: return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
6: }
8: export function secureRandomIdFragment(length = 8): string {
9: const normalizedLength = Math.max(1, Math.trunc(length));
10: const cryptoApi = globalThis.crypto;
12: if (typeof cryptoApi?.randomUUID === 'function') {
13: let randomValue = '';
14: while (randomValue.length < normalizedLength) {
15: randomValue += cryptoApi.randomUUID().(/-/g, '');
16: }
17: return randomValue.slice(0, normalizedLength);
18: }
20: if (typeof cryptoApi?.getRandomValues === 'function') {
21: const bytes = new Uint8Array(Math.ceil(normalizedLength / 2));
22: cryptoApi.getRandomValues(bytes);
23: return bytesToHex(bytes).slice(0, normalizedLength);
24: }
26: // Fallback for legacy runtimes without Web Crypto: monotonic and process-local unique.
27: fallbackCounter = (fallbackCounter + 1) >>> 0;
28: const fallbackValue = `${Date.now().toString(36)}${fallbackCounter.toString(36)}`;
29: return fallbackValue.length >= normalizedLength
30: ? fallbackValue.slice(-normalizedLength)
31: : fallbackValue.padStart(normalizedLength, '0');
32: }
34: export function genId(): string {
35: seq = (seq + 1) & 0xffff;
36: return `${Date.now().toString(36)}-${seq.toString(36)}-${secureRandomIdFragment(6)}`;
37: }
```

---

## 4. scripts/run-dev.js

> 该文件共有 **1** 个问题

### 4.1 Prefer top-level await over using a promise chain.

- **问题ID**: `AZynlCSSjMuSUnaHmpez`
- **项目**: huanchong-99
- **行号**: L5025
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 5025min effort
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

## 5. scripts/setup-dev-environment.js

> 该文件共有 **1** 个问题

### 5.1 Prefer top-level await over using a promise chain.

- **问题ID**: `AZygjZTNmRmdv3ynIrVB`
- **项目**: huanchong-99
- **行号**: L1365
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1365min effort
- **创建时间**: 1 day ago
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

**生成时间**: 2026/03/01 12:49
**项目**: huanchong-99_SoloDawn
**问题文件总数**: 2
**重复行总数**: 138
**重复块总数**: 34

---

## 统计信息

### 重复率分布

- **严重 (≥50%)**: 1 个文件
- **中等 (10-30%)**: 1 个文件

---

## 重复文件列表（按路径分组）

## 1. crates/db/migrations

> 该目录共有 **1** 个重复文件

### 1.1 20250716143725_add_default_templates.sql

- **路径**: `crates/db/migrations/20250716143725_add_default_templates.sql`
- **重复率**: 94.5%
- **重复行数**: 103 行
- **重复块数**: 26 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_SoloDawn&metric=new_duplicated_lines_density&selected=huanchong-99_SoloDawn%3Acrates%2Fdb%2Fmigrations%2F20250716143725_add_default_templates.sql)

---

## 2. frontend/src/pages

> 该目录共有 **1** 个重复文件

### 2.1 Workflows.test.tsx

- **路径**: `frontend/src/pages/Workflows.test.tsx`
- **重复率**: 27.3%
- **重复行数**: 35 行
- **重复块数**: 8 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_SoloDawn&metric=new_duplicated_lines_density&selected=huanchong-99_SoloDawn%3Afrontend%2Fsrc%2Fpages%2FWorkflows.test.tsx)

---



---

# SonarCloud 安全热点报告

**生成时间**: 2026/03/01 12:49
**项目**: huanchong-99_SoloDawn
**安全热点总数**: 1

---

## 统计信息

### 按审核优先级分布

| 优先级 | 数量 |
|--------|------|
| High | 0 |
| Medium | 1 |
| Low | 0 |

### 按类别分布

- **Denial of Service (DoS)**: 1 个

---

## 安全热点列表

### 🟡 Medium 优先级 (1 个)

#### 1. Make sure the regex used here, which is vulnerable to super-linear runtime due to backtracking, cannot lead to denial of service.

| 属性 | 值 |
|------|----|
| **文件路径** | `frontend/src/components/workflow/steps/Step2Tasks.tsx` |
| **规则ID** | [typescript:S5852](https://sonarcloud.io/organizations/huanchong-99/rules?open=typescript%3AS5852&rule_key=typescript%3AS5852) |
| **类别** | Denial of Service (DoS) |
| **状态** | To Review |
| **SonarCloud** | [查看详情](https://sonarcloud.io/project/security_hotspots?id=huanchong-99_SoloDawn) |

---

