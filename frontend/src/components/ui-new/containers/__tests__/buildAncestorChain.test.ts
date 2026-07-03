import { describe, it, expect } from 'vitest';
import { buildAncestorChain } from '../CreateChatBoxContainer';

interface D {
  id: string;
  parentDraftId: string | null;
}

const draft = (id: string, parentDraftId: string | null = null): D => ({
  id,
  parentDraftId,
});

describe('buildAncestorChain (rounds thread grouping)', () => {
  it('returns [] for a round-1 draft or missing data', () => {
    expect(buildAncestorChain([draft('a')], null)).toEqual([]);
    expect(buildAncestorChain([draft('a')], undefined)).toEqual([]);
    expect(buildAncestorChain(undefined, 'a')).toEqual([]);
  });

  it('orders a linear chain oldest-first', () => {
    // r1 <- r2 <- r3, active draft is r4 (parent r3).
    const drafts = [draft('r2', 'r1'), draft('r3', 'r2'), draft('r1')];
    const chain = buildAncestorChain(drafts, 'r3');
    expect(chain.map((d) => d.id)).toEqual(['r1', 'r2', 'r3']);
  });

  it('truncates when an ancestor is missing from the list', () => {
    const drafts = [draft('r3', 'r2')]; // r2 not loaded
    const chain = buildAncestorChain(drafts, 'r3');
    expect(chain.map((d) => d.id)).toEqual(['r3']);
  });

  it('terminates on a cycle instead of looping forever', () => {
    const drafts = [draft('a', 'b'), draft('b', 'a')];
    const chain = buildAncestorChain(drafts, 'a');
    expect(chain.map((d) => d.id)).toEqual(['b', 'a']);
  });
});
