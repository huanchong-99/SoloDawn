import { prefersReducedMotion } from 'svelte/motion';

/** Scrolls a main-line message `#m-<id>` into view without changing the hash route, and moves
 *  focus to its first enabled control, or to the message element itself when it has none. */
export function jumpToMessage(event: Event, id: string, kind?: string): void {
  event.preventDefault();
  const el = document.getElementById(`m-${id}`);
  if (!el) return;
  el.scrollIntoView({
    behavior: prefersReducedMotion.current ? 'auto' : 'smooth',
    block: 'center',
  });
  const selector =
    kind === 'decision'
      ? 'input:not([disabled]), textarea:not([disabled])'
      : 'button:not([disabled])';
  const control = el.querySelector<HTMLElement>(selector);
  if (control) {
    control.focus({ preventScroll: true });
    return;
  }
  el.tabIndex = -1;
  el.focus({ preventScroll: true });
}
