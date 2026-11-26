<script lang="ts">
  import { tick } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { statusOf } from '../../lib/members';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow, TaskRow } from '../../lib/proto';
  import {
    assignNotes,
    cancelNotes,
    changedRoles,
    sendTask,
    taskHold,
    taskOps,
    workingAIs,
    type Roles,
    type TaskOp,
  } from '../../lib/taskops';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** In a task card: why the task stands still and what moves it on, with the action that does;
   *  and, once the menu asks for them, the confirmation of a cancel and the form for other AIs. */
  let { group, task, onreply }: { group: Group; task: TaskRow; onreply: () => void } = $props();
  const uid = $props.id();

  let root: HTMLElement;
  let asked = $state<'assign' | 'cancel'>();
  let busy = $state(false);
  let failed = $state('');
  // Only what the person chose: the rest follows the row.
  let chosen = $state<Roles>({});

  const ops = $derived(taskOps(group, task));
  const hold = $derived(taskHold(group, task));
  const locked = $derived(busy || !group.home.ctl);

  const ais = $derived(workingAIs(group));
  const next = $derived({ assignee: task.assignee, reviewer: task.reviewer ?? '', ...chosen });
  const roles = $derived(changedRoles(task, chosen));
  const same = $derived(next.assignee === next.reviewer);
  const icon = $derived.by(() => {
    if (hold?.tone) return 'alert';
    return task.state === 'paused' ? 'pause' : 'clock';
  });
  const away = (ai: AIMemberRow) =>
    ai.state === 'offline' || ai.state === 'limited' ? ` · ${statusOf(group, ai).text}` : '';

  // What was asked for goes when the task no longer allows it: another person was faster.
  $effect(() => {
    if (asked && !ops.includes(asked)) asked = undefined;
  });
  // A refusal was about the row before.
  $effect(() => {
    void task;
    failed = '';
  });

  /** What the menu picked: a pause or a resume at once, the others after a question. */
  export async function pick(op: TaskOp) {
    if (op !== 'assign' && op !== 'cancel') return run(op);
    chosen = {};
    await ask(op);
  }

  /** Asks, or stops asking; focus goes to the question, or back to the menu that it came from. */
  async function ask(what: typeof asked, back = '.more') {
    asked = what;
    failed = '';
    await tick();
    root
      .closest('.card')!
      .querySelector<HTMLElement>(what ? '.ask' : back)
      ?.focus();
  }

  async function run(op: TaskOp, roles?: Roles) {
    busy = true;
    failed = '';
    // The pressed button is about to be disabled, and would let go of the focus.
    root.focus({ preventScroll: true });
    failed = (await sendTask(group, task, op, roles)).err?.code ?? '';
    busy = false;
    // A cancelled task has no menu any more, only its thread.
    if (!failed) await ask(undefined, op === 'cancel' ? 'summary' : '.more');
  }
</script>

{#snippet refusal()}
  {#if failed === 'conflict' || failed === 'paused'}
    <p class="fail" role="alert">{t('task.refused')}</p>
  {:else if failed}
    <Failure code={failed} />
  {/if}
{/snippet}

{#snippet button(label: string, onclick: () => void)}
  <button type="button" class="btn btn-sm" disabled={locked} {onclick}>{label}</button>
{/snippet}

<div class="acts" class:none={!hold && !asked && !failed} tabindex="-1" bind:this={root}>
  {#if hold}
    <div class="hold" data-tone={hold.tone}>
      <p role="status">
        <Icon name={icon} sm />
        <span>{hold.text}</span>
      </p>
      <div class="row">
        {#if hold.kind === 'rounds'}
          <button type="button" class="btn btn-sm" disabled={!group.home.ctl} onclick={onreply}>
            {t('task.op.reply')}
          </button>
        {/if}
        {#if hold.kind === 'reviewer' && ops.includes('assign')}
          {@render button(t('task.op.reviewer'), () => void pick('assign'))}
        {:else if hold.kind === 'denied' && ops.includes('assign')}
          {@render button(t('task.op.assign'), () => void pick('assign'))}
        {:else if hold.kind !== 'preempted' && ops.includes('resume')}
          {@render button(t('task.op.resume'), () => void pick('resume'))}
        {/if}
      </div>
    </div>
  {/if}

  {#if asked === 'cancel'}
    <div
      class="ask hot"
      role="group"
      aria-labelledby={`${uid}-title`}
      aria-describedby={`${uid}-body`}
      tabindex="-1"
    >
      <div class="grow">
        <b id={`${uid}-title`}>{t('task.cancel.title').replace('{task}', `T-${task.num}`)}</b>
        <div id={`${uid}-body`}>
          {#each cancelNotes(group, task) as note (note)}<p>{note}</p>{/each}
        </div>
        {@render refusal()}
      </div>
      <div class="row end">
        <button
          type="button"
          class="btn btn-sm btn-danger solid"
          disabled={locked}
          aria-busy={busy}
          onclick={() => void run('cancel')}
        >
          {t('task.op.cancel')}
        </button>
        <button
          type="button"
          class="btn btn-sm"
          disabled={busy}
          onclick={() => void ask(undefined)}
        >
          {t('task.cancel.keep')}
        </button>
      </div>
    </div>
  {:else if asked === 'assign'}
    <form
      class="ask"
      aria-label={t('task.op.assign')}
      tabindex="-1"
      onsubmit={(event) => {
        event.preventDefault();
        void run('assign', roles);
      }}
    >
      <div class="fields">
        <label class="field">
          <span>{t('task.assign.assignee')}</span>
          <select
            class="input"
            bind:value={() => next.assignee, (id) => (chosen.assignee = id)}
            disabled={busy}
          >
            {#each ais as ai (ai.id)}
              <option value={ai.id}>{ai.name} · {ai.host_name}{away(ai)}</option>
            {/each}
          </select>
        </label>
        <label class="field">
          <span>{t('task.assign.reviewer')}</span>
          <select
            class="input"
            bind:value={() => next.reviewer, (id) => (chosen.reviewer = id)}
            disabled={busy}
          >
            {#if !task.reviewer}
              <option value="">{t('task.assign.nobody')}</option>
            {/if}
            {#each ais as ai (ai.id)}
              <option value={ai.id}>{ai.name} · {ai.host_name}{away(ai)}</option>
            {/each}
          </select>
        </label>
      </div>
      <div class="notes" aria-live="polite">
        {#if same}
          <p class="fail">{t('task.assign.same')}</p>
        {:else}
          {#each assignNotes(group, task, roles) as note (note)}<p>{note}</p>{/each}
        {/if}
      </div>
      {@render refusal()}
      <div class="row end">
        <button
          class="btn btn-sm btn-primary"
          disabled={locked || same || !Object.keys(roles).length}
          aria-busy={busy}
        >
          {t('task.op.assign')}
        </button>
        <button
          type="button"
          class="btn btn-sm"
          disabled={busy}
          onclick={() => void ask(undefined)}
        >
          {t('task.assign.back')}
        </button>
      </div>
    </form>
  {:else}
    {@render refusal()}
  {/if}
</div>

<style>
  .acts {
    display: grid;
    gap: var(--sp-2);
    outline: none;
  }
  /* With nothing to say it takes no room in the card. */
  .none {
    display: contents;
  }
  /* What holds the task up, and a question asked in place: inset blocks on the second surface.
     Neither is tinted; a glyph in the tone says how it stands. */
  .hold {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--sp-2) var(--sp-3);
    padding: 10px var(--sp-3);
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
  }
  .hold p {
    display: flex;
    flex: 1 1 260px;
    gap: var(--sp-2);
    line-height: 1.5;
  }
  .hold :global(.i) {
    margin-top: 2px;
    color: var(--tone-text, var(--text-3));
  }
  .hold .row:empty {
    display: none;
  }
  .ask {
    display: grid;
    gap: var(--sp-3);
    padding: var(--sp-3);
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
    outline-offset: -2px;
    /* Clear of the day that stays at the top of the line. */
    scroll-margin-block: var(--sp-12);
    animation:
      rise var(--dur-2) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .ask b {
    font-size: var(--fs-ui);
    font-weight: 600;
  }
  .ask p {
    color: var(--text-2);
    line-height: 1.5;
  }
  .fields {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 200px), 1fr));
    gap: var(--sp-3);
  }
  .notes:empty {
    display: none;
  }
  .end {
    justify-content: flex-end;
  }
  p.fail {
    color: var(--danger);
    font-size: var(--fs-sm);
  }
</style>
