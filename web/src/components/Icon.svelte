<script lang="ts" module>
  // Line icons drawn for SoloDawn on a 24×24 grid; the stroke comes from .i in base.css.
  const icons = {
    chat: '<path d="M5 4h14a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H9l-5 4V5a1 1 0 0 1 1-1z"/>',
    pc: '<path d="M3 5h18v11H3zM8 20h8M12 16v4"/>',
    sliders:
      '<path d="M4 7h9M19 7h1M4 17h1M11 17h9"/><circle cx="16" cy="7" r="2.5"/><circle cx="8" cy="17" r="2.5"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    back: '<path d="M15 5l-7 7 7 7"/>',
    down: '<path d="M6 9l6 6 6-6"/>',
    at: '<circle cx="12" cy="12" r="3.5"/><path d="M15.5 8.5v5a2.5 2.5 0 0 0 5 0V12a8.5 8.5 0 1 0-3.4 6.8"/>',
    send: '<path d="M4 12l16-8-6 16-3-7-7-1z"/>',
    alert: '<path d="M12 4l9 16H3l9-16zM12 10v4M12 17v.5"/>',
    check: '<path d="M5 12.5l4.5 4.5L19 7.5"/>',
    shield: '<path d="M12 3l8 3v6c0 4.5-3.2 8-8 9-4.8-1-8-4.5-8-9V6l8-3z"/>',
    fork: '<path d="M6 4v5c0 3.5 6 3.5 6 7v4M18 4v5c0 3.5-6 3.5-6 7"/>',
    doc: '<path d="M6 3h9l4 4v14H6zM14 3v5h5M9 13h7M9 17h5"/>',
    home: '<path d="M4 11l8-7 8 7v9H4zM10 20v-6h4v6"/>',
    users:
      '<circle cx="9" cy="8" r="3"/><path d="M3 20c0-3.3 2.7-6 6-6s6 2.7 6 6M16 5.2a3 3 0 0 1 0 5.6M17 14.3c2.3.8 4 3 4 5.7"/>',
    x: '<path d="M6 6l12 12M18 6L6 18"/>',
    terminal: '<path d="M4 5h16v14H4zM8 10l3 2-3 2M13 15h3"/>',
  };
  export type IconName = keyof typeof icons;
</script>

<script lang="ts">
  let { name, sm = false }: { name: IconName; sm?: boolean } = $props();
</script>

<svg class="i" class:sm viewBox="0 0 24 24" aria-hidden="true">{@html icons[name]}</svg>
