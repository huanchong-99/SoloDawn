import type texts from './team.en.json';

declare module './texts' {
  interface Areas {
    team: typeof texts;
  }
}
