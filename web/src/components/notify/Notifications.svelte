<script lang="ts">
  import { onDestroy } from 'svelte';
  import { flip } from 'svelte/animate';
  import { prefersReducedMotion } from 'svelte/motion';
  import { fade } from 'svelte/transition';
  import type { Net } from '../../lib/net.svelte';
  import { Notifier, titleOf, whoOf, type NotifyKind } from '../../lib/notify.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { href } from '../../lib/route.svelte';
  import Icon, { type IconName } from '../Icon.svelte';

  /** Live in-page notifications mounted in the shell's overlay layer. */
  let { net }: { net: Net } = $props();

  const icons: Record<NotifyKind, IconName> = {
    approval: 'shield',
    decision: 'fork',
    brief: 'doc',
    done: 'check',
    mention: 'at',
  };
  const tones: Record<NotifyKind, 'dawn' | 'ok' | 'sky'> = {
    approval: 'dawn',
    decision: 'dawn',
    brief: 'dawn',
    done: 'ok',
    mention: 'sky',
  };

  const notifier = new Notifier();
  $effect(() => notifier.sync(net.homes));
  onDestroy(() => notifier.clear());
</script>

<section class="notify" aria-label={t('notify.label')} aria-live="polite">
  {#if !net.otherTab}
    {#each notifier.items as item (item.key)}
      {@const title = titleOf(item.group, item.msg)}
      <article
        class="toast"
        class:need={item.needsYou}
        data-tone={tones[item.kind]}
        animate:flip={{ duration: prefersReducedMotion.current ? 0 : 320 }}
        out:fade={{ duration: prefersReducedMotion.current ? 0 : 200 }}
        onfocusin={() => notifier.hold(item.key)}
        onfocusout={() => notifier.release(item.key)}
        onpointerenter={() => notifier.hold(item.key)}
        onpointerleave={() => notifier.release(item.key)}
      >
        <span class="glyph"><Icon name={icons[item.kind]} /></span>
        <div class="grow">
          <b class="cut">{t(`notify.kind.${item.kind}`)}{title ? ` · ${title}` : ''}</b>
          <p class="muted cut">{item.group.name} · {whoOf(item.group, item.msg)}</p>
        </div>
        <a
          class="btn btn-sm"
          class:btn-dawn={item.needsYou}
          href={href('chat', { g: item.group.g })}
          onclick={(event) => void notifier.open(event, item)}
        >
          {t('notify.open')}
        </a>
        <button
          type="button"
          class="icon-btn"
          aria-label={t('notify.dismiss')}
          onclick={() => notifier.dismiss(item.key)}
        >
          <Icon name="x" sm />
        </button>
      </article>
    {/each}
  {/if}
</section>

<style>
  /* Over the top of the page's area, so under the top bar and the banners at every width. */
  .notify {
    grid-area: main;
    align-self: start;
    justify-self: end;
    z-index: 10;
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
    width: min(380px, calc(100% - 2 * var(--sp-4)));
    margin: var(--sp-3) var(--sp-4) 0;
    pointer-events: none;
  }
  .toast {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    padding: 10px 10px 10px var(--sp-4);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    background: var(--surface);
    box-shadow: var(--shadow-pop);
    pointer-events: auto;
    animation:
      drop var(--dur-4) var(--spring),
      appear var(--dur-2) var(--ease-out);

    &.need {
      border-color: color-mix(in srgb, var(--dawn) 55%, var(--line));
      box-shadow:
        inset 3px 0 0 var(--dawn),
        var(--shadow-pop);
    }
  }
  .glyph {
    display: grid;
    place-items: center;
    flex: none;
    color: var(--tone-text, var(--text-2));
  }
  .toast b {
    display: block;
    font-size: 14px;
  }
  .toast p {
    font-size: var(--fs-xs);
  }
  @media (max-width: 767px) {
    .notify {
      justify-self: stretch;
      width: auto;
      margin-inline: var(--sp-3);
    }
    /* A phone's chat has its own head where the top bar was. */
    :global(.app[data-full='phone']) > .notify {
      margin-top: calc(var(--bar-h) + var(--sp-3));
    }
    .toast {
      gap: var(--sp-2);
      padding: 10px var(--sp-2) 10px var(--sp-3);
    }
  }
</style>
