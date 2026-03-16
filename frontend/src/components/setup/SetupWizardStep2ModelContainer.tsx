import { useCallback, useState } from 'react';

import { useUserSystem } from '@/components/ConfigProvider';
import { useModelVerification } from '@/hooks/useModelVerification';
import type { ApiType, ModelConfig } from '@/components/workflow/types';
import { SetupWizardStep2Model } from './SetupWizardStep2Model';

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
  const { updateAndSaveConfig } = useUserSystem();

  const [displayName, setDisplayName] = useState('');
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

  const canProceed = isVerified && modelId !== '';

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
      apiType === 'openai-compatible' ? baseUrl : undefined
    ).catch(() => { /* handled internally */ });
  }, [apiType, apiKey, baseUrl, fetchModels]);

  const handleVerify = useCallback(() => {
    verifyModel({
      apiType,
      apiKey,
      baseUrl: apiType === 'openai-compatible' ? baseUrl : undefined,
      modelId,
    }).catch(() => { /* handled internally */ });
  }, [apiType, apiKey, baseUrl, modelId, verifyModel]);

  const handleNext = useCallback(async () => {
    const newModel: ModelConfig = {
      id: `model-${crypto.randomUUID()}`,
      displayName: displayName || modelId,
      apiType: apiType as ApiType,
      baseUrl: baseUrl || DEFAULT_BASE_URLS[apiType] || '',
      apiKey,
      modelId,
      isVerified: true,
    };

    // Save to workflow_model_library in config
    await updateAndSaveConfig({
      workflow_model_library: [newModel],
    } as Parameters<typeof updateAndSaveConfig>[0]);

    onNext();
  }, [
    displayName,
    apiType,
    baseUrl,
    apiKey,
    modelId,
    updateAndSaveConfig,
    onNext,
  ]);

  return (
    <SetupWizardStep2Model
      displayName={displayName}
      apiType={apiType}
      apiKey={apiKey}
      baseUrl={baseUrl}
      modelId={modelId}
      models={models}
      isLoadingModels={isLoadingModels}
      isVerified={isVerified}
      verifyError={verifyError}
      isVerifying={isVerifying}
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
