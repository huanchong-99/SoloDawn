// The keys of the area `groups`, for the type checker (texts.ts).
import type texts from './groups.en.json';

declare module './texts' {
  interface Areas {
    groups: typeof texts;
  }
}
