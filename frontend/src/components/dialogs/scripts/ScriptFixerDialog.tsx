import { useState, useEffect, useMemo, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import NiceModal, { useModal } from '@ebay/nice-modal-react';
import { useQueryClient } from '@tanstack/react-query';
import { Loader2 } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { AutoExpandingTextarea } from '@/components/ui/auto-expanding-textarea';
import { VirtualizedProcessLogs } from '@/components/ui-new/containers/VirtualizedProcessLogs';
import { RunningDots } from '@/components/ui-new/primitives/RunningDots';
import { defineModal } from '@/lib/modals';
import { repoApi, attemptsApi } from '@/lib/api';
import { useLogStream } from '@/hooks/useLogStream';
import { useExecutionProcesses } from '@/hooks/useExecutionProcesses';
import type { Repo, RepoWithTargetBranch, PatchType, UpdateRepo, ExecutionProcess } from 'shared/types';
import type { TFunction } from 'i18next';

export type ScriptType = 'setup' | 'cleanup' | 'dev_server';

export interface ScriptFixerDialogProps {
  scriptType: ScriptType;
  repos: RepoWithTargetBranch[];
  workspaceId: string;
  sessionId?: string;
  initialRepoId?: string;
}

export type ScriptFixerDialogResult = {
  action: 'saved' | 'saved_and_tested' | 'canceled';
};

type LogEntry = Extract<PatchType, { type: 'STDOUT' } | { type: 'STDERR' }>;

// Helper to get update data based on script type
function getScriptUpdateData(scriptType: ScriptType, script: string): Partial<UpdateRepo> {
  const scriptValue = script.trim() || null;

  if (scriptType === 'setup') {
    return { setupScript: scriptValue };
  }
  if (scriptType === 'cleanup') {
    return { cleanupScript: scriptValue };
  }
  return { devServerScript: scriptValue };
}

// Helper to run script based on type
async function runScriptByType(scriptType: ScriptType, workspaceId: string) {
  if (scriptType === 'setup') {
    await attemptsApi.runSetupScript(workspaceId);
  } else if (scriptType === 'cleanup') {
    await attemptsApi.runCleanupScript(workspaceId);
  } else {
    await attemptsApi.startDevServer(workspaceId);
  }
}

// Helper to get dialog title
function getDialogTitle(scriptType: ScriptType, t: TFunction): string {
  if (scriptType === 'setup') {
    return t('scriptFixer.setupScriptTitle');
  }
  if (scriptType === 'cleanup') {
    return t('scriptFixer.cleanupScriptTitle');
  }
  return t('scriptFixer.devServerTitle');
}

// Helper to get placeholder text
function getPlaceholderText(scriptType: ScriptType): string {
  if (scriptType === 'setup') {
    return '#!/bin/bash\nnpm install';
  }
  if (scriptType === 'cleanup') {
    return '#!/bin/bash\nrm -rf node_modules';
  }
  return '#!/bin/bash\nnpm run dev';
}

// Helper to get script content from repo
function getScriptContent(repo: Repo | RepoWithTargetBranch, scriptType: ScriptType): string {
  if (scriptType === 'setup') {
    return repo.setupScript ?? '';
  }
  if (scriptType === 'cleanup') {
    return repo.cleanupScript ?? '';
  }
  return repo.devServerScript ?? '';
}

// Helper to get run reason from script type
function getRunReason(scriptType: ScriptType): string {
  if (scriptType === 'setup') {
    return 'setupscript';
  }
  if (scriptType === 'cleanup') {
    return 'cleanupscript';
  }
  return 'devserver';
}

// Status indicator component
function ProcessStatusIndicator({
  isProcessRunning,
  isProcessSuccessful,
  hasProcessError,
  isProcessKilled,
  exitCode,
  t,
}: Readonly<{
  isProcessRunning: boolean;
  isProcessSuccessful: boolean;
  hasProcessError: boolean;
  isProcessKilled: boolean;
  exitCode: number | bigint | null | undefined;
  t: TFunction;
}>) {
  if (isProcessRunning) {
    return (
      <>
        <RunningDots />
        <span className="text-muted-foreground">
          {t('scriptFixer.statusRunning')}
        </span>
      </>
    );
  }

  if (isProcessSuccessful) {
    return (
      <>
        <span className="size-2 rounded-full bg-success" />
        <span className="text-success">
          {t('scriptFixer.statusSuccess')}
        </span>
      </>
    );
  }

  if (hasProcessError) {
    return (
      <>
        <span className="size-2 rounded-full bg-destructive bg-error" />
        <span className="text-destructive text-error">
          {t('scriptFixer.statusFailed', {
            exitCode: Number(exitCode ?? 0),
          })}
        </span>
      </>
    );
  }

  if (isProcessKilled) {
    return (
      <>
        <span className="size-2 rounded-full bg-low" />
        <span className="text-muted-foreground">
          {t('scriptFixer.statusKilled')}
        </span>
      </>
    );
  }

  return null;
}

const ScriptFixerDialogImpl = NiceModal.create<ScriptFixerDialogProps>(
  ({ scriptType, repos, workspaceId, sessionId, initialRepoId }) => {
    const modal = useModal();
    const { t } = useTranslation(['tasks', 'common']);
    const queryClient = useQueryClient();

    // State
    const [selectedRepoId, setSelectedRepoId] = useState<string>(
      initialRepoId || repos[0]?.id || ''
    );
    const [script, setScript] = useState('');
    const [originalScript, setOriginalScript] = useState('');
    const [isLoadingRepo, setIsLoadingRepo] = useState(true);
    const [isSaving, setIsSaving] = useState(false);
    const [isTesting, setIsTesting] = useState(false);
    const [error, setError] = useState<string | null>(null);

    // Get execution processes for the session to find latest script process
    const { executionProcesses } = useExecutionProcesses(sessionId, undefined);

    // Find the latest process for this script type
    const latestProcess = useMemo(() => {
      const runReason = getRunReason(scriptType);
      const filtered = executionProcesses.filter(
        (p) => p.runReason === runReason && !p.dropped
      );
      // Sort by createdAt descending and return the first one
      return filtered
        .slice()
        .sort(
          (a: ExecutionProcess, b: ExecutionProcess) =>
            new Date(b.createdAt).getTime() -
            new Date(a.createdAt).getTime()
        )[0];
    }, [executionProcesses, scriptType]);

    // Stream logs for the latest process
    const { logs: rawLogs, error: logsError } = useLogStream(
      latestProcess?.id ?? ''
    );
    const logs: LogEntry[] = rawLogs.filter(
      (l): l is LogEntry => l.type === 'STDOUT' || l.type === 'STDERR'
    );

    // Compute status for the latest process
    const isProcessRunning = latestProcess?.status === 'running';
    const isProcessCompleted = latestProcess?.status === 'completed';
    const isProcessKilled = latestProcess?.status === 'killed';
    const isProcessFailed = latestProcess?.status === 'failed';
    // exitCode can be null, number, or BigInt - convert to Number for comparison
    const exitCode = latestProcess?.exitCode;
    const isExitCodeZero = exitCode == null || Number(exitCode) === 0;
    const isProcessSuccessful = isProcessCompleted && isExitCodeZero;
    const hasProcessError =
      isProcessFailed || (isProcessCompleted && !isExitCodeZero);

    // Reset selectedRepoId on dialog re-open
    useEffect(() => {
      if (!initialRepoId) return;
      setSelectedRepoId(initialRepoId);
    }, [initialRepoId]);

    // Fetch the selected repo's script
    useEffect(() => {
      if (!selectedRepoId) return;

      let cancelled = false;
      setIsLoadingRepo(true);
      setError(null);

      (async () => {
        try {
          const repo = await repoApi.getById(selectedRepoId);
          if (cancelled) return;

          const scriptContent = getScriptContent(repo, scriptType);
          setScript(scriptContent);
          setOriginalScript(scriptContent);
        } catch (err) {
          if (cancelled) return;
          setError(
            err instanceof Error ? err.message : t('common:error.generic')
          );
        } finally {
          if (!cancelled) setIsLoadingRepo(false);
        }
      })();

      return () => {
        cancelled = true;
      };
    }, [selectedRepoId, scriptType, t]);

    const hasChanges = script !== originalScript;

    const handleClose = useCallback(() => {
      modal.resolve({ action: 'canceled' } as ScriptFixerDialogResult);
      modal.hide();
    }, [modal]);

    const handleOpenChange = (open: boolean) => {
      if (!open) {
        handleClose();
      }
    };

    const handleSave = useCallback(async () => {
      if (!selectedRepoId) return;

      setIsSaving(true);
      setError(null);

      try {
        const updateData = getScriptUpdateData(scriptType, script);
        await repoApi.update(selectedRepoId, updateData as UpdateRepo);

        // Invalidate repos cache
        queryClient.invalidateQueries({ queryKey: ['repos'] });

        setOriginalScript(script);
        modal.resolve({ action: 'saved' } as ScriptFixerDialogResult);
        modal.hide();
      } catch (err) {
        setError(
          err instanceof Error ? err.message : t('common:error.generic')
        );
      } finally {
        setIsSaving(false);
      }
    }, [selectedRepoId, script, scriptType, queryClient, modal, t]);

    const handleSaveAndTest = useCallback(async () => {
      if (!selectedRepoId) return;

      setIsTesting(true);
      setError(null);

      try {
        const updateData = getScriptUpdateData(scriptType, script);
        await repoApi.update(selectedRepoId, updateData as UpdateRepo);

        // Invalidate repos cache
        queryClient.invalidateQueries({ queryKey: ['repos'] });

        setOriginalScript(script);

        // Then run the script
        await runScriptByType(scriptType, workspaceId);

        // Keep dialog open so user can see the new execution logs
        // The logs will update automatically via useLogStream/useExecutionProcesses
      } catch (err) {
        setError(
          err instanceof Error ? err.message : t('common:error.generic')
        );
      } finally {
        setIsTesting(false);
      }
    }, [selectedRepoId, script, scriptType, workspaceId, queryClient, t]);

    const dialogTitle = getDialogTitle(scriptType, t);

    return (
      <Dialog
        open={modal.visible}
        onOpenChange={handleOpenChange}
        className="max-w-4xl w-[90vw]"
      >
        <DialogContent className="max-h-[80vh] flex flex-col overflow-hidden">
          <DialogHeader>
            <DialogTitle>{dialogTitle}</DialogTitle>
          </DialogHeader>

          <div className="flex-1 flex flex-col gap-4 min-h-0 min-w-0 overflow-hidden">
            {/* Repo selector (only show if multiple repos) */}
            {repos.length > 1 && (
              <div className="flex items-center gap-2">
                <Label htmlFor="repo-select" className="shrink-0">
                  {t('scriptFixer.selectRepo')}
                </Label>
                <Select
                  value={selectedRepoId}
                  onValueChange={setSelectedRepoId}
                >
                  <SelectTrigger id="repo-select" className="flex-1">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {repos.map((repo) => (
                      <SelectItem key={repo.id} value={repo.id}>
                        {repo.displayName || repo.path}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}

            {/* Script editor */}
            <div className="flex flex-col gap-2 flex-1 min-h-0 min-w-0">
              <Label>{t('scriptFixer.scriptLabel')}</Label>
              <div className="bg-panel flex-1 min-h-[150px] max-h-[300px] overflow-auto border rounded-md min-w-0">
                {isLoadingRepo ? (
                  <div className="h-full flex items-center justify-center">
                    <Loader2 className="h-6 w-6 animate-spin" />
                  </div>
                ) : (
                  <AutoExpandingTextarea
                    value={script}
                    onChange={(e) => setScript(e.target.value)}
                    className="font-mono text-sm p-3 border-0 min-h-full bg-panel"
                    placeholder={getPlaceholderText(scriptType)}
                    disableInternalScroll
                  />
                )}
              </div>
            </div>

            {/* Logs section */}
            <div
              className="flex flex-col gap-2 min-h-0 min-w-0"
              style={{ height: '200px' }}
            >
              <div className="flex items-center justify-between gap-2">
                <Label>{t('scriptFixer.logsLabel')}</Label>
                {/* Status indicator */}
                {latestProcess && (
                  <div className="flex items-center gap-2 text-sm">
                    <ProcessStatusIndicator
                      isProcessRunning={isProcessRunning}
                      isProcessSuccessful={isProcessSuccessful}
                      hasProcessError={hasProcessError}
                      isProcessKilled={isProcessKilled}
                      exitCode={latestProcess.exitCode}
                      t={t}
                    />
                  </div>
                )}
              </div>
              <div className="bg-secondary py-base flex-1 border rounded-md bg-muted overflow-hidden min-w-0">
                {latestProcess ? (
                  <VirtualizedProcessLogs logs={logs} error={logsError} />
                ) : (
                  <div className="h-full flex items-center justify-center text-muted-foreground text-sm">
                    {t('scriptFixer.noLogs')}
                  </div>
                )}
              </div>
            </div>

            {/* Error display */}
            {error && <div className="text-destructive text-sm">{error}</div>}
          </div>

          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={handleClose}>
              {t('common:buttons.close')}
            </Button>
            <Button
              variant="outline"
              onClick={handleSave}
              disabled={!hasChanges || isSaving || isTesting}
            >
              {isSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('scriptFixer.saveButton')}
            </Button>
            <Button
              onClick={handleSaveAndTest}
              disabled={isSaving || isTesting}
            >
              {isTesting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('scriptFixer.saveAndTestButton')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }
);

export const ScriptFixerDialog = defineModal<
  ScriptFixerDialogProps,
  ScriptFixerDialogResult
>(ScriptFixerDialogImpl);
