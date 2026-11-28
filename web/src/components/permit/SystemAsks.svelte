<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';
  import { asks, browserPlatform } from './permit.svelte.ts';

  const uid = $props.id();

  const group = $derived(asks(browserPlatform(navigator)));
</script>

{#if group !== 'none'}
  <aside class="note sys-asks" aria-labelledby={uid}>
    <Icon name="info" sm />
    <div class="copy">
      <b id={uid}>{t('permit.sys.title')}</b>
      <p class="lead">{t('permit.sys.lead')}</p>
      {#if group === 'mac' || group === 'both'}
        {#if group === 'both'}<p class="grp">{t('permit.sys.mac')}</p>{/if}
        <ul>
          <li>{t('permit.sys.mac.1')}</li>
          <li>{t('permit.sys.mac.2')}</li>
          <li>{t('permit.sys.mac.3')}</li>
        </ul>
      {/if}
      {#if group === 'win' || group === 'both'}
        {#if group === 'both'}<p class="grp">{t('permit.sys.win')}</p>{/if}
        <ul>
          <li>{t('permit.sys.win.1')}</li>
        </ul>
      {/if}
    </div>
  </aside>
{/if}

<style>
  .sys-asks {
    padding: 12px 14px;
    gap: 10px;
  }
  .copy {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 4px;
  }
  b {
    font-size: var(--fs-sm);
    font-weight: 600;
  }
  .lead {
    font-size: var(--fs-xs);
    color: var(--text-3);
    line-height: 1.5;
  }
  .grp {
    font-size: var(--fs-xs);
    font-weight: 600;
    color: var(--text-2);
    margin-top: 6px;
  }
  ul {
    display: grid;
    gap: 4px;
    margin-top: 4px;
  }
  li {
    font-size: var(--fs-sm);
    color: var(--text-2);
    padding-left: 12px;
    position: relative;
    line-height: 1.5;
  }
  li::before {
    content: '';
    position: absolute;
    left: 0;
    top: 9px;
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: var(--text-3);
  }
</style>
