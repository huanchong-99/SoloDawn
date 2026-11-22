<script lang="ts" module>
  import type { Net } from '../../lib/net.svelte';

  let activeNet = $state.raw<Net>();
</script>

<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { find, type Group } from '../../lib/groups.svelte';
  import { indicator } from '../../lib/nodes.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { href, route } from '../../lib/route.svelte';

  let { net, group }: { net?: Net; group?: Group } = $props();

  const narrow = new MediaQuery('max-width: 767px');

  $effect(() => {
    if (!net) return;
    activeNet = net;
    return () => {
      if (activeNet === net) activeNet = undefined;
    };
  });

  const netSource = $derived(net ?? activeNet);
  const scope = $derived(group ?? (route.page === 'chat' ? find(route.params.g) : undefined));
  const shown = $derived(group ? narrow.current : !(route.page === 'chat' && narrow.current));
  const conn = $derived(shown && netSource ? indicator(netSource, scope) : undefined);
  const text = $derived(
    conn ? t(conn.st === 'down' && !scope ? 'nodes.conn.downAll' : `nodes.conn.${conn.st}`) : '',
  );
  const label = $derived(
    conn
      ? conn.rtt !== undefined
        ? t('nodes.conn.labelRtt')
            .replace('{status}', text.toLowerCase())
            .replace('{rtt}', String(conn.rtt))
        : t('nodes.conn.label').replace('{status}', text.toLowerCase())
      : '',
  );
</script>

{#if conn}
  <a class="conn st" data-st={conn.st} href={href('nodes')} aria-label={label}>
    <i></i><span>{text}</span>{#if conn.rtt !== undefined}
      {' '}<span class="ms muted">{conn.rtt} ms</span>{/if}
  </a>
{/if}

<style>
  .conn {
    height: 32px;
    padding: 0 10px;
    border: 1px solid var(--line);
    border-radius: var(--r-full);
    font-size: var(--fs-xs);
    white-space: nowrap;
  }
  .conn:hover {
    text-decoration: none;
  }
  @media (hover: hover) {
    .conn[data-st='ok']:hover,
    .conn[data-st='linking']:hover,
    .conn[data-st='offline']:hover {
      background: var(--surface-2);
    }
  }
  .conn[data-st='limited'] {
    border-color: color-mix(in srgb, var(--dawn) 50%, var(--line));
    background: var(--dawn-soft);
  }
  .conn[data-st='down'] {
    border-color: color-mix(in srgb, var(--danger) 50%, var(--line));
    background: var(--danger-soft);
    color: var(--danger);
  }
  @media (max-width: 767px) {
    .ms {
      display: none;
    }
  }
</style>
