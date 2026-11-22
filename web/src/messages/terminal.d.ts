// The keys of the area `terminal`, for the type checker (texts.ts).
import type texts from './terminal.en.json';

declare module './texts' {
  interface Areas {
    terminal: typeof texts;
  }
}
