// The keys of the area `member`, for the type checker (texts.ts).
import type texts from './member.en.json';

declare module './texts' {
  interface Areas {
    member: typeof texts;
  }
}
