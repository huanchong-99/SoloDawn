<script lang="ts">
  import { parts } from '../../lib/compose';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';

  /** Scored review block: score against pass threshold and itemized remarks. */
  let { msg }: CardProps<'review'> = $props();
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
          {#each parts(item.text, []) as part, j (j)}
            {#if part.kind === 'code'}<code>{part.text}</code>{:else}{part.text}{/if}
          {/each}
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
  .review {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: var(--sp-3);
    padding: 8px 10px;
    border-radius: var(--r-md);
    background: var(--tone-soft);
    font-size: var(--fs-sm);
  }
  .score {
    text-align: center;
    line-height: 1.2;
    color: var(--text-3);
    font-size: 11px;
  }
  .score b {
    display: block;
    color: var(--tone-text);
    font: 600 var(--fs-xl) / 1.1 var(--font-mono);
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
    background: var(--surface);
  }
  .loc {
    margin-left: 4px;
  }
  .veto {
    height: 18px;
    padding: 0 6px;
    margin-right: 4px;
    font-size: 11px;
  }
  .summary {
    align-self: center;
    color: var(--text-2);
  }
</style>
