import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useUserSystem } from '@/components/ConfigProvider';
import { useModelVerification } from '@/hooks/useModelVerification';
import { useNativeCredentials } from '@/hooks/useNativeCredentials';
import type { ApiType, ModelConfig } from '@/components/workflow/types';
import { createNativeModelConfig, NATIVE_MODEL_ID } from '@/components/workflow/types';
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

  const nativeAvailable = nativeStatus?.available === true;

  // Default to native mode when subscription is detected
  const [mode, setMode] = useState<SetupModelMode>('native');

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
      // Add the native subscription model (skip if already present)
      if (!currentModels.some((m) => m.id === NATIVE_MODEL_ID)) {
        const nativeModel = createNativeModelConfig();
        await updateAndSaveConfig({
          workflow_model_library: [...currentModels, nativeModel],
        } as Parameters<typeof updateAndSaveConfig>[0]);
      }
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
      displayName={displayName}
      cliTypeId={cliTypeId}
      onCliTypeIdChange={setCliTypeId}
      apiType={apiType}
      apiKey={apiKey}
      baseUrl={baseUrl}
      modelId={modelId}
      models={models}
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
