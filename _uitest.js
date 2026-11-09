const { JSDOM } = require("jsdom");
const fs = require("fs");
const html = fs.readFileSync("cmd/tp-app/web/index.html", "utf8");
const js = fs.readFileSync("cmd/tp-app/web/app.js", "utf8");
const dom = new JSDOM(html, { runScripts: "outside-only", url: "http://localhost/" });
const { window } = dom;
// 桥 mock：记录调用并返回数据
const calls = [];
window.mygo = {
  call: (name, ...args) => {
    calls.push(name + "(" + args.join(",") + ")");
    if (name === "App.Logs") return Promise.resolve(["l1 ERROR", "l2 INFO", "l3 WARN"]);
    if (name === "App.Status") return Promise.resolve({ state: "已连接", dot: "pass", meta: "", phone: "x", health: "", params: [], events: [], bar: "", version: "v1", logsCount: 3, rtt: "" });
    return Promise.resolve(null);
  },
};
try {
  window.eval(js);
  window.document.dispatchEvent(new window.Event("DOMContentLoaded", { bubbles: true }));
} catch (e) { console.log("EVAL ERROR:", e.message); }
setTimeout(() => {
  const segs = window.document.querySelectorAll(".logbar .seg button");
  console.log("seg 按钮数:", segs.length);
  if (segs.length) {
    segs[2].dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    console.log("点 WARN 后 aria-pressed:", segs[2].getAttribute("aria-pressed"), "| 调用:", calls.filter(c => c.startsWith("App.Logs")).slice(-1));
  }
  const follow = window.document.querySelector(".logbar .follow .switch");
  console.log("跟随开关存在:", !!follow);
  if (follow) {
    const before = follow.getAttribute("aria-checked");
    follow.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    console.log("点跟随: aria-checked", before, "->", follow.getAttribute("aria-checked"));
  }
}, 300);
