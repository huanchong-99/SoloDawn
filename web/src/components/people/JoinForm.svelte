<script lang="ts">
  import { onDestroy } from 'svelte';
  import { readCode, type Invitation, type JoinError } from '../../lib/join';
  import type { Net } from '../../lib/net.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { href } from '../../lib/route.svelte';
  import Icon from '../Icon.svelte';

  // The refusals a person can act on, in words (contracts/step-2/people.md 2.2); any other keeps
  // its code. A code has its own words for one that ended and for a home that never answered.
  const worded = ['forbidden', 'conflict', 'stale', 'version', 'timeout', 'lost'] as const;
  const isWorded = (code: string): code is (typeof worded)[number] =>
    (worded as readonly string[]).includes(code);

  /** With a link, `invitation` is what it holds; without one, the person types a code
   *  (contracts/step-2/people.md 4.1). */
  let { invitation, net }: { invitation: Invitation | undefined; net: Net } = $props();
  let pending = $state.raw<AbortController>();
  let error = $state.raw<JoinError>();
  /** The group joined, while some of the person's computers were not told of it. */
  let joined = $state('');
  let name = $state('');
  let code = $state('');
  let notCode = $state(false);

  onDestroy(() => pending?.abort());

  /** The glyph of a result, so that each is told by its shape: no answer, ended, other. */
  const glyph = (refusal: string) =>
    refusal === 'timeout' ? 'clock' : refusal === 'forbidden' ? 'x' : 'alert';

  function said({ step, code: refusal }: JoinError) {
    if (!invitation && (refusal === 'forbidden' || refusal === 'timeout')) {
      return t(`people.join.code.${refusal}`);
    }
    if (isWorded(refusal)) return t(`people.join.fail.${refusal}`);
    return t('people.join.failure').replace('{step}', step).replace('{code}', refusal);
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    error = undefined;
    // A text that is no code is refused here, and nothing is sent.
    const target = invitation ?? readCode(code);
    notCode = !target;
    if (!target) return;
    const controller = new AbortController();
    pending = controller;
    try {
      const result = await net.join!(target, name.trim(), controller.signal);
      if (controller.signal.aborted) return;
      if (result.notifications.length) joined = result.g;
      else location.hash = href('chat', { g: result.g });
    } catch (cause) {
      if (!controller.signal.aborted) error = cause as JoinError;
    } finally {
      pending = undefined;
    }
  }
</script>

{#if joined}
  <section class="card">
    <p role="status">{t('people.join.notifications')}</p>
    <div class="row">
      <a class="btn btn-primary" href={href('chat', { g: joined })}>
        {t('people.join.open')}
      </a>
    </div>
  </section>
{:else}
  <form class="card" onsubmit={submit}>
    {#if !invitation}
      <label class="field">
        <span>{t('people.join.code')}</span>
        <input
          class="input mono code"
          required
          bind:value={code}
          oninput={() => (notCode = false)}
          placeholder="XXXX-XXXX-XXXX-XXXX"
          autocomplete="off"
          autocapitalize="characters"
          spellcheck="false"
          aria-invalid={notCode}
          disabled={pending !== undefined}
        />
      </label>
    {/if}
    <label class="field">
      <span>{t('people.join.name')}</span>
      <input class="input" required bind:value={name} disabled={pending !== undefined} />
    </label>
    <!-- One place for what came of it: looking for the home, then each result with its glyph. -->
    {#if pending && !invitation}
      <div class="note seek" role="status">
        <span class="st" data-st="linking"><i></i></span>
        <span>{t('people.join.code.waiting')}</span>
      </div>
    {:else if notCode}
      <div class="note" role="alert">
        <Icon name="alert" sm /><span>{t('people.join.code.invalid')}</span>
      </div>
    {:else if error?.code === 'unpaired'}
      <p role="alert">{t('people.join.onboarding')}</p>
      <div class="row">
        <a class="btn" href={href('onboarding')}>{t('page.onboarding')}</a>
      </div>
    {:else if error}
      <div class="note" role="alert">
        <Icon name={glyph(error.code)} sm /><span>{said(error)}</span>
      </div>
    {/if}
    <footer class="row">
      <span class="grow"></span>
      <a class="btn btn-ghost" href={href('groups')}>{t('people.invite.cancel')}</a>
      <button
        class="btn btn-primary"
        disabled={pending !== undefined || !name.trim() || !(invitation || code.trim())}
      >
        {t(pending ? 'people.join.busy' : 'people.join.submit')}
      </button>
    </footer>
  </form>
{/if}

<style>
  .code {
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .code::placeholder {
    letter-spacing: 0.08em;
  }
  /* Looking for the home: the accent breathes, as for a computer being linked. */
  .note {
    animation:
      rise var(--dur-2) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .seek .st {
    height: 1.5em;
  }
</style>
