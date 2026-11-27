<script lang="ts">
  import { untrack } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import {
    addPerson,
    knownPeople,
    lifetimes,
    sendCode,
    type Known,
    type OpenCode,
  } from '../../lib/members';
  import { announcement, prefs, t, type Key } from '../../lib/prefs.svelte';
  import Avatar from '../Avatar.svelte';
  import { clock, now } from '../../lib/when';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import SecretaryWarning from '../groupinfo/SecretaryWarning.svelte';
  import CodeCard from './CodeCard.svelte';

  let { open = $bindable(), group }: { open: boolean; group: Group } = $props();
  const uid = $props.id();
  let dialog: HTMLDialogElement;
  let busy = $state(false);
  let failure = $state('');
  let result = $state.raw<{ link: string; expires: number }>();
  let copied = $state(false);
  const secretary = $derived(
    group.info!.secretary ? group.member(group.info!.secretary) : undefined,
  );
  const people = $derived(
    group.members.filter((member) => member.kind === 'human').map((member) => member.user),
  );

  $effect(() => dialog.showModal());

  // The group's invitation code (contracts/step-2/people.md 2.6): read when the dialog opens and
  // each time the home is reached again, since a command sent again is answered without it.
  let code = $state.raw<OpenCode>();
  let hours = $state<number>(24);
  let replacing = $state(false);
  let codeBusy = $state(false);
  let codeFailure = $state('');
  $effect(() => {
    if (group.home.ctl) untrack(() => void sendOp('show'));
  });

  /** A refusal of `show` is not said: the section then offers a new code, whose refusal is. */
  async function sendOp(op: 'new' | 'off' | 'show') {
    codeBusy = true;
    codeFailure = '';
    const answer = await sendCode(group.home, group.g, op, op === 'new' ? hours : undefined);
    codeBusy = false;
    if (answer.err) {
      if (op !== 'show') codeFailure = answer.err.code;
      return;
    }
    code = answer.open;
    replacing = false;
  }

  // The people of the person's other groups on this home join without a link
  // (contracts/step-2/people.md section 2.5); what each answer was, by user.
  // Read once when it opens: someone added here stays in the list with what became of them.
  const known = untrack(() => knownPeople(group.home, group));
  /** Each person's outcome: `busy` while asked, `''` once added, else the refusal's code. */
  let added = $state<Record<string, string>>({});

  async function add(person: Known) {
    added[person.user] = 'busy';
    added[person.user] = (await addPerson(group.home, group.g, person))?.code ?? '';
  }

  function outcome(code: string) {
    if (!code) return t('people.add.done');
    return t(`people.add.error.${code}` as Key) ?? `${t('chat.error.other')} ${code}`;
  }

  async function invite(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = '';
    const answer = await group.home.command({ t: 'invite', g: group.g });
    busy = false;
    if (answer.err) failure = answer.err.code;
    else result = { link: answer.link!, expires: answer.expires! };
  }

  async function copy(link: string) {
    try {
      await navigator.clipboard.writeText(link);
    } catch {
      announcement.text = t('people.invite.copyFailed');
      return;
    }
    copied = true;
    announcement.text = t('people.invite.copied');
  }
</script>

<dialog
  bind:this={dialog}
  class="dlg"
  aria-labelledby={`${uid}-title`}
  onclose={() => (open = false)}
>
  <form onsubmit={invite}>
    <header>
      <h2 id={`${uid}-title`}>{t('people.invite.title')}</h2>
      {#if secretary?.kind === 'ai'}
        <SecretaryWarning account={secretary.account} owner={secretary.user} {people} inviting />
      {/if}
    </header>
    <section class="tool" aria-labelledby={`${uid}-code`}>
      <h3 class="sec-title" id={`${uid}-code`}>{t('people.code.title')}</h3>
      <p class="lead">{t('people.code.lead')}</p>
      {#if code && !replacing}
        <CodeCard {code} group={group.name}>
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            disabled={codeBusy || !group.home.ctl}
            onclick={() => (replacing = true)}
          >
            {t('people.code.new')}
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            disabled={codeBusy || !group.home.ctl}
            onclick={() => sendOp('off')}
          >
            {t('people.code.off')}
          </button>
        </CodeCard>
      {:else}
        {#if replacing}
          <p class="note" role="status"><Icon name="info" sm />{t('people.code.replace')}</p>
        {/if}
        <div class="row make">
          <fieldset class="seg" style:--at={(lifetimes as readonly number[]).indexOf(hours)}>
            <legend class="sr">{t('people.code.hours')}</legend>
            {#each lifetimes as each (each)}
              <label>
                <input type="radio" name={`${uid}-hours`} value={each} bind:group={hours} />
                <span>{t(`people.code.hours.${each}`)}</span>
              </label>
            {/each}
          </fieldset>
          <button
            type="button"
            class="btn btn-primary"
            disabled={codeBusy || !group.home.ctl}
            onclick={() => sendOp('new')}
          >
            {t(replacing ? 'people.code.new' : 'people.code.create')}
          </button>
          {#if replacing}
            <button type="button" class="btn btn-ghost" onclick={() => (replacing = false)}>
              {t('people.invite.cancel')}
            </button>
          {/if}
        </div>
      {/if}
      {#if codeFailure}<Failure code={codeFailure} />{/if}
    </section>
    {#if known.length}
      <section class="part" aria-labelledby={`${uid}-known`}>
        <h3 class="sec-title" id={`${uid}-known`}>{t('people.add.known')} · {known.length}</h3>
        <p class="hint">{t('people.add.hint')}</p>
        {#each known as person (person.user)}
          {@const code = added[person.user]}
          <div class="pick-row">
            <Avatar id={person.user} name={person.name} />
            <span class="grow">
              <b>{person.name}</b>
              {#if code === ''}
                <span class="badge" data-tone="ok" role="status">
                  <Icon name="check" sm />{outcome(code)}
                </span>
              {:else if code !== undefined && code !== 'busy'}
                <small class="bad" role="status">{outcome(code)}</small>
              {/if}
            </span>
            {#if code !== ''}
              <button
                type="button"
                class="btn btn-sm"
                disabled={code === 'busy' || !group.home.ctl}
                aria-label={t('people.add.of').replace('{who}', () => person.name)}
                onclick={() => add(person)}
              >
                {t('people.add.action')}
              </button>
            {/if}
          </div>
        {/each}
      </section>
    {/if}
    <section class="tool" aria-labelledby={`${uid}-link`}>
      <h3 class="sec-title" id={`${uid}-link`}>{t('people.invite.link')}</h3>
      <p class="lead">{t('people.invite.lead')}</p>
      {#if result}
        {@const remaining = Math.ceil((result.expires - now()) / 60_000)}
        <pre class="cmd mono">{result.link}</pre>
        <div class="row">
          {#if remaining > 0}
            <button type="button" class="btn btn-dawn btn-sm" onclick={() => copy(result!.link)}>
              <Icon name={copied ? 'check' : 'copy'} sm />
              <span>{t(copied ? 'people.invite.copied' : 'people.invite.copy')}</span>
            </button>
            <span class="hint">
              <Icon name="clock" sm />
              {t('people.invite.expires')
                .replace('{n}', String(remaining))
                .replace('{time}', clock(result.expires, prefs.lang))}
            </span>
          {:else}
            <span class="hint expired">{t('people.invite.expired')}</span>
          {/if}
        </div>
      {:else}
        <div class="row">
          <button class="btn btn-sm" disabled={busy || !group.home.ctl}>
            {t('people.invite.create')}
          </button>
        </div>
      {/if}
      {#if failure}<Failure code={failure} />{/if}
    </section>
    <footer class="row">
      <span class="grow"></span>
      <!-- Once a link or a code was made or someone added, closing undoes nothing: it is done. -->
      {#if result || code || Object.values(added).includes('')}
        <button type="button" class="btn" onclick={() => dialog.close()}>
          {t('people.invite.done')}
        </button>
      {:else}
        <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
          {t('people.invite.cancel')}
        </button>
      {/if}
    </footer>
  </form>
</dialog>

<style>
  dialog {
    overflow: auto;
  }
  .lead {
    color: var(--text-2);
    font-size: var(--fs-ui);
  }
  .part {
    display: grid;
    --av: 32px;
  }
  /* The code, the people and the link: one hairline between each and the next. */
  .tool {
    display: grid;
    gap: var(--sp-2);
  }
  :is(.tool, .part) + :is(.tool, .part) {
    padding-top: var(--sp-4);
    border-top: 1px solid var(--line);
  }
  .make {
    gap: var(--sp-3);
  }
  .part .hint {
    margin-bottom: var(--sp-1);
  }
  .badge {
    margin-top: 2px;
  }
  .bad {
    color: var(--danger);
  }
  .cmd {
    padding: 10px 12px;
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    line-height: 1.5;
  }
  .hint {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .expired {
    color: var(--danger);
  }
</style>
