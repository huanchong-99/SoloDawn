const { chromium, firefox, webkit } = require('playwright');
const { generateKeyPairSync, createPublicKey, randomBytes, sign, verify } = require('node:crypto');
const { execFileSync } = require('node:child_process');
const assert = require('node:assert/strict');
const { setTimeout: sleep } = require('node:timers/promises');
const engines = { chromium, firefox, webkit },
  A = '10.71.0.1',
  B = '10.71.0.2';
const url = (host, path) => 'http://' + host + ':8080' + path;
const get = async (host, path) => (await fetch(url(host, path))).text();
const log = (v) => console.log(JSON.stringify(v));

// Only change rules in the isolated browser namespace. Leave both TURN and signalling reachable.
function gate(op) {
  assert.match(execFileSync('ip', ['netns', 'identify'], { encoding: 'utf8' }), /^s1-browser\s*$/);
  for (const [chain, addr, p] of [
    ['OUTPUT', '-d', '--dports'],
    ['INPUT', '-s', '--sports'],
  ]) {
    if (process.env.S1_DUAL === '1')
      execFileSync('sudo', ['-n', 'ip6tables', op, chain, addr, 'fd71::2', '-j', 'DROP']);
    for (const [proto, opt, ports] of [
      ['tcp', '--ports', '8080'],
      ['udp', p, '3478,22000:22099,5353'],
    ]) {
      execFileSync('sudo', [
        '-n',
        'iptables',
        op,
        chain,
        addr,
        B,
        '-p',
        proto,
        '-m',
        'multiport',
        '!',
        opt,
        ports,
        '-j',
        'DROP',
      ]);
    }
  }
}

async function browsers(name) {
  const browser = await engines[name].launch();
  const page = await browser.newPage();
  const { publicKey, privateKey } = generateKeyPairSync('ed25519');
  const k = publicKey.export({ format: 'jwk' }).x;
  const newID = () => randomBytes(16).toString('base64url');
  const wire = (p) => Buffer.from('solodawn/1\n' + p);
  await page.exposeFunction('newID', newID);
  await page.exposeFunction('signOffer', (o) => {
    const p = JSON.stringify({ ...o, n: newID(), ts: Date.now() });
    return { t: 'to', k: o.to, env: { p, k, s: sign(null, wire(p), privateKey).toString('base64url') } };
  });
  await page.exposeFunction('readAnswer', ({ env }) => {
    const key = createPublicKey({ key: { kty: 'OKP', crv: 'Ed25519', x: env.k }, format: 'jwk' });
    assert(verify(null, wire(env.p), key, Buffer.from(env.s, 'base64url')));
    const p = JSON.parse(env.p);
    assert.equal(p.to, k);
    return p;
  });
  await page.exposeFunction('evidence', (r) => log({ name, ...r }));
  await page.goto('http://127.0.0.1:8080');
  await page.addScriptTag({ path: __dirname + '/browser.js' });
  await page.evaluate((k) => join(k), k);
  log({ name, version: browser.version(), rtc: await page.evaluate(() => typeof RTCPeerConnection) });
  const open = async (mode) => {
    await get(B, '/mode?v=' + (mode === 'silent' ? 'all' : mode));
    return page.evaluate((mode) => openCtl(mode), mode);
  };
  const sample = async (conn) => ({
    echo: await page.evaluate((c) => echo(c), conn),
    ...(await page.evaluate((c) => pairs(c), conn)),
  });
  async function test(mode, fn) {
    if (process.env.S1_CASES && !process.env.S1_CASES.split(',').includes(mode)) return;
    try {
      await fn();
    } catch (e) {
      process.exitCode = 1;
      log({ name, mode, error: e.message.split('\n').slice(0, 2).join(' ') });
    } finally {
      await page.evaluate(() => {
        for (const { pc } of sessions.values()) pc.close();
        sessions.clear();
      });
    }
  }
  try {
    for (const mode of ['direct', 'tcp', 'all', 'silent', 'relay'])
      await test(mode, async () => {
        const conn = await open(mode);
        if (mode === 'direct')
          await page.evaluate(async () => {
            const closed = event(ws, 'close');
            ws.close();
            await closed;
          });
        try {
          log({ name, mode, result: await sample(conn) });
        } finally {
          if (mode === 'direct') await page.evaluate((k) => join(k), k);
        }
      });
    for (const mode of ['renom', 'renom-off'])
      await test(mode, async () => {
        let old;
        gate('-I');
        try {
          old = await open(mode === 'renom' ? 'renom' : 'all');
          const result = await sample(old);
          log({ name, mode, phase: 'blocked', result });
          assert(
            result.node.some((s) => s.startsWith('relay/')),
            'initial pair was not relayed',
          );
        } finally {
          gate('-D');
        }
        const start = Date.now();
        for (let i = 0; i < 7; i++) {
          if (i) await sleep(5000);
          log({ name, mode, phase: 'restored', ms: Date.now() - start, result: await sample(old) });
        }
        const probe = await open('probe');
        log({ name, mode, phase: 'overlap', old: await sample(old), probe: await sample(probe) });
      });
    // Independent of whether a browser automatically switched the all-candidate connection.
    await test('replacement', async () => {
      const old = await open('relay'),
        probe = await open('probe');
      log({ name, phase: 'replacement', old: await sample(old), probe: await sample(probe) });
      await page.evaluate((c) => sessions.get(c).pc.close(), old);
      log({ name, phase: 'after-close', probe: await sample(probe) });
    });
  } finally {
    await browser.close();
  }
}

async function reciprocal(mode) {
  const dial = async (host) => {
    const conn = await get(host, '/dial?mode=' + mode);
    log({ mode, host, dial: conn });
    return conn;
  };
  const sample = async (host, conn, seq) => {
    if (conn === 'timeout') return;
    const msg = host + '-' + seq;
    const pair = JSON.parse(await get(host, '/pair?conn=' + conn));
    const got = await get(host, '/echo?conn=' + conn + '&msg=' + msg);
    log({ mode, host, conn, msg, got, pair: [pair?.Local.port, pair?.Remote.port] });
    if (mode === 'ephemeral' || seq === 0) assert.equal(got, msg);
  };
  const ab = await dial(A);
  await sample(A, ab, 0);
  const ba = await dial(B);
  assert.notEqual(ab, ba);
  for (const seq of [1, 2]) {
    await sample(A, ab, seq);
    await sample(B, ba, seq);
  }
}
(async () => {
  for (const name of process.argv.slice(2).length
    ? process.argv.slice(2)
    : ['nodes', ...Object.keys(engines)]) {
    if (name === 'nodes') {
      await reciprocal('direct');
      await reciprocal('ephemeral');
    } else await browsers(name);
  }
})().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
