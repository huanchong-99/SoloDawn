<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { taskTitle } from '../../lib/task';
  import Icon from '../Icon.svelte';

  /** The secretary's summary card after a task is merged (contracts/messages.md 2.5). */
  let { group, msg }: CardProps<'summary'> = $props();

  const taskRow = $derived(group.task(msg.body.task));
  const cardLabel = $derived(
    taskRow ? `${t('summary.kind')} · T-${taskRow.num} · ${taskTitle(taskRow)}` : t('summary.kind'),
  );

  const rounds = $derived(
    t(msg.body.rounds === 1 ? 'summary.rounds.one' : 'summary.rounds.many').replace(
      '{n}',
      String(msg.body.rounds),
    ),
  );

  const duration = $derived.by(() => {
    const h = Math.floor(msg.body.minutes / 60);
    const m = msg.body.minutes % 60;
    if (h === 0) return t('summary.minutes').replace('{m}', String(m));
    if (m === 0) return t('summary.hours').replace('{h}', String(h));
    return t('summary.hours_minutes').replace('{h}', String(h)).replace('{m}', String(m));
  });
</script>

<section class="card" aria-label={cardLabel}>
  <header class="card-head">
    <span class="done-icon"><Icon name="check" /></span>
    <span class="kind">{t('summary.kind')}</span>
    {#if taskRow}<span class="mono">T-{taskRow.num}</span>{/if}
    <span class="badge" data-tone="ok">{t('summary.merged')}</span>
  </header>
  {#if taskRow}<h3>{taskTitle(taskRow)}</h3>{/if}
  <ul class="stats">
    <li><b>{msg.body.score}</b><span>{t('summary.score')}</span></li>
    <li><b>{rounds}</b><span>{t('summary.rounds')}</span></li>
    <li><b>{duration}</b><span>{t('summary.time')}</span></li>
    <li>
      <b><span class="add">+{msg.body.added}</span> <span class="del">−{msg.body.removed}</span></b>
      <span>
        {msg.body.pr !== undefined
          ? t('summary.pr').replace('{n}', String(msg.body.pr))
          : t('summary.changes')}
      </span>
    </li>
  </ul>
</section>

<style>
  .done-icon {
    display: inline-grid;
    color: var(--ok);
  }
  .done-icon :global(.i) {
    width: 16px;
    height: 16px;
  }
  /* The figures in a row: a large number over its small label. What was added is green and what
     was removed red: the two results in this card. */
  .stats {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-3) var(--sp-8);
    font-size: var(--fs-xs);
    color: var(--text-3);
  }
  .stats b {
    display: block;
    color: var(--text);
    font-family: var(--font-sans);
    font-size: var(--fs-2xl);
    font-weight: 600;
    line-height: 1.25;
    letter-spacing: -0.01em;
  }
  .add {
    color: var(--ok);
  }
  .del {
    color: var(--danger);
  }
  @media (max-width: 640px) {
    .stats {
      gap: var(--sp-3) var(--sp-5);
    }
    .stats b {
      font-size: var(--fs-xl);
    }
  }
</style>
