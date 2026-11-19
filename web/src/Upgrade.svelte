<script lang="ts">
  import { t } from './lib/prefs.svelte';
</script>

<main>
  <p class="brand"><img class="logo-mark" src="icon.svg" alt="" />SoloDawn</p>
  <section class="card need">
    <h1>{t('upgrade.title')}</h1>
    <p>{t('upgrade.body')}</p>
  </section>
</main>

<style>
  main {
    display: grid;
    place-content: center;
    gap: var(--sp-5);
    min-height: 100dvh;
    padding: var(--sp-5);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: var(--fs-lg);
    font-weight: 600;
    letter-spacing: -0.02em;
  }
  .card {
    max-width: 440px;
    padding: var(--sp-5);
  }
  h1 {
    font-size: var(--fs-xl);
  }
  .card p {
    color: var(--text-2);
  }
</style>
