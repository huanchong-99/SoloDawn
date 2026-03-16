import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { cloneDeep, isEqual } from 'lodash';
import {
  CaretDown,
  SpinnerGap,
  Trash,
  Plus,
  ArrowsClockwise,
  DownloadSimple,
} from '@phosphor-icons/react';

import { cn } from '@/lib/utils';
import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { SettingsSection } from '@/components/ui-new/primitives/SettingsSection';
import { SettingsToggle } from '@/components/ui-new/primitives/SettingsToggle';
import { SettingsSaveBar } from '@/components/ui-new/primitives/SettingsSaveBar';
import { Label } from '@/components/ui-new/primitives/Label';
import { Button } from '@/components/ui-new/primitives/Button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui-new/primitives/Dropdown';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { JSONEditor } from '@/components/ui/json-editor';
import { ExecutorConfigForm } from '@/components/ExecutorConfigForm';
import { useProfiles } from '@/hooks/useProfiles';
import { useUserSystem } from '@/components/ConfigProvider';
import { CreateConfigurationDialog } from '@/components/dialogs/settings/CreateConfigurationDialog';
import { DeleteConfigurationDialog } from '@/components/dialogs/settings/DeleteConfigurationDialog';
import { useAgentAvailability } from '@/hooks/useAgentAvailability';
import { AgentAvailabilityIndicator } from '@/components/AgentAvailabilityIndicator';
import { configApi } from '@/lib/api';
import type {
  BaseCodingAgent,
  ExecutorConfigs,
  ExecutorProfileId,
} from 'shared/types';

type ExecutorsMap = Record<string, Record<string, Record<string, unknown>>>;

/* ------------------------------------------------------------------ */
/*  Inline select primitive (native <select> styled for new design)   */
/* ------------------------------------------------------------------ */
interface NativeSelectProps {
  id?: string;
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
  placeholder?: string;
  options: Array<{ value: string; label: string }>;
  className?: string;
}

function NativeSelect({
  id,
  value,
  onChange,
  disabled,
  placeholder,
  options,
  className,
}: Readonly<NativeSelectProps>) {
  return (
    <div className={cn('relative', className)}>
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        className={cn(
          'w-full appearance-none rounded border border-border bg-secondary px-base py-1 pr-7 text-base text-normal',
          'focus:outline-none focus:ring-1 focus:ring-brand',
          disabled && 'opacity-60 cursor-not-allowed'
        )}
      >
        {placeholder && (
          <option value="" disabled>
            {placeholder}
          </option>
        )}
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
      <CaretDown
        className="size-icon-xs absolute right-1.5 top-1/2 -translate-y-1/2 text-low pointer-events-none"
        weight="bold"
      />
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Success / info alert for new design                               */
/* ------------------------------------------------------------------ */
function SuccessAlert({ message }: Readonly<{ message: string }>) {
  return (
    <output
      className="relative w-full border border-success bg-success/10 p-base text-sm text-success block"
    >
      {message}
    </output>
  );
}

function InfoAlert({
  children,
  className,
}: Readonly<{
  children: React.ReactNode;
  className?: string;
}>) {
  return (
    <div
      className={cn(
        'relative w-full border border-border bg-secondary p-base text-sm text-normal',
        className
      )}
    >
      {children}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Spinner icon (Phosphor-based)                                     */
/* ------------------------------------------------------------------ */
function Spinner({ className }: Readonly<{ className?: string }>) {
  return (
    <SpinnerGap
      className={cn('animate-spin', className)}
      weight="bold"
    />
  );
}

/* ------------------------------------------------------------------ */
/*  Pure helpers (outside component to reduce cognitive complexity)    */
/* ------------------------------------------------------------------ */

function parseInstalledCliNames(output: string): string[] {
  const names = new Set<string>();
  const lines = output.split('\n');

  for (const rawLine of lines) {
    const line = rawLine.trim();
    const okIndex = line.indexOf('OK ');
    if (okIndex < 0) {
      continue;
    }

    const nameStart = okIndex + 3;
    const colonIndex = line.indexOf(':', nameStart);
    if (colonIndex < 0) {
      continue;
    }

    const name = line.slice(nameStart, colonIndex).trim();
    if (name.length > 0) {
      names.add(name);
    }
  }

  return Array.from(names);
}

function getProfilesErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }

  if (typeof error === 'object' && error !== null) {
    const message = Reflect.get(error, 'message');
    if (typeof message === 'string' && message.trim()) {
      return message;
    }

    try {
      return JSON.stringify(error);
    } catch {
      return '[Unserializable error object]';
    }
  }

  return String(error);
}

/* ================================================================== */
/*  Main component                                                    */
/* ================================================================== */

export function AgentSettingsNew() {
  const { t } = useTranslation(['settings', 'common', 'workflow']);

  // ---- Server profiles state via hook ----
  const {
    profilesContent: serverProfilesContent,
    profilesPath,
    isLoading: profilesLoading,
    isSaving: profilesSaving,
    error: profilesError,
    save: saveProfiles,
  } = useProfiles();

  const { config, updateAndSaveConfig, profiles, reloadSystem } =
    useUserSystem();

  // ---- Local editor state (draft) ----
  const [localProfilesContent, setLocalProfilesContent] = useState('');
  const [profilesSuccess, setProfilesSuccess] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  // ---- Form-based editor state ----
  const [useFormEditor, setUseFormEditor] = useState(true);
  const [selectedExecutorType, setSelectedExecutorType] =
    useState<BaseCodingAgent>('CLAUDE_CODE' as BaseCodingAgent);
  const [selectedConfiguration, setSelectedConfiguration] =
    useState<string>('DEFAULT');
  const [localParsedProfiles, setLocalParsedProfiles] =
    useState<ExecutorConfigs | null>(null);
  const [isDirty, setIsDirty] = useState(false);

  // ---- Default executor profile state ----
  const [executorDraft, setExecutorDraft] = useState<ExecutorProfileId | null>(
    () => (config?.executor_profile ? cloneDeep(config.executor_profile) : null)
  );
  const [executorSaving, setExecutorSaving] = useState(false);
  const [executorSuccess, setExecutorSuccess] = useState(false);
  const [executorError, setExecutorError] = useState<string | null>(null);

  // ---- Agent availability ----
  const [availabilityRefreshToken, setAvailabilityRefreshToken] = useState(0);
  const agentAvailability = useAgentAvailability(
    executorDraft?.executor,
    {},
    availabilityRefreshToken
  );

  // ---- CLI installer state ----
  const [installingCli, setInstallingCli] = useState(false);
  const [installCliResult, setInstallCliResult] = useState<{
    installed: boolean;
    output: string;
    exitCode: number;
  } | null>(null);
  const [installElapsedSec, setInstallElapsedSec] = useState(0);

  // Elapsed-seconds timer for CLI install
  useEffect(() => {
    if (!installingCli) {
      setInstallElapsedSec(0);
      return;
    }

    setInstallElapsedSec(0);
    const startedAt = Date.now();
    const timer = globalThis.setInterval(() => {
      setInstallElapsedSec(Math.floor((Date.now() - startedAt) / 1000));
    }, 1000);

    return () => globalThis.clearInterval(timer);
  }, [installingCli]);

  // Sync server state to local state when not dirty
  useEffect(() => {
    if (!isDirty && serverProfilesContent) {
      setLocalProfilesContent(serverProfilesContent);
      try {
        const parsed = JSON.parse(serverProfilesContent);
        setLocalParsedProfiles(parsed);
      } catch (err) {
        console.error('Failed to parse profiles JSON:', err);
        setLocalParsedProfiles(null);
      }
    }
  }, [serverProfilesContent, isDirty]);

  // Check if executor draft differs from saved config
  const executorDirty =
    executorDraft && config?.executor_profile
      ? !isEqual(executorDraft, config.executor_profile)
      : false;

  // Sync executor draft when config changes (only if not dirty)
  useEffect(() => {
    if (config?.executor_profile) {
      setExecutorDraft((currentDraft) => {
        if (!currentDraft || isEqual(currentDraft, config.executor_profile)) {
          return cloneDeep(config.executor_profile);
        }
        return currentDraft;
      });
    }
  }, [config?.executor_profile]);

  // ---- Handlers ----

  const updateExecutorDraft = (newProfile: ExecutorProfileId) => {
    setExecutorDraft(newProfile);
  };

  const handleSaveExecutorProfile = async () => {
    if (!executorDraft || !config) return;

    setExecutorSaving(true);
    setExecutorError(null);

    try {
      await updateAndSaveConfig({ executor_profile: executorDraft });
      setExecutorSuccess(true);
      setTimeout(() => setExecutorSuccess(false), 3000);
      reloadSystem();
    } catch (err) {
      setExecutorError(t('settings.general.save.error'));
      console.error('Error saving executor profile:', err);
    } finally {
      setExecutorSaving(false);
    }
  };

  const handleInstallAiClis = async () => {
    setInstallingCli(true);
    setInstallCliResult(null);
    try {
      const result = await configApi.installAiClis();
      setInstallCliResult({
        installed: result.installed,
        output: result.output,
        exitCode: result.exit_code,
      });
      setAvailabilityRefreshToken((token) => token + 1);
    } catch (err) {
      setInstallCliResult({
        installed: false,
        output: err instanceof Error ? err.message : 'Install failed',
        exitCode: -1,
      });
    } finally {
      setInstallingCli(false);
    }
  };

  const getInstallPhaseText = (elapsedSec: number) => {
    if (elapsedSec < 8) {
      return t('settings.agents.installAiCliPhasePreparing', {
        defaultValue: 'Preparing installation environment...',
      });
    }
    if (elapsedSec < 90) {
      return t('settings.agents.installAiCliPhaseCore', {
        defaultValue: 'Installing core CLIs (Claude/Codex/Gemini)...',
      });
    }
    if (elapsedSec < 180) {
      return t('settings.agents.installAiCliPhaseExtended', {
        defaultValue: 'Installing extended CLIs (Qwen/Amp/OpenCode/Kilo)...',
      });
    }
    return t('settings.agents.installAiCliPhaseVerifying', {
      defaultValue: 'Verifying installation results...',
    });
  };

  const syncRawProfiles = (profileData: unknown) => {
    setLocalProfilesContent(JSON.stringify(profileData, null, 2));
  };

  const markDirty = (nextProfiles: unknown) => {
    setLocalParsedProfiles(nextProfiles as ExecutorConfigs);
    syncRawProfiles(nextProfiles);
    setIsDirty(true);
  };

  // ---- Profile CRUD ----

  const openCreateDialog = async () => {
    try {
      const result = await CreateConfigurationDialog.show({
        executorType: selectedExecutorType,
        existingConfigs: Object.keys(
          localParsedProfiles?.executors?.[selectedExecutorType] || {}
        ),
      });

      if (result.action === 'created' && result.configName) {
        createConfiguration(
          selectedExecutorType,
          result.configName,
          result.cloneFrom
        );
      }
    } catch (error) {
      console.debug('User cancelled configuration creation', error);
    }
  };

  const createConfiguration = (
    executorType: string,
    configName: string,
    baseConfig?: string | null
  ) => {
    if (!localParsedProfiles?.executors) return;

    const executorsMap =
      localParsedProfiles.executors as unknown as ExecutorsMap;
    const base =
      executorsMap[executorType]?.[baseConfig ?? '']?.[executorType] ?? {};

    const updatedProfiles = {
      ...localParsedProfiles,
      executors: {
        ...localParsedProfiles.executors,
        [executorType]: {
          ...executorsMap[executorType],
          [configName]: {
            [executorType]: base,
          },
        },
      },
    };

    markDirty(updatedProfiles);
    setSelectedConfiguration(configName);
  };

  const openDeleteDialog = async (configName: string) => {
    try {
      const result = await DeleteConfigurationDialog.show({
        configName,
        executorType: selectedExecutorType,
      });

      if (result === 'deleted') {
        await handleDeleteConfiguration(configName);
      }
    } catch (error) {
      console.debug('User cancelled configuration creation', error);
    }
  };

  const handleDeleteConfiguration = async (configToDelete: string) => {
    if (!localParsedProfiles) {
      return;
    }

    setSaveError(null);

    try {
      if (
        !localParsedProfiles.executors[selectedExecutorType]?.[configToDelete]
      ) {
        return;
      }

      const currentConfigs = Object.keys(
        localParsedProfiles.executors[selectedExecutorType] || {}
      );
      if (currentConfigs.length <= 1) {
        return;
      }

      const remainingConfigs = {
        ...localParsedProfiles.executors[selectedExecutorType],
      };
      delete remainingConfigs[configToDelete];

      const updatedProfiles = {
        ...localParsedProfiles,
        executors: {
          ...localParsedProfiles.executors,
          [selectedExecutorType]: remainingConfigs,
        },
      };

      const executorsMap = updatedProfiles.executors as unknown as ExecutorsMap;
      if (Object.keys(remainingConfigs).length === 0) {
        executorsMap[selectedExecutorType] = {
          DEFAULT: { [selectedExecutorType]: {} },
        };
      }

      try {
        await saveProfiles(JSON.stringify(updatedProfiles, null, 2));

        setLocalParsedProfiles(updatedProfiles);
        setLocalProfilesContent(JSON.stringify(updatedProfiles, null, 2));
        setIsDirty(false);

        const nextConfigs = Object.keys(
          executorsMap[selectedExecutorType] || {}
        );
        const nextSelected = nextConfigs[0] || 'DEFAULT';
        setSelectedConfiguration(nextSelected);

        setProfilesSuccess(true);
        setTimeout(() => setProfilesSuccess(false), 3000);

        reloadSystem();
      } catch (delSaveError: unknown) {
        console.error('Failed to save deletion to backend:', delSaveError);
        setSaveError(t('settings.agents.errors.deleteFailed'));
      }
    } catch (error) {
      console.error('Error deleting configuration:', error);
    }
  };

  const handleProfilesChange = (value: string) => {
    setLocalProfilesContent(value);
    setIsDirty(true);

    if (value.trim()) {
      try {
        const parsed = JSON.parse(value);
        setLocalParsedProfiles(parsed);
      } catch (err) {
        console.debug('Invalid JSON in profiles editor', err);
        setLocalParsedProfiles(null);
      }
    }
  };

  const handleSaveProfiles = async () => {
    setSaveError(null);

    try {
      const contentToSave =
        useFormEditor && localParsedProfiles
          ? JSON.stringify(localParsedProfiles, null, 2)
          : localProfilesContent;

      await saveProfiles(contentToSave);
      setProfilesSuccess(true);
      setIsDirty(false);
      setTimeout(() => setProfilesSuccess(false), 3000);

      if (useFormEditor && localParsedProfiles) {
        setLocalProfilesContent(contentToSave);
      }

      reloadSystem();
    } catch (err: unknown) {
      console.error('Failed to save profiles:', err);
      setSaveError(t('settings.agents.errors.saveFailed'));
    }
  };

  const handleExecutorConfigChange = (
    executorType: string,
    configuration: string,
    formData: unknown
  ) => {
    if (!localParsedProfiles?.executors) return;

    const executorsMap =
      localParsedProfiles.executors as unknown as ExecutorsMap;
    const updatedProfiles = {
      ...localParsedProfiles,
      executors: {
        ...localParsedProfiles.executors,
        [executorType]: {
          ...executorsMap[executorType],
          [configuration]: {
            [executorType]: formData,
          },
        },
      },
    };

    markDirty(updatedProfiles);
  };

  const handleExecutorConfigSave = async (formData: unknown) => {
    if (!localParsedProfiles?.executors) return;

    setSaveError(null);

    const updatedProfiles = {
      ...localParsedProfiles,
      executors: {
        ...localParsedProfiles.executors,
        [selectedExecutorType]: {
          ...localParsedProfiles.executors[selectedExecutorType],
          [selectedConfiguration]: {
            [selectedExecutorType]: formData,
          },
        },
      },
    };

    setLocalParsedProfiles(updatedProfiles);

    try {
      const contentToSave = JSON.stringify(updatedProfiles, null, 2);

      await saveProfiles(contentToSave);
      setProfilesSuccess(true);
      setIsDirty(false);
      setTimeout(() => setProfilesSuccess(false), 3000);

      setLocalProfilesContent(contentToSave);
      reloadSystem();
    } catch (err: unknown) {
      console.error('Failed to save profiles:', err);
      setSaveError(t('settings.agents.errors.saveConfigFailed'));
    }
  };

  const handleDiscardProfiles = () => {
    if (serverProfilesContent) {
      setLocalProfilesContent(serverProfilesContent);
      try {
        setLocalParsedProfiles(JSON.parse(serverProfilesContent));
      } catch {
        setLocalParsedProfiles(null);
      }
    }
    setIsDirty(false);
  };

  // ---- Loading state ----
  if (profilesLoading) {
    return (
      <div className="flex items-center justify-center py-double">
        <Spinner className="size-icon-lg text-low" />
        <span className="ml-base text-normal text-base">
          {t('settings.agents.loading')}
        </span>
      </div>
    );
  }

  const installedCliNames =
    installCliResult?.installed && installCliResult.output
      ? parseInstalledCliNames(installCliResult.output)
      : [];

  // ---- Variant dropdown helpers ----
  const currentProfileVariant = executorDraft;
  const selectedProfile =
    profiles?.[currentProfileVariant?.executor || ''];
  const hasVariants =
    !!selectedProfile && Object.keys(selectedProfile).length > 0;

  const variantDropdown = (() => {
    if (hasVariants) {
      return (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="w-full flex items-center justify-between rounded border border-border bg-secondary px-base py-1 text-base text-normal focus:outline-none focus:ring-1 focus:ring-brand"
            >
              <span className="truncate flex-1 text-left">
                {currentProfileVariant?.variant ||
                  t('settings.general.taskExecution.defaultLabel')}
              </span>
              <CaretDown
                className="size-icon-xs ml-1 shrink-0 text-low"
                weight="bold"
              />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            {Object.entries(selectedProfile).map(
              ([variantLabel]) => (
                <DropdownMenuItem
                  key={variantLabel}
                  onClick={() => {
                    const newProfile: ExecutorProfileId = {
                      executor: currentProfileVariant!.executor,
                      variant: variantLabel,
                    };
                    updateExecutorDraft(newProfile);
                  }}
                  className={
                    currentProfileVariant?.variant === variantLabel
                      ? 'bg-secondary'
                      : ''
                  }
                >
                  {variantLabel}
                </DropdownMenuItem>
              )
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      );
    }
    if (selectedProfile) {
      return (
        <button
          type="button"
          disabled
          className="w-full flex items-center justify-between rounded border border-border bg-secondary px-base py-1 text-base text-low opacity-60 cursor-not-allowed"
        >
          <span className="truncate flex-1 text-left">
            {t('settings.general.taskExecution.defaultLabel')}
          </span>
        </button>
      );
    }
    return null;
  })();

  return (
    <div className="space-y-base">
      {/* ---- Global alerts ---- */}
      {!!profilesError && (
        <ErrorAlert message={getProfilesErrorMessage(profilesError)} />
      )}

      {profilesSuccess && (
        <SuccessAlert message={t('settings.agents.save.success')} />
      )}

      {saveError && <ErrorAlert message={saveError} />}

      {executorError && <ErrorAlert message={executorError} />}

      {executorSuccess && (
        <SuccessAlert message={t('settings.general.save.success')} />
      )}

      {/* ================================================================ */}
      {/*  Card 1: Task Execution (default executor + availability)        */}
      {/* ================================================================ */}
      <SettingsCard
        title={t('settings.general.taskExecution.title')}
        description={t('settings.general.taskExecution.description')}
      >
        <SettingsSection>
          {/* Executor + Variant selectors */}
          <div className="space-y-half">
            <Label className="text-normal text-base">
              {t('settings.general.taskExecution.executor.label')}
            </Label>
            <div className="grid grid-cols-2 gap-half">
              <NativeSelect
                id="executor"
                value={executorDraft?.executor ?? ''}
                onChange={(value) => {
                  const variants = profiles?.[value];
                  const keepCurrentVariant =
                    variants &&
                    executorDraft?.variant &&
                    variants[executorDraft.variant];

                  const newProfile: ExecutorProfileId = {
                    executor: value as BaseCodingAgent,
                    variant: keepCurrentVariant
                      ? executorDraft!.variant
                      : null,
                  };
                  updateExecutorDraft(newProfile);
                }}
                disabled={!profiles}
                placeholder={t(
                  'settings.general.taskExecution.executor.placeholder'
                )}
                options={
                  profiles
                    ? Object.keys(profiles)
                        .sort((a, b) => a.localeCompare(b))
                        .map((key) => ({ value: key, label: key }))
                    : []
                }
              />

              {/* Variant dropdown */}
              {variantDropdown}
            </div>
          </div>

          {/* Availability indicator */}
          <AgentAvailabilityIndicator availability={agentAvailability} />

          {/* Install + Refresh buttons */}
          <div className="flex gap-half">
            <Button
              variant="outline"
              size="sm"
              onClick={handleInstallAiClis}
              disabled={installingCli}
            >
              {installingCli ? (
                <Spinner className="size-icon-xs" />
              ) : (
                <DownloadSimple className="size-icon-xs" weight="bold" />
              )}
              {t('settings.agents.installAiCli', {
                defaultValue: 'One-click Install AI CLIs',
              })}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() =>
                setAvailabilityRefreshToken((token) => token + 1)
              }
              disabled={installingCli}
            >
              <ArrowsClockwise className="size-icon-xs" weight="bold" />
              {t('settings.agents.refreshAvailability', {
                defaultValue: 'Refresh availability',
              })}
            </Button>
          </div>

          {/* CLI install progress */}
          {installingCli && (
            <InfoAlert>
              <p className="text-xs text-low">
                {getInstallPhaseText(installElapsedSec)}
              </p>
              <p className="text-xs text-low">
                {t('settings.agents.installAiCliInProgress', {
                  defaultValue:
                    'Installing AI CLIs... elapsed {{seconds}}s',
                  seconds: installElapsedSec,
                })}
              </p>
            </InfoAlert>
          )}

          {/* CLI install result */}
          {installCliResult && (
            <div
              className={cn(
                'relative w-full border p-base text-xs whitespace-pre-wrap break-words',
                installCliResult.installed
                  ? 'border-success bg-success/10 text-success'
                  : 'border-error bg-error/10 text-error'
              )}
            >
              {installCliResult.installed
                ? t('settings.agents.installAiCliSuccess', {
                    defaultValue:
                      'AI CLI installation finished successfully.',
                  })
                : t('settings.agents.installAiCliFailed', {
                    defaultValue: 'AI CLI installation failed.',
                  })}
              {installCliResult.installed &&
                installedCliNames.length > 0 && (
                  <>
                    {'\n'}
                    {t('settings.agents.installAiCliInstalledList', {
                      defaultValue:
                        'Installed successfully: {{names}}',
                      names: installedCliNames.join(', '),
                    })}
                  </>
                )}
              {'\n'}
              {t('settings.agents.installAiCliExitCode', {
                defaultValue: 'Exit code: {{code}}',
                code: installCliResult.exitCode,
              })}
              {'\n'}
              {installCliResult.output}
            </div>
          )}

          {/* Helper text */}
          <p className="text-sm text-low">
            {t('settings.general.taskExecution.executor.helper')}
          </p>

          {/* Save button */}
          <div className="flex justify-end">
            <Button
              variant="primary"
              size="sm"
              onClick={handleSaveExecutorProfile}
              disabled={!executorDirty || executorSaving}
            >
              {executorSaving && (
                <Spinner className="size-icon-xs" />
              )}
              {t('common:buttons.save')}
            </Button>
          </div>
        </SettingsSection>
      </SettingsCard>

      {/* ================================================================ */}
      {/*  Card 2: Agent Profiles (form editor / JSON editor)              */}
      {/* ================================================================ */}
      <SettingsCard
        title={t('settings.agents.title')}
        description={t('settings.agents.description')}
      >
        <SettingsSection>
          {/* Editor mode toggle */}
          <SettingsToggle
            label={t('settings.agents.editor.formLabel')}
            description={undefined}
            checked={!useFormEditor}
            onChange={(checked) => setUseFormEditor(!checked)}
            disabled={profilesLoading || !localParsedProfiles}
          />

          {useFormEditor && localParsedProfiles?.executors ? (
            /* ---- Form-based editor ---- */
            <div className="space-y-base">
              <div className="grid grid-cols-2 gap-base">
                {/* Agent type selector */}
                <div className="space-y-half">
                  <Label
                    htmlFor="executor-type"
                    className="text-normal text-base"
                  >
                    {t('settings.agents.editor.agentLabel')}
                  </Label>
                  <NativeSelect
                    id="executor-type"
                    value={selectedExecutorType}
                    onChange={(value) => {
                      setSelectedExecutorType(value as BaseCodingAgent);
                      setSelectedConfiguration('DEFAULT');
                    }}
                    placeholder={t(
                      'settings.agents.editor.agentPlaceholder'
                    )}
                    options={Object.keys(
                      localParsedProfiles.executors
                    ).map((type) => ({ value: type, label: type }))}
                  />
                </div>

                {/* Configuration selector + create/delete */}
                <div className="space-y-half">
                  <Label
                    htmlFor="configuration"
                    className="text-normal text-base"
                  >
                    {t('settings.agents.editor.configLabel')}
                  </Label>
                  <div className="flex gap-half">
                    <NativeSelect
                      id="configuration"
                      value={selectedConfiguration}
                      onChange={(value) => {
                        if (value === '__create__') {
                          openCreateDialog();
                        } else {
                          setSelectedConfiguration(value);
                        }
                      }}
                      disabled={
                        !localParsedProfiles.executors[
                          selectedExecutorType
                        ]
                      }
                      placeholder={t(
                        'settings.agents.editor.configPlaceholder'
                      )}
                      options={[
                        ...Object.keys(
                          localParsedProfiles.executors[
                            selectedExecutorType
                          ] || {}
                        ).map((cfg) => ({
                          value: cfg,
                          label: cfg,
                        })),
                        {
                          value: '__create__',
                          label: t(
                            'settings.agents.editor.createNew'
                          ),
                        },
                      ]}
                      className="flex-1"
                    />
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={openCreateDialog}
                      title={t('settings.agents.editor.createNew')}
                    >
                      <Plus
                        className="size-icon-xs"
                        weight="bold"
                      />
                    </Button>
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() =>
                        openDeleteDialog(selectedConfiguration)
                      }
                      disabled={
                        profilesSaving ||
                        !localParsedProfiles.executors[
                          selectedExecutorType
                        ] ||
                        Object.keys(
                          localParsedProfiles.executors[
                            selectedExecutorType
                          ] || {}
                        ).length <= 1
                      }
                      title={
                        Object.keys(
                          localParsedProfiles.executors[
                            selectedExecutorType
                          ] || {}
                        ).length <= 1
                          ? t(
                              'settings.agents.editor.deleteTitle'
                            )
                          : t(
                              'settings.agents.editor.deleteButton',
                              {
                                name: selectedConfiguration,
                              }
                            )
                      }
                    >
                      <Trash
                        className="size-icon-xs"
                        weight="bold"
                      />
                      {t('settings.agents.editor.deleteText')}
                    </Button>
                  </div>
                </div>
              </div>

              {/* Executor config form */}
              {(() => {
                const executorsMap =
                  localParsedProfiles.executors as unknown as ExecutorsMap;
                return (
                  !!executorsMap[selectedExecutorType]?.[
                    selectedConfiguration
                  ]?.[selectedExecutorType] && (
                    <ExecutorConfigForm
                      key={`${selectedExecutorType}-${selectedConfiguration}`}
                      executor={selectedExecutorType}
                      value={
                        (executorsMap[selectedExecutorType][
                          selectedConfiguration
                        ][
                          selectedExecutorType
                        ] as Record<string, unknown>) || {}
                      }
                      onChange={(formData) =>
                        handleExecutorConfigChange(
                          selectedExecutorType,
                          selectedConfiguration,
                          formData
                        )
                      }
                      onSave={handleExecutorConfigSave}
                      disabled={profilesSaving}
                      isSaving={profilesSaving}
                      isDirty={isDirty}
                    />
                  )
                );
              })()}
            </div>
          ) : (
            /* ---- Raw JSON editor ---- */
            <div className="space-y-base">
              <div className="space-y-half">
                <Label
                  htmlFor="profiles-editor"
                  className="text-normal text-base"
                >
                  {t('settings.agents.editor.jsonLabel')}
                </Label>
                <JSONEditor
                  id="profiles-editor"
                  placeholder={t(
                    'settings.agents.editor.jsonPlaceholder'
                  )}
                  value={
                    profilesLoading
                      ? t('settings.agents.editor.jsonLoading')
                      : localProfilesContent
                  }
                  onChange={handleProfilesChange}
                  disabled={profilesLoading}
                  minHeight={300}
                />
              </div>

              {!profilesError && profilesPath && (
                <div className="space-y-half">
                  <p className="text-sm text-low">
                    <span className="font-medium text-normal">
                      {t('settings.agents.editor.pathLabel')}
                    </span>{' '}
                    <span className="font-ibm-plex-mono text-xs">
                      {profilesPath}
                    </span>
                  </p>
                </div>
              )}
            </div>
          )}
        </SettingsSection>
      </SettingsCard>

      {/* ---- Sticky save bar for JSON editor mode ---- */}
      {!useFormEditor && (
        <SettingsSaveBar
          visible={isDirty}
          onSave={handleSaveProfiles}
          onDiscard={handleDiscardProfiles}
          saving={profilesSaving}
        />
      )}
    </div>
  );
}
