<script lang="ts">
  import type { CardProps } from '../../lib/mounts.svelte';
  import { t } from '../../lib/prefs.svelte';
  import { href } from '../../lib/route.svelte';
  import Rich from '../Rich.svelte';

  let { group, msg }: CardProps<'bulletin'> = $props();
</script>

<section class="card" aria-label={t('team.bulletin')}>
  <header class="card-head">
    <span class="kind">{msg.body.ai}</span>
    <a href={href('chat', { g: msg.body.from })}>{msg.body.group}</a>
  </header>
  <p class="hint">
    {t('team.computer').replace('{who}', () => group.member(msg.by)?.name ?? t('chat.gone'))}
  </p>
  <p><Rich {group} text={msg.text} mentions={msg.mentions} /></p>
</section>
