import type texts from './permit.en.json';

declare module './texts' {
  interface Areas {
    permit: typeof texts;
  }
}
