<script lang="ts" module>
  // Onboarding line icons from design/base.js, drawn on the same 24x24 grid as Icon.svelte.
  const paths = {
    copy: '<path d="M9 9h11v11H9zM5 15H4V4h11v1"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.5"/>',
    direct:
      '<circle cx="5" cy="12" r="2.5"/><circle cx="19" cy="12" r="2.5"/><path d="M7.5 12h9"/>',
    key: '<circle cx="8" cy="15" r="4"/><path d="M11 12l9-9M16 7l3 3"/>',
  };
  export type PairingIconName = keyof typeof paths;
</script>

<script lang="ts">
  let { name, sm = false }: { name: PairingIconName; sm?: boolean } = $props();
</script>

<svg class="i" class:sm viewBox="0 0 24 24" aria-hidden="true">{@html paths[name]}</svg>
