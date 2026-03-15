import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Plus, Edit, Trash2 } from 'lucide-react';
import { Loader } from '@/components/ui/loader';
import {
  useSlashCommands,
  useCreateSlashCommand,
  useUpdateSlashCommand,
  useDeleteSlashCommand,
} from '@/hooks/useSlashCommands';
import type { SlashCommandPresetDto } from 'shared/types';
import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';
import { useTranslation } from 'react-i18next';

export function SlashCommands() {
  const { t } = useTranslation('slashCommands');
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [showEditDialog, setShowEditDialog] = useState(false);
  const [selectedCommand, setSelectedCommand] = useState<SlashCommandPresetDto | null>(null);

  const { data: commands = [], isLoading, error } = useSlashCommands();
  const createMutation = useCreateSlashCommand();
  const updateMutation = useUpdateSlashCommand();
  const deleteMutation = useDeleteSlashCommand();

  // Filter out system commands from the main list
  const userCommands = commands.filter(cmd => !cmd.isSystem);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader message={t('loading')} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Card className="max-w-md">
          <CardContent className="pt-6">
            <p className="text-error mb-4">{t('errors.loadFailed')}</p>
            <p className="text-sm text-low">{error.message}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  const handleDeleteCommand = async (command: SlashCommandPresetDto) => {
    const result = await ConfirmDialog.show({
      title: 'Delete Command',
      message: t('errors.deleteConfirm', { command: command.command }),
      confirmText: 'Delete',
      cancelText: 'Cancel',
      variant: 'destructive',
    });

    if (result === 'confirmed') {
      await deleteMutation.mutateAsync(command.id);
    }
  };

  const handleEditCommand = (command: SlashCommandPresetDto) => {
    setSelectedCommand(command);
    setShowEditDialog(true);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t('title')}</h1>
          <p className="text-low">
            {t('description')}
          </p>
        </div>
        <Button onClick={() => setShowCreateDialog(true)}>
          <Plus className="w-4 h-4 mr-2" />
          {t('createButton')}
        </Button>
      </div>

      {/* Create Dialog */}
      {showCreateDialog && (
        <SlashCommandFormDialog
          mode="create"
          onClose={() => setShowCreateDialog(false)}
          onSubmit={async (data) => {
            await createMutation.mutateAsync(data);
            setShowCreateDialog(false);
          }}
        />
      )}

      {/* Edit Dialog */}
      {showEditDialog && selectedCommand && (
        <SlashCommandFormDialog
          mode="edit"
          command={selectedCommand}
          onClose={() => {
            setShowEditDialog(false);
            setSelectedCommand(null);
          }}
          onSubmit={async (data) => {
            await updateMutation.mutateAsync({
              id: selectedCommand.id,
              data,
            });
            setShowEditDialog(false);
            setSelectedCommand(null);
          }}
        />
      )}

      {/* Commands List */}
      {!userCommands || userCommands.length === 0 ? (
        <Card className="p-12 text-center">
          <h3 className="text-lg font-semibold mb-2">{t('empty.title')}</h3>
          <p className="text-low mb-6">
            {t('empty.description')}
          </p>
          <Button onClick={() => setShowCreateDialog(true)}>
            <Plus className="w-4 h-4 mr-2" />
            {t('empty.button')}
          </Button>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {userCommands.map((command) => (
            <Card
              key={command.id}
              className="transition-all hover:shadow-lg border-2 hover:border-brand"
            >
              <CardContent className="pt-6">
                <div className="flex items-start justify-between mb-4">
                  <div>
                    <h3 className="font-semibold text-lg">{command.command}</h3>
                    {command.description && (
                      <p className="text-sm text-low mt-1">{command.description}</p>
                    )}
                  </div>
                  <div className="flex gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleEditCommand(command)}
                    >
                      <Edit className="w-4 h-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDeleteCommand(command)}
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                </div>
                <div className="text-xs text-low">
                  {t('list.createdAt', { date: new Date(command.createdAt).toLocaleDateString() })}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

// ============================================================================
// Form Dialog Component
// ============================================================================

interface SlashCommandFormDialogProps {
  mode: 'create' | 'edit';
  command?: SlashCommandPresetDto;
  onClose: () => void;
  onSubmit: (data: {
    command: string;
    description: string;
    promptTemplate: string;
  }) => Promise<void>;
}

function SlashCommandFormDialog({
  mode,
  command,
  onClose,
  onSubmit,
}: Readonly<SlashCommandFormDialogProps>) {
  const { t } = useTranslation('slashCommands');
  const [cmd, setCmd] = useState(command?.command || '');
  const [description, setDescription] = useState(command?.description || '');
  const [promptTemplate, setPromptTemplate] = useState(command?.promptTemplate || '');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const normalizedCommand = cmd.trim();
  const normalizedDescription = description.trim();
  const normalizedTemplate = promptTemplate.trim();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!normalizedCommand || !normalizedTemplate) {
      setError(t('errors.validation.commandRequired'));
      return;
    }

    if (!normalizedCommand.startsWith('/')) {
      setError(t('errors.validation.commandMustStartWithSlash'));
      return;
    }

    if (!normalizedDescription) {
      setError('Description is required');
      return;
    }

    try {
      setIsSubmitting(true);
      await onSubmit({
        command: normalizedCommand,
        description: normalizedDescription,
        promptTemplate: normalizedTemplate,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errors.createFailed'));
      setIsSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-background rounded-lg shadow-lg max-w-2xl w-full mx-4 max-h-[90vh] overflow-y-auto">
        <div className="p-6">
          <h2 className="text-xl font-bold mb-4">
            {mode === 'create' ? t('form.createTitle') : t('form.editTitle')}
          </h2>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium mb-2">
                {t('form.commandLabel')} *
              </label>
              <input
                type="text"
                value={cmd}
                onChange={(e) => setCmd(e.target.value)}
                placeholder={t('form.commandPlaceholder')}
                className="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-brand"
                disabled={isSubmitting}
              />
              <p className="text-xs text-low mt-1">
                {t('form.commandHint')}
              </p>
            </div>

            <div>
              <label className="block text-sm font-medium mb-2">
                {t('form.descriptionLabel')}
              </label>
              <input
                type="text"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder={t('form.descriptionPlaceholder')}
                className="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-brand"
                disabled={isSubmitting}
              />
            </div>

            <div>
              <label className="block text-sm font-medium mb-2">
                {t('form.templateLabel')} *
              </label>
              <textarea
                value={promptTemplate}
                onChange={(e) => setPromptTemplate(e.target.value)}
                placeholder={t('form.templatePlaceholder')}
                rows={8}
                className="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-brand font-mono text-sm"
                disabled={isSubmitting}
              />
              <p className="text-xs text-low mt-1">
                {t('form.templateHint')}
              </p>
            </div>

            {error && (
              <div className="p-3 bg-error/10 border border-error rounded-md">
                <p className="text-sm text-error">{error}</p>
              </div>
            )}

            <div className="flex justify-end gap-2 pt-4">
              <Button
                type="button"
                variant="outline"
                onClick={onClose}
                disabled={isSubmitting}
              >
                {t('form.buttons.cancel')}
              </Button>
              <Button type="submit" disabled={isSubmitting}>
                {(() => {
                  if (isSubmitting) {
                    return t('form.buttons.saving');
                  }
                  return mode === 'create' ? t('form.buttons.create') : t('form.buttons.save');
                })()}
              </Button>
            </div>
          </form>
        </div>
      </div>
    </div>
  );
}
