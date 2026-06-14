import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import NiceModal, { useModal } from '@ebay/nice-modal-react';
import { defineModal } from '@/lib/modals';

export interface DeleteConfigurationDialogProps {
  configName: string;
  executorType: string;
}

export type DeleteConfigurationResult = 'deleted' | 'canceled';

const DeleteConfigurationDialogImpl =
  NiceModal.create<DeleteConfigurationDialogProps>(
    ({ configName, executorType }) => {
      const modal = useModal();

      const handleDelete = () => {
        modal.resolve('deleted' as DeleteConfigurationResult);
        modal.hide();
      };

      const handleCancel = () => {
        modal.resolve('canceled' as DeleteConfigurationResult);
        modal.hide();
      };

      const handleOpenChange = (open: boolean) => {
        if (!open) {
          handleCancel();
        }
      };

      return (
        <Dialog open={modal.visible} onOpenChange={handleOpenChange}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>Delete Configuration?</DialogTitle>
              <DialogDescription>
                This will permanently remove "{configName}" from the{' '}
                {executorType} executor. You can't undo this action.
              </DialogDescription>
            </DialogHeader>

            <DialogFooter>
              <Button
                variant="outline"
                onClick={handleCancel}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
              >
                Delete
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      );
    }
  );

export const DeleteConfigurationDialog = defineModal<
  DeleteConfigurationDialogProps,
  DeleteConfigurationResult
>(DeleteConfigurationDialogImpl);
