<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow } from '../../lib/proto';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import { requestWatch } from './watch.svelte';

  let { group, ai }: { group: Group; ai: AIMemberRow } = $props();
  let busy = $state(false);
  let sent = $state(false);
  let failed = $state('');

  async function ask() {
    busy = true;
    failed = '';
    const answer = await requestWatch(group, ai);
    busy = false;
    if (answer.err) failed = answer.err.code;
    else sent = true;
  }
</script>

<button
  type="button"
  class="btn btn-sm"
  disabled={busy || sent || !group.home.ctl || ai.state !== 'takeover'}
  onclick={ask}
>
  <Icon name="eye" sm />{t(sent ? 'watch.requested' : 'watch.ask')}
</button>
{#if failed}<Failure code={failed} />{/if}
