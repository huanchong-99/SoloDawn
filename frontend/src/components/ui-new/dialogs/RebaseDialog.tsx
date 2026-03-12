import { useEffect, useState } from 'react';
import { CaretRightIcon } from '@phosphor-icons/react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import BranchSelector from '@/components/tasks/BranchSelector';
import type { GitBranch, GitOperationError, Workspace } from 'shared/types';
import NiceModal, { useModal, type NiceModalHandler } from '@ebay/nice-modal-react';
import { defineModal } from '@/lib/modals';
import { GitOperationsProvider } from '@/contexts/GitOperationsContext';
import { useGitOperations } from '@/hooks/useGitOperations';
import { useAttempt } from '@/hooks/useAttempt';
import { attemptsApi, type Result } from '@/lib/api';
import { ResolveConflictsDialog } from './ResolveConflictsDialog';

// Helper to extract error type from Result
function getErrorType(err: unknown): string | undefined {
  const resultErr = err as Result<void, GitOperationError> | undefined;
  if (resultErr && !resultErr.success) {
    return resultErr.error?.type;
  }
  return undefined;
}

// Helper to check if error is a conflict error
function isConflictError(errorType: string | undefined): boolean {
  return errorType === 'merge_conflicts' || errorType === 'rebase_in_progress';
}

function stringifyErrorValue(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  if (value instanceof Error) {
    return value.message;
  }
  if (value && typeof value === 'object') {
    try {
      const serialized = JSON.stringify(value);
      return serialized ?? 'Failed to rebase';
    } catch {
      return 'Failed to rebase';
    }
  }
  if (
    typeof value === 'number' ||
    typeof value === 'boolean' ||
    typeof value === 'bigint' ||
    typeof value === 'symbol' ||
    typeof value === 'function'
  ) {
    return value.toString();
  }
  return 'Failed to rebase';
}

// Helper to extract error message from various error structures
function extractErrorMessage(err: unknown): string {
  if (!err || typeof err !== 'object') {
    return 'Failed to rebase';
  }

  // Handle Result<void, GitOperationError> structure
  if ('error' in err && err.error && typeof err.error === 'object' && 'message' in err.error) {
    return stringifyErrorValue(err.error.message);
  }

  if ('message' in err && err.message) {
    return stringifyErrorValue(err.message);
  }

  return 'Failed to rebase';
}

// Helper to handle conflict errors
async function handleConflictError(
  attemptId: string,
  repoId: string,
  workspace: Workspace | undefined,
  modal: NiceModalHandler
): Promise<void> {
  modal.hide();

  // Fetch fresh branch status to get conflict details
  const branchStatus = await attemptsApi.getBranchStatus(attemptId);
  const repoStatus = branchStatus?.find((s) => s.repo_id === repoId);

  if (repoStatus) {
    await ResolveConflictsDialog.show({
      workspaceId: attemptId,
      conflictOp: repoStatus.conflict_op ?? 'rebase',
      sourceBranch: workspace?.branch ?? null,
      targetBranch: repoStatus.target_branch_name,
      conflictedFiles: repoStatus.conflicted_files ?? [],
      repoName: repoStatus.repo_name,
    });
  }
}

export interface RebaseDialogProps {
  attemptId: string;
  repoId: string;
  branches: GitBranch[];
  initialTargetBranch?: string;
}

interface RebaseDialogContentProps {
  attemptId: string;
  repoId: string;
  branches: GitBranch[];
  initialTargetBranch?: string;
}

function RebaseDialogContent({
  attemptId,
  repoId,
  branches,
  initialTargetBranch,
}: Readonly<RebaseDialogContentProps>) {
  const modal = useModal();
  const { t } = useTranslation(['tasks', 'common']);
  const [selectedBranch, setSelectedBranch] = useState<string>(
    initialTargetBranch ?? ''
  );
  const [selectedUpstream, setSelectedUpstream] = useState<string>(
    initialTargetBranch ?? ''
  );
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const git = useGitOperations(attemptId, repoId);
  const { data: workspace } = useAttempt(attemptId);

  useEffect(() => {
    if (initialTargetBranch) {
      setSelectedBranch(initialTargetBranch);
      setSelectedUpstream(initialTargetBranch);
    }
  }, [initialTargetBranch]);

  const handleConfirm = async () => {
    if (!selectedBranch) return;

    setError(null);
    try {
      await git.actions.rebase({
        repoId,
        newBaseBranch: selectedBranch,
        oldBaseBranch: selectedUpstream,
      });
      modal.hide();
    } catch (err) {
      const errorType = getErrorType(err);

      if (isConflictError(errorType)) {
        await handleConflictError(attemptId, repoId, workspace, modal);
        return;
      }

      // Handle other errors
      const message = extractErrorMessage(err);
      setError(message);
    }
  };

  const handleCancel = () => {
    modal.hide();
  };

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      handleCancel();
    }
  };

  const isLoading = git.states.rebasePending;

  return (
    <Dialog open={modal.visible} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('rebase.dialog.title')}</DialogTitle>
          <DialogDescription>
            {t('rebase.dialog.description')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <label htmlFor="target-branch" className="text-sm font-medium">
              {t('rebase.dialog.targetLabel')}
            </label>
            <BranchSelector
              branches={branches}
              selectedBranch={selectedBranch}
              onBranchSelect={setSelectedBranch}
              placeholder={t('rebase.dialog.targetPlaceholder')}
              excludeCurrentBranch={false}
            />
          </div>
          <div className="space-y-2">
            <button
              type="button"
              onClick={() => setShowAdvanced((prev) => !prev)}
              className="flex w-full items-center gap-2 text-left text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              <CaretRightIcon
                className={`h-3 w-3 transition-transform ${showAdvanced ? 'rotate-90' : ''}`}
              />
              <span>{t('rebase.dialog.advanced')}</span>
            </button>
            {showAdvanced && (
              <div className="space-y-2">
                <label
                  htmlFor="upstream-branch"
                  className="text-sm font-medium"
                >
                  {t('rebase.dialog.upstreamLabel')}
                </label>
                <BranchSelector
                  branches={branches}
                  selectedBranch={selectedUpstream}
                  onBranchSelect={setSelectedUpstream}
                  placeholder={t('rebase.dialog.upstreamPlaceholder')}
                  excludeCurrentBranch={false}
                />
              </div>
            )}
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleCancel} disabled={isLoading}>
            {t('common:buttons.cancel')}
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={isLoading || !selectedBranch}
          >
            {isLoading
              ? t('rebase.common.inProgress')
              : t('rebase.common.action')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const RebaseDialogImpl = NiceModal.create<RebaseDialogProps>(
  ({ attemptId, repoId, branches, initialTargetBranch }) => {
    return (
      <GitOperationsProvider attemptId={attemptId}>
        <RebaseDialogContent
          attemptId={attemptId}
          repoId={repoId}
          branches={branches}
          initialTargetBranch={initialTargetBranch}
        />
      </GitOperationsProvider>
    );
  }
);

export const RebaseDialog = defineModal<RebaseDialogProps, void>(
  RebaseDialogImpl
);
