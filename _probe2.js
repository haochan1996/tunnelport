const { JSDOM } = require("jsdom");
const fs = require("fs");
const html = fs.readFileSync("cmd/tp-app/web/index.html", "utf8");
const js = fs.readFileSync("cmd/tp-app/web/app.js", "utf8");
const dom = new JSDOM(html, { runScripts: "outside-only", url: "http://localhost/" });
const { window } = dom;
window.mygo = { call: () => Promise.resolve(null) };
const stacks = [];
const origSet = window.Element.prototype.setAttribute;
window.Element.prototype.setAttribute = function (k, v) {
  if (this.getAttribute && this.getAttribute("aria-label") === "跟随最新日志") {
    stacks.push((new Error().stack || "").split("\n").slice(1, 4).join(" | "));
  }
  return origSet.call(this, k, v);
};
try {
  window.eval(js);
  window.document.dispatchEvent(new window.Event("DOMContentLoaded", { bubbles: true }));
} catch (e) { console.log("EVAL ERROR:", e.message); }
setTimeout(() => {
  const follow = window.document.querySelector(".logbar .follow .switch");
  follow.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
  stacks.forEach((s, i) => console.log("写入#" + (i + 1) + ":", s.replace(/file:\/\//g, "")));
}, 300);
