import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import { find, homes } from '../src/lib/groups.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import * as p from '../src/lib/proto';
import { browser, example, ownedNode, type OwnedNode, type TestHome } from './home';
import { all, closePage, one, openPage, read, text, texts } from './page';

const feature = await loadFeature('../features/web_stop.feature');
const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const mina = 'bWluYS1tZW1iZXItMDAwMQ';
const paused = { by: mina, at: new Date(2026, 10, 24, 17, 6).getTime() };

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let workstation: OwnedNode;
  let mini: OwnedNode;
  let home: TestHome;
  const hidden = vi.fn();

  /** The payloads of one type that a home received, or a computer as host commands. */
  const ops = (from: TestHome) =>
    from.asked
      .filter((req) => req.t === 'cmd')
      .map((req) => JSON.parse((req.env as p.Envelope).p) as { t: string; op?: string });
  const commands = (node: OwnedNode) =>
    node.commands.map((env) => {
      const { t, to } = p.decode(p.HostCommandOnly, env.p);
      expect(to).toBe(node.device);
      return t;
    });
  const settled = (check: () => void) =>
    vi.waitFor(() => {
      flushSync();
      check();
    });
  const button = (name: string, within = document.body as Element) =>
    [...within.querySelectorAll('button')].find((each) => read(each) === name)!;
  const press = (name: string, within?: Element) => {
    button(name, within).click();
    flushSync();
  };
  const stop = () => one<HTMLButtonElement>('.estop');
  /** The panel opens as the browser opens the popover of its button. */
  const openPanel = async () => {
    expect(stop().getAttribute('popovertarget')).toBe(one('.panel').id);
    for (const type of ['beforetoggle', 'toggle'])
      one('.panel').dispatchEvent(Object.assign(new Event(type), { newState: 'open' }));
    await tick();
    expect(document.activeElement).toBe(one('.panel'));
  };
  const openGroup = async (width?: number) => {
    openPage(`#/groups/${g}`, width);
    await settled(() => expect(nodes.infos.size).toBe(homes.filter((each) => each.ctl).length));
  };
  const openSettings = async () => {
    openPage('#/settings');
    await settled(() => expect(nodes.infos.size).toBe(homes.filter((each) => each.ctl).length));
  };
  const withMini = async () => {
    mini = ownedNode('mac-mini', 'bWluaS1kZXZpY2UtMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAw', {
      agents: [],
    });
    homes.push(mini.store);
    await mini.connect();
  };
  const pauseByMina = () =>
    home.write(g, { ...home.groups.get(g)!.rows.get(`group ${g}`)!, paused } as never);
  const refuseHost = (node: OwnedNode) => {
    const ctl = node.store.ctl!;
    const request = ctl.request;
    node.store.ctl = {
      request: async (req: { t: string }) =>
        req.t === 'cmd'
          ? { re: 1, err: { code: 'forbidden', msg: 'not the owner' } }
          : request(req as never),
    } as p.Ctl;
  };
  const notAllowed = (within: string) =>
    settled(() => expect(text(`${within} .fail`)).toBe('Not sent: this device may not do that.'));

  AfterEachScenario(async () => {
    await closePage();
    nodes.infos.clear();
  });

  Background(({ Given }) => {
    Given(
      'a browser of Alex, who owns "workstation", the home of "Website" with a working AI',
      async () => {
        HTMLElement.prototype.hidePopover = hidden;
        workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
        home = workstation.home;
        home.add(g, 'Website');
        home.write(g, example('row-member-human', { id: mina, g, name: 'Mina', user: 'mina' }));
        home.write(g, example('row-member-ai', { g, state: 'working', task: undefined }));
        homes.push(workstation.store);
        await workstation.connect();
        await vi.waitFor(() => expect(find(g)?.info).toBeDefined());
      },
    );
  });

  Scenario('The owner pauses the group and resumes it', ({ When, Then, And }) => {
    When('Alex opens "Website" and then "Stop AIs"', async () => {
      await openGroup();
      await openPanel();
    });
    Then(
      'the panel offers to pause the group, where one AI is working, and to take the AIs on workstation offline',
      () => {
        expect(read(stop())).toBe('Stop AIs');
        expect(all('.chat-head .estop')).toEqual([]);
        expect(texts('.panel h3')).toEqual(['Pause this group', 'Take all my AIs offline']);
        expect(texts('.panel p')).toEqual([
          'Every AI here stops at once. New messages wait until someone resumes. One is working now.',
          'On workstation, in every group. Sessions close; messages wait until you bring them back.',
        ]);
        expect(texts('.panel button')).toEqual(['Pause', 'Take offline']);
        home.write(g, example('row-member-ai', { id: 'second', g, state: 'working' }));
        flushSync();
        expect(texts('.panel p b')).toEqual(['2 are working now.']);
        home.write(g, example('row-member-ai', { id: 'second', g, state: 'dormant' }));
        home.write(g, example('row-member-ai', { g, state: 'dormant' }));
        flushSync();
        expect(all('.panel p b')).toEqual([]);
      },
    );
    When('Alex presses "Pause"', async () => {
      press('Pause');
      expect(button('Pause').disabled).toBe(true);
      await settled(() => expect(hidden).toHaveBeenCalledOnce());
    });
    Then('the home receives "group pause" and the panel closes', async () => {
      expect(ops(home).map(({ t, op }) => [t, op])).toEqual([['group', 'pause']]);
      await settled(() => expect(document.activeElement).toBe(stop()));
    });
    And(
      'above the composer it says that Alex paused the group, what becomes of new messages, and "Resume"',
      () => {
        const time = new Intl.DateTimeFormat('en', {
          hour: '2-digit',
          minute: '2-digit',
          hourCycle: 'h23',
        }).format(find(g)!.info!.paused!.at);
        expect(text('.dock .paused b')).toBe(`You paused this group · ${time}`);
        expect(text('.paused p')).toBe(
          'The AIs have stopped. New messages reach them once it resumes.',
        );
        expect(texts('.paused button')).toEqual(['Resume']);
      },
    );
    And(
      'the group list marks "Website" as paused, and "Stop AIs" no longer offers to pause it',
      () => {
        expect(texts('.glist .st')).toEqual(['Paused']);
        expect(one('.glist .st').dataset.st).toBe('paused');
        expect(texts('.panel h3')).toEqual(['Take all my AIs offline']);
      },
    );
    When('Alex presses "Resume"', () => press('Resume'));
    Then(
      'the home receives "group resume", the notice and the mark are gone and focus is in the composer',
      async () => {
        await settled(() => expect(all('.paused')).toEqual([]));
        expect(ops(home).map(({ op }) => op)).toEqual(['pause', 'resume']);
        expect(all('.glist .st')).toEqual([]);
        await settled(() => expect(document.activeElement).toBe(one('.composer textarea')));
      },
    );
  });

  Scenario('A refused pause stays in the panel with its reason', ({ Given, When, And, Then }) => {
    Given('the home refuses commands with "forbidden"', () => {
      home.refuse = 'forbidden';
    });
    When('Alex opens "Website" and then "Stop AIs"', async () => {
      await openGroup();
      await openPanel();
    });
    And('Alex presses "Pause"', () => press('Pause'));
    Then('the panel is still open and says that this is not allowed', async () => {
      await notAllowed('.panel');
      expect(hidden).not.toHaveBeenCalled();
      expect(button('Pause').disabled).toBe(false);
      // Opened again, the panel starts without the old refusal.
      await openPanel();
      flushSync();
      expect(all('.panel .fail')).toEqual([]);
    });
  });

  Scenario('A refused resume says why beside the button', ({ Given, And, When, Then }) => {
    Given('Mina paused the group', () => void pauseByMina());
    And('the home refuses commands with "forbidden"', () => {
      home.refuse = 'forbidden';
    });
    When('Alex opens "Website"', () => openGroup());
    Then('above the composer it says that Mina paused the group', () => {
      vi.spyOn(Date, 'now').mockReturnValue(paused.at + 86_400_000);
      expect(text('.paused b')).toBe('Mina paused this group · Nov 24, 17:06');
    });
    When('Alex presses "Resume"', () => press('Resume'));
    Then('the notice says that this is not allowed', async () => {
      await notAllowed('.paused');
      expect(button('Resume').disabled).toBe(false);
    });
  });

  Scenario(
    'A member sees who paused the group and cannot resume it',
    ({ Given, And, When, Then }) => {
      Given('Alex is an ordinary member of a group on a computer of someone else', async () => {
        workstation.drop();
        homes.length = 0;
        home.write(g, example('row-member-human', { g, name: 'Alex', role: 'member' }));
        const alex = browser(home);
        homes.push(alex.store);
        await alex.connect();
        alex.store.owner = false;
        await vi.waitFor(() => expect(find(g)?.info).toBeDefined());
      });
      And('Mina paused the group', () => void pauseByMina());
      When('Alex opens "Website"', () => openPage(`#/groups/${g}`));
      Then(
        'above the composer it says that Mina paused the group and that an admin resumes it, without a button',
        async () => {
          await settled(() => expect(all('.paused')).toHaveLength(1));
          expect(text('.paused b')).toContain('Mina paused this group');
          expect(text('.paused p')).toBe(
            'The AIs have stopped. New messages reach them once an admin resumes it.',
          );
          expect(all('.paused button')).toEqual([]);
        },
      );
      And('there is no "Stop AIs"', () => expect(all('.estop')).toEqual([]));
    },
  );

  Scenario('The home is away', ({ Given, When, Then }) => {
    Given('"workstation" stops', () => workstation.drop());
    When('Alex opens "Website" and then "Stop AIs"', async () => {
      await openGroup();
      await openPanel();
    });
    Then('both actions are disabled and the panel says that the computer cannot be reached', () => {
      expect(all<HTMLButtonElement>('.panel button').map((each) => each.disabled)).toEqual([
        true,
        true,
      ]);
      expect(texts('.panel .away')).toEqual([
        "The group's computer cannot be reached now.",
        'Cannot be reached now: workstation.',
      ]);
    });
  });

  Scenario(
    'The owner takes the AIs on their computers offline from any page, and back',
    ({ Given, When, Then, And }) => {
      Given('a second computer "mac-mini" of Alex', withMini);
      When('Alex opens the settings page and then "Stop AIs"', async () => {
        await openSettings();
        await openPanel();
      });
      Then('the panel offers only to take the AIs on workstation and mac-mini offline', () => {
        expect(texts('.panel h3')).toEqual(['Take all my AIs offline']);
        expect(text('.panel p')).toContain('On workstation and mac-mini, in every group.');
      });
      When('Alex presses "Take offline"', () => {
        press('Take offline');
        expect(button('Taking offline…').disabled).toBe(true);
      });
      Then(
        'both computers receive "host.offline", the panel closes and "Stop AIs" is gone',
        async () => {
          await settled(() => expect(all('.estop, .panel')).toEqual([]));
          expect([workstation, mini].map(commands)).toEqual([['host.offline'], ['host.offline']]);
          await settled(() => expect(document.activeElement).toBe(one('main')));
        },
      );
      And('under the top bar it says that the AIs on workstation and mac-mini are offline', () => {
        expect(text('.ban .off [role="status"]')).toBe(
          'Your AIs on workstation and mac-mini are offline. Messages to them wait.',
        );
      });
      And('the AIs of "Website" read "Offline · you took it offline"', async () => {
        location.hash = `#/groups/${g}`;
        dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
        home.write(g, example('row-member-ai', { g, state: 'offline', task: undefined }));
        flushSync();
        expect(texts('.member p.st')).toEqual(['Offline · you took it offline']);
        // The group can still be paused: the button is back for it.
        expect(texts('.panel h3')).toEqual(['Pause this group']);
      });
      When('Alex presses "Bring back online"', () => press('Bring back online'));
      Then('both computers receive "host.online" and the banner is gone', async () => {
        await settled(() => expect(all('.off')).toEqual([]));
        expect([workstation, mini].map(commands)).toEqual([
          ['host.offline', 'host.online'],
          ['host.offline', 'host.online'],
        ]);
        expect(document.activeElement).toBe(one('main'));
      });
    },
  );

  Scenario('One computer of two cannot be reached', ({ Given, And, When, Then }) => {
    Given('a second computer "mac-mini" of Alex', withMini);
    And('"mac-mini" stops', () => mini.drop());
    When('Alex opens the settings page and then "Stop AIs"', async () => {
      await openSettings();
      await openPanel();
    });
    Then('the panel says that mac-mini cannot be reached now', () =>
      expect(texts('.panel .away')).toEqual(['Cannot be reached now: mac-mini.']),
    );
    When('Alex presses "Take offline"', async () => {
      press('Take offline');
      await settled(() => expect(hidden).toHaveBeenCalledOnce());
    });
    Then('only workstation receives "host.offline"', () => {
      expect([workstation, mini].map(commands)).toEqual([['host.offline'], []]);
    });
  });

  Scenario(
    'A computer that refuses is told in the panel and under the top bar',
    ({ Given, When, And, Then }) => {
      Given('"workstation" refuses host commands', () => refuseHost(workstation));
      When('Alex opens the settings page and then "Stop AIs"', async () => {
        await openSettings();
        await openPanel();
      });
      And('Alex presses "Take offline"', () => press('Take offline'));
      Then('the panel is still open and says that this is not allowed', async () => {
        await notAllowed('.panel');
        expect(hidden).not.toHaveBeenCalled();
      });
      When('the AIs of "workstation" are offline and Alex presses "Bring back online"', () => {
        nodes.infos.set(workstation.store, {
          ...nodes.infos.get(workstation.store)!,
          offline: true,
        });
        flushSync();
        press('Bring back online');
      });
      Then('the banner says that this is not allowed', () => notAllowed('.off'));
    },
  );

  Scenario('On the computers page the owner confirms in place', ({ When, And, Then }) => {
    const strip = () => one('.strip');
    const asBefore = () => {
      expect(text('.strip b')).toBe('All AIs on this computer');
      expect(text('.strip p')).toBe('Stop them at once, in every group.');
      expect(texts('.strip button')).toEqual(['Take offline']);
      expect(text('.node header .st')).toBe('Online');
    };
    When('Alex opens the computers page', async () => {
      openPage('#/nodes');
      await settled(() => expect(all('.strip')).toHaveLength(1));
      asBefore();
    });
    And('Alex presses "Take offline" on "workstation"', () => press('Take offline', strip()));
    Then('the card says what happens and focus is on its strip', async () => {
      expect(text('.strip p')).toBe(
        'Running turns stop and every session closes. Messages wait until you bring the AIs back.',
      );
      expect(texts('.strip button')).toEqual(['Take offline', 'Cancel']);
      await settled(() => expect(document.activeElement).toBe(strip()));
    });
    When('Alex presses "Cancel"', () => press('Cancel', strip()));
    Then('the card is as before and "workstation" has received nothing', async () => {
      asBefore();
      expect(workstation.commands).toEqual([]);
      await settled(() => expect(document.activeElement).toBe(strip()));
    });
    When('Alex presses "Take offline" on "workstation" twice', () => {
      press('Take offline', strip());
      press('Take offline', strip());
      expect(all<HTMLButtonElement>('.strip button:disabled').map(read)).toEqual([
        'Taking offline…',
        'Cancel',
      ]);
    });
    Then(
      '"workstation" receives "host.offline" and its card and its AIs say that they are offline',
      async () => {
        await settled(() => expect(texts('.strip button')).toEqual(['Bring back online']));
        expect(commands(workstation)).toEqual(['host.offline']);
        expect(text('.strip b')).toBe('The AIs on this computer are offline');
        expect(text('.node header .st')).toBe('AIs offline');
        expect(one('.node header .st').dataset.st).toBe('offline');
        expect(texts('.ai-row p.st')).toEqual([
          'Offline · you took it offline',
          'Offline · you took it offline',
        ]);
      },
    );
    And('no banner repeats it on this page', () => expect(all('.off')).toEqual([]));
    When('Alex presses "Bring back online"', () => press('Bring back online', strip()));
    Then('"workstation" receives "host.online" and the card is as before', async () => {
      await settled(asBefore);
      expect(commands(workstation)).toEqual(['host.offline', 'host.online']);
    });
  });

  Scenario('Another browser of the owner takes the AIs offline', ({ When, And, Then }) => {
    When('Alex opens "Website"', () => openGroup());
    And('another browser takes the AIs on "workstation" offline', () => {
      expect(all('.off')).toEqual([]);
      workstation.info.offline = true;
      home.write(
        g,
        example('row-member-ai', {
          g,
          state: 'offline',
          task: undefined,
          host: workstation.device,
        }),
      );
      flushSync();
    });
    Then('under the top bar it says that the AIs on workstation are offline', () =>
      settled(() => expect(text('.off [role="status"]')).toContain('Your AIs on workstation')),
    );
  });

  Scenario(
    'After a reload a computer that connects later is read on every page',
    ({ Given, When, Then, And }) => {
      Given(
        'the AIs of "workstation" are offline, with no AI in a session, and its connection is down',
        () => {
          workstation.drop();
          workstation.info.offline = true;
          home.write(g, example('row-member-ai', { g, state: 'dormant', task: undefined }));
        },
      );
      When('Alex opens the settings page and then "workstation" connects', async () => {
        openPage('#/settings');
        await workstation.connect();
      });
      Then('under the top bar it says that the AIs on workstation are offline', () =>
        settled(() => expect(text('.off [role="status"]')).toContain('Your AIs on workstation')),
      );
      And('"Stop AIs" is gone, for there is nothing to stop', () =>
        expect(all('.estop')).toEqual([]),
      );
    },
  );

  Scenario('The panel opens under its button wherever the button is', ({ When, And, Then }) => {
    When('Alex opens "Website" and the button is pushed down to 155 px', async () => {
      await openGroup();
      vi.spyOn(stop(), 'getBoundingClientRect').mockReturnValue({ bottom: 155 } as DOMRect);
    });
    And('Alex opens "Stop AIs"', openPanel);
    Then('the panel starts 8 px under the button', () => {
      expect(one('.panel').style.top).toBe('163px');
    });
  });

  Scenario('On a phone the button is in the head of the chat', ({ When, Then }) => {
    When('Alex opens "Website" on a screen 390 px wide', () => openGroup(390));
    Then('"Stop AIs" is in the head of the chat and not in the top bar', () => {
      expect(all('.estop')).toHaveLength(1);
      expect(one('.chat-head .estop').getAttribute('aria-label')).toBe('Stop AIs');
    });
  });
});
