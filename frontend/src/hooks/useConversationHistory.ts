// useConversationHistory.ts
import {
  CommandExitStatus,
  ExecutionProcess,
  ExecutionProcessStatus,
  ExecutorAction,
  NormalizedEntry,
  PatchType,
  ToolStatus,
  Workspace,
} from 'shared/types';
import { useExecutionProcessesContext } from '@/contexts/ExecutionProcessesContext';
import { useCallback, useEffect, useMemo, useRef } from 'react';
import { streamJsonPatchEntries } from '@/utils/streamJsonPatchEntries';

export type PatchTypeWithKey = PatchType & {
  patchKey: string;
  executionProcessId: string;
};

export type AddEntryType = 'initial' | 'running' | 'historic' | 'plan';

export type OnEntriesUpdated = (
  newEntries: PatchTypeWithKey[],
  addType: AddEntryType,
  loading: boolean
) => void;

type ExecutionProcessStaticInfo = {
  id: string;
  createdAt: string;
  updatedAt: string;
  executorAction: ExecutorAction;
};

type ExecutionProcessState = {
  executionProcess: ExecutionProcessStaticInfo;
  entries: PatchTypeWithKey[];
};

type ExecutionProcessStateStore = Record<string, ExecutionProcessState>;
type PatchWithKeyBuilder = (
  patch: PatchType,
  executionProcessId: string,
  index: number | 'user'
) => PatchTypeWithKey;

interface UseConversationHistoryParams {
  attempt: Workspace;
  onEntriesUpdated: OnEntriesUpdated;
}

interface UseConversationHistoryResult {}

const MIN_INITIAL_ENTRIES = 10;
const REMAINING_BATCH_SIZE = 50;

const makeLoadingPatch = (executionProcessId: string): PatchTypeWithKey => ({
  type: 'NORMALIZED_ENTRY',
  content: {
    entry_type: {
      type: 'loading',
    },
    content: '',
    timestamp: null,
  },
  patchKey: `${executionProcessId}:loading`,
  executionProcessId,
});

const nextActionPatch: (
  failed: boolean,
  execution_processes: number,
  needs_setup: boolean,
  setup_help_text?: string
) => PatchTypeWithKey = (
  failed,
  execution_processes,
  needs_setup,
  setup_help_text
) => ({
  type: 'NORMALIZED_ENTRY',
  content: {
    entry_type: {
      type: 'next_action',
      failed: failed,
      execution_processes: execution_processes,
      needs_setup: needs_setup,
      setup_help_text: setup_help_text ?? null,
    },
    content: '',
    timestamp: null,
  },
  patchKey: 'next_action',
  executionProcessId: '',
});

// Helper to check if entries have pending approval
function hasPendingApprovalInEntries(entries: PatchTypeWithKey[]): boolean {
  return entries.some((entry) => {
    if (entry.type !== 'NORMALIZED_ENTRY') return false;
    const entryType = entry.content.entry_type;
    return (
      entryType.type === 'tool_use' &&
      entryType.status.status === 'pending_approval'
    );
  });
}

// Helper to check if entries have setup required error
function findSetupRequiredText(entries: PatchTypeWithKey[]): string | undefined {
  for (const entry of entries) {
    if (entry.type !== 'NORMALIZED_ENTRY') continue;
    if (
      entry.content.entry_type.type === 'error_message' &&
      entry.content.entry_type.error_type.type === 'setup_required'
    ) {
      return entry.content.content;
    }
  }
  return undefined;
}

// Helper to create user message patch
function createUserMessagePatch(
  executionProcessId: string,
  prompt: string,
  patchWithKey: PatchWithKeyBuilder
): PatchTypeWithKey {
  const userNormalizedEntry: NormalizedEntry = {
    entry_type: {
      type: 'user_message',
    },
    content: prompt,
    timestamp: null,
  };
  const userPatch: PatchType = {
    type: 'NORMALIZED_ENTRY',
    content: userNormalizedEntry,
  };
  return patchWithKey(userPatch, executionProcessId, 'user');
}

function buildCommandExitStatus(
  executionProcess: ExecutionProcess | undefined,
  exitCode: number
): CommandExitStatus | null {
  if (executionProcess?.status === ExecutionProcessStatus.running) {
    return null;
  }

  return {
    type: 'exit_code',
    code: exitCode,
  };
}

function buildToolStatus(
  executionProcess: ExecutionProcess | undefined,
  exitCode: number
): ToolStatus {
  if (executionProcess?.status === ExecutionProcessStatus.running) {
    return { status: 'created' };
  }

  if (exitCode === 0) {
    return { status: 'success' };
  }

  return { status: 'failed' };
}

function stringifyEntryContent(line: PatchTypeWithKey): string {
  if (typeof line.content === 'string') {
    return line.content;
  }

  return JSON.stringify(line.content);
}

function stringifyEntries(entries: PatchTypeWithKey[]): string {
  return entries.map(stringifyEntryContent).join('\n');
}

// Helper to create script tool patch
function createScriptToolPatch(
  executionProcess: ExecutionProcess | undefined,
  executionProcessId: string,
  toolName: string,
  script: string,
  entries: PatchTypeWithKey[],
  patchWithKey: PatchWithKeyBuilder
): PatchTypeWithKey {
  const exitCode = Number(executionProcess?.exitCode) || 0;
  const exit_status = buildCommandExitStatus(executionProcess, exitCode);
  const toolStatus = buildToolStatus(executionProcess, exitCode);
  const output = stringifyEntries(entries);

  const toolNormalizedEntry: NormalizedEntry = {
    entry_type: {
      type: 'tool_use',
      tool_name: toolName,
      action_type: {
        action: 'command_run',
        command: script,
        result: {
          output,
          exit_status,
        },
      },
      status: toolStatus,
    },
    content: toolName,
    timestamp: null,
  };
  const toolPatch: PatchType = {
    type: 'NORMALIZED_ENTRY',
    content: toolNormalizedEntry,
  };
  return patchWithKey(toolPatch, executionProcessId, 0);
}

// Helper to get script tool name
function getScriptToolName(context: string): string | null {
  switch (context) {
    case 'SetupScript':
      return 'Setup Script';
    case 'CleanupScript':
      return 'Cleanup Script';
    case 'ToolInstallScript':
      return 'Tool Install Script';
    default:
      return null;
  }
}

type CodingAgentProcessResult = {
  entries: PatchTypeWithKey[];
  hasPendingApproval: boolean;
  isProcessRunning: boolean;
  processFailedOrKilled: boolean;
  needsSetup: boolean;
  setupHelpText: string | undefined;
};

type ScriptProcessResult = {
  entries: PatchTypeWithKey[];
  isProcessRunning: boolean;
  processFailedOrKilled: boolean;
};

type EmitAggregationState = {
  allEntries: PatchTypeWithKey[];
  hasPendingApproval: boolean;
  hasRunningProcess: boolean;
  lastProcessFailedOrKilled: boolean;
  needsSetup: boolean;
  setupHelpText: string | undefined;
};

function createEmitAggregationState(): EmitAggregationState {
  return {
    allEntries: [],
    hasPendingApproval: false,
    hasRunningProcess: false,
    lastProcessFailedOrKilled: false,
    needsSetup: false,
    setupHelpText: undefined,
  };
}

function isCodingAgentActionType(actionType: ExecutorAction['typ']['type']): boolean {
  return (
    actionType === 'CodingAgentInitialRequest' ||
    actionType === 'CodingAgentFollowUpRequest' ||
    actionType === 'ReviewRequest'
  );
}

function mergeCodingAgentResult(
  aggregation: EmitAggregationState,
  result: CodingAgentProcessResult
): void {
  if (result.hasPendingApproval) {
    aggregation.hasPendingApproval = true;
  }
  if (result.isProcessRunning) {
    aggregation.hasRunningProcess = true;
  }
  if (result.processFailedOrKilled) {
    aggregation.lastProcessFailedOrKilled = true;
    if (result.needsSetup) {
      aggregation.needsSetup = true;
      aggregation.setupHelpText = result.setupHelpText;
    }
  }

  aggregation.allEntries.push(...result.entries);
}

function mergeScriptProcessResult(
  aggregation: EmitAggregationState,
  result: ScriptProcessResult
): void {
  if (result.isProcessRunning) {
    aggregation.hasRunningProcess = true;
  }
  if (result.processFailedOrKilled) {
    aggregation.lastProcessFailedOrKilled = true;
  }

  aggregation.allEntries.push(...result.entries);
}

function mapEntriesWithPatchKey(
  entries: PatchType[],
  executionProcessId: string,
  patchWithKey: PatchWithKeyBuilder
): PatchTypeWithKey[] {
  const entriesWithKey: PatchTypeWithKey[] = [];
  for (const [index, entry] of entries.entries()) {
    entriesWithKey.push(patchWithKey(entry, executionProcessId, index));
  }
  return entriesWithKey;
}

function createProcessEntriesMutator(
  executionProcess: ExecutionProcess,
  entries: PatchTypeWithKey[]
): (state: ExecutionProcessStateStore) => void {
  return (state) => {
    state[executionProcess.id] = { executionProcess, entries };
  };
}

function getPromptFromExecutorAction(typ: ExecutorAction['typ']): string {
  if (
    typ.type === 'CodingAgentInitialRequest' ||
    typ.type === 'CodingAgentFollowUpRequest'
  ) {
    return typ.prompt;
  }

  return '';
}

function buildExecutionProcessStreamUrl(executionProcess: ExecutionProcess): string {
  if (executionProcess.executorAction.typ.type === 'ScriptRequest') {
    return `/api/execution-processes/${executionProcess.id}/raw-logs/ws`;
  }

  return `/api/execution-processes/${executionProcess.id}/normalized-logs/ws`;
}

function getRunningOrInitialAddType(
  displayedCount: number
): Extract<AddEntryType, 'running' | 'initial'> {
  if (displayedCount > 1) {
    return 'running';
  }

  return 'initial';
}

function processCodingAgentEntries(
  processState: ExecutionProcessState,
  index: number,
  totalProcesses: number,
  getLiveExecutionProcess: (executionProcessId: string) => ExecutionProcess | undefined,
  patchWithKey: PatchWithKeyBuilder
): CodingAgentProcessResult {
  const entries: PatchTypeWithKey[] = [];

  const prompt = getPromptFromExecutorAction(processState.executionProcess.executorAction.typ);
  const userPatch = createUserMessagePatch(
    processState.executionProcess.id,
    prompt,
    patchWithKey
  );
  entries.push(userPatch);

  const entriesExcludingUser = processState.entries.filter(
    (entry) =>
      entry.type !== 'NORMALIZED_ENTRY' ||
      entry.content.entry_type.type !== 'user_message'
  );

  const hasPendingApproval = hasPendingApprovalInEntries(entriesExcludingUser);
  entries.push(...entriesExcludingUser);

  const liveProcessStatus = getLiveExecutionProcess(processState.executionProcess.id)?.status;
  const isProcessRunning = liveProcessStatus === ExecutionProcessStatus.running;
  const processFailedOrKilled =
    liveProcessStatus === ExecutionProcessStatus.failed ||
    liveProcessStatus === ExecutionProcessStatus.killed;

  let needsSetup = false;
  let setupHelpText: string | undefined;

  if (processFailedOrKilled && index === totalProcesses - 1) {
    const setupText = findSetupRequiredText(entriesExcludingUser);
    if (setupText) {
      needsSetup = true;
      setupHelpText = setupText;
    }
  }

  if (isProcessRunning && !hasPendingApprovalInEntries(entriesExcludingUser)) {
    entries.push(makeLoadingPatch(processState.executionProcess.id));
  }

  return {
    entries,
    hasPendingApproval,
    isProcessRunning,
    processFailedOrKilled,
    needsSetup,
    setupHelpText,
  };
}

function processScriptRequestEntries(
  processState: ExecutionProcessState,
  index: number,
  totalProcesses: number,
  getLiveExecutionProcess: (executionProcessId: string) => ExecutionProcess | undefined,
  patchWithKey: PatchWithKeyBuilder
): ScriptProcessResult {
  const typ = processState.executionProcess.executorAction.typ;
  const context = typ.type === 'ScriptRequest' ? typ.context : null;
  const toolName = context ? getScriptToolName(context) : null;

  if (!toolName) {
    return {
      entries: [],
      isProcessRunning: false,
      processFailedOrKilled: false,
    };
  }

  const executionProcess = getLiveExecutionProcess(processState.executionProcess.id);
  const isProcessRunning = executionProcess?.status === ExecutionProcessStatus.running;
  const processFailedOrKilled =
    (executionProcess?.status === ExecutionProcessStatus.failed ||
      executionProcess?.status === ExecutionProcessStatus.killed) &&
    index === totalProcesses - 1;

  const script = typ.type === 'ScriptRequest' ? typ.script : '';
  const toolPatch = createScriptToolPatch(
    executionProcess,
    processState.executionProcess.id,
    toolName,
    script,
    processState.entries,
    patchWithKey
  );

  return {
    entries: [toolPatch],
    isProcessRunning,
    processFailedOrKilled,
  };
}

export const useConversationHistory = ({
  attempt,
  onEntriesUpdated,
}: UseConversationHistoryParams): UseConversationHistoryResult => {
  const { executionProcessesVisible: executionProcessesRaw } =
    useExecutionProcessesContext();
  const executionProcesses = useRef<ExecutionProcess[]>(executionProcessesRaw);
  const displayedExecutionProcesses = useRef<ExecutionProcessStateStore>({});
  const loadedInitialEntries = useRef(false);
  const streamingProcessIdsRef = useRef<Set<string>>(new Set());
  const onEntriesUpdatedRef = useRef<OnEntriesUpdated | null>(null);

  const mergeIntoDisplayed = (
    mutator: (state: ExecutionProcessStateStore) => void
  ) => {
    const state = displayedExecutionProcesses.current;
    mutator(state);
  };
  useEffect(() => {
    onEntriesUpdatedRef.current = onEntriesUpdated;
  }, [onEntriesUpdated]);

  // Keep executionProcesses up to date
  useEffect(() => {
    executionProcesses.current = executionProcessesRaw.filter(
      (ep) =>
        ep.runReason === 'setupscript' ||
        ep.runReason === 'cleanupscript' ||
        ep.runReason === 'codingagent'
    );
  }, [executionProcessesRaw]);

  const loadEntriesForHistoricExecutionProcess = (
    executionProcess: ExecutionProcess
  ) => {
    const url = buildExecutionProcessStreamUrl(executionProcess);

    return new Promise<PatchType[]>((resolve) => {
      const controller = streamJsonPatchEntries<PatchType>(url, {
        onFinished: (allEntries) => {
          controller.close();
          resolve(allEntries);
        },
        onError: (err) => {
          console.warn(
            `Error loading entries for historic execution process ${executionProcess.id}`,
            err
          );
          controller.close();
          resolve([]);
        },
      });
    });
  };

  const getLiveExecutionProcess = (
    executionProcessId: string
  ): ExecutionProcess | undefined => {
    return executionProcesses?.current.find(
      (executionProcess) => executionProcess.id === executionProcessId
    );
  };

  const patchWithKey = (
    patch: PatchType,
    executionProcessId: string,
    index: number | 'user'
  ) => {
    return {
      ...patch,
      patchKey: `${executionProcessId}:${index}`,
      executionProcessId,
    };
  };

  const flattenEntries = (
    executionProcessState: ExecutionProcessStateStore
  ): PatchTypeWithKey[] => {
    return Object.values(executionProcessState)
      .filter(
        (p) =>
          p.executionProcess.executorAction.typ.type ===
            'CodingAgentFollowUpRequest' ||
          p.executionProcess.executorAction.typ.type ===
            'CodingAgentInitialRequest' ||
          p.executionProcess.executorAction.typ.type === 'ReviewRequest'
      )
      .sort(
        (a, b) =>
          new Date(
            a.executionProcess.createdAt
          ).getTime() -
          new Date(b.executionProcess.createdAt).getTime()
      )
      .flatMap((p) => p.entries);
  };

  const getActiveAgentProcesses = (): ExecutionProcess[] => {
    return (
      executionProcesses?.current.filter(
        (p) =>
          p.status === ExecutionProcessStatus.running &&
          p.runReason !== 'devserver'
      ) ?? []
    );
  };

  const flattenEntriesForEmit = useCallback(
    (executionProcessState: ExecutionProcessStateStore): PatchTypeWithKey[] => {
      const totalProcesses = Object.keys(executionProcessState).length;
      const sortedProcesses = Object.values(executionProcessState).sort(
        (a, b) =>
          new Date(a.executionProcess.createdAt).getTime() -
          new Date(b.executionProcess.createdAt).getTime()
      );
      const aggregation = createEmitAggregationState();

      for (const [index, processState] of sortedProcesses.entries()) {
        const actionType = processState.executionProcess.executorAction.typ.type;

        if (isCodingAgentActionType(actionType)) {
          const result = processCodingAgentEntries(
            processState,
            index,
            totalProcesses,
            getLiveExecutionProcess,
            patchWithKey
          );
          mergeCodingAgentResult(aggregation, result);
          if (!result.processFailedOrKilled && !result.isProcessRunning) {
            aggregation.lastProcessFailedOrKilled = false;
          }
          continue;
        }

        if (actionType !== 'ScriptRequest') {
          continue;
        }

        const result = processScriptRequestEntries(
          processState,
          index,
          totalProcesses,
          getLiveExecutionProcess,
          patchWithKey
        );
        mergeScriptProcessResult(aggregation, result);
        if (!result.processFailedOrKilled && !result.isProcessRunning) {
          aggregation.lastProcessFailedOrKilled = false;
        }
      }

      if (!aggregation.hasRunningProcess && !aggregation.hasPendingApproval) {
        aggregation.allEntries.push(
          nextActionPatch(
            aggregation.lastProcessFailedOrKilled,
            totalProcesses,
            aggregation.needsSetup,
            aggregation.setupHelpText
          )
        );
      }

      return aggregation.allEntries;
    },
    []
  );

  const emitEntries = useCallback(
    (
      executionProcessState: ExecutionProcessStateStore,
      addEntryType: AddEntryType,
      loading: boolean
    ) => {
      const entries = flattenEntriesForEmit(executionProcessState);
      let modifiedAddEntryType = addEntryType;

      // Modify so that if add entry type is 'running' and last entry is a plan, emit special plan type
      if (entries.length > 0) {
        const lastEntry = entries[entries.length - 1];
        if (
          lastEntry.type === 'NORMALIZED_ENTRY' &&
          lastEntry.content.entry_type.type === 'tool_use' &&
          lastEntry.content.entry_type.tool_name === 'ExitPlanMode'
        ) {
          modifiedAddEntryType = 'plan';
        }
      }

      onEntriesUpdatedRef.current?.(entries, modifiedAddEntryType, loading);
    },
    [flattenEntriesForEmit]
  );

  // This emits its own events as they are streamed
  const loadRunningAndEmit = useCallback(
    (executionProcess: ExecutionProcess): Promise<void> => {
      const url = executionProcess.executorAction.typ.type === 'ScriptRequest'
        ? `/api/execution-processes/${executionProcess.id}/raw-logs/ws`
        : `/api/execution-processes/${executionProcess.id}/normalized-logs/ws`;

      return new Promise((resolve, reject) => {
        const controller = streamJsonPatchEntries<PatchType>(url, {
          onEntries(entries) {
            const patchesWithKey = mapEntriesWithPatchKey(
              entries,
              executionProcess.id,
              patchWithKey
            );
            mergeIntoDisplayed(
              createProcessEntriesMutator(executionProcess, patchesWithKey)
            );
            emitEntries(displayedExecutionProcesses.current, 'running', false);
          },
          onFinished: () => {
            emitEntries(displayedExecutionProcesses.current, 'running', false);
            controller.close();
            resolve();
          },
          onError: () => {
            controller.close();
            reject(new Error('Failed to load running process'));
          },
        });
      });
    },
    [emitEntries]
  );

  // Sometimes it can take a few seconds for the stream to start, wrap the loadRunningAndEmit method
  const loadRunningAndEmitWithBackoff = useCallback(
    async (executionProcess: ExecutionProcess) => {
      for (let i = 0; i < 20; i++) {
        try {
          await loadRunningAndEmit(executionProcess);
          break;
        } catch (error) {
          console.debug('Failed to load running process, retrying...', error);
          await new Promise((resolve) => setTimeout(resolve, 500));
        }
      }
    },
    [loadRunningAndEmit]
  );

  const loadInitialEntries =
    useCallback(async (): Promise<ExecutionProcessStateStore> => {
      const localDisplayedExecutionProcesses: ExecutionProcessStateStore = {};

      if (!executionProcesses?.current) return localDisplayedExecutionProcesses;

      for (const executionProcess of [
        ...executionProcesses.current,
      ].reverse()) {
        if (executionProcess.status === ExecutionProcessStatus.running)
          continue;

        const entries =
          await loadEntriesForHistoricExecutionProcess(executionProcess);
        const entriesWithKey = entries.map((e, idx) =>
          patchWithKey(e, executionProcess.id, idx)
        );

        localDisplayedExecutionProcesses[executionProcess.id] = {
          executionProcess,
          entries: entriesWithKey,
        };

        if (
          flattenEntries(localDisplayedExecutionProcesses).length >
          MIN_INITIAL_ENTRIES
        ) {
          break;
        }
      }

      return localDisplayedExecutionProcesses;
    }, [executionProcesses]);

  const loadRemainingEntriesInBatches = useCallback(
    async (batchSize: number): Promise<boolean> => {
      if (!executionProcesses?.current) return false;

      let anyUpdated = false;
      for (const executionProcess of [
        ...executionProcesses.current,
      ].reverse()) {
        const current = displayedExecutionProcesses.current;
        if (
          current[executionProcess.id] ||
          executionProcess.status === ExecutionProcessStatus.running
        )
          continue;

        const entries =
          await loadEntriesForHistoricExecutionProcess(executionProcess);
        const entriesWithKey = entries.map((e, idx) =>
          patchWithKey(e, executionProcess.id, idx)
        );

        mergeIntoDisplayed((state) => {
          state[executionProcess.id] = {
            executionProcess,
            entries: entriesWithKey,
          };
        });

        if (
          flattenEntries(displayedExecutionProcesses.current).length > batchSize
        ) {
          anyUpdated = true;
          break;
        }
        anyUpdated = true;
      }
      return anyUpdated;
    },
    [executionProcesses]
  );

  const ensureProcessVisible = useCallback((p: ExecutionProcess) => {
    mergeIntoDisplayed((state) => {
      if (!state[p.id]) {
        state[p.id] = {
          executionProcess: {
            id: p.id,
            createdAt: p.createdAt,
            updatedAt: p.updatedAt,
            executorAction: p.executorAction,
          },
          entries: [],
        };
      }
    });
  }, []);

  const idListKey = useMemo(
    () => executionProcessesRaw?.map((p) => p.id).join(','),
    [executionProcessesRaw]
  );

  const idStatusKey = useMemo(
    () => executionProcessesRaw?.map((p) => `${p.id}:${p.status}`).join(','),
    [executionProcessesRaw]
  );

  // Initial load when attempt changes
  useEffect(() => {
    displayedExecutionProcesses.current = {};
    loadedInitialEntries.current = false;
    streamingProcessIdsRef.current.clear();
    let cancelled = false;
    (async () => {
      // Waiting for execution processes to load
      if (
        executionProcesses?.current.length === 0 ||
        loadedInitialEntries.current
      )
        return;

      // Initial entries
      const allInitialEntries = await loadInitialEntries();
      if (cancelled) return;
      mergeIntoDisplayed((state) => {
        Object.assign(state, allInitialEntries);
      });
      emitEntries(displayedExecutionProcesses.current, 'initial', false);
      loadedInitialEntries.current = true;

      // Then load the remaining in batches
      while (
        !cancelled &&
        (await loadRemainingEntriesInBatches(REMAINING_BATCH_SIZE))
      ) {
        if (cancelled) return;
      }
      await new Promise((resolve) => setTimeout(resolve, 100));
      emitEntries(displayedExecutionProcesses.current, 'historic', false);
    })();
    return () => {
      cancelled = true;
    };
  }, [
    attempt.id,
    idListKey,
    loadInitialEntries,
    loadRemainingEntriesInBatches,
    emitEntries,
  ]); // include idListKey so new processes trigger reload

  useEffect(() => {
    const activeProcesses = getActiveAgentProcesses();
    if (activeProcesses.length === 0) return;

    for (const activeProcess of activeProcesses) {
      if (!displayedExecutionProcesses.current[activeProcess.id]) {
        const runningOrInitial = getRunningOrInitialAddType(
          Object.keys(displayedExecutionProcesses.current).length
        );
        ensureProcessVisible(activeProcess);
        emitEntries(
          displayedExecutionProcesses.current,
          runningOrInitial,
          false
        );
      }

      if (
        activeProcess.status === ExecutionProcessStatus.running &&
        !streamingProcessIdsRef.current.has(activeProcess.id)
      ) {
        streamingProcessIdsRef.current.add(activeProcess.id);
        loadRunningAndEmitWithBackoff(activeProcess).finally(() => {
          streamingProcessIdsRef.current.delete(activeProcess.id);
        });
      }
    }
  }, [
    attempt.id,
    idStatusKey,
    emitEntries,
    ensureProcessVisible,
    loadRunningAndEmitWithBackoff,
  ]);

  // If an execution process is removed, remove it from the state
  useEffect(() => {
    if (!executionProcessesRaw) return;

    const removedProcessIds = Object.keys(
      displayedExecutionProcesses.current
    ).filter((id) => !executionProcessesRaw.some((p) => p.id === id));

    if (removedProcessIds.length > 0) {
      mergeIntoDisplayed((state) => {
        removedProcessIds.forEach((id) => {
          delete state[id];
        });
      });
    }
  }, [attempt.id, idListKey, executionProcessesRaw]);


  return {};
};
