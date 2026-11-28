// Five-step first-run guide state, pure step derivation, navigation intents, and brand-new
// browser check (DESIGN-1127 Part B).
import type { Group } from './groups.svelte';
import type { Computer } from './nodes.svelte';
import { cliName } from './onboard.svelte';
import { loadJoined, loadOwner } from './owner';
import { preference, t } from './prefs.svelte';
import type { HostInfoPush } from './proto';
import { href, type Page } from './route.svelte';

/** Persisted states of the first-run guide in `localStorage` (`sd-guide-state`). */
export const guideStates = ['new', 'open', 'closed', 'done'] as const;
export type GuideState = (typeof guideStates)[number];

/** Browser preference remembering whether the first-run guide was started, closed, or finished. */
export const guideState = preference('guide-state', guideStates, 'new');

/** Transient UI state for the guide dock and its 4 s closed note. */
export const guideUi = $state({ expanded: true, closedNote: false, focusHeading: false });

let closedTimer: ReturnType<typeof setTimeout> | undefined;
let userOpened = false;

function clearClosedTimer() {
  clearTimeout(closedTimer);
  closedTimer = undefined;
}

/** Whether the expanded dock was explicitly opened by the person since the last layout reset. */
export function isUserOpened(): boolean {
  return userOpened;
}

/** Clears the explicit user-open flag so automatic overlap collapse may re-evaluate. */
export function clearUserOpened() {
  userOpened = false;
}

/** Starts or reopens the guide in its expanded dock form. */
export function openGuide() {
  clearClosedTimer();
  userOpened = true;
  guideUi.closedNote = false;
  guideUi.expanded = true;
  guideUi.focusHeading = true;
  guideState.value = 'open';
}

/** Closes the guide and shows the 4 s reopen note (`guide.closed`). */
export function closeGuide() {
  clearClosedTimer();
  userOpened = false;
  guideState.value = 'closed';
  guideUi.expanded = false;
  guideUi.focusHeading = false;
  guideUi.closedNote = true;
  closedTimer = setTimeout(() => {
    guideUi.closedNote = false;
    closedTimer = undefined;
  }, 4000);
}

/** Collapses the open dock to its pill (or closes the phone bottom sheet). */
export function collapseGuide() {
  userOpened = false;
  guideUi.expanded = false;
  guideUi.focusHeading = false;
}

/** Expands the pill back into the dock (or opens the phone bottom sheet). */
export function expandGuide(focus = false, manual = focus) {
  userOpened = manual;
  guideUi.expanded = true;
  guideUi.focusHeading = focus;
}

/** Marks the guide as finished from the finish view. */
export function finishGuide() {
  clearClosedTimer();
  userOpened = false;
  guideUi.closedNote = false;
  guideUi.focusHeading = false;
  guideState.value = 'done';
}

/** Clears transient UI timers when resetting between tests. */
export function resetGuideUi() {
  clearClosedTimer();
  userOpened = false;
  guideUi.expanded = true;
  guideUi.closedNote = false;
  guideUi.focusHeading = false;
}

/** Opens a mounted native `<dialog>` modally and closes it when destroyed. */
export function modal(el: HTMLDialogElement) {
  if (!el.open) el.showModal();
  return {
    destroy() {
      if (el.open) el.close();
    },
  };
}

/** Result of step 1: connect a computer. */
export interface ConnectStep {
  id: 'connect';
  done: boolean;
  name: string;
}

/** Result of step 2: check the AIs found. */
export interface AisStep {
  id: 'ais';
  done: boolean;
  count: number;
  none: boolean;
  offline: string | undefined;
}

/** Result of step 3: set each AI's defaults. */
export interface DefaultsStep {
  id: 'defaults';
  done: boolean;
  left: string[];
}

/** Result of step 4: create the first group. */
export interface GroupStep {
  id: 'group';
  done: boolean;
  name: string | undefined;
  g: string | undefined;
}

/** Result of step 5: send the first request. */
export interface AskStep {
  id: 'ask';
  done: boolean;
}

export type GuideStep = [ConnectStep, AisStep, DefaultsStep, GroupStep, AskStep];

/** Target action triggered by clicking a current step's action button. */
export type StepIntent =
  { kind: 'refresh' } | { kind: 'go'; hash: string; selector?: string; create?: boolean };

function fixedIntent(id: 'connect' | 'defaults' | 'group'): StepIntent {
  return (
    {
      connect: { kind: 'go', hash: href('onboarding') },
      defaults: {
        kind: 'go',
        hash: href('nodes'),
        selector: '[data-guide="defaults"][data-unset]',
      },
      group: { kind: 'go', hash: href('groups'), create: true },
    } satisfies Record<'connect' | 'defaults' | 'group', StepIntent>
  )[id];
}

function aisIntent(step: AisStep): StepIntent {
  if (step.none) return { kind: 'refresh' };
  return { kind: 'go', hash: href('nodes'), selector: '.ai-row fieldset.acct' };
}

/** Maps a guide step (and current starter group id) to its navigation/refresh intent. */
export function stepIntent(
  step: GuideStep[number],
  groupId: string | undefined,
): StepIntent | undefined {
  if (step.id === 'ais') return aisIntent(step);
  if (step.id === 'ask') {
    return groupId
      ? { kind: 'go', hash: href('chat', { g: groupId }), selector: '.composer:not(.in-thread)' }
      : undefined;
  }
  return fixedIntent(step.id);
}

/** Whether Escape should collapse the desktop guide dock based on focus and state. */
export function canCollapseOnEscape(
  event: Pick<KeyboardEvent, 'key' | 'defaultPrevented'>,
  dock: Pick<HTMLElement, 'contains'> | undefined,
  active: Element | null,
  open: boolean,
): boolean {
  return (
    open && event.key === 'Escape' && !event.defaultPrevented && Boolean(dock?.contains(active))
  );
}

/** Viewport-relative bounding box matching `DOMRect` geometry fields. */
export interface Box {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
}

const onboardingTargets = '.onboarding .card, .onboarding fieldset.acct, .onboarding .done-btn';

const pageTargets: Partial<Record<Page, Partial<Record<GuideStep[number]['id'], string>>>> = {
  nodes: {
    ais: '.ai-row fieldset.acct',
    defaults: '[data-guide="defaults"][data-unset], .defaults .editor',
  },
  groups: {
    group: '.list .head button, .list .empty button',
  },
  chat: {
    ask: '.composer:not(.in-thread)',
  },
};

/** CSS selector for interactive targets the current step points to on `page`, if any. */
export function stepTargetSelector(
  stepId: GuideStep[number]['id'] | undefined,
  page: Page,
): string | undefined {
  if (!stepId) return undefined;
  if (page === 'onboarding') return onboardingTargets;
  return pageTargets[page]?.[stepId];
}

function hasArea(box: Box): boolean {
  return box.width > 0 && box.height > 0;
}

function spansOverlap(aStart: number, aEnd: number, bStart: number, bEnd: number): boolean {
  return aStart < bEnd && aEnd > bStart;
}

/** Whether two laid-out rectangles share positive overlapping area. */
export function boxesOverlap(a: Box, b: Box): boolean {
  if (!hasArea(a) || !hasArea(b)) return false;
  return (
    spansOverlap(a.left, a.right, b.left, b.right) && spansOverlap(a.top, a.bottom, b.top, b.bottom)
  );
}

/** Whether the expanded dock box overlaps any visible target element of the current step. */
export function dockOverlapsTargets(
  dock: Pick<Element, 'getBoundingClientRect'>,
  stepId: GuideStep[number]['id'] | undefined,
  page: Page,
  root: Pick<ParentNode, 'querySelectorAll'> = document,
): boolean {
  const selector = stepTargetSelector(stepId, page);
  if (!selector) return false;
  const dockBox = dock.getBoundingClientRect();
  for (const el of root.querySelectorAll(selector)) {
    if (boxesOverlap(dockBox, el.getBoundingClientRect())) return true;
  }
  return false;
}

/** Callbacks invoked when page layout changes vs when entrance animations settle. */
export interface DockLayoutHandlers {
  onLayoutChange: () => void;
  onSettled: () => void;
}

function sizeChanged(prev: [number, number] | undefined, rect: DOMRectReadOnly): boolean {
  return Boolean(prev) && (rect.width !== prev![0] || rect.height !== prev![1]);
}

function resizeHandler(onLayoutChange: () => void) {
  let prev: [number, number] | undefined;
  return (entries: ResizeObserverEntry[]) => {
    const rect = entries.at(-1)!.contentRect;
    if (sizeChanged(prev, rect)) onLayoutChange();
    prev = [rect.width, rect.height];
  };
}

function scrollHandler(dock: Element, onLayoutChange: () => void) {
  return (event: Event) => {
    if (event.target instanceof Node && dock.contains(event.target)) return;
    onLayoutChange();
  };
}

function watchMain(main: Element, onLayoutChange: () => void): () => void {
  const mo = new MutationObserver(onLayoutChange);
  const ro = new ResizeObserver(resizeHandler(onLayoutChange));
  mo.observe(main, { childList: true, subtree: true });
  ro.observe(main);
  main.addEventListener('pointerdown', onLayoutChange, { passive: true });
  main.addEventListener('focusin', onLayoutChange, { passive: true });
  return () => {
    mo.disconnect();
    ro.disconnect();
    main.removeEventListener('pointerdown', onLayoutChange);
    main.removeEventListener('focusin', onLayoutChange);
  };
}

/** Observes page DOM, viewport size, scroll, main interaction, and settled animations. */
export function observeDockLayout(
  main: Element,
  dock: Element,
  handlers: DockLayoutHandlers,
): () => void {
  const { onLayoutChange, onSettled } = handlers;
  const onScroll = scrollHandler(dock, onLayoutChange);
  const unwatchMain = watchMain(main, onLayoutChange);
  window.addEventListener('resize', onLayoutChange, { passive: true });
  window.addEventListener('scroll', onScroll, { capture: true, passive: true });
  window.addEventListener('animationend', onSettled, { capture: true, passive: true });
  return () => {
    unwatchMain();
    window.removeEventListener('resize', onLayoutChange);
    window.removeEventListener('scroll', onScroll, true);
    window.removeEventListener('animationend', onSettled, true);
  };
}

/** Reactive inputs controlling whether the expanded guide dock may overlap page targets. */
export interface OverlapSyncInput {
  visible: boolean;
  narrow: boolean;
  dock: HTMLElement | undefined;
  stepId: GuideStep[number]['id'] | undefined;
  page: Page;
  group: string | undefined;
  doneCount: number;
}

/** Route and step progress fields whose change resets explicit user-open override. */
export type OverlapTargetContext = Pick<
  OverlapSyncInput,
  'page' | 'group' | 'stepId' | 'doneCount'
>;

/** Whether two overlap contexts share identical route and step progress fields. */
export function sameTargetContext(a: OverlapTargetContext, b: OverlapTargetContext): boolean {
  return (
    a.page === b.page && a.group === b.group && a.stepId === b.stepId && a.doneCount === b.doneCount
  );
}

/** Whether the expanded desktop dock is currently mounted and eligible for overlap tracking. */
export function canWatchDock(
  input: Pick<OverlapSyncInput, 'visible' | 'narrow' | 'dock'>,
): input is Pick<OverlapSyncInput, 'visible' | 'narrow'> & { dock: HTMLElement } {
  return input.visible && !input.narrow && guideUi.expanded && Boolean(input.dock);
}

/** Collapses the dock over a target unless user-opened, notifying only if dock held focus. */
export function autoCollapseIfOverlapping(
  input: Pick<OverlapSyncInput, 'visible' | 'narrow' | 'dock' | 'stepId' | 'page'>,
  onFocusInsideDock: () => void,
  doc: Pick<Document, 'activeElement' | 'querySelectorAll'> = document,
): boolean {
  if (!canWatchDock(input) || userOpened) return false;
  if (!dockOverlapsTargets(input.dock, input.stepId, input.page, doc)) return false;
  if (input.dock.contains(doc.activeElement)) onFocusInsideDock();
  collapseGuide();
  return true;
}

function copyContext(ctx: OverlapTargetContext): OverlapTargetContext {
  return {
    page: ctx.page,
    group: ctx.group,
    stepId: ctx.stepId,
    doneCount: ctx.doneCount,
  };
}

/** Tracks route/step context transitions and attaches layout observers while dock stays open. */
export function createOverlapTracker(
  initial: OverlapTargetContext,
  onFocusInsideDock: () => void,
  doc: Pick<Document, 'activeElement' | 'querySelector' | 'querySelectorAll'> = document,
) {
  let prev = copyContext(initial);

  return {
    sync(input: OverlapSyncInput): (() => void) | undefined {
      if (!sameTargetContext(prev, input)) {
        prev = copyContext(input);
        clearUserOpened();
      }
      if (!canWatchDock(input)) return undefined;
      if (autoCollapseIfOverlapping(input, onFocusInsideDock, doc)) return undefined;
      return observeDockLayout(doc.querySelector('main')!, input.dock, {
        onLayoutChange: () => {
          clearUserOpened();
          autoCollapseIfOverlapping(input, onFocusInsideDock, doc);
        },
        onSettled: () => autoCollapseIfOverlapping(input, onFocusInsideDock, doc),
      });
    },
  };
}

function aisDoneText(count: number): string {
  return count === 1
    ? t('guide.step.ais.done.one')
    : t('guide.step.ais.done').replace('{n}', String(count));
}

/** Localized completion summary shown on the right side of a finished guide row. */
export function stepDoneText(step: GuideStep[number]): string {
  if (step.id === 'connect') return t('guide.step.connect.done').replace('{name}', step.name);
  if (step.id === 'ais') return aisDoneText(step.count);
  if (step.id === 'group') return t('guide.step.group.done').replace('{name}', step.name ?? '');
  return t(`guide.step.${step.id}.done`);
}

/** Localized action button label for the current guide step. */
export function stepActionLabel(step: GuideStep[number]): string {
  if (step.id === 'ais') {
    return step.none ? t('guide.step.ais.again') : t('guide.step.ais.go');
  }
  return t(`guide.step.${step.id}.go`);
}

function syncSeen(seen: Set<object>, computers: readonly Computer[]): boolean {
  let fresh = false;
  for (const comp of computers) {
    if (!comp.online || !comp.info) {
      seen.delete(comp.home);
    } else if (!seen.has(comp.home)) {
      seen.add(comp.home);
      fresh = true;
    }
  }
  return fresh;
}

function firstNewlyDone(
  steps: readonly GuideStep[number][],
  nextDone: readonly boolean[],
  prevDone: readonly boolean[],
): GuideStep[number] | undefined {
  return steps.find((step, idx) => nextDone[idx] && !prevDone[idx]);
}

function snapshot(steps: readonly GuideStep[number][], computers: readonly Computer[]) {
  const seen = new Set<object>();
  syncSeen(seen, computers);
  return {
    seen,
    prevDone: steps.map((s) => s.done),
    armed: computers.every((comp) => seen.has(comp.home)),
  };
}

/** Tracks per-computer `host.info` readiness so initial load and reconnects do not announce. */
export function createAnnouncer(
  initialSteps: readonly GuideStep[number][],
  initialComputers: readonly Computer[],
) {
  let state = snapshot(initialSteps, initialComputers);

  return {
    reset(steps: readonly GuideStep[number][], computers: readonly Computer[]) {
      state = snapshot(steps, computers);
    },
    next(
      visible: boolean,
      steps: readonly GuideStep[number][],
      computers: readonly Computer[],
    ): GuideStep[number] | undefined {
      const nextDone = steps.map((s) => s.done);
      const fresh = syncSeen(state.seen, computers);
      const allSeen = computers.every((comp) => state.seen.has(comp.home));
      const hit =
        visible && state.armed && allSeen && !fresh
          ? firstNewlyDone(steps, nextDone, state.prevDone)
          : undefined;
      state.armed = allSeen;
      state.prevDone = nextDone;
      return hit;
    },
  };
}

type Agent = HostInfoPush['agents'][number];

function starterGroup(group: Group): boolean {
  const me = group.me;
  return Boolean(
    group.info?.secretary &&
    me &&
    group.members.some((member) => member.kind === 'ai' && member.user === me.user),
  );
}

function connectStep(computers: readonly Computer[]): ConnectStep {
  const first = computers[0];
  return {
    id: 'connect',
    done: computers.length > 0,
    name: first ? (first.info?.name ?? first.home.name) : '',
  };
}

function aisStep(
  agents: readonly Agent[],
  infosCount: number,
  computersCount: number,
  firstName: string,
): AisStep {
  return {
    id: 'ais',
    done: agents.length > 0 && agents.every((agent) => agent.account !== null),
    count: agents.length,
    none: infosCount > 0 && agents.length === 0,
    offline: computersCount > 0 && infosCount === 0 ? firstName : undefined,
  };
}

function defaultsStep(aisDone: boolean, agents: readonly Agent[]): DefaultsStep {
  const withAccount = agents.filter((agent) => agent.account !== null);
  const missing = withAccount.filter((agent) => !agent.defaults);
  return {
    id: 'defaults',
    done: aisDone && missing.length === 0,
    left: [...new Set(missing.map((agent) => cliName(agent.agent)))],
  };
}

function groupStep(readyGroups: readonly Group[]): GroupStep {
  const latest = readyGroups[0];
  return {
    id: 'group',
    done: readyGroups.length > 0,
    name: latest?.name,
    g: latest?.g,
  };
}

function askStep(readyGroups: readonly Group[]): AskStep {
  return {
    id: 'ask',
    done: readyGroups.some((group) =>
      group.main.some(
        (msg) => msg.kind === 'text' && msg.thread === null && msg.by === group.me!.id,
      ),
    ),
  };
}

/** Derives the five first-run guide steps strictly from live computers and groups. */
export function guideSteps(computers: readonly Computer[], groups: readonly Group[]): GuideStep {
  const infos = computers.filter((comp) => comp.online && comp.info).map((comp) => comp.info!);
  const agents = infos.flatMap((info) => info.agents).filter((agent) => agent.agent !== 'external');
  const step1 = connectStep(computers);
  const step2 = aisStep(agents, infos.length, computers.length, step1.name);
  const step3 = defaultsStep(step2.done, agents);
  const readyGroups = groups.filter(starterGroup);
  return [step1, step2, step3, groupStep(readyGroups), askStep(readyGroups)];
}

/** Whether this browser has neither an owner record nor any joined home in IndexedDB. */
export async function isBrandNew(
  cryptoObj: Crypto = crypto,
  idb: () => IDBFactory = () => indexedDB,
): Promise<boolean> {
  try {
    const [{ log }, joined] = await Promise.all([loadOwner(cryptoObj, idb), loadJoined(idb)]);
    return !log && Object.keys(joined).length === 0;
  } catch {
    return false;
  }
}
