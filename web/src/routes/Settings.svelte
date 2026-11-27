<script lang="ts">
  import GuideCard from '../components/guide/GuideCard.svelte';
  import ReminderCard from '../components/reminder/ReminderCard.svelte';
  import { accents, choose, langs, prefs, t, themes } from '../lib/prefs.svelte';
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
    <div class="set-row accent">
      <div>
        <b>{t('pref.accent')}</b>
        <p>{t(`pref.accent.${prefs.accent}`)}</p>
      </div>
      <fieldset class="swatches">
        <legend class="sr">{t('pref.accent')}</legend>
        {#each accents as accent (accent)}
          <!-- Each swatch carries its own theme colour (accents.css); its name is its label. -->
          <label class="swatch" data-accent={accent} title={t(`pref.accent.${accent}`)}>
            <input
              type="radio"
              name="accent"
              checked={accent === prefs.accent}
              onchange={() => choose('accent', accent)}
            />
            <i class="tick" aria-hidden="true"></i>
            <span class="sr">{t(`pref.accent.${accent}`)}</span>
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

  <GuideCard {net} />
</div>

<style>
  .page {
    max-width: 760px;
  }
  .accent {
    flex-wrap: wrap;
  }
  .swatches {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  /* A disc of the colour in a 36px target; the chosen one has a ring of its colour and a tick in
     the colour that stands on it, so the choice is not told by colour alone. */
  .swatch {
    position: relative;
    display: grid;
    place-items: center;
    width: var(--ctl);
    height: var(--ctl);
    border-radius: var(--r-full);
    cursor: pointer;
    forced-color-adjust: none;

    &::before,
    &::after,
    .tick {
      grid-area: 1 / 1;
      pointer-events: none;
      transition:
        opacity var(--dur-1) var(--ease),
        transform var(--dur-4) var(--spring);
    }
    &::before {
      content: '';
      width: 24px;
      height: 24px;
      border-radius: inherit;
      background: var(--sky);
      box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text) 14%, transparent);
    }
    &::after {
      content: '';
      position: absolute;
      inset: 2px;
      border: 2px solid var(--sky);
      border-radius: inherit;
      opacity: 0;
      transform: scale(0.8);
    }
    &:hover::before {
      transform: scale(1.1);
    }
    &:has(:focus-visible) {
      outline: var(--focus);
      outline-offset: 1px;
    }
    &:has(:checked)::after {
      opacity: 1;
      transform: none;
    }
  }
  .swatch input {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    opacity: 0;
    cursor: pointer;
  }
  .tick {
    width: 10px;
    height: 6px;
    margin-top: -2px;
    border: solid var(--on-sky);
    border-width: 0 0 2px 2px;
    opacity: 0;
    transform: rotate(-45deg) scale(0.6);
  }
  .swatch:has(:checked) .tick {
    opacity: 1;
    transform: rotate(-45deg);
  }
  @media (pointer: coarse) {
    .swatch {
      width: 44px;
      height: 44px;
    }
  }
</style>
