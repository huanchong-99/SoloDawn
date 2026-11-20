import { expect, it } from 'vitest';
import { server } from 'vitest/browser';
import { decode, ed25519, utf8, verifyEnvelope } from '../src/lib/identity';
import { goldens } from '../src/lib/mock';
import type { Envelope } from '../src/lib/proto';

async function nativeVerify(k: string, s: string, message: string) {
  const key = await crypto.subtle.importKey('raw', decode(k)!, ed25519, false, ['verify']);
  return crypto.subtle.verify(ed25519, key, decode(s)!, utf8(message));
}

it('WebCrypto accepts the identity point; the envelope contract refuses it', async () => {
  const { envelopes }: { envelopes: Envelope[] } = JSON.parse(goldens.crypto);
  const env = envelopes.find((e) => e.k === 'AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA')!;
  expect(await nativeVerify(env.k, env.s, 'solodawn/1\n' + env.p)).toBe(true);
  expect(await verifyEnvelope(crypto, env)).toBe(false);
});

it('RFC 8032 §7.1 test 1: only Apple WebKit refuses the valid empty-message signature', async () => {
  const k = '11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo';
  const s =
    '5VZDAMNgrHKQhuLMgG6CioSHfx645dl02HPgZSJJAVVfuIIVkKM7rMYeOXAc-bRr0lv18FlbviRlUUFDjnoQCw';
  expect(await nativeVerify(k, s, '')).toBe(server.browser !== 'webkit');
});
