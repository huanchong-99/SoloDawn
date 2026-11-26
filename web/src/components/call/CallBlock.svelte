<script lang="ts">
  import { callView } from '../../lib/call';
  import type { CardProps } from '../../lib/mounts.svelte';
  import Avatar from '../Avatar.svelte';
  import Icon from '../Icon.svelte';
  import Rich from '../Rich.svelte';

  /** An AI-to-AI call block with route, question, waiting or elapsed time, and inline reply. */
  let { group, msg }: CardProps<'call'> = $props();

  const view = $derived(callView(group, msg));
</script>

<div class="call">
  <p class="call-route">
    <span>{view.caller}</span><Icon name="arrow" sm /><span class="sr">→</span><span>
      {view.callee}
    </span><span class="muted">
      {view.mode} ·
      {#if view.status === 'waiting'}
        <span class="st" data-st="waiting"><i></i>{view.timing}</span>
      {:else if view.status === 'denied'}
        <span class="badge" data-tone="danger" data-st="denied">{view.timing}</span>
      {:else}
        {view.timing}
      {/if}
      · <span class="depth">{view.depth}</span>
    </span>
  </p>

  {#if msg.text}
    <p class="text"><Rich {group} text={msg.text} mentions={msg.mentions} /></p>
  {/if}

  {#if view.reply}
    <div class="reply">
      <Avatar id={view.reply.by} name={view.reply.name} cli={view.reply.cli} size="sm" />
      <p class="text">
        <b>{view.reply.name}</b>{' '}<Rich
          {group}
          text={view.reply.text}
          mentions={view.reply.mentions}
        />
      </p>
    </div>
  {/if}
</div>

<style>
  /* One AI asks another: an inset block, with who asked whom on its first line. */
  .call {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px var(--sp-3);
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
    line-height: 1.5;
    min-width: 0;
  }
  .call-route {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    font-weight: 500;
  }
  .call-route :global(.i) {
    color: var(--text-3);
  }
  .call-route .muted {
    font-weight: 400;
    font-size: var(--fs-xs);
  }
  .call-route .st {
    color: inherit;
    font-size: inherit;
    font-weight: inherit;
    vertical-align: baseline;
  }
  .call-route .badge {
    background: var(--surface-3);
  }
  .depth {
    white-space: nowrap;
  }
  .text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .text :global(code) {
    background: var(--surface-3);
  }
  .reply {
    display: grid;
    grid-template-columns: 20px minmax(0, 1fr);
    gap: var(--sp-2);
    padding-top: var(--sp-2);
    border-top: 1px solid var(--line);
  }
  .reply b {
    font-weight: 500;
  }
  .reply > :global(.av) {
    margin-top: 1px;
  }
</style>
