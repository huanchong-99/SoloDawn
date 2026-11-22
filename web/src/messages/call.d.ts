// The keys of the area `call`, for the type checker (texts.ts).
import type texts from './call.en.json';

declare module './texts' {
  interface Areas {
    call: typeof texts;
  }
}
