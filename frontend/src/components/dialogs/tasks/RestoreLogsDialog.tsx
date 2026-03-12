import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { AlertTriangle, GitCommit, Loader2 } from 'lucide-react';
import NiceModal, { useModal } from '@ebay/nice-modal-react';
import { defineModal } from '@/lib/modals';
import { useKeySubmitTask } from '@/keyboard/hooks';
import { Scope } from '@/keyboard/registry';
import { executionProcessesApi } from '@/lib/api';
import {
  shouldShowInLogs,
  isCodingAgent,
  PROCESS_RUN_REASONS,
} from '@/constants/processes';
import type {
  RepoBranchStatus,
  ExecutionProcess,
  ExecutionProcessRepoState,
} from 'shared/types';
import type { TFunction } from 'i18next';

export interface RestoreLogsDialogProps {
  executionProcessId: string;
  branchStatus: RepoBranchStatus[] | undefined;
  processes: ExecutionProcess[] | undefined;
  initialWorktreeResetOn?: boolean;
  initialForceReset?: boolean;
}

export type RestoreLogsDialogResult = {
  action: 'confirmed' | 'canceled';
  performGitReset?: boolean;
  forceWhenDirty?: boolean;
};

// Helper to get container className for reset section
function getResetContainerClassName(worktreeResetOn: boolean, hasRisk: boolean): string {
  if (!worktreeResetOn) {
    return 'flex items-start gap-3 rounded-md border p-3';
  }
  if (hasRisk) {
    return 'flex items-start gap-3 rounded-md border border-destructive/30 bg-destructive/10 p-3';
  }
  return 'flex items-start gap-3 rounded-md border p-3 border-amber-300/60 bg-amber-50/70 dark:border-amber-400/30 dark:bg-amber-900/20';
}

// Helper to get icon className for reset section
function getResetIconClassName(worktreeResetOn: boolean, hasRisk: boolean): string {
  if (!worktreeResetOn) {
    return 'h-4 w-4 text-muted-foreground mt-0.5';
  }
  if (hasRisk) {
    return 'h-4 w-4 text-destructive mt-0.5';
  }
  return 'h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5';
}

// Helper to get toggle label text
function getToggleLabelText(forceReset: boolean, worktreeResetOn: boolean, t: TFunction): string {
  if (forceReset) {
    return worktreeResetOn
      ? t('restoreLogsDialog.resetWorktree.enabled')
      : t('restoreLogsDialog.resetWorktree.disabled');
  }
  return t('restoreLogsDialog.resetWorktree.disabledUncommitted');
}

// Toggle switch component
function ToggleSwitch({
  enabled,
  enabledColor = 'bg-emerald-500',
}: Readonly<{
  enabled: boolean;
  enabledColor?: string;
}>) {
  return (
    <div className="ml-auto relative inline-flex h-5 w-9 items-center rounded-full">
      <span
        className={
          (enabled ? enabledColor : 'bg-muted-foreground/30') +
          ' absolute inset-0 rounded-full transition-colors'
        }
      />
      <span
        className={
          (enabled ? 'translate-x-5' : 'translate-x-1') +
          ' pointer-events-none relative inline-block h-3.5 w-3.5 rounded-full bg-white shadow transition-transform'
        }
      />
    </div>
  );
}

// Repository list component
function RepositoryList({
  repoInfo,
  repoCount,
}: Readonly<{
  repoInfo: Array<{
    repoId: string;
    repoName: string;
    targetSha: string | null;
  }>;
  repoCount: number;
}>) {
  return (
    <div className="mt-1 space-y-1">
      {repoInfo.map((repo) => (
        <div
          key={repo.repoId}
          className="flex flex-wrap items-center gap-2 min-w-0"
        >
          <GitCommit className="h-3.5 w-3.5 text-muted-foreground" />
          {repoCount > 1 && (
            <span className="text-xs text-muted-foreground">
              {repo.repoName}:
            </span>
          )}
          {repo.targetSha && (
            <span className="font-mono text-xs px-2 py-0.5 rounded bg-muted">
              {repo.targetSha.slice(0, 7)}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}

// History change warning section
function HistoryChangeWarning({
  hasLater,
  laterCount,
  laterCoding,
  laterSetup,
  laterCleanup,
  t,
}: Readonly<{
  hasLater: boolean;
  laterCount: number;
  laterCoding: number;
  laterSetup: number;
  laterCleanup: number;
  t: TFunction;
}>) {
  if (!hasLater) return null;

  return (
    <div className="flex items-start gap-3 rounded-md border border-destructive/30 bg-destructive/10 p-3">
      <AlertTriangle className="h-4 w-4 text-destructive mt-0.5" />
      <div className="text-sm min-w-0 w-full break-words">
        <p className="font-medium text-destructive mb-2">
          {t('restoreLogsDialog.historyChange.title')}
        </p>
        <p className="mt-0.5">
          {t('restoreLogsDialog.historyChange.willDelete')}
          {laterCount > 0 && (
            <>
              {' '}
              {t('restoreLogsDialog.historyChange.andLaterProcesses', {
                count: laterCount,
              })}
            </>
          )}{' '}
          {t('restoreLogsDialog.historyChange.fromHistory')}
        </p>
        <ul className="mt-1 text-xs text-muted-foreground list-disc pl-5">
          {laterCoding > 0 && (
            <li>
              {t('restoreLogsDialog.historyChange.codingAgentRuns', {
                count: laterCoding,
              })}
            </li>
          )}
          {laterSetup + laterCleanup > 0 && (
            <li>
              {t('restoreLogsDialog.historyChange.scriptProcesses', {
                count: laterSetup + laterCleanup,
              })}
              {laterSetup > 0 && laterCleanup > 0 && (
                <>
                  {' '}
                  {t('restoreLogsDialog.historyChange.setupCleanupBreakdown', {
                    setup: laterSetup,
                    cleanup: laterCleanup,
                  })}
                </>
              )}
            </li>
          )}
        </ul>
        <p className="mt-1 text-xs text-muted-foreground">
          {t('restoreLogsDialog.historyChange.permanentWarning')}
        </p>
      </div>
    </div>
  );
}

// Uncommitted changes warning section
function UncommittedChangesWarning({
  anyDirty,
  totalUncommitted,
  totalUntracked,
  acknowledgeUncommitted,
  setAcknowledgeUncommitted,
  t,
}: Readonly<{
  anyDirty: boolean;
  totalUncommitted: number;
  totalUntracked: number;
  acknowledgeUncommitted: boolean;
  setAcknowledgeUncommitted: (v: boolean) => void;
  t: TFunction;
}>) {
  if (!anyDirty) return null;

  return (
    <div className="flex items-start gap-3 rounded-md border border-amber-300/60 bg-amber-50/70 dark:border-amber-400/30 dark:bg-amber-900/20 p-3">
      <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5" />
      <div className="text-sm min-w-0 w-full break-words">
        <p className="font-medium text-amber-700 dark:text-amber-300">
          {t('restoreLogsDialog.uncommittedChanges.title')}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">
          {t('restoreLogsDialog.uncommittedChanges.description', {
            count: totalUncommitted,
          })}
          {totalUntracked > 0 &&
            t('restoreLogsDialog.uncommittedChanges.andUntracked', {
              count: totalUntracked,
            })}
          .
        </p>
        <button
          type="button"
          className="mt-2 w-full flex items-center cursor-pointer select-none"
          aria-pressed={acknowledgeUncommitted}
          onClick={() => setAcknowledgeUncommitted(!acknowledgeUncommitted)}
        >
          <div className="text-xs text-muted-foreground flex-1 min-w-0 break-words">
            {t('restoreLogsDialog.uncommittedChanges.acknowledgeLabel')}
          </div>
          <ToggleSwitch
            enabled={acknowledgeUncommitted}
            enabledColor="bg-amber-500"
          />
        </button>
      </div>
    </div>
  );
}

// Reset worktree section (can reset)
function ResetWorktreeCanReset({
  needGitReset,
  canGitReset,
  worktreeResetOn,
  setWorktreeResetOn,
  repoCount,
  repoInfo,
  totalUncommitted,
  totalUntracked,
  t,
}: Readonly<{
  needGitReset: boolean;
  canGitReset: boolean;
  worktreeResetOn: boolean;
  setWorktreeResetOn: (v: boolean) => void;
  repoCount: number;
  repoInfo: Array<{
    repoId: string;
    repoName: string;
    targetSha: string | null;
  }>;
  totalUncommitted: number;
  totalUntracked: number;
  t: TFunction;
}>) {
  if (!(needGitReset && canGitReset)) return null;

  return (
    <div className={getResetContainerClassName(worktreeResetOn, false)}>
      <AlertTriangle className={getResetIconClassName(worktreeResetOn, false)} />
      <div className="text-sm min-w-0 w-full break-words">
        <p className="font-medium mb-2">
          {t('restoreLogsDialog.resetWorktree.title')}
          {repoCount > 1 && ` (${repoCount} repos)`}
        </p>
        <button
          type="button"
          className="mt-2 w-full flex items-center cursor-pointer select-none"
          aria-pressed={worktreeResetOn}
          onClick={() => setWorktreeResetOn(!worktreeResetOn)}
        >
          <div className="text-xs text-muted-foreground flex-1 min-w-0 break-words">
            {worktreeResetOn
              ? t('restoreLogsDialog.resetWorktree.enabled')
              : t('restoreLogsDialog.resetWorktree.disabled')}
          </div>
          <ToggleSwitch enabled={worktreeResetOn} />
        </button>
        {worktreeResetOn && (
          <>
            <p className="mt-2 text-xs text-muted-foreground">
              {t('restoreLogsDialog.resetWorktree.restoreDescription')}
            </p>
            <RepositoryList repoInfo={repoInfo} repoCount={repoCount} />
            {(totalUncommitted > 0 || totalUntracked > 0) && (
              <ul className="mt-2 space-y-1 text-xs text-muted-foreground list-disc pl-5">
                {totalUncommitted > 0 && (
                  <li>
                    {t('restoreLogsDialog.resetWorktree.discardChanges', {
                      count: totalUncommitted,
                    })}
                  </li>
                )}
                {totalUntracked > 0 && (
                  <li>
                    {t('restoreLogsDialog.resetWorktree.untrackedPresent', {
                      count: totalUntracked,
                    })}
                  </li>
                )}
              </ul>
            )}
          </>
        )}
      </div>
    </div>
  );
}

// Reset worktree section (cannot reset - has uncommitted)
function ResetWorktreeCannotReset({
  needGitReset,
  canGitReset,
  worktreeResetOn,
  setWorktreeResetOn,
  forceReset,
  setForceReset,
  repoCount,
  repoInfo,
  t,
}: Readonly<{
  needGitReset: boolean;
  canGitReset: boolean;
  worktreeResetOn: boolean;
  setWorktreeResetOn: (v: boolean) => void;
  forceReset: boolean;
  setForceReset: (v: boolean) => void;
  repoCount: number;
  repoInfo: Array<{
    repoId: string;
    repoName: string;
    targetSha: string | null;
  }>;
  t: TFunction;
}>) {
  if (!(needGitReset && !canGitReset)) return null;

  const containerClassName =
    forceReset && worktreeResetOn
      ? 'flex items-start gap-3 rounded-md border border-destructive/30 bg-destructive/10 p-3'
      : 'flex items-start gap-3 rounded-md border p-3';

  return (
    <div className={containerClassName}>
      <AlertTriangle className="h-4 w-4 text-destructive mt-0.5" />
      <div className="text-sm min-w-0 w-full break-words">
        <p className="font-medium text-destructive">
          {t('restoreLogsDialog.resetWorktree.title')}
          {repoCount > 1 && ` (${repoCount} repos)`}
        </p>
        <button
          type="button"
          className="mt-2 w-full flex items-center select-none cursor-pointer"
          aria-pressed={worktreeResetOn && forceReset}
          onClick={() => {
            setWorktreeResetOn(
              forceReset ? !worktreeResetOn : false
            );
          }}
        >
          <div className="text-xs text-muted-foreground flex-1 min-w-0 break-words">
            {getToggleLabelText(forceReset, worktreeResetOn, t)}
          </div>
          <ToggleSwitch
            enabled={worktreeResetOn && forceReset}
          />
        </button>
        <button
          type="button"
          className="mt-2 w-full flex items-center cursor-pointer select-none"
          aria-pressed={forceReset}
          onClick={() => {
            const next = !forceReset;
            if (next) setWorktreeResetOn(true);
            setForceReset(next);
          }}
        >
          <div className="text-xs font-medium text-destructive flex-1 min-w-0 break-words">
            {t('restoreLogsDialog.resetWorktree.forceReset')}
          </div>
          <ToggleSwitch
            enabled={forceReset}
            enabledColor="bg-destructive"
          />
        </button>
        <p className="mt-2 text-xs text-muted-foreground">
          {forceReset
            ? t('restoreLogsDialog.resetWorktree.uncommittedWillDiscard')
            : t('restoreLogsDialog.resetWorktree.uncommittedPresentHint')}
        </p>
        {repoInfo.length > 0 && (
          <>
            <p className="mt-2 text-xs text-muted-foreground">
              {t('restoreLogsDialog.resetWorktree.restoreDescription')}
            </p>
            <RepositoryList repoInfo={repoInfo} repoCount={repoCount} />
          </>
        )}
      </div>
    </div>
  );
}

const RestoreLogsDialogImpl = NiceModal.create<RestoreLogsDialogProps>(
  ({
    executionProcessId,
    branchStatus,
    processes,
    initialWorktreeResetOn = false,
    initialForceReset = false,
  }) => {
    const modal = useModal();
    const { t } = useTranslation(['tasks', 'common']);
    const [isLoading, setIsLoading] = useState(true);
    const [worktreeResetOn, setWorktreeResetOn] = useState(
      initialWorktreeResetOn
    );
    const [forceReset, setForceReset] = useState(initialForceReset);
    const [acknowledgeUncommitted, setAcknowledgeUncommitted] = useState(false);

    // Fetched data - stores all repo states for multi-repo support
    const [repoStates, setRepoStates] = useState<ExecutionProcessRepoState[]>(
      []
    );

    // Fetch execution process repo states
    useEffect(() => {
      let cancelled = false;
      setIsLoading(true);

      (async () => {
        try {
          // Fetch repo states for the execution process (supports multi-repo)
          const states =
            await executionProcessesApi.getRepoStates(executionProcessId);
          if (cancelled) return;
          setRepoStates(states);
        } finally {
          if (!cancelled) setIsLoading(false);
        }
      })();

      return () => {
        cancelled = true;
      };
    }, [executionProcessId]);

    // Compute later processes from props
    const { laterCount, laterCoding, laterSetup, laterCleanup } =
      useMemo(() => {
        const procs = (processes || []).filter(
          (p) => !p.dropped && shouldShowInLogs(p.runReason)
        );
        const idx = procs.findIndex((p) => p.id === executionProcessId);
        const later = idx >= 0 ? procs.slice(idx + 1) : [];
        return {
          laterCount: later.length,
          laterCoding: later.filter((p) => isCodingAgent(p.runReason)).length,
          laterSetup: later.filter(
            (p) => p.runReason === PROCESS_RUN_REASONS.SETUP_SCRIPT
          ).length,
          laterCleanup: later.filter(
            (p) => p.runReason === PROCESS_RUN_REASONS.CLEANUP_SCRIPT
          ).length,
        };
      }, [processes, executionProcessId]);

    // Join repo states with branch status to get repo names and compute aggregated values
    const repoInfo = useMemo(() => {
      return repoStates.map((state) => {
        const bs = branchStatus?.find((b) => b.repo_id === state.repoId);
        return {
          repoId: state.repoId,
          repoName: bs?.repo_name ?? state.repoId,
          targetSha: state.beforeHeadCommit,
          headOid: bs?.head_oid ?? null,
          hasUncommitted: bs?.has_uncommitted_changes ?? false,
          uncommittedCount: bs?.uncommitted_count ?? 0,
          untrackedCount: bs?.untracked_count ?? 0,
        };
      });
    }, [repoStates, branchStatus]);

    // Aggregate values across all repos
    const anyDirty = repoInfo.some((r) => r.hasUncommitted);
    const totalUncommitted = repoInfo.reduce(
      (sum, r) => sum + r.uncommittedCount,
      0
    );
    const totalUntracked = repoInfo.reduce(
      (sum, r) => sum + r.untrackedCount,
      0
    );
    const anyNeedsReset = repoInfo.some(
      (r) => r.targetSha && (r.targetSha !== r.headOid || r.hasUncommitted)
    );
    const needGitReset = anyNeedsReset;
    const canGitReset = needGitReset && !anyDirty;
    const hasRisk = anyDirty;

    const hasLater = laterCount > 0;
    const repoCount = repoInfo.length;

    const isConfirmDisabled =
      isLoading ||
      (anyDirty && !acknowledgeUncommitted) ||
      (hasRisk && worktreeResetOn && needGitReset && !forceReset);

    const handleConfirm = () => {
      modal.resolve({
        action: 'confirmed',
        performGitReset: worktreeResetOn,
        forceWhenDirty: forceReset,
      } as RestoreLogsDialogResult);
      modal.hide();
    };

    const handleCancel = () => {
      modal.resolve({ action: 'canceled' } as RestoreLogsDialogResult);
      modal.hide();
    };

    const handleOpenChange = (open: boolean) => {
      if (!open) {
        handleCancel();
      }
    };

    // CMD+Enter to confirm
    useKeySubmitTask(handleConfirm, {
      scope: Scope.DIALOG,
      when: modal.visible && !isConfirmDisabled,
    });

    return (
      <Dialog open={modal.visible} onOpenChange={handleOpenChange}>
        <DialogContent
          className="max-h-[92vh] sm:max-h-[88vh] overflow-y-auto overflow-x-hidden"
          onKeyDownCapture={(e) => {
            if (e.key === 'Escape') {
              e.stopPropagation();
              handleCancel();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 mb-3 md:mb-4">
              <AlertTriangle className="h-4 w-4 text-destructive" />{' '}
              {t('restoreLogsDialog.title')}
            </DialogTitle>
            <div className="mt-6 break-words text-sm text-muted-foreground">
              {isLoading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
              ) : (
                <div className="space-y-3">
                  <HistoryChangeWarning
                    hasLater={hasLater}
                    laterCount={laterCount}
                    laterCoding={laterCoding}
                    laterSetup={laterSetup}
                    laterCleanup={laterCleanup}
                    t={t}
                  />
                  <UncommittedChangesWarning
                    anyDirty={anyDirty}
                    totalUncommitted={totalUncommitted}
                    totalUntracked={totalUntracked}
                    acknowledgeUncommitted={acknowledgeUncommitted}
                    setAcknowledgeUncommitted={setAcknowledgeUncommitted}
                    t={t}
                  />
                  <ResetWorktreeCanReset
                    needGitReset={needGitReset}
                    canGitReset={canGitReset}
                    worktreeResetOn={worktreeResetOn}
                    setWorktreeResetOn={setWorktreeResetOn}
                    repoCount={repoCount}
                    repoInfo={repoInfo}
                    totalUncommitted={totalUncommitted}
                    totalUntracked={totalUntracked}
                    t={t}
                  />
                  <ResetWorktreeCannotReset
                    needGitReset={needGitReset}
                    canGitReset={canGitReset}
                    worktreeResetOn={worktreeResetOn}
                    setWorktreeResetOn={setWorktreeResetOn}
                    forceReset={forceReset}
                    setForceReset={setForceReset}
                    repoCount={repoCount}
                    repoInfo={repoInfo}
                    t={t}
                  />
                </div>
              )}
            </div>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={handleCancel}>
              {t('common:buttons.cancel')}
            </Button>
            <Button
              variant="destructive"
              disabled={isConfirmDisabled}
              onClick={handleConfirm}
            >
              {t('restoreLogsDialog.buttons.retry')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }
);

export const RestoreLogsDialog = defineModal<
  RestoreLogsDialogProps,
  RestoreLogsDialogResult
>(RestoreLogsDialogImpl);
