import { create } from 'zustand';
import { secureRandomIdFragment } from '@/utils/id';

// ── Types ──────────────────────────────────────────────────────────────────

export type ConciergeWsEventType =
  | 'concierge.message'
  | 'concierge.tool_start'
  | 'concierge.tool_result'
  | 'concierge.error'
  | 'system.heartbeat';

type ConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';
type MessageHandler = (payload: unknown) => void;

interface ConciergeWsState {
  // Public state
  connectionStatus: ConnectionStatus;
  lastError: string | null;

  // Internal
  _ws: WebSocket | null;
  _sessionId: string | null;
  _handlers: Map<string, Set<MessageHandler>>;
  _heartbeatInterval: ReturnType<typeof setInterval> | null;
  _reconnectTimeout: ReturnType<typeof setTimeout> | null;
  _reconnectAttempts: number;
  _manualDisconnect: boolean;
  _connectDebounceTimer: ReturnType<typeof setTimeout> | null;

  // Actions
  connect: (sessionId: string) => void;
  disconnect: (sessionId: string) => void;
  subscribe: (eventType: string, handler: MessageHandler) => () => void;
}

// ── Constants ──────────────────────────────────────────────────────────────

const HEARTBEAT_INTERVAL = 30_000;
const MAX_RECONNECT_ATTEMPTS = 5;
const BASE_RECONNECT_DELAY = 1_000;

function generateMessageId(): string {
  return `msg-${Date.now()}-${secureRandomIdFragment(7)}`;
}

function buildConciergeWsUrl(sessionId: string): string {
  const protocol =
    globalThis.window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = globalThis.window.location.host;
  return `${protocol}//${host}/api/ws/concierge/${sessionId}/events`;
}

// ── Store ──────────────────────────────────────────────────────────────────

export const useConciergeWsStore = create<ConciergeWsState>((set, get) => {
  function cleanup() {
    const state = get();
    if (state._connectDebounceTimer) clearTimeout(state._connectDebounceTimer);
    if (state._heartbeatInterval) clearInterval(state._heartbeatInterval);
    if (state._reconnectTimeout) clearTimeout(state._reconnectTimeout);
    if (state._ws) {
      state._ws.onopen = null;
      state._ws.onclose = null;
      state._ws.onmessage = null;
      state._ws.onerror = null;
      if (
        state._ws.readyState === WebSocket.OPEN ||
        state._ws.readyState === WebSocket.CONNECTING
      ) {
        state._ws.close();
      }
    }
    set({
      _ws: null,
      _connectDebounceTimer: null,
      _heartbeatInterval: null,
      _reconnectTimeout: null,
    });
  }

  function startHeartbeat(ws: WebSocket) {
    const interval = setInterval(() => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'system.heartbeat',
            payload: {},
            timestamp: new Date().toISOString(),
            id: generateMessageId(),
          })
        );
      }
    }, HEARTBEAT_INTERVAL);
    set({ _heartbeatInterval: interval });
  }

  function scheduleReconnect() {
    const state = get();
    if (state._manualDisconnect) return;
    if (state._reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
      set({ connectionStatus: 'disconnected', lastError: 'Max reconnect attempts reached' });
      return;
    }
    const delay = BASE_RECONNECT_DELAY * 2 ** state._reconnectAttempts;
    set({ connectionStatus: 'reconnecting', _reconnectAttempts: state._reconnectAttempts + 1 });
    const timeout = setTimeout(() => {
      const current = get();
      if (current._sessionId) {
        current.connect(current._sessionId);
      }
    }, delay);
    set({ _reconnectTimeout: timeout });
  }

  function dispatchMessage(type: string, payload: unknown) {
    const handlers = get()._handlers.get(type);
    if (handlers) {
      for (const handler of handlers) {
        try {
          handler(payload);
        } catch (handlerErr) {
          console.error(
            `[conciergeWsStore] handler for "${type}" failed`,
            handlerErr
          );
        }
      }
    }
  }

  return {
    connectionStatus: 'disconnected',
    lastError: null,
    _ws: null,
    _sessionId: null,
    _handlers: new Map(),
    _heartbeatInterval: null,
    _reconnectTimeout: null,
    _reconnectAttempts: 0,
    _manualDisconnect: false,
    _connectDebounceTimer: null,

    connect(sessionId: string) {
      const state = get();
      // Already connected to the same session
      if (state._sessionId === sessionId && state._ws?.readyState === WebSocket.OPEN) {
        return;
      }

      cleanup();
      set({
        connectionStatus: 'connecting',
        _sessionId: sessionId,
        _manualDisconnect: false,
        lastError: null,
      });

      // Debounce actual WebSocket creation to prevent rapid connect/disconnect
      // cycles caused by React StrictMode or effect re-runs
      const timer = setTimeout(() => {
        // Re-check: if disconnected manually during debounce, abort
        if (get()._manualDisconnect) return;

        const url = buildConciergeWsUrl(sessionId);
        const ws = new WebSocket(url);

        ws.onopen = () => {
          set({ _ws: ws, connectionStatus: 'connected', _reconnectAttempts: 0 });
          startHeartbeat(ws);
        };

        ws.onmessage = (event) => {
          try {
            const message = JSON.parse(event.data as string) as {
              type: string;
              payload: unknown;
            };
            dispatchMessage(message.type, message.payload);
          } catch {
            // ignore malformed messages
          }
        };

        ws.onerror = () => {
          set({ lastError: 'WebSocket error' });
        };

        ws.onclose = () => {
          cleanup();
          set({ connectionStatus: 'disconnected' });
          scheduleReconnect();
        };

        set({ _ws: ws, _connectDebounceTimer: null });
      }, 150);

      set({ _connectDebounceTimer: timer });
    },

    disconnect(_sessionId: string) {
      set({ _manualDisconnect: true });
      cleanup();
      set({
        connectionStatus: 'disconnected',
        _sessionId: null,
        _reconnectAttempts: 0,
      });
    },

    subscribe(eventType: string, handler: MessageHandler) {
      const handlers = get()._handlers;
      if (!handlers.has(eventType)) {
        handlers.set(eventType, new Set());
      }
      handlers.get(eventType)!.add(handler);

      // Return unsubscribe function
      return () => {
        const set = handlers.get(eventType);
        if (set) {
          set.delete(handler);
          if (set.size === 0) handlers.delete(eventType);
        }
      };
    },
  };
});
