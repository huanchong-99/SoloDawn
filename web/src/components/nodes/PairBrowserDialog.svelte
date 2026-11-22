<script lang="ts">
  import { untrack } from 'svelte';
  import type { Home } from '../../lib/groups.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { nodes, type PairLink } from '../../lib/nodes.svelte';
  import { announcement, prefs, t } from '../../lib/prefs.svelte';
  import { clock, now } from '../../lib/when';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** Dialog that requests a one-time `#pair=<secret>` link from an online owner node. */
  let { open = $bindable(false), net }: { open?: boolean; net: Net } = $props();
  const uid = $props.id();

  const pairable = $derived(
    nodes.list(net).filter((comp) => comp.online && comp.info !== undefined),
  );

  let dialog: HTMLDialogElement;
  let chosen = $state.raw<Home>();
  let busy = $state(false);
  let failed = $state('');
  let result = $state.raw<PairLink>();
  let copied = $state(false);

  const listed = () => pairable.find((comp) => comp.home === chosen);
  const pick = () => (chosen = (listed() ?? pairable[0])?.home);

  $effect(() => {
    if (!open) return;
    untrack(() => {
      result = undefined;
      failed = '';
      copied = false;
      pick();
    });
    dialog.showModal();
  });

  let none = true;
  $effect(() => {
    const some = pairable.length > 0;
    if (some && none) untrack(pick);
    none = !some;
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failed = '';
    copied = false;
    const target = listed();
    if (!target) {
      failed = 'lost';
      chosen = pairable[0]?.home;
      return;
    }
    busy = true;
    const answer = await nodes.pairBrowser(target.home, target.info!.device);
    busy = false;
    if ('err' in answer) {
      failed = answer.err.code;
      return;
    }
    result = answer;
  }

  async function copyLink() {
    if (!result) return;
    await navigator.clipboard?.writeText(result.link).catch(() => undefined);
    copied = true;
    announcement.text = t('nodes.pair.copied');
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
      <h2 id={`${uid}-title`}>{t('nodes.pair.title')}</h2>
      <p class="lead">{t('nodes.pair.lead')}</p>
    </header>

    <label class="field">
      <span id={`${uid}-computer`}>{t('nodes.pair.computer')}</span>
      <select
        class="input"
        aria-labelledby={`${uid}-computer`}
        bind:value={chosen}
        onchange={() => (result = undefined)}
      >
        {#each pairable as comp (comp.home)}
          <option value={comp.home}>{comp.info!.name}</option>
        {/each}
      </select>
    </label>

    {#if result}
      {@const remaining = Math.ceil((result.expires - now()) / 60_000)}
      <div class="pair-box">
        <span class="sec-title">{t('nodes.pair.link')}</span>
        <pre class="cmd mono">{result.link}</pre>
        <div class="row">
          <button type="button" class="btn btn-dawn btn-sm" onclick={copyLink}>
            {#if copied}
              <Icon name="check" sm /><span>{t('nodes.pair.copied')}</span>
            {:else}
              <Icon name="copy" sm /><span>{t('nodes.pair.copy')}</span>
            {/if}
          </button>
          {#if remaining > 0}
            <span class="hint">
              <Icon name="clock" sm />
              {t('nodes.pair.expires')
                .replace('{n}', String(remaining))
                .replace('{time}', clock(result.expires, prefs.lang))}
            </span>
          {:else}
            <span class="hint expired">{t('nodes.pair.expired')}</span>
          {/if}
        </div>
      </div>
    {/if}

    {#if failed}<Failure code={failed} />{/if}

    <footer class="row">
      <span class="grow"></span>
      {#if result}
        <button type="button" class="btn btn-primary" onclick={() => dialog.close()}>
          {t('nodes.pair.done')}
        </button>
      {:else}
        <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
          {t('nodes.pair.cancel')}
        </button>
        <button class="btn btn-primary" disabled={busy || !pairable.length}>
          {t('nodes.pair.create')}
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
    margin-top: 6px;
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .pair-box {
    display: grid;
    gap: var(--sp-2);
  }
  .cmd {
    padding: 10px 12px;
    border: 1px solid var(--line);
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
