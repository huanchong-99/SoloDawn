import type texts from './kb.en.json';

declare module './texts' {
  interface Areas {
    kb: typeof texts;
  }
}
