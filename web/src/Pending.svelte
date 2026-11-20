<script lang="ts">
  import Icon from './components/Icon.svelte';
  import { t } from './lib/prefs.svelte';
  import { href, route } from './lib/route.svelte';

  // A page of the route table whose component is not built yet.
</script>

<div class="page pending">
  <div class="page-head">
    <div class="grow">
      <h2>{t(`page.${route.page}`)}</h2>
      <p>{t('app.pending')}</p>
    </div>
  </div>
  <section class="card">
    <span class="horizon"></span>
    <p>{t('app.pending.body')}</p>
    <a class="btn btn-sm" href={href('groups')}><Icon name="back" sm />{t('app.home')}</a>
  </section>
</div>

<style>
  .pending {
    max-width: 640px;
  }
  .card {
    align-items: flex-start;
  }
  .card p {
    color: var(--text-2);
  }
  /* The brand's line of dawn, drawn from the left as the page arrives. */
  .horizon {
    align-self: stretch;
    height: 2px;
    border-radius: 1px;
    background: var(--horizon);
    transform-origin: left;
    animation: draw var(--dur-4) var(--ease-out) var(--dur-2) backwards;
  }
  @keyframes draw {
    from {
      transform: scaleX(0);
    }
  }
</style>
