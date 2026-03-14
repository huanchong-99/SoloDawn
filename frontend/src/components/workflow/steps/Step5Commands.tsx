import React, { useState, useEffect } from 'react';
import { CaretDownIcon, CaretUpIcon, DotsSixVerticalIcon, PlusIcon, XIcon, PencilSimpleIcon, CheckIcon } from '@phosphor-icons/react';
import { Field, FieldLabel, FieldError } from '../../ui-new/primitives/Field';
import { cn } from '@/lib/utils';
import type { CommandConfig } from '../types';
import { useErrorNotification } from '@/hooks/useErrorNotification';
import { useTranslation } from 'react-i18next';

// ============================================================================
// Types
// ============================================================================

interface CommandPreset {
  id: string;
  name: string;
  displayName?: string;
  description?: string;
  descriptionKey?: string;
  isSystem: boolean;
}

interface Step5CommandsProps {
  config: CommandConfig;
  errors: Record<string, string>;
  onUpdate: (updates: Partial<CommandConfig>) => void;
  onError?: (error: Error) => void;
}

// ============================================================================
// Constants
// ============================================================================

/** System presets - built-in command presets */
export const SYSTEM_PRESETS: CommandPreset[] = [
  {
    id: 'write-code',
    name: 'write-code',
    descriptionKey: 'step5.presets.writeCode.description',
    isSystem: true,
  },
  {
    id: 'review',
    name: 'review',
    descriptionKey: 'step5.presets.review.description',
    isSystem: true,
  },
  {
    id: 'fix-issues',
    name: 'fix-issues',
    descriptionKey: 'step5.presets.fixIssues.description',
    isSystem: true,
  },
  {
    id: 'test',
    name: 'test',
    descriptionKey: 'step5.presets.test.description',
    isSystem: true,
  },
  {
    id: 'refactor',
    name: 'refactor',
    descriptionKey: 'step5.presets.refactor.description',
    isSystem: true,
  },
  {
    id: 'document',
    name: 'document',
    descriptionKey: 'step5.presets.document.description',
    isSystem: true,
  },
];

/** Default preset IDs */
const DEFAULT_PRESET_IDS = ['write-code', 'review'];

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null;

const isCommandPreset = (value: unknown): value is CommandPreset => {
  if (!isRecord(value)) return false;
  return (
    typeof value.id === 'string' &&
    typeof value.name === 'string' &&
    typeof value.isSystem === 'boolean'
  );
};

const parseJson = async (response: Response): Promise<unknown> => {
  try {
    return (await response.json()) as unknown;
  } catch {
    return null;
  }
};

/** Format command name with slash prefix */
const formatCommandLabel = (name: string): string =>
  name.startsWith('/') ? name : `/${name}`;

// ============================================================================
// PresetEditorModal Component
// ============================================================================

interface PresetEditorModalProps {
  presetLabel: string;
  initialDescription: string;
  initialCommands: string[];
  initialParams: Record<string, unknown> | null;
  onSave: (description: string, commands: string[], params: Record<string, unknown>) => void;
  onCancel: () => void;
}

const PresetEditorModal: React.FC<PresetEditorModalProps> = ({
  presetLabel,
  initialDescription,
  initialCommands,
  initialParams,
  onSave,
  onCancel,
}) => {
  const { t } = useTranslation('workflow');
  const [description, setDescription] = useState(initialDescription);
  const [commandsText, setCommandsText] = useState(initialCommands.join('\n'));
  const [showAdvanced, setShowAdvanced] = useState(initialParams !== null && Object.keys(initialParams).length > 0);
  const [jsonText, setJsonText] = useState(
    initialParams ? JSON.stringify(initialParams, null, 2) : ''
  );
  const [error, setError] = useState<string | null>(null);

  const validateAndSave = () => {
    // Parse commands (one per line, filter empty lines)
    const commands = commandsText
      .split('\n')
      .map(line => line.trim())
      .filter(line => line.length > 0)
      .map(line => line.startsWith('/') ? line : `/${line}`); // Ensure slash prefix

    // Parse JSON params if provided
    try {
      const trimmedJson = jsonText.trim();
      const parsed = trimmedJson === '' ? {} : JSON.parse(trimmedJson);
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
        setError(t('step5.params.error.notObject'));
        return;
      }
      onSave(description.trim(), commands, parsed as Record<string, unknown>);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('step5.params.error.invalidJson'));
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-primary rounded-lg border p-double max-w-2xl w-full mx-4 shadow-lg max-h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-base">
          <h3 className="text-lg text-high font-semibold">
            {t('step5.editPreset.title')} {presetLabel}
          </h3>
          <button
            type="button"
            onClick={onCancel}
            className="flex items-center justify-center p-half rounded text-low hover:text-normal"
            aria-label={t('step5.params.cancel')}
          >
            <XIcon className="size-icon-sm" />
          </button>
        </div>

        {/* Command Description */}
        <div className="mb-base">
          <label className="block text-sm text-low mb-half">
            {t('step5.editPreset.descriptionLabel')}
          </label>
          <input
            type="text"
            value={description}
            onChange={(e) => {
              setDescription(e.target.value);
            }}
            className={cn(
              'w-full px-base py-half bg-secondary rounded border text-base text-normal',
              'focus:outline-none focus:ring-1 focus:ring-brand'
            )}
            placeholder={t('step5.editPreset.descriptionPlaceholder')}
          />
          <div className="mt-half text-xs text-low">
            {t('step5.editPreset.descriptionHint')}
          </div>
        </div>

        {/* Additional Commands Section */}
        <div className="mb-base">
          <label className="block text-sm text-low mb-half">
            {t('step5.commands.title')}
          </label>
          <p className="text-xs text-low mb-half">
            {t('step5.commands.description')}
          </p>
          <textarea
            value={commandsText}
            onChange={(e) => {
              setCommandsText(e.target.value);
            }}
            className={cn(
              'w-full h-24 px-base py-base bg-secondary rounded border text-base text-normal font-ibm-plex-mono',
              'focus:outline-none focus:ring-1 focus:ring-brand'
            )}
            placeholder={t('step5.commands.placeholder')}
            spellCheck={false}
          />
        </div>

        {/* Advanced: JSON Parameters (collapsible) */}
        <div className="mb-base">
          <button
            type="button"
            onClick={() => setShowAdvanced(!showAdvanced)}
            className="flex items-center gap-half text-sm text-low hover:text-normal"
          >
            {showAdvanced ? <CaretUpIcon className="size-icon-sm" /> : <CaretDownIcon className="size-icon-sm" />}
            {t('step5.params.advancedTitle')}
          </button>

          {showAdvanced && (
            <div className="mt-base">
              <p className="text-xs text-low mb-half">
                {t('step5.params.description')}
              </p>
              <textarea
                value={jsonText}
                onChange={(e) => {
                  setJsonText(e.target.value);
                  setError(null);
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && e.ctrlKey) {
                    validateAndSave();
                  }
                }}
                className={cn(
                  'w-full h-32 px-base py-base bg-secondary rounded border text-base text-normal font-ibm-plex-mono',
                  'focus:outline-none focus:ring-1 focus:ring-brand',
                  error && 'border-error'
                )}
                placeholder={'{\n  "key": "value"\n}'}
                spellCheck={false}
              />
              {error && (
                <div className="mt-half text-sm text-error">
                  {error}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between mt-base">
          <div className="text-xs text-low">
            {t('step5.params.hint')}
          </div>
          <div className="flex gap-base">
            <button
              type="button"
              onClick={onCancel}
              className={cn(
                'px-base py-half rounded-sm border text-base text-low',
                'hover:text-normal hover:border-brand'
              )}
            >
              {t('step5.params.cancel')}
            </button>
            <button
              type="button"
              onClick={validateAndSave}
              className={cn(
                'px-base py-half rounded-sm border text-base text-normal',
                'bg-brand text-white hover:opacity-90',
                'flex items-center gap-half'
              )}
            >
              <CheckIcon className="size-icon-sm" />
              {t('step5.params.save')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

// ============================================================================
// Component
// ============================================================================

/**
 * Step 5: Configures slash command presets and ordering.
 */
export const Step5Commands: React.FC<Step5CommandsProps> = ({
  config,
  errors,
  onUpdate,
  onError,
}) => {
  const { notifyError } = useErrorNotification({ onError, context: 'Step5Commands' });
  const { t } = useTranslation('workflow');
  const [userPresets, setUserPresets] = useState<CommandPreset[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [editingPresetId, setEditingPresetId] = useState<string | null>(null);

  /** Get command label with slash prefix */
  const getPresetLabel = (preset: CommandPreset): string =>
    formatCommandLabel(preset.name);

  /** Get default description from translation */
  const getDefaultDescription = (preset: CommandPreset): string =>
    preset.descriptionKey ? t(preset.descriptionKey) : preset.description ?? '';

  /** Get description (custom or default) */
  const getPresetDescription = (preset: CommandPreset): string => {
    const customDescription = config.customDescriptions?.[preset.id];
    if (customDescription !== undefined && customDescription !== '') {
      return customDescription;
    }
    return getDefaultDescription(preset);
  };

  // Fetch user presets from API
  useEffect(() => {
    const fetchUserPresets = async () => {
      setIsLoading(true);
      try {
        const response = await fetch('/api/workflows/presets/commands');
        if (response.ok) {
          const data = await parseJson(response);
          if (Array.isArray(data)) {
            const presets = data.filter(isCommandPreset);
            setUserPresets(presets.filter((preset) => !preset.isSystem));
          }
        }
      } catch (error) {
        notifyError(error, 'fetchUserPresets');
      } finally {
        setIsLoading(false);
      }
    };

    void fetchUserPresets();
  }, [notifyError]);

  // Combine all presets
  const allPresets = [...SYSTEM_PRESETS, ...userPresets];

  // Get selected preset objects
  const selectedPresets = allPresets.filter((p) =>
    config.presetIds.includes(p.id)
  );

  // Handlers
  const handleEnable = () => {
    onUpdate({ enabled: true });
  };

  const handleDisable = () => {
    onUpdate({ enabled: false, presetIds: [] });
  };

  const addPreset = (presetId: string) => {
    if (config.presetIds.includes(presetId)) return; // Prevent duplicates
    onUpdate({ presetIds: [...config.presetIds, presetId] });
  };

  const removePreset = (presetId: string) => {
    onUpdate({
      presetIds: config.presetIds.filter((id) => id !== presetId),
    });
  };

  const clearAll = () => {
    onUpdate({ presetIds: [] });
  };

  const resetDefault = () => {
    onUpdate({ presetIds: [...DEFAULT_PRESET_IDS] });
  };

  const moveUp = (index: number) => {
    if (index === 0) return;
    const newPresetIds = [...config.presetIds];
    [newPresetIds[index - 1], newPresetIds[index]] = [
      newPresetIds[index],
      newPresetIds[index - 1],
    ];
    onUpdate({ presetIds: newPresetIds });
  };

  const moveDown = (index: number) => {
    if (index === config.presetIds.length - 1) return;
    const newPresetIds = [...config.presetIds];
    [newPresetIds[index], newPresetIds[index + 1]] = [
      newPresetIds[index + 1],
      newPresetIds[index],
    ];
    onUpdate({ presetIds: newPresetIds });
  };

  const handleEditPreset = (presetId: string) => {
    setEditingPresetId(presetId);
  };

  const handleSavePreset = (
    presetId: string,
    description: string,
    commands: string[],
    params: Record<string, unknown>
  ) => {
    const preset = allPresets.find((p) => p.id === presetId);
    const defaultDescription = preset ? getDefaultDescription(preset) : '';

    // Update custom descriptions
    const currentDescriptions = config.customDescriptions || {};
    const nextDescriptions = { ...currentDescriptions };

    // Only store if different from default
    if (description && description !== defaultDescription) {
      nextDescriptions[presetId] = description;
    } else {
      delete nextDescriptions[presetId];
    }

    // Update additional commands
    const currentCommands = config.additionalCommands || {};
    const nextCommands = { ...currentCommands };

    // Only store if not empty
    if (commands.length > 0) {
      nextCommands[presetId] = commands;
    } else {
      delete nextCommands[presetId];
    }

    // Update custom params
    const currentParams = config.customParams || {};
    const nextParams = { ...currentParams };

    // Only store if not empty
    if (Object.keys(params).length > 0) {
      nextParams[presetId] = params;
    } else {
      delete nextParams[presetId];
    }

    onUpdate({
      customDescriptions: Object.keys(nextDescriptions).length > 0 ? nextDescriptions : undefined,
      additionalCommands: Object.keys(nextCommands).length > 0 ? nextCommands : undefined,
      customParams: Object.keys(nextParams).length > 0 ? nextParams : undefined,
    });
    setEditingPresetId(null);
  };

  const handleCancelPreset = () => {
    setEditingPresetId(null);
  };

  // Get the preset being edited
  const editingPreset = editingPresetId
    ? allPresets.find((p) => p.id === editingPresetId) ?? null
    : null;

  return (
    <div className="flex flex-col gap-base">
      {/* Enable/Disable Radio Buttons */}
      <Field>
        <FieldLabel>{t('step5.title')}</FieldLabel>
        <div className="flex flex-col gap-base">
          <label className="flex items-center gap-base cursor-pointer">
            <input
              type="radio"
              name="commandsEnabled"
              checked={config.enabled}
              onChange={handleEnable}
              className="size-icon-sm accent-brand"
            />
            <span className="text-base text-normal">
              {t('step5.enableLabel')}
            </span>
          </label>
          <label className="flex items-center gap-base cursor-pointer">
            <input
              type="radio"
              name="commandsEnabled"
              checked={!config.enabled}
              onChange={handleDisable}
              className="size-icon-sm accent-brand"
            />
            <span className="text-base text-normal">
              {t('step5.disableLabel')}
            </span>
          </label>
        </div>
        {errors.enabled && <FieldError>{t(errors.enabled)}</FieldError>}
      </Field>

      {/* Command List (shown only when enabled) */}
      {config.enabled && (
        <>
          {/* Selected Commands Section */}
          <Field>
            <FieldLabel>{t('step5.selectedTitle')}</FieldLabel>

            {selectedPresets.length === 0 ? (
              /* Empty State */
              <div className="bg-secondary rounded-sm border p-double text-center">
                <p className="text-base text-low">
                  {t('step5.selectedEmpty')}
                </p>
              </div>
            ) : (
              /* Command List */
              <div className="flex flex-col gap-sm">
                {selectedPresets.map((preset, index) => (
                  <div
                    key={preset.id}
                    className="flex items-center gap-base bg-secondary rounded-sm border px-base py-half"
                  >
                    {/* Drag Handle */}
                    <DotsSixVerticalIcon className="size-icon-sm text-low shrink-0" />

                    {/* Command Info */}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-half">
                        <div className="text-base text-high truncate font-ibm-plex-mono">
                          {getPresetLabel(preset)}
                        </div>
                        {(config.customParams?.[preset.id] || config.customDescriptions?.[preset.id] || config.additionalCommands?.[preset.id]?.length) && (
                          <span className="text-xs text-brand px-half py-quarter rounded border border-brand/30">
                            {t('step5.params.configured')}
                          </span>
                        )}
                      </div>
                      <div className="text-xs text-low truncate">
                        {getPresetDescription(preset)}
                      </div>
                      {/* Show additional commands if any */}
                      {config.additionalCommands?.[preset.id]?.length ? (
                        <div className="text-xs text-low mt-half font-ibm-plex-mono">
                          + {config.additionalCommands[preset.id].join(', ')}
                        </div>
                      ) : null}
                    </div>

                    {/* Edit Button */}
                    <button
                      type="button"
                      onClick={() => {
                        handleEditPreset(preset.id);
                      }}
                      className={cn(
                        'flex items-center justify-center p-half rounded border text-low',
                        'hover:text-normal hover:border-brand'
                      )}
                      aria-label={t('step5.edit')}
                      title={t('step5.edit')}
                    >
                      <PencilSimpleIcon className="size-icon-sm" />
                    </button>

                    {/* Move Buttons */}
                    <div className="flex items-center gap-sm">
                      <button
                        type="button"
                        onClick={() => {
                          moveUp(index);
                        }}
                        disabled={index === 0}
                        className={cn(
                          'flex items-center justify-center p-half rounded border text-low',
                          'hover:text-normal hover:border-brand disabled:opacity-50 disabled:cursor-not-allowed'
                        )}
                        aria-label={t('step5.moveUp')}
                      >
                        <CaretUpIcon className="size-icon-sm" />
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          moveDown(index);
                        }}
                        disabled={index === selectedPresets.length - 1}
                        className={cn(
                          'flex items-center justify-center p-half rounded border text-low',
                          'hover:text-normal hover:border-brand disabled:opacity-50 disabled:cursor-not-allowed'
                        )}
                        aria-label={t('step5.moveDown')}
                      >
                        <CaretDownIcon className="size-icon-sm" />
                      </button>
                    </div>

                    {/* Remove Button */}
                    <button
                      type="button"
                      onClick={() => {
                        removePreset(preset.id);
                      }}
                      className={cn(
                        'flex items-center justify-center p-half rounded border text-low',
                        'hover:text-error hover:border-error'
                      )}
                      aria-label={t('step5.remove')}
                    >
                      <XIcon className="size-icon-sm" />
                    </button>
                  </div>
                ))}
              </div>
            )}

            {/* Action Buttons */}
            {selectedPresets.length > 0 && (
              <div className="mt-base flex gap-base">
                <button
                  type="button"
                  onClick={clearAll}
                  className={cn(
                    'px-base py-half rounded-sm border text-base text-low',
                    'hover:text-normal hover:border-brand'
                  )}
                >
                  {t('step5.clearAll')}
                </button>
                <button
                  type="button"
                  onClick={resetDefault}
                  className={cn(
                    'px-base py-half rounded-sm border text-base text-low',
                    'hover:text-normal hover:border-brand'
                  )}
                >
                  {t('step5.resetDefault')}
                </button>
              </div>
            )}
          </Field>

          {/* Available Presets */}
          <Field>
            <FieldLabel>{t('step5.availableTitle')}</FieldLabel>

            {/* System Presets */}
            <div className="mb-base">
              <div className="text-sm text-high mb-base">{t('step5.systemPresetsTitle')}</div>
              <div className="flex flex-col gap-sm">
                {SYSTEM_PRESETS.map((preset) => {
                  const isSelected = config.presetIds.includes(preset.id);
                  return (
                    <div
                      key={preset.id}
                      className={cn(
                        'flex items-center gap-base bg-secondary rounded-sm border px-base py-half',
                        isSelected && 'border-brand bg-brand/5'
                      )}
                    >
                      <div className="flex-1 min-w-0">
                        <div className="text-base text-high truncate font-ibm-plex-mono">
                          {getPresetLabel(preset)}
                        </div>
                        <div className="text-xs text-low truncate">
                          {getPresetDescription(preset)}
                        </div>
                      </div>
                      <button
                        type="button"
                        onClick={() => {
                          addPreset(preset.id);
                        }}
                        disabled={isSelected}
                        className={cn(
                          'flex items-center justify-center p-half rounded border text-low',
                          'hover:text-normal hover:border-brand',
                          isSelected && 'opacity-50 cursor-not-allowed'
                        )}
                        aria-label={t('step5.add')}
                      >
                        <PlusIcon className="size-icon-sm" />
                      </button>
                    </div>
                  );
                })}
              </div>
            </div>

            {/* User Presets */}
            {userPresets.length > 0 && (
              <div>
                <div className="text-sm text-high mb-base">{t('step5.userPresetsTitle')}</div>
                <div className="flex flex-col gap-sm">
                  {userPresets.map((preset) => {
                    const isSelected = config.presetIds.includes(preset.id);
                    return (
                      <div
                        key={preset.id}
                        className={cn(
                          'flex items-center gap-base bg-secondary rounded-sm border px-base py-half',
                          isSelected && 'border-brand bg-brand/5'
                        )}
                      >
                        <div className="flex-1 min-w-0">
                          <div className="text-base text-high truncate font-ibm-plex-mono">
                            {getPresetLabel(preset)}
                          </div>
                          <div className="text-xs text-low truncate">
                            {getPresetDescription(preset)}
                          </div>
                        </div>
                        <button
                          type="button"
                          onClick={() => {
                            addPreset(preset.id);
                          }}
                          disabled={isSelected}
                          className={cn(
                            'flex items-center justify-center p-half rounded border text-low',
                            'hover:text-normal hover:border-brand',
                            isSelected && 'opacity-50 cursor-not-allowed'
                          )}
                          aria-label={t('step5.add')}
                        >
                          <PlusIcon className="size-icon-sm" />
                        </button>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

            {isLoading && (
              <div className="text-base text-low">
                {t('step5.loadingUserPresets')}
              </div>
            )}
          </Field>
        </>
      )}

      {/* Preset Editor Modal */}
      {editingPreset && (
        <PresetEditorModal
          presetLabel={getPresetLabel(editingPreset)}
          initialDescription={getPresetDescription(editingPreset)}
          initialCommands={config.additionalCommands?.[editingPreset.id] ?? []}
          initialParams={config.customParams?.[editingPreset.id] ?? null}
          onSave={(description, commands, params) => handleSavePreset(editingPreset.id, description, commands, params)}
          onCancel={handleCancelPreset}
        />
      )}
    </div>
  );
};
