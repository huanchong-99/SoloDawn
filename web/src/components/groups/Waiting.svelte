<script lang="ts">
  import { recent, subject } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Icon, { type IconName } from '../Icon.svelte';

  /** `bar` draws the cards as one scrolling row of links, for a narrow page; otherwise as a card
   *  with a line for each. */
  let { bar = false }: { bar?: boolean } = $props();

  const icons: Record<string, IconName> = { brief: 'doc', approval: 'shield', decision: 'fork' };
  type Asked = 'brief' | 'approval' | 'decision';

  // Every card that waits for this person, in every group, the group written to last first.
  const waiting = $derived(
    recent().flatMap((group) =>
      group.waiting.map((msg) => ({ group, msg, kind: msg.kind as Asked })),
    ),
  );
</script>

{#if bar}
  {#if waiting.length}
    <section class="needbar" aria-label={t('groups.waiting')}>
      <b><Icon name="alert" sm />{t('groups.waiting')} · {waiting.length}</b>
      {#each waiting as { group, msg, kind } (msg.id)}
        <a href={`#/groups/${group.g}`}>{t(`groups.waiting.${kind}`)} · {subject(msg)}</a>
      {/each}
    </section>
  {/if}
{:else}
  <section class="digest" aria-labelledby="waiting-title">
    <div class="page-head">
      <div class="grow">
        <h2 id="waiting-title">{t('groups.waiting')}</h2>
        <p>{t('groups.waiting.lead')}</p>
      </div>
    </div>
    {#if waiting.length}
      <ul class="card need todos">
        {#each waiting as { group, msg, kind }, i (msg.id)}
          <li class="todo" style:--i={Math.min(i, 8)}>
            <Icon name={icons[kind]} />
            <div class="grow">
              <b class="cut">{t(`groups.waiting.${kind}`)} · {subject(msg)}</b>
              <p class="muted cut">{group.name}</p>
            </div>
            <a class="btn btn-sm btn-dawn" href={`#/groups/${group.g}`}>
              {t('groups.waiting.open')}
            </a>
          </li>
        {/each}
      </ul>
    {:else}
      <div class="card calm">
        <span class="mark"><Icon name="check" /></span>
        <div>
          <b>{t('groups.waiting.none')}</b>
          <p class="muted">{t('groups.waiting.none.body')}</p>
        </div>
      </div>
    {/if}
  </section>
{/if}

<style>
  .needbar {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    padding: 8px var(--sp-4);
    border-bottom: 1px solid color-mix(in srgb, var(--dawn) 40%, var(--line));
    background: var(--dawn-soft);
    font-size: var(--fs-sm);
    white-space: nowrap;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .needbar b {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--dawn-text);
  }
  .needbar a {
    display: inline-flex;
    align-items: center;
    height: 26px;
    padding: 0 10px;
    border: 1px solid color-mix(in srgb, var(--dawn) 40%, var(--line));
    border-radius: var(--r-full);
    background: var(--surface);
    color: var(--text);
  }
  .digest {
    display: flex;
    flex-direction: column;
    gap: var(--sp-4);
    width: 100%;
    max-width: var(--measure);
    margin-inline: auto;
    padding: var(--sp-6);
  }
  .todos {
    padding: 0;
    gap: 0;
  }
  .todo {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    padding: 12px var(--sp-4);
    animation:
      rise var(--dur-3) var(--ease-out) calc(var(--i) * 45ms) backwards,
      appear var(--dur-2) var(--ease-out) calc(var(--i) * 45ms) backwards;
  }
  .todo + .todo {
    border-top: 1px solid var(--line);
  }
  .todo :global(.i) {
    color: var(--dawn-text);
  }
  .todo p {
    font-size: var(--fs-sm);
  }
  .calm {
    flex-direction: row;
    align-items: center;
    gap: var(--sp-3);
  }
  .calm p {
    font-size: var(--fs-sm);
  }
  .mark {
    display: grid;
    place-items: center;
    flex: none;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--ok-soft);
    color: var(--ok);
  }
</style>
