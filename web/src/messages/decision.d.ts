// The keys of the area `decision`, for the type checker (texts.ts).
import type texts from './decision.en.json';

declare module './texts' {
  interface Areas {
    decision: typeof texts;
  }
}
