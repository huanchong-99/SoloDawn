<script lang="ts">
  import { onDestroy } from 'svelte';
  import { flip } from 'svelte/animate';
  import { prefersReducedMotion } from 'svelte/motion';
  import { MediaQuery } from 'svelte/reactivity';
  import { fade } from 'svelte/transition';
  import { side } from '../../lib/mounts.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { Notifier, titleOf, whoOf, type NotifyKind } from '../../lib/notify.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { href, route } from '../../lib/route.svelte';
  import Icon, { type IconName } from '../Icon.svelte';

  /** Live in-page notifications mounted in the shell's overlay layer. */
  let { net }: { net: Net } = $props();

  const icons: Record<NotifyKind, IconName> = {
    approval: 'shield',
    decision: 'fork',
    brief: 'doc',
    confirm: 'shield',
    done: 'check',
    mention: 'at',
    alert: 'alert',
  };
  const tones: Record<NotifyKind, 'dawn' | 'ok' | 'sky' | 'danger'> = {
    approval: 'dawn',
    decision: 'dawn',
    brief: 'dawn',
    confirm: 'dawn',
    done: 'ok',
    mention: 'sky',
    alert: 'danger',
  };

  /** An alert's title is a sentence of its own. */
  const kindOf = (kind: NotifyKind) => (kind === 'alert' ? '' : t(`notify.kind.${kind}`));

  const sliding = new MediaQuery('max-width: 1023px');
  const notifier = new Notifier();
  $effect(() => notifier.sync(net.homes));
  onDestroy(() => notifier.clear());

  let stack: HTMLElement;
  /** Closing from the keyboard hands focus to the next notification's close button, or to the
   *  page when it was the last. */
  function close(button: HTMLElement, key: string) {
    if (button.matches(':focus-visible')) {
      const closers = [...stack.querySelectorAll<HTMLElement>('.toast:not([inert]) .icon-btn')];
      const at = closers.indexOf(button);
      (closers[at + 1] ?? closers[at - 1] ?? document.querySelector('main'))!.focus();
    }
    notifier.dismiss(key);
  }
</script>

<section
  class="notify"
  class:chat={route.page === 'chat'}
  bind:this={stack}
  inert={route.page === 'chat' && side.open && sliding.current}
  aria-label={t('notify.label')}
  aria-live="polite"
>
  {#if !net.otherTab}
    {#each notifier.items as item (item.key)}
      {@const title = titleOf(item.group, item.msg)}
      <article
        class="toast"
        class:need={item.needsYou}
        data-tone={tones[item.kind]}
        animate:flip={{ duration: prefersReducedMotion.current ? 0 : 240 }}
        out:fade={{ duration: prefersReducedMotion.current ? 0 : 140 }}
        onfocusin={() => notifier.hold(item.key)}
        onfocusout={() => notifier.release(item.key)}
        onpointerenter={() => notifier.hold(item.key)}
        onpointerleave={() => notifier.release(item.key)}
      >
        <span class="glyph"><Icon name={icons[item.kind]} /></span>
        <div class="grow">
          <b class:cut={item.kind === 'mention' || item.kind === 'done'} id={`n-${item.key}`}>
            {[kindOf(item.kind), title].filter(Boolean).join(' · ')}
          </b>
          <p class="muted cut">{item.group.name} · {whoOf(item.group, item.msg)}</p>
        </div>
        <a
          class="btn btn-sm"
          class:btn-dawn={item.needsYou}
          href={href('chat', { g: item.group.g })}
          aria-describedby={`n-${item.key}`}
          onclick={(event) => void notifier.open(event, item)}
        >
          {t('notify.open')}
        </a>
        <button
          type="button"
          class="icon-btn"
          aria-label={t('notify.dismiss')}
          aria-describedby={`n-${item.key}`}
          onclick={(event) => close(event.currentTarget, item.key)}
        >
          <Icon name="x" sm />
        </button>
      </article>
    {/each}
  {/if}
</section>

<style>
  /* Over the top of the page's area, so under the top bar and the banners at every width, and
     under a chat's side panel while that is open over the chat (z-index 5). */
  .notify {
    grid-area: main;
    align-self: start;
    justify-self: end;
    z-index: 4;
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
    width: min(360px, calc(100% - 2 * var(--sp-4)));
    margin: var(--sp-3) var(--sp-4) 0;
    pointer-events: none;
  }
  /* An open chat has its own head: the stack hangs under it, clear of the head's buttons and of
     "Add AI" beside it. */
  .notify.chat {
    margin-top: calc(var(--bar-h) + var(--sp-3));
  }
  /* There it lies over the side column, as wide as that column's cards. */
  @media (min-width: 1024px) {
    .notify.chat {
      width: calc(var(--side-w) - 2 * var(--sp-4));
    }
  }
  /* A raised card that comes down from the top and fades when it goes. One that waits for the
     reader has the tint of dawn in its edge and its glyph; the way in is always the accent. */
  .toast {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    padding: 10px 6px 10px var(--sp-4);
    border-radius: var(--r-lg);
    background: var(--raised);
    box-shadow: var(--shadow-pop);
    pointer-events: auto;
    animation:
      drop var(--dur-2) var(--ease-out),
      appear var(--dur-2) var(--ease-out);

    &.need {
      box-shadow:
        0 6px 20px light-dark(rgb(16 24 40 / 0.1), rgb(0 0 0 / 0.5)),
        0 0 0 1px var(--need-line);
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
    font-size: var(--fs-ui);
    line-height: 1.4;
  }
  .toast .btn {
    --hover: color-mix(in srgb, var(--sky) 86%, var(--text));
    border-color: transparent;
    background: var(--sky);
    box-shadow: none;
    color: var(--on-sky);
  }
  .toast .btn::before {
    inset: -1px;
    border-radius: inherit;
  }
  .toast .icon-btn {
    width: 32px;
    height: 32px;
    margin-left: -6px;
  }
  /* What waits for the reader has room to say who asks, and an alert to say what happened. */
  .toast b:not(.cut) {
    display: -webkit-box;
    overflow: hidden;
    overflow-wrap: anywhere;
    -webkit-box-orient: vertical;
    line-clamp: 2;
    -webkit-line-clamp: 2;
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
    /* A phone's terminal, like its chat, has its own head where the top bar was. */
    :global(.app[data-full='phone']) > .notify {
      margin-top: calc(var(--bar-h) + var(--sp-3));
    }
    .notify {
      margin-top: calc(var(--sp-2) + env(safe-area-inset-top));
    }
    .toast {
      gap: var(--sp-2);
      padding: 10px 2px 10px var(--sp-3);
    }
  }
</style>
