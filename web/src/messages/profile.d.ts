// The keys of the area `profile`, for the type checker (texts.ts).
import type texts from './profile.en.json';

declare module './texts' {
  interface Areas {
    profile: typeof texts;
  }
}
