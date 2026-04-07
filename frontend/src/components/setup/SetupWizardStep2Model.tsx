import { useTranslation } from 'react-i18next';
import {
  CheckCircleIcon,
  WarningCircleIcon,
  WarningIcon,
  CircleNotchIcon,
  EyeIcon,
  EyeSlashIcon,
  CaretDownIcon,
  LightningIcon,
  KeyIcon,
} from '@phosphor-icons/react';

import { cn } from '@/lib/utils';
import { CLI_TYPES } from '@/components/workflow/constants';
import type { SetupModelMode } from './SetupWizardStep2ModelContainer';

const API_TYPE_OPTIONS = [
  { value: 'anthropic', labelKey: 'setup:wizard.model.apiTypes.anthropic' },
  {
    value: 'anthropic-compatible',
    labelKey: 'setup:wizard.model.apiTypes.anthropicCompatible',
  },
  { value: 'openai', labelKey: 'setup:wizard.model.apiTypes.openai' },
  { value: 'google', labelKey: 'setup:wizard.model.apiTypes.google' },
  {
    value: 'openai-compatible',
    labelKey: 'setup:wizard.model.apiTypes.openaiCompatible',
  },
] as const;

const CLI_TYPE_OPTIONS = Object.values(CLI_TYPES).map((cli) => ({
  value: cli.id,
  label: cli.label,
}));

export interface SetupWizardStep2ModelProps {
  mode: SetupModelMode;
  onModeChange: (mode: SetupModelMode) => void;
  nativeAvailable: boolean;
  isNativeLoading: boolean;
  nativeCliVersion: string | null;
  displayName: string;
  cliTypeId: string;
  onCliTypeIdChange: (v: string) => void;
  apiType: string;
  apiKey: string;
  baseUrl: string;
  modelId: string;
  models: Array<{ id: string; name: string }>;
  isLoadingModels: boolean;
  isVerified: boolean;
  verifyError: string | null;
  isVerifying: boolean;
  urlWarning: string | null;
  onDisplayNameChange: (v: string) => void;
  onApiTypeChange: (v: string) => void;
  onApiKeyChange: (v: string) => void;
  onBaseUrlChange: (v: string) => void;
  onModelIdChange: (v: string) => void;
  showApiKey: boolean;
  onToggleApiKeyVisibility: () => void;
  onFetchModels: () => void;
  onVerify: () => void;
  onNext: () => void;
  onBack: () => void;
  onSkip: () => void;
  canProceed: boolean;
}

export function SetupWizardStep2Model({
  mode,
  onModeChange,
  nativeAvailable,
  isNativeLoading,
  nativeCliVersion,
  displayName,
  cliTypeId,
  onCliTypeIdChange,
  apiType,
  apiKey,
  baseUrl,
  modelId,
  models,
  isLoadingModels,
  isVerified,
  verifyError,
  isVerifying,
  urlWarning,
  onDisplayNameChange,
  onApiTypeChange,
  onApiKeyChange,
  onBaseUrlChange,
  onModelIdChange,
  showApiKey,
  onToggleApiKeyVisibility,
  onFetchModels,
  onVerify,
  onNext,
  onBack,
  onSkip,
  canProceed,
}: Readonly<SetupWizardStep2ModelProps>) {
  const { t } = useTranslation(['setup']);

  return (
    <div className="flex flex-col items-center justify-center max-w-lg mx-auto space-y-double">
      {/* Header */}
      <div className="text-center space-y-base">
        <h1 className="text-high text-xl font-medium">
          {t('setup:wizard.model.title')}
        </h1>
        <p className="text-low text-base leading-relaxed">
          {t('setup:wizard.model.subtitle')}
        </p>
      </div>

      {/* Mode selector tabs */}
      <div className="w-full flex rounded border border-border overflow-hidden">
        <button
          type="button"
          onClick={() => onModeChange('native')}
          className={cn(
            'flex-1 flex items-center justify-center gap-half px-base py-base text-base transition-colors',
            mode === 'native'
              ? 'bg-brand/10 text-high border-b-2 border-brand font-medium'
              : 'bg-secondary text-low hover:text-normal'
          )}
        >
          <LightningIcon className="size-icon-sm" weight="fill" />
          {t('setup:wizard.model.nativeTab')}
        </button>
        <button
          type="button"
          onClick={() => onModeChange('manual')}
          className={cn(
            'flex-1 flex items-center justify-center gap-half px-base py-base text-base transition-colors',
            mode === 'manual'
              ? 'bg-brand/10 text-high border-b-2 border-brand font-medium'
              : 'bg-secondary text-low hover:text-normal'
          )}
        >
          <KeyIcon className="size-icon-sm" weight="fill" />
          {t('setup:wizard.model.manualTab')}
        </button>
      </div>

      {/* Native subscription mode */}
      {mode === 'native' && (
        <div className="w-full space-y-base">
          {isNativeLoading ? (
            <div className="flex items-center justify-center gap-half py-double text-low">
              <CircleNotchIcon className="size-icon-sm animate-spin" weight="bold" />
              <span className="text-base">{t('setup:wizard.model.nativeDetecting')}</span>
            </div>
          ) : nativeAvailable ? (
            <div className="w-full rounded border border-brand/30 bg-brand/5 p-base space-y-base">
              <div className="flex items-center gap-half">
                <CheckCircleIcon className="size-icon-sm text-success" weight="fill" />
                <span className="text-high text-base font-medium">
                  {t('setup:wizard.model.nativeDetected')}
                </span>
              </div>
              <div className="space-y-half text-base text-normal">
                <div className="flex justify-between">
                  <span className="text-low">{t('setup:wizard.model.nativeModelLabel')}</span>
                  <span className="font-mono text-high">{t('setup:wizard.model.nativeModelAuto')}</span>
                </div>
                {nativeCliVersion && (
                  <div className="flex justify-between">
                    <span className="text-low">{t('setup:wizard.model.nativeVersionLabel')}</span>
                    <span className="font-mono text-high">{nativeCliVersion}</span>
                  </div>
                )}
              </div>
              <p className="text-sm text-low">
                {t('setup:wizard.model.nativeHint')}
              </p>
            </div>
          ) : (
            <div className="w-full rounded border border-border bg-secondary p-base space-y-base">
              <div className="flex items-center gap-half">
                <WarningCircleIcon className="size-icon-sm text-low" weight="fill" />
                <span className="text-normal text-base">
                  {t('setup:wizard.model.nativeNotFound')}
                </span>
              </div>
              <p className="text-sm text-low">
                {t('setup:wizard.model.nativeNotFoundHint')}
              </p>
            </div>
          )}
        </div>
      )}

      {/* Manual API key mode */}
      {mode === 'manual' && (
        <div className="w-full space-y-base">
          {/* Display Name */}
          <div className="space-y-half">
            <label
              htmlFor="setup-display-name"
              className="text-normal text-base"
            >
              {t('setup:wizard.model.displayNameLabel')}
            </label>
            <input
              id="setup-display-name"
              type="text"
              value={displayName}
              onChange={(e) => onDisplayNameChange(e.target.value)}
              placeholder={t('setup:wizard.model.displayNamePlaceholder')}
              className={cn(
                'w-full rounded border border-border bg-secondary',
                'px-base py-base text-base text-normal',
                'placeholder:text-low placeholder:opacity-80',
                'focus:outline-none focus:ring-1 focus:ring-brand'
              )}
            />
          </div>

          {/* CLI Type */}
          <div className="space-y-half">
            <span className="text-normal text-base">
              AI CLI
            </span>
            <div className="flex flex-wrap gap-half">
              {CLI_TYPE_OPTIONS.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => onCliTypeIdChange(opt.value)}
                  className={cn(
                    'px-base py-half rounded border text-base transition-colors cursor-pointer',
                    'hover:border-brand hover:text-high',
                    cliTypeId === opt.value
                      ? 'border-brand bg-brand/10 text-high font-medium'
                      : 'border-border bg-secondary text-normal'
                  )}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          </div>

          {/* API Type (segmented buttons) */}
          <div className="space-y-half">
            <span className="text-normal text-base">
              {t('setup:wizard.model.apiTypeLabel')}
            </span>
            <div className="flex flex-wrap gap-half">
              {API_TYPE_OPTIONS.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => onApiTypeChange(opt.value)}
                  className={cn(
                    'px-base py-half rounded border text-base transition-colors cursor-pointer',
                    'hover:border-brand hover:text-high',
                    apiType === opt.value
                      ? 'border-brand bg-brand/10 text-high font-medium'
                      : 'border-border bg-secondary text-normal'
                  )}
                >
                  {t(opt.labelKey)}
                </button>
              ))}
            </div>
          </div>

          {/* API Key */}
          <div className="space-y-half">
            <label htmlFor="setup-api-key" className="text-normal text-base">
              {t('setup:wizard.model.apiKeyLabel')}
            </label>
            <div className="relative">
              <input
                id="setup-api-key"
                type={showApiKey ? 'text' : 'password'}
                value={apiKey}
                onChange={(e) => onApiKeyChange(e.target.value)}
                placeholder={t('setup:wizard.model.apiKeyPlaceholder')}
                className={cn(
                  'w-full rounded border border-border bg-secondary',
                  'px-base py-base pr-10 text-base text-normal',
                  'placeholder:text-low placeholder:opacity-80',
                  'focus:outline-none focus:ring-1 focus:ring-brand'
                )}
              />
              <button
                type="button"
                onClick={onToggleApiKeyVisibility}
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-low hover:text-high transition-colors"
                aria-label={showApiKey ? 'Hide API key' : 'Show API key'}
              >
                {showApiKey ? (
                  <EyeSlashIcon className="size-icon-sm" />
                ) : (
                  <EyeIcon className="size-icon-sm" />
                )}
              </button>
            </div>
          </div>

          {/* Base URL (only for Compatible types) */}
          {(apiType === 'openai-compatible' || apiType === 'anthropic-compatible') && (
            <div className="space-y-half">
              <label
                htmlFor="setup-base-url"
                className="text-normal text-base"
              >
                {t('setup:wizard.model.baseUrlLabel')}
              </label>
              <input
                id="setup-base-url"
                type="text"
                value={baseUrl}
                onChange={(e) => onBaseUrlChange(e.target.value)}
                placeholder={t('setup:wizard.model.baseUrlPlaceholder')}
                className={cn(
                  'w-full rounded border border-border bg-secondary',
                  'px-base py-base text-base text-normal',
                  'placeholder:text-low placeholder:opacity-80',
                  'focus:outline-none focus:ring-1 focus:ring-brand'
                )}
              />
              {urlWarning && (
                <p className="flex items-center gap-half text-xs text-warning mt-half">
                  <WarningIcon className="size-icon-xs shrink-0" weight="fill" />
                  {urlWarning}
                </p>
              )}
            </div>
          )}

          {/* Fetch Models + Model Dropdown */}
          <div className="space-y-half">
            <button
              type="button"
              onClick={onFetchModels}
              disabled={isLoadingModels || !apiKey.trim()}
              className={cn(
                'flex items-center justify-center gap-half w-full',
                'px-base py-base rounded border border-border text-base',
                'bg-secondary text-normal transition-colors',
                'hover:border-brand hover:text-high',
                'disabled:opacity-50 disabled:cursor-not-allowed'
              )}
            >
              {isLoadingModels && (
                <CircleNotchIcon className="size-icon-sm animate-spin" weight="bold" />
              )}
              {isLoadingModels
                ? t('setup:wizard.model.fetchingModels')
                : t('setup:wizard.model.fetchModelsButton')}
            </button>
          </div>

          {/* Model Selector (dropdown if models fetched, manual input otherwise) */}
          <div className="space-y-half">
            <label
              htmlFor="setup-model-id"
              className="text-normal text-base"
            >
              {t('setup:wizard.model.modelIdLabel')}
            </label>
            {models.length > 0 ? (
              <div className="relative">
                <select
                  id="setup-model-id"
                  value={modelId}
                  onChange={(e) => onModelIdChange(e.target.value)}
                  className={cn(
                    'w-full appearance-none rounded border border-border bg-secondary',
                    'px-base py-base pr-8 text-base text-normal',
                    'focus:outline-none focus:ring-1 focus:ring-brand'
                  )}
                >
                  <option value="">
                    {t('setup:wizard.model.modelIdPlaceholder')}
                  </option>
                  {models.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name}
                    </option>
                  ))}
                </select>
                <CaretDownIcon
                  className="size-icon-xs absolute right-2 top-1/2 -translate-y-1/2 text-low pointer-events-none"
                  weight="bold"
                />
              </div>
            ) : (
              <input
                id="setup-model-id"
                type="text"
                value={modelId}
                onChange={(e) => onModelIdChange(e.target.value)}
                placeholder={t('setup:wizard.model.modelIdManualPlaceholder', { defaultValue: t('setup:wizard.model.modelIdPlaceholder') })}
                className={cn(
                  'w-full rounded border border-border bg-secondary',
                  'px-base py-base text-base text-normal',
                  'placeholder:text-low placeholder:opacity-80',
                  'focus:outline-none focus:ring-1 focus:ring-brand'
                )}
              />
            )}
          </div>

          {/* Verify Connection */}
          {modelId && (
            <div className="space-y-half">
              <button
                type="button"
                onClick={onVerify}
                disabled={isVerifying || !modelId}
                className={cn(
                  'flex items-center justify-center gap-half w-full',
                  'px-base py-base rounded border border-border text-base',
                  'bg-secondary text-normal transition-colors',
                  'hover:border-brand hover:text-high',
                  'disabled:opacity-50 disabled:cursor-not-allowed'
                )}
              >
                {isVerifying && (
                  <CircleNotchIcon
                    className="size-icon-sm animate-spin"
                    weight="bold"
                  />
                )}
                {isVerifying
                  ? t('setup:wizard.model.verifying')
                  : t('setup:wizard.model.verifyButton')}
              </button>

              {/* Verification result */}
              {isVerified && (
                <div className="flex items-center gap-half text-success text-base">
                  <CheckCircleIcon className="size-icon-sm" weight="fill" />
                  <span>{t('setup:wizard.model.verified')}</span>
                </div>
              )}
              {verifyError && !isVerified && (
                <div className="flex items-center gap-half text-error text-base">
                  <WarningCircleIcon className="size-icon-sm" weight="fill" />
                  <span>{t('setup:wizard.model.verifyFailed')}</span>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* Skip hint */}
      <p className="text-low text-sm text-center">
        {t('setup:wizard.model.skipHint')}
      </p>

      {/* Action buttons */}
      <div className="w-full flex flex-col items-center gap-base pt-base">
        <div className="w-full flex gap-base">
          <button
            type="button"
            onClick={onBack}
            className={cn(
              'flex-1 border border-border bg-secondary text-normal',
              'px-double py-base rounded font-medium text-base',
              'hover:text-high hover:border-brand transition-colors',
              'focus:outline-none focus:ring-1 focus:ring-brand focus:ring-offset-1'
            )}
          >
            {t('setup:wizard.welcome.backButton')}
          </button>
          <button
            type="button"
            onClick={onNext}
            disabled={!canProceed}
            className={cn(
              'flex-[2] bg-brand text-white px-double py-base rounded font-medium text-base',
              'hover:opacity-90 transition-opacity',
              'focus:outline-none focus:ring-1 focus:ring-brand focus:ring-offset-1',
              'disabled:opacity-50 disabled:cursor-not-allowed'
            )}
          >
            {t('setup:wizard.welcome.continueButton')}
          </button>
        </div>
        <button
          type="button"
          onClick={onSkip}
          className="text-low text-sm hover:text-normal underline transition-colors"
        >
          {t('setup:wizard.welcome.skipButton')}
        </button>
      </div>
    </div>
  );
}
