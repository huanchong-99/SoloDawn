import type texts from './forge.en.json';

declare module './texts' {
  interface Areas {
    forge: typeof texts;
  }
}
