import type texts from './external.en.json';

declare module './texts' {
  interface Areas {
    external: typeof texts;
  }
}
