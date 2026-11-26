<script lang="ts">
  import Icon from '../Icon.svelte';
  import { offeredAccounts } from '../driver/driver';
  import { creating } from '../../lib/groups.svelte';
  import { accountLabel, cliName, type PairingSession, type Stage } from '../../lib/onboard.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { href } from '../../lib/route.svelte';

  let { session }: { session: PairingSession } = $props();

  const tones: Partial<Record<Stage, string>> = {
    linking: 'sky',
    accounts: 'dawn',
    ready: 'ok',
    expired: 'danger',
    failed: 'danger',
  };
  const dots: Record<Stage, string> = {
    waiting: 'waiting',
    linking: 'linking',
    accounts: 'needyou',
    ready: 'ok',
    expired: 'down',
    failed: 'down',
  };

  const nodeLine = $derived(
    session.node
      ? [session.node.name, session.node.os, session.node.version].filter(Boolean).join(' · ')
      : t(session.role === 'b' ? 'pairing.step.node_wait_browser' : 'pairing.step.node_wait'),
  );
  const connectLine = $derived(
    !session.enrolled
      ? t('pairing.step.connect_try')
      : session.path?.kind === 'relay'
        ? t('pairing.step.connect_relay')
        : session.path?.kind === 'direct'
          ? t('pairing.step.connect_direct')
          : t('pairing.step.connect_ready'),
  );
  const foundLine = $derived.by(() => {
    if (!session.agents) return t('pairing.step.ai_wait');
    if (session.agents.length === 0) return t('pairing.step.ai_none');
    const sep = prefs.lang === 'zh-CN' ? '、' : ', ';
    const names = session.agents.map((item) => cliName(item.agent)).join(sep);
    return t('pairing.step.ai_found').replace('{list}', names);
  });

  type StepMark = 'done' | 'active' | 'needyou' | 'error' | 'todo';

  function stepState(n: 1 | 2 | 3 | 4): StepMark {
    if (session.error?.step === n) return 'error';
    if (n <= 2) {
      if (session.node) return 'done';
      return n === 1 && session.stage === 'waiting' ? 'active' : 'todo';
    }
    if (n === 3) {
      if (session.enrolled) return 'done';
      return session.node && session.stage === 'linking' ? 'active' : 'todo';
    }
    if (session.stage === 'ready') return 'done';
    if (session.stage === 'accounts') return 'needyou';
    return session.enrolled && session.stage !== 'failed' ? 'active' : 'todo';
  }

  const errorText = $derived(
    session.error
      ? t(`pairing.error.${session.error.key}`).replace('{code}', session.error.code ?? '')
      : '',
  );
</script>

{#snippet marker(state: StepMark)}
  {#if state === 'done'}
    <span class="mark ok"><Icon name="check" /></span>
  {:else if state === 'needyou'}
    <span class="st step-dot" data-st="needyou" aria-hidden="true"><i></i></span>
  {:else if state === 'active'}
    <span class="spin" aria-hidden="true"></span>
  {:else if state === 'error'}
    <span class="mark err"><Icon name="alert" /></span>
  {:else}
    <Icon name="plus" />
  {/if}
{/snippet}

<section class="card" aria-labelledby="st-h">
  <header class="card-head">
    <h2 class="kind" id="st-h">{t('pairing.status.title')}</h2>
    <span class="badge" data-tone={tones[session.stage]}>
      <span class="st" data-st={dots[session.stage]}><i></i></span>
      <span>{t(`pairing.status.${session.stage}`)}</span>
    </span>
  </header>

  <div role="status" aria-live="polite">
    <ol class="link-steps">
      <li class:todo={stepState(1) === 'todo'}>
        {@render marker(stepState(1))}
        <div>
          <b>{t('pairing.step.node')}</b>
          <span class="muted">{nodeLine}</span>
        </div>
      </li>
      <li class:todo={stepState(2) === 'todo'}>
        {@render marker(stepState(2))}
        <div>
          <b>{t('pairing.step.identity')}</b>
          {#if session.node}
            <span class="muted">
              <span>{t('pairing.step.fingerprint')}</span>
              <code>{session.node.fingerprint}</code>
            </span>
          {:else}
            <span class="muted">{t('pairing.step.identity_wait')}</span>
          {/if}
        </div>
      </li>
      <li class:todo={stepState(3) === 'todo'}>
        {@render marker(stepState(3))}
        <div>
          <b>{t('pairing.step.connect')}</b>
          <span class="muted">{connectLine}</span>
        </div>
      </li>
      <li class:todo={stepState(4) === 'todo'}>
        {@render marker(stepState(4))}
        <div>
          <b>{t('pairing.step.ai')}</b>
          <span class={session.agents ? undefined : 'muted'}>{foundLine}</span>
        </div>
      </li>
    </ol>
  </div>

  {#if session.error}
    <p class="note" data-tone="danger" role="alert">
      <Icon name="alert" sm />
      <span>{errorText}</span>
    </p>
  {/if}

  <p class="note">
    <Icon name="info" sm />
    <span>{t('pairing.status.relay_note')}</span>
  </p>
</section>

{#each session.agents ?? [] as item (item.agent)}
  {@const label = cliName(item.agent)}
  <section class="card" class:need={item.account === null} aria-labelledby={`acct-h-${item.agent}`}>
    <header class="card-head">
      <h2 class="kind" id={`acct-h-${item.agent}`}>
        {t('pairing.acct.title').replace('{name}', label)}
      </h2>
    </header>
    <fieldset class="acct" disabled={session.busy}>
      <legend class="sr">{t('pairing.acct.legend')} · {label}</legend>
      {#each offeredAccounts(item.driver) as acct (acct)}
        <label>
          <input
            type="radio"
            name={`acct-${item.agent}`}
            value={acct}
            checked={item.account === acct}
            onchange={() => void session.setAccount(item.agent, acct)}
          />
          <span>
            <b>{accountLabel(acct)}</b>
            <span>{t(`pairing.acct.${acct}_body`)}</span>
          </span>
        </label>
      {/each}
    </fieldset>
    <p class="hint">{t('pairing.acct.hint')}</p>
  </section>
{/each}

{#if session.stage === 'ready'}
  <a class="btn btn-dawn done-btn" href={href('groups')} onclick={() => (creating.open = true)}>
    {t('pairing.done')}
  </a>
{/if}

<style>
  /* The steps as a list: a 20px glyph, the step, and under it what is known so far. */
  .link-steps li {
    display: grid;
    grid-template-columns: 20px minmax(0, 1fr);
    gap: 10px;
    padding: 10px 0;
    color: var(--text-3);
    font-size: var(--fs-xs);
    line-height: 1.5;
  }
  .link-steps li + li {
    border-top: 1px solid var(--line);
  }
  .link-steps li:first-child {
    padding-top: 0;
  }
  .link-steps b {
    display: block;
    color: var(--text);
    font-size: var(--fs-ui);
    font-weight: 500;
  }
  .link-steps span:not(.muted) {
    color: var(--text-2);
  }
  .link-steps :global(.i) {
    margin-top: 1px;
  }
  .link-steps .todo b {
    color: var(--text-3);
  }
  .step-dot {
    justify-content: center;
    width: 20px;
    height: 20px;
    margin-top: 1px;
  }
  .mark {
    display: inline-flex;
  }
  /* A step that is done lands. */
  .mark.ok {
    color: var(--ok);
    animation: done var(--dur-4) var(--spring);
  }
  @keyframes done {
    from {
      opacity: 0;
      transform: scale(0.6);
    }
  }
  .mark.err {
    color: var(--danger);
  }
  .spin {
    width: 16px;
    height: 16px;
    margin: 3px 2px;
    border-radius: 50%;
    border: 2px solid var(--surface-3);
    border-top-color: var(--sky);
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .done-btn {
    width: 100%;
    height: 44px;
  }
</style>
