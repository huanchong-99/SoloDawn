<script lang="ts" module>
  // Line icons drawn for SoloDawn on a 24×24 grid; the stroke comes from .i in base.css.
  const icons = {
    chat: '<path d="M5 4h14a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H9l-5 4V5a1 1 0 0 1 1-1z"/>',
    pc: '<path d="M3 5h18v11H3zM8 20h8M12 16v4"/>',
    sliders:
      '<path d="M4 7h9M19 7h1M4 17h1M11 17h9"/><circle cx="16" cy="7" r="2.5"/><circle cx="8" cy="17" r="2.5"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z"/>',
  };
  export type IconName = keyof typeof icons;
</script>

<script lang="ts">
  let { name }: { name: IconName } = $props();
</script>

<svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html icons[name]}</svg>
