import type { ApiType } from './types';

/**
 * Built-in model catalogs for the official provider APIs.
 *
 * Single source of truth for every model dropdown/suggestion list in the app
 * (setup wizard, workflow wizard, model store). Compatible endpoints have no
 * fixed catalog — their models live behind the relay and are fetched live via
 * `/api/models/list` instead.
 *
 * Catalog refreshed 2026-07-05 against the providers' official model docs.
 */
export const OFFICIAL_MODELS: Record<ApiType, readonly string[]> = {
  anthropic: [
    'claude-fable-5',
    'claude-opus-4-8',
    'claude-sonnet-5',
    'claude-haiku-4-5',
    'claude-opus-4-7',
    'claude-opus-4-6',
    'claude-sonnet-4-6',
  ],
  'anthropic-compatible': [],
  google: [
    'gemini-3.5-flash',
    'gemini-3.1-pro-preview',
    'gemini-3.1-flash-lite',
    'gemini-3-flash-preview',
    'gemini-2.5-pro',
    'gemini-2.5-flash',
  ],
  openai: [
    'gpt-5.5',
    'gpt-5.4',
    'gpt-5.4-mini',
    'gpt-5.4-nano',
    'gpt-5.2',
    'gpt-5.1',
  ],
  'openai-compatible': [],
};

/**
 * Compatible types point at user-supplied base URLs (relays/proxies), so the
 * served model list is unknowable ahead of time — these are the only types
 * where fetching the live model list makes sense. Official APIs use
 * {@link OFFICIAL_MODELS} directly.
 */
export function isCompatibleApiType(apiType: string): boolean {
  return apiType === 'anthropic-compatible' || apiType === 'openai-compatible';
}
