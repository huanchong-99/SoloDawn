<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { terminalDisabledReason } from '../../lib/members';
  import { t } from '../../lib/prefs.svelte';
  import type { TaskRow } from '../../lib/proto';
  import { href } from '../../lib/route.svelte';
  import { ownTerminals, taskOps, type TaskOp } from '../../lib/taskops';
  import { hostHomeOf } from '../../lib/terminal.svelte';
  import { offersTerminal } from '../driver/driver';
  import Icon, { type IconName } from '../Icon.svelte';
  import { netHere } from '../net';

  /** The button in the head of a task card, and its menu: what the reader may do to the task, and
   *  the terminals of their own AIs on it. Without either there is no button. */
  let { group, task, onpick }: { group: Group; task: TaskRow; onpick: (op: TaskOp) => void } =
    $props();
  const uid = $props.id();
  const net = netHere();

  let button = $state<HTMLButtonElement>();
  let menu = $state<HTMLElement>();

  const icons: Record<TaskOp, IconName> = {
    pause: 'pause',
    resume: 'play',
    assign: 'users',
    cancel: 'x',
  };

  const ops = $derived(taskOps(group, task));
  const terminals = $derived(
    net
      ? ownTerminals(group, task)
          .filter((ai) => offersTerminal(hostHomeOf(net.homes, ai.host), ai.cli))
          .map((ai) => ({ ai, why: terminalDisabledReason(net, ai) }))
      : [],
  );
  const name = $derived(t('task.menu').replace('{task}', `T-${task.num}`));

  /** Under the button, or over it in the lower half of the screen. */
  function place(event: ToggleEvent) {
    if (event.newState !== 'open') return;
    const { top, bottom, right } = button!.getBoundingClientRect();
    const up = bottom > innerHeight / 2;
    menu!.style.right = `${Math.max(8, innerWidth - right)}px`;
    menu!.style.top = up ? 'auto' : `${bottom + 4}px`;
    menu!.style.bottom = up ? `${innerHeight - top + 4}px` : 'auto';
  }

  /** It stands still over the page, so it goes when what it belongs to moves. */
  function follow(event: ToggleEvent) {
    const move = event.newState === 'open' ? addEventListener : removeEventListener;
    move('scroll', hide, true);
  }
  const hide = () => menu?.hidePopover();
</script>

{#if ops.length || terminals.length}
  <button
    type="button"
    class="icon-btn more"
    bind:this={button}
    popovertarget={uid}
    aria-label={name}
  >
    <Icon name="more" />
  </button>
  <div
    class="menu"
    id={uid}
    popover="auto"
    role="group"
    aria-label={name}
    bind:this={menu}
    onbeforetoggle={place}
    ontoggle={follow}
  >
    {#each terminals as { ai, why } (ai.id)}
      {@const label = t('task.op.terminal').replace('{ai}', () => ai.name)}
      {#if why}
        <span class="item off">
          <Icon name="terminal" sm /><span class="grow">{label}<small>{why}</small></span>
        </span>
      {:else}
        <a class="item" href={href('terminal', { g: group.g, member: ai.id, task: task.id })}>
          <Icon name="terminal" sm /><span class="grow">{label}</span>
        </a>
      {/if}
    {/each}
    {#each ops as op (op)}
      <button
        type="button"
        class="item"
        class:danger={op === 'cancel'}
        disabled={!group.home.ctl}
        popovertarget={uid}
        popovertargetaction="hide"
        onclick={() => onpick(op)}
      >
        <Icon name={icons[op]} sm /><span class="grow">{t(`task.op.${op}`)}</span>
      </button>
    {/each}
  </div>
{/if}

<style>
  .more {
    margin: -7px -8px -7px 0;
  }
  .menu {
    position: fixed;
    inset: auto;
    min-width: 176px;
    max-width: min(300px, calc(100vw - 16px));
    margin: 0;
    padding: 6px;
    border: 0;
    border-radius: var(--r-lg);
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow-pop);
    font-size: var(--fs-sm);
    outline: none;
  }
  .menu:popover-open {
    display: grid;
    gap: 2px;
    animation:
      rise var(--dur-2) var(--ease-out),
      appear var(--dur-1) var(--ease-out);
  }
  .item {
    position: relative;
    isolation: isolate;
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    min-height: 36px;
    padding: 0 10px;
    border: 0;
    border-radius: var(--r-md);
    background: none;
    color: var(--text);
    text-align: start;
    text-decoration: none;
  }
  .item :global(.i) {
    color: var(--text-3);
  }
  .item::before {
    content: '';
    position: absolute;
    inset: 0;
    z-index: -1;
    border-radius: inherit;
    background: var(--surface-2);
    opacity: 0;
    transition: opacity var(--dur-1) var(--ease);
  }
  .item:not(.off, [disabled]):is(:focus-visible, :active)::before {
    opacity: 1;
  }
  @media (hover: hover) {
    .item:not(.off, [disabled]):hover::before {
      opacity: 1;
    }
  }
  .off,
  .item[disabled] {
    color: var(--text-3);
    cursor: not-allowed;
  }
  .off {
    padding-block: 6px;
    line-height: 1.4;
  }
  small {
    display: block;
    font-size: var(--fs-xs);
  }
  /* What ends the task stands apart from the rest. */
  .danger,
  .danger :global(.i) {
    color: var(--danger);
  }
  * + .danger {
    margin-top: 5px;
  }
  * + .danger::after {
    content: '';
    position: absolute;
    inset: -4px 4px auto;
    height: 1px;
    background: var(--line);
  }
  @media (pointer: coarse) {
    .more {
      margin-block: -11px;
    }
    .item {
      min-height: 44px;
    }
  }
</style>
