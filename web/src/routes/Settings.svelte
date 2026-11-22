<script lang="ts">
  import ReminderCard from '../components/reminder/ReminderCard.svelte';
  import { choose, langs, prefs, t, themes } from '../lib/prefs.svelte';
  import type { PageProps } from '../lib/route.svelte';

  let { net }: PageProps<'settings'> = $props();
</script>

<div class="page">
  <div class="page-head">
    <div class="grow">
      <h2>{t('page.settings')}</h2>
      <p>{t('settings.lead')}</p>
    </div>
  </div>

  <ReminderCard {net} />

  <section class="card set" aria-labelledby="s-look">
    <header><h3 id="s-look">{t('settings.look')}</h3></header>
    <div class="set-row">
      <div><b>{t('pref.theme')}</b></div>
      <fieldset class="seg" style:--at={themes.indexOf(prefs.theme)}>
        <legend class="sr">{t('pref.theme')}</legend>
        {#each themes as theme (theme)}
          <label>
            <input
              type="radio"
              name="theme"
              checked={theme === prefs.theme}
              onchange={() => choose('theme', theme)}
            />
            <span>{t(`pref.theme.${theme}`)}</span>
          </label>
        {/each}
      </fieldset>
    </div>
    <div class="set-row">
      <div>
        <b>{t('pref.lang')}</b>
        <p>{t('settings.lang.note')}</p>
      </div>
      <fieldset class="seg" style:--at={langs.indexOf(prefs.lang)}>
        <legend class="sr">{t('pref.lang')}</legend>
        {#each langs as lang (lang)}
          <!-- Each language is named in itself, so a screen reader speaks it in that language. -->
          <label {lang}>
            <input
              type="radio"
              name="lang"
              checked={lang === prefs.lang}
              onchange={() => choose('lang', lang)}
            />
            <span>{t(`pref.lang.${lang}`)}</span>
          </label>
        {/each}
      </fieldset>
    </div>
  </section>
</div>

<style>
  .page {
    max-width: 760px;
  }
</style>
