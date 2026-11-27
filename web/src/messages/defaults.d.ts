// The keys of the area `defaults`, for the type checker (texts.ts).
import type texts from './defaults.en.json';

declare module './texts' {
  interface Areas {
    defaults: typeof texts;
  }
}
