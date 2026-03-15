import * as React from 'react';
import { create } from 'zustand';
import { secureRandomIdFragment } from '@/utils/id';

/**
 * WebSocket message structure following design specification
 */
export interface WsMessage {
  type: string;
  payload: unknown;
  timestamp: string;
  id: string;
}

/**
 * WebSocket event types following namespace convention
 */
export type WsEventType =
  | 'workflow.status_changed'
  | 'terminal.status_changed'
  | 'task.status_changed'
  | 'terminal.completed'
  | 'git.commit_detected'
  | 'orchestrator.awakened'
  | 'orchestrator.decision'
  | 'system.heartbeat'
  | 'system.lagged'
  | 'system.error'
  | 'terminal.prompt_detected'
  | 'terminal.prompt_decision'
  | 'quality.gate_result'
  | 'provider.switched'
  | 'provider.exhausted'
  | 'provider.recovered';

type ConnectionStatus =
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'reconnecting';

// G30-007 / G30-008: Extended status used internally by WorkflowScopedConnection
// to distinguish transport errors ('error') and permanent reconnect failure ('failed')
// from a clean 'disconnected'. The public-facing connectionStatus / workflowConnectionStatus
// maps these back to the base ConnectionStatus for backward compatibility, while
// lastError carries the detail.
type InternalConnectionStatus = ConnectionStatus | 'error' | 'failed';

type MessageHandler = (payload: unknown) => void;

interface WorkflowScopedConnection {
  ws: WebSocket | null;
  status: InternalConnectionStatus;
  reconnectAttempts: number;
  heartbeatInterval: ReturnType<typeof setInterval> | null;
  reconnectTimeout: ReturnType<typeof setTimeout> | null;
  manualDisconnect: boolean;
  url: string;
  refCount: number;
  lastHeartbeat: Date | null;
}

export interface TerminalPromptResponsePayload {
  workflowId: string;
  terminalId: string;
  response: string;
}

interface WsState {
  // State
  connectionStatus: ConnectionStatus;
  workflowConnectionStatus: Record<string, ConnectionStatus>;
  lastHeartbeat: Date | null;
  lastError: string | null;
  reconnectAttempts: number;
  currentWorkflowId: string | null;

  // Internal
  _ws: WebSocket | null;
  _handlers: Map<string, Set<MessageHandler>>;
  _workflowHandlers: Map<string, Map<string, Set<MessageHandler>>>;
  _workflowConnections: Map<string, WorkflowScopedConnection>;
  _heartbeatInterval: ReturnType<typeof setInterval> | null;
  _reconnectTimeout: ReturnType<typeof setTimeout> | null;
  _url: string | null;
  _manualDisconnect: boolean;

  // Actions
  connect: (url: string) => void;
  connectToWorkflow: (workflowId: string) => void;
  disconnectWorkflow: (workflowId: string) => void;
  disconnect: () => void;
  send: (message: WsMessage) => boolean;
  sendPromptResponse: (payload: TerminalPromptResponsePayload) => boolean;
  subscribe: (eventType: string, handler: MessageHandler) => () => void;
  subscribeToWorkflow: (
    workflowId: string,
    eventType: string,
    handler: MessageHandler
  ) => () => void;
  getWorkflowConnectionStatus: (workflowId: string) => ConnectionStatus;
}

const HEARTBEAT_INTERVAL = 30000; // 30 seconds
const MAX_RECONNECT_ATTEMPTS = 5;
const BASE_RECONNECT_DELAY = 1000; // 1 second

/**
 * Generate a unique message ID
 */
function generateMessageId(): string {
  return `msg-${Date.now()}-${secureRandomIdFragment(7)}`;
}

type JsonRecord = Record<string, unknown>;

function isJsonRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function getStringField(
  payload: JsonRecord,
  ...keys: string[]
): string | undefined {
  for (const key of keys) {
    const value = payload[key];
    if (typeof value === 'string') {
      return value;
    }
  }
  return undefined;
}

function getNumberField(
  payload: JsonRecord,
  ...keys: string[]
): number | undefined {
  for (const key of keys) {
    const value = payload[key];
    if (typeof value === 'number' && Number.isFinite(value)) {
      return value;
    }
    if (typeof value === 'string') {
      const parsedNumber = Number(value);
      if (Number.isFinite(parsedNumber)) {
        return parsedNumber;
      }
    }
  }
  return undefined;
}

function getBooleanField(
  payload: JsonRecord,
  ...keys: string[]
): boolean | undefined {
  for (const key of keys) {
    const value = payload[key];
    if (typeof value === 'boolean') {
      return value;
    }
    if (typeof value === 'string') {
      if (value === 'true') {
        return true;
      }
      if (value === 'false') {
        return false;
      }
    }
  }
  return undefined;
}

function normalizeKeyToken(value: string): string {
  return value
    .trim()
    .replaceAll(/([a-z0-9])([A-Z])/g, '$1_$2')
    .replaceAll(/[\s-]+/g, '_')
    .replaceAll(/_+/g, '_')
    .toLowerCase();
}

function normalizePromptKindValue(value: unknown): TerminalPromptKind {
  if (typeof value !== 'string') {
    return 'unknown';
  }

  const normalizedValue = normalizeKeyToken(value);
  switch (normalizedValue) {
    case 'enter_confirm':
    case 'confirmation':
      return 'enter_confirm';
    case 'yes_no':
    case 'yesno':
      return 'yes_no';
    case 'choice':
      return 'choice';
    case 'arrow_select':
    case 'arrowselect':
      return 'arrow_select';
    case 'input':
      return 'input';
    case 'password':
      return 'password';
    default:
      return 'unknown';
  }
}

function normalizePromptDecisionAction(value: unknown): PromptDecisionAction {
  if (typeof value !== 'string') {
    return 'unknown';
  }

  const normalizedValue = normalizeKeyToken(value);
  switch (normalizedValue) {
    case 'auto_confirm':
      return 'auto_confirm';
    case 'llm_decision':
      return 'llm_decision';
    case 'ask_user':
      return 'ask_user';
    case 'skip':
      return 'skip';
    default:
      return 'unknown';
  }
}

function normalizeTerminalCompletedStatus(
  value: unknown
): TerminalCompletedStatus {
  if (typeof value !== 'string') {
    return 'unknown';
  }

  const normalizedValue = normalizeKeyToken(value);
  switch (normalizedValue) {
    case 'completed':
      return 'completed';
    case 'failed':
      return 'failed';
    case 'cancelled':
    case 'canceled':
      return 'failed';
    case 'checkpoint':
      return 'checkpoint';
    case 'review_pass':
    case 'review_passed':
      return 'review_pass';
    case 'review_reject':
    case 'review_rejected':
      return 'review_reject';
    default:
      return 'unknown';
  }
}

function normalizePromptOptions(payload: JsonRecord): {
  options: string[];
  optionDetails?: TerminalPromptOption[];
} {
  const parseOptionEntries = (rawEntries: unknown): TerminalPromptOption[] => {
    if (!Array.isArray(rawEntries)) {
      return [];
    }

    const parsedOptions: TerminalPromptOption[] = [];

    rawEntries.forEach((option, fallbackIndex) => {
      if (typeof option === 'string') {
        parsedOptions.push({
          index: fallbackIndex,
          label: option,
          selected: false,
        });
        return;
      }

      if (!isJsonRecord(option)) {
        return;
      }

      const label = getStringField(option, 'label', 'value', 'text');
      if (!label) {
        return;
      }

      const parsedIndex = getNumberField(option, 'index');
      const selected = getBooleanField(option, 'selected') ?? false;

      parsedOptions.push({
        index: parsedIndex ?? fallbackIndex,
        label,
        selected,
      });
    });

    return parsedOptions;
  };

  const optionDetailsFromField = parseOptionEntries(
    payload.optionDetails ?? payload.option_details
  );
  const optionDetailsFromOptions = parseOptionEntries(payload.options);
  const optionDetails =
    optionDetailsFromField.length > 0 ? optionDetailsFromField : optionDetailsFromOptions;

  const optionsFromField = Array.isArray(payload.options)
    ? payload.options.filter((value): value is string => typeof value === 'string')
    : [];
  const options =
    optionsFromField.length > 0 ? optionsFromField : optionDetails.map((option) => option.label);

  if (optionDetails.length === 0) {
    return { options };
  }

  return {
    options,
    optionDetails,
  };
}

function extractPromptEventContext(
  payload: JsonRecord
): Pick<PromptEventContext, 'taskId' | 'sessionId' | 'autoConfirm'> {
  const context: Pick<PromptEventContext, 'taskId' | 'sessionId' | 'autoConfirm'> = {};

  const taskId = getStringField(payload, 'taskId', 'task_id');
  if (taskId) {
    context.taskId = taskId;
  }

  const sessionId = getStringField(payload, 'sessionId', 'session_id');
  if (sessionId) {
    context.sessionId = sessionId;
  }

  // G27-001: Extract autoConfirm field from prompt payload
  const autoConfirm = getBooleanField(payload, 'autoConfirm', 'auto_confirm');
  if (autoConfirm !== undefined) {
    context.autoConfirm = autoConfirm;
  }

  return context;
}

function normalizeTerminalCompletedPayload(payload: unknown): unknown {
  if (!isJsonRecord(payload)) {
    return payload;
  }

  const workflowId = getStringField(payload, 'workflowId', 'workflow_id');
  const taskId = getStringField(payload, 'taskId', 'task_id');
  const terminalId = getStringField(payload, 'terminalId', 'terminal_id');

  if (!workflowId || !taskId || !terminalId) {
    return payload;
  }

  const status = normalizeTerminalCompletedStatus(payload.status);
  const rawStatus =
    typeof payload.status === 'string' ? payload.status : undefined;
  const normalizedPayload: TerminalCompletedPayload = {
    workflowId,
    taskId,
    terminalId,
    status,
    commitHash: getStringField(payload, 'commitHash', 'commit_hash'),
    commitMessage: getStringField(payload, 'commitMessage', 'commit_message'),
    metadata: payload.metadata,
  };

  if (status === 'unknown' && rawStatus) {
    normalizedPayload.statusRaw = rawStatus;
  }

  return normalizedPayload;
}

function normalizeTerminalPromptDetectedPayload(payload: unknown): unknown {
  if (!isJsonRecord(payload)) {
    return payload;
  }

  const workflowId = getStringField(payload, 'workflowId', 'workflow_id');
  const terminalId = getStringField(payload, 'terminalId', 'terminal_id');

  if (!workflowId || !terminalId) {
    return payload;
  }

  const rawPromptKind = getStringField(payload, 'promptKind', 'prompt_kind');
  const normalizedPromptKind = normalizePromptKindValue(rawPromptKind);
  const { options, optionDetails } = normalizePromptOptions(payload);
  const promptContext = extractPromptEventContext(payload);

  let selectedIndex = getNumberField(
    payload,
    'selectedIndex',
    'selected_index'
  );
  if (selectedIndex === undefined && optionDetails) {
    selectedIndex = optionDetails.find((option) => option.selected)?.index;
  }

  const normalizedPayload: TerminalPromptDetectedPayload = {
    workflowId,
    terminalId,
    ...promptContext,
    promptKind: normalizedPromptKind,
    promptText:
      getStringField(
        payload,
        'promptText',
        'prompt_text',
        'rawText',
        'raw_text'
      ) ?? '',
    confidence: getNumberField(payload, 'confidence') ?? 0,
    hasDangerousKeywords:
      getBooleanField(
        payload,
        'hasDangerousKeywords',
        'has_dangerous_keywords'
      ) ?? false,
    options,
    selectedIndex: selectedIndex ?? null,
  };

  if (rawPromptKind && rawPromptKind !== normalizedPromptKind) {
    normalizedPayload.promptKindRaw = rawPromptKind;
  }

  if (optionDetails) {
    normalizedPayload.optionDetails = optionDetails;
  }

  return normalizedPayload;
}

function normalizeDecisionDetail(decision: JsonRecord): PromptDecisionDetail {
  const detail: PromptDecisionDetail = { ...decision };

  if (!('targetIndex' in detail)) {
    const targetIndex = getNumberField(decision, 'targetIndex', 'target_index');
    if (targetIndex !== undefined) {
      detail.targetIndex = targetIndex;
    } else if (
      decision.targetIndex === null ||
      decision.target_index === null
    ) {
      detail.targetIndex = null;
    }
  }

  const suggestions = decision.suggestions;
  if (Array.isArray(suggestions)) {
    detail.suggestions = suggestions.filter(
      (value): value is string => typeof value === 'string'
    );
  } else if (suggestions === null) {
    detail.suggestions = null;
  }

  return detail;
}

// Helper to handle string decision source
function handleStringDecisionSource(
  decisionSource: string,
  payload: JsonRecord,
  workflowId: string,
  terminalId: string,
  promptContext: ReturnType<typeof extractPromptEventContext>
): TerminalPromptDecisionPayload {
  const normalizedDecision = normalizePromptDecisionAction(decisionSource);
  const decisionDetailFallback = isJsonRecord(payload.decision_detail) ? payload.decision_detail : undefined;
  const decisionDetailSource = isJsonRecord(payload.decisionDetail) ? payload.decisionDetail : decisionDetailFallback;
  const decisionRawSource = payload.decisionRaw ?? payload.decision_raw;

  const normalizedPayload: TerminalPromptDecisionPayload = {
    workflowId,
    terminalId,
    ...promptContext,
    decision: normalizedDecision,
  };

  if (decisionDetailSource) {
    normalizedPayload.decisionDetail = normalizeDecisionDetail(decisionDetailSource);
  }

  if (decisionRawSource !== undefined) {
    normalizedPayload.decisionRaw = decisionRawSource;
  }

  if (normalizedDecision === 'unknown' && decisionRawSource === undefined) {
    normalizedPayload.decisionRaw = decisionSource;
  }

  return normalizedPayload;
}

// Helper to handle object decision source
function handleObjectDecisionSource(
  decisionSource: JsonRecord,
  workflowId: string,
  terminalId: string,
  promptContext: ReturnType<typeof extractPromptEventContext>
): TerminalPromptDecisionPayload {
  const decisionAction = normalizePromptDecisionAction(
    getStringField(decisionSource, 'action')
  );
  return {
    workflowId,
    terminalId,
    ...promptContext,
    decision: decisionAction,
    decisionDetail: normalizeDecisionDetail(decisionSource),
    decisionRaw: decisionSource,
  };
}

// Helper to handle top-level decision action
function handleTopLevelDecisionAction(
  payload: JsonRecord,
  decisionSource: unknown,
  workflowId: string,
  terminalId: string,
  promptContext: ReturnType<typeof extractPromptEventContext>
): TerminalPromptDecisionPayload {
  const topLevelDecisionAction = normalizePromptDecisionAction(
    getStringField(payload, 'action')
  );
  const normalizedPayload: TerminalPromptDecisionPayload = {
    workflowId,
    terminalId,
    ...promptContext,
    decision: topLevelDecisionAction,
  };

  if (topLevelDecisionAction === 'unknown' && decisionSource !== undefined) {
    normalizedPayload.decisionRaw = decisionSource;
  }

  return normalizedPayload;
}

function normalizeTerminalPromptDecisionPayload(payload: unknown): unknown {
  if (!isJsonRecord(payload)) {
    return payload;
  }

  const workflowId = getStringField(payload, 'workflowId', 'workflow_id');
  const terminalId = getStringField(payload, 'terminalId', 'terminal_id');

  if (!workflowId || !terminalId) {
    return payload;
  }

  const promptContext = extractPromptEventContext(payload);
  const decisionSource = payload.decision;

  if (typeof decisionSource === 'string') {
    return handleStringDecisionSource(
      decisionSource,
      payload,
      workflowId,
      terminalId,
      promptContext
    );
  }

  if (isJsonRecord(decisionSource)) {
    return handleObjectDecisionSource(
      decisionSource,
      workflowId,
      terminalId,
      promptContext
    );
  }

  return handleTopLevelDecisionAction(
    payload,
    decisionSource,
    workflowId,
    terminalId,
    promptContext
  );
}

function normalizeWorkflowEventPayload(
  eventType: string,
  payload: unknown
): unknown {
  switch (eventType) {
    case 'terminal.completed':
      return normalizeTerminalCompletedPayload(payload);
    case 'terminal.prompt_detected':
      return normalizeTerminalPromptDetectedPayload(payload);
    case 'terminal.prompt_decision':
      return normalizeTerminalPromptDecisionPayload(payload);
    case 'provider.switched':
      console.warn('[wsStore] Provider switched:', payload);
      return payload;
    case 'provider.exhausted':
      console.warn('[wsStore] Provider exhausted:', payload);
      return payload;
    case 'provider.recovered':
      console.warn('[wsStore] Provider recovered:', payload);
      return payload;
    // G08-006: Log lagged events so consumers (e.g. useWorkflowEvents) can
    // trigger invalidateQueries for a full state refresh.
    case 'system.lagged': {
      const skipped = isJsonRecord(payload) ? getNumberField(payload, 'skipped') : undefined;
      console.warn(`[wsStore] system.lagged — ${skipped ?? '?'} messages skipped`);
      return payload;
    }
    default:
      return payload;
  }
}

function extractWorkflowIdFromMessage(message: WsMessage): string | null {
  const messageWithWorkflow = message as WsMessage & { workflowId?: unknown };
  if (typeof messageWithWorkflow.workflowId === 'string') {
    return messageWithWorkflow.workflowId;
  }

  if (!isJsonRecord(message.payload)) {
    return null;
  }

  return (
    getStringField(message.payload, 'workflowId', 'workflow_id') ??
    getStringField(message.payload, 'workflow') ??
    null
  );
}

const LEGACY_CONNECTION_ID = '__legacy__';

function buildWorkflowEventsUrl(workflowId: string): string {
  const protocol = globalThis.window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = globalThis.window.location.host;
  return `${protocol}//${host}/api/ws/workflow/${workflowId}/events`;
}

function aggregateConnectionStatus(
  workflowConnections: Map<string, WorkflowScopedConnection>
): ConnectionStatus {
  const statuses = new Set(
    Array.from(workflowConnections.values()).map(
      (connection) => connection.status
    )
  );

  if (statuses.has('connected')) {
    return 'connected';
  }
  if (statuses.has('reconnecting')) {
    return 'reconnecting';
  }
  if (statuses.has('connecting')) {
    return 'connecting';
  }
  // G30-007 / G30-008: Internal 'error' and 'failed' states are mapped to
  // 'disconnected' for the public ConnectionStatus API. Consumers should
  // check lastError for details on why the connection dropped.
  return 'disconnected';
}

function aggregateLastHeartbeat(
  workflowConnections: Map<string, WorkflowScopedConnection>
): Date | null {
  let lastHeartbeat: Date | null = null;

  for (const connection of workflowConnections.values()) {
    if (
      !lastHeartbeat ||
      (connection.lastHeartbeat && connection.lastHeartbeat > lastHeartbeat)
    ) {
      lastHeartbeat = connection.lastHeartbeat ?? null;
    }
  }

  return lastHeartbeat;
}

function aggregateReconnectAttempts(
  workflowConnections: Map<string, WorkflowScopedConnection>
): number {
  let attempts = 0;

  for (const connection of workflowConnections.values()) {
    attempts = Math.max(attempts, connection.reconnectAttempts);
  }

  return attempts;
}

function internalToPublicStatus(status: InternalConnectionStatus): ConnectionStatus {
  if (status === 'error' || status === 'failed') {
    return 'disconnected';
  }
  return status;
}

function toWorkflowConnectionStatusRecord(
  workflowConnections: Map<string, WorkflowScopedConnection>
): Record<string, ConnectionStatus> {
  const record: Record<string, ConnectionStatus> = {};

  for (const [workflowId, connection] of workflowConnections.entries()) {
    record[workflowId] = internalToPublicStatus(connection.status);
  }

  return record;
}

function createWorkflowConnection(
  url: string,
  refCount: number
): WorkflowScopedConnection {
  return {
    ws: null,
    status: 'disconnected',
    reconnectAttempts: 0,
    heartbeatInterval: null,
    reconnectTimeout: null,
    manualDisconnect: false,
    url,
    refCount,
    lastHeartbeat: null,
  };
}

function clearConnectionTimers(connection: WorkflowScopedConnection): void {
  if (connection.heartbeatInterval) {
    clearInterval(connection.heartbeatInterval);
  }

  if (connection.reconnectTimeout) {
    clearTimeout(connection.reconnectTimeout);
  }
}

/**
 * WebSocket connection management store
 * Handles connection lifecycle, heartbeat, reconnection, and message routing
 */
export const useWsStore = create<WsState>((set, get) => ({
  // Initial state
  connectionStatus: 'disconnected',
  workflowConnectionStatus: {},
  lastHeartbeat: null,
  lastError: null,
  reconnectAttempts: 0,
  currentWorkflowId: null,

  // Internal state
  _ws: null,
  // G12-001: _handlers is a Map<eventType, Set<handler>> that is mutated in-place
  // by subscribe(). This is intentional — Zustand's set() is only used for
  // top-level state that drives React re-renders. Handler registration is a
  // side-channel concern that should NOT trigger component re-renders, so we
  // mutate the Map directly and rely on the closure captured by the WS message
  // handler to always read the latest reference via get()._handlers.
  _handlers: new Map(),
  _workflowHandlers: new Map(),
  _workflowConnections: new Map(),
  _heartbeatInterval: null,
  _reconnectTimeout: null,
  _url: null,
  _manualDisconnect: false,

  getWorkflowConnectionStatus: (workflowId: string) => {
    const internal = get()._workflowConnections.get(workflowId)?.status ?? 'disconnected';
    return internalToPublicStatus(internal);
  },

  subscribeToWorkflow: (
    workflowId: string,
    eventType: string,
    handler: MessageHandler
  ) => {
    const workflowHandlers = get()._workflowHandlers;

    if (!workflowHandlers.has(workflowId)) {
      workflowHandlers.set(workflowId, new Map());
    }

    const eventHandlers = workflowHandlers.get(workflowId)!;
    if (!eventHandlers.has(eventType)) {
      eventHandlers.set(eventType, new Set());
    }

    eventHandlers.get(eventType)!.add(handler);

    return () => {
      const currentWorkflowHandlers = get()._workflowHandlers.get(workflowId);
      if (!currentWorkflowHandlers) {
        return;
      }

      const currentHandlers = currentWorkflowHandlers.get(eventType);
      if (!currentHandlers) {
        return;
      }

      currentHandlers.delete(handler);

      if (currentHandlers.size === 0) {
        currentWorkflowHandlers.delete(eventType);
      }

      if (currentWorkflowHandlers.size === 0) {
        get()._workflowHandlers.delete(workflowId);
      }
    };
  },

  disconnectWorkflow: (workflowId: string) => {
    const state = get();
    const connection = state._workflowConnections.get(workflowId);

    if (!connection) {
      return;
    }

    const nextConnections = new Map(state._workflowConnections);

    if (connection.refCount > 1) {
      nextConnections.set(workflowId, {
        ...connection,
        refCount: connection.refCount - 1,
      });
    } else {
      clearConnectionTimers(connection);

      if (connection.ws?.readyState !== undefined && connection.ws.readyState < WebSocket.CLOSING) {
        connection.ws.close();
      }

      nextConnections.delete(workflowId);

      const nextWorkflowHandlers = new Map(state._workflowHandlers);
      nextWorkflowHandlers.delete(workflowId);
      set({ _workflowHandlers: nextWorkflowHandlers });
    }

    const currentWorkflowId =
      nextConnections.has(state.currentWorkflowId ?? '')
        ? state.currentWorkflowId
        : (nextConnections.keys().next().value ?? null);
    const activeConnection = currentWorkflowId
      ? (nextConnections.get(currentWorkflowId) ?? null)
      : null;

    set({
      _workflowConnections: nextConnections,
      currentWorkflowId,
      _ws: activeConnection?.ws ?? null,
      _heartbeatInterval: activeConnection?.heartbeatInterval ?? null,
      _reconnectTimeout: activeConnection?.reconnectTimeout ?? null,
      _url: activeConnection?.url ?? null,
      _manualDisconnect: activeConnection?.manualDisconnect ?? false,
      workflowConnectionStatus:
        toWorkflowConnectionStatusRecord(nextConnections),
      connectionStatus: aggregateConnectionStatus(nextConnections),
      lastHeartbeat: aggregateLastHeartbeat(nextConnections),
      reconnectAttempts: aggregateReconnectAttempts(nextConnections),
    });
  },

  connectToWorkflow: (workflowId: string) => {
    // Helper: Create heartbeat interval handler
    // TODO: G12-008 — Track server heartbeat acknowledgement (pong) and trigger
    // reconnect if no pong is received within 2x the heartbeat interval.
    const createHeartbeatHandler = (targetWorkflowId: string) => {
      return () => {
        const latest = get()._workflowConnections.get(targetWorkflowId);
        if (latest?.ws?.readyState !== WebSocket.OPEN) return;

        const heartbeatMessage: WsMessage = {
          type: 'system.heartbeat',
          payload: {},
          timestamp: new Date().toISOString(),
          id: generateMessageId(),
        };

        latest.ws.send(JSON.stringify(heartbeatMessage));

        const heartbeatConnections = new Map(get()._workflowConnections);
        const active = heartbeatConnections.get(targetWorkflowId);
        if (!active || active.ws !== latest.ws) return;

        heartbeatConnections.set(targetWorkflowId, {
          ...active,
          lastHeartbeat: new Date(),
        });

        const activeConnection = heartbeatConnections.get(targetWorkflowId)!;
        set({
          _workflowConnections: heartbeatConnections,
          _ws: activeConnection.ws,
          _heartbeatInterval: activeConnection.heartbeatInterval,
          _reconnectTimeout: activeConnection.reconnectTimeout,
          _url: activeConnection.url,
          _manualDisconnect: activeConnection.manualDisconnect,
          workflowConnectionStatus: toWorkflowConnectionStatusRecord(heartbeatConnections),
          connectionStatus: aggregateConnectionStatus(heartbeatConnections),
          lastHeartbeat: aggregateLastHeartbeat(heartbeatConnections),
          reconnectAttempts: aggregateReconnectAttempts(heartbeatConnections),
        });
      };
    };

    // Helper: Handle WebSocket open event
    const handleWebSocketOpen = (targetWorkflowId: string, ws: WebSocket, isStale: () => boolean) => {
      return () => {
        if (isStale()) return;

        const heartbeatInterval = setInterval(
          createHeartbeatHandler(targetWorkflowId),
          HEARTBEAT_INTERVAL
        );

        const connectedConnections = new Map(get()._workflowConnections);
        const active = connectedConnections.get(targetWorkflowId);
        if (active?.ws !== ws) {
          clearInterval(heartbeatInterval);
          return;
        }

        connectedConnections.set(targetWorkflowId, {
          ...active,
          status: 'connected',
          reconnectAttempts: 0,
          heartbeatInterval,
          reconnectTimeout: null,
        });

        const activeConnection = connectedConnections.get(targetWorkflowId)!;
        set({
          _workflowConnections: connectedConnections,
          currentWorkflowId: targetWorkflowId,
          _ws: activeConnection.ws,
          _heartbeatInterval: activeConnection.heartbeatInterval,
          _reconnectTimeout: activeConnection.reconnectTimeout,
          _url: activeConnection.url,
          _manualDisconnect: activeConnection.manualDisconnect,
          workflowConnectionStatus: toWorkflowConnectionStatusRecord(connectedConnections),
          connectionStatus: aggregateConnectionStatus(connectedConnections),
          lastHeartbeat: aggregateLastHeartbeat(connectedConnections),
          reconnectAttempts: aggregateReconnectAttempts(connectedConnections),
        });
      };
    };

    // Helper: Handle message handlers execution
    const executeMessageHandlers = (
      handlers: Set<MessageHandler> | undefined,
      payload: unknown,
      messageType: string,
      targetWorkflowId: string,
      handlerType: 'Global' | 'Workflow'
    ) => {
      if (!handlers) return;

      handlers.forEach((handler) => {
        try {
          handler(payload);
        } catch (error) {
          console.error(
            `[wsStore] ${handlerType} handler failed for event "${messageType}" (workflow "${targetWorkflowId}")`,
            error
          );
        }
      });
    };

    // Helper: Handle WebSocket message event
    const handleWebSocketMessage = (targetWorkflowId: string, isStale: () => boolean) => {
      return (event: MessageEvent) => {
        if (isStale()) return;

        try {
          const message = JSON.parse(event.data as string) as WsMessage;
          const currentStateForMessage = get();
          const globalHandlers = currentStateForMessage._handlers.get(message.type);
          const scopedHandlers = currentStateForMessage._workflowHandlers
            .get(targetWorkflowId)
            ?.get(message.type);

          if (!globalHandlers && !scopedHandlers) return;

          const normalizedPayload = normalizeWorkflowEventPayload(
            message.type,
            message.payload
          );

          executeMessageHandlers(globalHandlers, normalizedPayload, message.type, targetWorkflowId, 'Global');
          executeMessageHandlers(scopedHandlers, normalizedPayload, message.type, targetWorkflowId, 'Workflow');
        } catch (error) {
          // Structured error logging for message parse failures
          const errorMessage = error instanceof Error ? error.message : 'Unknown parse error';
          console.error(
            `[wsStore] Failed to parse inbound message for workflow "${targetWorkflowId}":`,
            errorMessage
          );
          // Notify system.error subscribers of the parse failure
          const parseErrorHandlers = get()._handlers.get('system.error');
          if (parseErrorHandlers) {
            const parseErrorPayload: SystemErrorPayload = {
              workflowId: targetWorkflowId,
              error: 'message_parse_failed',
              message: errorMessage,
            };
            parseErrorHandlers.forEach((handler) => {
              try {
                handler(parseErrorPayload);
              } catch (handlerErr) {
                console.error('[wsStore] system.error handler failed', handlerErr);
              }
            });
          }
        }
      };
    };

    // Helper: Handle manual disconnect or zero refCount
    const handleManualDisconnect = (
      targetWorkflowId: string,
      ws: WebSocket,
      currentStateForClose: ReturnType<typeof get>
    ) => {
      const closedConnections = new Map(currentStateForClose._workflowConnections);
      const active = closedConnections.get(targetWorkflowId);

      if (active?.ws !== ws) return;

      closedConnections.set(targetWorkflowId, {
        ...active,
        ws: null,
        status: 'disconnected',
        heartbeatInterval: null,
        reconnectTimeout: null,
      });

      const activeConnection = closedConnections.get(targetWorkflowId)!;
      set({
        _workflowConnections: closedConnections,
        _ws: activeConnection.ws,
        _heartbeatInterval: activeConnection.heartbeatInterval,
        _reconnectTimeout: activeConnection.reconnectTimeout,
        _url: activeConnection.url,
        _manualDisconnect: activeConnection.manualDisconnect,
        workflowConnectionStatus: toWorkflowConnectionStatusRecord(closedConnections),
        connectionStatus: aggregateConnectionStatus(closedConnections),
        lastHeartbeat: aggregateLastHeartbeat(closedConnections),
        reconnectAttempts: aggregateReconnectAttempts(closedConnections),
      });
    };

    // Helper: Handle reconnection attempt
    const handleReconnectAttempt = (
      targetWorkflowId: string,
      ws: WebSocket,
      attempts: number,
      currentStateForClose: ReturnType<typeof get>,
      openConnection: (id: string) => void
    ) => {
      const delay = BASE_RECONNECT_DELAY * Math.pow(2, attempts - 1);
      const reconnectTimeout = setTimeout(() => {
        const latest = get()._workflowConnections.get(targetWorkflowId);
        if (!latest || latest.manualDisconnect || latest.refCount <= 0) return;
        openConnection(targetWorkflowId);
      }, delay);

      const reconnectingConnections = new Map(currentStateForClose._workflowConnections);
      const active = reconnectingConnections.get(targetWorkflowId);

      if (active?.ws !== ws) {
        clearTimeout(reconnectTimeout);
        return;
      }

      reconnectingConnections.set(targetWorkflowId, {
        ...active,
        ws: null,
        status: 'reconnecting',
        reconnectAttempts: attempts,
        heartbeatInterval: null,
        reconnectTimeout,
      });

      const activeConnection = reconnectingConnections.get(targetWorkflowId)!;
      set({
        _workflowConnections: reconnectingConnections,
        _ws: activeConnection.ws,
        _heartbeatInterval: activeConnection.heartbeatInterval,
        _reconnectTimeout: activeConnection.reconnectTimeout,
        _url: activeConnection.url,
        _manualDisconnect: activeConnection.manualDisconnect,
        workflowConnectionStatus: toWorkflowConnectionStatusRecord(reconnectingConnections),
        connectionStatus: aggregateConnectionStatus(reconnectingConnections),
        lastHeartbeat: aggregateLastHeartbeat(reconnectingConnections),
        reconnectAttempts: aggregateReconnectAttempts(reconnectingConnections),
      });
    };

    // Helper: Handle max reconnect attempts exceeded
    // G30-008: Set status to 'failed' (not 'disconnected') and expose lastError
    // so consumers can distinguish "gave up reconnecting" from "intentionally closed".
    const handleMaxReconnectExceeded = (
      targetWorkflowId: string,
      ws: WebSocket,
      currentStateForClose: ReturnType<typeof get>
    ) => {
      const disconnectedConnections = new Map(currentStateForClose._workflowConnections);
      const active = disconnectedConnections.get(targetWorkflowId);
      if (active?.ws !== ws) return;

      const failedErrorMsg = `WebSocket reconnection failed after ${MAX_RECONNECT_ATTEMPTS} attempts`;

      disconnectedConnections.set(targetWorkflowId, {
        ...active,
        ws: null,
        status: 'failed',
        heartbeatInterval: null,
        reconnectTimeout: null,
      });

      const activeConnection = disconnectedConnections.get(targetWorkflowId)!;
      set({
        _workflowConnections: disconnectedConnections,
        _ws: activeConnection.ws,
        _heartbeatInterval: activeConnection.heartbeatInterval,
        _reconnectTimeout: activeConnection.reconnectTimeout,
        _url: activeConnection.url,
        _manualDisconnect: activeConnection.manualDisconnect,
        workflowConnectionStatus: toWorkflowConnectionStatusRecord(disconnectedConnections),
        connectionStatus: aggregateConnectionStatus(disconnectedConnections),
        lastHeartbeat: aggregateLastHeartbeat(disconnectedConnections),
        reconnectAttempts: aggregateReconnectAttempts(disconnectedConnections),
        lastError: failedErrorMsg,
      });

      // Notify subscribers that reconnection has been exhausted
      console.error(
        `[wsStore] Max reconnect attempts (${MAX_RECONNECT_ATTEMPTS}) reached for workflow "${targetWorkflowId}"`
      );
      const errorHandlers = get()._handlers.get('system.error');
      if (errorHandlers) {
        const errorPayload: SystemErrorPayload = {
          workflowId: targetWorkflowId,
          error: 'max_reconnect_exceeded',
          message: `WebSocket reconnection failed after ${MAX_RECONNECT_ATTEMPTS} attempts`,
        };
        errorHandlers.forEach((handler) => {
          try {
            handler(errorPayload);
          } catch (err) {
            console.error('[wsStore] system.error handler failed', err);
          }
        });
      }
    };

    // Helper: Handle WebSocket close event
    const handleWebSocketClose = (
      targetWorkflowId: string,
      ws: WebSocket,
      isStale: () => boolean,
      openConnection: (id: string) => void
    ) => {
      return () => {
        if (isStale()) return;

        const currentStateForClose = get();
        const connection = currentStateForClose._workflowConnections.get(targetWorkflowId);

        if (!connection) return;

        if (connection.heartbeatInterval) {
          clearInterval(connection.heartbeatInterval);
        }

        if (connection.manualDisconnect || connection.refCount <= 0) {
          handleManualDisconnect(targetWorkflowId, ws, currentStateForClose);
          return;
        }

        const attempts = connection.reconnectAttempts + 1;

        if (attempts <= MAX_RECONNECT_ATTEMPTS) {
          handleReconnectAttempt(targetWorkflowId, ws, attempts, currentStateForClose, openConnection);
        } else {
          handleMaxReconnectExceeded(targetWorkflowId, ws, currentStateForClose);
        }
      };
    };

    const openConnection = (targetWorkflowId: string) => {
      const currentState = get();
      const currentConnection = currentState._workflowConnections.get(targetWorkflowId);

      if (!currentConnection) return;

      if (
        currentConnection.ws &&
        (currentConnection.ws.readyState === WebSocket.CONNECTING ||
          currentConnection.ws.readyState === WebSocket.OPEN)
      ) {
        return;
      }

      clearConnectionTimers(currentConnection);

      const ws = new WebSocket(currentConnection.url);
      const connectingConnections = new Map(currentState._workflowConnections);

      connectingConnections.set(targetWorkflowId, {
        ...currentConnection,
        ws,
        status: currentConnection.reconnectAttempts > 0 ? 'reconnecting' : 'connecting',
        manualDisconnect: false,
        heartbeatInterval: null,
        reconnectTimeout: null,
      });

      const connectingConnection = connectingConnections.get(targetWorkflowId)!;
      set({
        _workflowConnections: connectingConnections,
        currentWorkflowId: targetWorkflowId,
        _ws: connectingConnection.ws,
        _heartbeatInterval: connectingConnection.heartbeatInterval,
        _reconnectTimeout: connectingConnection.reconnectTimeout,
        _url: connectingConnection.url,
        _manualDisconnect: connectingConnection.manualDisconnect,
        workflowConnectionStatus: toWorkflowConnectionStatusRecord(connectingConnections),
        connectionStatus: aggregateConnectionStatus(connectingConnections),
        lastHeartbeat: aggregateLastHeartbeat(connectingConnections),
        reconnectAttempts: aggregateReconnectAttempts(connectingConnections),
      });

      const isStale = () => get()._workflowConnections.get(targetWorkflowId)?.ws !== ws;

      ws.onopen = handleWebSocketOpen(targetWorkflowId, ws, isStale);
      ws.onmessage = handleWebSocketMessage(targetWorkflowId, isStale);
      ws.onclose = handleWebSocketClose(targetWorkflowId, ws, isStale, openConnection);
      ws.onerror = () => {
        if (isStale()) return;
        const errorMsg = `WebSocket error on workflow "${targetWorkflowId}"`;
        console.error(`[wsStore] ${errorMsg}`);

        // G30-007: Update connection status to 'error' (not 'reconnecting')
        // so consumers can distinguish transport errors from intentional reconnects.
        const errorConnections = new Map(get()._workflowConnections);
        const errConn = errorConnections.get(targetWorkflowId);
        if (errConn?.ws === ws) {
          errorConnections.set(targetWorkflowId, {
            ...errConn,
            status: 'error',
          });
          set({
            _workflowConnections: errorConnections,
            workflowConnectionStatus: toWorkflowConnectionStatusRecord(errorConnections),
            connectionStatus: aggregateConnectionStatus(errorConnections),
            lastError: errorMsg,
          });
        }
      };
    };

    const state = get();
    const existingConnection = state._workflowConnections.get(workflowId);
    const nextConnections = new Map(state._workflowConnections);

    if (existingConnection) {
      nextConnections.set(workflowId, {
        ...existingConnection,
        refCount: existingConnection.refCount + 1,
        manualDisconnect: false,
        reconnectAttempts: 0,
        status: existingConnection.status === 'failed' ? 'disconnected' : existingConnection.status,
      });
    } else {
      nextConnections.set(workflowId, {
        ...createWorkflowConnection(buildWorkflowEventsUrl(workflowId), 1),
      });
    }

    const currentConnection = nextConnections.get(workflowId)!;
    set({
      _workflowConnections: nextConnections,
      currentWorkflowId: workflowId,
      _ws: currentConnection.ws,
      _heartbeatInterval: currentConnection.heartbeatInterval,
      _reconnectTimeout: currentConnection.reconnectTimeout,
      _url: currentConnection.url,
      _manualDisconnect: currentConnection.manualDisconnect,
      workflowConnectionStatus:
        toWorkflowConnectionStatusRecord(nextConnections),
      connectionStatus: aggregateConnectionStatus(nextConnections),
      lastHeartbeat: aggregateLastHeartbeat(nextConnections),
      reconnectAttempts: aggregateReconnectAttempts(nextConnections),
    });

    openConnection(workflowId);
  },

  connect: (url: string) => {
    const state = get();

    const legacyConnection =
      state._workflowConnections.get(LEGACY_CONNECTION_ID);
    if (legacyConnection) {
      clearConnectionTimers(legacyConnection);
      if (
        legacyConnection.ws &&
        legacyConnection.ws.readyState < WebSocket.CLOSING
      ) {
        legacyConnection.ws.close();
      }
    }

    const nextConnections = new Map(state._workflowConnections);
    nextConnections.set(LEGACY_CONNECTION_ID, createWorkflowConnection(url, 0));

    const activeConnection = nextConnections.get(LEGACY_CONNECTION_ID)!;
    set({
      _workflowConnections: nextConnections,
      currentWorkflowId: LEGACY_CONNECTION_ID,
      _ws: activeConnection.ws,
      _heartbeatInterval: activeConnection.heartbeatInterval,
      _reconnectTimeout: activeConnection.reconnectTimeout,
      _url: activeConnection.url,
      _manualDisconnect: activeConnection.manualDisconnect,
      workflowConnectionStatus:
        toWorkflowConnectionStatusRecord(nextConnections),
      connectionStatus: aggregateConnectionStatus(nextConnections),
      lastHeartbeat: aggregateLastHeartbeat(nextConnections),
      reconnectAttempts: aggregateReconnectAttempts(nextConnections),
    });

    get().connectToWorkflow(LEGACY_CONNECTION_ID);
  },

  disconnect: () => {
    const state = get();

    for (const connection of state._workflowConnections.values()) {
      clearConnectionTimers(connection);

      if (connection.ws?.readyState !== undefined && connection.ws.readyState < WebSocket.CLOSING) {
        connection.ws.close();
      }
    }

    // G12-003: Explicitly clear the old Map instances so any lingering
    // references (e.g. closures from a not-yet-closed WS) see empty maps.
    // Then replace with fresh Maps via set() for the store itself.
    state._handlers.clear();
    state._workflowHandlers.clear();

    set({
      connectionStatus: 'disconnected',
      workflowConnectionStatus: {},
      lastHeartbeat: null,
      lastError: null,
      currentWorkflowId: null,
      _ws: null,
      _workflowConnections: new Map(),
      _handlers: new Map(),
      _workflowHandlers: new Map(),
      _heartbeatInterval: null,
      _reconnectTimeout: null,
      _url: null,
      _manualDisconnect: true,
      reconnectAttempts: 0,
    });
  },

  send: (message: WsMessage) => {
    const state = get();
    const targetWorkflowId =
      extractWorkflowIdFromMessage(message) ?? state.currentWorkflowId;

    if (targetWorkflowId) {
      const targetConnection = state._workflowConnections.get(targetWorkflowId);
      if (targetConnection?.ws?.readyState === WebSocket.OPEN) {
        targetConnection.ws.send(JSON.stringify(message));
        return true;
      }

      console.warn(
        `[wsStore] Cannot send "${message.type}" because workflow "${targetWorkflowId}" is not connected`
      );
      return false;
    }

    const legacyConnection =
      state._workflowConnections.get(LEGACY_CONNECTION_ID);
    if (legacyConnection?.ws?.readyState === WebSocket.OPEN) {
      legacyConnection.ws.send(JSON.stringify(message));
      return true;
    }

    console.warn(
      `[wsStore] Cannot send "${message.type}" because there is no active WebSocket connection`
    );
    return false;
  },

  // Send a response for terminal interactive prompts via workflow-scoped WebSocket.
  // G27-007: workflowId is included both at the top-level (for routing) and
  // inside payload (for backward compatibility with existing backend handlers).
  sendPromptResponse: (payload: TerminalPromptResponsePayload) => {
    const { workflowId, terminalId, response } = payload;
    return get().send({
      type: 'terminal.prompt_response',
      workflowId,
      payload: {
        workflowId,
        terminalId,
        response,
      },
      timestamp: new Date().toISOString(),
      id: generateMessageId(),
    } as WsMessage);
  },

  // Design decision: _handlers and _workflowHandlers are mutated directly
  // (bypassing Zustand's set()) because they are internal subscription registries
  // that don't drive React renders. Routing through set() would trigger unnecessary
  // re-renders on every subscribe/unsubscribe call. Only connection-status fields
  // go through set() since those are consumed by React components.
  subscribe: (eventType: string, handler: MessageHandler) => {
    const handlers = get()._handlers;

    if (!handlers.has(eventType)) {
      handlers.set(eventType, new Set());
    }

    handlers.get(eventType)!.add(handler);

    // Return unsubscribe function
    return () => {
      const currentHandlers = get()._handlers.get(eventType);
      if (currentHandlers) {
        currentHandlers.delete(handler);
        if (currentHandlers.size === 0) {
          get()._handlers.delete(eventType);
        }
      }
    };
  },
}));

/**
 * Hook to subscribe to WebSocket events
 * Automatically unsubscribes on unmount
 */
export function useWsSubscription(
  eventType: string,
  handler: MessageHandler
) {
  const subscribe = useWsStore((s) => s.subscribe);

  // Use effect to manage subscription lifecycle
  React.useEffect(() => {
    return subscribe(eventType, handler);
  }, [eventType, handler, subscribe]);
}

/**
 * Payload types for workflow events
 */
export interface WorkflowStatusPayload {
  workflowId: string;
  status: string;
}

export interface TerminalStatusPayload {
  workflowId: string;
  terminalId: string;
  status: string;
}

export interface TaskStatusPayload {
  workflowId: string;
  taskId: string;
  status: string;
}

export interface GitCommitPayload {
  workflowId: string;
  commitHash: string;
  branch: string;
  message: string;
}

export type TerminalCompletedStatus =
  | 'completed'
  | 'failed'
  | 'checkpoint'
  | 'review_pass'
  | 'review_reject'
  | 'unknown';

export interface TerminalCompletedPayload {
  workflowId: string;
  taskId: string;
  terminalId: string;
  status: TerminalCompletedStatus;
  commitHash?: string;
  commitMessage?: string;
  statusRaw?: string;
  metadata?: unknown;
}

export interface SystemLaggedPayload {
  skipped: number;
}

export interface SystemErrorPayload {
  workflowId?: string;
  error?: string;
  message?: string;
}

export interface QualityGateResultPayload {
  workflowId: string;
  taskId: string;
  terminalId: string;
  qualityRunId: string;
  gateStatus: string;
  mode: string;
  totalIssues: number;
  blockingIssues: number;
  newIssues: number;
  passed: boolean;
  summary: string;
  /** G31-002: Optional commit hash associated with this quality gate run */
  commitHash?: string;
}

export type TerminalPromptKind =
  | 'enter_confirm'
  | 'yes_no'
  | 'choice'
  | 'arrow_select'
  | 'input'
  | 'password'
  | 'unknown';

export interface TerminalPromptOption {
  index: number;
  label: string;
  selected: boolean;
  [key: string]: unknown;
}

export interface PromptEventContext {
  workflowId: string;
  terminalId: string;
  taskId?: string;
  sessionId?: string;
  autoConfirm?: boolean;
}

export interface TerminalPromptDetectedPayload extends PromptEventContext {
  promptKind: TerminalPromptKind;
  promptText: string;
  confidence: number;
  hasDangerousKeywords: boolean;
  options: string[];
  selectedIndex: number | null;
  optionDetails?: TerminalPromptOption[];
  promptKindRaw?: string;
  legacyPromptKind?: string;
  detectedAt?: string;
  [key: string]: unknown;
}

export type PromptDecisionAction =
  | 'auto_confirm'
  | 'llm_decision'
  | 'ask_user'
  | 'skip'
  | 'unknown';

export interface PromptDecisionDetail {
  action?: string;
  response?: string;
  reason?: string;
  reasoning?: string;
  suggestions?: string[] | null;
  targetIndex?: number | null;
  [key: string]: unknown;
}

export interface TerminalPromptDecisionPayload extends PromptEventContext {
  decision: PromptDecisionAction;
  decisionDetail?: PromptDecisionDetail;
  decisionRaw?: unknown;
  decidedAt?: string;
  [key: string]: unknown;
}

// G08-002: Provider event payload types
// TODO: Refine these payload types once the backend provider event schema stabilizes
export interface ProviderSwitchedPayload {
  workflowId: string;
  terminalId?: string;
  fromProvider?: string;
  toProvider?: string;
  reason?: string;
}

export interface ProviderExhaustedPayload {
  workflowId: string;
  terminalId?: string;
  provider?: string;
  error?: string;
}

export interface ProviderRecoveredPayload {
  workflowId: string;
  terminalId?: string;
  provider?: string;
}

export type WorkflowEventHandlers = {
  onWorkflowStatusChanged?: (payload: WorkflowStatusPayload) => void;
  onTerminalStatusChanged?: (payload: TerminalStatusPayload) => void;
  onTaskStatusChanged?: (payload: TaskStatusPayload) => void;
  onTerminalCompleted?: (payload: TerminalCompletedPayload) => void;
  onTerminalPromptDetected?: (payload: TerminalPromptDetectedPayload) => void;
  onTerminalPromptDecision?: (payload: TerminalPromptDecisionPayload) => void;
  onGitCommitDetected?: (payload: GitCommitPayload) => void;
  onQualityGateResult?: (payload: QualityGateResultPayload) => void;
  onSystemError?: (payload: SystemErrorPayload) => void;
  onSystemLagged?: (payload: SystemLaggedPayload) => void;
  // G08-002 / G08-008: Provider failover event handlers
  onProviderSwitched?: (payload: ProviderSwitchedPayload) => void;
  onProviderExhausted?: (payload: ProviderExhaustedPayload) => void;
  onProviderRecovered?: (payload: ProviderRecoveredPayload) => void;
} & Record<string, unknown>;

/**
 * Hook to connect to workflow events and subscribe to specific event types
 * Automatically connects on mount and disconnects on unmount
 *
 * Uses useRef internally to cache handlers, avoiding unsubscribe/resubscribe
 * churn when callers pass unstable handler references (e.g. inline callbacks).
 */
export function useWorkflowEvents(
  workflowId: string | null | undefined,
  handlers?: WorkflowEventHandlers
) {
  const connectToWorkflow = useWsStore((s) => s.connectToWorkflow);
  const disconnectWorkflow = useWsStore((s) => s.disconnectWorkflow);
  const subscribeToWorkflow = useWsStore((s) => s.subscribeToWorkflow);
  const connectionStatus = useWsStore((s) =>
    workflowId
      ? (s.workflowConnectionStatus[workflowId] ?? 'disconnected')
      : s.connectionStatus
  );

  // Cache handlers in a ref so subscriptions remain stable across renders
  const handlersRef = React.useRef(handlers);
  React.useEffect(() => {
    handlersRef.current = handlers;
  });

  // Connect to workflow on mount, disconnect on unmount
  React.useEffect(() => {
    if (workflowId) {
      connectToWorkflow(workflowId);
    }

    return () => {
      if (workflowId) {
        disconnectWorkflow(workflowId);
      }
    };
  }, [workflowId, connectToWorkflow, disconnectWorkflow]);

  // Subscribe to events — stable subscriptions via handlersRef
  React.useEffect(() => {
    if (!workflowId) return;

    const unsubscribers: (() => void)[] = [];

    const eventMap: Array<[keyof WorkflowEventHandlers, string]> = [
      ['onWorkflowStatusChanged', 'workflow.status_changed'],
      ['onTerminalStatusChanged', 'terminal.status_changed'],
      ['onTaskStatusChanged', 'task.status_changed'],
      ['onTerminalCompleted', 'terminal.completed'],
      ['onTerminalPromptDetected', 'terminal.prompt_detected'],
      ['onTerminalPromptDecision', 'terminal.prompt_decision'],
      ['onGitCommitDetected', 'git.commit_detected'],
      ['onQualityGateResult', 'quality.gate_result'],
      ['onSystemError', 'system.error'],
      ['onSystemLagged', 'system.lagged'],
      // G08-008 / G17-002: Provider failover events
      ['onProviderSwitched', 'provider.switched'],
      ['onProviderExhausted', 'provider.exhausted'],
      ['onProviderRecovered', 'provider.recovered'],
    ];

    for (const [handlerKey, eventType] of eventMap) {
      unsubscribers.push(
        subscribeToWorkflow(
          workflowId,
          eventType,
          ((payload: unknown) => {
            const fn = handlersRef.current?.[handlerKey];
            if (typeof fn === 'function') {
              (fn as MessageHandler)(payload);
            }
          }) as MessageHandler
        )
      );
    }

    return () => {
      unsubscribers.forEach((unsub) => unsub());
    };
    // Only re-subscribe when workflowId changes — handlers are read from ref
  }, [workflowId, subscribeToWorkflow]);

  return { connectionStatus };
}
