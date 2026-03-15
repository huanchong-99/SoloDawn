import { useDiffStream } from '@/hooks/useDiffStream';
import { useMemo, useCallback, useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Loader } from '@/components/ui/loader';
import { Button } from '@/components/ui/button';
import DiffViewSwitch from '@/components/DiffViewSwitch';
import DiffCard from '@/components/DiffCard';
import { useDiffSummary } from '@/hooks/useDiffSummary';
import { NewCardHeader } from '@/components/ui/new-card';
import { ChevronsUp, ChevronsDown } from 'lucide-react';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import type { Diff, DiffChangeKind, Workspace } from 'shared/types';
import GitOperations, {
  type GitOperationsInputs,
} from '@/components/tasks/Toolbar/GitOperations.tsx';

interface DiffsPanelProps {
  selectedAttempt: Workspace | null;
  gitOps?: GitOperationsInputs;
}

type DiffCollapseDefaults = Record<DiffChangeKind, boolean>;

const DEFAULT_DIFF_COLLAPSE_DEFAULTS: DiffCollapseDefaults = {
  added: false,
  deleted: true,
  modified: false,
  renamed: true,
  copied: true,
  permissionChange: true,
};

const DEFAULT_COLLAPSE_MAX_LINES = 200;

const exceedsMaxLineCount = (d: Diff, maxLines: number): boolean => {
  if (d.additions != null || d.deletions != null)
    return (d.additions ?? 0) + (d.deletions ?? 0) > maxLines;

  return true;
};

const getDiffId = ({ diff, index }: Readonly<{ diff: Diff; index: number }>) =>
  `${diff.newPath || diff.oldPath || index}`;

export function DiffsPanel({ selectedAttempt, gitOps }: Readonly<DiffsPanelProps>) {
  const { t } = useTranslation('tasks');
  const [loadingState, setLoadingState] = useState<
    'loading' | 'loaded' | 'timed-out'
  >('loading');
  const [collapsedIds, setCollapsedIds] = useState<Set<string>>(new Set());
  const [processedIds, setProcessedIds] = useState<Set<string>>(new Set());
  const { diffs, error } = useDiffStream(selectedAttempt?.id ?? null, true);

  const attemptId = selectedAttempt?.id;
  useEffect(() => {
    setCollapsedIds(new Set());
    setProcessedIds(new Set());
  }, [attemptId]);
  const { fileCount, added, deleted } = useDiffSummary(
    selectedAttempt?.id ?? null
  );

  // If no diffs arrive within 3 seconds, stop showing the spinner
  useEffect(() => {
    if (loadingState !== 'loading') return;
    const timer = setTimeout(() => setLoadingState('timed-out'), 3000);
    return () => clearTimeout(timer);
  }, [loadingState]);

  useEffect(() => {
    if (diffs.length === 0 || loadingState !== 'loading') return;
    setLoadingState('loaded');
  }, [diffs.length, loadingState]);

  useEffect(() => {
    if (diffs.length === 0) return;

    const newDiffs = diffs
      .map((d, index) => ({ diff: d, index }))
      .filter((entry) => !processedIds.has(getDiffId(entry)));

    if (newDiffs.length === 0) return;

    const newIds = newDiffs.map(getDiffId);
    const toCollapse = newDiffs
      .filter(
        ({ diff }) =>
          DEFAULT_DIFF_COLLAPSE_DEFAULTS[diff.change] ||
          exceedsMaxLineCount(diff, DEFAULT_COLLAPSE_MAX_LINES)
      )
      .map(getDiffId);

    setProcessedIds((prev) => new Set([...prev, ...newIds]));
    if (toCollapse.length > 0) {
      setCollapsedIds((prev) => new Set([...prev, ...toCollapse]));
    }
  }, [diffs, processedIds]);

  const loading = loadingState === 'loading';

  const ids = useMemo(() => {
    return diffs.map((d, i) => getDiffId({ diff: d, index: i }));
  }, [diffs]);

  const toggle = useCallback((id: string) => {
    setCollapsedIds((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }, []);

  const allCollapsed = diffs.length > 0 && diffs.every((d, i) => collapsedIds.has(getDiffId({ diff: d, index: i })));
  const handleCollapseAll = useCallback(() => {
    setCollapsedIds(allCollapsed ? new Set() : new Set(ids));
  }, [allCollapsed, ids]);

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4 m-4">
        <div className="text-red-800 text-sm">
          {t('diff.errorLoadingDiff', { error })}
        </div>
      </div>
    );
  }

  return (
    <DiffsPanelContent
      diffs={diffs}
      fileCount={fileCount}
      added={added}
      deleted={deleted}
      collapsedIds={collapsedIds}
      allCollapsed={allCollapsed}
      handleCollapseAll={handleCollapseAll}
      toggle={toggle}
      selectedAttempt={selectedAttempt}
      gitOps={gitOps}
      loading={loading}
      t={t}
    />
  );
}

interface DiffsPanelContentProps {
  diffs: Diff[];
  fileCount: number;
  added: number;
  deleted: number;
  collapsedIds: Set<string>;
  allCollapsed: boolean;
  handleCollapseAll: () => void;
  toggle: (id: string) => void;
  selectedAttempt: Workspace | null;
  gitOps?: GitOperationsInputs;
  loading: boolean;
  t: (key: string, params?: Record<string, unknown>) => string;
}

// Content renderer component
function DiffListContent({
  loading,
  diffs,
  collapsedIds,
  toggle,
  selectedAttempt,
  t,
}: Readonly<{
  loading: boolean;
  diffs: Diff[];
  collapsedIds: Set<string>;
  toggle: (id: string) => void;
  selectedAttempt: Workspace | null;
  t: (key: string, params?: Record<string, unknown>) => string;
}>) {
  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader />
      </div>
    );
  }

  if (diffs.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
        {t('diff.noChanges')}
      </div>
    );
  }

  return (
    <>
      {diffs.map((diff, idx) => {
        const id = diff.newPath || diff.oldPath || String(idx);
        return (
          <DiffCard
            key={id}
            diff={diff}
            expanded={!collapsedIds.has(id)}
            onToggle={() => toggle(id)}
            selectedAttempt={selectedAttempt}
          />
        );
      })}
    </>
  );
}

function DiffsPanelContent({
  diffs,
  fileCount,
  added,
  deleted,
  collapsedIds,
  allCollapsed,
  handleCollapseAll,
  toggle,
  selectedAttempt,
  gitOps,
  loading,
  t,
}: Readonly<DiffsPanelContentProps>) {
  return (
    <div className="h-full flex flex-col relative">
      {diffs.length > 0 && (
        <NewCardHeader
          className="sticky top-0 z-10"
          actions={
            <>
              <DiffViewSwitch />
              <div className="h-4 w-px bg-border" />
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="icon"
                      onClick={handleCollapseAll}
                      aria-pressed={allCollapsed}
                      aria-label={
                        allCollapsed
                          ? t('diff.expandAll')
                          : t('diff.collapseAll')
                      }
                    >
                      {allCollapsed
                        ? <ChevronsDown className="h-4 w-4" />
                        : <ChevronsUp className="h-4 w-4" />}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {allCollapsed ? t('diff.expandAll') : t('diff.collapseAll')}
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </>
          }
        >
          <div className="flex items-center">
            <span
              className="text-sm text-muted-foreground whitespace-nowrap"
              aria-live="polite"
            >
              {t('diff.filesChanged', { count: fileCount })}{' '}
              <span className="text-green-600 dark:text-green-500">
                +{added}
              </span>{' '}
              <span className="text-red-600 dark:text-red-500">-{deleted}</span>
            </span>
          </div>
        </NewCardHeader>
      )}
      {gitOps && selectedAttempt && (
        <div className="px-3">
          <GitOperations selectedAttempt={selectedAttempt} {...gitOps} />
        </div>
      )}
      <div className="flex-1 overflow-y-auto px-3">
        <DiffListContent
          loading={loading}
          diffs={diffs}
          collapsedIds={collapsedIds}
          toggle={toggle}
          selectedAttempt={selectedAttempt}
          t={t}
        />
      </div>
    </div>
  );
}
