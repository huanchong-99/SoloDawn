import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { TerminalEmulator, type TerminalEmulatorRef } from './TerminalEmulator';
import { QualityBadge, type GateStatus } from '@/components/workflow/QualityBadge';
import { QualityReportPanel } from '@/components/quality/QualityReportPanel';
import { useTerminalLatestQuality } from '@/hooks/useQualityGate';
import { Dialog, DialogContent } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { Terminal } from '@/components/workflow/TerminalCard';
import type { WorkflowTask } from '@/components/workflow/PipelineView';
import { useTranslation } from 'react-i18next';
import { stripAnsi } from 'fancy-ansi';

interface Props {
  tasks: (WorkflowTask & { terminals: Terminal[] })[];
  wsUrl: string;
}

interface TerminalLogEntry {
  id: string;
  content: string;
}

interface TerminalHistoryState {
  loading: boolean;
  loaded: boolean;
  lines: string[];
  error: string | null;
}

const TERMINAL_HISTORY_LIMIT = 1000;
const HISTORY_PAGE_SIZE = 200;

const stripControlCharacters = (value: string): string =>
  Array.from(value)
    .filter((char) => {
      const code = char.codePointAt(0);
      if (code === undefined) {
        return false;
      }
      return code === 0x09 || code === 0x0a || code === 0x0d || (code >= 0x20 && code !== 0x7f);
    })
    .join('');

const sanitizeTerminalHistoryContent = (content: string) =>
  stripControlCharacters(
    stripAnsi(content)
    .replaceAll('\r\n', '\n')
    .replaceAll('\r', '\n')
  );

/**
 * Renders the terminal debugging UI with a terminal list and active emulator.
 */
export function TerminalDebugView({ tasks, wsUrl }: Readonly<Props>) {
  const { t } = useTranslation('workflow');
  const [selectedTerminalId, setSelectedTerminalId] = useState<string | null>(null);
  const [isQualityPanelOpen, setIsQualityPanelOpen] = useState(false);
  const [historyByTerminalId, setHistoryByTerminalId] = useState<Record<string, TerminalHistoryState>>({});
  // G28-006: readyTerminalIds, startingTerminalIds, and needsRestartIds affect
  // shouldRenderLiveTerminal which drives rendering. Promote to useState so the
  // UI re-renders when these change. autoStartedRef and restartAttemptsRef remain
  // refs because they only gate imperative logic (API calls), not rendering.
  const [readyTerminalIds, setReadyTerminalIds] = useState<Set<string>>(new Set());
  const [startingTerminalIds, setStartingTerminalIds] = useState<Set<string>>(new Set());
  const [needsRestartIds, setNeedsRestartIds] = useState<Set<string>>(new Set());
  const terminalRef = useRef<TerminalEmulatorRef | null>(null);
  const prevTerminalIdRef = useRef<string | null>(null);
  const autoStartedRef = useRef<Set<string>>(new Set());
  const restartAttemptsRef = useRef<Map<string, number>>(new Map());
  const [historyPage, setHistoryPage] = useState(0);
  const MAX_RESTART_ATTEMPTS = 3;
  const defaultRoleLabel = t('terminalCard.defaultRole');

  // G28-009: tasks is a new array reference on every parent render. Consider
  // memoizing the parent's tasks prop or using a stable selector to avoid
  // unnecessary recomputation here.
  const allTerminals = useMemo(
    () => tasks.flatMap((task) =>
      task.terminals.map((terminal) => ({ ...terminal, taskName: task.name }))
    ),
    [tasks]
  );

  useEffect(() => {
    if (allTerminals.length === 0) {
      if (selectedTerminalId !== null) {
        setSelectedTerminalId(null);
      }
      return;
    }

    if (!selectedTerminalId) {
      setSelectedTerminalId(allTerminals[0].id);
      return;
    }

    const selectedStillExists = allTerminals.some(
      (terminal) => terminal.id === selectedTerminalId
    );
    if (!selectedStillExists) {
      setSelectedTerminalId(allTerminals[0].id);
    }
  }, [allTerminals, selectedTerminalId]);

  // G28-005: Explicitly close the previous terminal's WS connection before
  // switching to avoid race conditions where the old connection lingers.
  useEffect(() => {
    if (prevTerminalIdRef.current && prevTerminalIdRef.current !== selectedTerminalId) {
      // The TerminalEmulator unmounts via key change, but we also reset the ref
      // to ensure no stale reference is held.
      terminalRef.current = null;
    }
    prevTerminalIdRef.current = selectedTerminalId;
    // Reset history page when switching terminals
    setHistoryPage(0);
  }, [selectedTerminalId]);

  const selectedTerminal = allTerminals.find((terminal) => terminal.id === selectedTerminalId);

  const getTerminalLabel = (terminal: Terminal) => {
    const role = terminal.role?.trim();
    return role || `${defaultRoleLabel} ${terminal.orderIndex + 1}`;
  };

  const getStatusLabel = (status: Terminal['status']) =>
    t(`terminalDebug.status.${status}`, { defaultValue: status });

  const isHistoricalTerminalStatus = useCallback((status: Terminal['status'] | undefined) => {
    if (!status) {
      return false;
    }

    return ['completed', 'failed'].includes(status);
  }, []);

  const shouldRenderLiveTerminal = useCallback(
    (terminal: Terminal | undefined) => {
      if (!terminal) {
        return false;
      }

      if (isHistoricalTerminalStatus(terminal.status)) {
        return false;
      }

      // Backend "waiting" means process is ready and can be attached directly.
      // Trust backend status over local transient flags to avoid getting stuck
      // in a perpetual "starting" placeholder after refresh/reconnect.
      if (terminal.status === 'waiting') {
        return true;
      }

      if (needsRestartIds.has(terminal.id)) {
        return false;
      }

      if (startingTerminalIds.has(terminal.id)) {
        return false;
      }

      if (readyTerminalIds.has(terminal.id)) {
        return true;
      }

      // G09-002 / G28-002: Only 'working' terminals should render live.
      // 'not_started' terminals haven't launched yet.
      return terminal.status === 'working';
    },
    [isHistoricalTerminalStatus, needsRestartIds, startingTerminalIds, readyTerminalIds]
  );

  const loadTerminalHistory = useCallback(
    async (terminalId: string) => {
      setHistoryByTerminalId((prev) => ({
        ...prev,
        [terminalId]: {
          loading: true,
          loaded: false,
          lines: prev[terminalId]?.lines ?? [],
          error: null,
        },
      }));

      try {
        const response = await fetch(`/api/terminals/${terminalId}/logs?limit=${TERMINAL_HISTORY_LIMIT}`);
        const payload = await response.json().catch(() => null);

        if (!response.ok) {
          let message = '';
          if (payload && typeof payload === 'object' && 'message' in payload) {
            const messageValue = (payload as { message?: unknown }).message;
            if (typeof messageValue === 'string') {
              message = messageValue;
            } else if (messageValue instanceof Error) {
              message = messageValue.message;
            }
          }

          throw new Error(message || `Failed to load terminal history (${response.status})`);
        }

        let entries: TerminalLogEntry[] = [];
        if (payload && typeof payload === 'object' && 'data' in payload) {
          const dataValue = (payload as { data?: unknown }).data;
          if (Array.isArray(dataValue)) {
            entries = dataValue as TerminalLogEntry[];
          }
        }

        const lines = entries
          .map((entry) => (typeof entry.content === 'string' ? entry.content : ''))
          .filter((line) => line.length > 0);

        setHistoryByTerminalId((prev) => ({
          ...prev,
          [terminalId]: {
            loading: false,
            loaded: true,
            lines,
            error: null,
          },
        }));
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to load terminal history';
        setHistoryByTerminalId((prev) => ({
          ...prev,
          [terminalId]: {
            loading: false,
            loaded: true,
            lines: prev[terminalId]?.lines ?? [],
            error: message,
          },
        }));
      }
    },
    []
  );

  const handleClear = () => {
    terminalRef.current?.clear();
  };

  const resetAutoStart = useCallback((terminalId: string) => {
    autoStartedRef.current.delete(terminalId);
  }, []);

  const startTerminal = useCallback(async (terminalId: string, retryAfterStop = false) => {
    // Allow multiple terminals to start in parallel
    setStartingTerminalIds((prev) => {
      if (prev.has(terminalId)) return prev;
      const next = new Set(prev);
      next.add(terminalId);
      return next;
    });
    // Mark as auto-started only after confirming we can start
    autoStartedRef.current.add(terminalId);
    try {
      const response = await fetch(`/api/terminals/${terminalId}/start`, {
        method: 'POST',
      });

      if (response.ok) {
        console.log('Terminal started successfully');
        // Mark this terminal as ready and clear restart flag
        setNeedsRestartIds((prev) => {
          if (!prev.has(terminalId)) return prev;
          const next = new Set(prev);
          next.delete(terminalId);
          return next;
        });
        setReadyTerminalIds((prev) => {
          if (prev.has(terminalId)) return prev;
          const next = new Set(prev);
          next.add(terminalId);
          return next;
        });
        // Note: Don't reset restart attempts here - only reset on manual restart
        // This prevents infinite loops when API succeeds but process doesn't actually start
      } else {
        const error = await response.json().catch(() => null);

        // Handle 409 Conflict by stopping first, then retrying
        if (response.status === 409 && !retryAfterStop) {
          console.log('Terminal conflict, stopping and retrying...');
          setStartingTerminalIds((prev) => {
            if (!prev.has(terminalId)) return prev;
            const next = new Set(prev);
            next.delete(terminalId);
            return next;
          });
          try {
            await fetch(`/api/terminals/${terminalId}/stop`, { method: 'POST' });
          } catch {
            // Ignore stop errors
          }
          // Retry start after stop
          return startTerminal(terminalId, true);
        }

        console.error('Failed to start terminal:', error);
        resetAutoStart(terminalId);
        // Clear ready state on failure
        setReadyTerminalIds((prev) => {
          if (!prev.has(terminalId)) return prev;
          const next = new Set(prev);
          next.delete(terminalId);
          return next;
        });
      }
    } catch (error) {
      console.error('Failed to start terminal:', error);
      resetAutoStart(terminalId);
      // Clear ready state on failure
      setReadyTerminalIds((prev) => {
        if (!prev.has(terminalId)) return prev;
        const next = new Set(prev);
        next.delete(terminalId);
        return next;
      });
    } finally {
      setStartingTerminalIds((prev) => {
        if (!prev.has(terminalId)) return prev;
        const next = new Set(prev);
        next.delete(terminalId);
        return next;
      });
    }
  }, [resetAutoStart]);

  const shouldAutoRecoverTerminal = useCallback((status: Terminal['status'] | undefined) => {
    if (!status) {
      return false;
    }

    return ['starting', 'waiting', 'working'].includes(status);
  }, []);

  // Helper to check if restart should be skipped
  const shouldSkipRestart = useCallback((terminalId: string, status: Terminal['status'] | undefined) => {
    if (!shouldAutoRecoverTerminal(status)) {
      console.info(
        `Skip auto-restart for terminal ${terminalId} because status is ${status ?? 'unknown'}`
      );
      return true;
    }

    if (startingTerminalIds.has(terminalId)) {
      console.info(
        `Skip auto-restart for terminal ${terminalId} because restart is already in progress`
      );
      return true;
    }

    return false;
  }, [shouldAutoRecoverTerminal, startingTerminalIds]);

  // Helper to handle max restart attempts reached
  const handleMaxRestartAttemptsReached = useCallback((terminalId: string) => {
    console.error(`Max restart attempts (${MAX_RESTART_ATTEMPTS}) reached for terminal ${terminalId}`);
    setNeedsRestartIds((prev) => {
      if (prev.has(terminalId)) return prev;
      const next = new Set(prev);
      next.add(terminalId);
      return next;
    });
    setReadyTerminalIds((prev) => {
      if (!prev.has(terminalId)) return prev;
      const next = new Set(prev);
      next.delete(terminalId);
      return next;
    });
  }, []);

  // Helper to perform terminal restart
  const performTerminalRestart = useCallback((terminalId: string, attempts: number) => {
    console.log(`Terminal process not running, auto-restarting... (attempt ${attempts + 1}/${MAX_RESTART_ATTEMPTS})`);
    restartAttemptsRef.current.set(terminalId, attempts + 1);
    setNeedsRestartIds((prev) => {
      if (prev.has(terminalId)) return prev;
      const next = new Set(prev);
      next.add(terminalId);
      return next;
    });
    setReadyTerminalIds((prev) => {
      if (!prev.has(terminalId)) return prev;
      const next = new Set(prev);
      next.delete(terminalId);
      return next;
    });
    autoStartedRef.current.delete(terminalId);
    startTerminal(terminalId);
  }, [startTerminal]);

  // Handle terminal errors - auto-restart if process is not running
  const handleTerminalError = useCallback((error: Error) => {
    console.error('Terminal error:', error.message);

    const isProcessNotRunning = error.message.includes('Terminal process not running');
    if (!isProcessNotRunning || !selectedTerminalId) return;

    if (shouldSkipRestart(selectedTerminalId, selectedTerminal?.status)) return;

    const attempts = restartAttemptsRef.current.get(selectedTerminalId) || 0;
    if (attempts >= MAX_RESTART_ATTEMPTS) {
      handleMaxRestartAttemptsReached(selectedTerminalId);
      return;
    }

    performTerminalRestart(selectedTerminalId, attempts);
  }, [selectedTerminal?.status, selectedTerminalId, shouldSkipRestart, handleMaxRestartAttemptsReached, performTerminalRestart]);

  // Auto-start terminal when selected and not yet started
  useEffect(() => {
    const selectedStatus = selectedTerminal?.status;
    if (!selectedTerminalId || !selectedStatus) return;

    // Only auto-start if terminal is not started and hasn't been auto-started before
    if (selectedStatus !== 'not_started') return;
    if (autoStartedRef.current.has(selectedTerminalId)) return;

    // Note: autoStartedRef is updated inside startTerminal after confirming it can start
    startTerminal(selectedTerminalId);
  }, [selectedTerminalId, selectedTerminal?.status, startTerminal]);

  // Clear ready state and autoStarted when terminal status changes to failed or not_started
  useEffect(() => {
    if (!selectedTerminalId || !selectedTerminal?.status) return;

    if (['failed', 'not_started'].includes(selectedTerminal.status)) {
      setReadyTerminalIds((prev) => {
        if (!prev.has(selectedTerminalId)) return prev;
        const next = new Set(prev);
        next.delete(selectedTerminalId);
        return next;
      });
      // Allow re-auto-start when status returns to not_started
      if (selectedTerminal.status === 'not_started') {
        autoStartedRef.current.delete(selectedTerminalId);
      }
    }
  }, [selectedTerminalId, selectedTerminal?.status]);

  useEffect(() => {
    if (!selectedTerminalId || !selectedTerminal) {
      return;
    }

    if (!isHistoricalTerminalStatus(selectedTerminal.status)) {
      return;
    }

    const existingHistory = historyByTerminalId[selectedTerminalId];
    if (existingHistory?.loading || existingHistory?.loaded) {
      return;
    }

    loadTerminalHistory(selectedTerminalId);
  }, [
    historyByTerminalId,
    isHistoricalTerminalStatus,
    loadTerminalHistory,
    selectedTerminal,
    selectedTerminalId,
  ]);

  const handleRestart = async () => {
    if (!selectedTerminalId) return;
    // Reset restart attempts when user manually restarts
    restartAttemptsRef.current.delete(selectedTerminalId);
    await startTerminal(selectedTerminalId);
  };

  const isHistoricalTerminal = isHistoricalTerminalStatus(selectedTerminal?.status);
  const currentHistory = selectedTerminalId ? historyByTerminalId[selectedTerminalId] : undefined;

  // G09-012 / G28-007: Paginate history lines instead of rendering all at once.
  // Sanitize each line individually for segmented rendering.
  const sanitizedHistoryLines = useMemo(() => {
    if (!currentHistory?.lines.length) return [];
    return currentHistory.lines.map((line) => sanitizeTerminalHistoryContent(line));
  }, [currentHistory?.lines]);

  const totalHistoryPages = Math.max(1, Math.ceil(sanitizedHistoryLines.length / HISTORY_PAGE_SIZE));
  const pagedHistoryLines = useMemo(() => {
    const start = historyPage * HISTORY_PAGE_SIZE;
    return sanitizedHistoryLines.slice(start, start + HISTORY_PAGE_SIZE);
  }, [sanitizedHistoryLines, historyPage]);

  return (
    <div className="flex h-full">
      <div className="w-64 border-r bg-muted/30 overflow-y-auto">
        <div className="p-4 border-b">
          <h3 className="font-semibold">{t('terminalDebug.listTitle')}</h3>
        </div>
        {allTerminals.length === 0 ? (
          <div className="p-4 text-sm text-muted-foreground">
            <div className="font-medium text-foreground">
              {t('terminalDebug.emptyTitle')}
            </div>
            <div className="mt-2">{t('terminalDebug.emptyDescription')}</div>
          </div>
        ) : (
          <ul className="p-2">
            {allTerminals.map((terminal) => {
              const terminalLabel = getTerminalLabel(terminal);
              const statusLabel = getStatusLabel(terminal.status);

              return (
                <li key={terminal.id}>
                  <button
                    aria-label={`${terminalLabel} - ${statusLabel}`}
                    aria-current={selectedTerminalId === terminal.id ? 'true' : 'false'}
                    className={cn(
                      'w-full p-3 rounded-lg text-left mb-2 transition-colors',
                      selectedTerminalId === terminal.id
                        ? 'bg-primary text-primary-foreground'
                        : 'hover:bg-muted'
                    )}
                    onClick={() => {
                      setSelectedTerminalId(terminal.id);
                    }}
                  >
                    <div className="font-medium text-sm">{terminalLabel}</div>
                    <div className="text-xs opacity-70">{terminal.taskName}</div>
                    <div className="flex flex-col gap-1.5 mt-1.5">
                      <div className="flex items-center gap-2">
                        <StatusDot status={terminal.status} />
                        <span className="text-xs">{statusLabel}</span>
                      </div>
                      <div className="mt-0.5">
                        <TerminalQualityBadgeInline terminalId={terminal.id} />
                      </div>
                    </div>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div className="flex-1 flex flex-col">
        {selectedTerminal ? (
          <>
            <div className="p-4 border-b flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div>
                  <h3 className="font-semibold flex items-center gap-2">
                    {getTerminalLabel(selectedTerminal)}
                  </h3>
                  <p className="text-sm text-muted-foreground">
                    {selectedTerminal.cliTypeId} - {selectedTerminal.modelConfigId}
                  </p>
                </div>
                <div>
                  <button type="button" className="appearance-none bg-transparent border-none p-0 m-0" onClick={() => setIsQualityPanelOpen(true)}>
                    <TerminalQualityBadgeInline terminalId={selectedTerminal.id} className="cursor-pointer hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors" />
                  </button>
                  <Dialog open={isQualityPanelOpen} onOpenChange={setIsQualityPanelOpen}>
                    <DialogContent className="sm:max-w-[700px] max-h-[85vh] overflow-hidden flex flex-col border-slate-200 dark:border-slate-800">
                      <QualityReportPanel terminalId={selectedTerminal.id} className="flex-1 overflow-y-auto pr-2 mt-2" />
                    </DialogContent>
                  </Dialog>
                </div>
              </div>
              <div className="flex gap-2">
                <Button variant="outline" size="sm" onClick={handleClear}>
                  {t('terminalDebug.clear')}
                </Button>
                <Button variant="outline" size="sm" onClick={handleRestart}>
                  {t('terminalDebug.restart')}
                </Button>
              </div>
            </div>
            <div className="flex-1 min-h-0 p-4">
              {(() => {
                if (shouldRenderLiveTerminal(selectedTerminal)) {
                  return (
                    <TerminalEmulator
                      key={selectedTerminal.id}
                      ref={terminalRef}
                      terminalId={selectedTerminal.id}
                      wsUrl={wsUrl}
                      onError={handleTerminalError}
                    />
                  );
                } else if (isHistoricalTerminal) {
                  return (
                    <div className="h-full min-h-0 rounded-lg border bg-background p-4 flex flex-col">
                      <div className="text-sm text-muted-foreground mb-3">
                        {t('terminalDebug.historyTitle', {
                          defaultValue: 'Terminal history',
                        })}
                      </div>

                      {(() => {
                        if (currentHistory?.loading) {
                          return (
                            <div className="text-sm text-muted-foreground">
                              {t('terminalDebug.historyLoading', {
                                defaultValue: 'Loading terminal history...',
                              })}
                            </div>
                          );
                        } else if (currentHistory?.error) {
                          return (
                            <div className="space-y-3">
                              <div className="text-sm text-red-500">
                                {t('terminalDebug.historyLoadFailed', {
                                  defaultValue: 'Failed to load terminal history.',
                                })}
                              </div>
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => {
                                  if (!selectedTerminalId) {
                                    return;
                                  }
                                  loadTerminalHistory(selectedTerminalId);
                                }}
                              >
                                {t('terminalDebug.reloadHistory', {
                                  defaultValue: 'Reload history',
                                })}
                              </Button>
                            </div>
                          );
                        } else if (currentHistory?.lines.length) {
                          // G09-012 / G28-007: Render paginated segments instead of a single <pre>
                          return (
                            <div className="flex-1 min-h-0 flex flex-col">
                              <div className="flex-1 min-h-0 overflow-y-auto overflow-x-auto pr-1">
                                {pagedHistoryLines.map((line, idx) => (
                                  <pre
                                    key={historyPage * HISTORY_PAGE_SIZE + idx}
                                    className="text-xs leading-5 whitespace-pre-wrap break-words text-foreground"
                                  >
                                    {line}
                                  </pre>
                                ))}
                              </div>
                              {totalHistoryPages > 1 && (
                                <div className="flex items-center justify-between pt-2 border-t mt-2">
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    disabled={historyPage === 0}
                                    onClick={() => setHistoryPage((p) => Math.max(0, p - 1))}
                                  >
                                    {t('terminalDebug.historyPrev', { defaultValue: 'Previous' })}
                                  </Button>
                                  <span className="text-xs text-muted-foreground">
                                    {historyPage + 1} / {totalHistoryPages}
                                  </span>
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    disabled={historyPage >= totalHistoryPages - 1}
                                    onClick={() => setHistoryPage((p) => Math.min(totalHistoryPages - 1, p + 1))}
                                  >
                                    {t('terminalDebug.historyNext', { defaultValue: 'Next' })}
                                  </Button>
                                </div>
                              )}
                            </div>
                          );
                        } else {
                          return (
                            <div className="text-sm text-muted-foreground">
                              {t('terminalDebug.historyEmpty', {
                                defaultValue: 'No terminal history available yet.',
                              })}
                            </div>
                          );
                        }
                      })()}
                    </div>
                  );
                } else {
                  return (
                    <div className="h-full flex items-center justify-center text-muted-foreground">
                      {t('terminalDebug.starting')}
                    </div>
                  );
                }
              })()}
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-muted-foreground">
            {t('terminalDebug.selectPrompt')}
          </div>
        )}
      </div>
    </div>
  );
}

function TerminalQualityBadgeInline({ terminalId, className }: Readonly<{ terminalId: string; className?: string }>) {
  const { data } = useTerminalLatestQuality(terminalId);
  if (!data) return null;
  return (
    <QualityBadge
      gateStatus={data.gateStatus as GateStatus}
      totalIssues={data.totalIssues}
      blockingIssues={data.blockingIssues}
      className={className}
    />
  );
}

function StatusDot({ status }: Readonly<{ status: string }>) {
  const colors: Record<string, string> = {
    not_started: 'bg-gray-400',
    starting: 'bg-yellow-400',
    waiting: 'bg-blue-400',
    working: 'bg-green-400 animate-pulse',
    completed: 'bg-green-500',
    failed: 'bg-red-500',
  };

  return <div className={cn('w-2 h-2 rounded-full', colors[status] || 'bg-gray-400')} />;
}
