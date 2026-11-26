<script lang="ts">
  import { tick } from 'svelte';
  import { MediaQuery } from 'svelte/reactivity';
  import { complete, fill, matching, mentioned, room, typing, unsent } from '../../lib/compose';
  import type { Group, Member } from '../../lib/groups.svelte';
  import { release } from '../../lib/jump';
  import type { Net } from '../../lib/net.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { TaskRow } from '../../lib/proto';
  import { replyNote, threadMembers } from '../../lib/taskops';
  import Avatar from '../Avatar.svelte';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** Where a message is written: mention completion, a hint before the length limit, send. With a
   *  task it goes to that task's thread, and a line says what becomes of it there; why the home
   *  cannot be reached the composer of the main line says for both. */
  let { group, net, task }: { group: Group; net?: Net; task?: TaskRow } = $props();
  const uid = $props.id();

  let field: HTMLTextAreaElement;
  let text = $state('');
  let caret = $state(0);
  let busy = $state(false);
  let failed = $state('');
  let active = $state(0);
  // The `@` whose list the person closed with Escape.
  let closed = $state(-1);

  // Enter sends and Shift+Enter breaks the line, also on a computer without a mouse. On a touch
  // keyboard Enter breaks the line and the button sends.
  const touch = new MediaQuery('pointer: coarse');

  const readonly = $derived(!group.home.ctl);
  const people = $derived(task ? threadMembers(group, task) : group.members);
  const mention = $derived(typing(text, caret));
  const found = $derived(mention && mention.at !== closed ? matching(people, mention.query) : []);
  // The members a text mentions make its row longer too.
  const mentions = $derived(mentioned(text, group.members));
  const named = $derived(mentions.length);
  const length = $derived(room(text, named));
  const written = $derived(text.trim() !== '');
  const sendable = $derived(written && length !== 'over' && !busy && !readonly);
  const note = $derived(task && written ? replyNote(group, task, mentions) : undefined);
  const intoThread = (key: 'thread.reply' | 'thread.reply.hint') =>
    task && t(key).replace('{task}', `T-${task.num}`);
  const label = $derived(intoThread('thread.reply') ?? t('chat.compose'));
  // A request goes to the secretary; a group known to have none has only the AIs to mention.
  const mainHint = $derived(
    group.info && !group.info.secretary ? 'chat.compose.hint.solo' : 'chat.compose.hint',
  );
  const hint = $derived(
    intoThread('thread.reply.hint') ?? t(group.info?.kind === 'team' ? 'team.compose' : mainHint),
  );

  // The field is as tall as its text, up to its limit in the styles.
  $effect(() => {
    void text;
    field.style.height = 'auto';
    field.style.height = `${field.scrollHeight}px`;
  });
  $effect(() => {
    void found.length;
    active = 0;
  });

  const read = () => {
    text = field.value;
    caret = field.selectionStart;
  };

  async function put(next: { text: string; caret: number }) {
    text = next.text;
    caret = next.caret;
    await tick();
    field.focus();
    field.setSelectionRange(caret, caret);
  }

  const pick = (member: Member) => put(complete(text, caret, mention!.at, member.name));

  /** The button beside the field starts a mention where the caret is. */
  const startMention = () =>
    put({ text: `${text.slice(0, caret)}@${text.slice(caret)}`, caret: caret + 1 });

  async function send() {
    if (!sendable) return;
    // What is sent shows at the end of the main line, which the jump's hold would keep out of view.
    release();
    busy = true;
    failed = '';
    const taken = text;
    const sent = taken.trim();
    const answer = await group.post(sent, mentioned(sent, group.members), task?.id);
    busy = false;
    if (answer.err) {
      failed = answer.err.code;
      return;
    }
    // Only what was sent leaves the field: what the writer typed meanwhile stays, caret and all.
    const left = unsent(text, taken);
    await put({ text: left, caret: Math.max(0, caret - (text.length - left.length)) });
  }

  function listKey(event: KeyboardEvent) {
    const steps: Record<string, number> = { ArrowDown: 1, ArrowUp: -1 };
    const step = steps[event.key];
    if (step) active = (active + step + found.length) % found.length;
    else if (event.key === 'Enter' || event.key === 'Tab') void pick(found[active]);
    else if (event.key === 'Escape') closed = mention!.at;
    else return;
    event.preventDefault();
  }

  function keydown(event: KeyboardEvent) {
    // A key pressed while an input method composes a character belongs to the input method.
    if (event.isComposing) return;
    if (found.length) return listKey(event);
    if (event.key === 'Enter' && !event.shiftKey && !touch.current) {
      event.preventDefault();
      void send();
    }
  }
</script>

<div class="composer" class:in-thread={task}>
  {#if readonly && net}
    <p class={net.otherTab ? 'state away' : 'state down'} id={`${uid}-state`} role="status">
      <i></i>{t(net.otherTab ? 'chat.otherTab' : 'chat.readonly')}
    </p>
  {:else if length !== 'ok'}
    <p class="state" id={`${uid}-state`} class:over={length === 'over'} role="status">
      <span class="meter" style:--v={Math.min(fill(text, named), 100) / 100}></span>
      {t(`chat.length.${length}`).replace('{n}', String(fill(text, named)))}
    </p>
  {/if}
  {#if failed}<Failure code={failed} />{/if}

  <form
    onsubmit={(event) => {
      event.preventDefault();
      void send();
    }}
  >
    {#if found.length}
      <div class="mentions" role="listbox" id={`${uid}-list`} aria-label={t('chat.mention.list')}>
        {#each found as member, i (member.id)}
          <button
            type="button"
            role="option"
            id={`${uid}-${i}`}
            tabindex="-1"
            aria-selected={i === active}
            onmousedown={(event) => event.preventDefault()}
            onclick={() => pick(member)}
          >
            <Avatar
              id={member.kind === 'human' ? member.user : member.id}
              name={member.name}
              cli={member.kind === 'ai' ? member.cli : undefined}
              size="sm"
            />
            <span class="cut">{member.name}</span>
            {#if member.kind === 'ai' && member.secretary}
              <span class="badge">{t('chat.secretary')}</span>
            {/if}
          </button>
        {/each}
      </div>
    {/if}
    <button
      type="button"
      class="icon-btn"
      aria-label={t('chat.mention')}
      disabled={readonly}
      onclick={startMention}
    >
      <Icon name="at" />
    </button>
    <textarea
      bind:this={field}
      rows="1"
      value={text}
      role="combobox"
      aria-label={label}
      aria-autocomplete="list"
      aria-expanded={found.length > 0}
      aria-controls={`${uid}-list`}
      aria-activedescendant={found.length ? `${uid}-${active}` : undefined}
      aria-invalid={length === 'over' || undefined}
      aria-describedby={(readonly && net) || length !== 'ok' ? `${uid}-state` : undefined}
      placeholder={hint}
      disabled={readonly}
      oninput={read}
      onkeyup={read}
      onclick={read}
      onfocus={read}
      onkeydown={keydown}></textarea>
    <button class="btn btn-primary send" aria-label={t('chat.send')} disabled={!sendable}>
      <Icon name="send" />
    </button>
  </form>
  {#if note}<p class="note-line" role="status">{note}</p>{/if}
</div>

<style>
  .composer {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--sp-2);
    padding: var(--sp-3) var(--sp-4) calc(var(--sp-3) + env(safe-area-inset-bottom));
    border-top: 1px solid var(--line);
    background: var(--surface);
  }
  .composer > * {
    width: 100%;
    max-width: var(--measure);
    margin-inline: auto;
  }
  /* At the end of a task's thread, inside its card. */
  .in-thread {
    padding: 0 var(--sp-4) var(--sp-4);
    border: 0;
    background: none;
    font-size: var(--fs-sm);
  }
  .note-line {
    padding-left: 10px;
    color: var(--text-3);
    font-size: var(--fs-xs);
    animation: appear var(--dur-2) var(--ease-out);
  }
  form {
    position: relative;
    display: flex;
    align-items: flex-end;
    gap: var(--sp-2);
    padding: 6px;
    border: 1px solid var(--line-strong);
    border-radius: var(--r-lg);
    background: var(--bg);
  }
  form:focus-within {
    outline: var(--focus);
    outline-offset: 1px;
  }
  textarea {
    flex: 1;
    min-width: 0;
    min-height: 36px;
    max-height: 40dvh;
    padding: 6px 4px;
    border: 0;
    background: none;
    resize: none;
    outline: none;
    line-height: 1.6;
  }
  /* The hint is one line, cut at the edge of a narrow field. */
  textarea:placeholder-shown {
    white-space: nowrap;
    overflow: hidden;
  }
  .send {
    width: 36px;
    padding: 0;
  }
  /* Told above the field, where the eye is: the home cannot be reached, or the text is long. */
  .state {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    color: var(--dawn-text);
    font-size: var(--fs-sm);
    animation:
      rise var(--dur-3) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .state.over,
  .state.down {
    color: var(--danger);
  }
  .state.away {
    color: var(--text-2);
  }
  .down i,
  .away i {
    flex: none;
    width: 8px;
    height: 8px;
    border: 2px solid currentColor;
    border-radius: 50%;
  }
  /* How full the row is: a bar that grows from the left, by transform alone. */
  .meter {
    flex: none;
    width: 56px;
    height: 6px;
    border-radius: 3px;
    background: var(--surface-3);
    overflow: hidden;
  }
  .meter::before {
    content: '';
    display: block;
    height: 100%;
    background: currentColor;
    transform: scaleX(var(--v));
    transform-origin: left;
    transition: transform var(--dur-2) var(--ease);
  }
  .mentions {
    position: absolute;
    left: 0;
    bottom: calc(100% + 8px);
    z-index: 2;
    display: grid;
    gap: 2px;
    width: min(320px, 100%);
    max-height: 240px;
    padding: 6px;
    border-radius: var(--r-lg);
    background: var(--surface);
    box-shadow: var(--shadow-pop);
    overflow: auto;
    transform-origin: bottom left;
    animation:
      rise var(--dur-2) var(--ease-out),
      appear var(--dur-1) var(--ease-out);
  }
  .mentions button {
    position: relative;
    isolation: isolate;
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    min-height: 36px;
    padding: 0 8px;
    border: 0;
    border-radius: var(--r-md);
    background: none;
    text-align: start;
  }
  /* The option the keys are on, or the pointer: a layer under it. */
  .mentions button::before {
    content: '';
    position: absolute;
    inset: 0;
    z-index: -1;
    border-radius: inherit;
    background: var(--surface-2);
    opacity: 0;
    transition: opacity var(--dur-1) var(--ease);
  }
  .mentions [aria-selected='true']::before {
    opacity: 1;
  }
  @media (hover: hover) {
    .mentions button:hover::before {
      opacity: 1;
    }
  }
  @media (pointer: fine) {
    .in-thread textarea {
      padding-block: 7.5px;
    }
  }
  @media (pointer: coarse) {
    textarea {
      min-height: 44px;
      padding-block: 10px;
      /* Below 16px, Safari on a phone zooms the page into a focused field. */
      font-size: 16px;
    }
    .send {
      width: 44px;
    }
  }
</style>
