// Five-step first-run guide state, pure step derivation, navigation intents, and brand-new
// browser check (DESIGN-1127 Part B).
import type { Group } from './groups.svelte';
import type { Computer } from './nodes.svelte';
import { cliName } from './onboard.svelte';
import { loadJoined, loadOwner } from './owner';
import { preference, t } from './prefs.svelte';
import type { HostInfoPush } from './proto';
import { href } from './route.svelte';

/** Persisted states of the first-run guide in `localStorage` (`sd-guide-state`). */
export const guideStates = ['new', 'open', 'closed', 'done'] as const;
export type GuideState = (typeof guideStates)[number];

/** Browser preference remembering whether the first-run guide was started, closed, or finished. */
export const guideState = preference('guide-state', guideStates, 'new');

/** Transient UI state for the guide dock and its 4 s closed note. */
export const guideUi = $state({ expanded: true, closedNote: false, focusHeading: false });

let closedTimer: ReturnType<typeof setTimeout> | undefined;

function clearClosedTimer() {
  clearTimeout(closedTimer);
  closedTimer = undefined;
}

/** Starts or reopens the guide in its expanded dock form. */
export function openGuide() {
  clearClosedTimer();
  guideUi.closedNote = false;
  guideUi.expanded = true;
  guideUi.focusHeading = true;
  guideState.value = 'open';
}

/** Closes the guide and shows the 4 s reopen note (`guide.closed`). */
export function closeGuide() {
  clearClosedTimer();
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
  guideUi.expanded = false;
  guideUi.focusHeading = false;
}

/** Expands the pill back into the dock (or opens the phone bottom sheet). */
export function expandGuide(focus = false) {
  guideUi.expanded = true;
  guideUi.focusHeading = focus;
}

/** Marks the guide as finished from the finish view. */
export function finishGuide() {
  clearClosedTimer();
  guideUi.closedNote = false;
  guideUi.focusHeading = false;
  guideState.value = 'done';
}

/** Clears transient UI timers when resetting between tests. */
export function resetGuideUi() {
  clearClosedTimer();
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
