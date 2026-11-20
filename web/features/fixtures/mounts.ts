// What stand-in areas mount, for the scenarios of the shell's mounting points (shell.spec.ts),
// by the path that `gather` reads them from: `components/<area>/mounts.ts`.
import type { Mounts } from '../../src/lib/mounts.svelte';
import Card from './Card.svelte';
import ChatPart from './ChatPart.svelte';
import Indicator from './Indicator.svelte';
import Notice from './Notice.svelte';
import Stop from './Stop.svelte';

type Areas = Record<string, Mounts>;

/** Two areas with a part of the top bar each: "Pause all" at 20, the indicator at 10. */
export const bar: Areas = {
  '../components/permission/mounts.ts': { bar: [{ order: 20, component: Stop }] },
  '../components/nodes/mounts.ts': { bar: [{ order: 10, component: Indicator }] },
};

/** An area with a notice as a banner and as a layer. */
export const notices: Areas = {
  '../components/status/mounts.ts': {
    banner: [{ order: 1, component: Notice }],
    layer: [{ order: 1, component: Notice }],
  },
};

/** An area with the card of approvals and of the home's notices. */
export const cards: Areas = {
  '../components/approval/mounts.ts': { cards: { approval: Card, system: Card } },
};

/** An area with a part in the head of a chat, above its main line and beside it. */
export const chat: Areas = {
  '../components/member/mounts.ts': {
    'chat.head': [{ order: 1, component: ChatPart }],
    'chat.top': [{ order: 1, component: ChatPart }],
    'chat.side': [{ order: 1, component: ChatPart }],
  },
};
