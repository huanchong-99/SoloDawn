import type texts from './stop.en.json';

declare module './texts' {
  interface Areas {
    stop: typeof texts;
  }
}
