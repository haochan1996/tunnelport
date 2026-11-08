// tunnelport 页面胶水：从 Go 服务（App.*）取真实数据填充设计稿。
// mygo.runtime.call 由 MyGO 自动注入的桥提供（window.mygo）。
(function () {
  "use strict";
  var call = function (m) {
    var a = Array.prototype.slice.call(arguments, 1);
    return window.mygo.runtime.call("App." + m, ...a);
  };
  var q = function (s, r) { return (r || document).querySelector(s); };
  var qa = function (s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); };

  // ---------- 状态屏 ----------
  function paintStatus(st) {
    var hero = q(".status-hero");
    if (hero) {
      var dot = q(".dot", hero);
      if (dot) dot.className = "dot dot--" + st.dot;
      // 更新文字节点（保留 dot 子元素）
      var t = "";
      hero.childNodes.forEach(function (n) { if (n.nodeType === 3) t = n; });
      if (t) t.nodeValue = st.state;
    }
    var meta = q(".page-meta");
    if (meta) meta.textContent = st.meta;

    qa(".entry__v").forEach(function (e) { e.textContent = st.phone; });
    qa("[data-copy]").forEach(function (b) {
      if (b.dataset.copy && b.dataset.copy.indexOf(":") >= 0) b.dataset.copy = st.phone;
    });

    // 接入参数
    var params = q('section[aria-label="接入参数"]');
    if (params && st.params) {
      var rows = qa(".lrow", params);
      st.params.forEach(function (kv, i) {
        if (!rows[i]) return;
        var k = q(".lrow__key", rows[i]); if (k) k.textContent = kv[0];
        var v = q(".lrow__val", rows[i]); if (v) v.textContent = kv[1];
      });
    }

    // 最近事件（深色终端条）
    var ev = q('section[aria-label="最近事件"]');
    if (ev && st.events) {
      var box = q(".term", ev) || q(".term__line", ev) ? (q(".term", ev) || ev) : null;
      if (box) {
        var cls = box.className.indexOf("term") >= 0 ? box : ev;
        var host = q(".term", ev) || box;
        host.innerHTML = "";
        st.events.slice(-5).forEach(function (l) {
          var d = document.createElement("div");
          d.className = "term__line" + (l.indexOf("ERROR") >= 0 ? " term__hint" : l.indexOf("WARN") >= 0 ? "" : " term__dim");
          d.textContent = l;
          host.appendChild(d);
        });
      }
    }

    // 底部状态栏
    var sb = q(".statusbar");
    if (sb) {
      var spans = qa("span", sb);
      if (spans[0]) spans[0].textContent = st.bar;
      if (spans[1] && st.reconnects !== undefined) spans[1].textContent = "重连 " + (st.reconnects || 0) + " 次 / 24 h";
    }
  }

  // ---------- 诊断屏 ----------
  function paintDoctor(d) {
    var v = q("#view-doctor");
    if (!v) return;
    var rows = qa(".readout__seg", v).map(function (seg) { return seg.parentElement; });
    (d.rows || []).forEach(function (r, i) {
      var row = rows[i]; if (!row) return;
      var mark = q(".readout__mark", row); if (mark) mark.textContent = r.state === "通过" ? "✔" : r.state === "失败" ? "✘" : "–";
      var meta = q(".readout__meta", row); if (meta) meta.textContent = r.name;
      var val = q(".readout__val", row); if (val) val.textContent = r.reading;
      var seg = q(".readout__seg", row); if (seg) seg.textContent = r.fact;
    });
    var verdict = q(".verdict", v) || q(".band__verdict", v);
    if (verdict) verdict.textContent = d.verdict;
    // 判定行的琥珀提示
    qa(".term__hint, .hint", v).forEach(function (e) {
      if (e.textContent.indexOf("health") >= 0 || e.textContent.indexOf("4G") >= 0) {
        e.textContent = "外部段需手机 4G 打开 " + d.healthUrl + "，显示 OK 即全链路通畅。";
      }
    });
    var meta = q(".page-meta", v);
    if (meta) meta.textContent = "上次诊断 " + d.at + " · 耗时 " + d.duration + " · 每段独立判定";
  }

  // ---------- 日志屏 ----------
  function paintLogs(lines) {
    var v = q("#view-logs");
    if (!v) return;
    var host = q(".term", v) || q(".term__line", v)?.parentElement;
    if (!host) return;
    host.innerHTML = "";
    (lines || []).slice(-60).forEach(function (l) {
      var d = document.createElement("div");
      d.className = "term__line" + (l.indexOf("ERROR") >= 0 ? " term__hint" : l.indexOf("WARN") >= 0 ? "" : " term__dim");
      d.textContent = l;
      host.appendChild(d);
    });
  }

  // ---------- 交互 ----------
  function wire() {
    // 复制按钮（设计稿自带 data-copy，接管到 Go 剪贴板）
    document.addEventListener("click", function (e) {
      var t = e.target.closest("[data-copy]");
      if (!t) return;
      e.preventDefault();
      call("Copy", t.dataset.copy).then(function () {
        var old = t.textContent;
        t.textContent = "已复制";
        setTimeout(function () { t.textContent = old; }, 1200);
      });
    }, true);

    // 二维码弹窗：拉真实 QR
    document.addEventListener("click", function (e) {
      if (e.target.closest("#open-qr")) {
        call("QR").then(function (uri) {
          var box = q("#qr-svg");
          if (!box) return;
          var img = document.createElement("img");
          img.src = uri;
          img.alt = "入口地址二维码";
          img.style.cssText = "width:220px;height:220px;image-rendering:pixelated;background:#fff;padding:8px;border-radius:8px";
          box.replaceWith(img);
        }).catch(function () {});
      }
    });

    // 重新诊断（data-goto=doctor 或 诊断屏按钮）
    document.addEventListener("click", function (e) {
      if (e.target.closest("[data-goto=doctor]") || (e.target.closest(".btn--primary") && q("#view-doctor.is-active, #view-doctor.active"))) {
        call("Doctor").then(paintDoctor).catch(function () {});
      }
    });

    // 设置屏：开关类行按 key 分发（开机自启 / 最小化到托盘）
    var settings = q("#view-settings");
    if (settings) {
      var dispatch = {
        "开机自启": ["Autostart", "SetAutostart"],
        "最小化到托盘": ["MinimizeToTray", "SetMinimizeToTray"]
      };
      qa(".srow", settings).forEach(function (row) {
        var key = q(".srow__key", row);
        if (!key) return;
        var pair = null;
        Object.keys(dispatch).forEach(function (k) {
          if (key.textContent.indexOf(k) >= 0) pair = dispatch[k];
        });
        if (!pair) return;
        var input = q("input[type=checkbox], input[type=radio], button[role=switch]", row);
        if (!input) return;
        call(pair[0]).then(function (on) {
          if (input.type === "checkbox" || input.type === "radio") input.checked = !!on;
          else input.setAttribute("aria-checked", String(!!on));
        }).catch(function () {});
        input.addEventListener("change", function () {
          var on = input.type === "checkbox" || input.type === "radio" ? input.checked : input.getAttribute("aria-checked") !== "true";
          call(pair[1], on).catch(function () {});
        });
      });
    }

    // 日志屏：级别/搜索 → 重新拉取（简单轮询）
    setInterval(function () {
      if (!location.hash || location.hash.indexOf("logs") < 0) return;
      var level = 0;
      call("Logs", level, "").then(paintLogs).catch(function () {});
    }, 4000);

    // 配对页（pairing.html）：连接按钮
    var connectBtn = q("#connect");
    if (connectBtn) {
      var field = q("input.field, input[type=text]");
      connectBtn.addEventListener("click", function () {
        var url = field ? field.value.trim() : "";
        call("Connect", url).then(function () {
          location.href = "/"; // 连上后跳主界面
        }).catch(function (err) {
          var msg = q(".pair__err") || q(".field-error");
          if (msg) msg.textContent = String(err && err.message || err);
        });
      });
    }
  }

  // ---------- 轮询 ----------
  function tick() {
    call("Status").then(paintStatus).catch(function () {});
    if (location.hash.indexOf("doctor") >= 0) call("Doctor").then(paintDoctor).catch(function () {});
    if (location.hash.indexOf("logs") >= 0) call("Logs", 0, "").then(paintLogs).catch(function () {});
  }

  document.addEventListener("DOMContentLoaded", function () {
    try { wire(); } catch (e) {}
    tick();
    setInterval(tick, 3000);
    window.addEventListener("hashchange", tick);
  });
})();
