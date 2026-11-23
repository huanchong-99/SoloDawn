<script lang="ts">
  import { untrack } from 'svelte';
  import { homes, type Group } from '../../lib/groups.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow } from '../../lib/proto';
  import { hostHomeOf } from '../../lib/terminal.svelte';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import { checkReplies, checkText, latestCheck, serverEntry, setupPrompt } from './setup';

  let {
    group,
    ai,
    autoCheck = false,
    onclose,
  }: { group: Group; ai: AIMemberRow; autoCheck?: boolean; onclose: () => void } = $props();
  const uid = $props.id();
  let dialog: HTMLDialogElement;
  let sent = $state(false);
  let failed = $state('');
  let clipboard = $state<'copied' | 'failed'>();
  let taken = $state<{ id: string; seq: number }>();

  const host = $derived(hostHomeOf(homes, ai.host));
  const exe = $derived(host && nodes.infos.get(host)?.exe);
  const prompt = $derived(
    exe && setupPrompt({ group: group.name, name: ai.name, member: ai.id, exe }),
  );
  const check = $derived(latestCheck(group.main, group.me?.id, ai.id));
  const replies = $derived(check ? checkReplies(group.main, check, ai.id) : []);
  const takenAt = $derived(
    replies.at(-1)?.seq ?? (taken?.id === check?.id ? taken?.seq : undefined),
  );
  const status = $derived(
    ai.state === 'waiting' || ai.state === 'working' || ai.state === 'paused'
      ? ai.state
      : 'offline',
  );
  const marks = $derived([
    ['runner', status !== 'offline'],
    ['taken', takenAt !== undefined],
    ['answered', replies.length === 1],
    ['ended', status === 'waiting' && takenAt !== undefined && ai.seq > takenAt],
  ] as const);

  $effect(() => {
    dialog.showModal();
    untrack(() => {
      if (autoCheck) void sendCheck();
    });
  });

  $effect(() => {
    if (check && status === 'working' && ai.seq > check.seq) {
      taken = { id: check.id, seq: ai.seq };
    }
  });

  async function sendCheck() {
    sent = true;
    failed = '';
    const answer = await group.post(checkText, [ai.id]);
    sent = false;
    if (answer.err) failed = answer.err.code;
  }

  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      clipboard = 'copied';
    } catch {
      clipboard = 'failed';
    }
  }
</script>

<dialog class="dlg setup" bind:this={dialog} aria-labelledby={`${uid}-title`} {onclose}>
  <header class="row">
    <h2 class="grow" id={`${uid}-title`}>{t('external.setup.title')} · {ai.name}</h2>
    <button
      type="button"
      class="icon-btn"
      aria-label={t('external.setup.close')}
      onclick={() => dialog.close()}
    >
      <Icon name="x" />
    </button>
  </header>

  <p class="muted">{t('external.setup.host').replace('{host}', () => ai.host_name)}</p>

  {#if prompt && exe}
    <section class="setup-part">
      <div class="row">
        <h3 class="grow" id={`${uid}-prompt`}>{t('external.setup.prompt')}</h3>
        <button type="button" class="btn btn-sm btn-primary" onclick={() => copy(prompt!)}>
          <Icon name="copy" sm />{t('external.setup.copy')}
        </button>
      </div>
      <textarea
        class="input mono"
        readonly
        value={prompt}
        rows="7"
        lang="en"
        aria-labelledby={`${uid}-prompt`}></textarea>
      {#if clipboard}
        <p class="hint" role="status">{t(`external.setup.${clipboard}`)}</p>
      {/if}
    </section>
    <section class="setup-part">
      <h3>{t('external.setup.config')}</h3>
      <pre class="mono">{serverEntry(exe, ai.id)}</pre>
    </section>
  {:else}
    <p class="hint">{t('external.setup.noPath')}</p>
  {/if}

  <section class="setup-part">
    <div class="row check-title">
      <h3 class="grow">{t('external.check.title')}</h3>
      <span class="st" data-st={status} role="status"><i></i>{t(`member.state.${status}`)}</span>
    </div>
    <ol class="marks" aria-live="polite">
      {#each marks as [mark, done] (mark)}
        <li class:done>
          <span class="mark" aria-hidden="true">
            {#if done}<Icon name="check" sm />{/if}
          </span>
          <span>{t(`external.check.${mark}`)}</span>
          <span class="sr">{t(done ? 'external.check.done' : 'external.check.pending')}</span>
        </li>
      {/each}
    </ol>
    {#if replies.length > 1}
      <p role="status" class="hint">{t('external.check.multiple')}</p>
    {/if}
    {#if failed}<Failure code={failed} />{/if}
    <footer class="row">
      <span class="grow"></span>
      <button
        type="button"
        class="btn btn-sm"
        disabled={sent || !group.home.ctl}
        onclick={sendCheck}
      >
        <Icon name="refresh" sm />{t('external.check.send')}
      </button>
    </footer>
  </section>
</dialog>

<style>
  .setup {
    width: min(560px, calc(100vw - 32px));
    overflow: auto;
    overflow-wrap: anywhere;
  }
  .setup-part {
    display: grid;
    gap: var(--sp-2);
    margin-top: var(--sp-4);
  }
  .setup-part h3 {
    font-size: var(--fs-sm);
  }
  pre {
    padding: var(--sp-3);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-xs);
    white-space: pre-wrap;
  }
  .marks {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
    gap: var(--sp-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .marks li {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    padding: var(--sp-2);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    color: var(--text-2);
    font-size: var(--fs-xs);
  }
  .mark {
    display: grid;
    place-items: center;
    flex: none;
    width: 20px;
    height: 20px;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-full);
  }
  .done .mark {
    border-color: transparent;
    background: var(--sky-soft);
    color: var(--sky);
    animation: -global-appear var(--dur-2) var(--ease-out);
  }
</style>
