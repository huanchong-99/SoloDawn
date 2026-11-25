<script lang="ts">
  import { limitFields } from '../../lib/groupset';
  import { t } from '../../lib/prefs.svelte';
  import type { GroupRow } from '../../lib/proto';

  /** The limits of a group, a line each (contracts/rows.md section 1), in the units shown. */
  let { limits = $bindable() }: { limits: GroupRow['limits'] } = $props();
  const uid = $props.id();
</script>

{#each limitFields as { key, unit, min, max, per } (key)}
  <div class="limit">
    <label for={`${uid}-${key}`}>{t(`groupinfo.limit.${key}`)}</label>
    <span class="value">
      <input
        class="input"
        id={`${uid}-${key}`}
        type="number"
        inputmode="numeric"
        min={min ?? 1}
        {max}
        step={per === 1 ? 1 : 'any'}
        required
        aria-describedby={`${uid}-${key}-unit ${uid}-${key}-hint`}
        bind:value={limits[key]}
      />
      <span class="muted" id={`${uid}-${key}-unit`}>{unit ? t(`groupinfo.unit.${unit}`) : ''}</span>
    </span>
    <p class="hint" id={`${uid}-${key}-hint`}>{t(`groupinfo.limit.${key}.hint`)}</p>
  </div>
{/each}

<style>
  .limit {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: 2px var(--sp-4);
    padding: 10px 0;
  }
  .limit + .limit {
    border-top: 1px solid var(--line);
  }
  label {
    align-self: end;
    font-weight: 500;
  }
  .hint {
    align-self: start;
  }
  .value {
    display: grid;
    grid-row: span 2;
    grid-template-columns: 72px 2.4em;
    align-items: center;
    gap: 6px;
  }
  input {
    text-align: right;
    appearance: textfield;
  }
  input::-webkit-inner-spin-button,
  input::-webkit-outer-spin-button {
    appearance: none;
  }
</style>
