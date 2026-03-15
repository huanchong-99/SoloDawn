import React, { useState, useEffect, useCallback } from 'react';
import {
  CaretLeft,
  CaretRight,
  Check as CheckPhosphor,
  X as XPhosphor,
  Warning as WarningPhosphor,
  ArrowsClockwise,
} from '@phosphor-icons/react';
import { Field, FieldLabel, FieldError } from '../../ui-new/primitives/Field';
import { ErrorAlert } from '../../ui-new/primitives/ErrorAlert';
import { cn } from '@/lib/utils';
import type { WizardConfig, TerminalConfig } from '../types';
import { useErrorNotification } from '@/hooks/useErrorNotification';
import { useTranslation } from 'react-i18next';

interface Step4TerminalsProps {
  config: WizardConfig;
  errors: Record<string, string>;
  onUpdate: (updates: Partial<WizardConfig>) => void;
  onError?: (error: Error) => void;
}

interface CliType {
  id: string;
  name: string;
  displayName: string;
  installed: boolean;
  installGuideUrl: string | null;
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null;

// Type for CLI detection API response
interface CliDetectResponse {
  cliTypeId: string;
  name: string;
  displayName: string;
  installed: boolean;
  version: string | null;
  executablePath: string | null;
  installGuideUrl: string | null;
}

const isCliDetectResponseArray = (value: unknown): value is CliDetectResponse[] => {
  if (!Array.isArray(value)) return false;
  return value.every(
    (item) =>
      isRecord(item) &&
      typeof item.cliTypeId === 'string' &&
      typeof item.name === 'string' &&
      typeof item.installed === 'boolean'
  );
};

const isLegacyCliDetectMap = (value: unknown): value is Record<string, boolean> => {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return false;
  }
  return Object.values(value).every((item) => typeof item === 'boolean');
};

const LEGACY_CLI_DISPLAY_NAMES: Record<string, string> = {
  'claude-code': 'Claude Code',
  'gemini-cli': 'Gemini CLI',
  codex: 'Codex',
  'cursor-agent': 'Cursor Agent',
  amp: 'Amp',
  'qwen-code': 'Qwen Code',
  copilot: 'Copilot',
  droid: 'Droid',
  opencode: 'Opencode',
};

const LEGACY_CLI_ID_ALIASES: Record<string, string> = {
  'claude-code': 'cli-claude-code',
  'gemini-cli': 'cli-gemini-cli',
  codex: 'cli-codex',
  'cursor-agent': 'cli-cursor-agent',
  amp: 'cli-amp',
  'qwen-code': 'cli-qwen-code',
  copilot: 'cli-copilot',
  droid: 'cli-droid',
  opencode: 'cli-opencode',
};

const parseJson = async (response: Response): Promise<unknown> => {
  try {
    return (await response.json()) as unknown;
  } catch {
    return null;
  }
};

const buildTerminalErrorKey = (
  terminalId: string,
  field: 'cli' | 'model'
): string => `terminal-${terminalId}-${field}`;

const terminalConfigEquals = (
  left: TerminalConfig,
  right: TerminalConfig
): boolean => {
  return (
    left.id === right.id &&
    left.taskId === right.taskId &&
    left.orderIndex === right.orderIndex &&
    left.cliTypeId === right.cliTypeId &&
    left.modelConfigId === right.modelConfigId &&
    (left.role ?? '') === (right.role ?? '') &&
    (left.autoConfirm ?? true) === (right.autoConfirm ?? true)
  );
};

const terminalConfigListEquals = (
  left: TerminalConfig[],
  right: TerminalConfig[]
): boolean => {
  if (left.length !== right.length) {
    return false;
  }

  return left.every((item, index) => terminalConfigEquals(item, right[index]));
};

/** Create a normalized terminal config for a given task and order index. */
const createNormalizedTerminal = (
  task: { id: string },
  orderIndex: number,
  existingTerminal?: TerminalConfig
): TerminalConfig => {
  if (existingTerminal) {
    return {
      ...existingTerminal,
      id: existingTerminal.id || `terminal-${task.id}-${orderIndex}`,
      taskId: task.id,
      orderIndex,
    };
  }

  return {
    id: `terminal-${task.id}-${orderIndex}`,
    taskId: task.id,
    orderIndex,
    cliTypeId: '',
    modelConfigId: '',
    role: '',
  };
};

/** Normalize terminals for a single task to match its terminalCount. */
const normalizeTerminalsForTask = (
  task: { id: string; terminalCount: number },
  existingTerminals: TerminalConfig[]
): TerminalConfig[] => {
  const sortedExisting = existingTerminals
    .filter((terminal) => terminal.taskId === task.id)
    .sort((a, b) => a.orderIndex - b.orderIndex);

  return Array.from({ length: task.terminalCount }, (_, orderIndex) => {
    const byOrderIndex = sortedExisting.find((terminal) => terminal.orderIndex === orderIndex);
    return createNormalizedTerminal(task, orderIndex, byOrderIndex);
  });
};

/**
 * Step 4: Assigns terminals and CLI/model settings for each task.
 */
export const Step4Terminals: React.FC<Step4TerminalsProps> = ({
  config,
  errors,
  onUpdate,
  onError,
}) => {
  const { notifyError } = useErrorNotification({ onError, context: 'Step4Terminals' });
  const { t } = useTranslation('workflow');
  const [currentTaskIndex, setCurrentTaskIndex] = useState(0);
  const [cliTypes, setCliTypes] = useState<CliType[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);

  // Filter to only installed CLI types
  const availableCliTypes = cliTypes.filter((ct) => ct.installed);

  // Get current task
  const hasTasks = config.tasks.length > 0;
  const currentTask = hasTasks ? config.tasks[currentTaskIndex] : null;

  // Keep current task index in bounds when task list changes
  useEffect(() => {
    if (config.tasks.length === 0) {
      return;
    }

    if (currentTaskIndex >= config.tasks.length) {
      setCurrentTaskIndex(config.tasks.length - 1);
    }
  }, [currentTaskIndex, config.tasks.length]);

  // Initialize/normalize terminals for all tasks
  useEffect(() => {
    if (!hasTasks) {
      return;
    }

    const normalizedTerminals: TerminalConfig[] = config.tasks.flatMap((task) =>
      normalizeTerminalsForTask(task, config.terminals)
    );

    if (!terminalConfigListEquals(config.terminals, normalizedTerminals)) {
      onUpdate({ terminals: normalizedTerminals });
    }
  }, [hasTasks, config.tasks, config.terminals, onUpdate]);

  // Detect CLI installation status from API
  const detectCliTypes = useCallback(async (isRefresh = false) => {
    if (isRefresh) {
      setIsRefreshing(true);
    } else {
      setIsLoading(true);
    }
    try {
      const response = await fetch('/api/cli_types/detect');
      const data = await parseJson(response);
      if (response.ok) {
        if (isCliDetectResponseArray(data)) {
          // Convert API response to CliType array
          const cliList: CliType[] = data.map((item) => ({
            id: item.cliTypeId,
            name: item.name,
            displayName: item.displayName,
            installed: item.installed,
            installGuideUrl: item.installGuideUrl,
          }));
          setCliTypes(cliList);
        } else if (isLegacyCliDetectMap(data)) {
          const cliList: CliType[] = Object.entries(data).map(([name, installed]) => ({
            id: LEGACY_CLI_ID_ALIASES[name] ?? name,
            name,
            displayName: LEGACY_CLI_DISPLAY_NAMES[name] ?? name,
            installed,
            installGuideUrl: installed
              ? null
              : `https://example.com/install/${encodeURIComponent(name)}`,
          }));
          setCliTypes(cliList);
        }
      } else {
        notifyError(new Error(`CLI detection failed with status ${response.status}`), 'detectCliTypes');
      }
    } catch (error) {
      notifyError(error, 'detectCliTypes');
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  }, [notifyError]);

  useEffect(() => {
    detectCliTypes();
  }, [detectCliTypes]);

  const handleRefreshCli = () => {
    detectCliTypes(true);
  };

  const updateTerminal = useCallback(
    (terminalId: string, updates: Partial<TerminalConfig>) => {
      const newTerminals = config.terminals.map((t) =>
        t.id === terminalId ? { ...t, ...updates } : t
      );
      onUpdate({ terminals: newTerminals });
    },
    [config.terminals, onUpdate]
  );

  const getModelsForCli = useCallback(
    (cliTypeId: string) =>
      config.models.filter((model) => {
        const boundCliTypeId = model.cliTypeId?.trim();
        if (!boundCliTypeId) {
          return true;
        }
        return boundCliTypeId === cliTypeId;
      }),
    [config.models]
  );

  const getCompatibleModelIds = useCallback(
    (cliTypeId: string): Set<string> =>
      new Set(getModelsForCli(cliTypeId).map((m) => m.id)),
    [getModelsForCli]
  );

  const handleCliSelect = useCallback(
    (terminalId: string, currentModelConfigId: string, cliId: string) => {
      const compatible = getCompatibleModelIds(cliId);
      updateTerminal(terminalId, {
        cliTypeId: cliId,
        modelConfigId: compatible.has(currentModelConfigId) ? currentModelConfigId : '',
      });
    },
    [getCompatibleModelIds, updateTerminal]
  );

  useEffect(() => {
    const normalizedTerminals = config.terminals.map((terminal) => {
      if (!terminal.modelConfigId.trim()) {
        return terminal;
      }

      const compatible = terminal.cliTypeId
        ? getCompatibleModelIds(terminal.cliTypeId)
        : new Set(config.models.map((m) => m.id));

      if (compatible.has(terminal.modelConfigId)) {
        return terminal;
      }

      return { ...terminal, modelConfigId: '' };
    });

    if (!terminalConfigListEquals(config.terminals, normalizedTerminals)) {
      onUpdate({ terminals: normalizedTerminals });
    }
  }, [config.models, config.terminals, getCompatibleModelIds, onUpdate]);

  const goToPreviousTask = () => {
    if (currentTaskIndex > 0) {
      setCurrentTaskIndex(currentTaskIndex - 1);
    }
  };

  const goToNextTask = () => {
    if (currentTaskIndex < config.tasks.length - 1) {
      setCurrentTaskIndex(currentTaskIndex + 1);
    }
  };

  // Guard against rendering before tasks are initialized
  if (!currentTask) {
    return null;
  }

  // Get terminals for current task, sorted by orderIndex
  const taskTerminals = config.terminals
    .filter((terminal) => terminal.taskId === currentTask.id)
    .sort((a, b) => a.orderIndex - b.orderIndex);

  const taskNameValue = currentTask.name.trim();
  const taskName = taskNameValue
    ? currentTask.name
    : t('step4.taskNameFallback', { index: currentTaskIndex + 1 });

  return (
    <div className="flex flex-col gap-base">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h2 className="text-lg text-high font-medium">{t('step4.title')}</h2>
        <div className="text-base text-low">
          {t('step4.taskIndicator', { current: currentTaskIndex + 1, total: config.tasks.length })}
        </div>
      </div>

      {/* No CLI Installed Error */}
      {!isLoading && availableCliTypes.length === 0 && (
        <ErrorAlert message={t('step4.errors.noCliInstalled')} />
      )}

      {/* Task Navigation */}
      {config.tasks.length > 1 && (
        <div className="flex items-center justify-between">
          <button
            type="button"
            onClick={goToPreviousTask}
            disabled={currentTaskIndex === 0}
            className={cn(
              'flex items-center gap-half px-base py-half rounded-sm border text-base',
              'transition-colors',
              'disabled:opacity-50 disabled:cursor-not-allowed',
              'hover:border-brand hover:text-high',
              'border-border text-normal bg-secondary'
            )}
          >
            <CaretLeft size={16} />
            {t('step4.previousTask')}
          </button>

          <div className="text-base text-normal">
            {taskName}
          </div>

          <button
            type="button"
            onClick={goToNextTask}
            disabled={currentTaskIndex === config.tasks.length - 1}
            className={cn(
              'flex items-center gap-half px-base py-half rounded-sm border text-base',
              'transition-colors',
              'disabled:opacity-50 disabled:cursor-not-allowed',
              'hover:border-brand hover:text-high',
              'border-border text-normal bg-secondary'
            )}
          >
            {t('step4.nextTask')}
            <CaretRight size={16} />
          </button>
        </div>
      )}

      {/* Terminal Count Display */}
      <div className="text-base text-normal">
        {t('step4.terminalCount', { count: currentTask.terminalCount })}
      </div>

      {/* CLI Installation Status */}
      <div className="bg-secondary border rounded-sm p-base">
        <div className="flex items-center justify-between mb-base">
          <div className="text-base text-high font-medium">{t('step4.cliStatusTitle')}</div>
          <button
            type="button"
            onClick={handleRefreshCli}
            disabled={isRefreshing}
            className={cn(
              'flex items-center gap-half px-base py-half rounded-sm border text-sm',
              'transition-colors hover:border-brand hover:text-high',
              'disabled:opacity-50 disabled:cursor-not-allowed',
              'border-border text-low bg-panel'
            )}
          >
            <ArrowsClockwise className={cn('size-icon-sm', isRefreshing && 'animate-spin')} />
            {t('step4.refreshCli', { defaultValue: 'Refresh' })}
          </button>
        </div>
        {isLoading ? (
          <div className="text-base text-low">{t('step4.loadingCli')}</div>
        ) : (
          <div className="grid grid-cols-2 gap-base">
            {cliTypes.map((cli) => (
              <div
                key={cli.id}
                className="flex items-center justify-between p-base rounded-sm bg-panel border"
              >
                <div className="flex items-center gap-base">
                  {cli.installed ? (
                    <CheckPhosphor className="size-icon-sm text-success" weight="bold" />
                  ) : (
                    <XPhosphor className="size-icon-sm text-error" weight="bold" />
                  )}
                  <span className="text-base text-normal">{cli.displayName}</span>
                </div>
                {!cli.installed && cli.installGuideUrl && (
                  <a
                    href={cli.installGuideUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-base text-brand hover:underline"
                  >
                    {t('step4.installGuide')}
                  </a>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Terminal Configuration Forms */}
      <div className="flex flex-col gap-base">
        {taskTerminals.map((terminal) => (
          <div
            key={terminal.id}
            className="bg-secondary border rounded-sm p-base"
          >
            <div className="text-base text-high font-medium mb-base">
              {t('step4.terminalLabel', { index: terminal.orderIndex + 1 })}
            </div>

            <div className="flex flex-col gap-base">
              {/* CLI Type Selection */}
              <Field>
                <FieldLabel>{t('step4.cliTypeLabel')}</FieldLabel>

                {/* Warning if CLI is not installed */}
                {terminal.cliTypeId && !cliTypes.find((ct) => ct.id === terminal.cliTypeId)?.installed && (
                  <div className="mb-base flex items-start gap-half p-base border border-warning bg-warning/10 rounded-sm">
                    <WarningPhosphor className="size-icon-sm text-warning shrink-0 mt-quarter" />
                    <div className="flex-1">
                      <div className="text-base text-warning font-medium mb-quarter">
                        {t('step4.cliNotInstalledTitle')}
                      </div>
                      <div className="text-sm text-warning/80">
                        {t('step4.cliNotInstalledDescription')}
                      </div>
                    </div>
                  </div>
                )}

                <div className="grid grid-cols-2 gap-base">
                  {cliTypes.map((cli) => (
                    <button
                      key={cli.id}
                      type="button"
                      onClick={() => handleCliSelect(terminal.id, terminal.modelConfigId, cli.id)}
                      disabled={!cli.installed}
                      className={cn(
                        'flex items-center gap-half px-base py-half rounded-sm border text-base transition-colors',
                        'hover:border-brand hover:text-high',
                        'disabled:opacity-50 disabled:cursor-not-allowed',
                        terminal.cliTypeId === cli.id
                          ? 'border-brand bg-brand/10 text-high'
                          : 'border-border text-normal bg-panel'
                      )}
                    >
                      {cli.installed ? (
                        <CheckPhosphor className="size-icon-sm text-success" weight="bold" />
                      ) : (
                        <XPhosphor className="size-icon-sm text-error" weight="bold" />
                      )}
                      {cli.displayName}
                    </button>
                  ))}
                </div>
                {errors[buildTerminalErrorKey(terminal.id, 'cli')] && (
                  <FieldError>{t(errors[buildTerminalErrorKey(terminal.id, 'cli')])}</FieldError>
                )}
              </Field>

              {/* Model Selection */}
              <Field>
                <FieldLabel>{t('step4.modelLabel')}</FieldLabel>
                <select
                  value={terminal.modelConfigId}
                  onChange={(e) => {
                    updateTerminal(terminal.id, { modelConfigId: e.target.value });
                  }}
                  disabled={!terminal.cliTypeId}
                  className={cn(
                    'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                    'focus:outline-none focus:ring-1 focus:ring-brand',
                    'disabled:opacity-50 disabled:cursor-not-allowed',
                    errors[buildTerminalErrorKey(terminal.id, 'model')] && 'border-error'
                  )}
                >
                  <option value="">{t('step4.modelPlaceholder')}</option>
                  {(terminal.cliTypeId ? getModelsForCli(terminal.cliTypeId) : []).map((model) => (
                    <option key={model.id} value={model.id}>
                      {model.displayName}
                    </option>
                  ))}
                </select>
                {errors[buildTerminalErrorKey(terminal.id, 'model')] && (
                  <FieldError>{t(errors[buildTerminalErrorKey(terminal.id, 'model')])}</FieldError>
                )}
              </Field>

              {/* Role Description */}
              <Field>
                <FieldLabel>{t('step4.roleLabel')}</FieldLabel>
                <input
                  type="text"
                  value={terminal.role ?? ''}
                  onChange={(e) => {
                    updateTerminal(terminal.id, { role: e.target.value });
                  }}
                  placeholder={t('step4.rolePlaceholder')}
                  className={cn(
                    'w-full bg-secondary rounded-sm border px-base py-half text-base text-normal',
                    'placeholder:text-low placeholder:opacity-80',
                    'focus:outline-none focus:ring-1 focus:ring-brand'
                  )}
                />
              </Field>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
