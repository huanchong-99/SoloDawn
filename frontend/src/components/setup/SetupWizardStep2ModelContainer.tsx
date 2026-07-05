import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';

import { useUserSystem } from '@/components/ConfigProvider';
import { useModelVerification } from '@/hooks/useModelVerification';
import { useNativeCredentials } from '@/hooks/useNativeCredentials';
import { setDefaultModelForCli } from '@/hooks/useCliTypes';
import {
  OFFICIAL_MODELS,
  isCompatibleApiType,
} from '@/components/workflow/modelCatalog';
import type { ApiType, ModelConfig } from '@/components/workflow/types';
import {
  createNativeModelConfigs,
  isNativeModelEntry,
} from '@/components/workflow/types';
import { SetupWizardStep2Model } from './SetupWizardStep2Model';

export type SetupModelMode = 'native' | 'manual';

const DEFAULT_BASE_URLS: Record<string, string> = {
  anthropic: 'https://api.anthropic.com',
  google: 'https://generativelanguage.googleapis.com',
  openai: 'https://api.openai.com',
  'openai-compatible': '',
};

interface SetupWizardStep2ModelContainerProps {
  onNext: () => void;
  onBack: () => void;
  onSkip: () => void;
}

export function SetupWizardStep2ModelContainer({
  onNext,
  onBack,
  onSkip,
}: Readonly<SetupWizardStep2ModelContainerProps>) {
  const { t } = useTranslation('workflow');
  const { config, updateAndSaveConfig } = useUserSystem();
  const { data: nativeStatus, isLoading: isNativeLoading } = useNativeCredentials();
  const queryClient = useQueryClient();

  const nativeAvailable = nativeStatus?.available === true;

  // Default to native mode when subscription is detected
  const [mode, setMode] = useState<SetupModelMode>('native');

  // Selected default Claude model for native (subscription) runs.
  const [nativeModelId, setNativeModelId] = useState('');
  useEffect(() => {
    if (!nativeModelId && nativeStatus?.models?.length) {
      const defaultModel =
        nativeStatus.models.find((m) => m.isDefault) ?? nativeStatus.models[0];
      setNativeModelId(defaultModel.id);
    }
  }, [nativeStatus, nativeModelId]);

  const [displayName, setDisplayName] = useState('');
  const [cliTypeId, setCliTypeId] = useState('cli-claude-code');
  const [apiType, setApiType] = useState<string>('anthropic');
  const [apiKey, setApiKey] = useState('');
  const [baseUrl, setBaseUrl] = useState(DEFAULT_BASE_URLS.anthropic);
  const [modelId, setModelId] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);

  const {
    models,
    isLoading: isLoadingModels,
    isVerified,
    isVerifying,
    verifyError,
    fetchModels,
    verifyModel,
    reset: resetVerification,
  } = useModelVerification();

  // Official APIs suggest the built-in catalog; compatible endpoints suggest
  // whatever the live fetch returned (empty until fetched).
  const modelOptions = useMemo(
    () =>
      isCompatibleApiType(apiType)
        ? models
        : (OFFICIAL_MODELS[apiType as ApiType] ?? []).map((m) => ({
            id: m,
            name: m,
          })),
    [apiType, models]
  );

  // Allow proceeding if model ID is manually entered, even without verification.
  // Third-party OpenAI-compatible endpoints may not support the verification API.
  const canProceed = mode === 'native'
    ? nativeAvailable
    : modelId.trim() !== '' && apiKey.trim() !== '';

  const urlWarning = useMemo(() => {
    const url = baseUrl.trim();
    if (!url || !apiType) return null;

    if (url.endsWith('/v1')) {
      return t('step3.warnings.urlV1Compatible');
    }

    if (url.includes('bigmodel.cn') && apiType === 'openai') {
      return t('step3.warnings.zhipuaiOpenai');
    }

    if (url.includes('bigmodel.cn') && apiType === 'anthropic') {
      return t('step3.warnings.zhipuaiAnthropic');
    }

    return null;
  }, [baseUrl, apiType, t]);

  const handleApiTypeChange = useCallback(
    (newType: string) => {
      setApiType(newType);
      setBaseUrl(DEFAULT_BASE_URLS[newType] ?? '');
      setModelId('');
      resetVerification();
    },
    [resetVerification]
  );

  const handleApiKeyChange = useCallback(
    (value: string) => {
      setApiKey(value);
      resetVerification();
    },
    [resetVerification]
  );

  const handleBaseUrlChange = useCallback(
    (value: string) => {
      setBaseUrl(value);
      resetVerification();
    },
    [resetVerification]
  );

  const handleModelIdChange = useCallback(
    (value: string) => {
      setModelId(value);
      resetVerification();
    },
    [resetVerification]
  );

  const handleFetchModels = useCallback(() => {
    fetchModels(
      apiType,
      apiKey,
      apiType === 'openai-compatible' || apiType === 'anthropic-compatible' ? baseUrl : undefined
    ).catch(() => { /* handled internally */ });
  }, [apiType, apiKey, baseUrl, fetchModels]);

  const handleVerify = useCallback(() => {
    verifyModel({
      apiType,
      apiKey,
      baseUrl: apiType === 'openai-compatible' || apiType === 'anthropic-compatible' ? baseUrl : undefined,
      modelId,
    }).catch(() => { /* handled internally */ });
  }, [apiType, apiKey, baseUrl, modelId, verifyModel]);

  const handleNext = useCallback(async () => {
    const existingModels = (config as Record<string, unknown>)?.workflow_model_library;
    const currentModels = Array.isArray(existingModels) ? existingModels as ModelConfig[] : [];

    if (mode === 'native') {
      const options = nativeStatus?.models ?? [];

      // Persist the chosen model as the Claude Code default — this is the
      // knob native (subscription) runs follow in both DIY and
      // agent-planned modes when nothing more specific is chosen.
      const currentDefaultId = options.find((m) => m.isDefault)?.id ?? '';
      if (nativeModelId && nativeModelId !== currentDefaultId) {
        try {
          await setDefaultModelForCli('cli-claude-code', nativeModelId);
          await queryClient.invalidateQueries({
            queryKey: ['native-credentials-status'],
          });
        } catch (error) {
          console.error('Failed to set default Claude model', error);
        }
      }

      // Store the native subscription models (chosen default first),
      // replacing any stale native entries already in the library.
      const picked = options.find((m) => m.id === nativeModelId);
      const orderedOptions = picked
        ? [picked, ...options.filter((m) => m.id !== picked.id)]
        : options;
      const nativeEntries = createNativeModelConfigs(orderedOptions);
      const nativeIds = new Set(nativeEntries.map((m) => m.id));
      const withoutNative = currentModels.filter(
        (m) => !isNativeModelEntry(m) && !nativeIds.has(m.id)
      );
      await updateAndSaveConfig({
        workflow_model_library: [...nativeEntries, ...withoutNative],
      } as Parameters<typeof updateAndSaveConfig>[0]);
    } else {
      const trimmedKey = apiKey.trim();
      const trimmedUrl = baseUrl.trim();
      const newModel: ModelConfig = {
        id: `model-${crypto.randomUUID()}`,
        displayName: (displayName || modelId).trim(),
        cliTypeId,
        apiType: apiType as ApiType,
        baseUrl: trimmedUrl || DEFAULT_BASE_URLS[apiType] || '',
        apiKey: trimmedKey,
        modelId: modelId.trim(),
        isVerified,
      };
      await updateAndSaveConfig({
        workflow_model_library: [...currentModels, newModel],
      } as Parameters<typeof updateAndSaveConfig>[0]);
    }

    onNext();
  }, [
    config,
    mode,
    nativeStatus,
    nativeModelId,
    queryClient,
    displayName,
    cliTypeId,
    apiType,
    baseUrl,
    apiKey,
    modelId,
    isVerified,
    updateAndSaveConfig,
    onNext,
  ]);

  return (
    <SetupWizardStep2Model
      mode={mode}
      onModeChange={setMode}
      nativeAvailable={nativeAvailable}
      isNativeLoading={isNativeLoading}
      nativeCliVersion={nativeStatus?.cliVersion ?? null}
      nativeModels={nativeStatus?.models ?? []}
      nativeModelId={nativeModelId}
      onNativeModelIdChange={setNativeModelId}
      displayName={displayName}
      cliTypeId={cliTypeId}
      onCliTypeIdChange={setCliTypeId}
      apiType={apiType}
      apiKey={apiKey}
      baseUrl={baseUrl}
      modelId={modelId}
      models={modelOptions}
      isLoadingModels={isLoadingModels}
      isVerified={isVerified}
      verifyError={verifyError}
      isVerifying={isVerifying}
      urlWarning={urlWarning}
      onDisplayNameChange={setDisplayName}
      onApiTypeChange={handleApiTypeChange}
      onApiKeyChange={handleApiKeyChange}
      onBaseUrlChange={handleBaseUrlChange}
      onModelIdChange={handleModelIdChange}
      showApiKey={showApiKey}
      onToggleApiKeyVisibility={() => setShowApiKey((prev) => !prev)}
      onFetchModels={handleFetchModels}
      onVerify={handleVerify}
      onNext={() => { handleNext().catch(() => { /* handled internally */ }); }}
      onBack={onBack}
      onSkip={onSkip}
      canProceed={canProceed}
    />
  );
}
