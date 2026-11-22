<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import { secretaryWarning } from './trust';

  let {
    account,
    owner,
    people,
    inviting = false,
  }: { account: string; owner: string; people: readonly string[]; inviting?: boolean } = $props();
</script>

{#if secretaryWarning(account, owner, people, inviting)}
  <p class="note" data-tone="dawn">{t('groupinfo.secretary.warning')}</p>
{/if}
