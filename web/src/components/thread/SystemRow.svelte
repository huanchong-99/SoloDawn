<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { clock, clockOn } from '../../lib/when';
  import Icon, { type IconName } from '../Icon.svelte';

  /** Notice written by the home (contracts/messages.md 2.11), localized by code. */
  let { group, msg }: CardProps<'system'> = $props();
  const body = $derived(msg.body);
  const time = $derived(clock(msg.ts, prefs.lang));
  const iso = $derived(new Date(msg.ts).toISOString());

  const short = (sha: string) => sha.slice(0, 7);
  const who = (id: string) => group.member(id)?.name ?? t('chat.gone');
  const taskTag = (id: string) => {
    const row = group.task(id);
    return row ? `T-${row.num}` : short(id);
  };
  const join = (...items: (string | undefined)[]) => items.filter(Boolean).join(' · ');

  const secretary = $derived(
    group.info?.secretary ? group.member(group.info.secretary) : undefined,
  );
  const secName = $derived(secretary?.name ?? t('chat.secretary'));

  const icons: Record<string, { icon: IconName; tone?: 'ok' | 'danger' | 'dawn' }> = {
    task_merged: { icon: 'check', tone: 'ok' },
    task_failed: { icon: 'x', tone: 'danger' },
    login_invalid: { icon: 'alert', tone: 'danger' },
    session_replaced: { icon: 'doc' },
    member_joined: { icon: 'users' },
    member_removed: { icon: 'users' },
    rate_limited: { icon: 'clock' },
    preempted: { icon: 'clock' },
    group_paused: { icon: 'alert' },
    group_resumed: { icon: 'arrow' },
  };
  // Warm only while a person's reply is what the task is waiting for.
  const roundsTone = (task: string) =>
    group.task(task)?.wait?.kind === 'rounds' ? ('dawn' as const) : undefined;
  const visual = $derived.by(() => {
    if (body.code === 'turn_interrupted')
      return {
        icon: 'alert' as const,
        tone: body.reason === 'stop' ? undefined : ('danger' as const),
      };
    if (body.code === 'rounds_limit')
      return { icon: 'alert' as const, tone: roundsTone(body.task) };
    return icons[body.code] ?? { icon: 'alert' as const, tone: 'dawn' as const };
  });

  const turnKeys = {
    crash: 'thread.sys.turn_interrupted.crash',
    timeout: 'thread.sys.turn_interrupted.timeout',
  } as const;

  function removed(name: string, by?: string) {
    if (by === undefined) return t('thread.sys.member_left').replace('{who}', () => name);
    return t('thread.sys.member_removed')
      .replace('{by}', () => who(by))
      .replace('{who}', () => name);
  }

  const notice = $derived.by(() => {
    switch (body.code) {
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
          .replace('{by}', () => who(body.by));
      case 'rate_limited':
        return t('thread.sys.rate_limited')
          .replace('{time}', clockOn(body.resets, msg.ts, prefs.lang))
          .replace('{ai}', () => who(body.ai));
      case 'group_paused':
        return t('thread.sys.group_paused').replace('{by}', () => who(body.by));
      case 'group_resumed':
        return t('thread.sys.group_resumed').replace('{by}', () => who(body.by));
      case 'member_joined':
        return t('thread.sys.member_joined')
          .replace('{by}', () => who(body.by))
          .replace('{who}', () => who(body.member));
      case 'member_removed':
        return removed(body.name, body.by);
      case 'no_secretary':
        return t('thread.sys.no_secretary');
      default:
        return body.code;
    }
  });
</script>

<div class="sys-row">
  <span class="glyph" data-tone={visual.tone}><Icon name={visual.icon} sm /></span>
  <p>
    {#if body.code === 'task_merged'}
      {@const mergedTask = group.task(body.task)}
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
  b,
  .title {
    color: var(--text-2);
  }
</style>
