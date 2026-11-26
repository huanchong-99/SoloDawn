<script lang="ts">
  import { tick } from 'svelte';
  import { jumpToMessage } from '../../lib/jump';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { reviewLine, taskBadge, taskSteps, threadPreview, waitBadge } from '../../lib/task';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';
  import Thread from '../thread/Thread.svelte';
  import TaskActions from './TaskActions.svelte';
  import TaskMenu from './TaskMenu.svelte';

  /** Draws a `task` message in the main line from the live `TaskRow` of its group. */
  let { group, msg }: CardProps<'task'> = $props();
  let open = $state(false);
  // A thread once opened is kept while it is folded, with what is being written in it.
  let opened = $state(false);
  let card = $state<HTMLElement>();
  let actions = $state<TaskActions>();
  const task = $derived(group.task(msg.body.task));

  $effect(() => {
    if (open) opened = true;
  });

  async function reply() {
    open = true;
    await tick();
    card?.querySelector<HTMLElement>('.composer textarea')?.focus();
  }
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
  <section
    class="card"
    bind:this={card}
    aria-labelledby={`task-num-${msg.id} task-title-${msg.id}`}
  >
    <header class="card-head">
      <span class="mono" id={`task-num-${msg.id}`}>T-{task.num}</span>
      <h3 id={`task-title-${msg.id}`}>{task.title}</h3>
      <span class="badge" class:solid={badge.solid} data-tone={badge.tone}>
        {#if badge.dot}<span class="st" data-st={badge.dot}><i></i></span>{/if}{badge.text}
      </span>
      <TaskMenu {group} {task} onpick={(op) => void actions?.pick(op)} />
    </header>

    <div class="steps" role="img" aria-label={steps.aria}>
      {#each steps.items as step (step.key)}
        <i data-s={step.mark} title={step.label}></i>
      {/each}
      <span>{steps.summary}</span>
    </div>

    <TaskActions bind:this={actions} {group} {task} onreply={() => void reply()} />

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
          <span>{group.nameOf(task.assignee)}</span>
          {#if assignee?.kind === 'ai' && assignee.host_name}
            <span class="muted">· {assignee.host_name}</span>
          {/if}
        </span>
      </dd>

      {#if assignee?.kind === 'ai'}
        <dt>{t('task.kv.folder')}</dt>
        <dd><code title={assignee.workspace}>{assignee.workspace}</code></dd>
      {/if}

      {#if task.wait?.msg && waiting}
        {@const waitMsg = task.wait.msg}
        {@const waitKind = task.wait.kind}
        <dt>{t('task.kv.wait')}</dt>
        <dd>
          <a
            href={`#m-${waitMsg}`}
            onclick={(event) => {
              event.stopPropagation();
              jumpToMessage(event, waitMsg, waitKind);
            }}
          >
            {waiting.text}
          </a>
        </dd>
      {/if}

      <dt>{t('task.kv.review')}</dt>
      <dd>
        {#if review.verdict}
          <span class="res" data-tone={review.tone}>
            <Icon name={review.tone === 'ok' ? 'check' : 'x'} sm />{review.verdict}
          </span>
        {/if}
        <span class="muted meta">{review.meta}</span>
      </dd>
    </dl>

    <details class="fold thread-fold" bind:open>
      <summary>
        <Icon name="down" sm />
        <b>{t('task.thread.count').replace('{n}', `${task.thread.count}`)}</b>
        <span class="cut grow" class:muted={!task.thread.last}>{threadPreview(group, task)}</span>
      </summary>
      {#if opened}
        <Thread {group} {task} />
      {/if}
    </details>
  </section>
{/if}

<style>
  .badge {
    margin-left: auto;
  }
  /* On a phone the title has a line of its own, under the number, the badge and the menu. */
  @media (max-width: 640px) {
    h3 {
      order: 1;
    }
  }
  /* The stages as one row of segments, however many there are: done, the one being worked on,
     one that waits for the person, and those to come. A new stage fills its segment from the
     left. The words beside the row say the stage, so colour is not its only carrier. */
  .steps {
    display: flex;
    align-items: center;
    gap: 2px;

    i {
      position: relative;
      flex: 1;
      height: 4px;
      border-radius: 2px;
      background: var(--surface-3);
      overflow: hidden;
    }
    i::before {
      content: '';
      position: absolute;
      inset: 0;
      background: var(--fill, transparent);
      transform: scaleX(0);
      transform-origin: left;
      transition: transform var(--dur-3) var(--ease-out);
    }
    i[data-s]::before {
      transform: none;
    }
    i[data-s='done'] {
      --fill: var(--ok);
    }
    i[data-s='now'] {
      --fill: var(--sky);
    }
    i[data-s='wait'] {
      --fill: var(--dawn);
    }
    i[data-s='fail'] {
      --fill: var(--danger);
    }
    span {
      margin-left: 10px;
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
    gap: 4px;
    margin-right: 10px;
    white-space: nowrap;
    color: var(--tone-text, var(--text-2));
  }
  /* "Codex 评 · 及格 90" breaks at its spaces only, never inside a word. */
  .meta {
    word-break: keep-all;
  }
  code {
    padding: 2px 6px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
    font-size: var(--fs-xs);
    overflow-wrap: anywhere;
  }
  /* The way into the thread: its count in the accent, the newest line beside it. */
  .thread-fold > summary b {
    color: var(--sky);
  }
  .thread-fold > summary .cut {
    color: var(--text-3);
  }
</style>
