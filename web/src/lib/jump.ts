import { prefersReducedMotion } from 'svelte/motion';

const controls = 'button:not([disabled])';
const fields = 'input:not([disabled]), textarea:not([disabled])';

/** What takes the focus in a message: of a task's card its thread, which opens. */
function controlIn(el: HTMLElement, kind?: string) {
  const thread = el.querySelector<HTMLDetailsElement>('.thread-fold');
  if (!thread) return el.querySelector<HTMLElement>(kind === 'decision' ? fields : controls);
  thread.open = true;
  return thread.querySelector('summary');
}

/** Scrolls a main-line message `#m-<id>` into view without changing the hash route, and moves
 *  focus to its first enabled control, or to the message element itself when it has none; of a
 *  task's card it opens the thread. */
export function jumpToMessage(event: Event, id: string, kind?: string): void {
  event.preventDefault();
  const el = document.getElementById(`m-${id}`);
  if (!el) return;
  // At once where a thread opens: the card grows while a smooth scroll is under way, whose end
  // Firefox does not correct.
  const jump = prefersReducedMotion.current || el.querySelector('.thread-fold:not([open])');
  el.scrollIntoView({ behavior: jump ? 'auto' : 'smooth', block: 'center' });
  const control = controlIn(el, kind);
  if (control) {
    control.focus({ preventScroll: true });
    return;
  }
  el.tabIndex = -1;
  el.focus({ preventScroll: true });
}
