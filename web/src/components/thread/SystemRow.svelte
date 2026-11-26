<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { secretaryName, systemText } from '../../lib/system';
  import { clock } from '../../lib/when';
  import Icon, { type IconName } from '../Icon.svelte';

  /** Notice written by the home (contracts/messages.md 2.11), localized by code. */
  let { group, msg }: CardProps<'system'> = $props();
  const body = $derived(msg.body);
  const time = $derived(clock(msg.ts, prefs.lang));
  const iso = $derived(new Date(msg.ts).toISOString());

  const icons: Record<string, { icon: IconName; tone?: 'ok' | 'danger' | 'dawn' }> = {
    task_done: { icon: 'check', tone: 'ok' },
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
    {#if body.code === 'task_done'}
      {@const doneTask = group.task(body.task)}
      <!-- Shown under the secretary's name (R-24). -->
      <b>{secretaryName(group)}</b>
      {systemText(group, msg)}
      {#if doneTask}· <span class="title">{doneTask.title}</span>{/if}
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
