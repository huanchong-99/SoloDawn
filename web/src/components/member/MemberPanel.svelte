<script lang="ts">
  import { homes, type Group } from '../../lib/groups.svelte';
  import {
    addDisabledReason,
    aiMembers,
    humanMembers,
    pathLabel,
    personTags,
    statusOf,
    usageMeter,
  } from '../../lib/members';
  import { accountLabel } from '../../lib/onboard.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';
  import AddAI from './AddAI.svelte';

  /** The member column of a group's chat: AI member cards, people, and the add-AI dialog. */
  let { group }: { group: Group } = $props();
  const uid = $props.id();

  let adding = $state(false);

  const ais = $derived(aiMembers(group));
  const humans = $derived(humanMembers(group));
  const addReason = $derived(addDisabledReason(group, homes));
</script>

<div class="members">
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
    {@const status = statusOf(group, ai)}
    {@const owner = ai.user === group.home.me.user ? undefined : group.personName(ai.user)}
    {@const meter = usageMeter(ai.usage, prefs.lang)}
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
      <dl class="kv">
        <dt>{t('member.computer')}</dt>
        <dd>{ai.host_name} <span class="muted">· {pathLabel(ai)}</span></dd>
        <dt>{t('member.account')}</dt>
        <dd>
          {accountLabel(ai.account)}{#if ai.account === 'personal'}{' '}<span class="muted">
              · {t('member.account.personal.note')}
            </span>{/if}
        </dd>
      </dl>
      {#if meter}
        <div class="meter" data-tone={meter.tone} style={`--v: ${meter.fill}%`}>
          <span>{meter.label}</span><b>{meter.value}</b><i></i>{#if meter.note}<small>
              {meter.note}
            </small>{/if}
        </div>
      {/if}
    </section>
  {:else}
    <p class="hint">{t('member.ai.empty')}</p>
  {/each}

  <h2 class="sec-title people-head">{t('member.people')} · {humans.length}</h2>
  <ul class="people">
    {#each humans as person (person.id)}
      {@const tags = personTags(group, person)}
      <li class="row person">
        <Avatar id={person.user} name={person.name} />
        <span class="grow cut">
          {person.name}{#if tags}{' '}<span class="muted">· {tags}</span>{/if}
        </span>
        <span class="st" data-st={person.online ? 'ok' : 'offline'}>
          <i></i>{t(person.online ? 'member.online' : 'member.offline')}
        </span>
      </li>
    {/each}
  </ul>
</div>

{#if adding}
  <AddAI {group} bind:open={adding} />
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
    display: flex;
    flex-direction: column;
  }
  .person {
    flex-wrap: nowrap;
    padding: 6px 0;
  }
</style>
