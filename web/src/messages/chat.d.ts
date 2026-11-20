// The keys of the area `chat`, for the type checker (texts.ts).
import type texts from './chat.en.json';

declare module './texts' {
  interface Areas {
    chat: typeof texts;
  }
}
