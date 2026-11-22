// The keys of the area `notify`, for the type checker (texts.ts).
import type texts from './notify.en.json';

declare module './texts' {
  interface Areas {
    notify: typeof texts;
  }
}
