import type { Mounts } from '../../lib/mounts.svelte';
import Bulletin from './Bulletin.svelte';

export default { cards: { bulletin: Bulletin } } satisfies Mounts;
