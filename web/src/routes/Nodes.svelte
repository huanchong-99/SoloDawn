<script lang="ts">
  import { tick } from 'svelte';
  import Avatar from '../components/Avatar.svelte';
  import Failure from '../components/Failure.svelte';
  import Icon from '../components/Icon.svelte';
  import { offeredAccounts } from '../components/driver/driver';
  import PairBrowserDialog from '../components/nodes/PairBrowserDialog.svelte';
  import NodeOffline from '../components/stop/NodeOffline.svelte';
  import { find } from '../lib/groups.svelte';
  import type { Net } from '../lib/net.svelte';
  import { nodes, outdated, type Computer } from '../lib/nodes.svelte';
  import { accountLabel, cliName, type AccountKind } from '../lib/onboard.svelte';
  import { prefs, t } from '../lib/prefs.svelte';
  import { href } from '../lib/route.svelte';
  import { now } from '../lib/when';

  let { net }: { net: Net } = $props();
  const uid = $props.id();

  $effect(() => nodes.sync(net.homes));

  const computers = $derived(nodes.list(net));
  const pairable = $derived(computers.filter((comp) => comp.online && comp.info !== undefined));
  let pairing = $state(false);
  let choosing = $state<Record<string, boolean>>({});
  let busyAcct = $state<Record<string, boolean>>({});
  let acctFailed = $state<Record<string, string>>({});

  function hostedGroups(comp: Computer) {
    const map = new Map<string, string>();
    if (comp.info) {
      for (const entry of comp.info.homes) {
        if (entry.home === comp.info.device) map.set(entry.g, entry.name);
      }
    }
    for (const group of comp.home.groups) map.set(group.g, group.name);
    return [...map].map(([g, name]) => ({ g, name }));
  }

  const stateTone = (state: string) => (state === 'dormant' ? 'sleeping' : state);

  function stateLabel(state: string) {
    const known = [
      'working',
      'waiting',
      'dormant',
      'limited',
      'offline',
      'takeover',
      'paused',
    ] as const;
    const match = known.find((each) => each === state);
    return match ? t(`nodes.ai.state.${match}`) : state;
  }

  function trackedCard(event: Event): HTMLElement | null {
    const card =
      event.currentTarget instanceof Element
        ? event.currentTarget.closest<HTMLElement>('.node')
        : null;
    return card?.contains(document.activeElement) ? card : null;
  }

  function focusNodeCard(card: HTMLElement | null) {
    if (!card) return;
    card.tabIndex = -1;
    card.focus({ preventScroll: true });
  }

  /** Declares the account type, then corrects the members of this CLI. The choice stays open. */
  async function chooseAccount(
    event: Event & { currentTarget: HTMLInputElement },
    comp: Computer,
    agent: string,
    before: string | null,
    account: AccountKind,
  ) {
    if (!comp.info) return;
    const { device } = comp.info;
    const radio = event.currentTarget;
    const card = trackedCard(event);
    const key = `${device}:${agent}`;
    choosing[key] = busyAcct[key] = true;
    acctFailed[key] = '';
    focusNodeCard(card);
    const answer = await nodes.setAccount(comp.home, device, agent, account);
    if (answer.err) {
      acctFailed[key] = answer.err.code;
      for (const each of radio.closest('fieldset')!.querySelectorAll('input'))
        each.checked = each.value === before;
    } else await correct(device, agent, account);
    busyAcct[key] = false;
    await tick();
    if (document.activeElement === card) radio.focus();
  }

  async function correct(device: string, agent: string, account: string) {
    const key = `${device}:${agent}`;
    busyAcct[key] = true;
    acctFailed[key] = (await nodes.correct(device, agent, account))?.code ?? '';
    busyAcct[key] = false;
  }
</script>

<div class="page">
  <div class="page-head">
    <div class="grow">
      <h2>{t('page.nodes')}</h2>
      <p>{t('nodes.lead')}</p>
    </div>
    <a class="btn" href={href('onboarding')}>
      <Icon name="plus" sm /><span>{t('nodes.add.computer')}</span>
    </a>
    <button type="button" class="btn" disabled={!pairable.length} onclick={() => (pairing = true)}>
      <Icon name="phone" sm /><span>{t('nodes.add.browser')}</span>
    </button>
  </div>

  {#if computers.length}
    {#each computers as comp, idx (comp.info?.device ?? comp.home)}
      {@const hosted = hostedGroups(comp)}
      {@const stopped = comp.online && comp.info?.offline}
      {@const relayed = comp.online && comp.path?.kind === 'relay'}
      <section class="card node" aria-labelledby={`${uid}-n-${idx}`}>
        <header class="row">
          <span class="node-ico"><Icon name="pc" /></span>
          <div class="grow">
            <h3 id={`${uid}-n-${idx}`}>{comp.info?.name ?? comp.home.name ?? ''}</h3>
            {#if comp.info}
              <p class="muted sub">
                {comp.info.os} · {t('nodes.node')}
                {comp.info.version}{#if comp.online}
                  {' '}· {t('nodes.sessions')
                    .replace('{open}', String(comp.info.open))
                    .replace('{max}', String(comp.info.max_open))}{/if}
              </p>
            {/if}
          </div>
          <span class="st" data-st={comp.online && !stopped ? 'ok' : 'offline'}>
            <i></i>{t(
              stopped
                ? 'stop.node.status'
                : comp.online
                  ? 'nodes.status.online'
                  : 'nodes.status.offline',
            )}
          </span>
        </header>

        {#if comp.online || hosted.length}
          <dl class="kv">
            {#if comp.online}
              <dt>{t('nodes.link')}</dt>
              <dd>
                {#if comp.path?.kind === 'direct'}
                  <span class="who">
                    <span class="ok-ico"><Icon name="direct" sm /></span>
                    <span>{t('nodes.conn.ok')}</span>
                    {#if comp.path.rtt !== undefined}
                      <span class="muted">· {comp.path.rtt} ms</span>
                    {/if}
                  </span>
                {:else if comp.path?.kind === 'relay'}
                  {@const mins = Math.max(1, Math.round((now() - comp.path.since) / 60_000))}
                  <span class="who relay-who">
                    <Icon name="relay" sm /><b>{t('nodes.relay')}</b>
                    <span class="muted">
                      {#if comp.path.rtt !== undefined}· {comp.path.rtt} ms{' '}{/if}· {t(
                        'nodes.relay.for',
                      ).replace('{n}', String(mins))}
                    </span>
                  </span>
                {:else}
                  <span class="muted">{t('nodes.conn.linking')}</span>
                {/if}
              </dd>
            {/if}
            {#if hosted.length}
              <dt>{t('nodes.hosts')}</dt>
              <dd>{hosted.map((group) => group.name).join(' · ')}</dd>
            {/if}
          </dl>
        {/if}

        {#if relayed}
          <div class="note" data-tone="dawn">
            <Icon name="alert" sm />
            <div class="grow">
              <b>{t('nodes.relay.title')}</b>
              {' '}{t('nodes.relay.body')}
            </div>
          </div>
        {/if}

        {#if hosted.length}
          <p class="note" data-tone="dawn">
            <Icon name="home" sm />
            <span>
              <b>
                {t(hosted.length === 1 ? 'nodes.hosts.warnOne' : 'nodes.hosts.warn').replace(
                  '{n}',
                  String(hosted.length),
                )}
              </b>
              {' '}{t('nodes.hosts.body')}
            </span>
          </p>
        {/if}

        {#if comp.online && comp.info}<NodeOffline home={comp.home} />{/if}

        {#if comp.info}
          {#each comp.info.agents.filter((entry) => entry.agent !== 'external') as agent (agent.agent)}
            {@const label = cliName(agent.agent)}
            {@const acctKey = `${comp.info.device}:${agent.agent}`}
            {@const open = agent.account === null || choosing[acctKey] === true}
            {@const behind = agent.account
              ? outdated(comp.info.device, agent.agent, agent.account)
              : []}
            <div class="ai-row">
              <div class="row">
                <Avatar id={agent.agent} name={label} cli={agent.agent} />
                <div class="grow">
                  <div class="row ai-title">
                    <b>{label}</b>
                    <span class="mono hint">v{agent.cli_version}</span>
                    <span class="badge" data-tone={agent.account ? undefined : 'dawn'}>
                      {agent.account ? accountLabel(agent.account) : t('nodes.account.unset')}
                    </span>
                  </div>
                  {#if !comp.online}
                    <p class="st" data-st="offline"><i></i>{t('nodes.ai.offline')}</p>
                  {:else if stopped}
                    <p class="st" data-st="offline"><i></i>{t('stop.ai.offline')}</p>
                  {:else if !agent.login_ok}
                    <p class="st" data-st="down"><i></i>{t('nodes.ai.notSignedIn')}</p>
                  {:else}
                    <p class="st" data-st="ok"><i></i>{t('nodes.ai.signedIn')}</p>
                  {/if}
                </div>
                {#if agent.account !== null}
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost turn"
                    aria-expanded={open}
                    aria-label={`${t('nodes.account.change')} · ${label}`}
                    onclick={() => (choosing[acctKey] = !open)}
                  >
                    {t('nodes.account.change')}<Icon name="down" sm />
                  </button>
                {/if}
              </div>

              {#if open}
                <header class="card-head">
                  <h4 class="kind">{t('pairing.acct.title').replace('{name}', label)}</h4>
                </header>
                <fieldset class="acct" disabled={!comp.online || busyAcct[acctKey]}>
                  <legend class="sr">{t('pairing.acct.legend')} · {label}</legend>
                  {#each offeredAccounts(agent.driver) as acct (acct)}
                    <label>
                      <input
                        type="radio"
                        name={`${uid}-acct-${idx}-${agent.agent}`}
                        value={acct}
                        checked={agent.account === acct}
                        onchange={(event) =>
                          void chooseAccount(event, comp, agent.agent, agent.account, acct)}
                      />
                      <span>
                        <b>{accountLabel(acct)}</b>
                        <span>{t(`pairing.acct.${acct}_body`)}</span>
                      </span>
                    </label>
                  {/each}
                </fieldset>
                <p class="hint">{t('pairing.acct.hint')}</p>
              {/if}
              {#if behind.length && !busyAcct[acctKey]}
                {@const device = comp.info.device}
                <div class="note behind" data-tone="dawn">
                  <Icon name="alert" sm />
                  <span class="grow">
                    {t('nodes.account.behind').replace('{groups}', () =>
                      behind.map(({ group }) => group.name).join(' · '),
                    )}
                  </span>
                  <button
                    type="button"
                    class="btn btn-sm"
                    onclick={() => void correct(device, agent.agent, agent.account!)}
                  >
                    {t('nodes.account.again')}
                  </button>
                </div>
              {/if}
              {#if acctFailed[acctKey]}<Failure code={acctFailed[acctKey]} />{/if}

              {#if agent.groups.length}
                {#each agent.groups as entry (entry.member)}
                  {@const group = find(entry.g)}
                  {@const groupName =
                    group?.name ?? comp.info.homes.find((h) => h.g === entry.g)?.name ?? entry.g}
                  {@const member = group?.member(entry.member)}
                  {@const isSec = member?.kind === 'ai' && member.secretary}
                  {@const state = comp.online ? entry.state : 'offline'}
                  {@const taskRow = group?.task(entry.task)}
                  <div class="perm">
                    <span class="row">
                      <a href={href('chat', { g: entry.g })}>{groupName}</a>
                      {#if member}<span class="muted">· {member.name}</span>{/if}
                      {#if isSec}
                        <span class="badge">{t('nodes.ai.secretary')}</span>
                      {/if}
                    </span>
                    <span class="st" data-st={stateTone(state)}>
                      <i></i>{stateLabel(state)}{#if taskRow}
                        {' '}· T-{taskRow.num}{/if}
                    </span>
                    <p class="path-row">
                      <Icon name="folder" sm />
                      <span class="sr">{t('nodes.ai.folder')}</span>
                      <code>{entry.workspace}</code>
                    </p>
                  </div>
                {/each}
              {:else}
                <p class="hint">{t('nodes.ai.nogroups')}</p>
              {/if}
            </div>
          {/each}
        {/if}
      </section>
    {/each}
  {:else}
    <div class="empty blank">
      <span class="blank-mark"><Icon name="pc" /></span>
      <h3>{t('nodes.empty.title')}</h3>
      <p>{t('nodes.empty.body')}</p>
    </div>
  {/if}
</div>

<PairBrowserDialog bind:open={pairing} {net} />

<style>
  /* The page's actions are small and plain. */
  .page-head .btn {
    height: var(--ctl-sm);
    padding: 0 10px;
    border-radius: 8px;
    font-size: var(--fs-sm);
  }
  /* A computer is a card: its head, what is known of it, and its AIs as a list under hairlines. */
  .node {
    padding: 0;
    gap: 0;
  }
  .node > header {
    gap: var(--sp-3);
    padding: var(--sp-4);
  }
  .node > header h3 {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    flex-wrap: wrap;
  }
  .sub {
    font-size: var(--fs-xs);
    margin-top: 2px;
  }
  /* Online or not, as a pill: told by the shape of its dot and by its words. */
  .node > header .st {
    height: 22px;
    padding: 0 10px 0 8px;
    border-radius: var(--r-full);
    background: var(--surface-2);
    font-size: var(--fs-xs);
  }
  .node > .kv,
  .node > .note {
    margin: 0 var(--sp-4) var(--sp-4);
  }
  /* This computer is the home of groups: it asks for attention, so it stands on dawn. */
  .node > p.note {
    background: var(--dawn-soft);
  }
  .node-ico {
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    border-radius: var(--r-md);
    background: var(--surface-2);
    color: var(--text-2);
  }
  .who {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    vertical-align: middle;
  }
  .ok-ico {
    display: inline-grid;
    color: var(--ok);
  }
  .relay-who {
    color: var(--dawn-text);
  }
  .ai-row {
    border-top: 1px solid var(--line);
    padding: var(--sp-4);
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
  }
  .ai-row > .row {
    gap: var(--sp-3);
  }
  .ai-title {
    gap: 6px;
  }
  .ai-title b {
    font-weight: 500;
  }
  .ai-row p.st {
    margin-top: 1px;
    font-size: var(--fs-xs);
  }
  .behind {
    align-items: center;
  }
  .turn {
    margin-right: -6px;
  }
  .turn :global(.i) {
    transition: transform var(--dur-2) var(--ease);
  }
  .turn[aria-expanded='true'] :global(.i) {
    transform: rotate(180deg);
  }
  .perm {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-2) var(--sp-3);
    font-size: var(--fs-sm);
  }
  .perm > span:first-child {
    flex: 1;
    min-width: 0;
  }
  /* The folder of the membership above it: a quiet line right under it, no fill. */
  .perm .path-row {
    flex-basis: 100%;
    align-self: stretch;
    min-height: 0;
    margin-top: calc(2px - var(--sp-2));
    padding: 0;
    background: none;
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .perm .path-row code {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .empty {
    margin-top: var(--sp-8);
  }
  @media (max-width: 640px) {
    .perm {
      flex-direction: column;
      align-items: flex-start;
      gap: 4px;
    }
    .perm .path-row {
      margin-top: -2px;
    }
  }
</style>
