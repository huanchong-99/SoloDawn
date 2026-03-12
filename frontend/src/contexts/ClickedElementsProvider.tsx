import {
  createContext,
  useContext,
  useState,
  ReactNode,
  useEffect,
  useCallback,
  useMemo,
} from 'react';
import type {
  OpenInEditorPayload,
  ComponentInfo,
  SelectedComponent,
} from '@/utils/previewBridge';
import type { Workspace } from 'shared/types';
import { genId } from '@/utils/id';

export interface ClickedEntry {
  id: string;
  payload: OpenInEditorPayload;
  timestamp: number;
  dedupeKey: string;
  selectedDepth?: number; // 0 = innermost (selected), 1 = parent, etc.
}

interface ClickedElementsContextType {
  elements: ClickedEntry[];
  addElement: (payload: OpenInEditorPayload) => void;
  removeElement: (id: string) => void;
  clearElements: () => void;
  selectComponent: (id: string, depthFromInner: number) => void;
  generateMarkdown: () => string;
}

const ClickedElementsContext = createContext<ClickedElementsContextType | null>(
  null
);

export function useClickedElements() {
  const context = useContext(ClickedElementsContext);
  if (!context) {
    throw new Error(
      'useClickedElements must be used within a ClickedElementsProvider'
    );
  }
  return context;
}

interface ClickedElementsProviderProps {
  children: ReactNode;
  attempt?: Workspace | null;
}

const MAX_ELEMENTS = 20;
const MAC_PRIVATE_PREFIX = '/private';
const MAC_PRIVATE_ALIAS_ROOTS = new Set(['var', 'tmp']);

// Helpers

function stripPrefix(value: string, prefix: string): string {
  return value.startsWith(prefix) ? value.slice(prefix.length) : value;
}

function stripPrefixes(p?: string): string {
  if (!p) return '';
  return stripPrefix(
    stripPrefix(stripPrefix(p, 'file://'), 'webpack:///'),
    'webpack://'
  ).trim();
}

// macOS alias handling; no-ops on other OSes
function normalizeMacPrivateAliases(p: string): string {
  if (!p) return p;

  if (!p.startsWith(`${MAC_PRIVATE_PREFIX}/`)) return p;

  // Security constraint: this normalization is only for UI/display strings.
  // This provider does not perform filesystem writes and never builds writable paths.
  const candidate = p.slice(MAC_PRIVATE_PREFIX.length);
  if (!candidate.startsWith('/')) return p;

  const root = candidate.slice(1).split('/', 1)[0];
  if (!MAC_PRIVATE_ALIAS_ROOTS.has(root)) return p;

  return candidate;
}

function trimTrailingSlashes(value: string): string {
  let end = value.length;
  while (end > 0 && value.codePointAt(end - 1) === 47) {
    end -= 1;
  }
  return value.slice(0, end);
}

function isAsciiWhitespace(code: number): boolean {
  return (
    code === 9 || // \t
    code === 10 || // \n
    code === 11 || // \v
    code === 12 || // \f
    code === 13 || // \r
    code === 32 // space
  );
}

function splitOnWhitespace(value: string): string[] {
  const out: string[] = [];
  let start = -1;

  for (let i = 0; i < value.length; i++) {
    const code = value.codePointAt(i) ?? -1;
    if (isAsciiWhitespace(code)) {
      if (start !== -1) {
        out.push(value.slice(start, i));
        start = -1;
      }
      continue;
    }

    if (start === -1) {
      start = i;
    }
  }

  if (start !== -1) {
    out.push(value.slice(start));
  }

  return out;
}

// Return { path, line?, col? } where `path` has no trailing :line(:col).
// Works even when Windows drive letters contain a colon.
function parsePathWithLineCol(raw?: string): {
  path: string;
  line?: number;
  col?: number;
} {
  const s = stripPrefixes(raw);
  if (!s) return { path: '' };
  const normalized = normalizeMacPrivateAliases(s);

  // Try to split trailing :line(:col). Last and second-to-last tokens must be numbers.
  const parts = normalized.split(':');
  if (parts.length <= 2) return { path: normalized };

  const last = parts.at(-1);
  const maybeCol = Number(last);
  if (!Number.isFinite(maybeCol)) return { path: normalized };

  const prev = parts.at(-2);
  const maybeLine = Number(prev);
  if (!Number.isFinite(maybeLine)) return { path: normalized };

  // Windows drive (e.g., "C") is at index 0; this still works because we only strip the end
  const basePath = parts.slice(0, -2).join(':');
  return { path: basePath, line: maybeLine, col: maybeCol };
}

function relativizePath(p: string, workspaceRoot?: string): string {
  if (!p) return '';
  const normalized = normalizeMacPrivateAliases(stripPrefixes(p));

  if (!workspaceRoot) return normalized;

  // Simple prefix strip; robust handling is on backend (path.rs).
  // This keeps the UI stable even when run inside macOS /private/var containers.
  const wr = normalizeMacPrivateAliases(trimTrailingSlashes(workspaceRoot));
  if (
    normalized.startsWith(wr.endsWith('/') ? wr : wr + '/') ||
    normalized === wr
  ) {
    const rel = normalized.slice(wr.length);
    return rel.startsWith('/') ? rel.slice(1) : rel || '.';
  }
  return normalized;
}

function formatLoc(path: string, line?: number, col?: number) {
  if (!path) return '';
  if (line == null) return path;
  const colPart = col == null ? '' : `:${col}`;
  const lineCol = `${line}${colPart}`;
  return `${path}:${lineCol}`;
}

function formatDomBits(ce?: OpenInEditorPayload['clickedElement']) {
  const bits: string[] = [];
  if (ce?.tag) bits.push(ce.tag.toLowerCase());
  if (ce?.id) bits.push(`#${String(ce.id)}`);
  const classes = normalizeClassName(ce?.className);
  if (classes) bits.push(`.${classes}`);
  if (ce?.role) bits.push(`@${String(ce.role)}`);
  return bits.join('') || '(unknown)';
}

function normalizeClassName(className?: string): string {
  if (!className) return '';
  return splitOnWhitespace(className)
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b))
    .join('.');
}

function makeDedupeKey(
  payload: OpenInEditorPayload,
  workspaceRoot?: string
): string {
  const s = payload.selected;
  const ce = payload.clickedElement;

  const { path } = parsePathWithLineCol(s.pathToSource);
  const rel = relativizePath(path, workspaceRoot);

  const domBits: string[] = [];
  if (ce?.tag) domBits.push(ce.tag.toLowerCase());
  if (ce?.id) domBits.push(`#${String(ce.id)}`);
  const normalizedClasses = normalizeClassName(ce?.className);
  if (normalizedClasses) domBits.push(`.${normalizedClasses}`);
  if (ce?.role) domBits.push(`@${String(ce.role)}`);

  const locKey = [
    rel,
    s.source?.lineNumber ?? '',
    s.source?.columnNumber ?? '',
  ].join(':');
  return `${s.name}|${locKey}|${domBits.join('')}`;
}

// Helper to prune primitive values
function prunePrimitive(value: unknown, maxString: number): unknown {
  const t = typeof value;
  if (t === 'string') {
    return (value as string).length > maxString
      ? (value as string).slice(0, maxString) + '…'
      : value;
  }
  if (t === 'number' || t === 'boolean') return value;
  if (t === 'function') return '[Function]';
  if (t === 'bigint') return (value as bigint).toString() + 'n';
  if (t === 'symbol') return (value as symbol).toString();
  return null; // Not a primitive
}

// Helper to prune array values
function pruneArray(
  value: unknown[],
  depth: number,
  maxString: number,
  maxArray: number
): unknown[] {
  const lim = value
    .slice(0, maxArray)
    .map((v) => pruneValue(v, depth - 1, maxString, maxArray));
  if (value.length > maxArray) {
    lim.push(`[+${value.length - maxArray} more]`);
  }
  return lim;
}

// Helper to prune object values
function pruneObject(
  obj: Record<string, unknown>,
  depth: number,
  maxString: number,
  maxArray: number
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  let count = 0;
  for (const k of Object.keys(obj).sort((a, b) => a.localeCompare(b))) {
    if (count++ > 50) {
      out['[TruncatedKeys]'] = true;
      break;
    }
    out[k] = pruneValue(obj[k], depth - 1, maxString, maxArray);
  }
  return out;
}

// Remove heavy or unsafe props while retaining debuggability
function pruneValue(
  value: unknown,
  depth: number,
  maxString = 200,
  maxArray = 20
): unknown {
  if (depth <= 0) return '[MaxDepth]';
  if (value == null) return value;

  // Try primitive pruning first
  const primitive = prunePrimitive(value, maxString);
  if (primitive !== null) return primitive;

  // Handle arrays
  if (Array.isArray(value)) {
    return pruneArray(value as unknown[], depth, maxString, maxArray);
  }

  // Handle objects
  if (typeof value === 'object') {
    return pruneObject(value as Record<string, unknown>, depth, maxString, maxArray);
  }

  return '[Unknown]';
}

function stripHeavyProps(payload: OpenInEditorPayload): OpenInEditorPayload {
  // Avoid mutating caller objects
  const shallowSelected = {
    ...payload.selected,
    props: pruneValue(payload.selected.props, 2) as Record<string, unknown>,
  };

  const shallowComponents = payload.components.map((c) => ({
    ...c,
    props: pruneValue(c.props, 2) as Record<string, unknown>,
  }));

  // dataset and coords are typically small; keep as-is.
  return {
    ...payload,
    selected: shallowSelected,
    components: shallowComponents,
  };
}

// Build component chain from inner-most to outer-most
function buildChainInnerToOuter(
  payload: OpenInEditorPayload,
  workspaceRoot?: string
) {
  const comps = payload.components ?? [];
  const s = payload.selected;

  // Start with the selected component as innermost
  const innerToOuter: (ComponentInfo | SelectedComponent)[] = [s];

  // Add components that aren't duplicates of selected
  const selectedKey = `${s.name}|${s.pathToSource}|${s.source?.lineNumber}|${s.source?.columnNumber}`;
  comps.forEach((c) => {
    const compKey = `${c.name}|${c.pathToSource}|${c.source?.lineNumber}|${c.source?.columnNumber}`;
    if (compKey !== selectedKey) {
      innerToOuter.push(c);
    }
  });

  // Remove duplicates by creating unique keys
  const seen = new Set<string>();
  return innerToOuter.filter((c) => {
    const parsed = parsePathWithLineCol(c.pathToSource);
    const rel = relativizePath(parsed.path, workspaceRoot);
    const loc = formatLoc(
      rel,
      c.source?.lineNumber ?? parsed.line,
      c.source?.columnNumber ?? parsed.col
    );
    const key = `${c.name}|${loc}`;

    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

function formatClickedMarkdown(
  entry: ClickedEntry,
  workspaceRoot?: string
): string {
  const { payload, selectedDepth = 0 } = entry;
  const chain = buildChainInnerToOuter(payload, workspaceRoot);
  const effectiveChain = chain.slice(selectedDepth); // Start from selected anchor outward

  // DOM
  const dom = formatDomBits(payload.clickedElement);

  // Use first component in effective chain as the "selected start"
  const first = effectiveChain[0];
  const parsed = parsePathWithLineCol(first.pathToSource);
  const rel = relativizePath(parsed.path, workspaceRoot);
  const lineNum = first.source?.lineNumber ?? parsed.line;
  const colNum = first.source?.columnNumber ?? parsed.col;
  const loc = formatLoc(rel, lineNum, colNum);

  // Build hierarchy from effective chain
  const items = effectiveChain.map((c, i) => {
    const p = parsePathWithLineCol(c.pathToSource);
    const r = relativizePath(p.path, workspaceRoot);
    const l = formatLoc(
      r,
      c.source?.lineNumber ?? p.line,
      c.source?.columnNumber ?? p.col
    );
    const indent = '  '.repeat(i);
    const arrow = i > 0 ? '└─ ' : '';
    const tag = i === 0 ? ' ← start' : '';
    const sourceText = l || 'no source';
    return `${indent}${arrow}${c.name} (\`${sourceText}\`)${tag}`;
  });

  const locDisplay = loc ? `\`${loc}\`` : 'no source';

  return [
    `From preview click:`,
    `- DOM: ${dom}`,
    `- Selected start: ${first.name} (${locDisplay})`,
    effectiveChain.length > 1
      ? ['- Component hierarchy:', ...items].join('\n')
      : '',
  ]
    .filter(Boolean)
    .join('\n');
}

export function ClickedElementsProvider({
  children,
  attempt,
}: Readonly<ClickedElementsProviderProps>) {
  const [elements, setElements] = useState<ClickedEntry[]>([]);
  const workspaceRoot = attempt?.containerRef;

  // Clear elements when attempt changes
  useEffect(() => {
    setElements([]);
  }, [attempt?.id]);

  const addElement = useCallback((payload: OpenInEditorPayload) => {
    const sanitized = stripHeavyProps(payload);
    const dedupeKey = makeDedupeKey(sanitized, workspaceRoot || undefined);

    setElements((prev) => {
      const last = prev[prev.length - 1];
      if (last?.dedupeKey === dedupeKey) {
        return prev; // Skip consecutive duplicate
      }
      const newEntry: ClickedEntry = {
        id: genId(),
        payload: sanitized,
        timestamp: Date.now(),
        dedupeKey,
      };
      const updated = [...prev, newEntry];
      return updated.length > MAX_ELEMENTS
        ? updated.slice(-MAX_ELEMENTS)
        : updated;
    });
  }, [workspaceRoot]);

  const removeElement = useCallback((id: string) => {
    setElements((prev) => prev.filter((e) => e.id !== id));
  }, []);

  const clearElements = useCallback(() => {
    setElements([]);
  }, []);

  const selectComponent = useCallback((id: string, depthFromInner: number) => {
    setElements((prev) =>
      prev.map((e) =>
        e.id === id ? { ...e, selectedDepth: depthFromInner } : e
      )
    );
  }, []);

  const generateMarkdown = useCallback(() => {
    if (elements.length === 0) return '';
    const header = `## Clicked Elements (${elements.length})\n\n`;
    const body = elements
      .map((e) => formatClickedMarkdown(e, workspaceRoot || undefined))
      .join('\n\n');
    return header + body;
  }, [elements, workspaceRoot]);

  // Memoize the context value to prevent unnecessary re-renders
  const contextValue = useMemo(
    () => ({
      elements,
      addElement,
      removeElement,
      clearElements,
      selectComponent,
      generateMarkdown,
    }),
    [elements, addElement, removeElement, clearElements, selectComponent, generateMarkdown]
  );

  return (
    <ClickedElementsContext.Provider value={contextValue}>
      {children}
    </ClickedElementsContext.Provider>
  );
}
