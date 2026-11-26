<script lang="ts">
  import type { Snippet } from 'svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow, TaskRow } from '../../lib/proto';
  import { taskTitle } from '../../lib/task';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';

  /** Side column of the terminal takeover page (`web/design/terminal.html`). */
  let {
    ai,
    task,
    link,
    children,
    watching = false,
  }: {
    ai: AIMemberRow | undefined;
    task: TaskRow | undefined;
    link: { st: 'ok' | 'limited' | 'linking' | 'down'; text: string };
    children?: Snippet;
    watching?: boolean;
  } = $props();

  // The column scrolls when the window is short, so the keyboard must reach it.
  const scrolls = (column: HTMLElement) => void (column.tabIndex = 0);
</script>

<aside class="info" aria-label={t('terminal.aria.info')} use:scrolls>
  <section class="card">
    <header class="card-head">
      <span class="kind">{t('terminal.info.session')}</span>
    </header>
    <dl class="kv">
      <dt>AI</dt>
      <dd>
        {#if ai}
          <span class="who">
            <Avatar id={ai.id} name={ai.name} cli={ai.cli} size="sm" />
            <span>{ai.name}</span>
          </span>
        {:else}
          <span class="muted">—</span>
        {/if}
      </dd>
      {#if !ai?.secretary}
        <dt>{t('terminal.kv.task')}</dt>
        <dd>{task ? `T-${task.num} ${taskTitle(task)}` : t('terminal.kv.task.none')}</dd>
        {#if task?.branch}
          <dt>{t('terminal.kv.branch')}</dt>
          <dd><code>{task.branch}</code></dd>
        {/if}
      {/if}
      <dt>{t('terminal.kv.mode')}</dt>
      <dd>{t(ai?.secretary ? 'terminal.session.secretary' : 'terminal.session.resume')}</dd>
      <dt>{t('terminal.kv.link')}</dt>
      <dd>
        <span class="st" data-st={link.st}><i></i><span>{link.text}</span></span>
      </dd>
    </dl>
    {#if !watching}<p class="hint">
        {#if ai?.secretary}
          {t('terminal.hint.secretary')}
        {:else if !task}
          {t('terminal.hint.newTask')}
        {:else}
          {t('terminal.hint.branch')}
        {/if}
      </p>{/if}
  </section>

  {#if !watching}<section class="card">
      <header class="card-head">
        <span class="kind">{t('terminal.queue.title')}</span>
      </header>
      <p class="hint">
        <Icon name="lock" sm />
        <span>{t('terminal.queue.body')}</span>
      </p>
    </section>{/if}

  {@render children?.()}

  <p class="note" id="term-leave">
    <Icon name="info" sm />
    <span>{t(watching ? 'watch.readonly' : 'terminal.note.keys')}</span>
  </p>
  <p class="note">
    <Icon name="eye" sm />
    <span>{t(watching ? 'watch.privacy' : 'terminal.note.privacy')}</span>
  </p>
  <p class="note" data-tone="dawn">
    <Icon name="relay" sm />
    <span><b>{t('terminal.note.relay.lead')}</b> {t('terminal.note.relay.body')}</span>
  </p>
</aside>

<style>
  /* Beside the terminal: sections under one another, then the hints as one quiet list. */
  .info {
    overflow: auto;
    padding: var(--sp-2) var(--sp-4) var(--sp-4);
    border-left: 1px solid var(--line);
    background: var(--surface);
    display: flex;
    flex-direction: column;
  }
  .info > * {
    flex: none;
  }
  .info > :global(.card) {
    padding: var(--sp-4) 0;
    border: 0;
    border-radius: 0;
    background: none;
    box-shadow: none;
  }
  .info > :global(.card + .card) {
    border-top: 1px solid var(--line);
  }
  .info :global(.card-head .kind) {
    color: var(--text);
    font-weight: 600;
  }
  .info > .note {
    padding: 6px 0;
    background: none;
    color: var(--text-2);
  }
  .info > #term-leave {
    margin-top: var(--sp-1);
    padding-top: var(--sp-4);
    border-top: 1px solid var(--line);
    border-radius: 0;
  }
  .who {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  code {
    padding: 2px 6px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
    font-size: var(--fs-xs);
    overflow-wrap: anywhere;
  }
  @media (max-width: 1023px) {
    .info {
      border-left: 0;
      border-top: 1px solid var(--line);
    }
  }
</style>
