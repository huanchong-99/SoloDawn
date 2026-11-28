// Draws a fixed-position spotlight ring around a target element after navigation.

const findTimeoutMs = 3000;
const autoDismissMs = 6000;
const fadeOutMs = 140;

let activeCleanup: (() => void) | undefined;
let beaconHost: HTMLElement | undefined;

/** Registers the layer element where beacons mount. */
export function setBeaconHost(el: HTMLElement | undefined) {
  beaconHost = el;
}

/** Dismisses any active beacon immediately and cancels a pending target search. */
export function clearBeacon() {
  activeCleanup?.();
  activeCleanup = undefined;
}

function reducedMotion() {
  return matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function firstVisible(selector: string): HTMLElement | undefined {
  return [...document.querySelectorAll<HTMLElement>(selector)].find((el) =>
    el.checkVisibility({ visibilityProperty: true }),
  );
}

function findTarget(selector: string, signal: AbortSignal): Promise<HTMLElement | undefined> {
  const immediate = firstVisible(selector);
  if (immediate) return Promise.resolve(immediate);
  const started = Date.now();
  return new Promise((resolve) => {
    let rafId = 0;
    const stop = () => cancelAnimationFrame(rafId);
    signal.addEventListener('abort', () => (stop(), resolve(undefined)));
    const poll = () => {
      if (signal.aborted) return;
      const found = firstVisible(selector);
      if (found) return resolve(found);
      if (Date.now() - started >= findTimeoutMs) return resolve(undefined);
      rafId = requestAnimationFrame(poll);
    };
    rafId = requestAnimationFrame(poll);
  });
}

function placeBeacon(el: HTMLElement, target: HTMLElement) {
  const rect = target.getBoundingClientRect();
  const radius = parseFloat(getComputedStyle(target).borderTopLeftRadius) || 0;
  el.style.left = `${rect.left - 4}px`;
  el.style.top = `${rect.top - 4}px`;
  el.style.width = `${rect.width + 8}px`;
  el.style.height = `${rect.height + 8}px`;
  el.style.borderRadius = `${radius + 4}px`;
}

function createBeacon(reduced: boolean): HTMLDivElement {
  const el = document.createElement('div');
  el.className = 'guide-beacon';
  el.setAttribute('aria-hidden', 'true');
  if (reduced) el.dataset.reduced = 'true';
  const ring = document.createElement('span');
  ring.className = 'guide-beacon-ring';
  el.appendChild(ring);
  if (!reduced) {
    const pulse = document.createElement('span');
    pulse.className = 'guide-beacon-pulse';
    el.appendChild(pulse);
  }
  return el;
}

function mountBeacon(target: HTMLElement, reduced: boolean, signal: AbortSignal) {
  const el = createBeacon(reduced);
  placeBeacon(el, target);
  beaconHost!.appendChild(el);

  let dismissed = false;
  let fadeTimer: ReturnType<typeof setTimeout> | undefined;
  let autoTimer: ReturnType<typeof setTimeout> | undefined;

  const removeNow = () => {
    clearTimeout(autoTimer);
    clearTimeout(fadeTimer);
    window.removeEventListener('scroll', sync, true);
    window.removeEventListener('resize', sync);
    window.removeEventListener('animationend', sync, true);
    target.removeEventListener('click', dismiss, true);
    target.removeEventListener('focusin', dismiss);
    el.remove();
  };

  const dismiss = () => {
    if (dismissed) return;
    dismissed = true;
    clearTimeout(autoTimer);
    if (reduced) return removeNow();
    el.classList.add('leaving');
    fadeTimer = setTimeout(removeNow, fadeOutMs);
  };

  const sync = () => {
    if (!target.isConnected) return removeNow();
    placeBeacon(el, target);
  };

  window.addEventListener('scroll', sync, { capture: true, passive: true });
  window.addEventListener('resize', sync, { passive: true });
  window.addEventListener('animationend', sync, { capture: true, passive: true });
  target.addEventListener('click', dismiss, { capture: true });
  target.addEventListener('focusin', dismiss);
  autoTimer = setTimeout(dismiss, autoDismissMs);
  signal.addEventListener('abort', removeNow);
}

/** Highlights the first element matching `selector` with a fixed spotlight ring. */
export async function beacon(selector: string): Promise<void> {
  clearBeacon();
  const controller = new AbortController();
  activeCleanup = () => controller.abort();
  const target = await findTarget(selector, controller.signal);
  if (!target || controller.signal.aborted) return;
  const reduced = reducedMotion();
  target.scrollIntoView({ block: 'center', behavior: reduced ? 'instant' : 'smooth' });
  mountBeacon(target, reduced, controller.signal);
}
