import type { Component } from 'svelte';
import { describe, expect, it } from 'vitest';
import MemberPanel from '../components/member/MemberPanel.svelte';
import { card, cards, gather, mounts, points, side } from './mounts.svelte';
import { files } from './mounts.glob';

// Stand-ins for components: gather only places them.
// A stand-in fits any point: gather only places it.
const part = (name: string) => name as unknown as Component<any>;
const at = (order: number, name: string) => ({ order, component: part(name) });
const placed = (entries: { component: unknown; area: string; order: number }[]) =>
  entries.map(({ component, area, order }) => `${String(component)}@${area}:${order}`);

describe('gather', () => {
  it('has every point empty and no card when no area mounts anything', () => {
    const found = gather({});
    expect(Object.keys(found.at)).toEqual([...points]);
    for (const point of points) expect(found.at[point]).toEqual([]);
    expect(found.cards.size).toBe(0);
    expect(found.card('approval')).toBeUndefined();
  });

  it('names each entry after the folder of its area', () => {
    const found = gather({
      '../components/status/mounts.ts': { banner: [at(1, 'Tab')] },
      '/src/components/notify/mounts.ts': { banner: [at(2, 'Toast')] },
    });
    expect(placed(found.at.banner)).toEqual(['Tab@status:1', 'Toast@notify:2']);
  });

  it('orders the entries of a point by order, then by area, and keeps the order within an area', () => {
    const found = gather({
      '../components/permission/mounts.ts': { bar: [at(20, 'Stop')] },
      '../components/nodes/mounts.ts': {
        bar: [at(10, 'Indicator'), at(20, 'Count'), at(20, 'Late')],
      },
      '../components/audit/mounts.ts': { bar: [at(20, 'Bell')], layer: [at(1, 'Toasts')] },
    });
    expect(placed(found.at.bar)).toEqual([
      'Indicator@nodes:10',
      'Bell@audit:20',
      'Count@nodes:20',
      'Late@nodes:20',
      'Stop@permission:20',
    ]);
    expect(placed(found.at.layer)).toEqual(['Toasts@audit:1']);
    expect(found.at['chat.side']).toEqual([]);
  });

  it('puts every point of an area where it belongs', () => {
    const found = gather({
      '../components/member/mounts.ts': {
        'chat.head': [at(1, 'Head')],
        'chat.top': [at(1, 'Top')],
        'chat.side': [at(1, 'Side')],
      },
    });
    expect(placed(found.at['chat.head'])).toEqual(['Head@member:1']);
    expect(placed(found.at['chat.top'])).toEqual(['Top@member:1']);
    expect(placed(found.at['chat.side'])).toEqual(['Side@member:1']);
  });

  it('mounts the people panel in the chat sidebar', () => {
    expect(mounts['chat.side']).toContainEqual({
      area: 'member',
      order: 10,
      component: MemberPanel,
    });
  });

  it('gives the card of each kind, with its area', () => {
    const found = gather({
      '../components/approval/mounts.ts': { cards: { approval: part('Approval') } },
      '../components/brief/mounts.ts': {
        cards: { brief: part('Brief'), decision: part('Decision') },
      },
    });
    expect(found.card('approval')).toBe('Approval');
    expect(found.card('decision')).toBe('Decision');
    expect(found.card('task')).toBeUndefined();
    expect([...found.cards].map(([kind, { area }]) => `${kind}@${area}`)).toEqual([
      'approval@approval',
      'brief@brief',
      'decision@brief',
    ]);
  });

  it('refuses a kind of card that two areas mount, naming both', () => {
    expect(() =>
      gather({
        '../components/approval/mounts.ts': { cards: { approval: part('Approval') } },
        '../components/thread/mounts.ts': { cards: { approval: part('Another') } },
      }),
    ).toThrow(new Error('the card approval is mounted by approval and by thread'));
  });
});

describe('the mounts of this client', () => {
  it('are gathered from components/<area>/mounts.ts', () => {
    const expected = gather(files);
    expect(mounts).toEqual(expected.at);
    expect(cards).toEqual(expected.cards);
    for (const [kind] of expected.cards) {
      expect(card(kind as Parameters<typeof card>[0])).toBeDefined();
    }
  });

  it('start with the side panel of a chat closed', () => {
    expect(side).toEqual({ open: false });
  });
});
