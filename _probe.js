const { JSDOM } = require("jsdom");
const fs = require("fs");
const html = fs.readFileSync("cmd/tp-app/web/index.html", "utf8");
const js = fs.readFileSync("cmd/tp-app/web/app.js", "utf8");
const dom = new JSDOM(html, { runScripts: "outside-only", url: "http://localhost/" });
const { window } = dom;
window.mygo = { call: () => Promise.resolve(null) };
// 探针：记录该元素 setAttribute 调用
const calls = [];
const origSet = window.Element.prototype.setAttribute;
window.Element.prototype.setAttribute = function (k, v) {
  if (this.getAttribute && this.getAttribute("aria-label") === "跟随最新日志") calls.push(k + "=" + v);
  return origSet.call(this, k, v);
};
try {
  window.eval(js);
  window.document.dispatchEvent(new window.Event("DOMContentLoaded", { bubbles: true }));
} catch (e) { console.log("EVAL ERROR:", e.message); }
setTimeout(() => {
  const follow = window.document.querySelector(".logbar .follow .switch");
  console.log("click 前:", follow.getAttribute("aria-checked"));
  follow.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
  console.log("click 后:", follow.getAttribute("aria-checked"), "| setAttribute 调用:", JSON.stringify(calls));
}, 300);
