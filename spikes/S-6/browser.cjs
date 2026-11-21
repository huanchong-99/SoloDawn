const { chromium } = require('../../web/node_modules/playwright');
const fs = require('node:fs');

async function main() {
  const input = JSON.parse(fs.readFileSync(0, 'utf8'));
  const browser = await chromium.launch({ executablePath: process.env.CHROME });
  try {
    console.log('browser', browser.version());
    const page = await browser.newPage();
    page.on('console', msg => console.log(msg.text()));
    await page.goto('https://rv.example.test');
    const result = await page.evaluate(async ({ offer, ice }) => {
      const pc = new RTCPeerConnection({ iceServers: [ice], iceTransportPolicy: 'relay' });
      pc.onicecandidateerror = e => console.log(e.url, e.errorCode, e.errorText);
      const echo = new Promise(resolve => {
        pc.ondatachannel = ({ channel }) => {
          channel.onopen = () => channel.send('s6-echo');
          channel.onmessage = e => resolve(e.data);
        };
      });
      const gathered = new Promise(resolve => {
        pc.onicecandidate = ({ candidate }) => {
          if (!candidate) resolve();
          else console.log('candidate', candidate.type, candidate.relayProtocol, candidate.url);
        };
      });
      const roundTrip = async () => {
        await pc.setRemoteDescription(offer);
        await pc.setLocalDescription(await pc.createAnswer());
        await gathered;
        const response = await fetch('/answer', { method: 'POST', body: JSON.stringify(pc.localDescription) });
        if (!response.ok) throw Error('answer HTTP ' + response.status);
        const value = await echo;
        const stats = await pc.getStats();
        const transport = [...stats.values()].find(s => s.type === 'transport' && s.selectedCandidatePairId);
        const pair = stats.get(transport.selectedCandidatePairId);
        const local = stats.get(pair.localCandidateId);
        const remote = stats.get(pair.remoteCandidateId);
        return { echo: value, local: local.candidateType, remote: remote.candidateType,
          relayProtocol: local.relayProtocol, url: local.url };
      };
      try {
        return await Promise.race([roundTrip(), new Promise((_, reject) =>
          setTimeout(() => reject(Error('relay echo deadline')), 20000))]);
      } finally { pc.close(); }
    }, input);
    console.log(JSON.stringify(result));
    if (result.echo !== 's6-echo' || result.local !== 'relay' || result.remote !== 'relay' ||
        result.relayProtocol !== 'tls' || result.url !== input.ice.urls[0]) throw Error('unexpected selected path');
  } finally { await browser.close(); }
}
main().catch(err => { console.error(err); process.exitCode = 1; });
