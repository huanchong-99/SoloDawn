<script lang="ts">
  import { untrack } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { addPerson, knownPeople, type Known } from '../../lib/members';
  import { announcement, prefs, t, type Key } from '../../lib/prefs.svelte';
  import Avatar from '../Avatar.svelte';
  import { clock, now } from '../../lib/when';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import SecretaryWarning from '../groupinfo/SecretaryWarning.svelte';

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
    <div class="link-head">
      {#if known.length}<h3 class="sec-title">{t('people.invite.link')}</h3>{/if}
      <p class="lead">{t('people.invite.lead')}</p>
    </div>
    {#if result}
      {@const remaining = Math.ceil((result.expires - now()) / 60_000)}
      <div class="pair-box">
        {#if !known.length}<span class="sec-title">{t('people.invite.link')}</span>{/if}
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
      </div>
    {/if}
    {#if failure}<Failure code={failure} />{/if}
    <footer class="row">
      <span class="grow"></span>
      {#if result}
        <button type="button" class="btn btn-primary" onclick={() => dialog.close()}>
          {t('people.invite.done')}
        </button>
      {:else}
        <!-- Once someone was added, closing undoes nothing: it is done. -->
        <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
          {t(Object.values(added).includes('') ? 'people.invite.done' : 'people.invite.cancel')}
        </button>
        <button class="btn btn-primary" disabled={busy || !group.home.ctl}>
          {t('people.invite.create')}
        </button>
      {/if}
    </footer>
  </form>
</dialog>

<style>
  dialog {
    overflow: auto;
  }
  .link-head {
    display: grid;
    gap: 4px;
  }
  .lead {
    color: var(--text-2);
    font-size: var(--fs-ui);
  }
  .part {
    display: grid;
    --av: 32px;
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
  .pair-box {
    display: grid;
    gap: var(--sp-2);
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
