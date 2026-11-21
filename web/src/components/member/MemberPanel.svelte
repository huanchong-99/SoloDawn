<script lang="ts">
  import { homes, type Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow, HumanMemberRow, MsgRow, TaskRow } from '../../lib/proto';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';
  import AddAI from './AddAI.svelte';

  /** The member column of a group's chat: AI member cards, people, and the add-AI dialog. */
  let { group }: { group: Group } = $props();

  let adding = $state(false);

  const ais = $derived(
    group.members
      .filter((m): m is AIMemberRow => m.kind === 'ai')
      .sort((a, b) => Number(b.secretary) - Number(a.secretary)),
  );
  const humans = $derived(group.members.filter((m): m is HumanMemberRow => m.kind === 'human'));
  const isAdmin = $derived(
    group.me?.kind === 'human' && (group.me.role === 'owner' || group.me.role === 'admin'),
  );
  const canAdd = $derived(
    Boolean(group.info && isAdmin && group.home.ctl && homes.some((h) => h.owner && h.ctl)),
  );

  const humanName = (userId: string | undefined) =>
    humans.find((person) => person.user === userId)?.name;

  const cardOfAI = (msg: MsgRow, ai: AIMemberRow) =>
    msg.by === ai.id || (msg.kind === 'approval' && msg.body.ai === ai.id);

  function taskOf(id: string | undefined): TaskRow | undefined {
    const row = id ? group.rows.get(`task ${id}`) : undefined;
    return row && !('gone' in row) && row.r === 'task' ? row : undefined;
  }

  function taskWaitKind(task: TaskRow): string | undefined {
    if (task.wait?.who?.includes(group.home.me.user)) return task.wait.kind;
    return group.waiting.find(
      (m) =>
        m.thread === task.id ||
        (m.kind === 'approval' && m.body.task === task.id) ||
        (m.kind === 'decision' && m.body.task === task.id),
    )?.kind;
  }

  function waitingOnMe(ai: AIMemberRow, task: TaskRow | undefined): string | undefined {
    const kind = task ? taskWaitKind(task) : undefined;
    if (task && kind) {
      const key =
        kind === 'approval'
          ? 'member.state.needyou.approval'
          : kind === 'decision'
            ? 'member.state.needyou.decision'
            : 'member.state.needyou.task';
      return t(key).replace('{task}', `T-${task.num}`);
    }
    const count = ai.secretary
      ? group.waiting.length
      : group.waiting.filter((m) => cardOfAI(m, ai)).length;
    if (!count) return undefined;
    return count === 1
      ? t('member.state.needyou.one')
      : t('member.state.needyou.many').replace('{n}', String(count));
  }

  function otherWaiter(ai: AIMemberRow, task: TaskRow | undefined): string | undefined {
    if (task?.wait?.who?.[0]) return humanName(task.wait.who[0]);
    for (const row of group.rows.values()) {
      if (row.r !== 'msg' || 'gone' in row || row.state?.status !== 'pending' || !cardOfAI(row, ai))
        continue;
      if (row.kind === 'brief') return group.member(row.body.requester)?.name;
      if (row.kind === 'approval') return humanName(row.body.approvers[0]);
      if (row.kind === 'decision') return humanName(row.body.to[0]);
    }
    return undefined;
  }

  function waitingOnOther(ai: AIMemberRow, task: TaskRow | undefined): string {
    const who = otherWaiter(ai, task);
    if (task && who) {
      const key =
        task.wait?.kind === 'approval'
          ? 'member.state.waiting.approval'
          : 'member.state.waiting.task';
      return t(key).replace('{task}', `T-${task.num}`).replace('{who}', who);
    }
    if (who) return t('member.state.waiting.who').replace('{who}', who);
    return task
      ? `${t('member.state.waiting')} · T-${task.num} ${task.title}`
      : t('member.state.waiting');
  }

  const fixedStates = {
    working: { st: 'working', key: 'member.state.working', withTask: true },
    dormant: { st: 'sleeping', key: 'member.state.sleeping', withTask: false },
    takeover: { st: 'takeover', key: 'member.state.takeover', withTask: true },
    paused: { st: 'paused', key: 'member.state.paused', withTask: true },
    limited: { st: 'limited', key: 'member.state.limited', withTask: false },
  } as const;

  function statusOf(ai: AIMemberRow): { st: string; text: string } {
    const task = taskOf(ai.task);
    if (ai.state === 'waiting') {
      const mine = waitingOnMe(ai, task);
      return mine
        ? { st: 'needyou', text: mine }
        : { st: 'waiting', text: waitingOnOther(ai, task) };
    }
    const entry = fixedStates[ai.state as keyof typeof fixedStates];
    if (!entry) return { st: 'offline', text: t('member.state.offline') };
    const base = t(entry.key);
    return {
      st: entry.st,
      text: entry.withTask && task ? `${base} · T-${task.num} ${task.title}` : base,
    };
  }

  function pathLabel(ai: AIMemberRow): string {
    if (ai.state === 'offline') return t('member.path.offline');
    if (ai.path === 'local' || (!ai.path && group.info?.home === ai.host))
      return t('member.path.local');
    if (ai.path === 'direct' || ai.path === 'relay') return t(`member.path.${ai.path}`);
    return ai.path;
  }

  const accountTitle = (account: string) =>
    account === 'personal' || account === 'team' || account === 'api_key'
      ? t(`member.account.${account}`)
      : account;

  function personTags(person: HumanMemberRow): string {
    const tags = person.id === group.me?.id ? [t('chat.you')] : [];
    if (person.role === 'owner') tags.push(t('member.owner'));
    else if (person.role === 'admin') tags.push(t('member.admin'));
    return tags.join(' · ');
  }
</script>

<div class="members">
  <div class="row">
    <h2 class="sec-title grow">{t('member.ai')} · {ais.length}</h2>
    <button class="btn btn-sm" onclick={() => (adding = true)} disabled={!canAdd}>
      <Icon name="plus" sm /><span>{t('member.add')}</span>
    </button>
  </div>

  {#each ais as ai (ai.id)}
    {@const status = statusOf(ai)}
    {@const owner = ai.user === group.home.me.user ? undefined : humanName(ai.user)}
    <section class="card member">
      <header class="row">
        <span
          class="av-wrap"
          class:sec={ai.secretary}
          data-sec={t('member.sec.mark')}
          data-st={status.st}
          aria-hidden="true"
        >
          <Avatar id={ai.id} name={ai.name} cli={ai.cli} />
        </span>
        <div class="grow">
          <h3>
            {ai.name}{#if owner}{' '}<span class="owner">· {owner}</span>{/if}
          </h3>
          <p class="st" data-st={status.st}><i></i><span>{status.text}</span></p>
        </div>
      </header>
      <dl class="kv">
        <dt>{t('member.computer')}</dt>
        <dd>{ai.host_name} <span class="muted">· {pathLabel(ai)}</span></dd>
        <dt>{t('member.account')}</dt>
        <dd>
          {accountTitle(ai.account)}{#if ai.account === 'personal'}{' '}<span class="muted">
              · {t('member.account.personal.note')}
            </span>{/if}
        </dd>
      </dl>
    </section>
  {:else}
    <p class="hint">{t('member.ai.empty')}</p>
  {/each}

  <h2 class="sec-title people-head">{t('member.people')} · {humans.length}</h2>
  <ul class="people">
    {#each humans as person (person.id)}
      {@const tags = personTags(person)}
      <li class="row person">
        <Avatar id={person.user} name={person.name} />
        <span class="grow cut">
          {person.name}{#if tags}{' '}<span class="muted">· {tags}</span>{/if}
        </span>
        <span class="st" data-st={person.online ? 'ok' : 'offline'}>
          <i></i>{t(person.online ? 'member.online' : 'member.offline')}
        </span>
      </li>
    {/each}
  </ul>
</div>

{#if adding}
  <AddAI {group} bind:open={adding} />
{/if}

<style>
  .members {
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
    padding: var(--sp-4);
  }
  .member header {
    flex-wrap: nowrap;
    align-items: flex-start;
  }
  .owner {
    color: var(--text-3);
    font-weight: 400;
  }
  .av-wrap {
    position: relative;
    display: inline-grid;
    flex: none;
  }
  .av-wrap :global(.av) {
    width: 40px;
    height: 40px;
    border-radius: 11px;
    font-size: 15px;
  }
  .av-wrap.sec::before {
    content: attr(data-sec);
    position: absolute;
    top: -5px;
    right: -5px;
    z-index: 1;
    display: grid;
    place-items: center;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    border: 2px solid var(--surface);
    background: var(--text);
    color: var(--bg);
    font-size: 10px;
    font-weight: 700;
    line-height: 1;
  }
  .av-wrap[data-st]::after {
    content: '';
    position: absolute;
    right: -3px;
    bottom: -3px;
    width: 11px;
    height: 11px;
    border-radius: 50%;
    border: 2px solid var(--surface);
    background: var(--text-3);
  }
  .av-wrap[data-st='working']::after {
    background: var(--sky);
  }
  .av-wrap[data-st='needyou']::after,
  .av-wrap[data-st='limited']::after {
    background: var(--dawn);
  }
  .av-wrap[data-st='waiting']::after {
    background: var(--surface);
    box-shadow: inset 0 0 0 2px var(--text-3);
  }
  .av-wrap[data-st='sleeping']::after {
    background: var(--surface-3);
  }
  .av-wrap[data-st='takeover']::after {
    background: var(--text);
  }
  /* Shape and colour change together so state is never carried by colour alone. */
  .st {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  p.st {
    display: flex;
    margin-top: 2px;
  }
  .st i {
    position: relative;
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    color: var(--text-3);
    background: currentColor;
  }
  .st[data-st='working'] i {
    color: var(--sky);
  }
  .st[data-st='working'] i::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background: currentColor;
    animation: pulse 2s var(--ease) infinite;
  }
  .st[data-st='needyou'] {
    color: var(--dawn-text);
    font-weight: 500;
  }
  .st[data-st='needyou'] i {
    color: var(--dawn);
  }
  .st[data-st='waiting'] i {
    background: none;
    border: 2px solid currentColor;
  }
  .st[data-st='sleeping'] {
    color: var(--text-3);
  }
  .st[data-st='sleeping'] i {
    width: 10px;
    height: 10px;
    background: none;
    box-shadow: inset -3px -2px 0 0 currentColor;
  }
  .st[data-st='offline'] {
    color: var(--text-3);
  }
  .st[data-st='offline'] i {
    background: none;
    border: 1.5px dashed currentColor;
  }
  .st[data-st='paused'] i {
    border-radius: 1px;
    background: linear-gradient(90deg, currentColor 34%, transparent 34% 66%, currentColor 66%);
  }
  .st[data-st='takeover'] {
    color: var(--text);
    font-weight: 500;
  }
  .st[data-st='takeover'] i {
    border-radius: 2px;
    color: var(--text);
  }
  .st[data-st='ok'] i {
    color: var(--ok);
  }
  .st[data-st='limited'] i {
    color: var(--dawn);
    border-radius: 2px;
    transform: rotate(45deg);
  }
  @keyframes pulse {
    0% {
      opacity: 0.55;
      transform: scale(1);
    }
    70%,
    100% {
      opacity: 0;
      transform: scale(2.5);
    }
  }
  .kv {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 6px var(--sp-4);
    font-size: var(--fs-sm);
  }
  .kv dt {
    color: var(--text-3);
  }
  .kv dd {
    min-width: 0;
  }
  .people-head {
    margin-top: var(--sp-2);
  }
  .people {
    display: flex;
    flex-direction: column;
  }
  .person {
    flex-wrap: nowrap;
    padding: 6px 0;
  }
</style>
