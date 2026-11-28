<script lang="ts">
  // Schematic macOS Full Disk Access drag panel.
</script>

<div class="sk sk-drag" aria-hidden="true">
  <div class="win">
    <span class="sk-bar"></span>
    <span class="sk-bar"></span>
    <span class="sk-bar new"></span>
  </div>
  <div class="pan">
    <span class="appi"></span>
    <span class="sk-bar"></span>
  </div>
  <span class="appi ghost"></span>
</div>

<style>
  .sk {
    width: 120px;
    height: 76px;
    position: relative;
  }
  .appi {
    width: 16px;
    height: 16px;
    border-radius: 5px;
    background: #0c0f16;
    position: relative;
    flex: none;
  }
  .appi::after {
    content: '';
    position: absolute;
    left: 3px;
    right: 3px;
    top: 8px;
    height: 2px;
    border-radius: 2px;
    background: var(--horizon);
  }
  .sk-bar {
    height: 4px;
    border-radius: 2px;
    background: var(--surface-3);
  }
  .win {
    position: absolute;
    inset: 0 0 30px 0;
    background: var(--raised);
    border-radius: 8px;
    box-shadow: var(--shadow-pop);
    padding: 8px;
    display: grid;
    gap: 5px;
    align-content: start;
  }
  .win .sk-bar {
    width: 100%;
  }
  .win .sk-bar.new {
    background: var(--ok);
    opacity: 0.8;
  }
  .pan {
    position: absolute;
    left: 12px;
    right: 12px;
    bottom: 0;
    height: 24px;
    background: var(--raised);
    border-radius: 8px;
    box-shadow: var(--shadow-pop);
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 6px;
  }
  .pan .sk-bar {
    width: 44px;
  }
  .ghost {
    position: absolute;
    left: 18px;
    bottom: 4px;
  }
  @media (prefers-reduced-motion: no-preference) {
    .win .sk-bar.new {
      opacity: 0;
      animation: newrow 1400ms var(--ease) 300ms 2 both;
    }
    .ghost {
      animation: lift 1400ms var(--ease) 300ms 2 both;
    }
  }
  @keyframes lift {
    0%,
    20% {
      transform: none;
      opacity: 1;
    }
    70% {
      transform: translate(8px, -38px) scale(0.9);
      opacity: 1;
    }
    85%,
    100% {
      transform: translate(8px, -38px) scale(0.9);
      opacity: 0;
    }
  }
  @keyframes newrow {
    0%,
    68% {
      opacity: 0;
    }
    85%,
    100% {
      opacity: 0.8;
    }
  }
</style>
