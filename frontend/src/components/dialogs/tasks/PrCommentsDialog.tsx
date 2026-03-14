import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import NiceModal, { useModal } from '@ebay/nice-modal-react';
import { defineModal } from '@/lib/modals';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ChatCircleIcon, WarningCircleIcon, SpinnerGapIcon } from '@phosphor-icons/react';
import { usePrComments } from '@/hooks/usePrComments';
import { PrCommentCard } from '@/components/ui/pr-comment-card';
import type { UnifiedPrComment } from 'shared/types';
import type { TFunction } from 'i18next';

export interface PrCommentsDialogProps {
  attemptId: string;
  repoId: string;
}

export interface PrCommentsDialogResult {
  comments: UnifiedPrComment[];
}

function getCommentId(comment: UnifiedPrComment): string {
  return comment.comment_type === 'general'
    ? comment.id
    : comment.id.toString();
}

function getCommentPath(comment: UnifiedPrComment): string | undefined {
  return comment.comment_type === 'review' ? comment.path : undefined;
}

function getCommentLine(comment: UnifiedPrComment): number | undefined {
  if (comment.comment_type === 'review' && comment.line != null) {
    return Number(comment.line);
  }
  return undefined;
}

function getCommentDiffHunk(comment: UnifiedPrComment): string | undefined {
  return comment.comment_type === 'review' ? comment.diff_hunk ?? undefined : undefined;
}

// Content renderer component
function DialogContentRenderer({
  errorMessage,
  isLoading,
  comments,
  selectedIds,
  isAllSelected,
  selectAll,
  deselectAll,
  toggleSelection,
  t,
}: Readonly<{
  errorMessage: string | null;
  isLoading: boolean;
  comments: UnifiedPrComment[];
  selectedIds: Set<string>;
  isAllSelected: boolean;
  selectAll: () => void;
  deselectAll: () => void;
  toggleSelection: (id: string) => void;
  t: TFunction;
}>) {
  if (errorMessage) {
    return (
      <Alert variant="destructive">
        <WarningCircleIcon className="h-4 w-4" />
        <AlertDescription>{errorMessage}</AlertDescription>
      </Alert>
    );
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <SpinnerGapIcon className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (comments.length === 0) {
    return (
      <p className="text-center text-muted-foreground py-8">
        {t('tasks:prComments.dialog.noComments')}
      </p>
    );
  }

  return (
    <>
      <div className="flex items-center justify-between mb-3">
        <span className="text-sm text-muted-foreground">
          {t('tasks:prComments.dialog.selectedCount', {
            selected: selectedIds.size,
            total: comments.length,
          })}
        </span>
        <Button
          variant="ghost"
          size="sm"
          onClick={isAllSelected ? deselectAll : selectAll}
        >
          {isAllSelected
            ? t('tasks:prComments.dialog.deselectAll')
            : t('tasks:prComments.dialog.selectAll')}
        </Button>
      </div>
      <div className="space-y-3">
        {comments.map((comment) => {
          const id = getCommentId(comment);
          return (
            <div key={id} className="flex items-start gap-3 min-w-0">
              <Checkbox
                checked={selectedIds.has(id)}
                onCheckedChange={() => toggleSelection(id)}
                className="mt-3"
              />
              <PrCommentCard
                author={comment.author}
                body={comment.body}
                createdAt={comment.created_at}
                url={comment.url}
                commentType={comment.comment_type}
                path={getCommentPath(comment)}
                line={getCommentLine(comment)}
                diffHunk={getCommentDiffHunk(comment)}
                variant="list"
                onClick={() => toggleSelection(id)}
                className="flex-1 min-w-0"
              />
            </div>
          );
        })}
      </div>
    </>
  );
}

const PrCommentsDialogImpl = NiceModal.create<PrCommentsDialogProps>(
  ({ attemptId, repoId }) => {
    const { t } = useTranslation(['tasks', 'common']);
    const modal = useModal();
    const { data, isLoading, isError, error } = usePrComments(
      attemptId,
      repoId
    );
    const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

    const comments = data?.comments ?? [];

    // Reset selection when dialog opens
    useEffect(() => {
      if (modal.visible) {
        setSelectedIds(new Set());
      }
    }, [modal.visible]);

    const toggleSelection = (id: string) => {
      setSelectedIds((prev) => {
        const newSet = new Set(prev);
        if (newSet.has(id)) {
          newSet.delete(id);
        } else {
          newSet.add(id);
        }
        return newSet;
      });
    };

    const selectAll = () => {
      setSelectedIds(new Set(comments.map((c) => getCommentId(c))));
    };

    const deselectAll = () => {
      setSelectedIds(new Set());
    };

    const isAllSelected =
      comments.length > 0 && selectedIds.size === comments.length;

    const handleConfirm = () => {
      const selected = comments.filter((c) => selectedIds.has(getCommentId(c)));
      modal.resolve({ comments: selected });
      modal.hide();
    };

    const handleOpenChange = (open: boolean) => {
      if (!open) {
        modal.resolve({ comments: [] });
        modal.hide();
      }
    };

    // Check for specific error types from the API
    const errorMessage = isError ? getErrorMessage(error, t) : null;

    return (
      <Dialog
        open={modal.visible}
        onOpenChange={handleOpenChange}
        className="max-w-2xl p-0 overflow-hidden"
      >
        <DialogContent
          className="p-0"
          onKeyDownCapture={(e) => {
            if (e.key === 'Escape') {
              e.stopPropagation();
              modal.resolve({ comments: [] });
              modal.hide();
            }
          }}
        >
          <DialogHeader className="px-4 py-3 border-b">
            <DialogTitle className="flex items-center gap-2">
              <ChatCircleIcon className="h-5 w-5" />
              {t('tasks:prComments.dialog.title')}
            </DialogTitle>
          </DialogHeader>

          <div className="max-h-[70vh] flex flex-col min-h-0">
            <div className="p-4 overflow-auto flex-1 min-h-0">
              <DialogContentRenderer
                errorMessage={errorMessage}
                isLoading={isLoading}
                comments={comments}
                selectedIds={selectedIds}
                isAllSelected={isAllSelected}
                selectAll={selectAll}
                deselectAll={deselectAll}
                toggleSelection={toggleSelection}
                t={t}
              />
            </div>
          </div>

          {!errorMessage && !isLoading && comments.length > 0 && (
            <DialogFooter className="px-4 py-3 border-t">
              <Button variant="outline" onClick={() => handleOpenChange(false)}>
                {t('common:buttons.cancel')}
              </Button>
              <Button onClick={handleConfirm} disabled={selectedIds.size === 0}>
                {t('tasks:prComments.dialog.add')}
                {selectedIds.size > 0 ? ` (${selectedIds.size})` : ''}
              </Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>
    );
  }
);

function getErrorMessage(error: unknown, t: TFunction): string {
  // Check if it's an API error with error_data
  if (error && typeof error === 'object' && 'error_data' in error) {
    const errorData = (error as { error_data?: { type?: string } }).error_data;
    if (errorData?.type === 'no_pr_attached') {
      return t('tasks:prComments.errors.noPrAttached');
    }
    if (errorData?.type === 'cli_not_installed') {
      return t('tasks:prComments.errors.cliNotInstalled');
    }
    if (errorData?.type === 'cli_not_logged_in') {
      return t('tasks:prComments.errors.cliNotLoggedIn');
    }
  }
  return t('tasks:prComments.errors.loadFailed');
}

export const PrCommentsDialog = defineModal<
  PrCommentsDialogProps,
  PrCommentsDialogResult
>(PrCommentsDialogImpl);
