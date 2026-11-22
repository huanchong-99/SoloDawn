import type texts from './allow.en.json';

declare module './texts' {
  interface Areas {
    allow: typeof texts;
  }
}
