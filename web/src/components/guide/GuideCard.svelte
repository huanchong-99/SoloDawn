<script lang="ts">
  import { recent } from '../../lib/groups.svelte';
  import { guideSteps, openGuide } from '../../lib/guide.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { t } from '../../lib/prefs.svelte';

  /** Settings card to reopen the five-step guide at any time. */
  let { net }: { net: Net } = $props();
  const uid = $props.id();

  const doneCount = $derived(
    guideSteps(nodes.list(net), recent()).filter((step) => step.done).length,
  );
</script>

<section class="card set guide-card" aria-labelledby={`${uid}-h`}>
  <div class="set-row">
    <div>
      <b id={`${uid}-h`}>{t('guide.settings.title')}</b>
      <p class="body">{t('guide.settings.body')}</p>
      <p class="progress">{t('guide.settings.progress').replace('{done}', String(doneCount))}</p>
    </div>
    <button type="button" class="btn" onclick={openGuide}>{t('guide.settings.open')}</button>
  </div>
</section>

<style>
  .body {
    color: var(--text-2);
  }
  .progress {
    font-size: var(--fs-xs);
    color: var(--text-3);
    margin-top: 2px;
  }
</style>
