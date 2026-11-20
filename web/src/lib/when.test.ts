import { describe, expect, it } from 'vitest';
import { clock, dayLabel, dayOf, days, shortWhen } from './when';

/** A local time, so that the tests read the same in every time zone. */
const at = (year: number, month: number, day: number, hour = 12, minute = 0) =>
  new Date(year, month - 1, day, hour, minute).getTime();
const now = at(2026, 11, 20, 11, 44);

describe('dayOf', () => {
  it('is the local midnight before a time', () => {
    expect(dayOf(at(2026, 11, 20, 23, 59))).toBe(at(2026, 11, 20, 0, 0));
    expect(dayOf(at(2026, 11, 20, 0, 0))).toBe(at(2026, 11, 20, 0, 0));
    expect(dayOf(new Date(2026, 10, 20, 9, 58, 31, 250).getTime())).toBe(at(2026, 11, 20, 0, 0));
  });
});

describe('clock', () => {
  it('is the time of day on a 24-hour clock, in both languages', () => {
    expect(clock(at(2026, 11, 20, 9, 58), 'en')).toBe('09:58');
    expect(clock(at(2026, 11, 20, 0, 5), 'en')).toBe('00:05');
    expect(clock(at(2026, 11, 20, 23, 7), 'zh-CN')).toBe('23:07');
  });
});

describe('dayLabel', () => {
  it('names today and yesterday', () => {
    expect(dayLabel(at(2026, 11, 20, 0, 1), now, 'en')).toBe('today · November 20');
    expect(dayLabel(at(2026, 11, 20, 23, 59), now, 'en')).toBe('today · November 20');
    expect(dayLabel(at(2026, 11, 19, 23, 59), now, 'en')).toBe('yesterday · November 19');
    expect(dayLabel(at(2026, 11, 19, 0, 0), now, 'zh-CN')).toBe('昨天 · 11月19日');
    expect(dayLabel(now, now, 'zh-CN')).toBe('今天 · 11月20日');
  });

  it('gives the weekday of an earlier day of the year', () => {
    expect(dayLabel(at(2026, 11, 18), now, 'en')).toBe('Wednesday, November 18');
    expect(dayLabel(at(2026, 1, 1), now, 'en')).toBe('Thursday, January 1');
    expect(dayLabel(at(2026, 11, 18), now, 'zh-CN')).toBe('11月18日星期三');
  });

  it('gives the year of a day of another year, and the date of a day to come', () => {
    expect(dayLabel(at(2025, 11, 18), now, 'en')).toBe('November 18, 2025');
    expect(dayLabel(at(2025, 12, 31), at(2026, 1, 1), 'en')).toBe('yesterday · December 31');
    expect(dayLabel(at(2026, 11, 21), now, 'en')).toBe('Saturday, November 21');
  });

  it('counts calendar days across a change of the clock', () => {
    // 25 and 23 hours long where clocks change; one day apart everywhere.
    expect(dayLabel(at(2026, 10, 24, 23, 30), at(2026, 10, 25, 23, 30), 'en')).toBe(
      'yesterday · October 24',
    );
    expect(dayLabel(at(2026, 3, 28, 0, 30), at(2026, 3, 29, 23, 30), 'en')).toBe(
      'yesterday · March 28',
    );
  });
});

describe('shortWhen', () => {
  it('is the time today and "yesterday" the day before', () => {
    expect(shortWhen(at(2026, 11, 20, 9, 58), now, 'en')).toBe('09:58');
    expect(shortWhen(at(2026, 11, 19, 9, 58), now, 'en')).toBe('yesterday');
    expect(shortWhen(at(2026, 11, 19, 9, 58), now, 'zh-CN')).toBe('昨天');
  });

  it('is the weekday for two to six days ago', () => {
    expect(shortWhen(at(2026, 11, 18), now, 'en')).toBe('Wed');
    expect(shortWhen(at(2026, 11, 14), now, 'en')).toBe('Sat');
    expect(shortWhen(at(2026, 11, 17), now, 'zh-CN')).toBe('周二');
  });

  it('is the date from a week ago on, with the year for another year', () => {
    expect(shortWhen(at(2026, 11, 13), now, 'en')).toBe('Nov 13');
    expect(shortWhen(at(2026, 2, 3), now, 'zh-CN')).toBe('2月3日');
    expect(shortWhen(at(2025, 11, 13), now, 'en')).toBe('Nov 13, 2025');
  });

  it('is the date for a day to come', () => {
    expect(shortWhen(at(2026, 11, 21, 9, 0), now, 'en')).toBe('Nov 21');
  });
});

describe('days', () => {
  const msg = (id: string, by: string, ts: number) => ({ id, by, ts });

  it('puts messages under the day they were written on', () => {
    const a = msg('a', 'alex', at(2026, 11, 19, 23, 58));
    const b = msg('b', 'mina', at(2026, 11, 20, 0, 1));
    const c = msg('c', 'alex', at(2026, 11, 20, 9, 0));
    expect(days([a, b, c])).toEqual([
      { day: at(2026, 11, 19, 0), msgs: [{ msg: a, joined: false }] },
      {
        day: at(2026, 11, 20, 0),
        msgs: [
          { msg: b, joined: false },
          { msg: c, joined: false },
        ],
      },
    ]);
    expect(days([])).toEqual([]);
  });

  it('joins a message to the one before it of the same author within five minutes', () => {
    const start = at(2026, 11, 20, 9, 0);
    const list = [
      msg('a', 'alex', start),
      msg('b', 'alex', start + 4 * 60_000 + 59_999),
      msg('c', 'alex', start + 9 * 60_000 + 59_999),
      msg('d', 'mina', start + 10 * 60_000),
      msg('e', 'alex', start + 10 * 60_000),
      msg('f', 'alex', start + 9 * 60_000),
    ];
    expect(days(list)[0].msgs.map((each) => each.joined)).toEqual([
      false,
      true,
      false,
      false,
      false,
      true,
    ]);
  });

  it('does not join across midnight, and starts a day again when the clocks disagree', () => {
    const late = msg('a', 'alex', at(2026, 11, 19, 23, 59));
    const early = msg('b', 'alex', at(2026, 11, 20, 0, 1));
    const back = msg('c', 'alex', at(2026, 11, 19, 23, 58));
    expect(days([late, early, back])).toEqual([
      { day: at(2026, 11, 19, 0), msgs: [{ msg: late, joined: false }] },
      { day: at(2026, 11, 20, 0), msgs: [{ msg: early, joined: false }] },
      { day: at(2026, 11, 19, 0), msgs: [{ msg: back, joined: false }] },
    ]);
  });
});
