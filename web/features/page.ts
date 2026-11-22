// The client's pages mounted in the test DOM, with the shell around them, and what a person does
// on them. happy-dom lays nothing out and has no input devices, so the two things the pages ask
// the browser are stood in for: whether the top of the main line is in view, and whether the
// person has a keyboard.
import { flushSync, mount, unmount, type Component } from 'svelte';
import { vi } from 'vitest';
import App from '../src/App.svelte';
import { homes } from '../src/lib/groups.svelte';
import { prefs } from '../src/lib/prefs.svelte';

let page: Record<string, never> | undefined;
const parts: Record<string, never>[] = [];
let inView: ((entries: { isIntersecting: boolean }[]) => void) | undefined;

/** Whether a media query holds on a screen of the given width, in a browser with a keyboard:
 *  `pointer: fine`, and `min-width` and `max-width` in pixels. */
function media(query: string, width: number) {
  const bound = /(min|max)-width:\s*(\d+)px/.exec(query);
  if (!bound) return query.includes('pointer: fine');
  return bound[1] === 'min' ? width >= Number(bound[2]) : width <= Number(bound[2]);
}

/** Opens the client at an address, in English, in a browser with a keyboard, on a screen of the
 *  given width as far as media queries in script go (the test DOM lays nothing out). The homes
 *  are those the test puts in `homes`. */
export function openPage(hash = '#/groups', width = 1440) {
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(seen: typeof inView) {
        inView = seen;
      }
      observe() {}
      disconnect() {}
    },
  );
  vi.spyOn(window, 'matchMedia').mockImplementation(
    (query) =>
      ({
        matches: media(query, width),
        addEventListener() {},
        removeEventListener() {},
        addListener() {},
        removeListener() {},
      }) as unknown as MediaQueryList,
  );
  prefs.lang = 'en';
  go(hash);
  page = mount(App, { target: document.body }) as Record<string, never>;
  flushSync();
}

/** Mounts one part of the pages on its own. */
export function show<Props extends Record<string, unknown>>(part: Component<Props>, props: Props) {
  parts.push(mount(part, { target: document.body, props }) as Record<string, never>);
  flushSync();
}

export async function closePage() {
  if (page) await unmount(page);
  page = undefined;
  for (const part of parts.splice(0)) await unmount(part);
  homes.length = 0;
  location.hash = '';
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
}

/** The address changes, as when the person follows a link of the page. */
export function go(hash: string) {
  location.hash = hash;
  dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
  flushSync();
}

/** The top of the main line comes into view, or leaves it. */
export function scrollToTop(isIntersecting = true) {
  inView!([{ isIntersecting }]);
  flushSync();
}

export const all = <E extends Element = HTMLElement>(selector: string) => [
  ...document.querySelectorAll<E>(selector),
];
export const one = <E extends Element = HTMLElement>(selector: string) =>
  document.querySelector<E>(selector)!;

/** The text of an element, with white space as a reader sees it. */
const read = (element: Element) => element.textContent!.replace(/\s+/g, ' ').trim();

/** The text of the first matching element, with white space as a reader sees it. */
export const text = (selector: string) => read(one(selector));

/** The texts of the matching elements, with white space as a reader sees it. */
export const texts = (selector: string) => all(selector).map(read);

export const field = () => one<HTMLTextAreaElement>('.composer textarea');

/** The person types: the field holds the text, with the caret at its end. */
export function type(text: string) {
  const input = field();
  input.value = text;
  input.setSelectionRange(text.length, text.length);
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

export function press(key: string, init: KeyboardEventInit = {}) {
  field().dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...init }));
  flushSync();
}

/** The person presses the send button. */
export function send() {
  one('.composer form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  flushSync();
}

/** Fills a field of a form as a person does. */
export function fill(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

/** The person chooses an option of a select. The test DOM does not match a chosen option with
 *  `:checked`, which is where Svelte reads the choice from; Svelte then takes the first option
 *  that is not disabled, so the other options are disabled for the moment of the choice. */
export function choose(select: HTMLSelectElement, label: string) {
  const others = [...select.options].filter((option) => option.textContent!.trim() !== label);
  for (const option of others) option.setAttribute('disabled', '');
  select.dispatchEvent(new Event('change', { bubbles: true }));
  for (const option of others) option.removeAttribute('disabled');
  flushSync();
}
