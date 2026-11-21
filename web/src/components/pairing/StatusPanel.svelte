<script lang="ts">
  import Icon from '../Icon.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { href } from '../../lib/route.svelte';
  import PairingIcon from './PairingIcon.svelte';
  import { cliName, type AccountKind, type PairingSession, type Stage } from './session.svelte';

  let { session }: { session: PairingSession } = $props();

  const accounts: AccountKind[] = ['personal', 'team', 'api_key'];
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
      : t('pairing.step.node_wait'),
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

  function stepState(n: 1 | 2 | 3 | 4): 'done' | 'active' | 'error' | 'todo' {
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
    return session.enrolled && session.stage !== 'failed' ? 'active' : 'todo';
  }

  const errorText = $derived(
    session.error
      ? t(`pairing.error.${session.error.key}`).replace('{code}', session.error.code ?? '')
      : '',
  );
</script>

{#snippet marker(state: 'done' | 'active' | 'error' | 'todo')}
  {#if state === 'done'}
    <span class="mark ok"><Icon name="check" /></span>
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

  <ol class="link-steps" role="status" aria-live="polite">
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

  {#if session.error}
    <p class="note" data-tone="danger" role="alert">
      <Icon name="alert" sm />
      <span>{errorText}</span>
    </p>
  {/if}

  <p class="note">
    <PairingIcon name="info" sm />
    <span>{t('pairing.status.relay_note')}</span>
  </p>

  {#if session.stage === 'ready'}
    <a class="btn btn-dawn done-btn" href={href('groups')}>{t('pairing.done')}</a>
  {:else if session.stage === 'expired' || session.stage === 'failed'}
    <button class="btn" onclick={() => void session.start()}>{t('pairing.prompt.renew')}</button>
  {/if}
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
      {#each accounts as acct (acct)}
        <label>
          <input
            type="radio"
            name={`acct-${item.agent}`}
            value={acct}
            checked={item.account === acct}
            onchange={() => void session.setAccount(item.agent, acct)}
          />
          <span>
            <b>{t(`pairing.acct.${acct}`)}</b>
            <span>{t(`pairing.acct.${acct}_body`)}</span>
          </span>
        </label>
      {/each}
    </fieldset>
    <p class="hint">{t('pairing.acct.hint')}</p>
  </section>
{/each}

<style>
  .card-head {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    flex-wrap: wrap;
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .kind {
    font-weight: 600;
    font-size: var(--fs-sm);
    color: var(--text);
  }
  .card-head .badge {
    margin-left: auto;
  }
  .st {
    display: inline-flex;
    align-items: center;
  }
  .st i {
    position: relative;
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    color: var(--text-3);
    background: currentColor;
  }
  .st[data-st='waiting'] i {
    background: none;
    border: 2px solid currentColor;
  }
  .st[data-st='linking'] i {
    color: var(--sky);
    animation: breatheDot 1.2s var(--ease) infinite;
  }
  .st[data-st='needyou'] i {
    color: var(--dawn);
  }
  .st[data-st='ok'] i {
    color: var(--ok);
  }
  .st[data-st='down'] i {
    color: var(--danger);
    background: none;
    border: 2px solid currentColor;
  }
  @keyframes breatheDot {
    50% {
      opacity: 0.35;
      transform: scale(0.82);
    }
  }
  .link-steps li {
    display: grid;
    grid-template-columns: 22px minmax(0, 1fr);
    gap: var(--sp-2);
    padding: 10px 0;
    font-size: var(--fs-sm);
  }
  .link-steps li + li {
    border-top: 1px solid var(--line);
  }
  .link-steps b {
    display: block;
    font-size: 14px;
  }
  .link-steps :global(.i) {
    margin-top: 2px;
  }
  .link-steps .todo {
    color: var(--text-3);
  }
  .mark {
    display: inline-flex;
  }
  .mark.ok {
    color: var(--ok);
  }
  .mark.err {
    color: var(--danger);
  }
  .spin {
    width: 16px;
    height: 16px;
    margin: 3px 1px;
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
  .note {
    display: flex;
    gap: var(--sp-2);
    padding: 10px 12px;
    border-radius: var(--r-md);
    background: var(--tone-soft, var(--surface-2));
    color: var(--text);
    font-size: var(--fs-sm);
  }
  .note :global(.i) {
    color: var(--tone-text, var(--text-3));
    margin-top: 2px;
  }
  .done-btn {
    width: 100%;
  }
  .acct {
    display: grid;
    gap: var(--sp-2);
  }
  .acct label {
    display: flex;
    gap: var(--sp-3);
    padding: 10px 12px;
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    cursor: pointer;
    font-size: var(--fs-sm);
    color: var(--text-2);
  }
  .acct label:has(:checked) {
    border-color: var(--sky);
    background: var(--sky-soft);
  }
  .acct input {
    margin-top: 4px;
    accent-color: var(--sky);
  }
  .acct b {
    display: block;
    color: var(--text);
    font-size: 14px;
  }
</style>
