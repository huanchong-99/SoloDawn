<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { isAdmin } from '../../lib/members';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { setPaused } from '../../lib/stop';
  import { clockOn, now } from '../../lib/when';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** Above the composer while the group is paused: who paused it, what that means for a message
   *  written now, and the way back for who may resume it. */
  let { group }: { group: Group } = $props();

  let busy = $state(false);
  let failed = $state('');

  const paused = $derived(group.info?.paused);
  const admin = $derived(isAdmin(group));

  async function resume() {
    busy = true;
    failed = '';
    failed = (await setPaused(group, false)).err?.code ?? '';
    busy = false;
    if (!failed) document.querySelector<HTMLElement>('.dock textarea')?.focus();
  }
</script>

{#if paused}
  {@const time = clockOn(paused.at, now(), prefs.lang)}
  <div class="paused">
    <div class="in">
      <span class="glyph"><Icon name="pause" /></span>
      <div class="grow" role="status">
        <b>
          {paused.by === group.me?.id
            ? t('stop.paused.you').replace('{time}', time)
            : t('stop.paused.by')
                .replace('{time}', time)
                .replace('{who}', () => group.member(paused.by)?.name ?? t('chat.gone'))}
        </b>
        <p>{t(admin ? 'stop.paused.body' : 'stop.paused.admin')}</p>
      </div>
      {#if admin}
        <button
          type="button"
          class="btn btn-sm"
          disabled={busy || !group.home.ctl}
          aria-busy={busy}
          onclick={() => void resume()}
        >
          <Icon name="play" sm />{t('stop.resume')}
        </button>
      {/if}
    </div>
    {#if failed}<Failure code={failed} />{/if}
  </div>
{/if}

<style>
  .paused {
    padding: 10px var(--sp-4);
    border-top: 1px solid color-mix(in srgb, var(--danger) 30%, var(--line));
    background: var(--danger-soft);
    font-size: var(--fs-sm);
    animation:
      rise var(--dur-3) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .paused > :global(*) {
    max-width: var(--measure);
    margin-inline: auto;
  }
  .in {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
  }
  .glyph {
    display: grid;
    flex: none;
    color: var(--danger);
  }
  p {
    color: var(--text-2);
    line-height: 1.5;
  }
</style>
