import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  PlusIcon,
  CopyIcon,
  PencilSimpleIcon,
  TrashIcon,
  CaretDownIcon,
  CaretUpIcon,
  SpinnerGapIcon,
} from '@phosphor-icons/react';

import {
  designStylesApi,
  makeRequest,
  handleApiResponse,
  type DesignStyleResponse,
} from '@/lib/api';
import { cn } from '@/lib/utils';
import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { SettingsSection } from '@/components/ui-new/primitives/SettingsSection';
import { SettingsSelect } from '@/components/ui-new/primitives/SettingsSelect';
import { SettingsToggle } from '@/components/ui-new/primitives/SettingsToggle';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { PrimaryButton } from '@/components/ui-new/primitives/PrimaryButton';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui-new/primitives/Dialog';

async function fetchSystemSettings(): Promise<Record<string, string>> {
  const response = await makeRequest('/api/system-settings');
  return handleApiResponse<Record<string, string>>(response);
}

async function saveDefaultDesignStyle(slug: string): Promise<void> {
  const response = await makeRequest('/api/system-settings', {
    method: 'PUT',
    body: JSON.stringify({ defaultDesignStyle: slug }),
  });
  await handleApiResponse(response);
}

interface StyleEditorState {
  /** Style being edited, or null when creating a new one. */
  editing: DesignStyleResponse | null;
  name: string;
  description: string;
  content: string;
}

export function DesignStylesSettingsNew() {
  const { t } = useTranslation(['settings', 'common']);

  const [styles, setStyles] = useState<DesignStyleResponse[]>([]);
  const [defaultSlug, setDefaultSlug] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [savingDefault, setSavingDefault] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [editor, setEditor] = useState<StyleEditorState | null>(null);
  const [editorSaving, setEditorSaving] = useState(false);
  const [editorError, setEditorError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setLoading(true);
      const [list, settings] = await Promise.all([
        designStylesApi.list(),
        fetchSystemSettings(),
      ]);
      setStyles(list);
      setDefaultSlug(settings['default_design_style'] ?? '');
      setError(null);
    } catch {
      setError(t('settings.designStyles.loadError'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const enabledOptions = useMemo(() => {
    const options = styles
      .filter((s) => s.enabled)
      .map((s) => ({ value: s.slug, label: s.name }));
    return [
      { value: '', label: t('settings.designStyles.defaultNone') },
      ...options,
    ];
  }, [styles, t]);

  const handleDefaultChange = async (slug: string) => {
    const previous = defaultSlug;
    setDefaultSlug(slug);
    try {
      setSavingDefault(true);
      await saveDefaultDesignStyle(slug);
    } catch {
      setDefaultSlug(previous);
      setError(t('settings.designStyles.saveError'));
    } finally {
      setSavingDefault(false);
    }
  };

  const handleToggleEnabled = async (style: DesignStyleResponse, enabled: boolean) => {
    try {
      setBusyId(style.id);
      await designStylesApi.update(style.id, { enabled });
      await refresh();
    } catch {
      setError(t('settings.designStyles.saveError'));
    } finally {
      setBusyId(null);
    }
  };

  const handleDelete = async (style: DesignStyleResponse) => {
    try {
      setBusyId(style.id);
      await designStylesApi.remove(style.id);
      await refresh();
    } catch {
      setError(t('settings.designStyles.deleteError'));
    } finally {
      setBusyId(null);
    }
  };

  const handleDuplicate = async (style: DesignStyleResponse) => {
    try {
      setBusyId(style.id);
      await designStylesApi.create({
        name: t('settings.designStyles.copyName', { name: style.name }),
        description: style.description,
        content: style.content,
      });
      await refresh();
    } catch {
      setError(t('settings.designStyles.saveError'));
    } finally {
      setBusyId(null);
    }
  };

  const openCreate = () => {
    setEditorError(null);
    setEditor({ editing: null, name: '', description: '', content: '' });
  };

  const openEdit = (style: DesignStyleResponse) => {
    setEditorError(null);
    setEditor({
      editing: style,
      name: style.name,
      description: style.description,
      content: style.content,
    });
  };

  const handleEditorSave = async () => {
    if (!editor) return;
    if (!editor.name.trim() || !editor.content.trim()) {
      setEditorError(t('settings.designStyles.editor.requiredFields'));
      return;
    }
    try {
      setEditorSaving(true);
      setEditorError(null);
      if (editor.editing) {
        await designStylesApi.update(editor.editing.id, {
          name: editor.name.trim(),
          description: editor.description.trim(),
          content: editor.content,
        });
      } else {
        await designStylesApi.create({
          name: editor.name.trim(),
          description: editor.description.trim(),
          content: editor.content,
        });
      }
      setEditor(null);
      await refresh();
    } catch {
      setEditorError(t('settings.designStyles.saveError'));
    } finally {
      setEditorSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-double">
        <SpinnerGapIcon className="size-icon-sm animate-spin text-low" />
      </div>
    );
  }

  return (
    <div className="space-y-double pb-16">
      {error && <ErrorAlert message={error} />}

      <SettingsSection title={t('settings.designStyles.title')}>
        <SettingsCard
          title={t('settings.designStyles.defaultCard.title')}
          description={t('settings.designStyles.defaultCard.description')}
        >
          <SettingsSelect
            label={t('settings.designStyles.defaultCard.label')}
            description={savingDefault ? t('common:states.saving') : undefined}
            value={defaultSlug}
            onChange={handleDefaultChange}
            options={enabledOptions}
          />
        </SettingsCard>

        <SettingsCard
          title={t('settings.designStyles.listCard.title')}
          description={t('settings.designStyles.listCard.description')}
        >
          <div className="space-y-base">
            {styles.map((style) => (
              <div
                key={style.id}
                className="rounded border border-border bg-secondary p-base space-y-half"
              >
                <div className="flex items-start justify-between gap-base">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-half flex-wrap">
                      <span className="text-high text-base font-medium">
                        {style.name}
                      </span>
                      <span
                        className={cn(
                          'rounded-full border px-half text-xs uppercase tracking-wider',
                          style.builtin
                            ? 'text-brand border-brand/40'
                            : 'text-low border-border'
                        )}
                      >
                        {style.builtin
                          ? t('settings.designStyles.badge.builtin')
                          : t('settings.designStyles.badge.custom')}
                      </span>
                      {style.license && (
                        <span className="rounded-full border border-border px-half text-xs text-low">
                          {style.license}
                        </span>
                      )}
                    </div>
                    {style.description && (
                      <p className="text-low text-sm mt-0.5">{style.description}</p>
                    )}
                    {style.sourceName && style.sourceUrl && (
                      <a
                        href={style.sourceUrl}
                        target="_blank"
                        rel="noreferrer"
                        className="text-low text-xs underline hover:text-normal"
                      >
                        {t('settings.designStyles.sourceLabel', {
                          source: style.sourceName,
                        })}
                      </a>
                    )}
                  </div>
                  <SettingsToggle
                    label=""
                    checked={style.enabled}
                    disabled={busyId === style.id}
                    onChange={(checked) => handleToggleEnabled(style, checked)}
                    className="shrink-0 w-auto"
                  />
                </div>

                <div className="flex items-center gap-half">
                  <button
                    type="button"
                    onClick={() =>
                      setExpandedId(expandedId === style.id ? null : style.id)
                    }
                    className="inline-flex items-center gap-0.5 rounded border border-border bg-panel px-half py-0.5 text-xs text-low hover:text-normal"
                  >
                    {expandedId === style.id ? (
                      <CaretUpIcon className="size-icon-xs" />
                    ) : (
                      <CaretDownIcon className="size-icon-xs" />
                    )}
                    {t('settings.designStyles.actions.viewContent')}
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDuplicate(style)}
                    disabled={busyId === style.id}
                    className="inline-flex items-center gap-0.5 rounded border border-border bg-panel px-half py-0.5 text-xs text-low hover:text-normal disabled:opacity-60"
                  >
                    <CopyIcon className="size-icon-xs" />
                    {t('settings.designStyles.actions.duplicate')}
                  </button>
                  {!style.builtin && (
                    <>
                      <button
                        type="button"
                        onClick={() => openEdit(style)}
                        disabled={busyId === style.id}
                        className="inline-flex items-center gap-0.5 rounded border border-border bg-panel px-half py-0.5 text-xs text-low hover:text-normal disabled:opacity-60"
                      >
                        <PencilSimpleIcon className="size-icon-xs" />
                        {t('settings.designStyles.actions.edit')}
                      </button>
                      <button
                        type="button"
                        onClick={() => handleDelete(style)}
                        disabled={busyId === style.id}
                        className="inline-flex items-center gap-0.5 rounded border border-border bg-panel px-half py-0.5 text-xs text-error hover:bg-error/10 disabled:opacity-60"
                      >
                        <TrashIcon className="size-icon-xs" />
                        {t('settings.designStyles.actions.delete')}
                      </button>
                    </>
                  )}
                </div>

                {expandedId === style.id && (
                  <pre className="mt-half max-h-64 overflow-auto whitespace-pre-wrap rounded border border-border bg-primary p-base text-xs text-normal font-ibm-plex-mono">
                    {style.content}
                  </pre>
                )}
              </div>
            ))}
          </div>

          <div className="mt-base">
            <PrimaryButton
              actionIcon={PlusIcon}
              value={t('settings.designStyles.actions.create')}
              onClick={openCreate}
            />
          </div>
        </SettingsCard>
      </SettingsSection>

      <Dialog open={editor !== null} onOpenChange={(open) => !open && setEditor(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {editor?.editing
                ? t('settings.designStyles.editor.editTitle')
                : t('settings.designStyles.editor.createTitle')}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-base p-double pt-0">
            {editorError && <ErrorAlert message={editorError} />}
            <div className="space-y-half">
              <label className="text-normal text-sm" htmlFor="design-style-name">
                {t('settings.designStyles.editor.nameLabel')}
              </label>
              <input
                id="design-style-name"
                type="text"
                value={editor?.name ?? ''}
                onChange={(e) =>
                  setEditor((prev) => (prev ? { ...prev, name: e.target.value } : prev))
                }
                className="w-full rounded border border-border bg-secondary px-base py-1 text-base text-normal focus:outline-none focus:ring-1 focus:ring-brand"
              />
            </div>
            <div className="space-y-half">
              <label
                className="text-normal text-sm"
                htmlFor="design-style-description"
              >
                {t('settings.designStyles.editor.descriptionLabel')}
              </label>
              <input
                id="design-style-description"
                type="text"
                value={editor?.description ?? ''}
                onChange={(e) =>
                  setEditor((prev) =>
                    prev ? { ...prev, description: e.target.value } : prev
                  )
                }
                className="w-full rounded border border-border bg-secondary px-base py-1 text-base text-normal focus:outline-none focus:ring-1 focus:ring-brand"
              />
            </div>
            <div className="space-y-half">
              <label className="text-normal text-sm" htmlFor="design-style-content">
                {t('settings.designStyles.editor.contentLabel')}
              </label>
              <p className="text-low text-xs">
                {t('settings.designStyles.editor.contentHint')}
              </p>
              <textarea
                id="design-style-content"
                value={editor?.content ?? ''}
                onChange={(e) =>
                  setEditor((prev) =>
                    prev ? { ...prev, content: e.target.value } : prev
                  )
                }
                rows={12}
                className="w-full rounded border border-border bg-secondary px-base py-1 text-sm text-normal font-ibm-plex-mono focus:outline-none focus:ring-1 focus:ring-brand"
              />
            </div>
            <div className="flex justify-end gap-half">
              <PrimaryButton
                variant="tertiary"
                value={t('common:buttons.cancel')}
                onClick={() => setEditor(null)}
                disabled={editorSaving}
              />
              <PrimaryButton
                actionIcon={editorSaving ? 'spinner' : undefined}
                value={t('common:buttons.save')}
                onClick={handleEditorSave}
                disabled={editorSaving}
              />
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
