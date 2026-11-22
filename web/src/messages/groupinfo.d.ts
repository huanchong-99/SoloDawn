// The keys of the area `groupinfo`, for the type checker (texts.ts).
import type texts from './groupinfo.en.json';

declare module './texts' {
  interface Areas {
    groupinfo: typeof texts;
  }
}
