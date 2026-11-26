<script lang="ts">
  import { recent, subject } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Icon, { type IconName } from '../Icon.svelte';

  /** `bar` draws the cards as one scrolling row of links, for a narrow page; otherwise as a card
   *  with a line for each. */
  let { bar = false }: { bar?: boolean } = $props();

  type Asked = 'brief' | 'approval' | 'decision' | 'confirm';
  const icons: Record<Asked, IconName> = {
    brief: 'doc',
    approval: 'shield',
    decision: 'fork',
    confirm: 'shield',
  };

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
        <a href={`#/groups/${group.g}`}>{t(`groups.waiting.${kind}`)} · {subject(group, msg)}</a>
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
      <ul class="card todos">
        {#each waiting as { group, msg, kind }, i (msg.id)}
          <li class="todo" style:--i={Math.min(i, 8)}>
            <Icon name={icons[kind]} />
            <div class="grow">
              <b class="cut">{t(`groups.waiting.${kind}`)} · {subject(group, msg)}</b>
              <p class="muted cut">{group.name}</p>
            </div>
            <a
              class="btn btn-sm btn-dawn"
              href={`#/groups/${group.g}`}
              aria-label={`${t('groups.waiting.open')} · ${t(`groups.waiting.${kind}`)} · ${subject(group, msg)} · ${group.name}`}
            >
              {t('groups.waiting.open')}
            </a>
          </li>
        {/each}
      </ul>
    {:else}
      <div class="calm blank">
        <span class="blank-mark"><Icon name="check" /></span>
        <b>{t('groups.waiting.none')}</b>
        <p>{t('groups.waiting.none.body')}</p>
      </div>
    {/if}
  </section>
{/if}

<style>
  .digest {
    display: flex;
    flex-direction: column;
    gap: var(--sp-4);
    width: 100%;
    max-width: calc(640px + 2 * var(--sp-6));
    margin-inline: auto;
    padding: var(--sp-8) var(--sp-6);
  }
  /* What waits, as rows on one card: the kind's glyph, the sentence, the group, the way there. */
  .todos {
    padding: 0;
    gap: 0;
  }
  .todo {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    min-height: 60px;
    padding: 10px var(--sp-4);
    animation:
      rise var(--dur-3) var(--ease-out) calc(var(--i) * var(--stagger)) backwards,
      appear var(--dur-2) var(--ease-out) calc(var(--i) * var(--stagger)) backwards;
  }
  .todo + .todo::before {
    content: '';
    position: absolute;
    inset: 0 0 auto var(--sp-4);
    border-top: 1px solid var(--line);
  }
  .todo :global(.i) {
    color: var(--text-2);
  }
  .todo b {
    display: block;
    font-size: var(--fs-ui);
    font-weight: 500;
    line-height: 1.45;
  }
  .todo p {
    font-size: var(--fs-xs);
    line-height: 1.4;
  }
  .calm {
    margin-top: var(--sp-8);
  }
</style>
