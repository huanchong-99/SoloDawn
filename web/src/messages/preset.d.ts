import type texts from './preset.en.json';

declare module './texts' {
  interface Areas {
    preset: typeof texts;
  }
}
