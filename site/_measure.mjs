import { chromium } from 'playwright';
const b = await chromium.launch({ executablePath: 'C:\Program Files\Google\Chrome\Application\chrome.exe' });
const p = await b.newPage({ viewport: { width: 390, height: 800 } });
await p.goto('file:///D:/ai/orca/workspace/tp-server/site/index.html');
const r = await p.evaluate(() => {
  const docW = document.scrollingElement.scrollWidth;
  const bad = [];
  document.querySelectorAll('*').forEach(el => {
    const w = el.scrollWidth;
    const rect = el.getBoundingClientRect();
    if (rect.width > 391 || (w > rect.width + 2 && getComputedStyle(el).overflowX === 'visible')) {
      bad.push({ tag: el.tagName, cls: el.className && String(el.className).slice(0,40), rectW: Math.round(rect.width), scrollW: w, text: (el.textContent||'').trim().slice(0,30) });
    }
  });
  return { docW, bodyW: document.body.scrollWidth, bad: bad.slice(0, 25) };
});
console.log(JSON.stringify(r, null, 1));
await b.close();
