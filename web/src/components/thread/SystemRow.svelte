<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { systemText, taskTag } from '../../lib/system';
  import { clock } from '../../lib/when';
  import Icon, { type IconName } from '../Icon.svelte';

  /** Notice written by the home (contracts/messages.md 2.11), localized by code. */
  let { group, msg }: CardProps<'system'> = $props();
  const body = $derived(msg.body);
  const time = $derived(clock(msg.ts, prefs.lang));
  const iso = $derived(new Date(msg.ts).toISOString());

  const short = (sha: string) => sha.slice(0, 7);
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
</script>

<div class="sys-row">
  <span class="glyph" data-tone={visual.tone}><Icon name={visual.icon} sm /></span>
  <p>
    {#if body.code === 'task_merged'}
      {@const mergedTask = group.task(body.task)}
      <b>{secName}</b>
      {t('thread.sys.task_merged').replace('{task}', taskTag(group, body.task))}
      {#if mergedTask}· <span class="title">{mergedTask.title}</span>{/if}
      · <code>{short(body.commit)}</code> → <code>{short(body.merge_commit)}</code>
    {:else}
      {systemText(group, msg)}
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
