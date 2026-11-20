// SoloDawn 样稿公共脚本：主题、语言、图标、外壳（导航 + 全局状态）、急停、复制。
// 只为演示交互，没有任何依赖。正式前端里这些会是组件。
const root = document.documentElement;
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const mem = {
  get: (k) => {
    try {
      return localStorage.getItem('sd-' + k);
    } catch {
      return null;
    }
  },
  set: (k, v) => {
    try {
      localStorage.setItem('sd-' + k, v);
    } catch {}
  },
};

// ── 主题：auto 跟随系统，light / dark 强制 ──
function setTheme(t) {
  t === 'auto' ? delete root.dataset.theme : (root.dataset.theme = t);
  mem.set('theme', t);
  $$('input[name=theme]').forEach((i) => (i.checked = i.value === t));
}
const isDark = () =>
  (root.dataset.theme ||
    (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')) === 'dark';

// ── 语言：带 data-en 的元素在中英文之间切换；用户发的消息不翻译 ──
function setLang(l) {
  root.lang = l === 'en' ? 'en' : 'zh-CN';
  mem.set('lang', l);
  $$('[data-en]').forEach((e) => {
    e.dataset.zh ??= e.innerHTML;
    e.innerHTML = l === 'en' ? e.dataset.en : e.dataset.zh;
  });
  $$('[data-en-label]').forEach((e) => {
    e.dataset.zhLabel ??= e.getAttribute('aria-label');
    e.setAttribute('aria-label', l === 'en' ? e.dataset.enLabel : e.dataset.zhLabel);
  });
  $$('input[name=lang]').forEach((i) => (i.checked = i.value === l));
}
setTheme(mem.get('theme') || 'auto'); // 在 <head> 里就执行，避免闪一下

// ── 图标：自己画的 24×24 线性图标，统一 1.75 线宽 ──
const ICONS = {
  chat: '<path d="M5 4h14a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H9l-5 4V5a1 1 0 0 1 1-1z"/>',
  pc: '<path d="M3 5h18v11H3zM8 20h8M12 16v4"/>',
  sliders:
    '<path d="M4 7h9M19 7h1M4 17h1M11 17h9"/><circle cx="16" cy="7" r="2.5"/><circle cx="8" cy="17" r="2.5"/>',
  terminal: '<path d="M4 5h16v14H4zM8 10l3 2-3 2M13 15h3"/>',
  pause: '<path d="M9 5v14M15 5v14"/>',
  play: '<path d="M7 5l12 7-12 7z"/>',
  check: '<path d="M5 12.5l4.5 4.5L19 7.5"/>',
  x: '<path d="M6 6l12 12M18 6L6 18"/>',
  chev: '<path d="M6 9l6 6 6-6"/>',
  back: '<path d="M15 5l-7 7 7 7"/>',
  arrow: '<path d="M5 12h13M13 6l6 6-6 6"/>',
  copy: '<path d="M9 9h11v11H9zM5 15H4V4h11v1"/>',
  branch:
    '<circle cx="6" cy="5" r="2"/><circle cx="6" cy="19" r="2"/><circle cx="18" cy="7" r="2"/><path d="M6 7v10M18 9c0 5-12 3-12 8"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  moon: '<path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.500 10.500z"/>',
  shield: '<path d="M12 3l8 3v6c0 4.500-3.200 8-8 9-4.800-1-8-4.500-8-9V6l8-3z"/>',
  lock: '<path d="M6 11h12v9H6zM8.500 11V8a3.500 3.500 0 0 1 7 0v3"/>',
  bell: '<path d="M6 16v-5a6 6 0 0 1 12 0v5l1.500 2h-15L6 16zM10 21h4"/>',
  users:
    '<circle cx="9" cy="8" r="3"/><path d="M3 20c0-3.300 2.700-6 6-6s6 2.700 6 6M16 5.200a3 3 0 0 1 0 5.600M17 14.300c2.300.8 4 3 4 5.700"/>',
  at: '<circle cx="12" cy="12" r="3.500"/><path d="M15.500 8.500v5a2.500 2.500 0 0 0 5 0V12a8.500 8.500 0 1 0-3.400 6.800"/>',
  send: '<path d="M4 12l16-8-6 16-3-7-7-1z"/>',
  alert: '<path d="M12 4l9 16H3l9-16zM12 10v4M12 17v.5"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.500v.5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  direct:
    '<circle cx="5" cy="12" r="2.500"/><circle cx="19" cy="12" r="2.500"/><path d="M7.500 12h9"/>',
  relay:
    '<circle cx="4.500" cy="17" r="2.200"/><circle cx="12" cy="6" r="2.200"/><circle cx="19.500" cy="17" r="2.200"/><path d="M5.800 15.200l4.900-7.400M13.300 7.800l4.900 7.400"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  home: '<path d="M4 11l8-7 8 7v9H4zM10 20v-6h4v6"/>',
  key: '<circle cx="8" cy="15" r="4"/><path d="M11 12l9-9M16 7l3 3"/>',
  globe:
    '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18"/>',
  power: '<path d="M12 3v9M6.300 6.300a8 8 0 1 0 11.400 0"/>',
  eye: '<path d="M2 12s3.500-7 10-7 10 7 10 7-3.500 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
  doc: '<path d="M6 3h9l4 4v14H6zM14 3v5h5M9 13h7M9 17h5"/>',
  fork: '<path d="M6 4v5c0 3.500 6 3.500 6 7v4M18 4v5c0 3.500-6 3.500-6 7"/>',
  phone: '<path d="M7 3h10v18H7zM11 18h2"/>',
};
// 标志：墨色方块上一道破晓的弧光（从轨道上看到的地平线）
const LOGO =
  '<rect width="32" height="32" rx="9" fill="#0C0F16"/><path d="M5 22.500Q16 11.500 27 22.500" fill="none" stroke="url(#sd-arc)" stroke-width="2.400" stroke-linecap="round"/><circle cx="16" cy="13" r="1.600" fill="#F6B152"/>';
// 品牌标志：@lobehub/icons-static-svg 1.104.0（MIT）。原文件在 icons/，这里把路径原样内联，
// 好让 OpenAI 的单色版跟着 currentColor 走（深色主题自动变白）。各标志为其所有者的商标，见README.md「品牌标志」。
const BRANDS = {
  claude:
    '<path d="M4.709 15.955l4.72-2.647.08-.23-.08-.128H9.2l-.79-.048-2.698-.073-2.339-.097-2.266-.122-.571-.121L0 11.784l.055-.352.48-.321.686.06 1.52.103 2.278.158 1.652.097 2.449.255h.389l.055-.157-.134-.098-.103-.097-2.358-1.596-2.552-1.688-1.336-.972-.724-.491-.364-.462-.158-1.008.656-.722.881.06.225.061.893.686 1.908 1.476 2.491 1.833.365.304.145-.103.019-.073-.164-.274-1.355-2.446-1.446-2.49-.644-1.032-.17-.619a2.97 2.97 0 01-.104-.729L6.283.134 6.696 0l.996.134.42.364.62 1.414 1.002 2.229 1.555 3.03.456.898.243.832.091.255h.158V9.01l.128-1.706.237-2.095.23-2.695.08-.76.376-.91.747-.492.584.28.48.685-.067.444-.286 1.851-.559 2.903-.364 1.942h.212l.243-.242.985-1.306 1.652-2.064.73-.82.85-.904.547-.431h1.033l.76 1.129-.34 1.166-1.064 1.347-.881 1.142-1.264 1.7-.79 1.36.073.11.188-.02 2.856-.606 1.543-.28 1.841-.315.833.388.091.395-.328.807-1.969.486-2.309.462-3.439.813-.042.03.049.061 1.549.146.662.036h1.622l3.02.225.79.522.474.638-.079.485-1.215.62-1.64-.389-3.829-.91-1.312-.329h-.182v.11l1.093 1.068 2.006 1.81 2.509 2.33.127.578-.322.455-.34-.049-2.205-1.657-.851-.747-1.926-1.62h-.128v.17l.444.649 2.345 3.521.122 1.08-.17.353-.608.213-.668-.122-1.374-1.925-1.415-2.167-1.143-1.943-.14.08-.674 7.254-.316.37-.729.28-.607-.461-.322-.747.322-1.476.389-1.924.315-1.53.286-1.9.17-.632-.012-.042-.14.018-1.434 1.967-2.18 2.945-1.726 1.845-.414.164-.717-.37.067-.662.401-.589 2.388-3.036 1.44-1.882.93-1.086-.006-.158h-.055L4.132 18.56l-1.13.146-.487-.456.061-.746.231-.243 1.908-1.312-.006.006z" fill="#D97757" fill-rule="nonzero"></path>',
  openai:
    '<path fill="currentColor" fill-rule="evenodd" d="M9.205 8.658v-2.26c0-.19.072-.333.238-.428l4.543-2.616c.619-.357 1.356-.523 2.117-.523 2.854 0 4.662 2.212 4.662 4.566 0 .167 0 .357-.024.547l-4.71-2.759a.797.797 0 00-.856 0l-5.97 3.473zm10.609 8.8V12.06c0-.333-.143-.57-.429-.737l-5.97-3.473 1.95-1.118a.433.433 0 01.476 0l4.543 2.617c1.309.76 2.189 2.378 2.189 3.948 0 1.808-1.07 3.473-2.76 4.163zM7.802 12.703l-1.95-1.142c-.167-.095-.239-.238-.239-.428V5.899c0-2.545 1.95-4.472 4.591-4.472 1 0 1.927.333 2.712.928L8.23 5.067c-.285.166-.428.404-.428.737v6.898zM12 15.128l-2.795-1.57v-3.33L12 8.658l2.795 1.57v3.33L12 15.128zm1.796 7.23c-1 0-1.927-.332-2.712-.927l4.686-2.712c.285-.166.428-.404.428-.737v-6.898l1.974 1.142c.167.095.238.238.238.428v5.233c0 2.545-1.974 4.472-4.614 4.472zm-5.637-5.303l-4.544-2.617c-1.308-.761-2.188-2.378-2.188-3.948A4.482 4.482 0 014.21 6.327v5.423c0 .333.143.571.428.738l5.947 3.449-1.95 1.118a.432.432 0 01-.476 0zm-.262 3.9c-2.688 0-4.662-2.021-4.662-4.519 0-.19.024-.38.047-.57l4.686 2.71c.286.167.571.167.856 0l5.97-3.448v2.26c0 .19-.07.333-.237.428l-4.543 2.616c-.619.357-1.356.523-2.117.523zm5.899 2.83a5.947 5.947 0 005.827-4.756C22.287 18.339 24 15.84 24 13.296c0-1.665-.713-3.282-1.998-4.448.119-.5.19-.999.19-1.498 0-3.401-2.759-5.947-5.946-5.947-.642 0-1.26.095-1.88.31A5.962 5.962 0 0010.205 0a5.947 5.947 0 00-5.827 4.757C1.713 5.447 0 7.945 0 10.49c0 1.666.713 3.283 1.998 4.448-.119.5-.19 1-.19 1.499 0 3.401 2.759 5.946 5.946 5.946.642 0 1.26-.095 1.88-.309a5.96 5.96 0 004.162 1.713z"></path>',
  gemini:
    '<defs><path id="b-gem-p" d="M20.616 10.835a14.147 14.147 0 01-4.45-3.001 14.111 14.111 0 01-3.678-6.452.503.503 0 00-.975 0 14.134 14.134 0 01-3.679 6.452 14.155 14.155 0 01-4.45 3.001c-.65.28-1.318.505-2.002.678a.502.502 0 000 .975c.684.172 1.35.397 2.002.677a14.147 14.147 0 014.45 3.001 14.112 14.112 0 013.679 6.453.502.502 0 00.975 0c.172-.685.397-1.351.677-2.003a14.145 14.145 0 013.001-4.45 14.113 14.113 0 016.453-3.678.503.503 0 000-.975 13.245 13.245 0 01-2.003-.678z"/><linearGradient gradientUnits="userSpaceOnUse" id="lobe-icons-gemini-0-_R_0_" x1="7" x2="11" y1="15.5" y2="12"><stop stop-color="#08B962"></stop><stop offset="1" stop-color="#08B962" stop-opacity="0"></stop></linearGradient><linearGradient gradientUnits="userSpaceOnUse" id="lobe-icons-gemini-1-_R_0_" x1="8" x2="11.5" y1="5.5" y2="11"><stop stop-color="#F94543"></stop><stop offset="1" stop-color="#F94543" stop-opacity="0"></stop></linearGradient><linearGradient gradientUnits="userSpaceOnUse" id="lobe-icons-gemini-2-_R_0_" x1="3.5" x2="17.5" y1="13.5" y2="12"><stop stop-color="#FABC12"></stop><stop offset=".46" stop-color="#FABC12" stop-opacity="0"></stop></linearGradient></defs><use href="#b-gem-p" fill="#3186FF"/><use href="#b-gem-p" fill="url(#lobe-icons-gemini-0-_R_0_)"/><use href="#b-gem-p" fill="url(#lobe-icons-gemini-1-_R_0_)"/><use href="#b-gem-p" fill="url(#lobe-icons-gemini-2-_R_0_)"/>',
};
const SPRITE = `<svg width="0" height="0" style="position:absolute" aria-hidden="true"><defs>
  <linearGradient id="sd-arc" x1="0" x2="1"><stop offset="0" stop-color="#4A5FE0"/><stop offset=".5" stop-color="#F9C987"/><stop offset="1" stop-color="#C7609A"/></linearGradient></defs>
  <symbol id="logo" viewBox="0 0 32 32">${LOGO}</symbol>
  ${Object.entries(ICONS)
    .map(([k, v]) => `<symbol id="i-${k}" viewBox="0 0 24 24">${v}</symbol>`)
    .join('')}
  ${Object.entries(BRANDS)
    .map(([k, v]) => `<symbol id="b-${k}" viewBox="0 0 24 24">${v}</symbol>`)
    .join('')}</svg>`;
const icon = (n, c = '') => `<svg class="i ${c}" aria-hidden="true"><use href="#i-${n}"/></svg>`;

// ── 外壳：左侧导航（手机上变成底部导航）+ 顶栏右侧的全局状态 ──
const NAV = [
  ['groups', 'chat', '群聊', 'Chats'],
  ['nodes', 'pc', '电脑和 AI', 'Computers'],
  ['settings', 'sliders', '设置', 'Settings'],
];
function shell() {
  const b = document.body,
    cur = b.dataset.nav;
  b.insertAdjacentHTML(
    'afterbegin',
    `
    <nav class="rail" aria-label="主导航" data-en-label="Main">
      <a class="logo" href="index.html" aria-label="SoloDawn 样稿目录"><svg class="logo-mark"><use href="#logo"/></svg></a>
      ${NAV.map(([p, i, zh, en]) => `<a href="${p}.html" ${p === cur ? 'aria-current="page"' : ''}>${icon(i)}<span data-en="${en}">${zh}</span></a>`).join('')}
      <span class="av me" style="--h:40" title="Alex（你）">A</span>
    </nav>`,
  );
  document.querySelector('.bar').insertAdjacentHTML(
    'beforeend',
    `
    <div class="global">
      <a class="conn st" data-st="ok" href="nodes.html" aria-label="连接状态：直连，延迟 18 毫秒" data-en-label="Connection: direct, 18 ms"><i></i><span data-en="Direct">直连</span><span class="ms muted">18 ms</span></a>
      <button class="btn btn-sm estop" data-act="estop" aria-haspopup="dialog" aria-label="全部暂停" data-en-label="Pause all AIs">${icon('pause', 'sm')}<span data-en="Pause all">全部暂停</span></button>
      <button class="icon-btn only-wide" data-act="theme" aria-label="切换深色 / 浅色" data-en-label="Toggle theme">${icon('moon')}</button>
      <button class="icon-btn only-wide" data-act="lang" aria-label="Switch language / 切换语言">EN</button>
    </div>`,
  );
  document.querySelector('.bar').insertAdjacentHTML(
    'afterend',
    `
    <div class="ban" role="status">${icon('pause')}<span class="grow"><b data-en="All AIs paused.">AI 已全部暂停。</b> <span data-en="Work in progress is saved; nothing new will start.">进行中的任务已保存现场，不会开始新的操作。</span></span>
      <button class="btn btn-sm" data-act="resume">${icon('play', 'sm')}<span data-en="Resume">恢复运行</span></button></div>
    <dialog class="dlg" id="estop" aria-labelledby="estop-h"><form method="dialog">
      <h2 id="estop-h" data-en="Pause all AIs?">要全部暂停吗？</h2>
      <label class="choice"><input type="radio" name="scope" checked><span><b data-en="Every AI in this group">本群的全部 AI</b><span class="muted" data-en="3 are working now. They stop after the current step.">现在有 3 个在干活，做完手上这一步就停。</span></span></label>
      <label class="choice"><input type="radio" name="scope"><span><b data-en="All my AIs, in every group">我的全部 AI（所有群）</b><span class="muted" data-en="Takes my computers' AIs offline. Only the owner can do this.">让我电脑上的 AI 全部下线，只有主人能按。</span></span></label>
      <div class="row" style="justify-content:flex-end"><button class="btn" value="no" data-en="Cancel">取消</button><button class="btn btn-danger" value="yes" data-en="Pause">暂停</button></div>
    </form></dialog>`,
  );
  const dlg = document.getElementById('estop');
  dlg.addEventListener('close', () => {
    if (dlg.returnValue === 'yes') root.dataset.paused = '';
    dlg.returnValue = '';
  });
}

addEventListener('DOMContentLoaded', () => {
  document.body.insertAdjacentHTML('afterbegin', SPRITE);
  if (document.body.classList.contains('app')) shell();
  setTheme(mem.get('theme') || 'auto');
  if (mem.get('lang') === 'en') setLang('en');
  syncThemeIcon();
});
const syncThemeIcon = () =>
  $$('[data-act=theme] use').forEach((u) =>
    u.setAttribute('href', isDark() ? '#i-sun' : '#i-moon'),
  );

// ── 点击：全部走 data-act，一个监听器 ──
addEventListener('click', (e) => {
  const t = e.target.closest('[data-act]');
  if (!t) return;
  const act = t.dataset.act;
  if (act === 'theme') {
    setTheme(isDark() ? 'light' : 'dark');
    syncThemeIcon();
  }
  if (act === 'lang') setLang(root.lang === 'en' ? 'zh' : 'en');
  if (act === 'estop') document.getElementById('estop').showModal();
  if (act === 'resume') delete root.dataset.paused;
  if (act === 'copy') {
    const text = document.querySelector(t.dataset.from).innerText;
    (navigator.clipboard?.writeText(text) ?? Promise.reject()).catch(() => {
      const r = document.createRange();
      r.selectNodeContents(document.querySelector(t.dataset.from));
      getSelection().removeAllRanges();
      getSelection().addRange(r);
      document.execCommand('copy');
    });
    const label = t.querySelector('span'),
      old = label.innerHTML;
    label.textContent = root.lang === 'en' ? 'Copied' : '已复制';
    setTimeout(() => (label.innerHTML = old), 1800);
  }
});
addEventListener('change', (e) => {
  if (e.target.name === 'theme') {
    setTheme(e.target.value);
    syncThemeIcon();
  }
  if (e.target.name === 'lang') setLang(e.target.value);
});
