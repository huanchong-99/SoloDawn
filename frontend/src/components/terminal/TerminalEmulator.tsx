import { useEffect, useRef, useCallback, forwardRef, useImperativeHandle, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { isWsOutputMessage, isWsErrorMessage } from '@/types/websocket';

const TERMINAL_ID_REGEX =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const isTestEnv = import.meta.env.MODE === 'test' || import.meta.env.VITEST;

const logInfo = (...args: unknown[]) => {
  if (isTestEnv) return;
  console.log(...args);
};

const DISCONNECTED_HINT = 'Disconnected from terminal stream. Click Restart to reconnect.';
const AUTO_RECONNECT_MAX_ATTEMPTS = 5;
const AUTO_RECONNECT_BASE_DELAY_MS = 1000;
const WS_KEEPALIVE_INTERVAL_MS = 60_000;
const WS_KEEPALIVE_MESSAGE = JSON.stringify({ type: 'heartbeat' });
const PENDING_INPUT_MAX_SIZE = 1000;

type ConnectionState = 'idle' | 'connecting' | 'connected' | 'disconnected';

interface Props {
  terminalId: string;
  wsUrl?: string;
  onData?: (data: string) => void;
  onResize?: (cols: number, rows: number) => void;
  onError?: (error: Error) => void;
}

export interface TerminalEmulatorRef {
  write: (data: string) => void;
  clear: () => void;
  reconnect: () => void;
}

/**
 * Terminal emulator with WebSocket-backed input/output.
 */
export const TerminalEmulator = forwardRef<TerminalEmulatorRef, Props>(
  ({ terminalId, wsUrl, onData, onResize, onError }, ref) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const terminalRef = useRef<Terminal | null>(null);
    const fitAddonRef = useRef<FitAddon | null>(null);
    const wsRef = useRef<WebSocket | null>(null);
    const pendingInputRef = useRef<string[]>([]);
    const terminalReadyRef = useRef(false);
    const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const keepAliveTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
    const reconnectAttemptsRef = useRef(0);
    const skipNextAutoReconnectRef = useRef(false);
    const hasReportedTransportErrorRef = useRef(false);
    const [wsKey, setWsKey] = useState(0);
    const [connectionState, setConnectionState] = useState<ConnectionState>('idle');
    const [disconnectHint, setDisconnectHint] = useState<string | null>(null);

    const markConnecting = useCallback(() => {
      setConnectionState('connecting');
      setDisconnectHint(null);
    }, []);

    const markDisconnected = useCallback((hint?: string) => {
      setConnectionState('disconnected');
      setDisconnectHint(hint ?? DISCONNECTED_HINT);
    }, []);

    const clearReconnectTimer = useCallback(() => {
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
    }, []);

    const clearKeepAliveTimer = useCallback(() => {
      if (keepAliveTimerRef.current) {
        globalThis.clearInterval(keepAliveTimerRef.current);
        keepAliveTimerRef.current = null;
      }
    }, []);

    const scheduleAutoReconnect = useCallback(() => {
      if (!wsUrl || skipNextAutoReconnectRef.current) {
        return;
      }

      if (reconnectTimerRef.current) {
        return;
      }

      if (reconnectAttemptsRef.current >= AUTO_RECONNECT_MAX_ATTEMPTS) {
        return;
      }

      const attempt = reconnectAttemptsRef.current + 1;
      reconnectAttemptsRef.current = attempt;
      const delay = AUTO_RECONNECT_BASE_DELAY_MS * Math.pow(2, attempt - 1);

      reconnectTimerRef.current = globalThis.setTimeout(() => {
        reconnectTimerRef.current = null;
        if (!wsUrl) {
          return;
        }
        markConnecting();
        setWsKey((k) => k + 1);
      }, delay);
    }, [markConnecting, wsUrl]);

    // Expose methods via ref
    useImperativeHandle(ref, () => ({
      write: (data: string) => {
        terminalRef.current?.write(data);
      },
      clear: () => {
        terminalRef.current?.clear();
      },
      reconnect: () => {
        // Close existing connection and trigger reconnect
        skipNextAutoReconnectRef.current = true;
        reconnectAttemptsRef.current = 0;
        hasReportedTransportErrorRef.current = false;
        clearReconnectTimer();
        clearKeepAliveTimer();
        if (wsUrl) {
          markConnecting();
        } else {
          setConnectionState('idle');
          setDisconnectHint(null);
        }
        if (wsRef.current) {
          wsRef.current.close();
          wsRef.current = null;
        }
        setWsKey((k) => k + 1);
      },
    }), [clearKeepAliveTimer, clearReconnectTimer, markConnecting, wsUrl]);

    // Stable handlers
    const handleData = useCallback((data: string) => {
      onData?.(data);
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(JSON.stringify({ type: 'input', data }));
      } else {
        if (pendingInputRef.current.length < PENDING_INPUT_MAX_SIZE) {
          pendingInputRef.current.push(data);
        }
      }
    }, [onData]);

    const handleResize = useCallback((cols: number, rows: number) => {
      onResize?.(cols, rows);
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(JSON.stringify({ type: 'resize', cols, rows }));
      }
    }, [onResize]);

    const notifyError = useCallback(
      (message: string, error?: unknown) => {
        const err =
          error instanceof Error ? error : new Error(message);
        onError?.(err);
      },
      [onError]
    );

    // Initialize terminal
    // G28-011: The dependency array includes handleData and handleResize which are
    // recreated when onData/onResize props change, causing the Terminal instance to
    // tear down and reinitialize. Wrapping these handlers in useCallback (above)
    // mitigates this by keeping stable references as long as the callback props
    // themselves don't change. If further stability is needed, consider using refs
    // for these callbacks to fully decouple Terminal lifecycle from prop changes.
    useEffect(() => {
      if (!containerRef.current) return;

      const terminal = new Terminal({
        cursorBlink: true,
        fontSize: 14,
        fontFamily: 'Menlo, Monaco, "Courier New", monospace',
        theme: {
          background: '#1e1e1e',
          foreground: '#d4d4d4',
          cursor: '#d4d4d4',
          selectionBackground: '#264f78',
        },
        scrollback: 10000,
        convertEol: true,
      });

      const fitAddon = new FitAddon();
      terminal.loadAddon(fitAddon);

      terminal.open(containerRef.current);
      fitAddon.fit();

      terminalRef.current = terminal;
      fitAddonRef.current = fitAddon;
      terminalReadyRef.current = true;

      // Handle user input
      terminal.onData(handleData);

      // Handle window resize
      const handleWindowResize = () => {
        fitAddon.fit();
        const { cols, rows } = terminal;
        handleResize(cols, rows);
      };

      globalThis.addEventListener('resize', handleWindowResize);

      return () => {
        globalThis.removeEventListener('resize', handleWindowResize);
        terminalReadyRef.current = false;
        terminal.dispose();
      };
    }, [handleData, handleResize]);

    // WebSocket connection
    useEffect(() => {
      if (!wsUrl || !terminalReadyRef.current) {
        clearReconnectTimer();
        clearKeepAliveTimer();
        reconnectAttemptsRef.current = 0;
        hasReportedTransportErrorRef.current = false;
        skipNextAutoReconnectRef.current = false;
        setConnectionState('idle');
        setDisconnectHint(null);
        return;
      }

      markConnecting();
      clearKeepAliveTimer();
      skipNextAutoReconnectRef.current = false;

      // Basic validation
      if (!terminalId || !TERMINAL_ID_REGEX.test(terminalId)) {
        markDisconnected('Disconnected: invalid terminal identifier.');
        notifyError('Invalid terminalId');
        return;
      }

      let isActive = true;
      const ws = new WebSocket(`${wsUrl}/terminal/${terminalId}`);

      ws.onopen = () => {
        if (!isActive) return;
        clearReconnectTimer();
        reconnectAttemptsRef.current = 0;
        hasReportedTransportErrorRef.current = false;
        skipNextAutoReconnectRef.current = false;
        setConnectionState('connected');
        setDisconnectHint(null);
        logInfo('Terminal WebSocket connected');

        // Clear xterm buffer on reconnection to avoid stale output before replay
        if (wsKey > 0) {
          terminalRef.current?.clear();
        }

        clearKeepAliveTimer();
        keepAliveTimerRef.current = globalThis.setInterval(() => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(WS_KEEPALIVE_MESSAGE);
          }
        }, WS_KEEPALIVE_INTERVAL_MS);

        if (pendingInputRef.current.length > 0) {
          const pending = pendingInputRef.current.splice(0, pendingInputRef.current.length);
          for (const input of pending) {
            ws.send(JSON.stringify({ type: 'input', data: input }));
          }
        }

        if (terminalRef.current) {
          const { cols, rows } = terminalRef.current;
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'resize', cols, rows }));
          }
        }
      };

      ws.onmessage = (event: MessageEvent<string>) => {
        try {
          const message: unknown = JSON.parse(event.data);

          // Use type guards for safe message handling
          if (isWsOutputMessage(message)) {
            terminalRef.current?.write(message.data);
          } else if (isWsErrorMessage(message)) {
            // Handle error message properly
            notifyError('Terminal WebSocket error message', new Error(message.message));
          } else {
            console.warn('Unknown WebSocket message type:', message);
          }
        } catch (error) {
          notifyError('Failed to parse WebSocket message', error);
        }
      };

      ws.onerror = (error) => {
        if (isActive) {
          markDisconnected();
          scheduleAutoReconnect();
        }
        if (!hasReportedTransportErrorRef.current) {
          hasReportedTransportErrorRef.current = true;
          notifyError('Terminal WebSocket error', error);
        }
      };

      ws.onclose = (event?: CloseEvent) => {
        const code = event?.code;
        const isExpectedClose = code === 1000 || code === 1005;
        clearKeepAliveTimer();

        if (isActive) {
          const reason = event?.reason?.trim();
          const showCode = typeof code === 'number' && code !== 1000 && code !== 1005;
          const detail = reason || (showCode ? `code ${code}` : '');
          markDisconnected(detail ? `${DISCONNECTED_HINT} (${detail})` : DISCONNECTED_HINT);
          if (!isExpectedClose && !skipNextAutoReconnectRef.current) {
            scheduleAutoReconnect();
          }
        }
        skipNextAutoReconnectRef.current = false;
        logInfo('Terminal WebSocket closed');
      };

      wsRef.current = ws;

      return () => {
        isActive = false;
        clearKeepAliveTimer();
        clearReconnectTimer();
        if (wsRef.current === ws) {
          wsRef.current = null;
        }
        ws.close();
      };
    }, [
      wsUrl,
      terminalId,
      notifyError,
      markConnecting,
      markDisconnected,
      clearReconnectTimer,
      clearKeepAliveTimer,
      scheduleAutoReconnect,
      wsKey,
    ]);

    const showConnectingHint = Boolean(wsUrl && connectionState === 'connecting');

    return (
      <div className="relative w-full h-full min-h-[300px]">
        <div
          ref={containerRef}
          className="w-full h-full bg-[#1e1e1e] rounded-lg overflow-hidden"
          role="application"
          aria-label="Terminal emulator"
        />
        {showConnectingHint ? (
          <div
            role="status"
            aria-live="polite"
            className="pointer-events-none absolute right-3 top-3 rounded-md bg-black/70 px-2 py-1 text-xs text-amber-200"
          >
            Connecting terminal stream...
          </div>
        ) : null}
        {connectionState === 'disconnected' ? (
          <div
            role="status"
            aria-live="assertive"
            className="pointer-events-none absolute left-3 right-3 top-3 rounded-md border border-red-500/60 bg-red-950/90 px-3 py-2 text-xs text-red-100"
          >
            {disconnectHint ?? DISCONNECTED_HINT}
          </div>
        ) : null}
      </div>
    );
  }
);

TerminalEmulator.displayName = 'TerminalEmulator';
