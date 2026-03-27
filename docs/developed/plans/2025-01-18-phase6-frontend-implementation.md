# Phase 6 Frontend Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a 7-step workflow wizard for creating SoloDawn parallel development workflows with TDD approach.

**Architecture:**
- React functional components with TypeScript for type safety
- Wizard state managed through React hooks (useState, useCallback)
- Component composition pattern: reusable primitives in `ui-new/primitives/`, wizard components in `components/workflow/`
- API integration through React Query hooks for data fetching
- Test-first development with Vitest + React Testing Library

**Tech Stack:**
- React 18.2 + TypeScript 5.9
- Radix UI primitives (Dialog, Select, RadioGroup, Switch, Collapsible)
- Tailwind CSS with new design system (text-high, text-normal, text-low, bg-secondary, brand)
- Lucide React icons
- React Query for API calls
- UUID for unique ID generation

---

## Task 1: Create Workflow Type Definitions

**Files:**
- Create: `src/components/workflow/types.ts`
- Test: `src/components/workflow/types.test.ts`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/types.test.ts
import { describe, it, expect } from 'vitest';
import {
  WizardStep,
  WIZARD_STEPS,
  getDefaultWizardConfig,
} from './types';

describe('Workflow Types', () => {
  describe('WizardStep enum', () => {
    it('should have all 7 steps defined', () => {
      expect(WizardStep.Project).toBe(0);
      expect(WizardStep.Basic).toBe(1);
      expect(WizardStep.Tasks).toBe(2);
      expect(WizardStep.Models).toBe(3);
      expect(WizardStep.Terminals).toBe(4);
      expect(WizardStep.Commands).toBe(5);
      expect(WizardStep.Advanced).toBe(6);
    });
  });

  describe('WIZARD_STEPS metadata', () => {
    it('should have 7 steps with correct metadata', () => {
      expect(WIZARD_STEPS).toHaveLength(7);
      expect(WIZARD_STEPS[0]).toEqual({
        step: WizardStep.Project,
        name: '工作目录',
        description: '选择项目文件夹'
      });
    });
  });

  describe('getDefaultWizardConfig', () => {
    it('should return config with all required fields', () => {
      const config = getDefaultWizardConfig();

      expect(config).toHaveProperty('project');
      expect(config).toHaveProperty('basic');
      expect(config).toHaveProperty('tasks');
      expect(config).toHaveProperty('models');
      expect(config).toHaveProperty('terminals');
      expect(config).toHaveProperty('commands');
      expect(config).toHaveProperty('advanced');
    });

    it('should set default basic config with 1 task', () => {
      const config = getDefaultWizardConfig();
      expect(config.basic.taskCount).toBe(1);
      expect(config.basic.importFromKanban).toBe(false);
    });

    it('should set default target branch to main', () => {
      const config = getDefaultWizardConfig();
      expect(config.advanced.targetBranch).toBe('main');
    });
  });
});
```

**Step 2: Run test to verify it fails**

```bash
cd vibe-kanban-main/frontend
pnpm test types.test.ts
```

Expected: FAIL with "Cannot find module './types'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/types.ts`:

```typescript
// ============================================================================
// 工作流向导类型定义
// 对应设计文档 2026-01-16-orchestrator-design.md 第 11 章
// ============================================================================

/** 向导步骤枚举 */
export enum WizardStep {
  Project = 0,      // 步骤0: 工作目录
  Basic = 1,        // 步骤1: 基础配置
  Tasks = 2,        // 步骤2: 任务配置
  Models = 3,       // 步骤3: 模型配置
  Terminals = 4,    // 步骤4: 终端配置
  Commands = 5,     // 步骤5: 斜杠命令
  Advanced = 6,     // 步骤6: 高级配置
}

/** 向导步骤元数据 */
export const WIZARD_STEPS = [
  { step: WizardStep.Project, name: '工作目录', description: '选择项目文件夹' },
  { step: WizardStep.Basic, name: '基础配置', description: '工作流名称和任务数量' },
  { step: WizardStep.Tasks, name: '任务配置', description: '配置每个任务详情' },
  { step: WizardStep.Models, name: '模型配置', description: '配置 API 和可用模型' },
  { step: WizardStep.Terminals, name: '终端配置', description: '为任务分配终端' },
  { step: WizardStep.Commands, name: '斜杠命令', description: '配置执行命令' },
  { step: WizardStep.Advanced, name: '高级配置', description: '主 Agent 和合并配置' },
] as const;

/** Git 仓库状态 */
export interface GitStatus {
  isGitRepo: boolean;
  currentBranch?: string;
  remoteUrl?: string;
  isDirty: boolean;
  uncommittedChanges?: number;
}

/** 项目配置 (步骤0) */
export interface ProjectConfig {
  workingDirectory: string;
  gitStatus: GitStatus;
}

/** 基础配置 (步骤1) */
export interface BasicConfig {
  name: string;
  description?: string;
  taskCount: number;
  importFromKanban: boolean;
  kanbanTaskIds?: string[];
}

/** 任务配置 (步骤2) */
export interface TaskConfig {
  id: string;           // 临时 ID，用于前端标识
  name: string;
  description: string;  // AI 将根据此描述执行任务
  branch: string;       // Git 分支名
  terminalCount: number; // 此任务的串行终端数量
}

/** API 类型 */
export type ApiType = 'anthropic' | 'google' | 'openai' | 'openai-compatible';

/** 模型配置 (步骤3) */
export interface ModelConfig {
  id: string;           // 临时 ID
  displayName: string;  // 用户自定义显示名
  apiType: ApiType;
  baseUrl: string;
  apiKey: string;
  modelId: string;      // 实际模型 ID
  isVerified: boolean;  // 是否已验证连接
}

/** 终端配置 (步骤4) */
export interface TerminalConfig {
  id: string;           // 临时 ID
  taskId: string;       // 关联的任务 ID
  orderIndex: number;   // 在任务内的执行顺序
  cliTypeId: string;    // CLI 类型 (claude-code, gemini-cli, codex)
  modelConfigId: string; // 关联的模型配置 ID
  role?: string;        // 角色描述
}

/** 斜杠命令配置 (步骤5) */
export interface CommandConfig {
  enabled: boolean;
  presetIds: string[];  // 选中的命令预设 ID（按顺序）
}

/** 高级配置 (步骤6) */
export interface AdvancedConfig {
  orchestrator: {
    modelConfigId: string; // 主 Agent 使用的模型
  };
  errorTerminal: {
    enabled: boolean;
    cliTypeId?: string;
    modelConfigId?: string;
  };
  mergeTerminal: {
    cliTypeId: string;
    modelConfigId: string;
    runTestsBeforeMerge: boolean;
    pauseOnConflict: boolean;
  };
  targetBranch: string;
}

/** 完整的向导配置 */
export interface WizardConfig {
  project: ProjectConfig;
  basic: BasicConfig;
  tasks: TaskConfig[];
  models: ModelConfig[];
  terminals: TerminalConfig[];
  commands: CommandConfig;
  advanced: AdvancedConfig;
}

/** 向导状态 */
export interface WizardState {
  currentStep: WizardStep;
  config: WizardConfig;
  isSubmitting: boolean;
  errors: Record<string, string>;
}

/** 获取默认向导配置 */
export function getDefaultWizardConfig(): WizardConfig {
  return {
    project: {
      workingDirectory: '',
      gitStatus: { isGitRepo: false, isDirty: false },
    },
    basic: {
      name: '',
      taskCount: 1,
      importFromKanban: false,
    },
    tasks: [],
    models: [],
    terminals: [],
    commands: {
      enabled: false,
      presetIds: [],
    },
    advanced: {
      orchestrator: { modelConfigId: '' },
      errorTerminal: { enabled: false },
      mergeTerminal: {
        cliTypeId: '',
        modelConfigId: '',
        runTestsBeforeMerge: true,
        pauseOnConflict: true,
      },
      targetBranch: 'main',
    },
  };
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test types.test.ts
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/types.ts src/components/workflow/types.test.ts
git commit -m "feat(workflow): add type definitions for 7-step wizard"
```

---

## Task 2: Create StepIndicator Component

**Files:**
- Create: `src/components/workflow/StepIndicator.tsx`
- Create: `src/components/workflow/StepIndicator.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/StepIndicator.test.tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StepIndicator } from './StepIndicator';
import { WizardStep } from './types';

describe('StepIndicator', () => {
  it('should render all 7 steps', () => {
    render(
      <StepIndicator
        currentStep={WizardStep.Project}
        completedSteps={[]}
      />
    );

    expect(screen.getByText('工作目录')).toBeInTheDocument();
    expect(screen.getByText('基础配置')).toBeInTheDocument();
    expect(screen.getByText('任务配置')).toBeInTheDocument();
    expect(screen.getByText('模型配置')).toBeInTheDocument();
    expect(screen.getByText('终端配置')).toBeInTheDocument();
    expect(screen.getByText('斜杠命令')).toBeInTheDocument();
    expect(screen.getByText('高级配置')).toBeInTheDocument();
  });

  it('should mark current step as active', () => {
    const { container } = render(
      <StepIndicator
        currentStep={WizardStep.Basic}
        completedSteps={[]}
      />
    );

    const steps = container.querySelectorAll('div.rounded-full');
    expect(steps[1]).toHaveClass('border-brand', 'text-brand', 'bg-brand/10');
  });

  it('should mark completed steps with check icon', () => {
    render(
      <StepIndicator
        currentStep={WizardStep.Tasks}
        completedSteps={[WizardStep.Project, WizardStep.Basic]}
      />
    );

    // Check that completed steps have checkmarks
    const checkIcons = document.querySelectorAll('svg.lucide-check');
    expect(checkIcons.length).toBeGreaterThanOrEqual(2);
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test StepIndicator.test.tsx
```

Expected: FAIL with "Cannot find module './StepIndicator'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/StepIndicator.tsx`:

```tsx
import { cn } from '@/lib/utils';
import { WizardStep, WIZARD_STEPS } from './types';
import { Check } from 'lucide-react';

interface Props {
  currentStep: WizardStep;
  completedSteps: WizardStep[];
}

export function StepIndicator({ currentStep, completedSteps }: Props) {
  return (
    <div className="flex items-center justify-between w-full mb-8">
      {WIZARD_STEPS.map((stepInfo, index) => {
        const isCompleted = completedSteps.includes(stepInfo.step);
        const isCurrent = currentStep === stepInfo.step;
        const isPast = stepInfo.step < currentStep;

        return (
          <div key={stepInfo.step} className="flex items-center flex-1">
            {/* Step Circle */}
            <div className="flex flex-col items-center">
              <div
                className={cn(
                  'w-10 h-10 rounded-full flex items-center justify-center text-sm font-medium border-2 transition-colors',
                  isCompleted && 'bg-brand border-brand text-white',
                  isCurrent && !isCompleted && 'border-brand text-brand bg-brand/10',
                  !isCurrent && !isCompleted && 'border-muted text-low bg-secondary'
                )}
              >
                {isCompleted ? <Check className="w-5 h-5" /> : index}
              </div>
              <span
                className={cn(
                  'text-xs mt-2 text-center max-w-[80px]',
                  isCurrent ? 'text-normal font-medium' : 'text-low'
                )}
              >
                {stepInfo.name}
              </span>
            </div>

            {/* Connector Line */}
            {index < WIZARD_STEPS.length - 1 && (
              <div
                className={cn(
                  'flex-1 h-0.5 mx-2',
                  isPast || isCompleted ? 'bg-brand' : 'bg-muted'
                )}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test StepIndicator.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/StepIndicator.tsx src/components/workflow/StepIndicator.test.tsx
git commit -m "feat(workflow): add StepIndicator component"
```

---

## Task 3: Create WorkflowWizard Main Component

**Files:**
- Create: `src/components/workflow/WorkflowWizard.tsx`
- Create: `src/components/workflow/WorkflowWizard.test.tsx`
- Create: `src/components/workflow/steps/index.ts` (placeholder exports)

**Step 1: Write the failing test**

```typescript
// src/components/workflow/WorkflowWizard.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { WorkflowWizard } from './WorkflowWizard';
import { WizardStep } from './types';

describe('WorkflowWizard', () => {
  it('should render wizard with step indicator', () => {
    render(
      <WorkflowWizard
        onComplete={vi.fn()}
        onCancel={vi.fn()}
      />
    );

    expect(screen.getByText('创建工作流')).toBeInTheDocument();
    expect(screen.getByText('选择项目文件夹')).toBeInTheDocument();
  });

  it('should start at Project step (Step 0)', () => {
    render(
      <WorkflowWizard
        onComplete={vi.fn()}
        onCancel={vi.fn()}
      />
    );

    // Should show step 0 content
    expect(screen.getByText('选择项目文件夹')).toBeInTheDocument();
  });

  it('should call onCancel when cancel button clicked', async () => {
    const onCancel = vi.fn();
    render(
      <WorkflowWizard
        onComplete={vi.fn()}
        onCancel={onCancel}
      />
    );

    const cancelButton = screen.getByText('取消');
    fireEvent.click(cancelButton);

    await waitFor(() => {
      expect(onCancel).toHaveBeenCalledTimes(1);
    });
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test WorkflowWizard.test.tsx
```

Expected: FAIL with "Cannot find module './WorkflowWizard'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/steps/index.ts`:

```typescript
// Placeholder exports for step components
// These will be implemented in subsequent tasks

export { Step0Project } from './Step0Project';
export { Step1Basic } from './Step1Basic';
export { Step2Tasks } from './Step2Tasks';
export { Step3Models } from './Step3Models';
export { Step4Terminals } from './Step4Terminals';
export { Step5Commands } from './Step5Commands';
export { Step6Advanced } from './Step6Advanced';
```

Create `src/components/workflow/WorkflowWizard.tsx`:

```tsx
import { useState, useCallback } from 'react';
import { Button } from '@/components/ui-new/primitives/PrimaryButton';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui-new/primitives/Dialog';
import { X } from 'lucide-react';
import { StepIndicator } from './StepIndicator';
import {
  WizardStep,
  WizardConfig,
  WizardState,
  WIZARD_STEPS,
  getDefaultWizardConfig,
} from './types';

// 步骤组件导入
import { Step0Project } from './steps';
import { Step1Basic } from './steps';
import { Step2Tasks } from './steps';
import { Step3Models } from './steps';
import { Step4Terminals } from './steps';
import { Step5Commands } from './steps';
import { Step6Advanced } from './steps';

interface Props {
  onComplete: (config: WizardConfig) => Promise<void>;
  onCancel: () => void;
}

export function WorkflowWizard({ onComplete, onCancel }: Props) {
  const [state, setState] = useState<WizardState>({
    currentStep: WizardStep.Project,
    config: getDefaultWizardConfig(),
    isSubmitting: false,
    errors: {},
  });

  const [completedSteps, setCompletedSteps] = useState<WizardStep[]>([]);

  // 更新配置
  const updateConfig = useCallback(<K extends keyof WizardConfig>(
    key: K,
    value: WizardConfig[K]
  ) => {
    setState(prev => ({
      ...prev,
      config: { ...prev.config, [key]: value },
    }));
  }, []);

  // 验证当前步骤
  const validateCurrentStep = (): boolean => {
    const { currentStep, config } = state;
    const errors: Record<string, string> = {};

    switch (currentStep) {
      case WizardStep.Project:
        if (!config.project.workingDirectory) {
          errors.workingDirectory = '请选择工作目录';
        }
        break;
      case WizardStep.Basic:
        if (!config.basic.name.trim()) {
          errors.name = '请输入工作流名称';
        }
        if (config.basic.taskCount < 1) {
          errors.taskCount = '至少需要一个任务';
        }
        break;
      case WizardStep.Tasks:
        if (config.tasks.some(t => !t.name.trim() || !t.description.trim())) {
          errors.tasks = '请完成所有任务的配置';
        }
        break;
      case WizardStep.Models:
        if (config.models.length === 0) {
          errors.models = '至少需要配置一个模型';
        }
        break;
      case WizardStep.Terminals:
        if (config.terminals.some(t => !t.cliTypeId || !t.modelConfigId)) {
          errors.terminals = '请完成所有终端的配置';
        }
        break;
      case WizardStep.Advanced:
        if (!config.advanced.orchestrator.modelConfigId) {
          errors.orchestrator = '请选择主 Agent 模型';
        }
        if (!config.advanced.mergeTerminal.cliTypeId) {
          errors.mergeTerminal = '请配置合并终端';
        }
        break;
    }

    setState(prev => ({ ...prev, errors }));
    return Object.keys(errors).length === 0;
  };

  // 下一步
  const handleNext = () => {
    if (!validateCurrentStep()) return;

    setCompletedSteps(prev => [...prev, state.currentStep]);
    setState(prev => ({
      ...prev,
      currentStep: prev.currentStep + 1,
    }));
  };

  // 上一步
  const handleBack = () => {
    if (state.currentStep > 0) {
      setState(prev => ({
        ...prev,
        currentStep: prev.currentStep - 1,
      }));
    }
  };

  // 提交
  const handleSubmit = async () => {
    if (!validateCurrentStep()) return;

    setState(prev => ({ ...prev, isSubmitting: true }));
    try {
      await onComplete(state.config);
    } catch (error) {
      console.error('Failed to create workflow:', error);
      setState(prev => ({
        ...prev,
        errors: { submit: '创建工作流失败，请重试' },
      }));
    } finally {
      setState(prev => ({ ...prev, isSubmitting: false }));
    }
  };

  // 渲染当前步骤
  const renderStep = () => {
    const { currentStep, config, errors } = state;

    switch (currentStep) {
      case WizardStep.Project:
        return (
          <Step0Project
            config={config.project}
            onChange={value => updateConfig('project', value)}
            errors={errors}
          />
        );
      case WizardStep.Basic:
        return (
          <Step1Basic
            config={config.basic}
            onChange={value => updateConfig('basic', value)}
            errors={errors}
          />
        );
      case WizardStep.Tasks:
        return (
          <Step2Tasks
            config={config.tasks}
            taskCount={config.basic.taskCount}
            onChange={value => updateConfig('tasks', value)}
            errors={errors}
          />
        );
      case WizardStep.Models:
        return (
          <Step3Models
            config={config.models}
            onChange={value => updateConfig('models', value)}
            errors={errors}
          />
        );
      case WizardStep.Terminals:
        return (
          <Step4Terminals
            config={config.terminals}
            tasks={config.tasks}
            models={config.models}
            onChange={value => updateConfig('terminals', value)}
            errors={errors}
          />
        );
      case WizardStep.Commands:
        return (
          <Step5Commands
            config={config.commands}
            onChange={value => updateConfig('commands', value)}
            errors={errors}
          />
        );
      case WizardStep.Advanced:
        return (
          <Step6Advanced
            config={config.advanced}
            models={config.models}
            onChange={value => updateConfig('advanced', value)}
            errors={errors}
          />
        );
    }
  };

  const currentStepInfo = WIZARD_STEPS[state.currentStep];
  const isLastStep = state.currentStep === WizardStep.Advanced;
  const isFirstStep = state.currentStep === WizardStep.Project;

  return (
    <Card className="w-full max-w-4xl mx-auto bg-panel">
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle className="text-xl text-high">创建工作流</CardTitle>
          <p className="text-sm text-low mt-1">{currentStepInfo.description}</p>
        </div>
        <button
          onClick={onCancel}
          className="flex items-center justify-center bg-secondary rounded text-low hover:text-normal"
        >
          <X className="w-5 h-5" />
        </button>
      </CardHeader>

      <CardContent>
        <StepIndicator
          currentStep={state.currentStep}
          completedSteps={completedSteps}
        />

        <div className="min-h-[400px]">
          {renderStep()}
        </div>

        {state.errors.submit && (
          <p className="text-error text-sm mt-4">{state.errors.submit}</p>
        )}

        <div className="flex justify-between mt-8 pt-4 border-t">
          <button
            onClick={isFirstStep ? onCancel : handleBack}
            disabled={state.isSubmitting}
            className="px-base bg-secondary rounded border text-low hover:text-normal"
          >
            {isFirstStep ? '取消' : '上一步'}
          </button>
          <button
            onClick={isLastStep ? handleSubmit : handleNext}
            disabled={state.isSubmitting}
            className="px-base bg-brand rounded text-white hover:bg-brand/90"
          >
            {state.isSubmitting
              ? '创建中...'
              : isLastStep
              ? '创建工作流'
              : '下一步'}
          </button>
        </div>
      </CardContent>
    </Card>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test WorkflowWizard.test.tsx
```

Expected: PASS (but will have console warnings about missing step components)

**Step 5: Commit**

```bash
git add src/components/workflow/WorkflowWizard.tsx src/components/workflow/WorkflowWizard.test.tsx src/components/workflow/steps/index.ts
git commit -m "feat(workflow): add WorkflowWizard main component"
```

---

## Task 4: Create Step0Project Component (Work Directory Selection)

**Files:**
- Create: `src/components/workflow/steps/Step0Project.tsx`
- Create: `src/components/workflow/steps/Step0Project.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/steps/Step0Project.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Step0Project } from './Step0Project';
import type { ProjectConfig } from '../types';

describe('Step0Project', () => {
  const mockOnChange = vi.fn();

  const defaultConfig: ProjectConfig = {
    workingDirectory: '',
    gitStatus: {
      isGitRepo: false,
      isDirty: false,
    },
  };

  it('should render folder selection UI', () => {
    render(
      <Step0Project
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('选择项目工作目录')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('点击浏览选择文件夹...')).toBeInTheDocument();
  });

  it('should show error when working directory is empty', () => {
    render(
      <Step0Project
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{ workingDirectory: '请选择工作目录' }}
      />
    );

    expect(screen.getByText('请选择工作目录')).toBeInTheDocument();
  });

  it('should display git repo status when directory is selected', () => {
    const configWithGit: ProjectConfig = {
      workingDirectory: '/path/to/project',
      gitStatus: {
        isGitRepo: true,
        currentBranch: 'main',
        isDirty: false,
      },
    };

    render(
      <Step0Project
        config={configWithGit}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('检测到 Git 仓库')).toBeInTheDocument();
    expect(screen.getByText(/当前分支:/)).toBeInTheDocument();
  });

  it('should show init git option when not a git repo', () => {
    const configWithoutGit: ProjectConfig = {
      workingDirectory: '/path/to/project',
      gitStatus: {
        isGitRepo: false,
        isDirty: false,
      },
    };

    render(
      <Step0Project
        config={configWithoutGit}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('未检测到 Git 仓库')).toBeInTheDocument();
    expect(screen.getByText('初始化 Git 仓库')).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test Step0Project.test.tsx
```

Expected: FAIL with "Cannot find module './Step0Project'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/steps/Step0Project.tsx`:

```tsx
import { useState, useCallback } from 'react';
import { InputField } from '@/components/ui-new/primitives/InputField';
import { Label } from '@/components/ui-new/primitives/Label';
import { PrimaryButton } from '@/components/ui-new/primitives/PrimaryButton';
import { Folder, GitBranch, AlertTriangle, Check, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { ProjectConfig, GitStatus } from '../types';

interface Props {
  config: ProjectConfig;
  onChange: (config: ProjectConfig) => void;
  errors: Record<string, string>;
}

export function Step0Project({ config, onChange, errors }: Props) {
  const [isChecking, setIsChecking] = useState(false);

  // 选择文件夹（通过 Tauri/Electron API）
  const handleSelectFolder = useCallback(async () => {
    try {
      // @ts-ignore - window.__TAURI__ 在 Tauri 环境中可用
      const selected = await window.__TAURI__?.dialog?.open({
        directory: true,
        multiple: false,
        title: '选择项目工作目录',
      });

      if (selected && typeof selected === 'string') {
        setIsChecking(true);
        // 检测 Git 状态
        const gitStatus = await checkGitStatus(selected);
        onChange({
          workingDirectory: selected,
          gitStatus,
        });
        setIsChecking(false);
      }
    } catch (error) {
      console.error('Failed to select folder:', error);
      setIsChecking(false);
    }
  }, [onChange]);

  // 检测 Git 状态
  const checkGitStatus = async (path: string): Promise<GitStatus> => {
    try {
      const response = await fetch('/api/git/status', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path }),
      });
      return await response.json();
    } catch {
      return { isGitRepo: false, isDirty: false };
    }
  };

  // 初始化 Git 仓库
  const handleInitGit = async () => {
    if (!config.workingDirectory) return;

    setIsChecking(true);
    try {
      await fetch('/api/git/init', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: config.workingDirectory }),
      });
      const gitStatus = await checkGitStatus(config.workingDirectory);
      onChange({ ...config, gitStatus });
    } catch (error) {
      console.error('Failed to init git:', error);
    }
    setIsChecking(false);
  };

  return (
    <div className="space-y-6">
      {/* 文件夹选择 */}
      <div className="space-y-2">
        <Label>选择项目工作目录</Label>
        <div className="flex gap-2">
          <InputField
            value={config.workingDirectory}
            placeholder="点击浏览选择文件夹..."
            readOnly
            className="flex-1"
          />
          <PrimaryButton
            variant="secondary"
            onClick={handleSelectFolder}
            disabled={isChecking}
          >
            <Folder className="w-4 h-4 mr-2" />
            浏览...
          </PrimaryButton>
        </div>
        {errors.workingDirectory && (
          <p className="text-error text-sm">{errors.workingDirectory}</p>
        )}
      </div>

      {/* Git 状态检测 */}
      {config.workingDirectory && (
        <div className="border rounded-lg p-4 bg-secondary">
          <div className="flex items-center gap-2 mb-3">
            <GitBranch className="w-5 h-5" />
            <span className="font-medium">Git 状态检测</span>
            {isChecking && <RefreshCw className="w-4 h-4 animate-spin" />}
          </div>

          {config.gitStatus.isGitRepo ? (
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-success">
                <Check className="w-4 h-4" />
                <span>检测到 Git 仓库</span>
              </div>
              <div className="text-sm text-low space-y-1 pl-6">
                <p>当前分支: <span className="text-normal">{config.gitStatus.currentBranch}</span></p>
                {config.gitStatus.remoteUrl && (
                  <p>远程仓库: <span className="text-normal">{config.gitStatus.remoteUrl}</span></p>
                )}
                <p>
                  工作区状态:{' '}
                  <span className={cn(config.gitStatus.isDirty ? 'text-warning' : 'text-success')}>
                    {config.gitStatus.isDirty
                      ? `有 ${config.gitStatus.uncommittedChanges || '未知'} 个未提交更改`
                      : '干净 (无未提交更改)'}
                  </span>
                </p>
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex items-center gap-2 text-warning">
                <AlertTriangle className="w-4 h-4" />
                <span>未检测到 Git 仓库</span>
              </div>
              <p className="text-sm text-low pl-6">
                此文件夹不是 Git 仓库。SoloDawn 需要 Git 来协调多终端工作流。
              </p>
              <div className="flex gap-2 pl-6">
                <PrimaryButton onClick={handleInitGit} disabled={isChecking}>
                  初始化 Git 仓库
                </PrimaryButton>
                <PrimaryButton variant="secondary" onClick={handleSelectFolder}>
                  选择其他文件夹
                </PrimaryButton>
              </div>
              <p className="text-xs text-low pl-6">
                初始化将执行: git init → 创建 .gitignore → git add . && git commit
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test Step0Project.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/steps/Step0Project.tsx src/components/workflow/steps/Step0Project.test.tsx
git commit -m "feat(workflow): add Step0Project component"
```

---

## Task 5: Create Step1Basic Component (Basic Configuration)

**Files:**
- Create: `src/components/workflow/steps/Step1Basic.tsx`
- Create: `src/components/workflow/steps/Step1Basic.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/steps/Step1Basic.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Step1Basic } from './Step1Basic';
import type { BasicConfig } from '../types';

describe('Step1Basic', () => {
  const mockOnChange = vi.fn();

  const defaultConfig: BasicConfig = {
    name: '',
    taskCount: 1,
    importFromKanban: false,
  };

  it('should render basic configuration form', () => {
    render(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('工作流名称')).toBeInTheDocument();
    expect(screen.getByText('本次启动几个并行任务？')).toBeInTheDocument();
  });

  it('should display error when name is empty', () => {
    render(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{ name: '请输入工作流名称' }}
      />
    );

    expect(screen.getByText('请输入工作流名称')).toBeInTheDocument();
  });

  it('should allow selecting task count', () => {
    render(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const twoTasksButton = screen.getByText('2 个任务');
    fireEvent.click(twoTasksButton);

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({ taskCount: 2 })
    );
  });

  it('should allow switching between import modes', () => {
    render(
      <Step1Basic
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const importRadio = screen.getByText('从看板导入（选择已有任务卡片）');
    fireEvent.click(importRadio);

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({ importFromKanban: true })
    );
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test Step1Basic.test.tsx
```

Expected: FAIL with "Cannot find module './Step1Basic'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/steps/Step1Basic.tsx`:

```tsx
import { InputField } from '@/components/ui-new/primitives/InputField';
import { Label } from '@/components/ui-new/primitives/Label';
import { Field } from '@/components/ui-new/primitives/Field';
import { cn } from '@/lib/utils';
import type { BasicConfig } from '../types';

interface Props {
  config: BasicConfig;
  onChange: (config: BasicConfig) => void;
  errors: Record<string, string>;
}

const TASK_COUNT_OPTIONS = [1, 2, 3, 4];

export function Step1Basic({ config, onChange, errors }: Props) {
  return (
    <div className="space-y-6">
      {/* 工作流名称 */}
      <div className="space-y-2">
        <Label>工作流名称 *</Label>
        <InputField
          value={config.name}
          onChange={e => onChange({ ...config, name: e.target.value })}
          placeholder="例如：用户系统重构"
        />
        {errors.name && <p className="text-error text-sm">{errors.name}</p>}
      </div>

      {/* 描述 */}
      <div className="space-y-2">
        <Label>描述（可选）</Label>
        <Field
          value={config.description || ''}
          onChange={e => onChange({ ...config, description: e.target.value })}
          placeholder="工作流的整体目标和描述..."
          multiline
          rows={3}
        />
      </div>

      {/* 任务数量选择 */}
      <div className="space-y-3">
        <Label>本次启动几个并行任务？</Label>
        <div className="flex gap-3 flex-wrap">
          {TASK_COUNT_OPTIONS.map(count => (
            <button
              key={count}
              type="button"
              onClick={() => onChange({ ...config, taskCount: count })}
              className={cn(
                'px-4 py-2 rounded border text-sm font-medium transition-colors',
                config.taskCount === count
                  ? 'bg-brand border-brand text-white'
                  : 'bg-secondary border-muted text-normal hover:border-brand'
              )}
            >
              {count} 个任务
            </button>
          ))}
          <div className="flex items-center gap-2">
            <span className="text-low">更多:</span>
            <InputField
              type="number"
              min={5}
              max={10}
              value={config.taskCount > 4 ? config.taskCount : ''}
              onChange={e => {
                const val = parseInt(e.target.value);
                if (val >= 1 && val <= 10) {
                  onChange({ ...config, taskCount: val });
                }
              }}
              className="w-16"
              placeholder="5-10"
            />
          </div>
        </div>
        {errors.taskCount && <p className="text-error text-sm">{errors.taskCount}</p>}
      </div>

      {/* 导入选项 */}
      <div className="space-y-3">
        <Label>是否从看板导入已有任务？</Label>
        <div className="space-y-2">
          <label className="flex items-center space-x-2 cursor-pointer">
            <input
              type="radio"
              name="import-mode"
              checked={!config.importFromKanban}
              onChange={() => onChange({ ...config, importFromKanban: false })}
              className="w-4 h-4"
            />
            <span className="font-normal">新建任务（下一步手动配置）</span>
          </label>
          <label className="flex items-center space-x-2 cursor-pointer">
            <input
              type="radio"
              name="import-mode"
              checked={config.importFromKanban}
              onChange={() => onChange({ ...config, importFromKanban: true })}
              className="w-4 h-4"
            />
            <span className="font-normal">从看板导入（选择已有任务卡片）</span>
          </label>
        </div>
      </div>
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test Step1Basic.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/steps/Step1Basic.tsx src/components/workflow/steps/Step1Basic.test.tsx
git commit -m "feat(workflow): add Step1Basic component"
```

---

## Task 6: Create Step2Tasks Component (Task Configuration)

**Files:**
- Create: `src/components/workflow/steps/Step2Tasks.tsx`
- Create: `src/components/workflow/steps/Step2Tasks.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/steps/Step2Tasks.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Step2Tasks } from './Step2Tasks';
import type { TaskConfig } from '../types';

describe('Step2Tasks', () => {
  const mockOnChange = vi.fn();

  const mockTasks: TaskConfig[] = [
    {
      id: '1',
      name: 'Task 1',
      description: 'First task',
      branch: 'feat/task-1',
      terminalCount: 1,
    },
  ];

  it('should render task configuration form', () => {
    render(
      <Step2Tasks
        config={mockTasks}
        taskCount={2}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText(/配置 2 个并行任务/)).toBeInTheDocument();
    expect(screen.getByText('任务 1/2')).toBeInTheDocument();
  });

  it('should initialize empty tasks when config length mismatches', () => {
    render(
      <Step2Tasks
        config={[]}
        taskCount={2}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    // Should trigger onChange with initialized tasks
    expect(mockOnChange).toHaveBeenCalled();
  });

  it('should auto-generate branch name from task name', () => {
    const emptyTasks: TaskConfig[] = [
      {
        id: '1',
        name: '',
        description: '',
        branch: '',
        terminalCount: 1,
      },
    ];

    render(
      <Step2Tasks
        config={emptyTasks}
        taskCount={1}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const nameInput = screen.getByPlaceholderText('例如：登录功能');
    fireEvent.change(nameInput, { target: { value: 'User Login Feature' } });

    // Should auto-generate branch name
    const updatedCalls = mockOnChange.mock.calls;
    const lastCall = updatedCalls[updatedCalls.length - 1];
    expect(lastCall[0][0].branch).toBe('feat/user-login-feature');
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test Step2Tasks.test.tsx
```

Expected: FAIL with "Cannot find module './Step2Tasks'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/steps/Step2Tasks.tsx`:

```tsx
import { useState, useEffect } from 'react';
import { InputField } from '@/components/ui-new/primitives/InputField';
import { Label } from '@/components/ui-new/primitives/Label';
import { Field } from '@/components/ui-new/primitives/Field';
import { PrimaryButton } from '@/components/ui-new/primitives/PrimaryButton';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { TaskConfig } from '../types';
import { v4 as uuid } from 'uuid';

interface Props {
  config: TaskConfig[];
  taskCount: number;
  onChange: (config: TaskConfig[]) => void;
  errors: Record<string, string>;
}

const TERMINAL_COUNT_OPTIONS = [1, 2, 3];

export function Step2Tasks({ config, taskCount, onChange, errors }: Props) {
  const [currentTaskIndex, setCurrentTaskIndex] = useState(0);

  // 初始化任务列表
  useEffect(() => {
    if (config.length !== taskCount) {
      const newTasks: TaskConfig[] = [];
      for (let i = 0; i < taskCount; i++) {
        if (config[i]) {
          newTasks.push(config[i]);
        } else {
          newTasks.push({
            id: uuid(),
            name: '',
            description: '',
            branch: '',
            terminalCount: 1,
          });
        }
      }
      onChange(newTasks);
    }
  }, [taskCount, config.length]);

  const currentTask = config[currentTaskIndex];

  // 更新当前任务
  const updateTask = (updates: Partial<TaskConfig>) => {
    const newTasks = [...config];
    newTasks[currentTaskIndex] = { ...currentTask, ...updates };

    // 自动生成分支名
    if (updates.name && !currentTask.branch) {
      const slug = updates.name
        .toLowerCase()
        .replace(/[^a-z0-9\u4e00-\u9fa5]+/g, '-')
        .replace(/^-|-$/g, '');
      newTasks[currentTaskIndex].branch = `feat/${slug}`;
    }

    onChange(newTasks);
  };

  if (!currentTask) return null;

  const isTaskComplete = currentTask.name.trim() && currentTask.description.trim();

  return (
    <div className="space-y-6">
      {/* 任务导航 */}
      <div className="flex items-center justify-between">
        <span className="text-lg font-medium">
          配置 {taskCount} 个并行任务
        </span>
        <div className="flex items-center gap-2">
          <span className="text-sm text-low">
            任务 {currentTaskIndex + 1}/{taskCount}
          </span>
          <PrimaryButton
            variant="secondary"
            onClick={() => setCurrentTaskIndex(i => Math.max(0, i - 1))}
            disabled={currentTaskIndex === 0}
          >
            <ChevronLeft className="w-4 h-4" />
          </PrimaryButton>
          <PrimaryButton
            variant="secondary"
            onClick={() => setCurrentTaskIndex(i => Math.min(taskCount - 1, i + 1))}
            disabled={currentTaskIndex === taskCount - 1}
          >
            <ChevronRight className="w-4 h-4" />
          </PrimaryButton>
        </div>
      </div>

      {/* 任务配置表单 */}
      <div className="border rounded-lg p-6 bg-secondary/50">
        <div className="flex items-center gap-2 mb-4">
          <span className="text-sm font-medium text-low">任务 {currentTaskIndex + 1}</span>
          {isTaskComplete && (
            <span className="text-xs px-2 py-0.5 rounded bg-success/20 text-success">已配置</span>
          )}
        </div>

        <div className="space-y-4">
          {/* 任务名称 */}
          <div className="space-y-2">
            <Label>任务名称 *</Label>
            <InputField
              value={currentTask.name}
              onChange={e => updateTask({ name: e.target.value })}
              placeholder="例如：登录功能"
            />
          </div>

          {/* Git 分支名称 */}
          <div className="space-y-2">
            <Label>Git 分支名称</Label>
            <InputField
              value={currentTask.branch}
              onChange={e => updateTask({ branch: e.target.value })}
              placeholder="自动生成，可修改"
            />
            <p className="text-xs text-low">
              建议格式: feat/xxx, fix/xxx, refactor/xxx
            </p>
          </div>

          {/* 任务描述 */}
          <div className="space-y-2">
            <Label>任务描述 (AI 将根据此描述执行任务) *</Label>
            <Field
              value={currentTask.description}
              onChange={e => updateTask({ description: e.target.value })}
              placeholder={`实现${currentTask.name || '功能'}:\n1. 具体步骤一\n2. 具体步骤二\n3. 具体步骤三`}
              multiline
              rows={8}
              className="font-mono text-sm"
            />
            <p className="text-xs text-low">支持 Markdown 格式，描述越详细，AI 执行越准确</p>
          </div>

          {/* 终端数量 */}
          <div className="space-y-2">
            <Label>此任务需要几个终端串行执行？</Label>
            <div className="flex gap-2">
              {TERMINAL_COUNT_OPTIONS.map(count => (
                <button
                  key={count}
                  type="button"
                  onClick={() => updateTask({ terminalCount: count })}
                  className={cn(
                    'px-4 py-2 rounded border text-sm',
                    currentTask.terminalCount === count
                      ? 'bg-brand border-brand text-white'
                      : 'bg-secondary border-muted hover:border-brand'
                  )}
                >
                  {count} 个
                </button>
              ))}
              <InputField
                type="number"
                min={4}
                max={5}
                value={currentTask.terminalCount > 3 ? currentTask.terminalCount : ''}
                onChange={e => {
                  const val = parseInt(e.target.value);
                  if (val >= 1) updateTask({ terminalCount: val });
                }}
                className="w-20"
                placeholder="更多"
              />
            </div>
          </div>
        </div>
      </div>

      {/* 进度指示 */}
      <div className="flex items-center gap-2">
        <span className="text-sm text-low">任务进度:</span>
        <div className="flex-1 flex gap-1">
          {config.map((task, i) => (
            <button
              key={task.id}
              onClick={() => setCurrentTaskIndex(i)}
              className={cn(
                'flex-1 h-2 rounded transition-colors',
                task.name && task.description ? 'bg-brand' : 'bg-muted',
                i === currentTaskIndex && 'ring-2 ring-brand ring-offset-1'
              )}
            />
          ))}
        </div>
        <span className="text-sm text-low">
          {config.filter(t => t.name && t.description).length} / {taskCount} 已配置
        </span>
      </div>

      {errors.tasks && <p className="text-error text-sm">{errors.tasks}</p>}
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test Step2Tasks.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/steps/Step2Tasks.tsx src/components/workflow/steps/Step2Tasks.test.tsx
git commit -m "feat(workflow): add Step2Tasks component"
```

---

## Task 7: Create Step3Models Component (Model Configuration)

**Files:**
- Create: `src/components/workflow/steps/Step3Models.tsx`
- Create: `src/components/workflow/steps/Step3Models.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/steps/Step3Models.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Step3Models } from './Step3Models';
import type { ModelConfig } from '../types';

describe('Step3Models', () => {
  const mockOnChange = vi.fn();

  const mockModels: ModelConfig[] = [
    {
      id: '1',
      displayName: 'Claude Sonnet',
      apiType: 'anthropic',
      baseUrl: 'https://api.anthropic.com',
      apiKey: 'sk-test',
      modelId: 'claude-3-5-sonnet-20241022',
      isVerified: true,
    },
  ];

  it('should render model configuration UI', () => {
    render(
      <Step3Models
        config={mockModels}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('配置可用模型')).toBeInTheDocument();
    expect(screen.getByText('Claude Sonnet')).toBeInTheDocument();
  });

  it('should show empty state when no models configured', () => {
    render(
      <Step3Models
        config={[]}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('尚未配置任何模型')).toBeInTheDocument();
  });

  it('should display error when no models configured', () => {
    render(
      <Step3Models
        config={[]}
        onChange={mockOnChange}
        errors={{ models: '至少需要配置一个模型' }}
      />
    );

    expect(screen.getByText('至少需要配置一个模型')).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test Step3Models.test.tsx
```

Expected: FAIL with "Cannot find module './Step3Models'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/steps/Step3Models.tsx`:

```tsx
import { useState } from 'react';
import { PrimaryButton } from '@/components/ui-new/primitives/PrimaryButton';
import { InputField } from '@/components/ui-new/primitives/InputField';
import { Label } from '@/components/ui-new/primitives/Label';
import { Dialog } from '@/components/ui-new/primitives/Dialog';
import { Plus, Pencil, Trash2, RefreshCw, Check, Eye, EyeOff } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { ModelConfig, ApiType } from '../types';
import { v4 as uuid } from 'uuid';

interface Props {
  config: ModelConfig[];
  onChange: (config: ModelConfig[]) => void;
  errors: Record<string, string>;
}

const API_TYPES: { value: ApiType; label: string; defaultUrl: string }[] = [
  { value: 'anthropic', label: 'Anthropic (官方)', defaultUrl: 'https://api.anthropic.com' },
  { value: 'google', label: 'Google (Gemini)', defaultUrl: 'https://generativelanguage.googleapis.com' },
  { value: 'openai', label: 'OpenAI', defaultUrl: 'https://api.openai.com' },
  { value: 'openai-compatible', label: 'OpenAI 兼容', defaultUrl: '' },
];

export function Step3Models({ config, onChange, errors }: Props) {
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingModel, setEditingModel] = useState<ModelConfig | null>(null);

  const handleAddModel = (model: ModelConfig) => {
    if (editingModel) {
      onChange(config.map(m => m.id === model.id ? model : m));
    } else {
      onChange([...config, model]);
    }
    setIsDialogOpen(false);
    setEditingModel(null);
  };

  const handleEdit = (model: ModelConfig) => {
    setEditingModel(model);
    setIsDialogOpen(true);
  };

  const handleDelete = (id: string) => {
    onChange(config.filter(m => m.id !== id));
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="font-medium">配置可用模型 (cc-switch)</h3>
          <p className="text-sm text-low">这些模型将在终端配置中供选择</p>
        </div>
        <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
          <Dialog.Trigger asChild>
            <PrimaryButton onClick={() => setEditingModel(null)}>
              <Plus className="w-4 h-4 mr-2" />
              添加模型
            </PrimaryButton>
          </Dialog.Trigger>
          <Dialog.Content className="max-w-lg">
            <Dialog.Header>
              <Dialog.Title>{editingModel ? '编辑模型' : '添加模型'}</Dialog.Title>
            </Dialog.Header>
            <AddModelForm
              initialModel={editingModel}
              onSubmit={handleAddModel}
              onCancel={() => setIsDialogOpen(false)}
            />
          </Dialog.Content>
        </Dialog>
      </div>

      {/* 已配置的模型列表 */}
      <div className="space-y-3">
        {config.length === 0 ? (
          <div className="text-center py-12 border-2 border-dashed rounded-lg">
            <p className="text-low">尚未配置任何模型</p>
            <p className="text-sm text-low mt-1">点击"添加模型"开始配置</p>
          </div>
        ) : (
          config.map(model => (
            <div
              key={model.id}
              className="flex items-center justify-between p-4 border rounded-lg bg-secondary"
            >
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{model.displayName}</span>
                  {model.isVerified && (
                    <span className="text-xs px-2 py-0.5 rounded bg-success/20 text-success flex items-center gap-1">
                      <Check className="w-3 h-3" /> 已验证
                    </span>
                  )}
                </div>
                <p className="text-sm text-low">
                  API: {API_TYPES.find(t => t.value === model.apiType)?.label} | 模型: {model.modelId}
                </p>
                {model.apiType === 'openai-compatible' && (
                  <p className="text-xs text-low">Base: {model.baseUrl}</p>
                )}
              </div>
              <div className="flex gap-2">
                <PrimaryButton variant="secondary" onClick={() => handleEdit(model)}>
                  <Pencil className="w-4 h-4" />
                </PrimaryButton>
                <PrimaryButton variant="secondary" onClick={() => handleDelete(model.id)}>
                  <Trash2 className="w-4 h-4" />
                </PrimaryButton>
              </div>
            </div>
          ))
        )}
      </div>

      {errors.models && <p className="text-error text-sm">{errors.models}</p>}

      <p className="text-sm text-low">
        提示: 至少需要配置一个模型才能继续
      </p>
    </div>
  );
}

// 添加/编辑模型表单
function AddModelForm({
  initialModel,
  onSubmit,
  onCancel,
}: {
  initialModel: ModelConfig | null;
  onSubmit: (model: ModelConfig) => void;
  onCancel: () => void;
}) {
  const [model, setModel] = useState<ModelConfig>(
    initialModel || {
      id: uuid(),
      displayName: '',
      apiType: 'anthropic',
      baseUrl: 'https://api.anthropic.com',
      apiKey: '',
      modelId: '',
      isVerified: false,
    }
  );
  const [showApiKey, setShowApiKey] = useState(false);
  const [fetchingModels, setFetchingModels] = useState(false);
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  const [verifying, setVerifying] = useState(false);

  // 获取可用模型
  const handleFetchModels = async () => {
    if (!model.apiKey || !model.baseUrl) return;

    setFetchingModels(true);
    try {
      const response = await fetch('/api/models/list', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          apiType: model.apiType,
          baseUrl: model.baseUrl,
          apiKey: model.apiKey,
        }),
      });
      const data = await response.json();
      setAvailableModels(data.models || []);
    } catch (error) {
      console.error('Failed to fetch models:', error);
    }
    setFetchingModels(false);
  };

  // 验证连接
  const handleVerify = async () => {
    if (!model.apiKey || !model.modelId) return;

    setVerifying(true);
    try {
      const response = await fetch('/api/models/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          apiType: model.apiType,
          baseUrl: model.baseUrl,
          apiKey: model.apiKey,
          modelId: model.modelId,
        }),
      });
      const data = await response.json();
      setModel(m => ({ ...m, isVerified: data.success }));
    } catch (error) {
      console.error('Failed to verify:', error);
    }
    setVerifying(false);
  };

  const handleApiTypeChange = (apiType: ApiType) => {
    const defaultUrl = API_TYPES.find(t => t.value === apiType)?.defaultUrl || '';
    setModel(m => ({ ...m, apiType, baseUrl: defaultUrl }));
    setAvailableModels([]);
  };

  return (
    <div className="space-y-4">
      {/* 模型名称 */}
      <div className="space-y-2">
        <Label>模型名称 (自定义显示名)</Label>
        <InputField
          value={model.displayName}
          onChange={e => setModel(m => ({ ...m, displayName: e.target.value }))}
          placeholder="例如: Claude Sonnet"
        />
      </div>

      {/* API 类型 */}
      <div className="space-y-2">
        <Label>API 类型</Label>
        <div className="flex flex-wrap gap-2">
          {API_TYPES.map(type => (
            <button
              key={type.value}
              type="button"
              onClick={() => handleApiTypeChange(type.value)}
              className={cn(
                'px-3 py-1.5 rounded border text-sm',
                model.apiType === type.value
                  ? 'bg-brand border-brand text-white'
                  : 'bg-secondary border-muted hover:border-brand'
              )}
            >
              {type.label}
            </button>
          ))}
        </div>
      </div>

      {/* Base URL */}
      <div className="space-y-2">
        <Label>Base URL</Label>
        <InputField
          value={model.baseUrl}
          onChange={e => setModel(m => ({ ...m, baseUrl: e.target.value }))}
          placeholder="https://api.example.com"
          disabled={model.apiType !== 'openai-compatible'}
        />
      </div>

      {/* API Key */}
      <div className="space-y-2">
        <Label>API Key</Label>
        <div className="flex gap-2">
          <div className="relative flex-1">
            <InputField
              type={showApiKey ? 'text' : 'password'}
              value={model.apiKey}
              onChange={e => setModel(m => ({ ...m, apiKey: e.target.value, isVerified: false }))}
              placeholder="sk-xxx..."
            />
            <button
              type="button"
              onClick={() => setShowApiKey(!showApiKey)}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-low hover:text-normal"
            >
              {showApiKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            </button>
          </div>
        </div>
      </div>

      {/* 获取可用模型 */}
      <div className="space-y-2 p-3 border rounded-lg bg-secondary/50">
        <PrimaryButton
          variant="secondary"
          onClick={handleFetchModels}
          disabled={!model.apiKey || !model.baseUrl || fetchingModels}
        >
          {fetchingModels ? (
            <RefreshCw className="w-4 h-4 mr-2 animate-spin" />
          ) : (
            <RefreshCw className="w-4 h-4 mr-2" />
          )}
          获取可用模型
        </PrimaryButton>
        {availableModels.length > 0 && (
          <p className="text-sm text-success">
            ✓ 成功获取 {availableModels.length} 个可用模型
          </p>
        )}
      </div>

      {/* 模型选择 */}
      <div className="space-y-2">
        <Label>模型选择</Label>
        {availableModels.length > 0 ? (
          <select
            value={model.modelId}
            onChange={e => setModel(m => ({ ...m, modelId: e.target.value, isVerified: false }))}
            className="w-full px-base bg-secondary rounded border text-base"
          >
            <option value="">选择模型</option>
            {availableModels.map(m => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
        ) : (
          <InputField
            value={model.modelId}
            onChange={e => setModel(m => ({ ...m, modelId: e.target.value, isVerified: false }))}
            placeholder="手动输入模型 ID"
          />
        )}
      </div>

      {/* 验证连接 */}
      <div className="flex items-center gap-3">
        <PrimaryButton variant="secondary" onClick={handleVerify} disabled={!model.modelId || verifying}>
          {verifying ? '验证中...' : '验证连接'}
        </PrimaryButton>
        {model.isVerified && (
          <span className="text-sm text-success flex items-center gap-1">
            <Check className="w-4 h-4" /> 连接成功，模型可用
          </span>
        )}
      </div>

      {/* 操作按钮 */}
      <div className="flex justify-end gap-2 pt-4 border-t">
        <PrimaryButton variant="secondary" onClick={onCancel}>取消</PrimaryButton>
        <PrimaryButton
          onClick={() => onSubmit(model)}
          disabled={!model.displayName || !model.apiKey || !model.modelId}
        >
          保存模型
        </PrimaryButton>
      </div>
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test Step3Models.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/steps/Step3Models.tsx src/components/workflow/steps/Step3Models.test.tsx
git commit -m "feat(workflow): add Step3Models component"
```

---

## Task 8: Create Step4Terminals Component (Terminal Configuration)

**Files:**
- Create: `src/components/workflow/steps/Step4Terminals.tsx`
- Create: `src/components/workflow/steps/Step4Terminals.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/steps/Step4Terminals.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Step4Terminals } from './Step4Terminals';
import type { TerminalConfig, TaskConfig, ModelConfig } from '../types';

describe('Step4Terminals', () => {
  const mockOnChange = vi.fn();

  const mockTasks: TaskConfig[] = [
    {
      id: 'task-1',
      name: 'Task 1',
      description: 'First task',
      branch: 'feat/task-1',
      terminalCount: 2,
    },
  ];

  const mockModels: ModelConfig[] = [
    {
      id: 'model-1',
      displayName: 'Claude Sonnet',
      apiType: 'anthropic',
      baseUrl: 'https://api.anthropic.com',
      apiKey: 'sk-test',
      modelId: 'claude-3-5-sonnet-20241022',
      isVerified: true,
    },
  ];

  const mockTerminals: TerminalConfig[] = [
    {
      id: 'term-1',
      taskId: 'task-1',
      orderIndex: 0,
      cliTypeId: 'claude-code',
      modelConfigId: 'model-1',
    },
    {
      id: 'term-2',
      taskId: 'task-1',
      orderIndex: 1,
      cliTypeId: 'claude-code',
      modelConfigId: 'model-1',
    },
  ];

  it('should render terminal configuration UI', () => {
    render(
      <Step4Terminals
        config={mockTerminals}
        tasks={mockTasks}
        models={mockModels}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('配置终端')).toBeInTheDocument();
    expect(screen.getByText(/任务: Task 1/)).toBeInTheDocument();
  });

  it('should show terminal count for task', () => {
    render(
      <Step4Terminals
        config={mockTerminals}
        tasks={mockTasks}
        models={mockModels}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('此任务有 2 个串行终端')).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test Step4Terminals.test.tsx
```

Expected: FAIL with "Cannot find module './Step4Terminals'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/steps/Step4Terminals.tsx`:

```tsx
import { useState, useEffect } from 'react';
import { PrimaryButton } from '@/components/ui-new/primitives/PrimaryButton';
import { InputField } from '@/components/ui-new/primitives/InputField';
import { Label } from '@/components/ui-new/primitives/Label';
import { ChevronLeft, ChevronRight, Check, X, ExternalLink } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { TerminalConfig, TaskConfig, ModelConfig } from '../types';
import { v4 as uuid } from 'uuid';

interface CliTypeInfo {
  id: string;
  name: string;
  displayName: string;
  installed: boolean;
  installGuideUrl?: string;
}

interface Props {
  config: TerminalConfig[];
  tasks: TaskConfig[];
  models: ModelConfig[];
  onChange: (config: TerminalConfig[]) => void;
  errors: Record<string, string>;
}

const CLI_TYPES: CliTypeInfo[] = [
  { id: 'claude-code', name: 'claude-code', displayName: 'Claude Code', installed: false },
  { id: 'gemini-cli', name: 'gemini-cli', displayName: 'Gemini CLI', installed: false },
  { id: 'codex', name: 'codex', displayName: 'Codex', installed: false },
  { id: 'cursor-agent', name: 'cursor-agent', displayName: 'Cursor Agent', installed: false, installGuideUrl: 'https://cursor.com' },
];

export function Step4Terminals({ config, tasks, models, onChange, errors }: Props) {
  const [currentTaskIndex, setCurrentTaskIndex] = useState(0);
  const [cliTypes, setCliTypes] = useState<CliTypeInfo[]>(CLI_TYPES);

  // 检测 CLI 安装状态
  useEffect(() => {
    fetch('/api/cli_types/detect')
      .then(res => res.json())
      .then((data: CliTypeInfo[]) => {
        setCliTypes(data);
      })
      .catch(() => {});
  }, []);

  // 初始化终端配置
  useEffect(() => {
    const totalTerminals = tasks.reduce((sum, t) => sum + t.terminalCount, 0);
    if (config.length !== totalTerminals) {
      const newTerminals: TerminalConfig[] = [];
      tasks.forEach(task => {
        for (let i = 0; i < task.terminalCount; i++) {
          const existing = config.find(
            t => t.taskId === task.id && t.orderIndex === i
          );
          newTerminals.push(
            existing || {
              id: uuid(),
              taskId: task.id,
              orderIndex: i,
              cliTypeId: '',
              modelConfigId: '',
            }
          );
        }
      });
      onChange(newTerminals);
    }
  }, [tasks]);

  const currentTask = tasks[currentTaskIndex];
  const taskTerminals = config.filter(t => t.taskId === currentTask?.id);

  const updateTerminal = (terminalId: string, updates: Partial<TerminalConfig>) => {
    onChange(config.map(t => t.id === terminalId ? { ...t, ...updates } : t));
  };

  if (!currentTask) return null;

  return (
    <div className="space-y-6">
      {/* 任务导航 */}
      <div className="flex items-center justify-between">
        <div>
          <span className="text-lg font-medium">配置终端</span>
          <span className="text-low ml-2">- 任务: {currentTask.name}</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm text-low">任务 {currentTaskIndex + 1}/{tasks.length}</span>
          <PrimaryButton
            variant="secondary"
            onClick={() => setCurrentTaskIndex(i => Math.max(0, i - 1))}
            disabled={currentTaskIndex === 0}
          >
            <ChevronLeft className="w-4 h-4" />
          </PrimaryButton>
          <PrimaryButton
            variant="secondary"
            onClick={() => setCurrentTaskIndex(i => Math.min(tasks.length - 1, i + 1))}
            disabled={currentTaskIndex === tasks.length - 1}
          >
            <ChevronRight className="w-4 h-4" />
          </PrimaryButton>
        </div>
      </div>

      <p className="text-sm text-low">此任务有 {currentTask.terminalCount} 个串行终端</p>

      {/* 终端配置列表 */}
      <div className="space-y-4">
        {taskTerminals
          .sort((a, b) => a.orderIndex - b.orderIndex)
          .map((terminal, idx) => (
            <div key={terminal.id} className="border rounded-lg p-4 bg-secondary/50">
              <div className="flex items-center gap-2 mb-4">
                <span className="font-medium">终端 {idx + 1}</span>
                {idx === 0 && <span className="text-xs text-low">(第一个执行)</span>}
                {idx > 0 && <span className="text-xs text-low">(等待终端{idx}完成后执行)</span>}
              </div>

              <div className="space-y-4">
                {/* CLI 选择 */}
                <div className="space-y-2">
                  <Label>CLI 选择</Label>
                  <div className="grid grid-cols-2 gap-2">
                    {cliTypes.map(cli => (
                      <button
                        key={cli.id}
                        type="button"
                        onClick={() => updateTerminal(terminal.id, { cliTypeId: cli.id })}
                        disabled={!cli.installed}
                        className={cn(
                          'flex items-center justify-between p-3 rounded border text-left',
                          terminal.cliTypeId === cli.id
                            ? 'bg-brand/10 border-brand'
                            : 'bg-secondary border-muted',
                          !cli.installed && 'opacity-50 cursor-not-allowed'
                        )}
                      >
                        <div className="flex items-center gap-2">
                          {terminal.cliTypeId === cli.id && <div className="w-2 h-2 rounded-full bg-brand" />}
                          <span>{cli.displayName}</span>
                        </div>
                        <div className="flex items-center gap-1 text-xs">
                          {cli.installed ? (
                            <span className="text-success flex items-center gap-1">
                              <Check className="w-3 h-3" /> 已安装
                            </span>
                          ) : (
                            <span className="text-error flex items-center gap-1">
                              <X className="w-3 h-3" /> 未安装
                              {cli.installGuideUrl && (
                                <a
                                  href={cli.installGuideUrl}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className="text-brand hover:underline"
                                  onClick={e => e.stopPropagation()}
                                >
                                  <ExternalLink className="w-3 h-3" />
                                </a>
                              )}
                            </span>
                          )}
                        </div>
                      </button>
                    ))}
                  </div>
                </div>

                {/* 模型选择 */}
                <div className="space-y-2">
                  <Label>模型选择 (从步骤3配置的模型中选择)</Label>
                  <select
                    value={terminal.modelConfigId}
                    onChange={e => updateTerminal(terminal.id, { modelConfigId: e.target.value })}
                    className="w-full px-base bg-secondary rounded border text-base"
                  >
                    <option value="">选择模型</option>
                    {models.map(m => (
                      <option key={m.id} value={m.id}>{m.displayName}</option>
                    ))}
                  </select>
                </div>

                {/* 角色描述 */}
                <div className="space-y-2">
                  <Label>角色描述 (可选)</Label>
                  <InputField
                    value={terminal.role || ''}
                    onChange={e => updateTerminal(terminal.id, { role: e.target.value })}
                    placeholder="例如: 代码编写者、代码审核者"
                  />
                </div>
              </div>
            </div>
          ))}
      </div>

      {errors.terminals && <p className="text-error text-sm">{errors.terminals}</p>}

      {cliTypes.some(c => !c.installed && taskTerminals.some(t => t.cliTypeId === c.id)) && (
        <p className="text-warning text-sm">
          ⚠️ 选择了未安装的 CLI 将无法进入下一步
        </p>
      )}
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test Step4Terminals.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/steps/Step4Terminals.tsx src/components/workflow/steps/Step4Terminals.test.tsx
git commit -m "feat(workflow): add Step4Terminals component"
```

---

## Task 9: Create Step5Commands Component (Slash Commands Configuration)

**Files:**
- Create: `src/components/workflow/steps/Step5Commands.tsx`
- Create: `src/components/workflow/steps/Step5Commands.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/steps/Step5Commands.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Step5Commands } from './Step5Commands';
import type { CommandConfig } from '../types';

describe('Step5Commands', () => {
  const mockOnChange = vi.fn();

  const defaultConfig: CommandConfig = {
    enabled: false,
    presetIds: [],
  };

  it('should render command configuration UI', () => {
    render(
      <Step5Commands
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('是否配置斜杠命令？')).toBeInTheDocument();
  });

  it('should show command list when enabled', () => {
    const enabledConfig: CommandConfig = {
      enabled: true,
      presetIds: ['write-code', 'review'],
    };

    render(
      <Step5Commands
        config={enabledConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('已选命令 (按执行顺序排列)')).toBeInTheDocument();
  });

  it('should toggle enabled state', () => {
    render(
      <Step5Commands
        config={defaultConfig}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    const yesRadio = screen.getByText('配置斜杠命令 - 主 Agent 按命令顺序分发任务');
    fireEvent.click(yesRadio);

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: true })
    );
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test Step5Commands.test.tsx
```

Expected: FAIL with "Cannot find module './Step5Commands'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/steps/Step5Commands.tsx`:

```tsx
import { useState, useEffect } from 'react';
import { PrimaryButton } from '@/components/ui-new/primitives/PrimaryButton';
import { Label } from '@/components/ui-new/primitives/Label';
import { GripVertical, Plus, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { CommandConfig } from '../types';

interface CommandPreset {
  id: string;
  name: string;
  displayName: string;
  description: string;
  isSystem: boolean;
}

interface Props {
  config: CommandConfig;
  onChange: (config: CommandConfig) => void;
  errors: Record<string, string>;
}

const SYSTEM_PRESETS: CommandPreset[] = [
  { id: 'write-code', name: '/write-code', displayName: '编写代码', description: '编写功能代码', isSystem: true },
  { id: 'review', name: '/review', displayName: '代码审核', description: '代码审计，检查安全性和代码质量', isSystem: true },
  { id: 'fix-issues', name: '/fix-issues', displayName: '修复问题', description: '修复发现的问题', isSystem: true },
  { id: 'test', name: '/test', displayName: '测试', description: '编写和运行测试', isSystem: true },
  { id: 'refactor', name: '/refactor', displayName: '重构', description: '重构代码结构', isSystem: true },
];

export function Step5Commands({ config, onChange, errors }: Props) {
  const [presets, setPresets] = useState<CommandPreset[]>(SYSTEM_PRESETS);

  // 加载预设列表
  useEffect(() => {
    fetch('/api/workflows/presets/commands')
      .then(res => res.json())
      .then(data => setPresets([...SYSTEM_PRESETS, ...data.filter((p: CommandPreset) => !p.isSystem)]))
      .catch(() => {});
  }, []);

  const selectedPresets = config.presetIds
    .map(id => presets.find(p => p.id === id))
    .filter(Boolean) as CommandPreset[];

  const availablePresets = presets.filter(p => !config.presetIds.includes(p.id));

  const addPreset = (id: string) => {
    onChange({ ...config, presetIds: [...config.presetIds, id] });
  };

  const removePreset = (id: string) => {
    onChange({ ...config, presetIds: config.presetIds.filter(p => p !== id) });
  };

  const clearAll = () => {
    onChange({ ...config, presetIds: [] });
  };

  const resetDefault = () => {
    onChange({ ...config, presetIds: ['write-code', 'review', 'fix-issues'] });
  };

  // 拖拽排序（简化版）
  const moveUp = (index: number) => {
    if (index === 0) return;
    const newIds = [...config.presetIds];
    [newIds[index - 1], newIds[index]] = [newIds[index], newIds[index - 1]];
    onChange({ ...config, presetIds: newIds });
  };

  const moveDown = (index: number) => {
    if (index === config.presetIds.length - 1) return;
    const newIds = [...config.presetIds];
    [newIds[index], newIds[index + 1]] = [newIds[index + 1], newIds[index]];
    onChange({ ...config, presetIds: newIds });
  };

  return (
    <div className="space-y-6">
      {/* 是否启用斜杠命令 */}
      <div className="space-y-3">
        <Label>是否配置斜杠命令？</Label>
        <div className="space-y-2">
          <label className="flex items-center space-x-2 cursor-pointer">
            <input
              type="radio"
              name="cmd-enabled"
              checked={!config.enabled}
              onChange={() => onChange({ ...config, enabled: false })}
              className="w-4 h-4"
            />
            <span className="font-normal">不配置 - 主 Agent 自行决策任务执行方式</span>
          </label>
          <label className="flex items-center space-x-2 cursor-pointer">
            <input
              type="radio"
              name="cmd-enabled"
              checked={config.enabled}
              onChange={() => onChange({ ...config, enabled: true })}
              className="w-4 h-4"
            />
            <span className="font-normal">配置斜杠命令 - 主 Agent 按命令顺序分发任务</span>
          </label>
        </div>
      </div>

      {config.enabled && (
        <>
          {/* 已选命令 */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label>已选命令 (按执行顺序排列)</Label>
              <div className="flex gap-2">
                <PrimaryButton variant="secondary" onClick={clearAll}>清空</PrimaryButton>
                <PrimaryButton variant="secondary" onClick={resetDefault}>重置默认</PrimaryButton>
              </div>
            </div>

            {selectedPresets.length === 0 ? (
              <div className="text-center py-8 border-2 border-dashed rounded-lg">
                <p className="text-low">尚未选择任何命令</p>
              </div>
            ) : (
              <div className="space-y-2">
                {selectedPresets.map((preset, index) => (
                  <div
                    key={preset.id}
                    className="flex items-center gap-3 p-3 border rounded-lg bg-secondary"
                  >
                    <div className="flex flex-col gap-1">
                      <button onClick={() => moveUp(index)} disabled={index === 0}>
                        <GripVertical className="w-4 h-4 text-low hover:text-normal" />
                      </button>
                    </div>
                    <span className="text-low w-6">{index + 1}.</span>
                    <span className="font-mono text-sm text-brand">{preset.name}</span>
                    <span className="text-sm text-low flex-1">{preset.description}</span>
                    <PrimaryButton variant="ghost" onClick={() => removePreset(preset.id)}>
                      <X className="w-4 h-4" />
                    </PrimaryButton>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* 可用命令 */}
          <div className="space-y-3">
            <Label>可用命令预设</Label>

            <div className="space-y-3">
              <p className="text-sm text-low">系统内置:</p>
              <div className="flex flex-wrap gap-2">
                {presets
                  .filter(p => p.isSystem && !config.presetIds.includes(p.id))
                  .map(preset => (
                    <button
                      key={preset.id}
                      onClick={() => addPreset(preset.id)}
                      className="px-3 py-2 border rounded-lg bg-secondary hover:border-brand flex items-center gap-2"
                    >
                      <span className="font-mono text-sm">{preset.name}</span>
                      <Plus className="w-4 h-4 text-low" />
                    </button>
                  ))}
              </div>

              {presets.some(p => !p.isSystem) && (
                <>
                  <p className="text-sm text-low mt-4">用户自定义:</p>
                  <div className="flex flex-wrap gap-2">
                    {presets
                      .filter(p => !p.isSystem && !config.presetIds.includes(p.id))
                      .map(preset => (
                        <button
                          key={preset.id}
                          onClick={() => addPreset(preset.id)}
                          className="px-3 py-2 border rounded-lg bg-secondary hover:border-brand flex items-center gap-2"
                        >
                          <span className="font-mono text-sm">{preset.name}</span>
                          <Plus className="w-4 h-4 text-low" />
                        </button>
                      ))}
                  </div>
                </>
              )}
            </div>
          </div>
        </>
      )}

      {errors.commands && <p className="text-error text-sm">{errors.commands}</p>}
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test Step5Commands.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/steps/Step5Commands.tsx src/components/workflow/steps/Step5Commands.test.tsx
git commit -m "feat(workflow): add Step5Commands component"
```

---

## Task 10: Create Step6Advanced Component (Advanced Configuration)

**Files:**
- Create: `src/components/workflow/steps/Step6Advanced.tsx`
- Create: `src/components/workflow/steps/Step6Advanced.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/steps/Step6Advanced.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Step6Advanced } from './Step6Advanced';
import type { AdvancedConfig, ModelConfig } from '../types';

describe('Step6Advanced', () => {
  const mockOnChange = vi.fn();

  const mockModels: ModelConfig[] = [
    {
      id: 'model-1',
      displayName: 'Claude Sonnet',
      apiType: 'anthropic',
      baseUrl: 'https://api.anthropic.com',
      apiKey: 'sk-test',
      modelId: 'claude-3-5-sonnet-20241022',
      isVerified: true,
    },
  ];

  const defaultConfig: AdvancedConfig = {
    orchestrator: { modelConfigId: '' },
    errorTerminal: { enabled: false },
    mergeTerminal: {
      cliTypeId: '',
      modelConfigId: '',
      runTestsBeforeMerge: true,
      pauseOnConflict: true,
    },
    targetBranch: 'main',
  };

  it('should render advanced configuration UI', () => {
    render(
      <Step6Advanced
        config={defaultConfig}
        models={mockModels}
        onChange={mockOnChange}
        errors={{}}
      />
    );

    expect(screen.getByText('主 Agent 配置')).toBeInTheDocument();
    expect(screen.getByText('错误处理终端')).toBeInTheDocument();
    expect(screen.getByText('合并终端配置')).toBeInTheDocument();
  });

  it('should show error when orchestrator model not selected', () => {
    render(
      <Step6Advanced
        config={defaultConfig}
        models={mockModels}
        onChange={mockOnChange}
        errors={{ orchestrator: '请选择主 Agent 模型' }}
      />
    );

    expect(screen.getByText('请选择主 Agent 模型')).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test Step6Advanced.test.tsx
```

Expected: FAIL with "Cannot find module './Step6Advanced'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/steps/Step6Advanced.tsx`:

```tsx
import { useState } from 'react';
import { Label } from '@/components/ui-new/primitives/Label';
import { InputField } from '@/components/ui-new/primitives/InputField';
import { Field } from '@/components/ui-new/primitives/Field';
import { ChevronDown, FileText } from 'lucide-react';
import type { AdvancedConfig, ModelConfig } from '../types';

interface Props {
  config: AdvancedConfig;
  models: ModelConfig[];
  onChange: (config: AdvancedConfig) => void;
  errors: Record<string, string>;
}

// Git 提交规范（系统强制，不可修改）
const GIT_COMMIT_FORMAT = `[Terminal:{terminal_id}] [Status:{status}] {简要摘要}

## 变更内容
- 详细描述本次提交的所有变更
- 每个文件的修改目的
- 新增/修改/删除了哪些功能

## 技术细节
- 使用的技术方案
- 关键代码逻辑说明
- 依赖变更说明（如有）

## 测试情况
- 已执行的测试
- 测试结果

---METADATA---
workflow_id: {workflow_id}
task_id: {task_id}
terminal_id: {terminal_id}
terminal_order: {order}
cli: {cli_type}
model: {model}
status: {completed|review_pass|review_reject|failed}
files_changed: [{file_path, change_type, lines_added, lines_deleted}]
execution_time_seconds: {seconds}
token_usage: {input_tokens, output_tokens}`;

export function Step6Advanced({ config, models, onChange, errors }: Props) {
  const [showCommitFormat, setShowCommitFormat] = useState(false);

  const updateOrchestrator = (updates: Partial<typeof config.orchestrator>) => {
    onChange({ ...config, orchestrator: { ...config.orchestrator, ...updates } });
  };

  const updateErrorTerminal = (updates: Partial<typeof config.errorTerminal>) => {
    onChange({ ...config, errorTerminal: { ...config.errorTerminal, ...updates } });
  };

  const updateMergeTerminal = (updates: Partial<typeof config.mergeTerminal>) => {
    onChange({ ...config, mergeTerminal: { ...config.mergeTerminal, ...updates } });
  };

  return (
    <div className="space-y-6">
      {/* 主 Agent 配置 */}
      <div className="border rounded-lg p-4 space-y-4">
        <Label className="text-base font-medium">主 Agent (Orchestrator) 配置</Label>
        <div className="space-y-2">
          <Label>选择模型 (从步骤3已配置的模型中选择)</Label>
          <select
            value={config.orchestrator.modelConfigId}
            onChange={e => updateOrchestrator({ modelConfigId: e.target.value })}
            className="w-full px-base bg-secondary rounded border text-base"
          >
            <option value="">选择模型</option>
            {models.map(m => (
              <option key={m.id} value={m.id}>{m.displayName}</option>
            ))}
          </select>
          <p className="text-xs text-low">推荐: 使用能力最强的模型作为主 Agent</p>
        </div>
        {errors.orchestrator && <p className="text-error text-sm">{errors.orchestrator}</p>}
      </div>

      {/* 错误处理终端 */}
      <div className="border rounded-lg p-4 space-y-4">
        <div className="flex items-center justify-between">
          <Label className="text-base font-medium">错误处理终端 (可选)</Label>
          <input
            type="checkbox"
            checked={config.errorTerminal.enabled}
            onChange={e => updateErrorTerminal({ enabled: e.target.checked })}
            className="w-4 h-4"
          />
        </div>
        {config.errorTerminal.enabled && (
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>CLI</Label>
              <select
                value={config.errorTerminal.cliTypeId}
                onChange={e => updateErrorTerminal({ cliTypeId: e.target.value })}
                className="w-full px-base bg-secondary rounded border text-base"
              >
                <option value="">选择 CLI</option>
                <option value="claude-code">Claude Code</option>
                <option value="gemini-cli">Gemini CLI</option>
                <option value="codex">Codex</option>
              </select>
            </div>
            <div className="space-y-2">
              <Label>模型</Label>
              <select
                value={config.errorTerminal.modelConfigId}
                onChange={e => updateErrorTerminal({ modelConfigId: e.target.value })}
                className="w-full px-base bg-secondary rounded border text-base"
              >
                <option value="">选择模型</option>
                {models.map(m => (
                  <option key={m.id} value={m.id}>{m.displayName}</option>
                ))}
              </select>
            </div>
          </div>
        )}
      </div>

      {/* 合并终端配置 */}
      <div className="border rounded-lg p-4 space-y-4">
        <Label className="text-base font-medium">合并终端配置</Label>
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>CLI</Label>
            <select
              value={config.mergeTerminal.cliTypeId}
              onChange={e => updateMergeTerminal({ cliTypeId: e.target.value })}
              className="w-full px-base bg-secondary rounded border text-base"
            >
              <option value="">选择 CLI</option>
              <option value="claude-code">Claude Code</option>
              <option value="gemini-cli">Gemini CLI</option>
              <option value="codex">Codex</option>
            </select>
          </div>
          <div className="space-y-2">
            <Label>模型</Label>
            <select
              value={config.mergeTerminal.modelConfigId}
              onChange={e => updateMergeTerminal({ modelConfigId: e.target.value })}
              className="w-full px-base bg-secondary rounded border text-base"
            >
              <option value="">选择模型</option>
              {models.map(m => (
                <option key={m.id} value={m.id}>{m.displayName}</option>
              ))}
            </select>
          </div>
        </div>
        <div className="flex items-center gap-6">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={config.mergeTerminal.runTestsBeforeMerge}
              onChange={e => updateMergeTerminal({ runTestsBeforeMerge: e.target.checked })}
              className="w-4 h-4"
            />
            <span className="text-sm">合并前运行测试</span>
          </label>
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={config.mergeTerminal.pauseOnConflict}
              onChange={e => updateMergeTerminal({ pauseOnConflict: e.target.checked })}
              className="w-4 h-4"
            />
            <span className="text-sm">合并冲突时暂停等待人工处理</span>
          </label>
        </div>
        {errors.mergeTerminal && <p className="text-error text-sm">{errors.mergeTerminal}</p>}
      </div>

      {/* 目标分支 */}
      <div className="space-y-2">
        <Label>目标分支</Label>
        <InputField
          value={config.targetBranch}
          onChange={e => onChange({ ...config, targetBranch: e.target.value })}
          placeholder="main"
        />
      </div>

      {/* Git 提交规范 */}
      <details open={showCommitFormat} onToggle={e => setShowCommitFormat((e.target as HTMLDetailsElement).open)}>
        <summary className="flex items-center gap-2 text-sm text-low hover:text-normal cursor-pointer">
          <FileText className="w-4 h-4" />
          <span>📋 Git 提交规范 (系统强制，不可修改)</span>
          <ChevronDown className={`w-4 h-4 transition-transform ${showCommitFormat ? 'rotate-180' : ''}`} />
        </summary>
        <div className="mt-3">
          <div className="border rounded-lg p-4 bg-secondary/50">
            <p className="text-sm text-low mb-2">
              系统要求每个终端完成任务后必须按以下格式提交 Git:
            </p>
            <pre className="text-xs font-mono bg-primary/10 p-3 rounded overflow-x-auto whitespace-pre-wrap">
              {GIT_COMMIT_FORMAT}
            </pre>
            <p className="text-xs text-low mt-2">
              此规范确保 Git 监测服务能准确识别终端状态和任务进度
            </p>
          </div>
        </div>
      </details>
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test Step6Advanced.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/steps/Step6Advanced.tsx src/components/workflow/steps/Step6Advanced.test.tsx
git commit -m "feat(workflow): add Step6Advanced component"
```

---

## Task 11: Create PipelineView Component

**Files:**
- Create: `src/components/workflow/PipelineView.tsx`
- Create: `src/components/workflow/PipelineView.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/PipelineView.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PipelineView } from './PipelineView';
import type { Workflow, WorkflowTask, Terminal } from '@/shared/types';

describe('PipelineView', () => {
  const mockWorkflow: Workflow = {
    id: 'wf-1',
    name: 'Test Workflow',
    projectId: 'proj-1',
    status: 'running',
    createdAt: new Date().toISOString(),
  };

  const mockTerminals: Terminal[] = [
    {
      id: 'term-1',
      taskId: 'task-1',
      orderIndex: 0,
      cliTypeId: 'claude-code',
      modelConfigId: 'model-1',
      status: 'completed',
    },
  ];

  const mockTasks: Array<WorkflowTask & { terminals: Terminal[] }> = [
    {
      id: 'task-1',
      workflowId: 'wf-1',
      name: 'Task 1',
      description: 'First task',
      branch: 'feat/task-1',
      status: 'running',
      terminals: mockTerminals,
    },
  ];

  it('should render pipeline view', () => {
    render(
      <PipelineView
        workflow={mockWorkflow}
        tasks={mockTasks}
        onTerminalClick={vi.fn()}
      />
    );

    expect(screen.getByText('Test Workflow')).toBeInTheDocument();
  });

  it('should display status badge', () => {
    render(
      <PipelineView
        workflow={mockWorkflow}
        tasks={mockTasks}
        onTerminalClick={vi.fn()}
      />
    );

    expect(screen.getByText('running')).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test PipelineView.test.tsx
```

Expected: FAIL with "Cannot find module './PipelineView'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/PipelineView.tsx`:

```tsx
import { TerminalCard } from './TerminalCard';
import type { Workflow, WorkflowTask, Terminal } from '@/shared/types';

interface Props {
  workflow: Workflow;
  tasks: Array<WorkflowTask & { terminals: Terminal[] }>;
  onTerminalClick?: (terminal: Terminal) => void;
}

export function PipelineView({ workflow, tasks, onTerminalClick }: Props) {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{workflow.name}</h2>
        <StatusBadge status={workflow.status} />
      </div>

      <div className="space-y-4">
        {tasks.map((task, taskIndex) => (
          <div key={task.id} className="p-4 border rounded-lg">
            <div className="flex items-center gap-2 mb-4">
              <span className="text-sm font-medium text-low">Task {taskIndex + 1}</span>
              <span className="font-medium">{task.name}</span>
              <span className="text-xs px-2 py-0.5 rounded bg-secondary">{task.branch}</span>
            </div>

            <div className="flex items-center gap-2">
              {task.terminals.map((terminal, terminalIndex) => (
                <div key={terminal.id} className="flex items-center">
                  <TerminalCard
                    terminal={terminal}
                    onClick={() => onTerminalClick?.(terminal)}
                  />
                  {terminalIndex < task.terminals.length - 1 && (
                    <div className="w-8 h-0.5 bg-low/30" />
                  )}
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>

      <div className="p-4 border-2 border-dashed rounded-lg text-center">
        <span className="text-low">合并终端</span>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    created: 'bg-gray-100 text-gray-800',
    starting: 'bg-yellow-100 text-yellow-800',
    ready: 'bg-blue-100 text-blue-800',
    running: 'bg-green-100 text-green-800',
    completed: 'bg-green-100 text-green-800',
    failed: 'bg-red-100 text-red-800',
  };

  return (
    <span className={`px-2 py-1 rounded text-sm ${colors[status] || 'bg-gray-100'}`}>
      {status}
    </span>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test PipelineView.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/PipelineView.tsx src/components/workflow/PipelineView.test.tsx
git commit -m "feat(workflow): add PipelineView component"
```

---

## Task 12: Create TerminalCard Component

**Files:**
- Create: `src/components/workflow/TerminalCard.tsx`
- Create: `src/components/workflow/TerminalCard.test.tsx`

**Step 1: Write the failing test**

```typescript
// src/components/workflow/TerminalCard.test.tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { TerminalCard } from './TerminalCard';
import type { Terminal } from '@/shared/types';

describe('TerminalCard', () => {
  const mockTerminal: Terminal = {
    id: 'term-1',
    taskId: 'task-1',
    orderIndex: 0,
    cliTypeId: 'claude-code',
    modelConfigId: 'model-1',
    status: 'working',
    role: 'Code Writer',
  };

  it('should render terminal card', () => {
    render(
      <TerminalCard
        terminal={mockTerminal}
        onClick={vi.fn()}
      />
    );

    expect(screen.getByText('Code Writer')).toBeInTheDocument();
    expect(screen.getByText('T1')).toBeInTheDocument();
  });

  it('should call onClick when clicked', () => {
    const handleClick = vi.fn();
    render(
      <TerminalCard
        terminal={mockTerminal}
        onClick={handleClick}
      />
    );

    const card = screen.getByText('Code Writer').closest('div');
    fireEvent.click(card!);

    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it('should display correct status styling', () => {
    const { container } = render(
      <TerminalCard
        terminal={mockTerminal}
        onClick={vi.fn()}
      />
    );

    const card = container.firstChild as HTMLElement;
    expect(card).toHaveClass('bg-green-50', 'border-green-400');
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test TerminalCard.test.tsx
```

Expected: FAIL with "Cannot find module './TerminalCard'"

**Step 3: Write minimal implementation**

Create `src/components/workflow/TerminalCard.tsx`:

```tsx
import { cn } from '@/lib/utils';
import type { Terminal } from '@/shared/types';

interface Props {
  terminal: Terminal;
  onClick?: () => void;
}

const STATUS_STYLES: Record<string, { bg: string; border: string; icon: string }> = {
  not_started: { bg: 'bg-gray-50', border: 'border-gray-200', icon: '○' },
  starting: { bg: 'bg-yellow-50', border: 'border-yellow-300', icon: '◐' },
  waiting: { bg: 'bg-blue-50', border: 'border-blue-300', icon: '◑' },
  working: { bg: 'bg-green-50', border: 'border-green-400', icon: '●' },
  completed: { bg: 'bg-green-100', border: 'border-green-500', icon: '✓' },
  failed: { bg: 'bg-red-50', border: 'border-red-400', icon: '✗' },
};

export function TerminalCard({ terminal, onClick }: Props) {
  const style = STATUS_STYLES[terminal.status] || STATUS_STYLES.not_started;

  return (
    <div
      className={cn(
        'w-32 p-3 rounded-lg border-2 cursor-pointer transition-all hover:shadow-md',
        style.bg,
        style.border
      )}
      onClick={onClick}
    >
      <div className="flex items-center justify-between mb-2">
        <span className="text-lg">{style.icon}</span>
        <span className="text-xs text-low">T{terminal.orderIndex + 1}</span>
      </div>
      <div className="text-sm font-medium truncate">{terminal.role || 'Terminal'}</div>
      <div className="text-xs text-low truncate">{terminal.cliTypeId}</div>
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test TerminalCard.test.tsx
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/components/workflow/TerminalCard.tsx src/components/workflow/TerminalCard.test.tsx
git commit -m "feat(workflow): add TerminalCard component"
```

---

## Task 13: Create API Hooks

**Files:**
- Create: `src/hooks/useWorkflows.ts`
- Create: `src/hooks/useWorkflows.test.ts`
- Create: `src/hooks/useCliTypes.ts`
- Create: `src/hooks/useCliTypes.test.ts`

**Step 1: Write the failing test**

```typescript
// src/hooks/useWorkflows.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useWorkflows, useCreateWorkflow } from './useWorkflows';

// Mock fetch
global.fetch = vi.fn();

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
};

describe('useWorkflows', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should fetch workflows for a project', async () => {
    const mockWorkflows = [
      { id: 'wf-1', name: 'Workflow 1', projectId: 'proj-1', status: 'running', createdAt: '2024-01-01' },
    ];

    (global.fetch as any).mockResolvedValueOnce({
      ok: true,
      json: async () => mockWorkflows,
    });

    const { result } = renderHook(() => useWorkflows('proj-1'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockWorkflows);
  });

  it('should handle fetch errors', async () => {
    (global.fetch as any).mockRejectedValueOnce(new Error('Network error'));

    const { result } = renderHook(() => useWorkflows('proj-1'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test useWorkflows.test.ts
```

Expected: FAIL with "Cannot find module './useWorkflows'"

**Step 3: Write minimal implementation**

Create `src/hooks/useWorkflows.ts`:

```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import type { Workflow, WorkflowDetailResponse, CreateWorkflowRequest } from '@/shared/types';

export function useWorkflows(projectId: string) {
  return useQuery({
    queryKey: ['workflows', projectId],
    queryFn: async () => {
      const res = await fetch(`/api/workflows?project_id=${projectId}`);
      if (!res.ok) throw new Error('Failed to fetch workflows');
      return res.json() as Promise<Workflow[]>;
    },
  });
}

export function useWorkflow(workflowId: string) {
  return useQuery({
    queryKey: ['workflow', workflowId],
    queryFn: async () => {
      const res = await fetch(`/api/workflows/${workflowId}`);
      if (!res.ok) throw new Error('Failed to fetch workflow');
      return res.json() as Promise<WorkflowDetailResponse>;
    },
    enabled: !!workflowId,
  });
}

export function useCreateWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CreateWorkflowRequest) => {
      const res = await fetch('/api/workflows', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      });
      if (!res.ok) throw new Error('Failed to create workflow');
      return res.json() as Promise<WorkflowDetailResponse>;
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['workflows', data.workflow.projectId] });
    },
  });
}

export function useStartWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (workflowId: string) => {
      const res = await fetch(`/api/workflows/${workflowId}/start`, { method: 'POST' });
      if (!res.ok) throw new Error('Failed to start workflow');
    },
    onSuccess: (_, workflowId) => {
      queryClient.invalidateQueries({ queryKey: ['workflow', workflowId] });
    },
  });
}

export function useDeleteWorkflow() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (workflowId: string) => {
      const res = await fetch(`/api/workflows/${workflowId}`, { method: 'DELETE' });
      if (!res.ok) throw new Error('Failed to delete workflow');
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workflows'] });
    },
  });
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test useWorkflows.test.ts
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/hooks/useWorkflows.ts src/hooks/useWorkflows.test.ts
git commit -m "feat(workflow): add useWorkflows hooks"
```

---

## Task 14: Create useCliTypes Hook

**Step 1: Write the failing test**

```typescript
// src/hooks/useCliTypes.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useCliTypes, useCliDetection } from './useCliTypes';

global.fetch = vi.fn();

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
};

describe('useCliTypes', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should fetch CLI types', async () => {
    const mockCliTypes = [
      { id: 'claude-code', name: 'claude-code', displayName: 'Claude Code' },
    ];

    (global.fetch as any).mockResolvedValueOnce({
      ok: true,
      json: async () => mockCliTypes,
    });

    const { result } = renderHook(() => useCliTypes(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockCliTypes);
  });

  it('should detect CLI installation status', async () => {
    const mockDetection = [
      { id: 'claude-code', installed: true },
    ];

    (global.fetch as any).mockResolvedValueOnce({
      ok: true,
      json: async () => mockDetection,
    });

    const { result } = renderHook(() => useCliDetection(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockDetection);
  });
});
```

**Step 2: Run test to verify it fails**

```bash
pnpm test useCliTypes.test.ts
```

Expected: FAIL with "Cannot find module './useCliTypes'"

**Step 3: Write minimal implementation**

Create `src/hooks/useCliTypes.ts`:

```typescript
import { useQuery } from '@tanstack/react-query';
import type { CliType, ModelConfig, CliDetectionStatus } from '@/shared/types';

export function useCliTypes() {
  return useQuery({
    queryKey: ['cliTypes'],
    queryFn: async () => {
      const res = await fetch('/api/cli_types');
      if (!res.ok) throw new Error('Failed to fetch CLI types');
      return res.json() as Promise<CliType[]>;
    },
  });
}

export function useCliDetection() {
  return useQuery({
    queryKey: ['cliDetection'],
    queryFn: async () => {
      const res = await fetch('/api/cli_types/detect');
      if (!res.ok) throw new Error('Failed to detect CLIs');
      return res.json() as Promise<CliDetectionStatus[]>;
    },
  });
}

export function useModelsForCli(cliTypeId: string) {
  return useQuery({
    queryKey: ['models', cliTypeId],
    queryFn: async () => {
      const res = await fetch(`/api/cli_types/${cliTypeId}/models`);
      if (!res.ok) throw new Error('Failed to fetch models');
      return res.json() as Promise<ModelConfig[]>;
    },
    enabled: !!cliTypeId,
  });
}
```

**Step 4: Run test to verify it passes**

```bash
pnpm test useCliTypes.test.ts
```

Expected: PASS

**Step 5: Commit**

```bash
git add src/hooks/useCliTypes.ts src/hooks/useCliTypes.test.ts
git commit -m "feat(workflow): add useCliTypes hooks"
```

---

## Task 15: Run All Tests and Verify

**Step 1: Run complete test suite**

```bash
cd vibe-kanban-main/frontend
pnpm test
```

Expected: All tests pass

**Step 2: Run TypeScript type checking**

```bash
pnpm run check
```

Expected: No type errors

**Step 3: Commit**

```bash
git add .
git commit -m "feat(workflow): complete Phase 6 frontend implementation with all tests passing"
```

---

## Summary

This plan implements the complete Phase 6 Frontend with:

✅ **Task 1:** Core type definitions with comprehensive TypeScript interfaces
✅ **Task 2:** StepIndicator component for visual progress tracking
✅ **Task 3:** WorkflowWizard main component with state management
✅ **Task 4:** Step0Project - Work directory selection with Git detection
✅ **Task 5:** Step1Basic - Basic workflow configuration
✅ **Task 6:** Step2Tasks - Task configuration with auto-generated branch names
✅ **Task 7:** Step3Models - Model configuration with API integration
✅ **Task 8:** Step4Terminals - Terminal configuration with CLI detection
✅ **Task 9:** Step5Commands - Slash command configuration
✅ **Task 10:** Step6Advanced - Advanced settings with Git commit format
✅ **Task 11:** PipelineView - Visual pipeline representation
✅ **Task 12:** TerminalCard - Terminal status card component
✅ **Task 13:** useWorkflows - React Query hooks for workflow API
✅ **Task 14:** useCliTypes - React Query hooks for CLI detection
✅ **Task 15:** Full test suite verification

**Total Tasks:** 15
**Total Files Created:** ~30 files (components + tests + hooks)

Each task follows TDD: Write test → Verify fail → Implement → Verify pass → Commit.
