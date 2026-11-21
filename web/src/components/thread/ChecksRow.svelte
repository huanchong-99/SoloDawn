<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { clock } from '../../lib/when';
  import Icon from '../Icon.svelte';

  /** Result of local, cloud or merge checks written by the home. */
  let { msg }: CardProps<'checks'> = $props();
  const body = $derived(msg.body);
  const time = $derived(clock(msg.ts, prefs.lang));
  const iso = $derived(new Date(msg.ts).toISOString());

  const keys = {
    local: ['thread.checks.local.fail', 'thread.checks.local.ok'],
    cloud: ['thread.checks.cloud.fail', 'thread.checks.cloud.ok'],
    merge: ['thread.checks.merge.fail', 'thread.checks.merge.ok'],
  } as const;
  const stage = $derived(body.stage === 'local' || body.stage === 'cloud' ? body.stage : 'merge');
  const label = $derived(t(keys[stage][body.ok ? 1 : 0]));
</script>

<div class="sys-row">
  <span class="glyph" data-tone={body.ok ? 'ok' : 'danger'}>
    <Icon name={body.ok ? 'check' : 'x'} sm />
  </span>
  <p>
    <span class="outcome" data-tone={body.ok ? undefined : 'danger'}>{label}</span>
    · <code>{body.commit.slice(0, 7)}</code>
    {#each body.items as item, i (i)}
      · <code>{item.name}</code>{#if item.detail}{` (${item.detail})`}{/if}
    {/each}
    · <time datetime={iso}>{time}</time>
  </p>
</div>

<style>
  .sys-row {
    display: grid;
    grid-template-columns: 22px minmax(0, 1fr);
    gap: 2px var(--sp-2);
    align-items: start;
    color: var(--text-3);
    font-size: var(--fs-sm);
  }
  .glyph {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    color: var(--tone-text, var(--text-3));
  }
  .outcome[data-tone='danger'] {
    color: var(--danger);
    font-weight: 500;
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
    color: var(--text-2);
  }
  time {
    font-size: var(--fs-xs);
  }
</style>
