<script lang="ts">
  import type { Component } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { TaskRow } from '../../lib/proto';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';
  import {
    findTask,
    reviewLine,
    safePrUrl,
    taskBadge,
    taskSteps,
    threadPreview,
    waitBadge,
  } from './task';

  const threads = import.meta.glob<Component<{ group: Group; task: TaskRow }>>(
    '../thread/Thread.svelte',
    { eager: true, import: 'default' },
  );
  const Thread = threads['../thread/Thread.svelte'];

  /** Draws a `task` message in the main line from the live `TaskRow` of its group. */
  let { group, msg }: CardProps<'task'> = $props();
  let open = $state(false);
  const task = $derived(findTask(group, msg.body.task));
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
  {@const prHref = safePrUrl(task.pr_url)}
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

      <dt>{t('task.kv.branch')}</dt>
      <dd>
        <code>{task.branch ?? `task/${task.num}-${task.slug}`}</code>
        {#if task.pr !== undefined}
          <span class="muted">{' · '}</span>{#if prHref}<a
              href={prHref}
              target="_blank"
              rel="noreferrer"
            >
              PR #{task.pr}
            </a>{:else}<span class="muted">PR #{task.pr}</span>{/if}
        {/if}
      </dd>

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
          {#if noChecks}<span class="muted">{' · '}</span>{/if}<a href={`#m-${task.wait.msg}`}>
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
        {#if task.thread.rounds > 0}
          <span class="badge only-wide">
            {t('task.thread.rounds')
              .replace('{n}', `${task.thread.rounds}`)
              .replace('{max}', `${group.info?.limits.rounds ?? 10}`)}
          </span>
        {/if}
      </summary>
      {#if open && Thread}
        <Thread {group} {task} />
      {/if}
    </details>
  </section>
{/if}

<style>
  .card-head {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    flex-wrap: wrap;
    color: var(--text-2);
    font-size: var(--fs-sm);
    .kind {
      font-weight: 600;
      color: var(--text);
    }
    h3 {
      flex: 1;
      min-width: 12ch;
      color: var(--text);
    }
    .badge:last-child {
      margin-left: auto;
    }
  }
  .st {
    display: inline-flex;
    align-items: center;
    i {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: currentColor;
    }
    &[data-st='working'] i {
      color: var(--sky);
      animation: pulse 2s var(--ease) infinite;
    }
    &[data-st='paused'] i {
      border-radius: 1px;
      background: linear-gradient(90deg, currentColor 34%, transparent 34% 66%, currentColor 66%);
    }
    &[data-st='takeover'] i {
      border-radius: 2px;
      color: var(--text);
    }
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
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
  .kv {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 6px var(--sp-4);
    font-size: var(--fs-sm);
    dt {
      color: var(--text-3);
    }
    dd {
      min-width: 0;
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
  .fold {
    border-top: 1px solid var(--line);
    margin: 0 calc(-1 * var(--sp-4)) -14px;
    > summary {
      position: relative;
      isolation: isolate;
      display: flex;
      align-items: center;
      gap: var(--sp-2);
      padding: 10px var(--sp-4);
      cursor: pointer;
      list-style: none;
      font-size: var(--fs-sm);
      color: var(--text-2);
      border-radius: 0 0 var(--r-lg) var(--r-lg);
    }
    > summary::-webkit-details-marker {
      display: none;
    }
    > summary::before {
      content: '';
      position: absolute;
      inset: 0;
      z-index: -1;
      border-radius: inherit;
      background: var(--surface-2);
      opacity: 0;
      transition: opacity var(--dur-1) var(--ease);
    }
    > summary :global(.i) {
      transform: rotate(-90deg);
      transition: transform var(--dur-2) var(--ease);
    }
    > summary b {
      color: var(--text);
      white-space: nowrap;
    }
    &[open] > summary {
      border-radius: 0;
      :global(.i) {
        transform: none;
      }
    }
  }
  @media (hover: hover) {
    .fold > summary:hover::before {
      opacity: 1;
    }
  }
</style>
