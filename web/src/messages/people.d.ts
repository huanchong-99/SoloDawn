import type texts from './people.en.json';

declare module './texts' {
  interface Areas {
    people: typeof texts;
  }
}
