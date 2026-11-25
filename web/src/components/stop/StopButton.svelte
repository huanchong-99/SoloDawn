<script lang="ts">
  import { tick } from 'svelte';
  import { MediaQuery } from 'svelte/reactivity';
  import { find, homes, type Group } from '../../lib/groups.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { route } from '../../lib/route.svelte';
  import { computers, mayPause, names, setPaused, working } from '../../lib/stop';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** The emergency stop. Its panel is the confirmation: each action in it says what it does. It is
   *  in the top bar, or in the head of a chat where that hides the top bar (`group`). */
  let { group: headOf }: { group?: Group } = $props();
  const uid = $props.id();

  let button = $state<HTMLButtonElement>();
  let panel = $state<HTMLElement>();
  let busy = $state('');
  let failed = $state('');

  const narrow = new MediaQuery('max-width: 767px');
  const inChat = $derived(route.page === 'chat');
  const here = $derived(headOf ? narrow.current : !(inChat && narrow.current));
  const group = $derived(headOf ?? (inChat ? find(route.params.g) : undefined));
  const toPause = $derived(group && mayPause(group) && !group.info?.paused ? group : undefined);
  const running = $derived(computers(homes).running);
  const reached = $derived(running.filter((home) => home.ctl));
  const away = $derived(running.filter((home) => !home.ctl));
  const busyAIs = $derived(toPause ? working(toPause) : 0);

  function opened(event: ToggleEvent) {
    if (event.newState !== 'open') return;
    failed = '';
    nodes.sync(homes, true);
    panel!.focus();
  }

  /** Runs one action; the panel stays open on a refusal. */
  async function act(name: string, run: () => Promise<string>) {
    busy = name;
    failed = '';
    // The pressed button is about to be disabled, and would let go of the focus.
    panel!.focus();
    failed = await run();
    busy = '';
    if (failed) return;
    panel?.hidePopover();
    await tick();
    (button ?? document.querySelector('main')!).focus();
  }

  const pause = (target: Group) =>
    act('group', async () => (await setPaused(target, true)).err?.code ?? '');

  const takeOffline = () =>
    act('mine', async () => {
      const answers = await Promise.all(reached.map((home) => nodes.setOffline(home, true)));
      return answers.find((answer) => answer.err)?.err?.code ?? '';
    });
</script>

{#if here && (toPause || running.length)}
  <button
    type="button"
    class="btn btn-sm btn-danger estop"
    bind:this={button}
    popovertarget={uid}
    aria-label={t('stop.button')}
  >
    <Icon name="pause" sm /><span>{t('stop.button')}</span>
  </button>
  <div
    class="panel"
    id={uid}
    popover="auto"
    role="group"
    aria-labelledby={`${uid}-h`}
    tabindex="-1"
    bind:this={panel}
    ontoggle={opened}
  >
    <h2 class="sec-title" id={`${uid}-h`}>{t('stop.panel')}</h2>
    {#if toPause}
      <section>
        <div>
          <h3>{t('stop.group.title')}</h3>
          <p>
            {t('stop.group.body')}
            {#if busyAIs}
              <b>
                {t(busyAIs === 1 ? 'stop.group.working.one' : 'stop.group.working').replace(
                  '{n}',
                  String(busyAIs),
                )}
              </b>
            {/if}
          </p>
          {#if !toPause.home.ctl}<p class="away">{t('stop.group.away')}</p>{/if}
        </div>
        <button
          type="button"
          class="btn btn-danger"
          disabled={Boolean(busy) || !toPause.home.ctl}
          aria-busy={busy === 'group'}
          onclick={() => void pause(toPause)}
        >
          <Icon name="pause" sm />{t('stop.group.action')}
        </button>
      </section>
    {/if}
    {#if running.length}
      <section>
        <div>
          <h3>{t('stop.mine.title')}</h3>
          <p>{t('stop.mine.body').replace('{names}', () => names(running, prefs.lang))}</p>
          {#if away.length}
            <p class="away">
              {t('stop.mine.away').replace('{names}', () => names(away, prefs.lang))}
            </p>
          {/if}
        </div>
        <button
          type="button"
          class="btn btn-danger"
          disabled={Boolean(busy) || !reached.length}
          aria-busy={busy === 'mine'}
          onclick={() => void takeOffline()}
        >
          <Icon name="power" sm />{t(busy === 'mine' ? 'stop.mine.busy' : 'stop.mine.action')}
        </button>
      </section>
    {/if}
    {#if failed}<Failure code={failed} />{/if}
  </div>
{/if}

<style>
  .panel {
    position: fixed;
    inset: calc(var(--bar-h) + var(--sp-2)) var(--sp-4) auto auto;
    width: min(400px, calc(100vw - 2 * var(--sp-3)));
    margin: 0;
    padding: var(--sp-4);
    border: 0;
    border-radius: var(--r-lg);
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow-pop);
    outline: none;
  }
  .panel:popover-open {
    display: grid;
    gap: var(--sp-3);
    animation:
      drop var(--dur-4) var(--spring),
      appear var(--dur-2) var(--ease-out);
  }
  section {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--sp-4);
  }
  section + section {
    padding-top: var(--sp-3);
    border-top: 1px solid var(--line);
  }
  section .btn {
    min-width: 112px;
  }
  h3 {
    font-size: var(--fs-md);
  }
  p {
    margin-top: 2px;
    color: var(--text-2);
    font-size: var(--fs-sm);
    line-height: 1.5;
  }
  p b {
    color: var(--text);
    font-weight: 500;
  }
  .away {
    color: var(--danger);
  }
  @media (max-width: 767px) {
    .panel {
      right: var(--sp-3);
    }
    .estop {
      width: 36px;
      padding: 0;
    }
    .estop span {
      display: none;
    }
  }
  @media (pointer: coarse) {
    .estop {
      min-width: 44px;
    }
  }
</style>
