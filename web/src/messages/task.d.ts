// The keys of the area `task`, for the type checker (texts.ts).
import type texts from './task.en.json';

declare module './texts' {
  interface Areas {
    task: typeof texts;
  }
}
