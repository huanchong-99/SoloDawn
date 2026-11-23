// The keys of the area `model`, for the type checker (texts.ts).
import type texts from './model.en.json';

declare module './texts' {
  interface Areas {
    model: typeof texts;
  }
}
