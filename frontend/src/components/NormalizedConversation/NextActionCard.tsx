import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  PlayIcon,
  PauseIcon,
  TerminalIcon,
  GitDiffIcon,
  CopyIcon,
  CheckIcon,
  GitBranchIcon,
  GearIcon,
} from '@phosphor-icons/react';
import { useNavigate } from 'react-router-dom';
import { ViewProcessesDialog } from '@/components/dialogs/tasks/ViewProcessesDialog';
import { CreateAttemptDialog } from '@/components/dialogs/tasks/CreateAttemptDialog';
import { GitActionsDialog } from '@/components/dialogs/tasks/GitActionsDialog';
import { useOpenInEditor } from '@/hooks/useOpenInEditor';
import { useDiffSummary } from '@/hooks/useDiffSummary';
import { useDevServer } from '@/hooks/useDevServer';
import { useHasDevServerScript } from '@/hooks/useHasDevServerScript';
import { Button } from '@/components/ui/button';
import { IdeIcon, getIdeName } from '@/components/ide/IdeIcon';
import { useUserSystem } from '@/components/ConfigProvider';
import { useProject } from '@/contexts/ProjectContext';
import { useQuery } from '@tanstack/react-query';
import { attemptsApi } from '@/lib/api';
import {
  BaseAgentCapability,
  type BaseCodingAgent,
  type EditorType,
  type ExecutionProcess,
  type TaskWithAttemptStatus,
} from 'shared/types';
import type { TFunction } from 'i18next';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';

type NextActionCardProps = {
  attemptId?: string;
  sessionId?: string;
  containerRef?: string | null;
  failed: boolean;
  execution_processes: number;
  task?: TaskWithAttemptStatus;
  needsSetup?: boolean;
};

const DiffSummaryButton: React.FC<{
  fileCount: number;
  added: number;
  deleted: number;
  onClick: () => void;
  label: string;
}> = ({ fileCount, added, deleted, onClick, label }) => {
  const { t } = useTranslation('tasks');
  return (
    <button
      onClick={onClick}
      className="flex items-center gap-1.5 text-sm shrink-0 cursor-pointer hover:underline transition-all"
      aria-label={label}
    >
      <span>{t('diff.filesChanged', { count: fileCount })}</span>
      <span className="opacity-50">•</span>
      <span className="text-green-600 dark:text-green-400">+{added}</span>
      <span className="opacity-50">•</span>
      <span className="text-red-600 dark:text-red-400">-{deleted}</span>
    </button>
  );
};

const ActionButton: React.FC<{
  needsSetup: boolean;
  onRunSetup: () => void;
  onTryAgain: () => void;
  disabled: boolean;
  tryAgainDisabled: boolean;
}> = ({ needsSetup, onRunSetup, onTryAgain, disabled, tryAgainDisabled }) => {
  const { t } = useTranslation('tasks');

  if (needsSetup) {
    return (
      <Button
        variant="default"
        size="sm"
        onClick={onRunSetup}
        disabled={disabled}
        className="text-sm w-full sm:w-auto"
        aria-label={t('attempt.runSetup')}
      >
        {t('attempt.runSetup')}
      </Button>
    );
  }

  return (
    <Button
      variant="destructive"
      size="sm"
      onClick={onTryAgain}
      disabled={tryAgainDisabled}
      className="text-sm w-full sm:w-auto"
      aria-label={t('attempt.tryAgain')}
    >
      {t('attempt.tryAgain')}
    </Button>
  );
};

const DevServerTooltipContent: React.FC<{
  projectHasDevScript: boolean;
  hasRunningDevServer: boolean;
}> = ({ projectHasDevScript, hasRunningDevServer }) => {
  const { t } = useTranslation('tasks');

  if (!projectHasDevScript) {
    return <>{t('attempt.devScriptMissingTooltip')}</>;
  }

  return <>{hasRunningDevServer ? t('attempt.pauseDev') : t('attempt.startDev')}</>;
};

// Helper component for the file action toolbar buttons
function FileActionToolbar({
  containerRef,
  copied,
  handleCopy,
  handleOpenDiffs,
  handleOpenInEditor,
  handleViewLogs,
  handleGitActions,
  attemptId,
  editorName,
  editorType,
  hasRunningDevServer,
  projectHasDevScript,
  isStarting,
  isStopping,
  start,
  stop,
  devServerProcesses,
  t,
}: Readonly<{
  containerRef?: string | null;
  copied: boolean;
  handleCopy: () => void;
  handleOpenDiffs: () => void;
  handleOpenInEditor: () => void;
  handleViewLogs: () => void;
  handleGitActions: () => void;
  attemptId?: string;
  editorName: string;
  editorType?: EditorType | null;
  hasRunningDevServer: boolean;
  projectHasDevScript: boolean;
  isStarting: boolean;
  isStopping: boolean;
  start: () => void;
  stop: () => void;
  devServerProcesses: ExecutionProcess[];
  t: TFunction;
}>) {
  return (
    <div className="flex items-center gap-1 shrink-0 sm:ml-auto">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="sm" className="h-7 w-7 p-0" onClick={handleOpenDiffs} aria-label={t('attempt.diffs')}>
            <GitDiffIcon className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('attempt.diffs')}</TooltipContent>
      </Tooltip>

      {containerRef && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="sm" className="h-7 w-7 p-0" onClick={handleCopy} aria-label={t('attempt.clickToCopy')}>
              {copied ? <CheckIcon className="h-3.5 w-3.5 text-green-600" /> : <CopyIcon className="h-3.5 w-3.5" />}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{copied ? t('attempt.copied') : t('attempt.clickToCopy')}</TooltipContent>
        </Tooltip>
      )}

      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="sm" className="h-7 w-7 p-0" onClick={handleOpenInEditor} disabled={!attemptId} aria-label={t('attempt.openInEditor', { editor: editorName })}>
            <IdeIcon editorType={editorType} className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('attempt.openInEditor', { editor: editorName })}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-block">
            <Button
              variant="ghost" size="sm" className="h-7 w-7 p-0"
              onClick={hasRunningDevServer ? () => stop() : () => start()}
              disabled={(hasRunningDevServer ? isStopping : isStarting) || !attemptId || !projectHasDevScript}
              aria-label={hasRunningDevServer ? t('attempt.pauseDev') : t('attempt.startDev')}
            >
              {hasRunningDevServer ? <PauseIcon className="h-3.5 w-3.5 text-destructive" /> : <PlayIcon className="h-3.5 w-3.5" />}
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <DevServerTooltipContent projectHasDevScript={projectHasDevScript} hasRunningDevServer={hasRunningDevServer} />
        </TooltipContent>
      </Tooltip>

      {devServerProcesses.length > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="sm" className="h-7 w-7 p-0" onClick={handleViewLogs} disabled={!attemptId} aria-label={t('attempt.viewDevLogs')}>
              <TerminalIcon className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('attempt.viewDevLogs')}</TooltipContent>
        </Tooltip>
      )}

      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="sm" className="h-7 w-7 p-0" onClick={handleGitActions} disabled={!attemptId} aria-label={t('attempt.gitActions')}>
            <GitBranchIcon className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('attempt.gitActions')}</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function NextActionCard({
  attemptId,
  sessionId,
  containerRef,
  failed,
  execution_processes,
  task,
  needsSetup,
}: Readonly<NextActionCardProps>) {
  const { t } = useTranslation('tasks');
  const { config } = useUserSystem();
  const { projectId } = useProject();
  const navigate = useNavigate();
  const [copied, setCopied] = useState(false);
  const copyTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (copyTimeoutRef.current !== null) {
        clearTimeout(copyTimeoutRef.current);
      }
    };
  }, []);

  const { data: attempt } = useQuery({
    queryKey: ['attemptWithSession', attemptId],
    queryFn: () => attemptsApi.getWithSession(attemptId!),
    enabled: !!attemptId && failed,
  });
  const { capabilities } = useUserSystem();

  const openInEditor = useOpenInEditor(attemptId);
  const { fileCount, added, deleted, error } = useDiffSummary(
    attemptId ?? null
  );
  const {
    start,
    stop,
    isStarting,
    isStopping,
    runningDevServers,
    devServerProcesses,
  } = useDevServer(attemptId);

  const hasRunningDevServer = runningDevServers.length > 0;

  const { data: projectHasDevScript = false } =
    useHasDevServerScript(projectId);

  const handleCopy = useCallback(async () => {
    if (!containerRef) return;

    try {
      await navigator.clipboard.writeText(containerRef);
      setCopied(true);
      if (copyTimeoutRef.current !== null) {
        clearTimeout(copyTimeoutRef.current);
      }
      copyTimeoutRef.current = setTimeout(() => {
        setCopied(false);
        copyTimeoutRef.current = null;
      }, 2000);
    } catch (err) {
      console.warn('Copy to clipboard failed:', err);
    }
  }, [containerRef]);

  const handleOpenInEditor = useCallback(() => {
    openInEditor();
  }, [openInEditor]);

  const handleViewLogs = useCallback(() => {
    if (sessionId) {
      ViewProcessesDialog.show({
        sessionId,
        initialProcessId: devServerProcesses[0]?.id,
      });
    }
  }, [sessionId, devServerProcesses]);

  const handleOpenDiffs = useCallback(() => {
    navigate({ search: '?view=diffs' });
  }, [navigate]);

  const handleTryAgain = useCallback(() => {
    if (!attempt?.taskId) return;
    CreateAttemptDialog.show({
      taskId: attempt.taskId,
    });
  }, [attempt?.taskId]);

  const handleGitActions = useCallback(() => {
    if (!attemptId) return;
    GitActionsDialog.show({
      attemptId,
      task,
    });
  }, [attemptId, task]);

  const handleRunSetup = useCallback(async () => {
    if (!attemptId || !attempt?.session?.executor) return;
    try {
      await attemptsApi.runAgentSetup(attemptId, {
        executor_profile_id: {
          executor: attempt.session.executor as BaseCodingAgent,
          variant: null,
        },
      });
    } catch (error) {
      console.error('Failed to run setup:', error);
    }
  }, [attemptId, attempt]);

  const canAutoSetup = !!(
    attempt?.session?.executor &&
    capabilities?.[attempt.session.executor]?.includes(
      BaseAgentCapability.SETUP_HELPER
    )
  );

  const setupHelpText = canAutoSetup
    ? t('attempt.setupHelpText', { agent: attempt?.session?.executor })
    : null;

  const editorName = getIdeName(config?.editor?.editor_type);

  const shouldHide =
    (!failed || (execution_processes > 2 && !needsSetup)) && fileCount === 0;

  if (shouldHide) {
    return <div className="h-24"></div>;
  }

  const borderClass = failed ? 'border-destructive' : 'border-foreground';
  const bgClass = failed ? 'bg-destructive' : 'bg-foreground';
  const showActionButton = failed && (needsSetup || execution_processes <= 2);

  return (
    <TooltipProvider>
      <div className="pt-4 pb-8">
        <div className={`px-3 py-1 text-background flex ${bgClass}`}>
          <span className="font-semibold flex-1">
            {t('attempt.labels.summaryAndActions')}
          </span>
        </div>

        {needsSetup && setupHelpText && (
          <div
            className={`border-x border-t ${borderClass} px-3 py-2 flex items-start gap-2`}
          >
            <GearIcon className="h-4 w-4 mt-0.5 flex-shrink-0" />
            <span className="text-sm">{setupHelpText}</span>
          </div>
        )}

        <div
          className={`border px-3 py-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3 min-w-0 ${borderClass} ${needsSetup && setupHelpText ? 'border-t-0' : ''}`}
        >
          {!error && (
            <DiffSummaryButton
              fileCount={fileCount}
              added={added}
              deleted={deleted}
              onClick={handleOpenDiffs}
              label={t('attempt.diffs')}
            />
          )}

          {showActionButton && (
            <ActionButton
              needsSetup={!!needsSetup}
              onRunSetup={handleRunSetup}
              onTryAgain={handleTryAgain}
              disabled={!attempt}
              tryAgainDisabled={!attempt?.taskId}
            />
          )}

          {fileCount > 0 && (
            <FileActionToolbar
              containerRef={containerRef}
              copied={copied}
              handleCopy={handleCopy}
              handleOpenDiffs={handleOpenDiffs}
              handleOpenInEditor={handleOpenInEditor}
              handleViewLogs={handleViewLogs}
              handleGitActions={handleGitActions}
              attemptId={attemptId}
              editorName={editorName}
              editorType={config?.editor?.editor_type}
              hasRunningDevServer={hasRunningDevServer}
              projectHasDevScript={projectHasDevScript}
              isStarting={isStarting}
              isStopping={isStopping}
              start={start}
              stop={stop}
              devServerProcesses={devServerProcesses}
              t={t}
            />
          )}
        </div>
      </div>
    </TooltipProvider>
  );
}
