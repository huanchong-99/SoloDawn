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
  // Refs are used intentionally here instead of useState to avoid unnecessary re-renders.
  // These track transient process lifecycle state (ready, starting, autoStarted, needsRestart)
  // that only matters for imperative logic (API calls, terminal launch decisions),
  // not for rendering. Converting them to useState would cause render cascades
  // on every terminal start/stop cycle without any visible UI benefit.
  // TODO: G28-006 — Consider consolidating these refs into a single useReducer
  // or a ref-backed state machine to reduce the number of independent mutable refs.
  const readyTerminalIdsRef = useRef<Set<string>>(new Set());
  const startingTerminalIdsRef = useRef<Set<string>>(new Set());
  const terminalRef = useRef<TerminalEmulatorRef>(null);
  const autoStartedRef = useRef<Set<string>>(new Set());
  const needsRestartRef = useRef<Set<string>>(new Set());
  const restartAttemptsRef = useRef<Map<string, number>>(new Map());
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

  const selectedTerminal = allTerminals.find((terminal) => terminal.id === selectedTerminalId);

  const getTerminalLabel = (terminal: Terminal) => {
    const role = terminal.role?.trim();
    return role ?? `${defaultRoleLabel} ${terminal.orderIndex + 1}`;
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

      if (needsRestartRef.current.has(terminal.id)) {
        return false;
      }

      if (startingTerminalIdsRef.current.has(terminal.id)) {
        return false;
      }

      if (readyTerminalIdsRef.current.has(terminal.id)) {
        return true;
      }

      // Only 'working' remains after filtering out historical, waiting, needsRestart,
      // starting, and ready states above.
      return terminal.status === 'working';
    },
    [isHistoricalTerminalStatus]
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
    if (startingTerminalIdsRef.current.has(terminalId)) return;
    startingTerminalIdsRef.current.add(terminalId);
    // Mark as auto-started only after confirming we can start
    autoStartedRef.current.add(terminalId);
    try {
      const response = await fetch(`/api/terminals/${terminalId}/start`, {
        method: 'POST',
      });

      if (response.ok) {
        console.log('Terminal started successfully');
        // Mark this terminal as ready and clear restart flag
        needsRestartRef.current.delete(terminalId);
        readyTerminalIdsRef.current.add(terminalId);
        // Note: Don't reset restart attempts here - only reset on manual restart
        // This prevents infinite loops when API succeeds but process doesn't actually start
      } else {
        const error = await response.json().catch(() => null);

        // Handle 409 Conflict by stopping first, then retrying
        if (response.status === 409 && !retryAfterStop) {
          console.log('Terminal conflict, stopping and retrying...');
          startingTerminalIdsRef.current.delete(terminalId);
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
        readyTerminalIdsRef.current.delete(terminalId);
      }
    } catch (error) {
      console.error('Failed to start terminal:', error);
      resetAutoStart(terminalId);
      // Clear ready state on failure
      readyTerminalIdsRef.current.delete(terminalId);
    } finally {
      startingTerminalIdsRef.current.delete(terminalId);
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

    if (startingTerminalIdsRef.current.has(terminalId)) {
      console.info(
        `Skip auto-restart for terminal ${terminalId} because restart is already in progress`
      );
      return true;
    }

    return false;
  }, [shouldAutoRecoverTerminal]);

  // Helper to handle max restart attempts reached
  const handleMaxRestartAttemptsReached = useCallback((terminalId: string) => {
    console.error(`Max restart attempts (${MAX_RESTART_ATTEMPTS}) reached for terminal ${terminalId}`);
    needsRestartRef.current.add(terminalId);
    readyTerminalIdsRef.current.delete(terminalId);
  }, []);

  // Helper to perform terminal restart
  const performTerminalRestart = useCallback((terminalId: string, attempts: number) => {
    console.log(`Terminal process not running, auto-restarting... (attempt ${attempts + 1}/${MAX_RESTART_ATTEMPTS})`);
    restartAttemptsRef.current.set(terminalId, attempts + 1);
    needsRestartRef.current.add(terminalId);
    readyTerminalIdsRef.current.delete(terminalId);
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
      if (readyTerminalIdsRef.current.has(selectedTerminalId)) {
        readyTerminalIdsRef.current.delete(selectedTerminalId);
      }
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
  const currentHistoryText = currentHistory?.lines.length
    ? sanitizeTerminalHistoryContent(currentHistory.lines.join(''))
    : '';

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
                          // TODO: Use virtualization (e.g., react-window) for large terminal logs
                          // to avoid rendering thousands of lines into a single <pre> element.
                          return (
                            <div className="flex-1 min-h-0 overflow-y-auto overflow-x-auto pr-1">
                              <pre className="text-xs leading-5 whitespace-pre-wrap break-words text-foreground">
                                {currentHistoryText}
                              </pre>
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
