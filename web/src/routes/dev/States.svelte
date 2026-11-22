<script lang="ts">
  // The design's seventh page, the design system and its states (web/design/components.html), as
  // a development server shows it: every message of the node's wire examples as the chat draws
  // it, through the card that an area mounts for its kind or the one-line stub, and what each
  // mounting point holds. It reads the examples, so only a development server loads it.
  import Message from '../../components/chat/Message.svelte';
  import PresetStates from '../../components/preset/States.svelte';
  import { Group, Home } from '../../lib/groups.svelte';
  import { goldens } from '../../lib/mock';
  import { cards, mounts, points } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { decode, MsgRow, Row } from '../../lib/proto';

  const row = (name: string, changes: object = {}) =>
    ({ ...decode(Row, goldens[name]), ...changes }) as Row;

  // The group of the examples, read by its owner, with the members that wrote its messages: the
  // examples name a secretary and a reviewer that have no row of their own.
  const owner = row('row-member-human') as Extract<Row, { r: 'member' }>;
  const group = new Group(
    new Home({ user: owner.user, sign: () => new Promise(() => {}) }),
    owner.g,
  );
  group.apply([
    row('row-group'),
    owner,
    row('row-member-ai'),
    row('row-member-ai', {
      id: 'qBSFMsr2hHYKOMblEA_kdA',
      name: 'Secretary',
      cli: 'claude',
      secretary: true,
    }),
    row('row-member-ai', { id: 'LXCZmuGAXkvO-bSrOkuCfw', name: 'Claude Code', cli: 'claude' }),
    row('row-task'),
    row('row-msg-reply'),
  ]);

  // Every message example, by kind, in the order of the files.
  const kinds = Map.groupBy(
    Object.keys(goldens)
      .filter((name) => name.startsWith('row-msg-'))
      .sort()
      .map((name) => ({ name, msg: decode(MsgRow, goldens[name]) })),
    ({ msg }) => msg.kind,
  );
</script>

<div class="page states">
  <div class="page-head">
    <div class="grow">
      <h2>{t('page.states')}</h2>
      <p>{t('states.lead')}</p>
    </div>
  </div>

  <section class="card points" aria-labelledby="states-points">
    <h3 id="states-points">{t('states.points')}</h3>
    <dl>
      {#each points as point (point)}
        <dt class="mono">{point}</dt>
        <dd>
          {#each mounts[point] as entry (entry)}
            <span class="badge">{entry.area} · {entry.order}</span>
          {:else}
            <span class="hint">{t('states.none')}</span>
          {/each}
        </dd>
      {/each}
    </dl>
  </section>

  <PresetStates />

  {#each kinds as [kind, examples] (kind)}
    <section class="kind" aria-labelledby={`states-${kind}`}>
      <header>
        <h3 id={`states-${kind}`}>{t(`chat.kind.${kind}`)}</h3>
        <span class="mono hint">{kind}</span>
        <span class="grow"></span>
        {#if cards.get(kind)}
          <span class="badge" data-tone="ok">{t('states.by')} {cards.get(kind)!.area}</span>
        {:else}
          <span class="badge">{t('states.stub')}</span>
        {/if}
      </header>
      {#each examples as { name, msg } (name)}
        <figure>
          <figcaption class="mono hint">{name}</figcaption>
          <Message {group} {msg} joined={false} />
        </figure>
      {/each}
    </section>
  {/each}
</div>

<style>
  .states {
    max-width: calc(var(--measure) + 2 * var(--sp-5));
  }
  dl {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: var(--sp-2) var(--sp-4);
    font-size: var(--fs-sm);
  }
  dd {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-1);
  }
  .kind {
    display: grid;
    gap: var(--sp-3);
  }
  .kind header {
    display: flex;
    align-items: baseline;
    gap: var(--sp-2);
    padding-bottom: var(--sp-2);
    border-bottom: 1px solid var(--line);
  }
  .kind h3 {
    font-size: var(--fs-lg);
  }
  figure {
    padding: var(--sp-3) var(--sp-4) var(--sp-4);
    border: 1px dashed var(--line);
    border-radius: var(--r-lg);
  }
</style>
