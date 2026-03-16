import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SpinnerGap, WarningCircle } from '@phosphor-icons/react';

import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { Step3Models } from '@/components/workflow/steps/Step3Models';
import { getDefaultWizardConfig } from '@/components/workflow/types';
import { useUserSystem } from '@/components/ConfigProvider';
import type {
  ModelConfig as WorkflowModelConfig,
  WizardConfig,
} from '@/components/workflow/types';

const isWorkflowModelConfig = (
  value: unknown
): value is WorkflowModelConfig => {
  if (typeof value !== 'object' || value === null) {
    return false;
  }

  const item = value as Record<string, unknown>;
  const cliTypeId = item.cliTypeId;
  return (
    typeof item.id === 'string' &&
    typeof item.displayName === 'string' &&
    (cliTypeId === undefined || typeof cliTypeId === 'string') &&
    typeof item.apiType === 'string' &&
    typeof item.baseUrl === 'string' &&
    typeof item.apiKey === 'string' &&
    typeof item.modelId === 'string' &&
    typeof item.isVerified === 'boolean'
  );
};

const parseWorkflowModelLibrary = (config: unknown): WorkflowModelConfig[] => {
  const rawLibrary = (config as { workflow_model_library?: unknown } | null)
    ?.workflow_model_library;
  if (!Array.isArray(rawLibrary)) {
    return [];
  }

  return rawLibrary
    .filter(isWorkflowModelConfig)
    .map((model) => ({ ...model }));
};

export function ModelsSettingsNew() {
  const { t } = useTranslation(['settings', 'workflow']);
  const { config, updateAndSaveConfig } = useUserSystem();

  const [workflowModelLibraryDraft, setWorkflowModelLibraryDraft] = useState<
    WorkflowModelConfig[]
  >([]);
  const [workflowModelsSaving, setWorkflowModelsSaving] = useState(false);
  const [workflowModelsError, setWorkflowModelsError] = useState<string | null>(
    null
  );

  useEffect(() => {
    setWorkflowModelLibraryDraft(parseWorkflowModelLibrary(config));
  }, [config]);

  const workflowModelLibraryWizardConfig = useMemo<WizardConfig>(
    () => ({
      ...getDefaultWizardConfig(),
      models: workflowModelLibraryDraft,
    }),
    [workflowModelLibraryDraft]
  );

  const handleWorkflowModelLibraryUpdate = (
    updates: Partial<WizardConfig>
  ) => {
    if (!updates.models) {
      return;
    }

    const nextModels = updates.models.map((model) => ({ ...model }));
    setWorkflowModelLibraryDraft(nextModels);
    setWorkflowModelsError(null);
    setWorkflowModelsSaving(true);

    void (async () => {
      try {
        await updateAndSaveConfig({
          workflow_model_library: nextModels,
        } as Partial<NonNullable<typeof config>>);
      } catch (error) {
        console.error('Failed to save workflow model library', error);
        setWorkflowModelsError(t('settings.general.save.error'));
      } finally {
        setWorkflowModelsSaving(false);
      }
    })();
  };

  return (
    <div className="flex flex-col gap-base">
      {workflowModelsError && (
        <div className="flex items-center gap-half rounded-sm border border-error bg-error/10 px-base py-half text-sm text-error">
          <WarningCircle className="size-icon-sm shrink-0" weight="fill" />
          {workflowModelsError}
        </div>
      )}

      <SettingsCard
        title={t('workflow:step3.title')}
        description={t('workflow:steps.models.description')}
      >
        <div className="flex flex-col gap-base">
          {workflowModelsSaving && (
            <div className="flex items-center gap-half text-sm text-low">
              <SpinnerGap className="size-icon-xs animate-spin" weight="bold" />
              {t('settings.general.save.button')}
            </div>
          )}
          <Step3Models
            config={workflowModelLibraryWizardConfig}
            onUpdate={handleWorkflowModelLibraryUpdate}
            dialogContentClassName="bg-panel border border-border text-high shadow-xl"
          />
        </div>
      </SettingsCard>
    </div>
  );
}
