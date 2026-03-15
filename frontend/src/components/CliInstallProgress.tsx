import React, { useEffect, useRef, useCallback } from 'react';
import { Check, X } from 'lucide-react';
import {
  useCliInstallProgress,
  type InstallLogLine,
} from '@/hooks/useCliInstallProgress';

// ============================================================================
// Types
// ============================================================================

interface CliInstallProgressProps {
  readonly cliTypeId: string;
  readonly jobId: string;
  readonly onComplete?: (success: boolean) => void;
}

// ============================================================================
// Helpers
// ============================================================================

function getLineColor(type: InstallLogLine['type']): string {
  switch (type) {
    case 'stdout':
      return 'text-gray-200';
    case 'stderr':
      return 'text-yellow-400';
    case 'error':
      return 'text-red-400';
    case 'completed':
      return 'text-green-400';
  }
}

// ============================================================================
// Component
// ============================================================================

/**
 * Terminal-like display for real-time CLI install/uninstall progress.
 * Connects via WebSocket and auto-scrolls as new log lines arrive.
 */
export function CliInstallProgress({
  cliTypeId,
  jobId,
  onComplete,
}: CliInstallProgressProps) {
  const { lines, isComplete, exitCode, error } = useCliInstallProgress(
    cliTypeId,
    jobId
  );
  const scrollRef = useRef<HTMLDivElement>(null);
  const onCompleteRef = useRef(onComplete);
  onCompleteRef.current = onComplete;

  // Auto-scroll to bottom when new lines arrive
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [lines.length]);

  // Notify parent when complete
  const hasNotifiedRef = useRef(false);
  const notifyComplete = useCallback(
    (success: boolean) => {
      if (!hasNotifiedRef.current) {
        hasNotifiedRef.current = true;
        onCompleteRef.current?.(success);
      }
    },
    []
  );

  useEffect(() => {
    if (isComplete) {
      const success = error === null && (exitCode === null || exitCode === 0);
      notifyComplete(success);
    }
  }, [isComplete, exitCode, error, notifyComplete]);

  return (
    <div className="rounded border bg-gray-950 font-ibm-plex-mono text-sm">
      {/* Terminal header */}
      <div className="flex items-center justify-between border-b px-3 py-1.5">
        <span className="text-xs text-gray-400">
          {isComplete ? 'Completed' : 'Running...'}
        </span>
        {isComplete && (
          <span className="flex items-center gap-1 text-xs">
            {error === null && (exitCode === null || exitCode === 0) ? (
              <>
                <Check className="h-3 w-3 text-green-400" />
                <span className="text-green-400">
                  Exit code: {exitCode ?? 0}
                </span>
              </>
            ) : (
              <>
                <X className="h-3 w-3 text-red-400" />
                <span className="text-red-400">
                  {error ?? `Exit code: ${exitCode}`}
                </span>
              </>
            )}
          </span>
        )}
      </div>

      {/* Terminal output */}
      <div
        ref={scrollRef}
        className="max-h-64 overflow-y-auto p-3"
      >
        {lines.length === 0 && !isComplete && (
          <div className="text-gray-500">Waiting for output...</div>
        )}
        {lines.map((line, index) => (
          <div
            key={index}
            className={`whitespace-pre-wrap break-all text-xs leading-5 ${getLineColor(line.type)}`}
          >
            {line.content}
          </div>
        ))}
      </div>
    </div>
  );
}
