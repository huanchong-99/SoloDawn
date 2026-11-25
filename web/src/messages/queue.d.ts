import type texts from './queue.en.json';

declare module './texts' {
  interface Areas {
    queue: typeof texts;
  }
}
