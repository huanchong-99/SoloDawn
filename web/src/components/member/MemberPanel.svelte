<script lang="ts">
  import { homes, type Group } from '../../lib/groups.svelte';
  import {
    addDisabledReason,
    aiMembers,
    humanMembers,
    isAdmin,
    pathLabel,
    personTags,
    terminalDisabledReason,
    usageMeter,
    watchDisabledReason,
  } from '../../lib/members';
  import type { Points } from '../../lib/mounts.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { accountLabel } from '../../lib/onboard.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { href } from '../../lib/route.svelte';
  import { statusIn } from '../../lib/stop';
  import { endTakeover, hostHomeOf, terminalTask } from '../../lib/terminal.svelte';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';
  import AllowEditor from '../allow/AllowEditor.svelte';
  import Mount from '../Mount.svelte';
  import { offersTerminal } from '../driver/driver';
  import InvitationDialog from '../people/InvitationDialog.svelte';
  import PersonActions from '../people/PersonActions.svelte';
  import Profile from '../profile/Profile.svelte';
  import { suggestedTier } from '../profile/profile';
  import AskToWatch from '../watch/AskToWatch.svelte';
  import AddAI from './AddAI.svelte';
  import RemoveAI from './RemoveAI.svelte';

  /** The member column of a group's chat: AI member cards, people, and the add-AI dialog. */
  let { group, net }: { group: Group; net: Net } = $props();
  const uid = $props.id();

  let adding = $state(false);
  let joined = $state.raw<Points['member.joined']>();
  let inviting = $state(false);

  const ais = $derived(aiMembers(group));
  const humans = $derived(humanMembers(group));
  const admin = $derived(isAdmin(group));
  const addReason = $derived(addDisabledReason(group, homes));

  $effect(() => nodes.sync(net.homes));
</script>

<div class="members">
  {#if group.info?.kind !== 'team'}
    <div class="head">
      <div class="row">
        <h2 class="sec-title grow">{t('member.ai')} · {ais.length}</h2>
        <button
          class="btn btn-sm"
          onclick={() => (adding = true)}
          disabled={!group.info || Boolean(addReason)}
          aria-describedby={addReason ? `${uid}-add-why` : undefined}
        >
          <Icon name="plus" sm /><span>{t('member.add')}</span>
        </button>
      </div>
      {#if addReason}
        <p id={`${uid}-add-why`} class="hint add-why">{addReason}</p>
      {/if}
    </div>

    {#each ais as ai (ai.id)}
      {@const host = hostHomeOf(net.homes, ai.host)}
      {@const status = statusIn(group, ai, host)}
      {@const mine = ai.user === group.home.me.user}
      {@const owner = mine ? undefined : group.personName(ai.user)}
      {@const termWhy = mine ? terminalDisabledReason(net, ai) : undefined}
      {@const terminal = offersTerminal(host, ai.cli)}
      {@const watchWhy = watchDisabledReason(net, ai)}
      {@const meter = usageMeter(ai, prefs.lang)}
      <section class="card member">
        <header class="row">
          <span
            class="av-wrap"
            class:sec={ai.secretary}
            data-sec={t('member.sec.mark')}
            data-st={status.st}
            aria-hidden="true"
          >
            <Avatar id={ai.id} name={ai.name} cli={ai.cli} />
          </span>
          <div class="grow">
            <h3>
              {ai.name}{#if owner}{' '}<span class="owner">· {owner}</span>{/if}
            </h3>
            <p class="st" data-st={status.st}><i></i><span>{status.text}</span></p>
          </div>
        </header>
        <Profile {group} {ai} suggested={suggestedTier(host, ai)}>
          {#snippet children(profileRows, editProfile)}
            <AllowEditor {group} {ai} {humans}>
              {#snippet children(accessRows, editAccess)}
                <dl class="kv">
                  <dt>{t('member.computer')}</dt>
                  <dd>{ai.host_name} <span class="muted">· {pathLabel(ai)}</span></dd>
                  <dt>{t('member.account')}</dt>
                  <dd>
                    {accountLabel(ai.account)}{#if ai.account === 'personal'}{' '}<span
                        class="muted"
                      >
                        · {t('member.account.personal.note')}
                      </span>{/if}
                  </dd>
                  <Mount at="member.ai" props={{ group, ai }} />
                  {@render profileRows()}
                  {@render accessRows()}
                </dl>
                {#if meter?.fill !== undefined}
                  <div
                    class="meter"
                    role="meter"
                    aria-label={meter.label}
                    aria-valuenow={meter.fill}
                    aria-valuetext={meter.note ? `${meter.value} · ${meter.note}` : meter.value}
                    data-tone={meter.tone}
                    style={`--v: ${meter.fill}%`}
                  >
                    <span>{meter.label}</span><b>{meter.value}</b><i></i>{#if meter.note}<small>
                        {meter.note}
                      </small>{/if}
                  </div>
                {:else if meter}
                  <p class="meter"><span>{meter.label}</span><b>{meter.value}</b></p>
                {/if}
                {#if mine}
                  <footer class="card-foot">
                    {#if ai.state === 'takeover'}
                      <button
                        type="button"
                        class="btn btn-sm btn-dawn"
                        onclick={() =>
                          endTakeover(hostHomeOf(net.homes, ai.host), ai.host, group.g, ai.id)}
                      >
                        {t('member.handback')}
                      </button>
                    {:else if terminal && !termWhy}
                      <a
                        class="btn btn-sm"
                        href={href('terminal', {
                          g: group.g,
                          member: ai.id,
                          task: terminalTask(group, ai),
                        })}
                      >
                        <Icon name="terminal" sm /><span>{t('member.terminal')}</span>
                      </a>
                    {:else if terminal}
                      <button
                        type="button"
                        class="btn btn-sm"
                        disabled
                        aria-describedby={`${uid}-term-${ai.id}`}
                      >
                        <Icon name="terminal" sm /><span>{t('member.terminal')}</span>
                      </button>
                    {/if}
                    {@render editProfile()}
                    {@render editAccess()}
                    {#if ai.state === 'takeover'}
                      <span class="hint">{t('member.handback.queued')}</span>
                    {:else if terminal && termWhy}
                      <span id={`${uid}-term-${ai.id}`} class="hint">{termWhy}</span>
                    {/if}
                    <RemoveAI {group} {ai} />
                  </footer>
                {:else if ai.state === 'takeover' || admin}
                  <footer class="card-foot">
                    {#if ai.state === 'takeover'}
                      <AskToWatch {group} {ai} />
                      {#if watchWhy}
                        <button
                          type="button"
                          class="btn btn-sm"
                          disabled
                          aria-describedby={`${uid}-watch-${ai.id}`}
                        >
                          {t('watch.open')}
                        </button>
                        <span id={`${uid}-watch-${ai.id}`} class="hint">{watchWhy}</span>
                      {:else}
                        <a
                          class="btn btn-sm"
                          href={href('terminal', { g: group.g, member: ai.id })}
                        >
                          {t('watch.open')}
                        </a>
                      {/if}
                    {/if}
                    {#if admin}<RemoveAI {group} {ai} />{/if}
                  </footer>
                {/if}
              {/snippet}
            </AllowEditor>
          {/snippet}
        </Profile>
      </section>
    {:else}
      <p class="hint">{t('member.ai.empty')}</p>
    {/each}
  {/if}

  <div class="row people-head">
    <h2 class="sec-title grow">{t('member.people')} · {humans.length}</h2>
    {#if admin}
      <button
        type="button"
        class="btn btn-sm"
        disabled={!group.home.ctl}
        onclick={() => (inviting = true)}
      >
        <Icon name="plus" sm /><span>{t('people.invite.action')}</span>
      </button>
    {/if}
  </div>
  <ul class="people">
    {#each humans as person (person.id)}
      {@const tags = personTags(group, person)}
      <li class="person">
        <Avatar id={person.user} name={person.name} />
        <span class="person-name grow">
          {person.name}{#if tags}{' '}<span class="muted">· {tags}</span>{/if}
        </span>
        <PersonActions {group} {person} />
      </li>
    {/each}
  </ul>
</div>

{#if adding}
  <AddAI {group} bind:open={adding} onjoined={(event) => (joined = event)} />
{/if}
{#if joined}
  {#key joined}
    <Mount at="member.joined" props={joined} />
  {/key}
{/if}
{#if inviting}
  <InvitationDialog {group} bind:open={inviting} />
{/if}

<style>
  .members {
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
    padding: var(--sp-4);
  }
  .head {
    display: grid;
    gap: 4px;
  }
  .add-why {
    text-align: right;
  }
  .member header {
    flex-wrap: nowrap;
    align-items: flex-start;
  }
  .member p.st {
    margin-top: 2px;
  }
  .owner {
    color: var(--text-3);
    font-weight: 400;
  }
  .av-wrap {
    position: relative;
    display: inline-grid;
    flex: none;
  }
  .av-wrap :global(.av) {
    width: 40px;
    height: 40px;
    border-radius: 11px;
    font-size: 15px;
  }
  .av-wrap.sec::before {
    content: attr(data-sec);
    position: absolute;
    top: -5px;
    right: -5px;
    z-index: 1;
    display: grid;
    place-items: center;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    border: 2px solid var(--surface);
    background: var(--text);
    color: var(--bg);
    font-size: 10px;
    font-weight: 700;
    line-height: 1;
  }
  .av-wrap[data-st]::after {
    content: '';
    position: absolute;
    right: -3px;
    bottom: -3px;
    width: 11px;
    height: 11px;
    border-radius: 50%;
    border: 2px solid var(--surface);
    background: var(--text-3);
  }
  .av-wrap[data-st='working']::after {
    background: var(--sky);
  }
  .av-wrap[data-st='needyou']::after,
  .av-wrap[data-st='limited']::after {
    background: var(--dawn);
  }
  .av-wrap[data-st='waiting']::after {
    background: var(--surface);
    box-shadow: inset 0 0 0 2px var(--text-3);
  }
  .av-wrap[data-st='sleeping']::after {
    background: var(--surface-3);
  }
  .av-wrap[data-st='takeover']::after {
    border-radius: 3px;
    background: var(--text);
  }
  .kv {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 6px var(--sp-4);
    font-size: var(--fs-sm);
  }
  .kv dt {
    color: var(--text-3);
  }
  .kv dd {
    min-width: 0;
  }
  .people-head {
    margin-top: var(--sp-2);
  }
  .people {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
  }
  .person {
    display: grid;
    grid-column: 1 / -1;
    grid-template-columns: subgrid;
    align-items: center;
    gap: var(--sp-2);
    padding: 6px 0;
  }
  .person-name {
    overflow-wrap: anywhere;
  }
</style>
