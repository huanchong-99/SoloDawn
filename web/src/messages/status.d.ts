// The keys of the area `status`, for the type checker (texts.ts).
import type texts from './status.en.json';

declare module './texts' {
  interface Areas {
    status: typeof texts;
  }
}
