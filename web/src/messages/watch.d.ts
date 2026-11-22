import type texts from './watch.en.json';

declare module './texts' {
  interface Areas {
    watch: typeof texts;
  }
}
