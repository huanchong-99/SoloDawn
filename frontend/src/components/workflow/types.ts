// ============================================================================
// Workflow Wizard Types
// ============================================================================

/** Wizard step order */
export enum WizardStep {
  Project = 0,
  Basic = 1,
  Tasks = 2,
  Models = 3,
  Terminals = 4,
  Commands = 5,
  Advanced = 6,
}

export type WorkflowExecutionMode = 'diy' | 'agent_planned';

export interface WizardStepMeta {
  step: WizardStep;
  nameKey: string;
  descriptionKey: string;
}

/** Step metadata for the wizard indicator */
export const WIZARD_STEPS: readonly WizardStepMeta[] = [
  {
    step: WizardStep.Project,
    nameKey: 'steps.project.name',
    descriptionKey: 'steps.project.description',
  },
  {
    step: WizardStep.Basic,
    nameKey: 'steps.basic.name',
    descriptionKey: 'steps.basic.description',
  },
  {
    step: WizardStep.Tasks,
    nameKey: 'steps.tasks.name',
    descriptionKey: 'steps.tasks.description',
  },
  {
    step: WizardStep.Models,
    nameKey: 'steps.models.name',
    descriptionKey: 'steps.models.description',
  },
  {
    step: WizardStep.Terminals,
    nameKey: 'steps.terminals.name',
    descriptionKey: 'steps.terminals.description',
  },
  {
    step: WizardStep.Commands,
    nameKey: 'steps.commands.name',
    descriptionKey: 'steps.commands.description',
  },
  {
    step: WizardStep.Advanced,
    nameKey: 'steps.advanced.name',
    descriptionKey: 'steps.advanced.description',
  },
] as const;

export function isAgentPlannedMode(
  executionMode: WorkflowExecutionMode | undefined
): executionMode is 'agent_planned' {
  return executionMode === 'agent_planned';
}

export function getVisibleWizardSteps(
  executionMode: WorkflowExecutionMode = 'diy'
): WizardStepMeta[] {
  if (isAgentPlannedMode(executionMode)) {
    return WIZARD_STEPS.filter(
      (s) => s.step !== WizardStep.Tasks && s.step !== WizardStep.Terminals
    );
  }
  return [...WIZARD_STEPS];
}

export function getVisibleWizardStepIds(
  executionMode: WorkflowExecutionMode = 'diy'
): WizardStep[] {
  return getVisibleWizardSteps(executionMode).map((stepMeta) => stepMeta.step);
}

/** Git repository status */
export interface GitStatus {
  isGitRepo: boolean;
  currentBranch?: string;
  remoteUrl?: string;
  isDirty: boolean;
  uncommittedChanges?: number;
}

/** Project config (Step 0) */
export interface ProjectConfig {
  workingDirectory: string;
  gitStatus: GitStatus;
}

/** Basic config (Step 1) */
export interface BasicConfig {
  name: string;
  description?: string;
  executionMode: WorkflowExecutionMode;
  initialGoal?: string;
  taskCount: number;
  importFromKanban: boolean;
  kanbanTaskIds?: string[];
}

/** Task config (Step 2) */
export interface TaskConfig {
  id: string;
  name: string;
  description: string;
  branch: string;
  terminalCount: number;
}

/** API provider type */
export type ApiType = 'anthropic' | 'google' | 'openai' | 'openai-compatible';

/** Model config (Step 3) */
export interface ModelConfig {
  id: string;
  displayName: string;
  cliTypeId?: string; // Legacy models may not have this field yet.
  apiType: ApiType;
  baseUrl: string;
  apiKey: string;
  modelId: string;
  isVerified: boolean;
}

/** Terminal config (Step 4) */
export interface TerminalConfig {
  id: string;
  taskId: string;
  orderIndex: number;
  cliTypeId: string;
  modelConfigId: string;
  role?: string;
  autoConfirm?: boolean;
}

/** Slash command config (Step 5) */
export interface CommandConfig {
  enabled: boolean;
  presetIds: string[];
  customDescriptions?: Record<string, string>;
  additionalCommands?: Record<string, string[]>;
  customParams?: Record<string, Record<string, unknown>>;
}

/** Advanced config (Step 6) */
export interface AdvancedConfig {
  orchestrator: {
    modelConfigId: string;
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
  gitWatcherEnabled: boolean;
}

/** Full wizard config */
export interface WizardConfig {
  project: ProjectConfig;
  basic: BasicConfig;
  tasks: TaskConfig[];
  models: ModelConfig[];
  terminals: TerminalConfig[];
  commands: CommandConfig;
  advanced: AdvancedConfig;
}

/** Wizard state */
export interface WizardState {
  currentStep: WizardStep;
  config: WizardConfig;
  isSubmitting: boolean;
  errors: Record<string, string>;
}

/** Default wizard config */
export function getDefaultWizardConfig(): WizardConfig {
  return {
    project: {
      workingDirectory: '',
      gitStatus: { isGitRepo: false, isDirty: false },
    },
    basic: {
      name: '',
      executionMode: 'diy',
      initialGoal: '',
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
      gitWatcherEnabled: true,
    },
  };
}

// ============================================================================
// API Request Types (from useWorkflows.ts)
// ============================================================================

import type { CreateWorkflowRequest, InlineModelConfig } from '@/hooks/useWorkflows';

/**
 * Transform WizardConfig to CreateWorkflowRequest.
 * Matches backend API contract at workflows_dto.rs
 */
export function wizardConfigToCreateRequest(
  projectId: string,
  config: WizardConfig
): CreateWorkflowRequest {
  const executionMode = config.basic.executionMode;
  const isAgentPlanned = isAgentPlannedMode(executionMode);

  const isModelCompatibleWithCli = (
    model: ModelConfig,
    cliTypeId: string
  ): boolean => {
    const boundCliTypeId = model.cliTypeId?.trim();
    if (!boundCliTypeId) {
      return true;
    }
    return boundCliTypeId === cliTypeId;
  };

  const resolveTerminalModel = (
    modelConfigId: string,
    cliTypeId: string,
    context: string
  ): ModelConfig => {
    const model = config.models.find((candidate) => candidate.id === modelConfigId);
    if (!model) {
      throw new Error(`Model not found for ${context}`);
    }
    if (!isModelCompatibleWithCli(model, cliTypeId)) {
      throw new Error(
        `Model "${model.displayName}" is bound to "${model.cliTypeId}" and cannot be used by CLI "${cliTypeId}" in ${context}`
      );
    }
    return model;
  };

  // Build orchestrator config from models.
  const orchestratorModel = config.models.find(
    (model) => model.id === config.advanced.orchestrator.modelConfigId
  );
  if (!orchestratorModel) {
    throw new Error('Orchestrator model not found in configured models');
  }

  // Helper to create inline model config from model ID.
  const toInlineModelConfig = (
    modelConfigId?: string
  ): InlineModelConfig | undefined => {
    const model = config.models.find((candidate) => candidate.id === modelConfigId);
    return model
      ? { displayName: model.displayName, modelId: model.modelId }
      : undefined;
  };

  const tasks = isAgentPlanned
    ? []
    : config.tasks.map((task, taskIndex) => {
        const taskTerminals = config.terminals
          .filter((terminal) => terminal.taskId === task.id)
          .sort((left, right) => left.orderIndex - right.orderIndex);

        if (taskTerminals.length !== task.terminalCount) {
          throw new Error(
            `Task "${task.name}" terminals mismatch: expected ${task.terminalCount}, got ${taskTerminals.length}`
          );
        }

        return {
          id: task.id,
          name: task.name,
          description: task.description,
          branch: task.branch,
          orderIndex: taskIndex,
          terminals: taskTerminals.map((terminal) => {
            const model = resolveTerminalModel(
              terminal.modelConfigId,
              terminal.cliTypeId,
              `terminal ${terminal.id}`
            );

            return {
              id: terminal.id,
              cliTypeId: terminal.cliTypeId,
              modelConfigId: terminal.modelConfigId,
              modelConfig: {
                displayName: model.displayName,
                modelId: model.modelId,
              },
              customBaseUrl: model.baseUrl || null,
              customApiKey: model.apiKey || null,
              role: terminal.role,
              roleDescription: undefined,
              autoConfirm: terminal.autoConfirm ?? true,
              orderIndex: terminal.orderIndex,
            };
          }),
        };
      });

  if (config.advanced.errorTerminal.enabled) {
    const errorTerminalCliTypeId = config.advanced.errorTerminal.cliTypeId?.trim();
    const errorTerminalModelConfigId =
      config.advanced.errorTerminal.modelConfigId?.trim();

    if (!errorTerminalCliTypeId || !errorTerminalModelConfigId) {
      throw new Error('Error terminal config is incomplete');
    }

    resolveTerminalModel(
      errorTerminalModelConfigId,
      errorTerminalCliTypeId,
      'error terminal'
    );
  }

  const mergeCliTypeId = config.advanced.mergeTerminal.cliTypeId.trim();
  const mergeModelConfigId = config.advanced.mergeTerminal.modelConfigId.trim();
  if (!mergeCliTypeId || !mergeModelConfigId) {
    throw new Error('Merge terminal config is incomplete');
  }
  resolveTerminalModel(mergeModelConfigId, mergeCliTypeId, 'merge terminal');

  const request: CreateWorkflowRequest = {
    projectId,
    name: config.basic.name,
    description: config.basic.description,
    executionMode,
    initialGoal: isAgentPlanned
      ? config.basic.initialGoal?.trim() || undefined
      : undefined,
    useSlashCommands: config.commands.enabled,
    commandPresetIds:
      config.commands.presetIds.length > 0
        ? config.commands.presetIds
        : undefined,
    commands: config.commands.presetIds.map((presetId, index) => ({
      presetId,
      orderIndex: index,
      customParams: config.commands.customParams?.[presetId]
        ? JSON.stringify(config.commands.customParams[presetId])
        : null,
    })),
    orchestratorConfig: {
      apiType: orchestratorModel.apiType,
      baseUrl: orchestratorModel.baseUrl,
      apiKey: orchestratorModel.apiKey,
      model: orchestratorModel.modelId,
    },
    errorTerminalConfig: config.advanced.errorTerminal.enabled
      ? {
          cliTypeId: config.advanced.errorTerminal.cliTypeId!,
          modelConfigId: config.advanced.errorTerminal.modelConfigId!,
          modelConfig: toInlineModelConfig(
            config.advanced.errorTerminal.modelConfigId
          ),
          customBaseUrl: null,
          customApiKey: null,
        }
      : undefined,
    mergeTerminalConfig: {
      cliTypeId: config.advanced.mergeTerminal.cliTypeId,
      modelConfigId: config.advanced.mergeTerminal.modelConfigId,
      modelConfig: toInlineModelConfig(
        config.advanced.mergeTerminal.modelConfigId
      ),
      customBaseUrl: null,
      customApiKey: null,
    },
    targetBranch: config.advanced.targetBranch,
    gitWatcherEnabled: config.advanced.gitWatcherEnabled,
    tasks,
  };

  return request;
}
