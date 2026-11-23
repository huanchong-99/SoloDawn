<script lang="ts">
  import type { Points } from '../../lib/mounts.svelte';
  import { accountLabel, accounts } from '../../lib/onboard.svelte';
  import { t } from '../../lib/prefs.svelte';

  let { candidate, draft = $bindable() }: Points['member.add'] = $props();
</script>

{#if candidate?.agent === 'external'}
  <label class="field">
    <span>{t('member.account')}</span>
    <select class="input" bind:value={draft.account} required>
      <option value="" disabled selected>{t('external.account.choose')}</option>
      {#each accounts as account (account)}
        <option value={account}>{accountLabel(account)}</option>
      {/each}
    </select>
  </label>
  <label class="field">
    <span>{t('external.tool.label')}</span>
    <input class="input" bind:value={draft.tool} pattern={'[^\\p{Cc}]{0,32}'} />
    <small class="hint">{t('external.tool.hint')}</small>
  </label>
{/if}
