<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import type * as p from '../../lib/proto';
  import type { Listed } from '../model/model';
  import Bars from '../profile/Bars.svelte';
  import { summaryParts } from './defaults';

  /** Toggle switch to follow a CLI's computer-level defaults in the add-AI and create-group
   *  dialogs, showing the one-line summary of those defaults and an optional note when a second
   *  member of the same CLI starts with its own setting (design A.4.3). */
  let {
    follow = $bindable(),
    defaults,
    models,
    note,
    disabled = false,
    onchange,
  }: {
    follow: boolean;
    defaults: p.Profile | undefined;
    models: readonly Listed[];
    note?: string;
    disabled?: boolean;
    onchange?: (follow: boolean) => void;
  } = $props();

  const uid = $props.id();
  const summary = $derived(defaults ? summaryParts(defaults, models) : undefined);

  function toggle() {
    follow = !follow;
    onchange?.(follow);
  }
</script>

<div class="sw-row">
  <label id={`${uid}-lbl`} for={`${uid}-sw`} class="sw-lbl">{t('defaults.switch')}</label>
  <button
    id={`${uid}-sw`}
    type="button"
    role="switch"
    class="switch"
    aria-checked={follow}
    aria-labelledby={`${uid}-lbl`}
    aria-describedby={note ? `${uid}-sum ${uid}-tip` : `${uid}-sum`}
    {disabled}
    onclick={toggle}
  ></button>
  {#if summary}
    <p id={`${uid}-sum`} class="sum">
      <span>{summary.model}</span>{' '}<span class="sep" aria-hidden="true">·</span>{' '}<span>
        {summary.effort}
      </span>{' '}<span class="sep" aria-hidden="true">·</span>{' '}<span class="sum-tier">
        {t('profile.tier')}
        <Bars tier={summary.tier} />
        {summary.tierLabel}
      </span>{#if summary.skills}{' '}<span class="sep" aria-hidden="true">·</span>{' '}<span>
          {summary.skills}
        </span>{/if}
    </p>
  {:else}
    <p id={`${uid}-sum`} class="sum">{t('defaults.unset.line')}</p>
  {/if}
  {#if note}
    <p id={`${uid}-tip`} class="tip">{note}</p>
  {/if}
</div>

<style>
  .sw-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 2px 12px;
    padding: 12px;
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    background: var(--surface);
  }
  .sw-lbl {
    font-size: var(--fs-ui);
    font-weight: 500;
    cursor: pointer;
  }
  .sw-row:has(.switch:disabled) .sw-lbl {
    cursor: not-allowed;
  }
  .sum {
    grid-column: 1;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 6px;
    font-size: var(--fs-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
  .sum .sep {
    color: var(--text-3);
  }
  .sum-tier {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .tip {
    grid-column: 1 / 3;
    margin-top: 2px;
    font-size: var(--fs-xs);
    color: var(--text-3);
    line-height: 1.45;
  }
  /* 44 by 26, the track in the accent when it is on; the thumb slides by transform. */
  .switch {
    appearance: none;
    grid-row: 1 / 3;
    grid-column: 2;
    align-self: center;
    flex: none;
    width: 44px;
    height: 26px;
    padding: 0;
    margin: 0;
    border: 0;
    border-radius: 13px;
    background: var(--line-strong);
    cursor: pointer;
    position: relative;

    &::before {
      content: '';
      position: absolute;
      top: 3px;
      left: 3px;
      width: 20px;
      height: 20px;
      border-radius: 50%;
      background: #fff;
      box-shadow: var(--shadow-1);
      transition: transform var(--dur-2) var(--ease);
    }
    &[aria-checked='true'] {
      background: var(--sky);
    }
    &[aria-checked='true']::before {
      transform: translateX(18px);
      background: var(--on-sky);
    }
    &:disabled {
      opacity: 0.5;
      cursor: not-allowed;
    }
  }
</style>
