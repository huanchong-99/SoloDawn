import { flushSync } from 'svelte';
import { afterEach, expect, it, vi } from 'vitest';
import { homes } from '../src/lib/groups.svelte';
import { JoinError, readInvitation } from '../src/lib/join';
import { goldens } from '../src/lib/mock';
import type { Net, Source } from '../src/lib/net.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import { href } from '../src/lib/route.svelte';
import { browser, example, owner, TestHome } from './home';
import { all, closePage, fill, go, one, openPage, text } from './page';

const join = vi.fn<NonNullable<Net['join']>>();
const hash = new URL(JSON.parse(goldens['cmd-invite-link']).link).hash;
const invitation = readInvitation(hash)!;
const { g } = invitation;

vi.mock('../src/lib/net.svelte', async (actual) => ({
  ...(await actual<typeof import('../src/lib/net.svelte')>()),
  start: (source: Source) => ({ net: { source, homes, path: () => undefined, join }, stop() {} }),
}));

afterEach(async () => {
  await closePage();
  join.mockReset();
});

const name = () => one<HTMLInputElement>('main input');
const submit = () => one<HTMLButtonElement>('main button[type="submit"], main form button');

it('rejects malformed invitations and accepts a valid hash in the same page', () => {
  openPage('#/join');
  expect(text('main [role=alert]')).toBe('This invitation link is invalid.');
  go('#join=invalid');
  expect(text('main [role=alert]')).toBe('This invitation link is invalid.');
  expect(document.querySelector('main form')).toBeNull();
  go(hash);
  expect(name().value).toBe('');
  expect(submit().disabled).toBe(true);
  expect(join).not.toHaveBeenCalled();
});

it('labels the join form, rejects blank names, and aborts a pending submission on navigation', async () => {
  const response = Promise.withResolvers<Awaited<ReturnType<NonNullable<Net['join']>>>>();
  join.mockReturnValueOnce(response.promise);
  openPage(hash);
  expect(text('main label span')).toBe('Your name in this group');
  expect(text('main .card a')).toBe('Cancel');
  expect(name().disabled).toBe(false);
  expect(name().value).toBe('');
  expect(submit().disabled).toBe(true);
  fill(name(), '   ');
  flushSync();
  expect(submit().disabled).toBe(true);
  fill(name(), '  Reader  ');
  flushSync();
  submit().click();
  flushSync();
  expect(join.mock.calls).toEqual([[invitation, 'Reader', expect.any(AbortSignal)]]);
  expect(name().disabled).toBe(true);
  expect(submit().disabled).toBe(true);
  const signal = join.mock.lastCall![2];
  go(href('groups'));
  expect(signal.aborted).toBe(true);
  response.resolve({ notifications: [] });
  await response.promise;
  expect(location.hash).toBe(href('groups'));
});

it('recommends pairing when unpaired and shows the link after partial notification', async () => {
  openPage(hash);
  expect(text('main h1')).toBe('Join a group');
  expect(document.querySelector('main [role=status]')).toBeNull();
  join.mockRejectedValueOnce(new JoinError('onboarding', 'unpaired'));
  fill(name(), 'Reader');
  flushSync();
  submit().click();
  await vi.waitFor(() =>
    expect(text('main [role=alert]')).toBe(
      'Connect a computer before joining. Then return to this invitation.',
    ),
  );
  expect(one('main .card a[href="#/onboarding"]').textContent).toBe('Connect a computer');
  join.mockResolvedValueOnce({
    notifications: [{ node: 'own', error: new JoinError('home.add', 'lost') }],
  });
  submit().click();
  await vi.waitFor(() =>
    expect(text('main [role=status]')).toBe(
      'You joined the group, but not all of your connected computers were notified.',
    ),
  );
  expect(one('main .card a').getAttribute('href')).toBe(href('chat', { g }));
});

it.each([
  ['owner', [], ['Make admin', 'Remove'], ['Make member', 'Remove']],
  ['admin', ['Leave'], ['Remove'], []],
  ['member', ['Leave'], [], []],
] as const)(
  'shows exactly the permitted removal and role actions for a %s',
  async (role, own, member, admin) => {
    const home = new TestHome();
    home.add(g, 'Website');
    home.write(
      g,
      example('row-member-human', {
        id: owner,
        g,
        name: 'Reader',
        role,
      }),
    );
    for (const [name, target] of [
      ['Owner', 'owner'],
      ['Member', 'member'],
      ['Admin', 'admin'],
    ]) {
      if (target === 'owner' && role === 'owner') continue;
      home.write(
        g,
        example('row-member-human', { id: name, user: name, g, name, role: target, online: true }),
      );
    }
    const client = browser(home);
    homes.push(client.store);
    await client.connect();
    openPage(href('chat', { g }));
    const row = (name: string) =>
      all('.person').find((row) =>
        row.querySelector('.person-name')?.textContent?.trim().startsWith(name),
      )!;
    const actions = (name: string) =>
      [...row(name).querySelectorAll(':scope > .person-actions > button')].map((b) =>
        b.textContent!.trim(),
      );
    await vi.waitFor(() => expect(row('Reader')).toBeDefined());
    expect(row('Admin').querySelector(':scope > .person-name')?.textContent?.trim()).toBe(
      'Admin · Admin',
    );
    expect(row('Admin').querySelector(':scope > .st')?.textContent?.trim()).toBe('Online');
    expect(actions('Reader')).toEqual(own);
    expect(actions('Member')).toEqual(member);
    expect(actions('Admin')).toEqual(admin);
    if (role !== 'owner') expect(actions('Owner')).toEqual([]);
  },
);

it('renders the join heading, explanatory text, and bilingual and theme controls', () => {
  openPage(hash);
  prefs.theme = 'light';
  flushSync();
  expect(text('main .badge')).toBe('Group invitation');
  expect(text('main h1')).toBe('Join a group');
  expect(text('main .hero p')).toBe('Choose the name people in this group will see.');
  expect(one('main .wrap.single > form.card')).not.toBeNull();
  expect(one('main .top .only-wide')).toBeNull();
  expect(text('main .top .swap[lang="zh-CN"]')).toBe('中');
  expect(text('main .top .sr [lang=en]')).toBe('Switch language');
  expect(text('main .top .sr [lang=zh-CN]')).toBe('切换语言');
  const light = one('main .top .swap svg');
  expect(light.querySelector('circle')).toBeNull();
  one('main button[aria-label="Toggle theme"]').click();
  flushSync();
  expect(one('main .top .swap svg')).not.toBe(light);
  expect(one('main .top .swap svg').querySelector('circle')).not.toBeNull();
  one('main .top button:first-of-type').click();
  flushSync();
  expect(text('main .top .swap[lang=en]')).toBe('EN');
  expect(text('main h1')).toBe('加入群聊');
});
