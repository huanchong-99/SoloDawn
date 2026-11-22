<script lang="ts">
  import { type Group, type Home } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Avatar from '../Avatar.svelte';
  import Failure from '../Failure.svelte';
  import { stopWatch, watchers } from './watch.svelte';

  let { group, host, member }: { group: Group; host: Home; member: string } = $props();
  let busy = $state(false);
  let failed = $state('');
  const users = $derived(watchers(host.hostInfo, group.g, member));

  $effect(() => {
    const ctl = host.ctl;
    if (ctl && host.owner)
      void ctl
        .request({ t: 'host.info' })
        .then((answer) => {
          if (!answer.err && host.ctl === ctl) host.hostInfo = answer;
        })
        .catch(() => {
          failed = 'lost';
        });
  });

  async function stop(user: string) {
    busy = true;
    failed = '';
    const answer = await stopWatch(host, group.g, member, user);
    busy = false;
    if (answer.err) failed = answer.err.code;
  }
</script>

<section class="card" aria-label={t('watch.watchers')}>
  <header class="card-head"><span class="kind">{t('watch.watchers')}</span></header>
  {#each users as user (user)}
    <div class="row">
      <Avatar id={user} name={group.personName(user) ?? t('chat.gone')} />
      <span class="grow">{group.personName(user) ?? t('chat.gone')}</span>
      {#if host.owner}
        <button
          type="button"
          class="btn btn-sm"
          disabled={busy || !host.ctl}
          onclick={() => stop(user)}
        >
          {t('watch.stop')}
        </button>
      {/if}
    </div>
  {:else}
    <p class="hint">{t('watch.empty')}</p>
  {/each}
  {#if failed}<Failure code={failed} />{/if}
</section>
