<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Avatar from '../Avatar.svelte';

  let { group }: { group: Group } = $props();
  const humans = $derived(group.members.filter((member) => member.kind === 'human'));
</script>

<div class="members">
  <h2 class="sec-title">{[t('member.people'), humans.length].join(' · ')}</h2>
  <ul class="people">
    {#each humans as person (person.id)}
      <li class="row person">
        <Avatar id={person.user} name={person.name} />
        <span class="grow cut">{person.name}</span>
        <span class="st" data-st={person.online ? 'ok' : 'offline'}>
          <i></i>{t(person.online ? 'member.online' : 'member.offline')}
        </span>
      </li>
    {/each}
  </ul>
</div>

<style>
  .members {
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
    padding: var(--sp-4);
  }
  .people {
    display: flex;
    flex-direction: column;
  }
  .person {
    flex-wrap: nowrap;
    padding: 6px 0;
  }
</style>
