<script lang="ts">
  import { untrack } from 'svelte';
  import { homes, lost, newId, type Group, type Home } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { HostInfo } from '../../lib/proto';
  import Avatar from '../Avatar.svelte';
  import Failure from '../Failure.svelte';

  /** The dialog that adds a registered AI from one of the owner's computers to the group
   *  (contracts/commands.md section 2, `host.join`). */
  let { group, open = $bindable(false) }: { group: Group; open?: boolean } = $props();
  const uid = $props.id();

  const cliTitle = (agent: string) =>
    agent === 'claude' ? 'Claude Code' : agent === 'codex' ? 'Codex' : agent;
  const accountName = (account: string) =>
    account === 'personal' || account === 'team' || account === 'api_key'
      ? t(`member.account.${account}`)
      : account;

  const owned = $derived(homes.filter((home) => home.owner && home.ctl));
  const canSecretary = $derived(
    !group.info?.secretary ||
      (group.me?.kind === 'human' && (group.me.role === 'owner' || group.me.role === 'admin')),
  );

  let dialog: HTMLDialogElement;
  let infos = $state.raw(new Map<Home, HostInfo>());
  let name = $state('');
  let customName = $state(false);
  let secretary = $state(false);
  let loading = $state(false);
  let busy = $state(false);
  let failed = $state('');
  let asked = new Set<Home>();

  const candidates = $derived(
    owned.flatMap((home) => {
      const info = infos.get(home);
      if (!info) return [];
      return info.agents.map((entry) => ({
        key: `${info.device}:${entry.agent}`,
        home,
        device: info.device,
        hostName: info.name,
        agent: entry.agent,
        account: entry.account,
        loginOk: entry.login_ok,
        inGroup:
          entry.groups.some((g) => g.g === group.g) ||
          group.members.some(
            (m) => m.kind === 'ai' && m.host === info.device && m.cli === entry.agent,
          ),
      }));
    }),
  );
  type Candidate = (typeof candidates)[number];
  const selectable = $derived(candidates.filter((c) => c.account !== null && !c.inGroup));
  let chosen = $state.raw<Candidate>();

  const suggestedName = (agent: string | undefined, asSecretary: boolean) =>
    asSecretary && !group.info?.secretary
      ? t('member.join.secretary.name')
      : agent
        ? cliTitle(agent)
        : '';

  async function loadHosts(targets: Home[]) {
    const fresh = targets.filter((home) => !asked.has(home) && home.ctl);
    if (!fresh.length) return;
    for (const home of fresh) asked.add(home);
    loading = true;
    const results = await Promise.all(
      fresh.map(async (home) => ({
        home,
        answer: await home.ctl!.request({ t: 'host.info' }).catch(lost),
      })),
    );
    loading = false;
    if (!open) return;
    const next = new Map(infos);
    let firstErr = '';
    for (const { home, answer } of results) {
      if (answer.err) firstErr ||= answer.err.code;
      else next.set(home, answer);
    }
    infos = next;
    if (!infos.size && firstErr) failed = firstErr;
  }

  $effect(() => {
    if (!open) {
      if (dialog.open) dialog.close();
      return;
    }
    untrack(() => {
      asked = new Set();
      infos = new Map();
      chosen = undefined;
      name = '';
      customName = false;
      secretary = false;
      failed = '';
    });
    if (!dialog.open) dialog.showModal();
  });

  $effect(() => {
    if (!open) return;
    void loadHosts(owned);
    if (!chosen && selectable.length) {
      const first = selectable[0];
      chosen = first;
      if (!untrack(() => customName))
        name = suggestedName(
          first.agent,
          untrack(() => secretary),
        );
    }
  });

  function pickCandidate(candidate: Candidate) {
    chosen = candidate;
    if (!customName) name = suggestedName(candidate.agent, secretary);
  }

  function toggleSecretary(checked: boolean) {
    secretary = checked;
    if (!customName) name = suggestedName(chosen?.agent, checked);
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failed = '';
    if (!chosen || !group.info) return;
    const target = owned.find((home) => home === chosen!.home);
    if (!target?.ctl) {
      failed = 'lost';
      chosen = selectable[0];
      return;
    }
    const trimmed = name.trim();
    if (!trimmed) return;
    busy = true;
    const env = await target.me.sign(
      JSON.stringify({
        t: 'host.join',
        n: newId(),
        ts: Date.now(),
        to: chosen.device,
        g: group.g,
        home: group.info.home,
        agent: chosen.agent,
        name: trimmed,
        secretary,
      }),
    );
    const answer = await target.ctl.request({ t: 'cmd', env }).catch(lost);
    busy = false;
    if (answer.err) failed = answer.err.code;
    else dialog.close();
  }
</script>

<dialog
  bind:this={dialog}
  class="dlg"
  aria-labelledby={`${uid}-title`}
  onclose={() => (open = false)}
>
  <form onsubmit={submit}>
    <header>
      <h2 id={`${uid}-title`}>{t('member.join.title')}</h2>
      <p class="lead">{t('member.join.lead')}</p>
    </header>

    <fieldset class="field">
      <legend>{t('member.join.cli')}</legend>
      {#if candidates.length}
        <div class="choices">
          {#each candidates as candidate (candidate.key)}
            {@const disabled = candidate.account === null || candidate.inGroup}
            <label class="choice" class:disabled>
              <input
                type="radio"
                name="candidate"
                checked={chosen?.key === candidate.key}
                {disabled}
                onchange={() => pickCandidate(candidate)}
              />
              <Avatar id={candidate.key} name={cliTitle(candidate.agent)} cli={candidate.agent} />
              <span class="grow">
                <b>{cliTitle(candidate.agent)} · {candidate.hostName}</b>
                <span class="sub">
                  {#if candidate.inGroup}
                    {t('member.join.joined')}
                  {:else if candidate.account === null}
                    {t('member.join.unconfigured')}
                  {:else}
                    {accountName(candidate.account)}{#if !candidate.loginOk}{' '}<span class="warn">
                        · {t('member.join.nologin')}
                      </span>{/if}
                  {/if}
                </span>
              </span>
            </label>
          {/each}
        </div>
      {:else}
        <p class="hint" role={loading ? 'status' : undefined}>
          {t(loading ? 'member.join.loading' : 'member.join.none')}
        </p>
      {/if}
    </fieldset>

    <label class="field">
      <span>{t('member.join.name')}</span>
      <input
        class="input"
        bind:value={name}
        oninput={() => (customName = true)}
        required
        pattern=".*\S.*"
        maxlength="40"
      />
    </label>

    {#if canSecretary}
      <div class="sec-opt">
        <label class="check">
          <input
            type="checkbox"
            checked={secretary}
            onchange={(e) => toggleSecretary(e.currentTarget.checked)}
          />
          <span>{t('member.join.secretary')}</span>
        </label>
        {#if group.info?.secretary}
          <p class="hint">{t('member.join.secretary.replace')}</p>
        {/if}
      </div>
    {/if}

    {#if failed}<Failure code={failed} />{/if}
    <footer class="row">
      <span class="grow"></span>
      <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
        {t('member.join.cancel')}
      </button>
      <button class="btn btn-primary" disabled={busy || loading || (!chosen && !selectable.length)}>
        {t('member.join.submit')}
      </button>
    </footer>
  </form>
</dialog>

<style>
  dialog {
    overflow: auto;
  }
  .lead {
    margin-top: 6px;
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  legend {
    padding: 0;
    margin-bottom: 6px;
    font-weight: 500;
  }
  .choices {
    display: grid;
    gap: var(--sp-2);
  }
  .choice {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    padding: 10px 12px;
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    cursor: pointer;
    font-size: var(--fs-sm);
    color: var(--text-2);
  }
  .choice:has(:checked) {
    border-color: var(--sky);
    background: var(--sky-soft);
  }
  .choice.disabled {
    border-style: dashed;
    background: var(--surface-2);
    color: var(--text-3);
    cursor: not-allowed;
  }
  .choice input {
    margin: 0;
    accent-color: var(--sky);
  }
  .choice b {
    display: block;
    color: var(--text);
    font-size: 14px;
  }
  .choice.disabled b {
    color: var(--text-3);
  }
  .sub {
    display: block;
    font-size: var(--fs-xs);
  }
  .warn {
    color: var(--dawn-text);
  }
  .sec-opt {
    display: grid;
    gap: 4px;
  }
  .check {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-2);
    font-size: var(--fs-sm);
    cursor: pointer;
  }
  .check input {
    width: 16px;
    height: 16px;
    margin: 0;
    accent-color: var(--sky);
  }
</style>
