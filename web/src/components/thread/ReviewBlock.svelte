<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Rich from '../Rich.svelte';

  /** Scored review block: score against pass threshold and itemized remarks. */
  let { group, msg }: CardProps<'review'> = $props();
  const body = $derived(msg.body);
</script>

<div class="review" data-tone={body.ok ? 'ok' : 'danger'}>
  <div class="score">
    <b>{body.score}</b>{t('thread.review.pass').replace('{n}', String(body.pass))}
  </div>
  {#if body.items.length}
    <ul class="bul">
      {#each body.items as item, i (i)}
        <li>
          {#if item.veto}
            <span class="badge solid veto" data-tone="danger">{t('thread.review.veto')}</span>
          {/if}
          <Rich {group} text={item.text} mentions={[]} />
          {#if item.file}
            <code class="loc">
              {item.line !== undefined ? `${item.file}:${item.line}` : item.file}
            </code>
          {/if}
        </li>
      {/each}
    </ul>
  {:else}
    <p class="summary">
      {t(body.ok ? 'thread.review.ok' : 'thread.review.fail')} ·
      <code>{body.commit.slice(0, 7)}</code>
    </p>
  {/if}
</div>

<style>
  /* A scored review: an inset block with the score, a result, in its tone. */
  .review {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: var(--sp-3);
    padding: 10px var(--sp-3);
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
  }
  .score {
    text-align: center;
    line-height: 1.3;
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .score b {
    display: block;
    color: var(--tone-text);
    font: 600 var(--fs-xl) / 1.2 var(--font-sans);
  }
  .bul {
    list-style: disc;
    padding-left: 1.2em;
  }
  .bul li + li {
    margin-top: 2px;
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-3);
  }
  .loc {
    margin-left: 4px;
  }
  .veto {
    height: 18px;
    padding: 0 6px;
    margin-right: 4px;
  }
  .summary {
    align-self: center;
    color: var(--text-2);
  }
</style>
