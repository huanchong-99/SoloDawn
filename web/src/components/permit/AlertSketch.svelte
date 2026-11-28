<script lang="ts">
  // Schematic macOS permission prompt.
</script>

<div class="sk sk-alert" aria-hidden="true">
  <span class="appi"></span>
  <span class="sk-bar wide"></span>
  <span class="sk-bar mid"></span>
  <span class="pills"><i></i><i class="go"></i></span>
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
  .sk-alert {
    background: var(--raised);
    border-radius: 10px;
    box-shadow: var(--shadow-pop);
    display: grid;
    justify-items: center;
    align-content: center;
    gap: 5px;
  }
  .sk-bar {
    height: 4px;
    border-radius: 2px;
    background: var(--surface-3);
  }
  .wide {
    width: 64px;
  }
  .mid {
    width: 44px;
  }
  .pills {
    display: flex;
    gap: 6px;
    margin-top: 3px;
  }
  .pills i {
    width: 44px;
    height: 14px;
    border-radius: 7px;
    background: var(--surface-3);
    position: relative;
  }
  .pills i.go {
    background: var(--sky);
  }
  @media (prefers-reduced-motion: no-preference) {
    .pills i.go::after {
      content: '';
      position: absolute;
      inset: -3px;
      border-radius: 10px;
      box-shadow: 0 0 0 2px var(--sky);
      opacity: 0;
      animation: pulse 900ms var(--ease-out) 300ms 2;
    }
  }
  @keyframes pulse {
    from {
      opacity: 0.5;
      transform: scale(1);
    }
    to {
      opacity: 0;
      transform: scale(1.25);
    }
  }
</style>
