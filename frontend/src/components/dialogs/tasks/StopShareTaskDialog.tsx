import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Alert } from '@/components/ui/alert';
import NiceModal, { useModal } from '@ebay/nice-modal-react';
import { defineModal } from '@/lib/modals';
import { useTranslation } from 'react-i18next';
import type { SharedTaskRecord } from '@/hooks/useProjectTasks';
import { useTaskMutations } from '@/hooks/useTaskMutations';
import { useProject } from '@/contexts/ProjectContext';

export interface StopShareTaskDialogProps {
  sharedTask: SharedTaskRecord;
}

const StopShareTaskDialogImpl = NiceModal.create<StopShareTaskDialogProps>(
  ({ sharedTask }) => {
    const modal = useModal();
    const { t } = useTranslation('tasks');
    const { projectId } = useProject();
    const { stopShareTask } = useTaskMutations(projectId ?? undefined);
    const [error, setError] = useState<string | null>(null);
    // E12-10: Replace the two-ref signaling (isProgrammaticCloseRef +
    // didConfirmRef) with a single state-machine enum. 'open' is the initial
    // state; 'confirmed' and 'cancelled' record how the dialog closed so the
    // radix onOpenChange handler can resolve/reject correctly.
    const [closeOutcome, setCloseOutcome] = useState<
      'open' | 'confirmed' | 'cancelled'
    >('open');

    const getReadableError = (err: unknown) =>
      err instanceof Error && err.message
        ? err.message
        : t('stopShareDialog.genericError');

    const requestClose = (outcome: 'confirmed' | 'cancelled') => {
      if (stopShareTask.isPending) {
        return;
      }
      setCloseOutcome(outcome);
      modal.hide();
    };

    const handleCancel = () => {
      requestClose('cancelled');
    };

    const handleConfirm = async () => {
      setError(null);
      try {
        await stopShareTask.mutateAsync(sharedTask.id);
        requestClose('confirmed');
      } catch (err: unknown) {
        setError(getReadableError(err));
      }
    };

    return (
      <Dialog
        open={modal.visible}
        onOpenChange={(open) => {
          if (open) {
            stopShareTask.reset();
            setError(null);
            setCloseOutcome('open');
            return;
          }

          if (stopShareTask.isPending) {
            return;
          }

          const outcome = closeOutcome;
          setCloseOutcome('open');
          stopShareTask.reset();

          if (outcome === 'confirmed') {
            modal.resolve();
          } else {
            modal.reject(new Error('Stop share cancelled by user'));
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('stopShareDialog.title')}</DialogTitle>
            <DialogDescription>
              {t('stopShareDialog.description', { title: sharedTask.title })}
            </DialogDescription>
          </DialogHeader>

          <Alert variant="destructive" className="mb-4">
            {t('stopShareDialog.warning')}
          </Alert>

          {error && (
            <Alert variant="destructive" className="mb-4">
              {error}
            </Alert>
          )}

          <DialogFooter>
            <Button
              variant="outline"
              onClick={handleCancel}
              disabled={stopShareTask.isPending}
              autoFocus
            >
              {t('common:buttons.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleConfirm}
              disabled={stopShareTask.isPending}
            >
              {stopShareTask.isPending
                ? t('stopShareDialog.inProgress')
                : t('stopShareDialog.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }
);

export const StopShareTaskDialog = defineModal<StopShareTaskDialogProps, void>(
  StopShareTaskDialogImpl
);
