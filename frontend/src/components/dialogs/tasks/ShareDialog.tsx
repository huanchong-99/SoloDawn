import { useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription } from '@/components/ui/alert';
import NiceModal, { useModal } from '@ebay/nice-modal-react';
import { defineModal } from '@/lib/modals';
import { OAuthDialog } from '@/components/dialogs/global/OAuthDialog';
import { LinkProjectDialog } from '@/components/dialogs/projects/LinkProjectDialog';
import { useTranslation } from 'react-i18next';
import { useUserSystem } from '@/components/ConfigProvider';
import { Link as LinkIcon, Loader2 } from 'lucide-react';
import type { TaskWithAttemptStatus } from 'shared/types';
import { LoginRequiredPrompt } from '@/components/dialogs/shared/LoginRequiredPrompt';
import { useAuth } from '@/hooks';
import { useProject } from '@/contexts/ProjectContext';
import { useTaskMutations } from '@/hooks/useTaskMutations';
import type { TFunction } from 'i18next';

export interface ShareDialogProps {
  task: TaskWithAttemptStatus;
}

// Status content renderer
function StatusContent({
  isRemoteDisabled,
  isSignedIn,
  isProjectLinked,
  shareTask,
  shareError,
  remoteDisabledMessage,
  handleLinkProject,
  t,
}: Readonly<{
  isRemoteDisabled: boolean;
  isSignedIn: boolean;
  isProjectLinked: boolean;
  shareTask: ReturnType<typeof useTaskMutations>['shareTask'];
  shareError: string | null;
  remoteDisabledMessage: string;
  handleLinkProject: () => void;
  t: TFunction;
}>) {
  if (isRemoteDisabled) {
    return (
      <Alert className="mt-1">
        <AlertDescription>{remoteDisabledMessage}</AlertDescription>
      </Alert>
    );
  }

  if (!isSignedIn) {
    return (
      <LoginRequiredPrompt
        buttonVariant="outline"
        buttonSize="sm"
        buttonClassName="mt-1"
      />
    );
  }

  if (!isProjectLinked) {
    return (
      <Alert className="mt-1">
        <LinkIcon className="h-4 w-4" />
        <AlertDescription className="flex items-center justify-between">
          <span>{t('shareDialog.linkProjectRequired.description')}</span>
          <Button
            variant="outline"
            size="sm"
            onClick={handleLinkProject}
            className="ml-2"
          >
            {t('shareDialog.linkProjectRequired.action')}
          </Button>
        </AlertDescription>
      </Alert>
    );
  }

  if (shareTask.isSuccess) {
    return <Alert variant="success">{t('shareDialog.success')}</Alert>;
  }

  if (shareError) {
    return <Alert variant="destructive">{shareError}</Alert>;
  }

  return null;
}

const ShareDialogImpl = NiceModal.create<ShareDialogProps>(({ task }) => {
  const modal = useModal();
  const { t } = useTranslation('tasks');
  const { loading: systemLoading, remoteFeaturesEnabled } = useUserSystem();
  const { isSignedIn } = useAuth();
  const { project } = useProject();
  const { shareTask } = useTaskMutations(task.projectId);
  const { reset: resetShareTask } = shareTask;

  const [shareError, setShareError] = useState<string | null>(null);
  const remoteDisabledMessage = t('shareDialog.remoteDisabled', {
    defaultValue: 'Sharing is disabled in this build.',
  });

  useEffect(() => {
    resetShareTask();
    setShareError(null);
  }, [task.id, resetShareTask]);

  const handleClose = () => {
    modal.resolve(shareTask.isSuccess);
    modal.hide();
  };

  const getStatus = (err: unknown) =>
    err && typeof err === 'object' && 'status' in err
      ? (err as { status?: number }).status
      : undefined;

  const getReadableError = (err: unknown) => {
    const status = getStatus(err);
    if (status === 401) {
      return err instanceof Error && err.message
        ? err.message
        : t('shareDialog.loginRequired.description');
    }
    return err instanceof Error ? err.message : t('shareDialog.genericError');
  };

  const handleShare = async () => {
    if (!remoteFeaturesEnabled && !systemLoading) {
      setShareError(remoteDisabledMessage);
      return;
    }
    setShareError(null);
    try {
      await shareTask.mutateAsync(task.id);
      modal.hide();
    } catch (err) {
      if (getStatus(err) === 401) {
        // Hide this dialog first so OAuthDialog appears on top
        modal.hide();
        const result = await OAuthDialog.show();
        // If user successfully authenticated, re-show this dialog
        if (result) {
          ShareDialog.show({ task });
        }
        return;
      }
      setShareError(getReadableError(err));
    }
  };

  const handleLinkProject = () => {
    if (!project) return;
    if (!remoteFeaturesEnabled && !systemLoading) return;

    LinkProjectDialog.show({
      projectId: project.id,
      projectName: project.name,
    });
  };

  const isRemoteDisabled = !systemLoading && !remoteFeaturesEnabled;
  const isShareDisabled =
    systemLoading || shareTask.isPending || isRemoteDisabled;
  const isProjectLinked = project?.remoteProjectId != null;
  const canShare =
    isSignedIn && isProjectLinked && !shareTask.isSuccess && !isRemoteDisabled;

  return (
    <Dialog
      open={modal.visible}
      onOpenChange={(open) => {
        if (open) {
          shareTask.reset();
          setShareError(null);
        } else {
          handleClose();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('shareDialog.title')}</DialogTitle>
          <DialogDescription>
            {t('shareDialog.description', { title: task.title })}
          </DialogDescription>
        </DialogHeader>

        <StatusContent
          isRemoteDisabled={isRemoteDisabled}
          isSignedIn={isSignedIn}
          isProjectLinked={isProjectLinked}
          shareTask={shareTask}
          shareError={shareError}
          remoteDisabledMessage={remoteDisabledMessage}
          handleLinkProject={handleLinkProject}
          t={t}
        />

        <DialogFooter className="flex sm:flex-row sm:justify-end gap-2">
          <Button variant="outline" onClick={handleClose}>
            {shareTask.isSuccess
              ? t('shareDialog.closeButton')
              : t('shareDialog.cancel')}
          </Button>
          {canShare && (
            <Button
              onClick={handleShare}
              disabled={isShareDisabled}
              className="gap-2"
            >
              {shareTask.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" />
                  {t('shareDialog.inProgress')}
                </>
              ) : (
                t('shareDialog.confirm')
              )}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
});

export const ShareDialog = defineModal<ShareDialogProps, boolean>(
  ShareDialogImpl
);
