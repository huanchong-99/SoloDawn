import {
  useRef,
  useEffect,
  useCallback,
  useState,
  useMemo,
  MutableRefObject,
} from 'react';
import { ChangesPanel } from '../views/ChangesPanel';
import { sortDiffs } from '@/utils/fileTreeUtils';
import { useChangesView } from '@/contexts/ChangesViewContext';
import { useWorkspaceContext } from '@/contexts/WorkspaceContext';
import { useTask } from '@/hooks/useTask';
import { useWorkflow, useWorkflowTaskDiff } from '@/hooks/useWorkflows';
import type { Diff, DiffChangeKind } from 'shared/types';

/**
 * FE-2 (Phase C step 11): alternate, orchestration-mode diff source. When set,
 * `ChangesPanelContainer` renders the per-task branch diff (via the Phase-A
 * endpoint) instead of the `Workspace`/worktree-scoped `useWorkspaceContext().diffs`.
 * Either pass `diffs` pre-fetched, or `{ workflowId, taskId }` to fetch lazily.
 */
export type TaskDiffSource =
  | { readonly workflowId: string; readonly taskId: string; readonly diffs?: undefined }
  | { readonly diffs: Diff[]; readonly workflowId?: undefined; readonly taskId?: undefined };

// Auto-collapse defaults based on change type (matches DiffsPanel behavior)
const COLLAPSE_BY_CHANGE_TYPE: Record<DiffChangeKind, boolean> = {
  added: false, // Expand added files
  deleted: true, // Collapse deleted files
  modified: false, // Expand modified files
  renamed: true, // Collapse renamed files
  copied: true, // Collapse copied files
  permissionChange: true, // Collapse permission changes
};

// Collapse large diffs (over 200 lines)
const COLLAPSE_MAX_LINES = 200;

function shouldAutoCollapse(diff: Diff): boolean {
  // Collapse based on change type
  if (COLLAPSE_BY_CHANGE_TYPE[diff.change]) {
    return true;
  }

  // Collapse large diffs
  const totalLines = (diff.additions ?? 0) + (diff.deletions ?? 0);
  if (totalLines > COLLAPSE_MAX_LINES) {
    return true;
  }

  return false;
}

// Hook to observe which diff is currently in view and report it
function useInViewObserver(
  diffRefs: MutableRefObject<Map<string, HTMLDivElement>>,
  containerRef: MutableRefObject<HTMLDivElement | null>,
  onFileInViewChange?: (path: string) => void
) {
  const observerRef = useRef<IntersectionObserver | null>(null);
  const visiblePathsRef = useRef<Set<string>>(new Set());

  // Helper to handle intersection observer entries
  const handleIntersectionEntries = useCallback((entries: IntersectionObserverEntry[]) => {
    entries.forEach((entry) => {
      const path = (entry.target as HTMLElement).dataset.diffPath;
      if (!path) return;

      if (entry.isIntersecting) {
        visiblePathsRef.current.add(path);
      } else {
        visiblePathsRef.current.delete(path);
      }
    });

    // Report the first visible path (topmost in the list)
    if (visiblePathsRef.current.size > 0 && onFileInViewChange) {
      // Get all visible paths and find the one that appears first in the DOM
      const allRefs = diffRefs.current;
      for (const [path] of allRefs) {
        if (visiblePathsRef.current.has(path)) {
          onFileInViewChange(path);
          break;
        }
      }
    }
  }, [diffRefs, onFileInViewChange]);

  useEffect(() => {
    if (!onFileInViewChange) return;

    const createObserver = () => {
      // Disconnect existing observer if any
      observerRef.current?.disconnect();

      // Create observer that tracks which diffs are in the top portion of the container
      observerRef.current = new IntersectionObserver(
        handleIntersectionEntries,
        {
          // Use the scrollable container as root (null = viewport)
          root: containerRef.current,
          // Observe intersection with the top 20% of the container
          rootMargin: '0px 0px -80% 0px',
          threshold: 0,
        }
      );

      // Re-observe all currently registered elements
      diffRefs.current.forEach((el) => {
        observerRef.current?.observe(el);
      });
    };

    // Create observer once container is available
    if (containerRef.current) {
      createObserver();
    }

    return () => {
      observerRef.current?.disconnect();
    };
  }, [onFileInViewChange, diffRefs, containerRef, handleIntersectionEntries]);

  // Callback to observe/unobserve elements
  const observeElement = useCallback(
    (el: HTMLDivElement | null, path: string) => {
      if (!observerRef.current) return;

      // Unobserve previous element with this path if it exists
      const existingEl = diffRefs.current.get(path);
      if (existingEl) {
        observerRef.current.unobserve(existingEl);
      }

      if (el) {
        el.dataset.diffPath = path;
        observerRef.current.observe(el);
      }
    },
    [diffRefs]
  );

  return observeElement;
}

interface ChangesPanelContainerProps {
  className?: string;
  /** Attempt ID for opening files in IDE */
  attemptId?: string;
  /**
   * FE-2: orchestration-mode diff source. When provided, the panel renders
   * these per-task branch diffs instead of `useWorkspaceContext().diffs`.
   */
  taskDiffSource?: TaskDiffSource;
}

export function ChangesPanelContainer({
  className,
  attemptId,
  taskDiffSource,
}: Readonly<ChangesPanelContainerProps>) {
  const { diffs: workspaceDiffs, workspace } = useWorkspaceContext();
  const { data: task } = useTask(workspace?.taskId, {
    enabled: !!workspace?.taskId,
  });

  // FE-2: when a task diff source is supplied (orchestration mode), fetch the
  // per-task branch diff (unless pre-fetched diffs were passed) and use it +
  // the workflow's projectId for @-mentions instead of the workspace-scoped data.
  const isOrchestrationSource = !!taskDiffSource;
  const sourceWorkflowId = taskDiffSource?.workflowId;
  const sourceTaskId = taskDiffSource?.taskId;
  const { data: fetchedTaskDiffs } = useWorkflowTaskDiff(
    sourceWorkflowId ?? null,
    sourceTaskId ?? null,
    { enabled: !!sourceWorkflowId && !!sourceTaskId && !taskDiffSource?.diffs }
  );
  const { data: sourceWorkflow } = useWorkflow(sourceWorkflowId ?? '', {
    retry: false,
  });

  const diffs = useMemo(
    () =>
      isOrchestrationSource
        ? (taskDiffSource?.diffs ?? fetchedTaskDiffs ?? [])
        : workspaceDiffs,
    [isOrchestrationSource, taskDiffSource?.diffs, fetchedTaskDiffs, workspaceDiffs]
  );
  const projectId = isOrchestrationSource
    ? sourceWorkflow?.projectId
    : task?.projectId;
  const { selectedFilePath, setFileInView } = useChangesView();
  const diffRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const containerRef = useRef<HTMLDivElement | null>(null);
  // Track which diffs we've processed for auto-collapse
  const [processedPaths] = useState(() => new Set<string>());

  // Set up intersection observer to track which file is in view
  const observeElement = useInViewObserver(
    diffRefs,
    containerRef,
    setFileInView
  );

  useEffect(() => {
    if (!selectedFilePath) return;

    // Defer to next frame to ensure ref is attached after render
    const timeoutId = setTimeout(() => {
      diffRefs.current.get(selectedFilePath)?.scrollIntoView({
        behavior: 'smooth',
        block: 'start',
      });
    }, 0);

    return () => clearTimeout(timeoutId);
  }, [selectedFilePath]);

  const handleDiffRef = useCallback(
    (path: string, el: HTMLDivElement | null) => {
      if (el) {
        diffRefs.current.set(path, el);
      } else {
        diffRefs.current.delete(path);
      }
      // Also observe/unobserve for intersection tracking
      observeElement(el, path);
    },
    [observeElement]
  );

  // Compute initial expanded state, but pass the Diff directly for stable references
  // Sort diffs to match FileTree ordering
  const diffItems = useMemo(() => {
    return sortDiffs(diffs).map((diff) => {
      const path = diff.newPath || diff.oldPath || '';

      // Determine initial expanded state for new diffs
      let initialExpanded = true;
      if (!processedPaths.has(path)) {
        processedPaths.add(path);
        initialExpanded = !shouldAutoCollapse(diff);
      }

      return { diff, initialExpanded };
    });
  }, [diffs, processedPaths]);

  return (
    <ChangesPanel
      ref={containerRef}
      className={className}
      diffItems={diffItems}
      onDiffRef={handleDiffRef}
      projectId={projectId}
      attemptId={attemptId}
    />
  );
}
