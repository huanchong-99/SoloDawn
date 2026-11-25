import { getContext, setContext } from 'svelte';
import type { Net } from '../lib/net.svelte';

const key = Symbol();

/** Gives the connection to everything inside: a card is drawn too deep to be handed it. */
export const provideNet = (net: Net) => setContext(key, net);

/** The connection, where the shell gave one. */
export const netHere = () => getContext<Net | undefined>(key);
