// The keys of the area `states`, for the type checker (texts.ts).
import type texts from './states.en.json';

declare module './texts' {
  interface Areas {
    states: typeof texts;
  }
}
