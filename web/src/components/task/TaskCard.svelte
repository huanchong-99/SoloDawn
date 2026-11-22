<script lang="ts">
  import { jumpToMessage } from '../../lib/jump';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { reviewLine, taskBadge, taskSteps, threadPreview, waitBadge } from '../../lib/task';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';
  import Thread from '../thread/Thread.svelte';

  /** Draws a `task` message in the main line from the live `TaskRow` of its group. */
  let { group, msg }: CardProps<'task'> = $props();
  let open = $state(false);
  const task = $derived(group.task(msg.body.task));
</script>

{#if !task}
  <section class="card" aria-label={t('chat.kind.task')}>
    <header class="card-head">
      <span class="kind">{t('chat.kind.task')}</span>
      <code class="cut">{msg.body.task}</code>
    </header>
  </section>
{:else}
  {@const assignee = group.member(task.assignee)}
  {@const badge = taskBadge(group, task)}
  {@const steps = taskSteps(group, task, badge.text)}
  {@const waiting = waitBadge(group, task.wait)}
  {@const review = reviewLine(group, task)}
  {@const noChecks = !task.checks.local && !task.checks.cloud && task.wait?.kind !== 'conflict'}
  <section class="card" aria-labelledby={`task-num-${msg.id} task-title-${msg.id}`}>
    <header class="card-head">
      <span class="mono" id={`task-num-${msg.id}`}>T-{task.num}</span>
      <h3 id={`task-title-${msg.id}`}>{task.title}</h3>
      <span class="badge" class:solid={badge.solid} data-tone={badge.tone}>
        {#if badge.dot}<span class="st" data-st={badge.dot}><i></i></span>{/if}{badge.text}
      </span>
    </header>

    <div class="steps" role="img" aria-label={steps.aria}>
      {#each steps.items as step (step.key)}
        <i data-s={step.mark} class:skip={step.skipped} title={step.label}></i>
      {/each}
      <span>{steps.summary}</span>
    </div>

    <dl class="kv">
      <dt>{t('task.kv.owner')}</dt>
      <dd>
        <span class="who">
          <Avatar
            id={assignee?.kind === 'human' ? assignee.user : task.assignee}
            name={assignee?.name ?? '?'}
            cli={assignee?.kind === 'ai' ? assignee.cli : undefined}
            size="sm"
          />
          <span>{assignee?.name ?? t('chat.gone')}</span>
          {#if assignee?.kind === 'ai' && assignee.host_name}
            <span class="muted">· {assignee.host_name}</span>
          {/if}
        </span>
      </dd>

      {#if task.branch}
        <dt>{t('task.kv.branch')}</dt>
        <dd>
          <code>{task.branch}</code>
          {#if task.pr !== undefined}
            <span class="muted">{' · '}</span>{#if task.pr_url}<a
                href={task.pr_url}
                target="_blank"
                rel="noreferrer"
              >
                PR #{task.pr}
              </a>{:else}<span class="muted">PR #{task.pr}</span>{/if}
          {/if}
        </dd>
      {/if}

      <dt>{t('task.kv.checks')}</dt>
      <dd>
        {#if task.checks.local}
          <span class="res" data-tone={task.checks.local.ok ? 'ok' : 'danger'}>
            <Icon name={task.checks.local.ok ? 'check' : 'x'} sm />{t(
              task.checks.local.ok ? 'task.checks.local.ok' : 'task.checks.local.fail',
            )}
          </span>
        {/if}
        {#if task.checks.cloud}
          <span class="res" data-tone={task.checks.cloud.ok ? 'ok' : 'danger'}>
            <Icon name={task.checks.cloud.ok ? 'check' : 'x'} sm />{t(
              task.checks.cloud.ok ? 'task.checks.cloud.ok' : 'task.checks.cloud.fail',
            )}
          </span>
        {/if}
        {#if task.wait?.kind === 'conflict'}
          <span class="res" data-tone="danger"><Icon name="x" sm />{t('task.badge.conflict')}</span>
        {/if}
        {#if noChecks}<span class="muted">{t('task.checks.none')}</span>{/if}
        {#if task.wait?.msg && waiting}
          {@const waitMsg = task.wait.msg}
          {@const waitKind = task.wait.kind}
          {#if noChecks}<span class="muted">{' · '}</span>{/if}<a
            href={`#m-${waitMsg}`}
            onclick={(event) => {
              event.stopPropagation();
              jumpToMessage(event, waitMsg, waitKind);
            }}
          >
            {waiting.text}
          </a>
        {/if}
      </dd>

      <dt>{t('task.kv.review')}</dt>
      <dd>
        {#if review.verdict}
          <span class="res" data-tone={review.tone}>
            <Icon name={review.tone === 'ok' ? 'check' : 'x'} sm />{review.verdict}
          </span>
        {/if}
        <span class="muted">{review.meta}</span>
      </dd>
    </dl>

    <details class="fold" bind:open>
      <summary>
        <Icon name="down" sm />
        <b>{t('task.thread.count').replace('{n}', `${task.thread.count}`)}</b>
        <span class="cut grow" class:muted={!task.thread.last}>{threadPreview(group, task)}</span>
      </summary>
      {#if open}
        <Thread {group} {task} />
      {/if}
    </details>
  </section>
{/if}

<style>
  .steps {
    display: flex;
    align-items: center;
    gap: 3px;
    i {
      flex: 1;
      height: 4px;
      border-radius: 2px;
      background: var(--surface-3);
    }
    i.skip {
      opacity: 0.45;
    }
    i[data-s='done'] {
      background: color-mix(in srgb, var(--ok) 75%, var(--surface));
    }
    i[data-s='now'] {
      background: var(--sky);
    }
    i[data-s='wait'] {
      background: var(--dawn);
    }
    i[data-s='fail'] {
      background: var(--danger);
    }
    span {
      margin-left: 8px;
      color: var(--text-3);
      font-size: var(--fs-xs);
      white-space: nowrap;
    }
  }
  .who {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    vertical-align: middle;
  }
  .res {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    margin-right: 10px;
    font-weight: 500;
    white-space: nowrap;
    color: var(--tone-text, var(--text-2));
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
    overflow-wrap: anywhere;
  }
</style>
