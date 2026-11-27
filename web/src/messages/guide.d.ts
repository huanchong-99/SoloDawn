// The keys of the area `guide`, for the type checker (texts.ts).
import type texts from './guide.en.json';

declare module './texts' {
  interface Areas {
    guide: typeof texts;
  }
}
