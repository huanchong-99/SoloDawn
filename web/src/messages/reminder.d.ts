// The keys of the area `reminder`, for the type checker (texts.ts).
import type texts from './reminder.en.json';

declare module './texts' {
  interface Areas {
    reminder: typeof texts;
  }
}
