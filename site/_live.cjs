const { chromium } = require('playwright');
(async () => {
  const b = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe' });
  const p = await b.newPage({ viewport: { width: 1440, height: 900 } });
  await p.goto('https://haochan1996.github.io/tunnelport/#donate', { waitUntil: 'networkidle', timeout: 45000 });
  const el = await p.$('#donate');
  await el.screenshot({ path: '_live_donate.png' });
  console.log('live shot ok');
  await b.close();
})();
