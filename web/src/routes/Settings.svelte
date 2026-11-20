<script lang="ts">
  import { choose, langs, prefs, t, themes } from '../lib/prefs.svelte';
</script>

<div class="page">
  <div class="page-head">
    <div class="grow">
      <h2>{t('settings.title')}</h2>
      <p>{t('settings.lead')}</p>
    </div>
  </div>

  <section class="card set" aria-labelledby="s-look">
    <header><h3 id="s-look">{t('settings.look')}</h3></header>
    <div class="set-row">
      <div><b>{t('settings.theme')}</b></div>
      <fieldset class="seg" style:--at={themes.indexOf(prefs.theme)}>
        <legend class="sr">{t('settings.theme')}</legend>
        {#each themes as theme (theme)}
          <label>
            <input
              type="radio"
              name="theme"
              checked={theme === prefs.theme}
              onchange={() => choose('theme', theme)}
            />
            <span>{t(`settings.theme.${theme}`)}</span>
          </label>
        {/each}
      </fieldset>
    </div>
    <div class="set-row">
      <div>
        <b>{t('settings.lang')}</b>
        <p>{t('settings.lang.note')}</p>
      </div>
      <fieldset class="seg" style:--at={langs.indexOf(prefs.lang)}>
        <legend class="sr">{t('settings.lang')}</legend>
        {#each langs as lang (lang)}
          <label>
            <input
              type="radio"
              name="lang"
              checked={lang === prefs.lang}
              onchange={() => choose('lang', lang)}
            />
            <span>{t(`settings.lang.${lang}`)}</span>
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
