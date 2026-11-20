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
  unsent,
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
    // A name is a mention only after an @, wherever it stands.
    expect(parts('xCodex', members)).toEqual([{ kind: 'text', text: 'xCodex' }]);
    expect(parts('@x Codex', members)).toEqual([{ kind: 'text', text: '@x Codex' }]);
    expect(parts('Codex @Codex', members)).toEqual([
      { kind: 'text', text: 'Codex ' },
      { kind: 'at', text: '@Codex', id: 'x' },
    ]);
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

describe('unsent', () => {
  it('is what was typed after the text that was sent', () => {
    expect(unsent('first and more', 'first')).toBe(' and more');
    expect(unsent('first', 'first')).toBe('');
    expect(unsent(' first \nnext', ' first ')).toBe('\nnext');
  });

  it('is the whole text when the sent text itself was changed meanwhile', () => {
    expect(unsent('fist', 'first')).toBe('fist');
    expect(unsent('the first', 'first')).toBe('the first');
    expect(unsent('', 'first')).toBe('');
  });
});

describe('the size of a row', () => {
  /** The row a home would write for a person's post, from the wire example of one: its size as
   *  the home measures it. `number` is its `seq` and its `pos`: three digits in the example. */
  function real(text: string, mentions: string[], number = 790) {
    const row = JSON.parse(goldens['row-msg-text']);
    const payload = JSON.parse(row.env.p);
    delete payload.as;
    payload.text = row.text = text;
    payload.mentions = row.mentions = mentions;
    row.env.p = JSON.stringify(payload);
    row.seq = row.pos = number;
    return new TextEncoder().encode(JSON.stringify(row)).length;
  }
  const member = JSON.parse(goldens['row-member-ai']).id as string;
  const named = (count: number) => Array.from({ length: count }, () => member);
  // The longest a sequence number can be: sixteen digits.
  const longest = Number.MAX_SAFE_INTEGER;

  // The contract's own figures (contracts/rows.md section 5).
  it('counts 2 bytes for a letter, 5 for a line break, 6 for a quote, a backslash or a Chinese character', () => {
    const rest = rowSize('');
    expect(rest).toBe(627);
    expect(rowSize('a') - rest).toBe(2);
    expect(rowSize('<&>') - rest).toBe(6);
    expect(rowSize('\n') - rest).toBe(5);
    expect(rowSize('"') - rest).toBe(6);
    expect(rowSize('\\') - rest).toBe(6);
    expect(rowSize('中') - rest).toBe(6);
    expect(rowSize('a "中"\n\\') - rest).toBe(2 + 2 + 6 + 6 + 6 + 5 + 6);
  });

  it('counts 52 bytes for each member mentioned', () => {
    expect(rowSize('', 1) - rowSize('')).toBe(52);
    expect(rowSize('@Codex', 3) - rowSize('@Codex')).toBe(156);
    expect(rowSize('a', 20)).toBe(627 + 1040 + 2);
  });

  it('is the size of the real row with the longest numbers, give or take a mention', () => {
    const text = 'Run `go test ./...`,\n then "say" 完成 \\o/';
    for (const count of [0, 1, 5, 20]) {
      const over = rowSize(text, count) - real(text, named(count), longest);
      expect([count, over >= 0, over <= 2]).toEqual([count, true, true]);
    }
    // The numbers of the wire example are short: 26 bytes less.
    expect(rowSize(text) - real(text, [])).toBe(26);
  });

  it('never allows a text whose row would be over the limit, with 20 members mentioned', () => {
    const allowed = Array.from({ length: 100 }, (_, i) => 'a'.repeat(7300 + i)).filter(
      (text) => room(text, 20) !== 'over',
    );
    expect(allowed.map((text) => text.length)).toEqual(
      Array.from({ length: 59 }, (_, i) => 7300 + i),
    );
    expect(allowed.filter((text) => real(text, named(20), longest) > rowLimit)).toEqual([]);
    // The longest one allowed makes a row three bytes under the limit; two letters more do not fit.
    expect(real('a'.repeat(7358), named(20), longest)).toBe(16381);
    expect(real('a'.repeat(7360), named(20), longest)).toBe(16385);
    // Without its mentions counted, a text that makes a row over the limit would pass.
    expect(room('a'.repeat(7400))).toBe('near');
    expect(real('a'.repeat(7400), named(20), longest)).toBeGreaterThan(rowLimit);
  });

  it('is at most 16 KiB', () => {
    expect([rowLimit, nearLimit]).toEqual([16384, 12288]);
  });

  it('is told as hundredths of the limit, rounded up', () => {
    // 627 + 5 + 2 n bytes for a line break and n letters.
    expect(fill('')).toBe(4);
    expect(fill('\n' + 'a'.repeat(3780))).toBe(50);
    expect(fill('\n' + 'a'.repeat(7876))).toBe(100);
    expect(fill('\n' + 'a'.repeat(7877))).toBe(101);
    expect(fill('中'.repeat(5188))).toBe(194);
    expect(fill('', 20)).toBe(11);
  });

  it('has room to spare, nears the limit, or is over it', () => {
    expect(room('')).toBe('ok');
    expect(room('\n' + 'a'.repeat(5827))).toBe('ok');
    expect(room('\n' + 'a'.repeat(5828))).toBe('near');
    expect(room('\n' + 'a'.repeat(7876))).toBe('near');
    expect(room('\n' + 'a'.repeat(7877))).toBe('over');
    expect(room('中'.repeat(2700))).toBe('over');
    expect(room('\n' + 'a'.repeat(5802), 1)).toBe('near');
    expect(room('\n' + 'a'.repeat(5801), 1)).toBe('ok');
  });
});
