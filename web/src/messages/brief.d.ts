// The keys of the area `brief`, for the type checker (texts.ts).
import type texts from './brief.en.json';

declare module './texts' {
  interface Areas {
    brief: typeof texts;
  }
}
