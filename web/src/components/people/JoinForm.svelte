<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { Invitation, JoinError } from '../../lib/join';
  import type { Net } from '../../lib/net.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { href } from '../../lib/route.svelte';

  // The refusals a person can act on, in words (contracts/step-2/people.md 2.2); any other keeps
  // its code.
  const worded = ['forbidden', 'conflict', 'stale', 'version', 'timeout', 'lost'] as const;
  const isWorded = (code: string): code is (typeof worded)[number] =>
    (worded as readonly string[]).includes(code);

  let { invitation, net }: { invitation: Invitation | undefined; net: Net } = $props();
  let pending = $state.raw<AbortController>();
  let error = $state.raw<JoinError>();
  let notifiedPartially = $state(false);
  let name = $state('');

  onDestroy(() => pending?.abort());

  async function submit(event: SubmitEvent, invitation: Invitation) {
    event.preventDefault();
    const controller = new AbortController();
    pending = controller;
    error = undefined;
    try {
      const result = await net.join!(invitation, name.trim(), controller.signal);
      if (controller.signal.aborted) return;
      if (result.notifications.length) notifiedPartially = true;
      else location.hash = href('chat', { g: invitation.g });
    } catch (cause) {
      if (!controller.signal.aborted) error = cause as JoinError;
    } finally {
      pending = undefined;
    }
  }
</script>

{#if invitation && notifiedPartially}
  <section class="card">
    <p role="status">{t('people.join.notifications')}</p>
    <div class="row">
      <a class="btn btn-primary" href={href('chat', { g: invitation.g })}>
        {t('people.join.open')}
      </a>
    </div>
  </section>
{:else if invitation}
  <form class="card" onsubmit={(event) => submit(event, invitation)}>
    <label class="field">
      <span>{t('people.join.name')}</span>
      <input class="input" required bind:value={name} disabled={pending !== undefined} />
    </label>
    {#if error?.code === 'unpaired'}
      <p role="alert">{t('people.join.onboarding')}</p>
      <div class="row">
        <a class="btn" href={href('onboarding')}>{t('page.onboarding')}</a>
      </div>
    {:else if error && isWorded(error.code)}
      <p role="alert">{t(`people.join.fail.${error.code}`)}</p>
    {:else if error}
      <p role="alert">
        {t('people.join.failure').replace('{step}', error.step).replace('{code}', error.code)}
      </p>
    {/if}
    <footer class="row">
      <span class="grow"></span>
      <a class="btn btn-ghost" href={href('groups')}>{t('people.invite.cancel')}</a>
      <button class="btn btn-primary" disabled={pending !== undefined || !name.trim()}>
        {t(pending ? 'people.join.busy' : 'people.join.submit')}
      </button>
    </footer>
  </form>
{:else}
  <section class="card">
    <p role="alert">{t('people.join.invalid')}</p>
  </section>
{/if}
