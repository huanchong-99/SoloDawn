<script lang="ts">
  import { find } from '../../lib/groups.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { indicator } from '../../lib/nodes.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { href, route } from '../../lib/route.svelte';

  let { net }: { net: Net } = $props();

  const group = $derived(route.page === 'chat' ? find(route.params.g) : undefined);
  const conn = $derived(indicator(net, group));
</script>

{#if conn}
  <a class="conn st" data-st={conn.st} href={href('nodes')}>
    <i></i><span>{t(`nodes.conn.${conn.st}`)}</span>{#if conn.rtt !== undefined}
      {' '}<span class="ms muted">{conn.rtt} ms</span>{/if}
  </a>
{/if}

<style>
  .conn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 32px;
    padding: 0 10px;
    border: 1px solid var(--line);
    border-radius: var(--r-full);
    color: var(--text-2);
    font-size: var(--fs-xs);
    white-space: nowrap;
  }
  .conn:hover {
    text-decoration: none;
  }
  @media (hover: hover) {
    .conn[data-st='ok']:hover,
    .conn[data-st='linking']:hover {
      background: var(--surface-2);
    }
  }
  .conn i {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    color: var(--text-3);
    background: currentColor;
  }
  .conn[data-st='ok'] i {
    color: var(--ok);
  }
  .conn[data-st='limited'] {
    border-color: color-mix(in srgb, var(--dawn) 50%, var(--line));
    background: var(--dawn-soft);
  }
  .conn[data-st='limited'] i {
    color: var(--dawn);
    border-radius: 2px;
    transform: rotate(45deg);
  }
  .conn[data-st='linking'] i {
    color: var(--sky);
    animation: pulse 1.2s var(--ease) infinite;
  }
  .conn[data-st='down'] {
    border-color: color-mix(in srgb, var(--danger) 50%, var(--line));
    background: var(--danger-soft);
    color: var(--danger);
  }
  .conn[data-st='down'] i {
    color: var(--danger);
    background: none;
    border: 2px solid currentColor;
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
      transform: scale(0.82);
    }
  }
  @media (max-width: 767px) {
    .ms {
      display: none;
    }
  }
</style>
