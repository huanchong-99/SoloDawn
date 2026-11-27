<script lang="ts">
  import type { Snippet } from 'svelte';
  import { codeUntil, invitationText, type OpenCode } from '../../lib/members';
  import { announcement, prefs, t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  /** An open invitation code of group `group`: the code, large, when it ends, and its two copies
   *  (contracts/step-2/people.md 2.6). `children`, beside when it ends, holds what else can be
   *  done with it. */
  let { code, group, children }: { code: OpenCode; group: string; children?: Snippet } = $props();
  /** Which copy of this code was taken last; none again once another code is shown. */
  let copied: 'code' | 'invitation' | undefined = $derived(void code.code);

  async function copy(what: 'code' | 'invitation') {
    const text = what === 'code' ? code.code : invitationText(group, code, prefs.lang);
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      announcement.text = t('people.code.copyFailed');
      return;
    }
    copied = what;
    announcement.text = t('people.code.copied');
  }
</script>

{#key code.code}
  <div class="code-card">
    <output class="code mono" aria-label={t('people.code.title')}>{code.code}</output>
    <div class="meta">
      <span class="until grow"><Icon name="clock" sm />{codeUntil(code, prefs.lang)}</span>
      {@render children?.()}
    </div>
    <div class="row">
      <button type="button" class="btn btn-primary btn-sm" onclick={() => copy('code')}>
        <Icon name={copied === 'code' ? 'check' : 'copy'} sm />
        <span>{t('people.code.copy')}</span>
      </button>
      <button type="button" class="btn btn-sm" onclick={() => copy('invitation')}>
        <Icon name={copied === 'invitation' ? 'check' : 'copy'} sm />
        <span>{t('people.code.copyInvitation')}</span>
      </button>
    </div>
  </div>
{/key}

<style>
  /* The code arrives from just below its place; a new code arrives the same way. */
  .code-card {
    display: grid;
    gap: var(--sp-2);
    padding: var(--sp-4);
    border-radius: var(--r-lg);
    background: var(--surface-2);
    animation:
      rise var(--dur-3) var(--ease-out),
      appear var(--dur-3) var(--ease-out);
  }
  .code {
    font-size: clamp(20px, 6vw, 26px);
    font-weight: 600;
    letter-spacing: 0.08em;
    line-height: 1.2;
    color: var(--text);
    user-select: all;
    overflow-wrap: anywhere;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-1);
    min-height: var(--ctl-sm);
  }
  .until {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-width: fit-content;
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .row {
    margin-top: var(--sp-1);
  }
</style>
