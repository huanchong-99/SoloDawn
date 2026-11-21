// The keys of the area `summary`, for the type checker (texts.ts).
import type texts from './summary.en.json';

declare module './texts' {
  interface Areas {
    summary: typeof texts;
  }
}
