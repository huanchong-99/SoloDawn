<script lang="ts">
  import { tiers } from './profile';

  /** Three rising bars, as many lit as the tier is high. */
  let { tier }: { tier: string } = $props();
  const level = $derived((tiers as readonly string[]).indexOf(tier) + 1);
</script>

<i class="bars" aria-hidden="true">
  {#each [1, 2, 3] as n (n)}<b class:on={n <= level}></b>{/each}
</i>

<style>
  .bars {
    display: inline-flex;
    align-items: flex-end;
    gap: 2px;
    height: 10px;
    flex: none;
  }
  b {
    width: 3px;
    height: 4px;
    border-radius: 1px;
    background: currentColor;
    opacity: 0.25;
  }
  b:nth-child(2) {
    height: 7px;
  }
  b:nth-child(3) {
    height: 10px;
  }
  b.on {
    opacity: 1;
  }
  @media (prefers-reduced-motion: no-preference) {
    b {
      transition: opacity var(--dur-2) var(--ease-out);
    }
  }
</style>
