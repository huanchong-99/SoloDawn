import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { cloneDeep, merge, isEqual } from 'lodash';
import { SpeakerHighIcon, ArrowCounterClockwiseIcon, SpinnerGapIcon } from '@phosphor-icons/react';
import {
  DEFAULT_PR_DESCRIPTION_PROMPT,
  EditorType,
  SoundFile,
  ThemeMode,
  UiLanguage,
} from 'shared/types';
import { getLanguageOptions } from '@/i18n/languages';

import { toPrettyCase } from '@/utils/string';
import { useEditorAvailability } from '@/hooks/useEditorAvailability';
import { EditorAvailabilityIndicator } from '@/components/EditorAvailabilityIndicator';
import { useTheme } from '@/components/ThemeProvider';
import { useUserSystem } from '@/components/ConfigProvider';
import { TagManager } from '@/components/TagManager';
import { cn } from '@/lib/utils';

import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { SettingsToggle } from '@/components/ui-new/primitives/SettingsToggle';
import { SettingsSelect } from '@/components/ui-new/primitives/SettingsSelect';
import { SettingsInput } from '@/components/ui-new/primitives/SettingsInput';
import { SettingsSaveBar } from '@/components/ui-new/primitives/SettingsSaveBar';
import { SettingsSection } from '@/components/ui-new/primitives/SettingsSection';

export function GeneralSettingsNew() {
  const { t } = useTranslation(['settings', 'common']);

  // Get language options with proper display names
  const languageOptions = getLanguageOptions(
    t('language.browserDefault', {
      ns: 'common',
      defaultValue: 'Browser Default',
    })
  );
  const {
    config,
    loading,
    updateAndSaveConfig,
  } = useUserSystem();

  // Draft state management
  const [draft, setDraft] = useState(() => (config ? cloneDeep(config) : null));
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [branchPrefixError, setBranchPrefixError] = useState<string | null>(
    null
  );
  const { setTheme } = useTheme();

  // Check editor availability when draft editor changes
  const editorAvailability = useEditorAvailability(draft?.editor.editor_type);

  const validateBranchPrefix = useCallback(
    (prefix: string): string | null => {
      if (!prefix) return null; // empty allowed
      if (prefix.includes('/'))
        return t('settings.general.git.branchPrefix.errors.slash');
      if (prefix.startsWith('.'))
        return t('settings.general.git.branchPrefix.errors.startsWithDot');
      if (prefix.endsWith('.') || prefix.endsWith('.lock'))
        return t('settings.general.git.branchPrefix.errors.endsWithDot');
      if (prefix.includes('..') || prefix.includes('@{'))
        return t('settings.general.git.branchPrefix.errors.invalidSequence');
      if (/[ \t~^:?*[\\]/.test(prefix))
        return t('settings.general.git.branchPrefix.errors.invalidChars');
      // Control chars check
      for (let i = 0; i < prefix.length; i++) {
        const code = prefix.codePointAt(i)!;
        if (code < 0x20 || code === 0x7f)
          return t('settings.general.git.branchPrefix.errors.controlChars');
      }
      return null;
    },
    [t]
  );

  // When config loads or changes externally, update draft only if not dirty
  useEffect(() => {
    if (!config) return;
    if (!dirty) {
      setDraft(cloneDeep(config));
    }
  }, [config, dirty]);

  // Check for unsaved changes
  const hasUnsavedChanges = useMemo(() => {
    if (!draft || !config) return false;
    return !isEqual(draft, config);
  }, [draft, config]);

  // Generic draft update helper
  const updateDraft = useCallback(
    (patch: Partial<typeof config>) => {
      setDraft((prev: typeof config) => {
        if (!prev) return prev;
        const next = merge({}, prev, patch);
        // Mark dirty if changed
        if (!isEqual(next, config)) {
          setDirty(true);
        }
        return next;
      });
    },
    [config]
  );

  // Optional: warn on tab close/navigation with unsaved changes
  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (hasUnsavedChanges) {
        e.preventDefault();
      }
    };
    globalThis.addEventListener('beforeunload', handler);
    return () => globalThis.removeEventListener('beforeunload', handler);
  }, [hasUnsavedChanges]);

  const playSound = async (soundFile: SoundFile) => {
    const audio = new Audio(`/api/sounds/${soundFile}`);
    try {
      await audio.play();
    } catch (err) {
      console.error('Failed to play sound:', err);
    }
  };

  const handleSave = async () => {
    if (!draft) return;

    setSaving(true);
    setError(null);
    setSuccess(false);

    try {
      await updateAndSaveConfig(draft);
      setTheme(draft.theme);
      setDirty(false);
      setSuccess(true);
      setTimeout(() => setSuccess(false), 3000);
    } catch (err) {
      setError(t('settings.general.save.error'));
      console.error('Error saving config:', err);
    } finally {
      setSaving(false);
    }
  };

  const handleDiscard = () => {
    if (!config) return;
    setDraft(cloneDeep(config));
    setDirty(false);
  };

  const handleResetDisclaimer = async () => {
    if (!config) return;
    try {
      await updateAndSaveConfig({ disclaimer_acknowledged: false });
    } catch (err) {
      console.error('Failed to reset disclaimer:', err);
    }
  };

  const handleResetOnboarding = async () => {
    if (!config) return;
    try {
      await updateAndSaveConfig({ onboarding_acknowledged: false });
    } catch (err) {
      console.error('Failed to reset onboarding:', err);
    }
  };

  // Theme options for SettingsSelect
  const themeOptions = useMemo(
    () =>
      Object.values(ThemeMode).map((theme) => ({
        value: theme,
        label: toPrettyCase(theme),
      })),
    []
  );

  // Language options for SettingsSelect
  const languageSelectOptions = useMemo(
    () =>
      languageOptions.map((opt) => ({
        value: opt.value,
        label: opt.label,
      })),
    [languageOptions]
  );

  // Editor options for SettingsSelect
  const editorOptions = useMemo(
    () =>
      Object.values(EditorType).map((editor) => ({
        value: editor,
        label: toPrettyCase(editor),
      })),
    []
  );

  // Sound file options for SettingsSelect
  const soundFileOptions = useMemo(
    () =>
      Object.values(SoundFile).map((sf) => ({
        value: sf,
        label: toPrettyCase(sf),
      })),
    []
  );

  // Editors that support remote SSH
  const supportsRemoteSsh =
    draft?.editor.editor_type === EditorType.VS_CODE ||
    draft?.editor.editor_type === EditorType.CURSOR ||
    draft?.editor.editor_type === EditorType.WINDSURF ||
    draft?.editor.editor_type === EditorType.GOOGLE_ANTIGRAVITY ||
    draft?.editor.editor_type === EditorType.ZED;

  if (loading) {
    return (
      <div className="flex items-center justify-center py-double">
        <SpinnerGapIcon className="size-icon-xl animate-spin text-low" weight="bold" />
        <span className="ml-base text-normal text-base">
          {t('settings.general.loading')}
        </span>
      </div>
    );
  }

  if (!config) {
    return (
      <div className="py-double">
        <div className="rounded border border-error bg-error/10 px-base py-base text-error text-sm">
          {t('settings.general.loadError')}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-base pb-16">
      {error && (
        <div className="rounded border border-error bg-error/10 px-base py-base text-error text-sm">
          {error}
        </div>
      )}

      {success && (
        <div className="rounded border border-success bg-success/10 px-base py-base text-success text-sm font-medium">
          {t('settings.general.save.success')}
        </div>
      )}

      {/* Appearance */}
      <SettingsCard
        title={t('settings.general.appearance.title')}
        description={t('settings.general.appearance.description')}
      >
        <SettingsSection>
          <SettingsSelect
            label={t('settings.general.appearance.theme.label')}
            description={t('settings.general.appearance.theme.helper')}
            value={draft?.theme ?? ThemeMode.SYSTEM}
            onChange={(value) => updateDraft({ theme: value as ThemeMode })}
            options={themeOptions}
            placeholder={t('settings.general.appearance.theme.placeholder')}
          />

          <SettingsSelect
            label={t('settings.general.appearance.language.label')}
            description={t('settings.general.appearance.language.helper')}
            value={draft?.language ?? 'BROWSER'}
            onChange={(value) => updateDraft({ language: value as UiLanguage })}
            options={languageSelectOptions}
            placeholder={t('settings.general.appearance.language.placeholder')}
          />
        </SettingsSection>
      </SettingsCard>

      {/* Editor */}
      <SettingsCard
        title={t('settings.general.editor.title')}
        description={t('settings.general.editor.description')}
      >
        <SettingsSection>
          <div className="space-y-base">
            <SettingsSelect
              label={t('settings.general.editor.type.label')}
              description={t('settings.general.editor.type.helper')}
              value={draft?.editor.editor_type ?? EditorType.VS_CODE}
              onChange={(value) =>
                updateDraft({
                  editor: { ...draft!.editor, editor_type: value as EditorType },
                })
              }
              options={editorOptions}
              placeholder={t('settings.general.editor.type.placeholder')}
            />

            {/* Editor availability status indicator */}
            {draft?.editor.editor_type !== EditorType.CUSTOM && (
              <div className="pl-0">
                <EditorAvailabilityIndicator availability={editorAvailability} />
              </div>
            )}
          </div>

          {draft?.editor.editor_type === EditorType.CUSTOM && (
            <SettingsInput
              label={t('settings.general.editor.customCommand.label')}
              description={t('settings.general.editor.customCommand.helper')}
              placeholder={t(
                'settings.general.editor.customCommand.placeholder'
              )}
              value={draft?.editor.custom_command || ''}
              onChange={(value) =>
                updateDraft({
                  editor: {
                    ...draft!.editor,
                    custom_command: value || null,
                  },
                })
              }
            />
          )}

          {supportsRemoteSsh && (
            <>
              <SettingsInput
                label={t('settings.general.editor.remoteSsh.host.label')}
                description={t('settings.general.editor.remoteSsh.host.helper')}
                placeholder={t(
                  'settings.general.editor.remoteSsh.host.placeholder'
                )}
                value={draft?.editor.remote_ssh_host || ''}
                onChange={(value) =>
                  updateDraft({
                    editor: {
                      ...draft!.editor,
                      remote_ssh_host: value || null,
                    },
                  })
                }
              />

              {draft?.editor.remote_ssh_host && (
                <SettingsInput
                  label={t('settings.general.editor.remoteSsh.user.label')}
                  description={t('settings.general.editor.remoteSsh.user.helper')}
                  placeholder={t(
                    'settings.general.editor.remoteSsh.user.placeholder'
                  )}
                  value={draft?.editor.remote_ssh_user || ''}
                  onChange={(value) =>
                    updateDraft({
                      editor: {
                        ...draft!.editor,
                        remote_ssh_user: value || null,
                      },
                    })
                  }
                />
              )}
            </>
          )}
        </SettingsSection>
      </SettingsCard>

      {/* Git */}
      <SettingsCard
        title={t('settings.general.git.title')}
        description={t('settings.general.git.description')}
      >
        <SettingsSection>
          <div className="space-y-base">
            <SettingsInput
              label={t('settings.general.git.branchPrefix.label')}
              description={t('settings.general.git.branchPrefix.helper')}
              placeholder={t('settings.general.git.branchPrefix.placeholder')}
              value={draft?.git_branch_prefix ?? ''}
              onChange={(value) => {
                const trimmed = value.trim();
                updateDraft({ git_branch_prefix: trimmed });
                setBranchPrefixError(validateBranchPrefix(trimmed));
              }}
              error={branchPrefixError ?? undefined}
            />
            <div className="text-low text-sm">
              {draft?.git_branch_prefix ? (
                <>
                  {t('settings.general.git.branchPrefix.preview')}{' '}
                  <code className="text-xs bg-secondary px-1 py-0.5 rounded font-ibm-plex-mono">
                    {t('settings.general.git.branchPrefix.previewWithPrefix', {
                      prefix: draft.git_branch_prefix,
                    })}
                  </code>
                </>
              ) : (
                <>
                  {t('settings.general.git.branchPrefix.preview')}{' '}
                  <code className="text-xs bg-secondary px-1 py-0.5 rounded font-ibm-plex-mono">
                    {t('settings.general.git.branchPrefix.previewNoPrefix')}
                  </code>
                </>
              )}
            </div>
          </div>
        </SettingsSection>
      </SettingsCard>

      {/* Pull Requests */}
      <SettingsCard
        title={t('settings.general.pullRequests.title')}
        description={t('settings.general.pullRequests.description')}
      >
        <SettingsSection>
          <SettingsToggle
            label={t('settings.general.pullRequests.autoDescription.label')}
            description={t('settings.general.pullRequests.autoDescription.helper')}
            checked={draft?.pr_auto_description_enabled ?? false}
            onChange={(checked) =>
              updateDraft({ pr_auto_description_enabled: checked })
            }
          />

          <SettingsToggle
            label={t('settings.general.pullRequests.customPrompt.useCustom')}
            checked={draft?.pr_auto_description_prompt != null}
            onChange={(checked) => {
              if (checked) {
                updateDraft({
                  pr_auto_description_prompt: DEFAULT_PR_DESCRIPTION_PROMPT,
                });
              } else {
                updateDraft({ pr_auto_description_prompt: null });
              }
            }}
          />

          <div className="space-y-1">
            <textarea
              id="pr-custom-prompt"
              className={cn(
                'flex min-h-[100px] w-full rounded border border-border bg-secondary px-base py-base text-base text-normal placeholder:text-low',
                'focus:outline-none focus:ring-1 focus:ring-brand',
                draft?.pr_auto_description_prompt == null &&
                  'opacity-50 cursor-not-allowed'
              )}
              value={
                draft?.pr_auto_description_prompt == null
                  ? ''
                  : draft.pr_auto_description_prompt
              }
              disabled={draft?.pr_auto_description_prompt == null}
              onChange={(e) =>
                updateDraft({
                  pr_auto_description_prompt: e.target.value,
                })
              }
            />
            <p className="text-low text-sm">
              {t('settings.general.pullRequests.customPrompt.helper')}
            </p>
          </div>
        </SettingsSection>
      </SettingsCard>

      {/* Notifications */}
      <SettingsCard
        title={t('settings.general.notifications.title')}
        description={t('settings.general.notifications.description')}
      >
        <SettingsSection>
          <SettingsToggle
            label={t('settings.general.notifications.sound.label')}
            description={t('settings.general.notifications.sound.helper')}
            checked={draft?.notifications.sound_enabled ?? false}
            onChange={(checked) =>
              updateDraft({
                notifications: {
                  ...draft!.notifications,
                  sound_enabled: checked,
                },
              })
            }
          />

          {draft?.notifications.sound_enabled && (
            <div className="ml-double space-y-base">
              <div className="flex items-start justify-between gap-double">
                <div className="flex-1 min-w-0">
                  <span className="text-normal text-base">
                    {t('settings.general.notifications.sound.fileLabel')}
                  </span>
                  <p className="text-low text-sm mt-0.5">
                    {t('settings.general.notifications.sound.fileHelper')}
                  </p>
                </div>
                <div className="flex items-center gap-half shrink-0">
                  <div className="relative">
                    <select
                      value={draft.notifications.sound_file}
                      onChange={(e) =>
                        updateDraft({
                          notifications: {
                            ...draft.notifications,
                            sound_file: e.target.value as SoundFile,
                          },
                        })
                      }
                      className="appearance-none rounded border border-border bg-secondary px-base py-1 pr-7 text-base text-normal focus:outline-none focus:ring-1 focus:ring-brand"
                    >
                      {soundFileOptions.map((opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                      ))}
                    </select>
                  </div>
                  <button
                    type="button"
                    onClick={() => playSound(draft.notifications.sound_file)}
                    className="flex items-center justify-center rounded border border-border bg-secondary px-half py-1 text-low hover:text-normal transition-colors duration-200"
                    aria-label="Play sound preview"
                  >
                    <SpeakerHighIcon className="size-icon-sm" weight="bold" />
                  </button>
                </div>
              </div>
            </div>
          )}

          <SettingsToggle
            label={t('settings.general.notifications.push.label')}
            description={t('settings.general.notifications.push.helper')}
            checked={draft?.notifications.push_enabled ?? false}
            onChange={(checked) =>
              updateDraft({
                notifications: {
                  ...draft!.notifications,
                  push_enabled: checked,
                },
              })
            }
          />
        </SettingsSection>
      </SettingsCard>

      {/* Privacy */}
      <SettingsCard
        title={t('settings.general.privacy.title')}
        description={t('settings.general.privacy.description')}
      >
        <SettingsSection>
          <SettingsToggle
            label={t('settings.general.privacy.telemetry.label')}
            description={t('settings.general.privacy.telemetry.helper')}
            checked={draft?.analytics_enabled ?? false}
            onChange={(checked) =>
              updateDraft({ analytics_enabled: checked })
            }
          />
        </SettingsSection>
      </SettingsCard>

      {/* Task Templates */}
      <SettingsCard
        title={t('settings.general.taskTemplates.title')}
        description={t('settings.general.taskTemplates.description')}
      >
        <TagManager />
      </SettingsCard>

      {/* Safety / Onboarding */}
      <SettingsCard
        title={t('settings.general.safety.title')}
        description={t('settings.general.safety.description')}
      >
        <SettingsSection>
          <div className="flex items-start justify-between gap-double">
            <div className="flex-1 min-w-0">
              <span className="text-normal text-base">
                {t('settings.general.safety.disclaimer.title')}
              </span>
              <p className="text-low text-sm mt-0.5">
                {t('settings.general.safety.disclaimer.description')}
              </p>
            </div>
            <button
              type="button"
              onClick={handleResetDisclaimer}
              className="inline-flex items-center gap-1.5 shrink-0 rounded border border-border bg-secondary px-base py-1 text-sm text-normal hover:bg-surface-2 transition-colors duration-200"
            >
              <ArrowCounterClockwiseIcon className="size-icon-xs" weight="bold" />
              {t('settings.general.safety.disclaimer.button')}
            </button>
          </div>

          <div className="flex items-start justify-between gap-double">
            <div className="flex-1 min-w-0">
              <span className="text-normal text-base">
                {t('settings.general.safety.onboarding.title')}
              </span>
              <p className="text-low text-sm mt-0.5">
                {t('settings.general.safety.onboarding.description')}
              </p>
            </div>
            <button
              type="button"
              onClick={handleResetOnboarding}
              className="inline-flex items-center gap-1.5 shrink-0 rounded border border-border bg-secondary px-base py-1 text-sm text-normal hover:bg-surface-2 transition-colors duration-200"
            >
              <ArrowCounterClockwiseIcon className="size-icon-xs" weight="bold" />
              {t('settings.general.safety.onboarding.button')}
            </button>
          </div>
        </SettingsSection>
      </SettingsCard>

      {/* Beta Features */}
      <SettingsCard
        title={t('settings.general.beta.title')}
        description={t('settings.general.beta.description')}
      >
        <SettingsSection>
          <SettingsToggle
            label={t('settings.general.beta.workspaces.label')}
            description={t('settings.general.beta.workspaces.helper')}
            checked={draft?.beta_workspaces ?? false}
            onChange={(checked) =>
              updateDraft({ beta_workspaces: checked })
            }
          />
        </SettingsSection>
      </SettingsCard>

      {/* Save Bar */}
      <SettingsSaveBar
        visible={hasUnsavedChanges}
        onSave={handleSave}
        onDiscard={handleDiscard}
        saving={saving || !!branchPrefixError}
      />
    </div>
  );
}
