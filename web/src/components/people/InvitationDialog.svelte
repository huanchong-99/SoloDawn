<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { announcement, prefs, t } from '../../lib/prefs.svelte';
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
      <p class="lead">{t('people.invite.lead')}</p>
    </header>
    {#if result}
      {@const remaining = Math.ceil((result.expires - now()) / 60_000)}
      <div class="pair-box">
        <span class="sec-title">{t('people.invite.link')}</span>
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
        <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
          {t('people.invite.cancel')}
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
  .lead {
    margin-top: 4px;
    color: var(--text-2);
    font-size: var(--fs-ui);
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
