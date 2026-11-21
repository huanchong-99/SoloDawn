<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import type { TaskRow } from '../../lib/proto';
  import { clock } from '../../lib/when';
  import Icon, { type IconName } from '../Icon.svelte';

  /** Notice written by the home (contracts/messages.md 2.11), localized by code. */
  let { group, msg }: CardProps<'system'> = $props();
  const body = $derived(msg.body);
  const time = $derived(clock(msg.ts, prefs.lang));
  const iso = $derived(new Date(msg.ts).toISOString());

  const short = (sha: string) => sha.slice(0, 7);
  const who = (id: string) => group.member(id)?.name ?? t('chat.gone');
  const taskRow = (id: string): TaskRow | undefined => {
    const row = group.rows.get(`task ${id}`);
    return row && row.r === 'task' && !('gone' in row) ? row : undefined;
  };
  const taskTag = (id: string) => {
    const row = taskRow(id);
    return row ? `T-${row.num}` : short(id);
  };
  const join = (...items: (string | undefined)[]) => items.filter(Boolean).join(' · ');

  const secretary = $derived(
    (group.info?.secretary ? group.member(group.info.secretary) : undefined) ??
      group.members.find((m) => m.kind === 'ai' && m.secretary),
  );
  const secName = $derived(secretary?.name ?? t('chat.secretary'));

  const icons: Record<string, { icon: IconName; tone?: 'ok' | 'danger' | 'dawn' }> = {
    task_merged: { icon: 'check', tone: 'ok' },
    group_resumed: { icon: 'check', tone: 'ok' },
    task_failed: { icon: 'x', tone: 'danger' },
    login_invalid: { icon: 'alert', tone: 'danger' },
    group_paused: { icon: 'alert', tone: 'danger' },
    session_replaced: { icon: 'doc' },
    preempted: { icon: 'doc' },
  };
  const visual = $derived(
    body.code === 'turn_interrupted'
      ? { icon: 'alert' as const, tone: body.reason === 'stop' ? undefined : ('danger' as const) }
      : (icons[body.code] ?? { icon: 'alert' as const, tone: 'dawn' as const }),
  );

  const turnKeys = {
    crash: 'thread.sys.turn_interrupted.crash',
    timeout: 'thread.sys.turn_interrupted.timeout',
  } as const;

  const notice = $derived.by(() => {
    switch (body.code) {
      case 'task_merged':
        return '';
      case 'task_failed':
        return t(
          body.reason === 'review_rounds'
            ? 'thread.sys.task_failed.review_rounds'
            : 'thread.sys.task_failed.failures',
        ).replace('{task}', taskTag(body.task));
      case 'turn_interrupted': {
        const key =
          body.reason === 'crash' || body.reason === 'timeout'
            ? turnKeys[body.reason]
            : 'thread.sys.turn_interrupted.stop';
        return join(
          t(key).replace('{ai}', who(body.ai)),
          body.task ? taskTag(body.task) : undefined,
          body.detail,
        );
      }
      case 'login_invalid':
        return join(t('thread.sys.login_invalid').replace('{ai}', who(body.ai)), body.detail);
      case 'rate_limited':
        return t('thread.sys.rate_limited')
          .replace('{ai}', who(body.ai))
          .replace('{time}', clock(body.resets, prefs.lang));
      case 'cap_reached':
        return join(
          t(
            body.cap === 'day' ? 'thread.sys.cap_reached.day' : 'thread.sys.cap_reached.task',
          ).replace('{ai}', who(body.ai)),
          body.task ? taskTag(body.task) : undefined,
        );
      case 'session_replaced':
        return join(
          t('thread.sys.session_replaced').replace('{ai}', who(body.ai)),
          body.task ? taskTag(body.task) : undefined,
        );
      case 'rounds_limit':
        return t('thread.sys.rounds_limit')
          .replace('{task}', taskTag(body.task))
          .replace('{n}', String(body.n));
      case 'preempted':
        return t('thread.sys.preempted')
          .replace('{task}', taskTag(body.task))
          .replace('{by}', who(body.by));
      case 'group_paused':
        return t('thread.sys.group_paused').replace('{by}', who(body.by));
      case 'group_resumed':
        return t('thread.sys.group_resumed').replace('{by}', who(body.by));
      case 'no_secretary':
        return t('thread.sys.no_secretary');
    }
  });
</script>

<div class="sys-row">
  <span class="glyph" data-tone={visual.tone}><Icon name={visual.icon} sm /></span>
  <p>
    {#if body.code === 'task_merged'}
      {@const mergedTask = taskRow(body.task)}
      <b>{secName}</b>
      {t('thread.sys.task_merged').replace('{task}', taskTag(body.task))}
      {#if mergedTask}· <span class="title">{mergedTask.title}</span>{/if}
      · <code>{short(body.commit)}</code> → <code>{short(body.merge_commit)}</code>
    {:else}
      {notice}
    {/if}
    · <time datetime={iso}>{time}</time>
  </p>
</div>

<style>
  .sys-row {
    display: grid;
    grid-template-columns: 22px minmax(0, 1fr);
    gap: 2px var(--sp-2);
    align-items: start;
    color: var(--text-3);
    font-size: var(--fs-sm);
  }
  .glyph {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    color: var(--tone-text, var(--text-3));
  }
  b,
  .title {
    color: var(--text-2);
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
    color: var(--text-2);
  }
  time {
    font-size: var(--fs-xs);
  }
</style>
