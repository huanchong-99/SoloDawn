import { afterEach, expect, it, vi } from 'vitest';
import type { Group } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { all, closePage, show, texts } from '../../../features/page';
import StateSheet from '../../routes/dev/States.svelte';

const messages = vi.hoisted(() => [] as { group: Group }[]);
vi.mock('../chat/Message.svelte', () => ({
  default: (_anchor: unknown, props: { group: Group }) => {
    messages.push(props);
  },
}));
vi.mock('../../lib/mounts.svelte', async (original) => {
  const actual = await original<typeof import('../../lib/mounts.svelte')>();
  return { ...actual, mounts: { ...actual.mounts, bar: [] }, cards: new Map() };
});
afterEach(closePage);

it('documents empty mounting points and unmounted kinds and leaves demo signing pending', async () => {
  show(StateSheet, {});
  expect(texts('.points dd .hint')).toEqual(['Nothing mounted yet']);
  expect(all('.kind figure')).toHaveLength(
    Object.keys(goldens).filter((name) => name.startsWith('row-msg-')).length,
  );
  expect(texts('.kind header .badge').every((text) => text === 'One-line stub')).toBe(true);
  let signed = false;
  void messages[0].group.home.me.sign('demo').then(() => (signed = true));
  await Promise.resolve();
  expect(signed).toBe(false);
});
