<script lang="ts">
  import { untrack } from 'svelte';
  import { announcement, prefs, t } from '../../lib/prefs.svelte';
  import type { HostPermitRequest } from '../../lib/proto';
  import Icon from '../Icon.svelte';
  import AlertSketch from './AlertSketch.svelte';
  import DragSketch from './DragSketch.svelte';
  import {
    needs,
    rows,
    type CoachButton,
    type Opened,
    type Permissions,
    type PermitAsk,
    type PermitRow,
    type RowId,
  } from './permit.svelte.ts';

  let {
    name,
    os,
    relayed,
    permissions,
    ask,
    plain = false,
    open = false,
  }: {
    name: string;
    os: string;
    relayed: boolean;
    permissions: Permissions | undefined;
    ask: PermitAsk;
    plain?: boolean;
    open?: boolean;
  } = $props();

  const uid = $props.id();

  const TITLE_KEY: Record<RowId, 'permit.lan' | 'permit.in' | 'permit.files'> = {
    local_network: 'permit.lan',
    incoming: 'permit.in',
    full_disk: 'permit.files',
  };

  let opened = $state<Opened>({});
  let expanded = $state(false);
  let replay = $state(0);
  let prev: Permissions | undefined;

  const active = $derived(Boolean(permissions));
  $effect(() => {
    if (!active) return;
    untrack(() => probe());
  });

  $effect(() => {
    const next = permissions;
    if (!next) {
      prev = undefined;
      return;
    }
    untrack(() => {
      if (!prev) {
        if (open && !next.local_network) opened = { local_network: 'ask' };
        if (needs(next)) expanded = true;
        prev = next;
        return;
      }
      for (const key of ['local_network', 'incoming', 'full_disk'] as const) {
        if (!prev[key] && next[key]) {
          if (opened[key]) opened = { ...opened, [key]: undefined };
          announcement.text = t('permit.done').replace('{what}', () => t(TITLE_KEY[key]));
        }
      }
      if (needs(next)) expanded = true;
      prev = next;
    });
  });

  function probe(body: Omit<HostPermitRequest, 't' | 'id'> = {}) {
    void ask(body);
  }

  function act(row: PermitRow) {
    if (row.id === 'local_network') {
      opened = { ...opened, local_network: 'ask' };
      probe();
    } else if (row.id === 'incoming') {
      opened = { ...opened, incoming: true };
      probe({ open: 'incoming' });
    } else {
      opened = { ...opened, full_disk: true };
      probe({ open: 'full_disk', lang: prefs.lang });
    }
  }

  function coachAct(row: PermitRow, action: CoachButton['action']) {
    if (action === 'nowindow') {
      opened = { ...opened, local_network: 'settings' };
      probe({ open: 'local_network' });
    } else if (action === 'again') {
      probe();
    } else if (row.id === 'full_disk') {
      opened = { ...opened, full_disk: false };
    }
  }

  const need = $derived(permissions ? needs(permissions) : false);
  const list = $derived(permissions ? rows(permissions, opened) : []);
  const showBody = $derived(!plain || need || expanded);
  const winRelayed = $derived(!permissions && os.startsWith('windows/') && relayed);
</script>

{#if permissions}
  {#snippet content()}
    <div class="perm-head">
      {#if !showBody}
        <span id={uid} class="perm-summary">
          {t('permit.title.short')} · {t('permit.badge.ready')}
        </span>
      {:else}
        {#if plain}
          <h3 id={uid} class="perm-title">{t('permit.title.short')}</h3>
        {:else}
          <h2 id={uid} class="perm-title">{t('permit.title')}</h2>
        {/if}
        <span class="badge" data-tone={need ? 'dawn' : 'ok'}>
          {#key need}
            <span class="swap">{t(need ? 'permit.badge.need' : 'permit.badge.ready')}</span>
          {/key}
        </span>
      {/if}
      {#if plain && !need}
        <button
          type="button"
          class="btn btn-sm btn-ghost turn"
          aria-expanded={showBody}
          aria-label={t('permit.title.short')}
          onclick={() => (expanded = !expanded)}
        >
          <Icon name="down" sm />
        </button>
      {/if}
    </div>

    {#if showBody}
      <ul class="perm-rows">
        {#each list as row (row.id)}
          <li>
            {#if row.ok}
              <span class="mark ok" aria-hidden="true"><Icon name="check" /></span>
            {:else}
              <span
                class="st mark"
                data-st={row.mark === 'need' ? 'needyou' : 'waiting'}
                aria-hidden="true"
              >
                <i></i>
              </span>
            {/if}
            <div class="perm-text">
              <b>{t(row.title)}</b>
              {#key row.line}
                <span class="perm-line">{t(row.line)}</span>
              {/key}
              {#if row.hint}
                <span class="perm-hint">{t(row.hint)}</span>
              {/if}
            </div>
            {#if row.action}
              <button
                type="button"
                class="btn btn-sm act"
                class:btn-primary={row.action.primary}
                onclick={() => act(row)}
              >
                {t(row.action.label)}
              </button>
            {:else}
              <span></span>
            {/if}
            {#if row.coach}
              <div
                class="coach"
                class:plain={!row.coach.sketch}
                role="group"
                onpointerenter={() => replay++}
                onfocusin={() => replay++}
              >
                {#if row.coach.sketch === 'alert'}
                  <AlertSketch />
                {:else if row.coach.sketch === 'drag'}
                  {#key replay}<DragSketch />{/key}
                {/if}
                <div class="coach-body">
                  <p role="status">{t(row.coach.line).replace('{name}', () => name)}</p>
                  {#if row.coach.more}
                    <p class="more">{t(row.coach.more)}</p>
                  {/if}
                  <div class="acts">
                    {#each row.coach.buttons as button (button.action)}
                      <button
                        type="button"
                        class="btn btn-sm"
                        class:btn-link={button.variant === 'link'}
                        class:btn-ghost={button.variant === 'ghost'}
                        onclick={() => coachAct(row, button.action)}
                      >
                        {t(button.label)}
                      </button>
                    {/each}
                  </div>
                </div>
              </div>
            {/if}
          </li>
        {/each}
      </ul>
      <div class="foot">
        {#if !plain}<p>{t('permit.foot.login')}</p>{/if}
        <p>{t('permit.foot.update')}</p>
      </div>
    {/if}
  {/snippet}

  {#if plain}
    <section class="perm-plain" aria-labelledby={uid}>
      {@render content()}
    </section>
  {:else}
    <section class="card perm-card" class:need aria-labelledby={uid}>
      {@render content()}
    </section>
  {/if}
{:else if winRelayed}
  {#if plain}
    <p class="win-relay plain-blk">{t('permit.win.relay')}</p>
  {:else}
    <section class="card perm-card">
      <p class="win-relay">{t('permit.win.relay')}</p>
    </section>
  {/if}
{/if}

<style>
  .perm-card {
    container-type: inline-size;
  }
  .perm-plain {
    border-top: 1px solid var(--line);
    padding: var(--sp-3) var(--sp-4);
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
    container-type: inline-size;
    min-width: 0;
  }
  .perm-head {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    min-height: 22px;
  }
  .perm-head .perm-title,
  .perm-summary {
    flex: 1;
    min-width: 0;
    font-size: var(--fs-ui);
    line-height: 1.4;
  }
  .perm-head .perm-title {
    font-weight: 600;
    color: var(--text);
  }
  .perm-summary {
    font-weight: 500;
    color: var(--text-2);
  }
  .turn {
    margin-right: -6px;
  }
  .turn :global(.i) {
    transition: transform var(--dur-2) var(--ease);
  }
  .turn[aria-expanded='true'] :global(.i) {
    transform: rotate(180deg);
  }
  .perm-rows > li {
    display: grid;
    grid-template-columns: 20px minmax(0, 1fr) auto;
    gap: 4px 10px;
    padding: 12px 0;
    align-items: start;
  }
  .perm-rows > li + li {
    border-top: 1px solid var(--line);
  }
  .perm-rows > li:first-child {
    padding-top: 0;
  }
  .perm-rows > li:last-child {
    padding-bottom: 0;
  }
  .perm-text {
    min-width: 0;
  }
  .perm-rows b {
    display: block;
    font-size: var(--fs-ui);
    font-weight: 500;
    line-height: 20px;
  }
  .perm-line {
    display: block;
    font-size: var(--fs-xs);
    line-height: 1.5;
    color: var(--text-2);
  }
  .perm-hint {
    display: block;
    font-size: var(--fs-xs);
    line-height: 1.5;
    color: var(--text-3);
    margin-top: 2px;
  }
  .act {
    align-self: center;
  }
  .mark {
    width: 20px;
    height: 20px;
    display: grid;
    place-items: center;
  }
  .mark.ok {
    color: var(--ok);
  }
  .st.mark {
    justify-content: center;
    gap: 0;
  }
  .st.mark[data-st='needyou'] i {
    width: 10px;
    height: 10px;
    background: none;
    border: 2px solid var(--dawn);
  }
  .st.mark[data-st='waiting'] i {
    width: 10px;
    height: 10px;
    border-width: 1.5px;
    border-color: var(--text-3);
  }
  .coach {
    grid-column: 1 / -1;
    margin-top: 8px;
    background: var(--surface-2);
    border-radius: var(--r-md);
    padding: 12px;
    display: grid;
    grid-template-columns: 120px minmax(0, 1fr);
    gap: 12px;
    align-items: center;
  }
  .coach.plain {
    grid-template-columns: minmax(0, 1fr);
  }
  .coach-body {
    width: 100%;
    min-width: 0;
  }
  .coach p {
    font-size: var(--fs-sm);
    line-height: 1.55;
  }
  .coach .more {
    font-size: var(--fs-xs);
    color: var(--text-2);
    margin-top: 4px;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: 2px;
    margin: 6px 0 0 -6px;
  }
  .acts :global(.btn-link) {
    --hover: var(--sky-soft);
    border-color: transparent;
    background: none;
    box-shadow: none;
    color: var(--sky);
    padding: 0 6px;
  }
  .foot {
    border-top: 1px solid var(--line);
    padding-top: 10px;
    display: grid;
    gap: 2px;
  }
  .foot p,
  .win-relay {
    font-size: var(--fs-xs);
    color: var(--text-3);
    line-height: 1.5;
  }
  .win-relay.plain-blk {
    border-top: 1px solid var(--line);
    padding: var(--sp-3) var(--sp-4);
  }
  @media (prefers-reduced-motion: no-preference) {
    .mark.ok {
      animation: tick-land var(--dur-4) var(--spring) both;
    }
    .perm-line {
      animation: line-appear var(--dur-2) var(--ease-out);
    }
    .coach {
      animation: coach-appear var(--dur-2) var(--ease-out) both;
    }
  }
  @container (max-width: 520px) {
    .coach {
      grid-template-columns: minmax(0, 1fr);
      justify-items: start;
    }
  }
  @media (max-width: 520px) {
    .perm-rows > li {
      grid-template-columns: 20px minmax(0, 1fr);
    }
    .perm-rows .act {
      grid-column: 2;
      justify-self: stretch;
      margin-top: 6px;
      min-height: 44px;
      height: auto;
      white-space: normal;
      text-align: center;
    }
    .acts {
      margin-left: 0;
      gap: 6px;
    }
    .acts button,
    .turn {
      min-height: 44px;
      height: auto;
      white-space: normal;
      text-align: center;
    }
    .turn {
      margin-right: 0;
      min-width: 44px;
    }
  }
  @keyframes tick-land {
    from {
      opacity: 0;
      transform: scale(0.6);
    }
  }
  @keyframes line-appear {
    from {
      opacity: 0;
    }
  }
  @keyframes coach-appear {
    from {
      opacity: 0;
      transform: translateY(var(--rise));
    }
  }
</style>
