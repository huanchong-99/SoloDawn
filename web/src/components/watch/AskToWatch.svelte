<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow } from '../../lib/proto';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import { requestWatch, watchRequest } from './watch.svelte';

  /** `again`: offered also after the consent, to someone whom the computer refuses. */
  let { group, ai, again = false }: { group: Group; ai: AIMemberRow; again?: boolean } = $props();
  let busy = $state(false);
  let failed = $state('');
  const status = $derived(watchRequest(group, ai)?.state.status);
  const asked = $derived(status === 'pending' || (status === 'confirmed' && !again));

  async function ask() {
    busy = true;
    failed = '';
    const answer = await requestWatch(group, ai);
    busy = false;
    if (answer.err) failed = answer.err.code;
  }
</script>

<button
  type="button"
  class="btn btn-sm"
  disabled={busy || asked || !group.home.ctl || ai.state !== 'takeover'}
  onclick={ask}
>
  <Icon name="eye" sm />{t(asked ? 'watch.requested' : 'watch.ask')}
</button>
{#if failed}<Failure code={failed} />{/if}
