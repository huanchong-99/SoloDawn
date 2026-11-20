import { describe, expect, it } from 'vitest';
import {
  complete,
  fill,
  matching,
  mentioned,
  nearLimit,
  parts,
  room,
  rowLimit,
  rowSize,
  typing,
} from './compose';
import { goldens } from './mock';

const claude = { id: 'c', name: 'Claude' };
const claudeCode = { id: 'cc', name: 'Claude Code' };
const codex = { id: 'x', name: 'Codex' };
const secretary = { id: 's', name: '秘书' };
const members = [claude, claudeCode, codex, secretary];

describe('typing', () => {
  it('finds the mention that ends at the caret', () => {
    expect(typing('@', 1)).toEqual({ at: 0, query: '' });
    expect(typing('ask @Cl', 7)).toEqual({ at: 4, query: 'Cl' });
    expect(typing('@a @bc', 6)).toEqual({ at: 3, query: 'bc' });
    expect(typing('@Claude Co', 10)).toEqual({ at: 0, query: 'Claude Co' });
  });

  it('reads no further than the caret', () => {
    expect(typing('ask @Claude now', 7)).toEqual({ at: 4, query: 'Cl' });
    expect(typing('@Codex', 0)).toBeUndefined();
    expect(typing('hi @Codex', 2)).toBeUndefined();
  });

  it('finds none without an @, or after the line of one', () => {
    expect(typing('hello', 5)).toBeUndefined();
    expect(typing('', 0)).toBeUndefined();
    expect(typing('@Codex\nnext', 11)).toBeUndefined();
  });
});

describe('matching', () => {
  it('keeps the members whose name starts with what was typed, whatever its case', () => {
    expect(matching(members, 'cl')).toEqual([claude, claudeCode]);
    expect(matching(members, 'CLAUDE c')).toEqual([claudeCode]);
    expect(matching(members, '秘')).toEqual([secretary]);
    expect(matching(members, '')).toEqual(members);
    expect(matching(members, 'ode')).toEqual([]);
    expect(matching(members, 'x')).toEqual([]);
  });
});

describe('complete', () => {
  it('replaces what was typed by the name and a space, and puts the caret after it', () => {
    expect(complete('ask @Cl', 7, 4, 'Claude Code')).toEqual({
      text: 'ask @Claude Code ',
      caret: 17,
    });
    expect(complete('ask @co please', 7, 4, 'Codex')).toEqual({
      text: 'ask @Codex  please',
      caret: 11,
    });
    expect(complete('@', 1, 0, '秘书')).toEqual({ text: '@秘书 ', caret: 4 });
  });
});

describe('parts', () => {
  it('is the text itself when it has no code and no mention', () => {
    expect(parts('The path is direct.', members)).toEqual([
      { kind: 'text', text: 'The path is direct.' },
    ]);
    expect(parts('', members)).toEqual([]);
    expect(parts('mail me@example.org', members)).toEqual([
      { kind: 'text', text: 'mail me@example.org' },
    ]);
  });

  it('cuts out the mentions of the members given', () => {
    expect(parts('@Codex and @秘书你好, see @Codex.', members)).toEqual([
      { kind: 'at', text: '@Codex', id: 'x' },
      { kind: 'text', text: ' and ' },
      { kind: 'at', text: '@秘书', id: 's' },
      { kind: 'text', text: '你好, see ' },
      { kind: 'at', text: '@Codex', id: 'x' },
      { kind: 'text', text: '.' },
    ]);
    expect(parts('ask @Codex', [claude])).toEqual([{ kind: 'text', text: 'ask @Codex' }]);
    expect(parts('ask @Codex', members)).toEqual([
      { kind: 'text', text: 'ask ' },
      { kind: 'at', text: '@Codex', id: 'x' },
    ]);
  });

  it('takes the longest name where one name begins another', () => {
    expect(parts('@Claude Code and @Claude', members)).toEqual([
      { kind: 'at', text: '@Claude Code', id: 'cc' },
      { kind: 'text', text: ' and ' },
      { kind: 'at', text: '@Claude', id: 'c' },
    ]);
    expect(parts('@Claude Code', [claudeCode, claude])).toEqual([
      { kind: 'at', text: '@Claude Code', id: 'cc' },
    ]);
  });

  it('takes a name that runs on into a letter, a digit or an underscore for another word', () => {
    for (const text of ['@Claudette', '@Codex2', '@Codex_x', '@codex'])
      expect(parts(text, members)).toEqual([{ kind: 'text', text }]);
    expect(parts('@Claude Codec', members)).toEqual([
      { kind: 'at', text: '@Claude', id: 'c' },
      { kind: 'text', text: ' Codec' },
    ]);
  });

  it('shows what is between backticks as code, mentions and all', () => {
    expect(parts('run `go test ./...` then tell @Codex', members)).toEqual([
      { kind: 'text', text: 'run ' },
      { kind: 'code', text: 'go test ./...' },
      { kind: 'text', text: ' then tell ' },
      { kind: 'at', text: '@Codex', id: 'x' },
    ]);
    expect(parts('`@Codex` `a``b`', members)).toEqual([
      { kind: 'code', text: '@Codex' },
      { kind: 'text', text: ' ' },
      { kind: 'code', text: 'a' },
      { kind: 'code', text: 'b' },
    ]);
  });

  it('takes a backtick that nothing closes for text', () => {
    expect(parts('a `b` c `d @Codex', members)).toEqual([
      { kind: 'text', text: 'a ' },
      { kind: 'code', text: 'b' },
      { kind: 'text', text: ' c `d ' },
      { kind: 'at', text: '@Codex', id: 'x' },
    ]);
    expect(parts('`', members)).toEqual([{ kind: 'text', text: '`' }]);
  });
});

describe('mentioned', () => {
  it('lists the members a text mentions, each once, in the order of first mention', () => {
    expect(mentioned('@秘书 ask @Codex, then @秘书 again', members)).toEqual(['s', 'x']);
    expect(mentioned('nobody here, not even `@Codex`', members)).toEqual([]);
  });
});

describe('the size of a row', () => {
  // The contract's own figures (contracts/rows.md section 5).
  it('counts 2 bytes for a letter, 5 for a line break, 6 for a quote, a backslash or a Chinese character', () => {
    const rest = rowSize('');
    expect(rest).toBe(820);
    expect(rowSize('a') - rest).toBe(2);
    expect(rowSize('<&>') - rest).toBe(6);
    expect(rowSize('\n') - rest).toBe(5);
    expect(rowSize('"') - rest).toBe(6);
    expect(rowSize('\\') - rest).toBe(6);
    expect(rowSize('中') - rest).toBe(6);
    expect(rowSize('a "中"\n\\') - rest).toBe(2 + 2 + 6 + 6 + 6 + 5 + 6);
  });

  it('is no less than a real row holds, and within a quarter KiB of it', () => {
    const row = JSON.parse(goldens['row-msg-text']);
    const real = new TextEncoder().encode(JSON.stringify(row)).length;
    const over = rowSize(row.text) - real;
    expect(over).toBeGreaterThan(0);
    expect(over).toBeLessThan(256);
  });

  it('is at most 16 KiB', () => {
    expect([rowLimit, nearLimit]).toEqual([16384, 12288]);
  });

  it('is told as hundredths of the limit', () => {
    expect(fill('')).toBe(6);
    expect(fill('a'.repeat(7782))).toBe(100);
    expect(fill('a'.repeat(7783))).toBe(101);
    expect(fill('a'.repeat(3686))).toBe(50);
    expect(fill('中'.repeat(5188))).toBe(195);
  });

  it('has room to spare, nears the limit, or is over it', () => {
    // 820 + 2 n bytes for n letters.
    expect(room('')).toBe('ok');
    expect(room('a'.repeat(5733))).toBe('ok');
    expect(room('a'.repeat(5734))).toBe('near');
    expect(room('a'.repeat(7782))).toBe('near');
    expect(room('a'.repeat(7783))).toBe('over');
    expect(room('中'.repeat(2600))).toBe('over');
  });
});
