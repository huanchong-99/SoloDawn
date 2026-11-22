<script lang="ts">
  import type { Net } from '../../lib/net.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { defaultPushEnv, reminders, type PushState, type ReminderKind } from '../../lib/push';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** The Notifications card on `#/settings` (`web/design/settings.html` `#s-noti`). */
  let { net }: { net: Net } = $props();

  const order: readonly ReminderKind[] = ['mention', 'decision', 'approval', 'brief', 'done'];
  const dots: Record<PushState, string> = {
    allowed: 'ok',
    off: 'offline',
    denied: 'down',
    ios: 'limited',
    unsupported: 'offline',
  };

  let busy = $state(false);
  let failed = $state<string>();
  let tick = $state(0);

  const env = $derived.by(() => {
    void tick;
    return defaultPushEnv();
  });
  const status = $derived(reminders.state(env));
  const blocked = $derived(status === 'denied' || status === 'ios' || status === 'unsupported');

  async function toggleSwitch(event: Event & { currentTarget: HTMLInputElement }) {
    const input = event.currentTarget;
    const want = input.checked;
    busy = true;
    failed = undefined;
    const result = want
      ? await reminders.enable(net.homes, env)
      : await reminders.disable(net.homes, env);
    tick += 1;
    busy = false;
    input.checked = reminders.state(defaultPushEnv()) === 'allowed';
    if ('code' in result && result.code !== 'dismissed' && result.code !== 'denied') {
      failed = result.code;
    }
  }

  async function toggleKind(kind: ReminderKind, checked: boolean) {
    failed = undefined;
    const result = await reminders.setKind(kind, checked, net.homes, env);
    if ('code' in result) failed = result.code;
  }
</script>

<section class="card set" aria-labelledby="s-noti">
  <header>
    <h3 id="s-noti">{t('reminder.title')}</h3>
    <p>{t('reminder.lead')}</p>
  </header>
  <div class="set-row">
    <div>
      <b id="s-push">{t('reminder.browser')}</b>
      <p>
        <span class="st" data-st={dots[status]}>
          <i></i><span>{t(`reminder.status.${status}`)}</span>
        </span>
      </p>
    </div>
    <input
      class="switch"
      type="checkbox"
      role="switch"
      checked={status === 'allowed'}
      disabled={busy || blocked}
      aria-labelledby="s-push"
      onchange={toggleSwitch}
    />
  </div>
  <div class="set-row">
    <div><b>{t('reminder.when')}</b></div>
    <div class="stack">
      {#each order as kind (kind)}
        <label class="check">
          <input
            type="checkbox"
            checked={reminders.hasKind(kind)}
            onchange={(event) => void toggleKind(kind, event.currentTarget.checked)}
          />
          <span>{t(`reminder.kind.${kind}`)}</span>
        </label>
      {/each}
    </div>
  </div>
  {#if failed}
    <div class="err">
      {#if failed === 'subscribe_failed'}
        <p class="fail" role="alert">{t('reminder.error.subscribe')}</p>
      {:else}
        <Failure code={failed} />
      {/if}
    </div>
  {/if}
  {#if status === 'ios'}
    <p class="note ios" data-tone="dawn">
      <Icon name="phone" sm />
      <span>{t('reminder.ios')}</span>
    </p>
  {/if}
</section>

<style>
  .switch {
    appearance: none;
    flex: none;
    width: 42px;
    height: 24px;
    margin: 0;
    border: 0;
    border-radius: 12px;
    background: var(--line-strong);
    cursor: pointer;
    position: relative;

    &::before {
      content: '';
      position: absolute;
      top: 3px;
      left: 3px;
      width: 18px;
      height: 18px;
      border-radius: 50%;
      background: #fff;
      transition: transform var(--dur-2) var(--ease);
    }
    &:checked {
      background: var(--sky);
    }
    &:checked::before {
      transform: translateX(18px);
      background: var(--on-sky);
    }
    &:disabled {
      opacity: 0.5;
      cursor: not-allowed;
    }
  }
  .set-row:has(.stack) > div:first-child {
    flex: none;
  }
  .stack {
    display: flex;
    flex: 1;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 6px var(--sp-4);
  }
  .check {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--fs-sm);
    cursor: pointer;

    input {
      width: 16px;
      height: 16px;
      margin: 0;
      accent-color: var(--sky);
    }
  }
  .ios {
    margin: var(--sp-3) var(--sp-4) var(--sp-4);
  }
  .err {
    padding: 0 var(--sp-4) var(--sp-3);
  }
  .fail {
    color: var(--danger);
    font-size: var(--fs-sm);
  }
  @media (max-width: 640px) {
    .set-row:has(.stack) {
      flex-direction: column;
      align-items: stretch;
      gap: var(--sp-2);
    }
    .stack {
      justify-content: flex-start;
    }
  }
</style>
