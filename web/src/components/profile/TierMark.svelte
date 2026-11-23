<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import Bars from './Bars.svelte';
  import type { Tier } from './profile';

  /** The tier of an AI member as a small badge. */
  let { tier }: { tier: string } = $props();
</script>

<span class="badge tier" data-tier={tier} data-tone={tier === 'strong' ? 'sky' : undefined}>
  <Bars {tier} />{t(`profile.tier.${tier as Tier}`)}
</span>

<style>
  .tier[data-tier='light'] {
    color: var(--text-3);
  }
</style>
