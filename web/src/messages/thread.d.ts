// The keys of the area `thread`, for the type checker (texts.ts).
import type texts from './thread.en.json';

declare module './texts' {
  interface Areas {
    thread: typeof texts;
  }
}
