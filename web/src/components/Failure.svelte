<script lang="ts">
  import { t } from '../lib/prefs.svelte';

  /** The code of an error answer (contracts/datachannel.md section 9), or `lost`. The client owns
   *  the wording; a code without wording of its own is shown as it is. */
  let { code }: { code: string } = $props();

  const worded = ['lost', 'forbidden', 'bad_request', 'stale'] as const;
  const known = $derived(worded.find((each) => each === code));
</script>

<p class="fail" role="alert">
  {#if known}
    {t(`chat.error.${known}`)}
  {:else}
    {t('chat.error.other')} <code>{code}</code>
  {/if}
</p>

<style>
  .fail {
    color: var(--danger);
    font-size: var(--fs-sm);
  }
</style>
