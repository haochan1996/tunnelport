const { JSDOM } = require("jsdom");
const fs = require("fs");
const html = fs.readFileSync("cmd/tp-app/web/index.html", "utf8");
const js = fs.readFileSync("cmd/tp-app/web/app.js", "utf8");
const dom = new JSDOM(html, { runScripts: "outside-only", url: "http://localhost/" });
const { window } = dom;
window.mygo = { call: () => Promise.resolve(null) };
window.eval(js);
// 不手动派发：让 jsdom 自然触发 DOMContentLoaded（避免 wire 双跑伪影）
setTimeout(() => {
  const follow = window.document.querySelector(".logbar .follow .switch");
  const before = follow.getAttribute("aria-checked");
  follow.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
  console.log("跟随开关:", before, "->", follow.getAttribute("aria-checked"));
  const segs = window.document.querySelectorAll(".logbar .seg button");
  segs[3].dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
  console.log("点 ERROR: aria-pressed =", segs[3].getAttribute("aria-pressed"));
}, 500);
