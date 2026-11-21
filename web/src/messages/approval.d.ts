// The keys of the area `approval`, for the type checker (texts.ts).
import type texts from './approval.en.json';

declare module './texts' {
  interface Areas {
    approval: typeof texts;
  }
}
