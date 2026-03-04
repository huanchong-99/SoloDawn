import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { cloneDeep, isEqual } from 'lodash';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Checkbox } from '@/components/ui/checkbox';
import { JSONEditor } from '@/components/ui/json-editor';
import { ChevronDown, Loader2 } from 'lucide-react';

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

export function AgentSettings() {
  const { t } = useTranslation(['settings', 'common', 'workflow']);
  // Use profiles hook for server state
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

  // Local editor state (draft that may differ from server)
  const [localProfilesContent, setLocalProfilesContent] = useState('');
  const [profilesSuccess, setProfilesSuccess] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  // Form-based editor state
  const [useFormEditor, setUseFormEditor] = useState(true);
  const [selectedExecutorType, setSelectedExecutorType] =
    useState<BaseCodingAgent>('CLAUDE_CODE' as BaseCodingAgent);
  const [selectedConfiguration, setSelectedConfiguration] =
    useState<string>('DEFAULT');
  const [localParsedProfiles, setLocalParsedProfiles] =
    useState<ExecutorConfigs | null>(null);
  const [isDirty, setIsDirty] = useState(false);

  // Default executor profile state
  const [executorDraft, setExecutorDraft] = useState<ExecutorProfileId | null>(
    () => (config?.executor_profile ? cloneDeep(config.executor_profile) : null)
  );
  const [executorSaving, setExecutorSaving] = useState(false);
  const [executorSuccess, setExecutorSuccess] = useState(false);
  const [executorError, setExecutorError] = useState<string | null>(null);

  // Check agent availability when draft executor changes
  const [availabilityRefreshToken, setAvailabilityRefreshToken] = useState(0);
  const agentAvailability = useAgentAvailability(
    executorDraft?.executor,
    {},
    availabilityRefreshToken
  );
  const [installingCli, setInstallingCli] = useState(false);
  const [installCliResult, setInstallCliResult] = useState<{
    installed: boolean;
    output: string;
    exitCode: number;
  } | null>(null);
  const [installElapsedSec, setInstallElapsedSec] = useState(0);

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
      // Parse JSON inside effect to avoid object dependency
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
        // Only update if draft matches the old config (not dirty)
        if (!currentDraft || isEqual(currentDraft, config.executor_profile)) {
          return cloneDeep(config.executor_profile);
        }
        return currentDraft;
      });
    }
  }, [config?.executor_profile]);

  // Update executor draft
  const updateExecutorDraft = (newProfile: ExecutorProfileId) => {
    setExecutorDraft(newProfile);
  };

  // Save executor profile
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

  const parseInstalledCliNames = (output: string): string[] => {
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
  };

  // Sync raw profiles with parsed profiles
  const syncRawProfiles = (profiles: unknown) => {
    setLocalProfilesContent(JSON.stringify(profiles, null, 2));
  };

  // Mark profiles as dirty
  const markDirty = (nextProfiles: unknown) => {
    setLocalParsedProfiles(nextProfiles as ExecutorConfigs);
    syncRawProfiles(nextProfiles);
    setIsDirty(true);
  };

  // Open create dialog
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

  // Create new configuration
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

  // Open delete dialog
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

  // Handle delete configuration
  const handleDeleteConfiguration = async (configToDelete: string) => {
    if (!localParsedProfiles) {
      return;
    }

    // Clear any previous errors
    setSaveError(null);

    try {
      // Validate that the configuration exists
      if (
        !localParsedProfiles.executors[selectedExecutorType]?.[configToDelete]
      ) {
        return;
      }

      // Check if this is the last configuration
      const currentConfigs = Object.keys(
        localParsedProfiles.executors[selectedExecutorType] || {}
      );
      if (currentConfigs.length <= 1) {
        return;
      }

      // Remove the configuration from the executor
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
      // If no configurations left, create a blank DEFAULT (should not happen due to check above)
      if (Object.keys(remainingConfigs).length === 0) {
        executorsMap[selectedExecutorType] = {
          DEFAULT: { [selectedExecutorType]: {} },
        };
      }

      try {
        // Save using hook
        await saveProfiles(JSON.stringify(updatedProfiles, null, 2));

        // Update local state and reset dirty flag
        setLocalParsedProfiles(updatedProfiles);
        setLocalProfilesContent(JSON.stringify(updatedProfiles, null, 2));
        setIsDirty(false);

        // Select the next available configuration
        const nextConfigs = Object.keys(
          executorsMap[selectedExecutorType] || {}
        );
        const nextSelected = nextConfigs[0] || 'DEFAULT';
        setSelectedConfiguration(nextSelected);

        // Show success
        setProfilesSuccess(true);
        setTimeout(() => setProfilesSuccess(false), 3000);

        // Refresh global system so deleted configs are removed elsewhere
        reloadSystem();
      } catch (saveError: unknown) {
        console.error('Failed to save deletion to backend:', saveError);
        setSaveError(t('settings.agents.errors.deleteFailed'));
      }
    } catch (error) {
      console.error('Error deleting configuration:', error);
    }
  };

  const handleProfilesChange = (value: string) => {
    setLocalProfilesContent(value);
    setIsDirty(true);

    // Validate JSON on change
    if (value.trim()) {
      try {
        const parsed = JSON.parse(value);
        setLocalParsedProfiles(parsed);
      } catch (err) {
        console.debug('Invalid JSON in profiles editor', err);
        // Invalid JSON, keep local content but clear parsed
        setLocalParsedProfiles(null);
      }
    }
  };

  const handleSaveProfiles = async () => {
    // Clear any previous errors
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

      // Update the local content if using form editor
      if (useFormEditor && localParsedProfiles) {
        setLocalProfilesContent(contentToSave);
      }

      // Refresh global system so new profiles are available elsewhere
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
    // Update the parsed profiles with the new config
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

    // Clear any previous errors
    setSaveError(null);

    // Update the parsed profiles with the saved config
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

    // Update state
    setLocalParsedProfiles(updatedProfiles);

    // Save the updated profiles directly
    try {
      const contentToSave = JSON.stringify(updatedProfiles, null, 2);

      await saveProfiles(contentToSave);
      setProfilesSuccess(true);
      setIsDirty(false);
      setTimeout(() => setProfilesSuccess(false), 3000);

      // Update the local content as well
      setLocalProfilesContent(contentToSave);

      // Refresh global system so new profiles are available elsewhere
      reloadSystem();
    } catch (err: unknown) {
      console.error('Failed to save profiles:', err);
      setSaveError(t('settings.agents.errors.saveConfigFailed'));
    }
  };

  const getProfilesErrorMessage = (error: unknown): string => {
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
  };

  if (profilesLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-8 w-8 animate-spin" />
        <span className="ml-2">{t('settings.agents.loading')}</span>
      </div>
    );
  }

  const installedCliNames =
    installCliResult?.installed && installCliResult.output
      ? parseInstalledCliNames(installCliResult.output)
      : [];

  return (
    <div className="space-y-6">
      {!!profilesError && (
        <Alert variant="destructive">
          <AlertDescription>
            {getProfilesErrorMessage(profilesError)}
          </AlertDescription>
        </Alert>
      )}

      {profilesSuccess && (
        <Alert variant="success">
          <AlertDescription className="font-medium">
            {t('settings.agents.save.success')}
          </AlertDescription>
        </Alert>
      )}

      {saveError && (
        <Alert variant="destructive">
          <AlertDescription>{saveError}</AlertDescription>
        </Alert>
      )}

      {executorError && (
        <Alert variant="destructive">
          <AlertDescription>{executorError}</AlertDescription>
        </Alert>
      )}

      {executorSuccess && (
        <Alert variant="success">
          <AlertDescription className="font-medium">
            {t('settings.general.save.success')}
          </AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle>{t('settings.general.taskExecution.title')}</CardTitle>
          <CardDescription>
            {t('settings.general.taskExecution.description')}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="executor">
              {t('settings.general.taskExecution.executor.label')}
            </Label>
            <div className="grid grid-cols-2 gap-2">
              <Select
                value={executorDraft?.executor ?? ''}
                onValueChange={(value: string) => {
                  const variants = profiles?.[value];
                  const keepCurrentVariant =
                    variants &&
                    executorDraft?.variant &&
                    variants[executorDraft.variant];

                  const newProfile: ExecutorProfileId = {
                    executor: value as BaseCodingAgent,
                    variant: keepCurrentVariant ? executorDraft!.variant : null,
                  };
                  updateExecutorDraft(newProfile);
                }}
                disabled={!profiles}
              >
                <SelectTrigger id="executor">
                  <SelectValue
                    placeholder={t(
                      'settings.general.taskExecution.executor.placeholder'
                    )}
                  />
                </SelectTrigger>
                <SelectContent>
                  {profiles &&
                    Object.entries(profiles)
                      .sort((a, b) => a[0].localeCompare(b[0]))
                      .map(([profileKey]) => (
                        <SelectItem key={profileKey} value={profileKey}>
                          {profileKey}
                        </SelectItem>
                      ))}
                </SelectContent>
              </Select>

              {/* Show variant selector if selected profile has variants */}
              {(() => {
                const currentProfileVariant = executorDraft;
                const selectedProfile =
                  profiles?.[currentProfileVariant?.executor || ''];
                const hasVariants =
                  !!selectedProfile && Object.keys(selectedProfile).length > 0;

                if (hasVariants) {
                  return (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="outline"
                          className="w-full h-10 px-2 flex items-center justify-between"
                        >
                          <span className="text-sm truncate flex-1 text-left">
                            {currentProfileVariant?.variant ||
                              t('settings.general.taskExecution.defaultLabel')}
                          </span>
                          <ChevronDown className="h-4 w-4 ml-1 flex-shrink-0" />
                        </Button>
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
                                  ? 'bg-accent'
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
                } else if (selectedProfile) {
                  // Show disabled button when profile exists but has no variants
                  return (
                    <Button
                      variant="outline"
                      className="w-full h-10 px-2 flex items-center justify-between"
                      disabled
                    >
                      <span className="text-sm truncate flex-1 text-left">
                        {t('settings.general.taskExecution.defaultLabel')}
                      </span>
                    </Button>
                  );
                }
                return null;
              })()}
            </div>
            <AgentAvailabilityIndicator availability={agentAvailability} />
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={handleInstallAiClis}
                disabled={installingCli}
              >
                {installingCli && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t('settings.agents.installAiCli', {
                  defaultValue: 'One-click Install AI CLIs',
                })}
              </Button>
              <Button
                type="button"
                variant="ghost"
                onClick={() => setAvailabilityRefreshToken((token) => token + 1)}
                disabled={installingCli}
              >
                {t('settings.agents.refreshAvailability', {
                  defaultValue: 'Refresh availability',
                })}
              </Button>
            </div>
            {installingCli && (
              <Alert>
                <AlertDescription className="text-xs">
                  {getInstallPhaseText(installElapsedSec)}
                  {'\n'}
                  {t('settings.agents.installAiCliInProgress', {
                    defaultValue: 'Installing AI CLIs... elapsed {{seconds}}s',
                    seconds: installElapsedSec,
                  })}
                </AlertDescription>
              </Alert>
            )}
            {installCliResult && (
              <Alert
                variant={installCliResult.installed ? 'default' : 'destructive'}
              >
                <AlertDescription className="whitespace-pre-wrap break-words text-xs">
                  {installCliResult.installed
                    ? t('settings.agents.installAiCliSuccess', {
                        defaultValue: 'AI CLI installation finished successfully.',
                      })
                    : t('settings.agents.installAiCliFailed', {
                        defaultValue: 'AI CLI installation failed.',
                      })}
                  {installCliResult.installed && installedCliNames.length > 0 && (
                    <>
                      {'\n'}
                      {t('settings.agents.installAiCliInstalledList', {
                        defaultValue: 'Installed successfully: {{names}}',
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
                </AlertDescription>
              </Alert>
            )}
            <p className="text-sm text-muted-foreground">
              {t('settings.general.taskExecution.executor.helper')}
            </p>
          </div>
          <div className="flex justify-end">
            <Button
              onClick={handleSaveExecutorProfile}
              disabled={!executorDirty || executorSaving}
            >
              {executorSaving && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              {t('common:buttons.save')}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('settings.agents.title')}</CardTitle>
          <CardDescription>{t('settings.agents.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Editor type toggle */}
          <div className="flex items-center space-x-2">
            <Checkbox
              id="use-form-editor"
              checked={!useFormEditor}
              onCheckedChange={(checked) => setUseFormEditor(!checked)}
              disabled={profilesLoading || !localParsedProfiles}
            />
            <Label htmlFor="use-form-editor">
              {t('settings.agents.editor.formLabel')}
            </Label>
          </div>

          {useFormEditor &&
          localParsedProfiles?.executors ? (
            // Form-based editor
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="executor-type">
                    {t('settings.agents.editor.agentLabel')}
                  </Label>
                  <Select
                    value={selectedExecutorType}
                    onValueChange={(value) => {
                      setSelectedExecutorType(value as BaseCodingAgent);
                      // Reset configuration selection when executor type changes
                      setSelectedConfiguration('DEFAULT');
                    }}
                  >
                    <SelectTrigger id="executor-type">
                      <SelectValue
                        placeholder={t(
                          'settings.agents.editor.agentPlaceholder'
                        )}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {Object.keys(localParsedProfiles.executors).map(
                        (type) => (
                          <SelectItem key={type} value={type}>
                            {type}
                          </SelectItem>
                        )
                      )}
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="configuration">
                    {t('settings.agents.editor.configLabel')}
                  </Label>
                  <div className="flex gap-2">
                    <Select
                      value={selectedConfiguration}
                      onValueChange={(value) => {
                        if (value === '__create__') {
                          openCreateDialog();
                        } else {
                          setSelectedConfiguration(value);
                        }
                      }}
                      disabled={
                        !localParsedProfiles.executors[selectedExecutorType]
                      }
                    >
                      <SelectTrigger id="configuration">
                        <SelectValue
                          placeholder={t(
                            'settings.agents.editor.configPlaceholder'
                          )}
                        />
                      </SelectTrigger>
                      <SelectContent>
                        {Object.keys(
                          localParsedProfiles.executors[selectedExecutorType] ||
                            {}
                        ).map((configuration) => (
                          <SelectItem key={configuration} value={configuration}>
                            {configuration}
                          </SelectItem>
                        ))}
                        <SelectItem value="__create__">
                          {t('settings.agents.editor.createNew')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                    <Button
                      variant="destructive"
                      size="sm"
                      className="h-10"
                      onClick={() => openDeleteDialog(selectedConfiguration)}
                      disabled={
                        profilesSaving ||
                        !localParsedProfiles.executors[selectedExecutorType] ||
                        Object.keys(
                          localParsedProfiles.executors[selectedExecutorType] ||
                            {}
                        ).length <= 1
                      }
                      title={
                        Object.keys(
                          localParsedProfiles.executors[selectedExecutorType] ||
                            {}
                        ).length <= 1
                          ? t('settings.agents.editor.deleteTitle')
                          : t('settings.agents.editor.deleteButton', {
                              name: selectedConfiguration,
                            })
                      }
                    >
                      {t('settings.agents.editor.deleteText')}
                    </Button>
                  </div>
                </div>
              </div>

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
                        ][selectedExecutorType] as Record<string, unknown>) ||
                        {}
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
            // Raw JSON editor
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="profiles-editor">
                  {t('settings.agents.editor.jsonLabel')}
                </Label>
                <JSONEditor
                  id="profiles-editor"
                  placeholder={t('settings.agents.editor.jsonPlaceholder')}
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
                <div className="space-y-2">
                  <p className="text-sm text-muted-foreground">
                    <span className="font-medium">
                      {t('settings.agents.editor.pathLabel')}
                    </span>{' '}
                    <span className="font-mono text-xs">{profilesPath}</span>
                  </p>
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      {!useFormEditor && (
        <div className="sticky bottom-0 z-10 bg-background/80 backdrop-blur-sm border-t py-4">
          <div className="flex justify-end">
            <Button
              onClick={handleSaveProfiles}
              disabled={!isDirty || profilesSaving || !!profilesError}
            >
              {profilesSaving && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              {t('settings.agents.save.button')}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
