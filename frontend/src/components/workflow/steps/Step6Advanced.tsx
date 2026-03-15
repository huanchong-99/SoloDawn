import React, { useCallback, useEffect } from 'react';
import { Field, FieldLabel, FieldError } from '../../ui-new/primitives/Field';
import { CollapsibleSection } from '../../ui-new/primitives/CollapsibleSection';
import { cn } from '@/lib/utils';
import { CLI_TYPES, GIT_COMMIT_FORMAT } from '../constants';
import type { WizardConfig, AdvancedConfig } from '../types';
import { useTranslation } from 'react-i18next';

interface Step6AdvancedProps {
  config: WizardConfig;
  errors: Record<string, string>;
  onUpdate: (updates: Partial<WizardConfig>) => void;
}

/**
 * Step 6: Configures orchestrator, error recovery, and merge settings.
 */
export const Step6Advanced: React.FC<Step6AdvancedProps> = ({
  config,
  errors,
  onUpdate,
}) => {
  const { t } = useTranslation('workflow');
  const advancedConfig = config.advanced;

  const getModelsForCli = useCallback((cliTypeId?: string) => {
    if (!cliTypeId) {
      return [];
    }
    return config.models.filter((model) => {
      const boundCliTypeId = model.cliTypeId?.trim();
      if (!boundCliTypeId) {
        return true;
      }
      return boundCliTypeId === cliTypeId;
    });
  }, [config.models]);

  const isModelCompatibleWithCli = useCallback((
    modelConfigId: string | undefined,
    cliTypeId: string | undefined
  ): boolean => {
    if (!modelConfigId?.trim()) {
      return true;
    }
    if (!cliTypeId?.trim()) {
      return false;
    }
    return getModelsForCli(cliTypeId).some((model) => model.id === modelConfigId);
  }, [getModelsForCli]);

  const updateOrchestrator = (updates: Partial<AdvancedConfig['orchestrator']>) => {
    onUpdate({
      advanced: {
        ...advancedConfig,
        orchestrator: {
          ...advancedConfig.orchestrator,
          ...updates,
        },
      },
    });
  };

  const updateErrorTerminal = (updates: Partial<AdvancedConfig['errorTerminal']>) => {
    onUpdate({
      advanced: {
        ...advancedConfig,
        errorTerminal: {
          ...advancedConfig.errorTerminal,
          ...updates,
        },
      },
    });
  };

  const updateMergeTerminal = (updates: Partial<AdvancedConfig['mergeTerminal']>) => {
    onUpdate({
      advanced: {
        ...advancedConfig,
        mergeTerminal: {
          ...advancedConfig.mergeTerminal,
          ...updates,
        },
      },
    });
  };

  const handleOrchestratorModelChange = (modelConfigId: string) => {
    updateOrchestrator({ modelConfigId });
  };

  const handleErrorTerminalEnable = () => {
    updateErrorTerminal({ enabled: true });
  };

  const handleErrorTerminalDisable = () => {
    updateErrorTerminal({ enabled: false, cliTypeId: undefined, modelConfigId: undefined });
  };

  const handleErrorTerminalCliChange = (cliTypeId: string) => {
    const nextModelConfigId = isModelCompatibleWithCli(
      advancedConfig.errorTerminal.modelConfigId,
      cliTypeId
    )
      ? advancedConfig.errorTerminal.modelConfigId
      : undefined;
    updateErrorTerminal({ cliTypeId, modelConfigId: nextModelConfigId });
  };

  const handleErrorTerminalModelChange = (modelConfigId: string) => {
    updateErrorTerminal({ modelConfigId });
  };

  const handleMergeTerminalCliChange = (cliTypeId: string) => {
    const nextModelConfigId = isModelCompatibleWithCli(
      advancedConfig.mergeTerminal.modelConfigId,
      cliTypeId
    )
      ? advancedConfig.mergeTerminal.modelConfigId
      : '';
    updateMergeTerminal({ cliTypeId, modelConfigId: nextModelConfigId });
  };

  const handleMergeTerminalModelChange = (modelConfigId: string) => {
    updateMergeTerminal({ modelConfigId });
  };

  const handleRunTestsBeforeMergeChange = (runTestsBeforeMerge: boolean) => {
    updateMergeTerminal({ runTestsBeforeMerge });
  };

  const handlePauseOnConflictChange = (pauseOnConflict: boolean) => {
    updateMergeTerminal({ pauseOnConflict });
  };

  const handleTargetBranchChange = (targetBranch: string) => {
    onUpdate({
      advanced: {
        ...advancedConfig,
        targetBranch,
      },
    });
  };

  const handleGitWatcherEnabledChange = (gitWatcherEnabled: boolean) => {
    onUpdate({
      advanced: {
        ...advancedConfig,
        gitWatcherEnabled,
      },
    });
  };

  useEffect(() => {
    const errorTerminalModelIncompatible =
      advancedConfig.errorTerminal.enabled &&
      !!advancedConfig.errorTerminal.cliTypeId?.trim() &&
      !!advancedConfig.errorTerminal.modelConfigId?.trim() &&
      !isModelCompatibleWithCli(
        advancedConfig.errorTerminal.modelConfigId,
        advancedConfig.errorTerminal.cliTypeId
      );

    const mergeTerminalModelIncompatible = !isModelCompatibleWithCli(
      advancedConfig.mergeTerminal.modelConfigId,
      advancedConfig.mergeTerminal.cliTypeId
    );

    if (!errorTerminalModelIncompatible && !mergeTerminalModelIncompatible) {
      return;
    }

    onUpdate({
      advanced: {
        ...advancedConfig,
        errorTerminal: {
          ...advancedConfig.errorTerminal,
          modelConfigId: errorTerminalModelIncompatible
            ? undefined
            : advancedConfig.errorTerminal.modelConfigId,
        },
        mergeTerminal: {
          ...advancedConfig.mergeTerminal,
          modelConfigId: mergeTerminalModelIncompatible
            ? ''
            : advancedConfig.mergeTerminal.modelConfigId,
        },
      },
    });
  }, [advancedConfig, onUpdate, isModelCompatibleWithCli]);

  return (
    <div className="flex flex-col gap-base">
      <Field>
        <FieldLabel htmlFor="orchestratorModel">{t('step6.orchestrator.label')}</FieldLabel>
        <div className="text-sm text-low mb-half">
          {t('step6.orchestrator.description')}
        </div>
        <select
          id="orchestratorModel"
          value={advancedConfig.orchestrator.modelConfigId}
          onChange={(e) => {
            handleOrchestratorModelChange(e.target.value);
          }}
          className={cn(
            'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
            'focus:outline-none focus:ring-1 focus:ring-brand',
            errors.orchestratorModel && 'border-error'
          )}
        >
          <option value="">{t('step6.orchestrator.placeholder')}</option>
          {config.models.map((model) => (
            <option key={model.id} value={model.id}>
              {model.displayName}
            </option>
          ))}
        </select>
        {errors.orchestratorModel && <FieldError>{t(errors.orchestratorModel)}</FieldError>}
      </Field>

      <Field>
        <div className="flex items-center gap-base mb-half">
          <input
            type="checkbox"
            id="errorTerminalEnabled"
            checked={advancedConfig.errorTerminal.enabled}
            onChange={(e) => {
              if (e.target.checked) {
                handleErrorTerminalEnable();
              } else {
                handleErrorTerminalDisable();
              }
            }}
            className="size-icon-sm accent-brand"
          />
          <FieldLabel htmlFor="errorTerminalEnabled" className="mb-0">
            {t('step6.errorTerminal.enableLabel')}
          </FieldLabel>
        </div>
        <div className="text-sm text-low mb-base">
          {t('step6.errorTerminal.description')}
        </div>

        {advancedConfig.errorTerminal.enabled && (
          <div className="flex flex-col gap-base p-base border rounded-sm bg-panel">
            <Field>
              <FieldLabel htmlFor="errorTerminalCli">{t('step6.errorTerminal.cliLabel')}</FieldLabel>
              <select
                id="errorTerminalCli"
                value={advancedConfig.errorTerminal.cliTypeId ?? ''}
                onChange={(e) => {
                  handleErrorTerminalCliChange(e.target.value);
                }}
                className={cn(
                  'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                  'focus:outline-none focus:ring-1 focus:ring-brand',
                  errors.errorTerminalCli && 'border-error'
                )}
              >
                <option value="">{t('step6.errorTerminal.cliPlaceholder')}</option>
                {Object.values(CLI_TYPES).map((cli) => (
                  <option key={cli.id} value={cli.id}>
                    {cli.label} - {cli.description}
                  </option>
                ))}
              </select>
              {errors.errorTerminalCli && <FieldError>{t(errors.errorTerminalCli)}</FieldError>}
            </Field>

            <Field>
              <FieldLabel htmlFor="errorTerminalModel">{t('step6.errorTerminal.modelLabel')}</FieldLabel>
              <select
                id="errorTerminalModel"
                value={advancedConfig.errorTerminal.modelConfigId ?? ''}
                onChange={(e) => {
                  handleErrorTerminalModelChange(e.target.value);
                }}
                disabled={!advancedConfig.errorTerminal.cliTypeId}
                className={cn(
                  'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                  'focus:outline-none focus:ring-1 focus:ring-brand',
                  'disabled:opacity-50 disabled:cursor-not-allowed',
                  errors.errorTerminalModel && 'border-error'
                )}
              >
                <option value="">{t('step6.errorTerminal.modelPlaceholder')}</option>
                {getModelsForCli(advancedConfig.errorTerminal.cliTypeId).map((model) => (
                  <option key={model.id} value={model.id}>
                    {model.displayName}
                  </option>
                ))}
              </select>
              {errors.errorTerminalModel && <FieldError>{t(errors.errorTerminalModel)}</FieldError>}
            </Field>
          </div>
        )}
      </Field>

      <Field>
        <FieldLabel>{t('step6.mergeTerminal.title')}</FieldLabel>
        <div className="text-sm text-low mb-base">
          {t('step6.mergeTerminal.description')}
        </div>

        <div className="flex flex-col gap-base p-base border rounded-sm bg-panel">
          <Field>
            <FieldLabel htmlFor="mergeTerminalCli">{t('step6.mergeTerminal.cliLabel')}</FieldLabel>
            <select
              id="mergeTerminalCli"
              value={advancedConfig.mergeTerminal.cliTypeId}
              onChange={(e) => {
                handleMergeTerminalCliChange(e.target.value);
              }}
              className={cn(
                'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                'focus:outline-none focus:ring-1 focus:ring-brand',
                errors.mergeCli && 'border-error'
              )}
            >
              <option value="">{t('step6.mergeTerminal.cliPlaceholder')}</option>
              {Object.values(CLI_TYPES).map((cli) => (
                <option key={cli.id} value={cli.id}>
                  {cli.label} - {cli.description}
                </option>
              ))}
            </select>
            {errors.mergeCli && <FieldError>{t(errors.mergeCli)}</FieldError>}
          </Field>

          <Field>
            <FieldLabel htmlFor="mergeTerminalModel">{t('step6.mergeTerminal.modelLabel')}</FieldLabel>
            <select
              id="mergeTerminalModel"
              value={advancedConfig.mergeTerminal.modelConfigId}
              onChange={(e) => {
                handleMergeTerminalModelChange(e.target.value);
              }}
              disabled={!advancedConfig.mergeTerminal.cliTypeId}
              className={cn(
                'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                'focus:outline-none focus:ring-1 focus:ring-brand',
                'disabled:opacity-50 disabled:cursor-not-allowed',
                errors.mergeModel && 'border-error'
              )}
            >
              <option value="">{t('step6.mergeTerminal.modelPlaceholder')}</option>
              {getModelsForCli(advancedConfig.mergeTerminal.cliTypeId).map((model) => (
                <option key={model.id} value={model.id}>
                  {model.displayName}
                </option>
              ))}
            </select>
            {errors.mergeModel && <FieldError>{t(errors.mergeModel)}</FieldError>}
          </Field>

          <div className="flex flex-col gap-base">
            <label className="flex items-center gap-base cursor-pointer">
              <input
                type="checkbox"
                checked={advancedConfig.mergeTerminal.runTestsBeforeMerge}
                onChange={(e) => {
                  handleRunTestsBeforeMergeChange(e.target.checked);
                }}
                className="size-icon-sm accent-brand"
              />
              <span className="text-base text-normal">
                {t('step6.mergeTerminal.runTestsLabel')}
              </span>
            </label>

            <label className="flex items-center gap-base cursor-pointer">
              <input
                type="checkbox"
                checked={advancedConfig.mergeTerminal.pauseOnConflict}
                onChange={(e) => {
                  handlePauseOnConflictChange(e.target.checked);
                }}
                className="size-icon-sm accent-brand"
              />
              <span className="text-base text-normal">
                {t('step6.mergeTerminal.pauseOnConflictLabel')}
              </span>
            </label>
          </div>
        </div>
      </Field>

      <Field>
        <div className="flex items-center gap-base mb-half">
          <input
            type="checkbox"
            id="gitWatcherEnabled"
            checked={advancedConfig.gitWatcherEnabled}
            onChange={(e) => {
              handleGitWatcherEnabledChange(e.target.checked);
            }}
            className="size-icon-sm accent-brand"
          />
          <FieldLabel htmlFor="gitWatcherEnabled" className="mb-0">
            {t('step6.gitWatcher.enableLabel')}
          </FieldLabel>
        </div>
        <div className="text-sm text-low">
          {t('step6.gitWatcher.description')}
        </div>
      </Field>

      <Field>
        <FieldLabel htmlFor="targetBranch">{t('step6.targetBranch.label')}</FieldLabel>
        <input
          id="targetBranch"
          type="text"
          value={advancedConfig.targetBranch}
          onChange={(e) => {
            handleTargetBranchChange(e.target.value);
          }}
          placeholder={t('step6.targetBranch.placeholder')}
          className={cn(
            'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
            'placeholder:text-low placeholder:opacity-80',
            'focus:outline-none focus:ring-1 focus:ring-brand',
            errors.targetBranch && 'border-error'
          )}
        />
        {errors.targetBranch && <FieldError>{t(errors.targetBranch)}</FieldError>}
      </Field>

      <Field>
        <CollapsibleSection
          persistKey="wizard-git-commit-format"
          title={t('step6.gitCommit.title')}
          defaultExpanded={false}
        >
          <div className="mt-base">
            <div className="text-sm text-low mb-base">
              {t('step6.gitCommit.description')}
            </div>
            <pre className="bg-secondary border rounded-sm p-base text-sm font-mono text-normal overflow-x-auto">
              {GIT_COMMIT_FORMAT}
            </pre>
          </div>
        </CollapsibleSection>
      </Field>
    </div>
  );
};
