// The keys of the area `settings`, for the type checker (texts.ts).
import type texts from './settings.en.json';

declare module './texts' {
  interface Areas {
    settings: typeof texts;
  }
}
