import { prefersReducedMotion } from 'svelte/motion';

const controls = 'button:not([disabled])';
const fields = 'input:not([disabled]), textarea:not([disabled])';
/** What the person does to scroll the main line while the jump's smooth scroll is on its way,
 *  whose own scroll events do not tell theirs apart: a key that scrolls, or a press on the
 *  scrollbar, counts only then. */
const scrolls = ['wheel', 'touchmove'] as const;
const scrollKeys = /^(Arrow(Up|Down)|Page(Up|Down)|Home|End| )$/;

interface Held {
  el: HTMLElement;
  /** The main line's reversed scroller, and its one child. */
  scroller: HTMLElement;
  line: HTMLElement;
  /** Where the message's top is kept, below the scroller's top. */
  y: number;
  /** The offset the scroller was last sent to. */
  top: number;
  /** How far the message's top was from the line's end then. */
  end: number;
  /** Whether the message has been at `y` since the jump. */
  landed: boolean;
  stop: () => void;
}

/** The message last jumped to, until the person scrolls the main line or jumps elsewhere. */
let held: Held | undefined;

/** Lets the message jumped to go: the main line follows the browser's scroll anchoring again. */
export function release() {
  held?.stop();
  held = undefined;
}

// How far the message's top is from the line's end. The scroller's offset counts from its bottom,
// so what the line gains above the message moves nothing and what it gains below moves the message
// up. Layout offsets, not boxes: those of a message that still rises (`rise`) are moved.
const fromEnd = ({ el, line }: Held) => line.offsetTop + line.offsetHeight - el.offsetTop;

// Scrolls the held message's top to `y` below the scroller's top, as far as the line reaches.
function place(h: Held, behavior: ScrollBehavior) {
  const { scroller } = h;
  h.end = fromEnd(h);
  const room = scroller.scrollHeight - scroller.clientHeight;
  h.top = Math.min(0, Math.max(-room, scroller.clientHeight - h.y - h.end));
  h.y = scroller.clientHeight - h.top - h.end;
  h.landed ||= behavior === 'auto';
  scroller.scrollTo({ top: h.top, behavior });
}

// The line changed size: the message is put back at once if the change moved it, and else left
// alone: the observer also calls once when it starts, in the jump's first frame, where a scroll
// would end a smooth jump at once.
function keep(h: Held) {
  if (!h.el.isConnected) return release();
  if (fromEnd(h) !== h.end) place(h, 'auto');
}

// A scroll to where the hold sent the scroller is its own, also when the line grew again before
// the hold saw it; one that leaves the message at `y` is the browser's, when the line grows below
// the message at its top. Once the message has landed, any other is the person's, by key,
// scrollbar, focus, assistive technology or find, and lets it go. Offsets are rounded to a pixel.
function scrolled(h: Held) {
  const { scroller } = h;
  const y = scroller.clientHeight - scroller.scrollTop - fromEnd(h);
  if (Math.abs(scroller.scrollTop - h.top) < 2 || Math.abs(y - h.y) < 2) h.landed = true;
  else if (h.landed) release();
}

// Keeps a message of the main line where the jump puts it, the middle, while pages and messages
// arrive above and below it; `y` is taken before its thread opens. The browser's scroll anchoring
// cannot: it picks a row above the message as its anchor, and stops while a row rises; Safari has
// none.
function hold(el: HTMLElement, scroller: HTMLElement) {
  release();
  const line = scroller.firstElementChild as HTMLElement;
  const sizes = new ResizeObserver(() => keep(held!));
  const scroll = () => scrolled(held!);
  const press = () => held!.landed || release();
  const key = (event: KeyboardEvent) => scrollKeys.test(event.key) && press();
  for (const type of scrolls) scroller.addEventListener(type, release, { passive: true });
  scroller.addEventListener('scroll', scroll);
  scroller.addEventListener('pointerdown', press);
  scroller.addEventListener('keydown', key);
  scroller.style.overflowAnchor = 'none';
  const stop = () => {
    sizes.disconnect();
    for (const type of scrolls) scroller.removeEventListener(type, release);
    scroller.removeEventListener('scroll', scroll);
    scroller.removeEventListener('pointerdown', press);
    scroller.removeEventListener('keydown', key);
    scroller.style.overflowAnchor = '';
  };
  const y = (scroller.clientHeight - el.offsetHeight) / 2;
  held = { el, scroller, line, y, top: 0, end: 0, landed: false, stop };
  sizes.observe(line);
  return held;
}

/** What takes the focus in a message: of a task's card its thread, which opens. */
function controlIn(el: HTMLElement, kind?: string) {
  const thread = el.querySelector<HTMLDetailsElement>('.thread-fold');
  if (!thread) return el.querySelector<HTMLElement>(kind === 'decision' ? fields : controls);
  thread.open = true;
  return thread.querySelector('summary');
}

// The main line keeps the message where the jump puts it, unless that is its end, where the line
// already keeps it and the newest in view; elsewhere, as on the terminal page, nothing arrives
// around it.
function show(el: HTMLElement, h: Held | undefined, behavior: ScrollBehavior) {
  if (!h) return el.scrollIntoView({ behavior, block: 'center' });
  place(h, behavior);
  if (h.top === 0) release();
}

/** Scrolls a message `#m-<id>` to the middle of the view, as far as the view reaches, without
 *  changing the hash route; in the main line it stays there until the person scrolls or sends a
 *  message (`release`). Moves focus to its first enabled control, or to the message element
 *  itself when it has none; of a task's card it opens the thread. */
export function jumpToMessage(event: Event, id: string, kind?: string): void {
  event.preventDefault();
  const el = document.getElementById(`m-${id}`);
  if (!el) return;
  // At once where a thread opens: the card grows while a smooth scroll is under way, whose end
  // Firefox does not correct.
  const jump = prefersReducedMotion.current || el.querySelector('.thread-fold:not([open])');
  const scroller = el.closest<HTMLElement>('.scroller');
  const h = scroller ? hold(el, scroller) : undefined;
  const control = controlIn(el, kind);
  show(el, h, jump ? 'auto' : 'smooth');
  if (control) {
    control.focus({ preventScroll: true });
    return;
  }
  el.tabIndex = -1;
  el.focus({ preventScroll: true });
}
