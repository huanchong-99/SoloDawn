<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import { secretaryWarning } from './trust';

  let { account, owner, people }: { account: string; owner: string; people: readonly string[] } =
    $props();
</script>

{#if secretaryWarning(account, owner, people)}
  <p class="note" data-tone="dawn">{t('groupinfo.secretary.warning')}</p>
{/if}
