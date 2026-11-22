import type texts from './move.en.json';

declare module './texts' {
  interface Areas {
    move: typeof texts;
  }
}
