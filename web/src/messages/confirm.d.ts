import type texts from './confirm.en.json';

declare module './texts' {
  interface Areas {
    confirm: typeof texts;
  }
}
