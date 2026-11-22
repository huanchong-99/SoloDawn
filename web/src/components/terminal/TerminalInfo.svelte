<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow, TaskRow } from '../../lib/proto';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';

  /** Side column of the terminal takeover page (`web/design/terminal.html`). */
  let {
    ai,
    task,
    link,
    queued,
  }: {
    ai: AIMemberRow | undefined;
    task: TaskRow | undefined;
    link: { st: 'ok' | 'limited' | 'linking' | 'down'; text: string };
    queued: number;
  } = $props();
</script>

<aside class="info" aria-label={t('terminal.aria.info')}>
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
        <dd>{task ? `T-${task.num} ${task.title}` : t('terminal.kv.task.none')}</dd>
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
    <p class="hint">
      {#if ai?.secretary}
        {t('terminal.hint.secretary')}
      {:else if !task}
        {t('terminal.hint.newTask')}
      {:else}
        {t('terminal.hint.branch')}
      {/if}
    </p>
  </section>

  <section class="card">
    <header class="card-head">
      <span class="kind">{t('terminal.queue.title')}</span>
      <span class="badge count">{queued}</span>
    </header>
    <p class="hint">
      <Icon name="lock" sm />
      <span>{t(queued > 0 ? 'terminal.queue.held' : 'terminal.queue.empty')}</span>
    </p>
  </section>

  <p class="note">
    <Icon name="eye" sm />
    <span>{t('terminal.note.privacy')}</span>
  </p>
  <p class="note" data-tone="dawn">
    <Icon name="relay" sm />
    <span><b>{t('terminal.note.relay.lead')}</b> {t('terminal.note.relay.body')}</span>
  </p>
</aside>

<style>
  .info {
    overflow: auto;
    padding: var(--sp-4);
    border-left: 1px solid var(--line);
    background: var(--surface);
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
  }
  .info > * {
    flex: none;
  }
  .who {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
    overflow-wrap: anywhere;
  }
  @media (max-width: 1023px) {
    .info {
      border-left: 0;
      border-top: 1px solid var(--line);
    }
  }
</style>
