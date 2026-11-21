<script lang="ts" module>
  // Line icons specific to the computers and pairing area, from web/design/base.js on the same
  // 24x24 grid as Icon.svelte.
  const icons = {
    direct:
      '<circle cx="5" cy="12" r="2.5"/><circle cx="19" cy="12" r="2.5"/><path d="M7.5 12h9"/>',
    relay:
      '<circle cx="4.5" cy="17" r="2.2"/><circle cx="12" cy="6" r="2.2"/><circle cx="19.5" cy="17" r="2.2"/><path d="M5.8 15.2l4.9-7.4M13.3 7.8l4.9 7.4"/>',
    copy: '<path d="M9 9h11v11H9zM5 15H4V4h11v1"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    phone: '<path d="M7 3h10v18H7zM11 18h2"/>',
  };
  export type NodeIconName = keyof typeof icons;
</script>

<script lang="ts">
  let { name, sm = false }: { name: NodeIconName; sm?: boolean } = $props();
</script>

<svg class="i" class:sm viewBox="0 0 24 24" aria-hidden="true">{@html icons[name]}</svg>
