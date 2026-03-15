import React, { useState } from 'react';
import {
  PlusIcon,
  PencilSimpleIcon,
  TrashIcon,
  CheckCircleIcon,
  ArrowsClockwiseIcon,
  EyeIcon,
  EyeSlashIcon,
} from '@phosphor-icons/react';
import { Field, FieldLabel, FieldError } from '../../ui-new/primitives/Field';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '../../ui-new/primitives/Dialog';
import { IconButton } from '../../ui-new/primitives/IconButton';
import { cn } from '@/lib/utils';
import { CLI_TYPES } from '../constants';
import type { WizardConfig, ModelConfig, ApiType } from '../types';
import { useTranslation } from 'react-i18next';
import { useModelStore } from '@/stores/modelStore';
import { useToast } from '@/components/ui/toast';

interface Step3ModelsProps {
  config: WizardConfig;
  onUpdate: (updates: Partial<WizardConfig>) => void;
  dialogContentClassName?: string;
}

const API_TYPES = {
  anthropic: {
    label: 'Anthropic',
    defaultBaseUrl: 'https://api.anthropic.com',
    defaultModels: ['claude-3-5-sonnet-20241022', 'claude-3-5-haiku-20241022'],
  },
  google: {
    label: 'Google',
    defaultBaseUrl: 'https://generativelanguage.googleapis.com',
    defaultModels: ['gemini-2.0-flash-exp', 'gemini-1.5-pro'],
  },
  openai: {
    label: 'OpenAI',
    defaultBaseUrl: 'https://api.openai.com',
    defaultModels: ['gpt-4', 'gpt-4-turbo', 'gpt-3.5-turbo'],
  },
  'openai-compatible': {
    label: 'OpenAI Compatible',
    defaultBaseUrl: '',
    defaultModels: [],
  },
} as const;

interface ModelFormData {
  displayName: string;
  cliTypeId: string;
  apiType: ApiType;
  baseUrl: string;
  apiKey: string;
  modelId: string;
}

const initialFormData: ModelFormData = {
  displayName: '',
  cliTypeId: '',
  apiType: 'anthropic',
  baseUrl: API_TYPES.anthropic.defaultBaseUrl,
  apiKey: '',
  modelId: '',
};

/**
 * Step 3: Manages AI model configurations and verification.
 */
export const Step3Models: React.FC<Step3ModelsProps> = ({
  config,
  onUpdate,
  dialogContentClassName,
}) => {
  const { t } = useTranslation('workflow');
  const { showToast } = useToast();
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingModel, setEditingModel] = useState<ModelConfig | null>(null);
  const [formData, setFormData] = useState<ModelFormData>(initialFormData);
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  const [isFetching, setIsFetching] = useState(false);
  const [isVerifying, setIsVerifying] = useState(false);
  const [showApiKey, setShowApiKey] = useState(false);
  const [isFormVerified, setIsFormVerified] = useState(false);

  const handleOpenAddDialog = () => {
    setEditingModel(null);
    setFormData({
      displayName: '',
      cliTypeId: '',
      apiType: 'anthropic',
      baseUrl: API_TYPES.anthropic.defaultBaseUrl,
      apiKey: '',
      modelId: '',
    });
    setAvailableModels([]);
    setFormErrors({});
    setIsFormVerified(false);
    setIsDialogOpen(true);
  };

  const handleOpenEditDialog = (model: ModelConfig) => {
    setEditingModel(model);
    setFormData({
      displayName: model.displayName,
      cliTypeId: model.cliTypeId ?? '',
      apiType: model.apiType,
      baseUrl: model.baseUrl,
      apiKey: model.apiKey,
      modelId: model.modelId,
    });
    setAvailableModels([...API_TYPES[model.apiType].defaultModels]);
    setFormErrors({});
    // Initialize verification state from the model being edited
    setIsFormVerified(model.isVerified);
    setIsDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setIsDialogOpen(false);
    setEditingModel(null);
    setFormData(initialFormData);
    setAvailableModels([]);
    setFormErrors({});
    setShowApiKey(false);
    setIsFormVerified(false);
  };

  const handleApiTypeChange = (apiType: ApiType) => {
    const config = API_TYPES[apiType];
    setFormData({
      ...formData,
      apiType,
      baseUrl: config.defaultBaseUrl,
      modelId: '',
    });
    setAvailableModels([...config.defaultModels]);
    // Clear verification state when API type changes
    setIsFormVerified(false);
  };

  const handleFetchModels = async () => {
    if (!formData.apiKey.trim()) {
      setFormErrors({ apiKey: t('step3.errors.apiKeyRequired') });
      return;
    }

    setIsFetching(true);
    setFormErrors({});

    try {
      // Use modelStore to fetch models from API
      const { fetchModels } = useModelStore.getState();
      const models = await fetchModels(
        formData.apiType,
        formData.apiKey,
        formData.apiType === 'openai-compatible' ? formData.baseUrl : undefined
      );
      setAvailableModels(models);
    } catch (error) {
      console.debug('Failed to fetch models, using defaults', error);
      // Fallback to default models on error
      const defaultModels = API_TYPES[formData.apiType].defaultModels;
      setAvailableModels([...defaultModels]);
      setFormErrors({ fetch: t('step3.errors.fetchFailed') });
    } finally {
      setIsFetching(false);
    }
  };

  const handleVerify = async () => {
    if (!formData.baseUrl || !formData.apiKey || !formData.modelId) {
      setFormErrors({ verify: t('step3.errors.verifyMissingInfo') });
      return;
    }

    setIsVerifying(true);
    setFormErrors({});

    try {
      // Use modelStore to verify model connection
      const { verifyModel } = useModelStore.getState();
      const tempModel: ModelConfig = {
        id: editingModel?.id ?? `temp-${crypto.randomUUID()}`,
        displayName: formData.displayName || 'Temp',
        cliTypeId: formData.cliTypeId,
        apiType: formData.apiType,
        baseUrl: formData.baseUrl,
        apiKey: formData.apiKey,
        modelId: formData.modelId,
        isVerified: false,
      };

      const verified = await verifyModel(tempModel);

      if (verified) {
        // Update form verification state for use when saving
        setIsFormVerified(true);
        setFormErrors({});
        // Show success toast notification
        showToast(t('step3.messages.verifySuccess'), 'success');
      } else {
        setIsFormVerified(false);
        setFormErrors({ verify: t('step3.errors.verifyFailed') });
      }
    } catch (error) {
      console.error('Model verification failed', error);
      setIsFormVerified(false);
      setFormErrors({ verify: t('step3.errors.verifyFailed') });
    } finally {
      setIsVerifying(false);
    }
  };

  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    if (!formData.displayName.trim()) {
      errors.displayName = t('step3.errors.displayNameRequired');
    }
    if (!formData.cliTypeId.trim()) {
      errors.cliTypeId = t('validation.terminals.cliRequired');
    }
    if (!formData.baseUrl.trim()) {
      errors.baseUrl = t('step3.errors.baseUrlRequired');
    }
    if (!formData.apiKey.trim()) {
      errors.apiKey = t('step3.errors.apiKeyRequired');
    }
    if (!formData.modelId.trim()) {
      errors.modelId = t('step3.errors.modelIdRequired');
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSave = () => {
    if (!validateForm()) {
      return;
    }

    const newModel: ModelConfig = {
      id: editingModel?.id ?? `model-${crypto.randomUUID()}`,
      displayName: formData.displayName,
      cliTypeId: formData.cliTypeId,
      apiType: formData.apiType,
      baseUrl: formData.baseUrl,
      apiKey: formData.apiKey,
      modelId: formData.modelId,
      isVerified: isFormVerified,
    };

    let updatedModels: ModelConfig[];
    if (editingModel) {
      updatedModels = config.models.map((m) =>
        m.id === editingModel.id ? newModel : m
      );
    } else {
      updatedModels = [...config.models, newModel];
    }

    onUpdate({ models: updatedModels });
    handleCloseDialog();
  };

  const handleDelete = (modelId: string) => {
    const modelName =
      config.models.find((model) => model.id === modelId)?.displayName ?? '';
    if (globalThis.window.confirm(t('step3.messages.confirmDelete', { name: modelName }))) {
      const updatedModels = config.models.filter((m) => m.id !== modelId);
      onUpdate({ models: updatedModels });
    }
  };

  return (
    <div className="flex flex-col gap-base">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-high">{t('step3.title')}</h2>
        <button
          type="button"
          onClick={handleOpenAddDialog}
          className="flex items-center gap-half px-base py-half rounded-sm bg-brand text-on-brand text-base hover:bg-brand-hover transition-colors"
        >
          <PlusIcon className="size-icon-sm" weight="bold" />
          {t('step3.addModel')}
        </button>
      </div>

      {config.models.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-double text-low">
          <div className="text-base">{t('step3.emptyTitle')}</div>
          <div className="text-sm mt-half">{t('step3.emptyDescription')}</div>
        </div>
      ) : (
        <div className="flex flex-col gap-base">
          {config.models.map((model) => (
            <div
              key={model.id}
              className="bg-secondary border rounded-sm p-base flex items-center justify-between"
            >
              <div className="flex items-center gap-base flex-1">
                {model.isVerified && (
                  <CheckCircleIcon
                    className="size-icon-sm text-success"
                    weight="fill"
                    data-testid={`verified-badge-${model.id}`}
                  />
                )}
                <div className="flex-1">
                  <div className="text-base font-medium text-high">{model.displayName}</div>
                  <div className="text-sm text-low mt-quarter">
                    {API_TYPES[model.apiType].label} - {model.modelId}
                    {model.cliTypeId
                      ? ` · ${(CLI_TYPES as Record<string, { label: string }>)[model.cliTypeId]?.label ?? model.cliTypeId}`
                      : ''}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-half">
                <IconButton
                  icon={PencilSimpleIcon}
                  onClick={() => {
                    handleOpenEditDialog(model);
                  }}
                  aria-label={`${t('step3.editLabel')} ${model.displayName}`}
                  title={t('step3.editLabel')}
                />
                <IconButton
                  icon={TrashIcon}
                  onClick={() => {
                    handleDelete(model.id);
                  }}
                  aria-label={`${t('step3.deleteLabel')} ${model.displayName}`}
                  title={t('step3.deleteLabel')}
                />
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Add/Edit Model Dialog */}
      <Dialog open={isDialogOpen} onOpenChange={(open) => {
        if (!open) {
          handleCloseDialog();
        }
      }}>
        <DialogContent
          className={cn(
            'max-w-2xl max-h-[90vh] overflow-y-auto',
            dialogContentClassName
          )}
        >
          <DialogHeader>
            <DialogTitle>
              {editingModel ? t('step3.dialog.editTitle') : t('step3.dialog.addTitle')}
            </DialogTitle>
            <DialogDescription className="sr-only">
              {t('step3.dialog.description')}
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-base py-base">
            {/* Display Name */}
            <Field>
              <FieldLabel htmlFor="displayName">{t('step3.fields.displayName.label')}</FieldLabel>
              <input
                id="displayName"
                type="text"
                value={formData.displayName}
                onChange={(e) => {
                  setFormData({ ...formData, displayName: e.target.value });
                }}
                placeholder={t('step3.fields.displayName.placeholder')}
                className={cn(
                  'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                  'placeholder:text-low placeholder:opacity-80',
                  'focus:outline-none focus:ring-1 focus:ring-brand',
                  formErrors.displayName && 'border-error'
                )}
              />
              {formErrors.displayName && <FieldError>{formErrors.displayName}</FieldError>}
            </Field>

            <Field>
              <FieldLabel htmlFor="cliTypeId">{t('step4.cliTypeLabel')}</FieldLabel>
              <select
                id="cliTypeId"
                value={formData.cliTypeId}
                onChange={(e) => {
                  setFormData({ ...formData, cliTypeId: e.target.value });
                }}
                className={cn(
                  'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                  'focus:outline-none focus:ring-1 focus:ring-brand',
                  formErrors.cliTypeId && 'border-error'
                )}
              >
                <option value="">{t('step6.errorTerminal.cliPlaceholder')}</option>
                {Object.values(CLI_TYPES).map((cli) => (
                  <option key={cli.id} value={cli.id}>
                    {cli.label}
                  </option>
                ))}
              </select>
              {formErrors.cliTypeId && <FieldError>{formErrors.cliTypeId}</FieldError>}
            </Field>

            {/* API Type */}
            <Field>
              <FieldLabel htmlFor="apiType">{t('step3.fields.apiType.label')}</FieldLabel>
              <div className="flex flex-wrap gap-3">
                {(Object.keys(API_TYPES) as ApiType[]).map((type) => (
                  <label
                    key={type}
                    className={cn(
                      'inline-flex items-center px-3 py-2 rounded-md border text-base cursor-pointer transition-colors',
                      'hover:border-brand hover:text-high',
                      formData.apiType === type
                        ? 'border-brand bg-brand/10 text-high'
                        : 'border-border text-normal bg-secondary'
                    )}
                  >
                    <input
                      type="radio"
                      name="apiType"
                      value={type}
                      checked={formData.apiType === type}
                      onChange={() => {
                        handleApiTypeChange(type);
                      }}
                      className="hidden"
                    />
                    {API_TYPES[type].label}
                  </label>
                ))}
              </div>
            </Field>

            {/* Base URL */}
            <Field>
              <FieldLabel htmlFor="baseUrl">{t('step3.fields.baseUrl.label')}</FieldLabel>
              <input
                id="baseUrl"
                type="text"
                value={formData.baseUrl}
                onChange={(e) => {
                  setFormData({ ...formData, baseUrl: e.target.value });
                  setIsFormVerified(false);
                }}
                placeholder={t('step3.fields.baseUrl.placeholder')}
                disabled={formData.apiType !== 'openai-compatible'}
                className={cn(
                  'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                  'placeholder:text-low placeholder:opacity-80',
                  'focus:outline-none focus:ring-1 focus:ring-brand',
                  'disabled:opacity-50 disabled:cursor-not-allowed',
                  formErrors.baseUrl && 'border-error'
                )}
              />
              {formErrors.baseUrl && <FieldError>{formErrors.baseUrl}</FieldError>}
            </Field>

            {/* API Key */}
            <Field>
              <FieldLabel htmlFor="apiKey">{t('step3.fields.apiKey.label')}</FieldLabel>
              <div className="relative">
                <input
                  id="apiKey"
                  type={showApiKey ? 'text' : 'password'}
                  value={formData.apiKey}
                  onChange={(e) => {
                    setFormData({ ...formData, apiKey: e.target.value });
                    setIsFormVerified(false);
                  }}
                  placeholder={t('step3.fields.apiKey.placeholder')}
                  className={cn(
                    'w-full bg-secondary rounded-sm border px-base py-half pr-10 text-base text-high',
                    'placeholder:text-low placeholder:opacity-80',
                    'focus:outline-none focus:ring-1 focus:ring-brand',
                    formErrors.apiKey && 'border-error'
                  )}
                />
                <button
                  type="button"
                  onClick={() => setShowApiKey(!showApiKey)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-low hover:text-high transition-colors"
                  aria-label={showApiKey ? t('step3.fields.apiKey.hide') : t('step3.fields.apiKey.show')}
                >
                  {showApiKey ? (
                    <EyeSlashIcon className="size-icon-sm" />
                  ) : (
                    <EyeIcon className="size-icon-sm" />
                  )}
                </button>
              </div>
              {formErrors.apiKey && <FieldError>{formErrors.apiKey}</FieldError>}
            </Field>

            {/* Fetch Models Button */}
            <Field>
              <button
                type="button"
                onClick={() => {
                  void handleFetchModels();
                }}
                disabled={isFetching || !formData.apiKey}
                className={cn(
                  'flex items-center justify-center gap-half w-full px-base py-half rounded-sm border text-base',
                  'hover:border-brand hover:text-high transition-colors',
                  'disabled:opacity-50 disabled:cursor-not-allowed',
                  'bg-secondary text-normal'
                )}
              >
                {isFetching && <ArrowsClockwiseIcon className="size-icon-sm animate-spin" />}
                {isFetching ? t('step3.actions.fetching') : t('step3.actions.fetchModels')}
              </button>
              {formErrors.fetch && <FieldError>{formErrors.fetch}</FieldError>}
            </Field>

            {/* Model Selection/Input */}
            <Field>
              <FieldLabel htmlFor="modelId">{t('step3.fields.modelId.label')}</FieldLabel>
              {availableModels.length > 0 ? (
                <select
                  id="modelId"
                  value={formData.modelId}
                  onChange={(e) => {
                    setFormData({ ...formData, modelId: e.target.value });
                    setIsFormVerified(false);
                  }}
                  className={cn(
                    'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                    'focus:outline-none focus:ring-1 focus:ring-brand',
                    formErrors.modelId && 'border-error'
                  )}
                >
                  <option value="">{t('step3.fields.modelId.selectPlaceholder')}</option>
                  {availableModels.map((model) => (
                    <option key={model} value={model}>
                      {model}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  id="modelId"
                  type="text"
                  value={formData.modelId}
                  onChange={(e) => {
                    setFormData({ ...formData, modelId: e.target.value });
                    setIsFormVerified(false);
                  }}
                  placeholder={t('step3.fields.modelId.placeholder')}
                  className={cn(
                    'w-full bg-secondary rounded-sm border px-base py-half text-base text-high',
                    'placeholder:text-low placeholder:opacity-80',
                    'focus:outline-none focus:ring-1 focus:ring-brand',
                    formErrors.modelId && 'border-error'
                  )}
                />
              )}
              {formErrors.modelId && <FieldError>{formErrors.modelId}</FieldError>}
            </Field>

            {/* Verify Connection Button */}
            <Field>
              <button
                type="button"
                onClick={() => {
                  void handleVerify();
                }}
                disabled={isVerifying}
                className={cn(
                  'flex items-center justify-center gap-half w-full px-base py-half rounded-sm border text-base',
                  'hover:border-brand hover:text-high transition-colors',
                  'disabled:opacity-50 disabled:cursor-not-allowed',
                  'bg-secondary text-normal'
                )}
              >
                {isVerifying ? t('step3.actions.verifying') : t('step3.actions.verify')}
              </button>
              {formErrors.verify && <FieldError>{formErrors.verify}</FieldError>}
            </Field>
          </div>

          <DialogFooter className="sticky bottom-0 z-10 border-t border-border bg-[hsl(var(--card))] px-base py-base">
            <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:justify-end">
              <button
                type="button"
                onClick={handleCloseDialog}
                className={cn(
                  'w-full sm:w-auto min-w-[5.5rem] px-base py-half rounded-sm border text-base whitespace-nowrap',
                  'bg-secondary text-normal hover:bg-panel hover:text-high',
                  'transition-colors'
                )}
              >
                {t('step3.actions.cancel')}
              </button>
              <button
                type="button"
                onClick={handleSave}
                className={cn(
                  'w-full sm:w-auto min-w-[5.5rem] px-base py-half rounded-sm text-base whitespace-nowrap',
                  'bg-brand text-on-brand hover:bg-brand-hover',
                  'transition-colors'
                )}
              >
                {t('step3.actions.save')}
              </button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};
