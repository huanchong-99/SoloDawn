// The mounting points of the shell: where each area of the interface puts its components without
// editing the shell, the chat or another area. An area declares what it mounts in
// `components/<area>/mounts.ts`, whose default export is a `Mounts`; this module finds every such
// file by its name and gathers them. `Mount.svelte` draws the entries of a point.
import type { Component } from 'svelte';
import type { Group } from './groups.svelte';
import type { Net } from './net.svelte';
import type { MsgRow } from './proto';

export type Kind = MsgRow['kind'];

/** The message of each kind. */
type Of = { [K in Kind]: Extract<MsgRow, { kind: K }> };

/** What the card of a message is given: its group, and the message, of its kind. */
export interface CardProps<K extends Kind = Kind> {
  group: Group;
  msg: Of[K];
}

/** The points, each with what a component mounted there is given. */
export interface Points {
  /** The top bar, before the theme and language buttons, on every page and at every width:
   *  global state, such as the connection indicator and the emergency stop. */
  bar: { net: Net };
  /** Under the top bar, across the page: a notice about the whole client. A banner with nothing
   *  to say draws nothing, and then takes no room. */
  banner: { net: Net };
  /** After the page, above it: dialogs and notifications. */
  layer: { net: Net };
  /** The head of a chat, after the group's name: buttons. */
  'chat.head': { group: Group };
  /** Between the head of a chat and its main line. */
  'chat.top': { group: Group };
  /** Beside the main line of a chat from 1024 px; below that, a panel that slides over the chat
   *  when the person opens it with the button the chat's head then shows. */
  'chat.side': { group: Group };
}
export type Point = keyof Points;
export const points = [
  'bar',
  'banner',
  'layer',
  'chat.head',
  'chat.top',
  'chat.side',
] as const satisfies readonly Point[];

/** One component at a point. The lower `order` comes first; the order of areas breaks a tie. A
 *  component that needs nothing of what the point gives takes no props. */
export interface Entry<P extends Points[Point]> {
  order: number;
  component: Component<P> | Component<{}>;
}

/** What one area mounts. */
export type Mounts = { [P in Point]?: Entry<Points[P]>[] } & {
  /** The card of a message, by kind, drawn in place of the one-line stub of the main line, and
   *  wherever a thread draws a message. One area per kind. */
  cards?: { [K in Kind]?: Component<CardProps<K>> };
};

type Placed<P extends Points[Point]> = Entry<P> & { area: string };
type Any = { order: number; component: unknown; area: string };

/** Gathers the mounts of every area, by file path (`../components/<area>/mounts.ts`): the entries
 *  of each point in order, and the card of each kind of message with its area. A kind of card
 *  that two areas mount throws, naming both. */
export function gather(files: Record<string, Mounts>) {
  const at = Object.fromEntries(points.map((point) => [point, [] as Any[]]));
  const cards = new Map<string, { area: string; component: unknown }>();
  for (const [path, mounts] of Object.entries(files)) {
    const area = path.split('/').at(-2)!;
    for (const [kind, component] of Object.entries(mounts.cards ?? {})) {
      const taken = cards.get(kind);
      if (taken) throw new Error(`the card ${kind} is mounted by ${taken.area} and by ${area}`);
      cards.set(kind, { area, component });
    }
    for (const point of points) {
      at[point].push(...(mounts[point] ?? []).map((entry) => ({ ...entry, area })));
    }
  }
  for (const point of points) {
    at[point].sort((a, b) => a.order - b.order || a.area.localeCompare(b.area));
  }
  return {
    at: at as unknown as { [P in Point]: Placed<Points[P]>[] },
    cards: cards as ReadonlyMap<string, { area: string }>,
    /** The card of a message's kind, if an area mounts one. */
    card: <K extends Kind>(kind: K) =>
      cards.get(kind)?.component as Component<CardProps<K>> | undefined,
  };
}

/** What the areas of this client mount: the entries of each point (`mounts`), the card of each
 *  kind with its area (`cards`), and the card of one kind (`card`). */
export const {
  at: mounts,
  cards,
  card,
} = gather(
  import.meta.glob<Mounts>('../components/*/mounts.ts', { eager: true, import: 'default' }),
);

/** Whether the side panel of the open chat slides over it, below 1024 px. */
export const side = $state({ open: false });
