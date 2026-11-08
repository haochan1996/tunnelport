// tunnelport 页面胶水：从 Go 服务（App.*）取真实数据填充设计稿。
// mygo.runtime.call 由 MyGO 自动注入的桥提供（window.mygo）。
(function () {
  "use strict";
  var call = function (m) {
    var a = Array.prototype.slice.call(arguments, 1);
    return window.mygo.call("App." + m, ...a);
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

    // 底部状态栏（结构：状态·RTT | 入口 | 重连 | 版本）
    var sb = q(".statusbar");
    if (sb) {
      var spans = qa(":scope > span", sb);
      if (spans[0]) spans[0].innerHTML = "<strong>" + st.state + "</strong> · RTT " + (st.rtt || "—");
      if (spans[1]) spans[1].textContent = "入口 " + st.phone;
      var right = q(".sb-right", sb);
      if (right) {
        var rs = qa("span", right);
        if (rs[0]) rs[0].textContent = "重连 " + (st.reconnects || 0) + " 次 / 24 h";
        if (rs[1]) rs[1].textContent = st.version || "v0.1.0";
      }
    }

    // tab 计数：日志真实行数
    qa(".tab__n").forEach(function (n) {
      var tab = n.closest(".tab");
      if (tab && tab.dataset.view === "logs" && st.logsCount !== undefined) n.textContent = st.logsCount;
    });
  }

  // ---------- 诊断屏 ----------
  function paintDoctor(d) {
    var v = q("#view-doctor");
    if (!v) return;
    var rows = qa(".readout__seg", v).map(function (seg) { return seg.parentElement; });
    (d.rows || []).forEach(function (r, i) {
      var row = rows[i]; if (!row) return;
      // 设计稿结构：mark=符号 seg=段名 val=判定描述 meta=读数
      var mark = q(".readout__mark", row); if (mark) mark.textContent = r.state === "通过" ? "✔" : r.state === "失败" ? "✘" : "–";
      var seg = q(".readout__seg", row); if (seg) seg.textContent = r.name;
      var val = q(".readout__val", row); if (val) val.textContent = r.fact;
      var meta = q(".readout__meta", row); if (meta) meta.textContent = r.reading;
    });
    qa("*", v).forEach(function (e) {
      if (e.children.length) {
        // 带 <code> 子元素的提示行也要重写（占位地址藏在 code 里，v0.1.2）
        if (e.classList && e.classList.contains("band__hint") && d.healthUrl) {
          e.textContent = "外部段需手机 4G 打开 " + d.healthUrl + "，显示 OK 即全链路通畅。";
        }
        return;
      }
      var t = e.textContent || "";
      if (t.indexOf("链路可用") >= 0 || t.indexOf("链路畅通") >= 0 || t.indexOf("逐项排查") >= 0) {
        e.textContent = d.verdict;
      } else if (t.indexOf("即全链路通畅") >= 0 || (t.indexOf("外部段") >= 0 && t.indexOf("4G") >= 0)) {
        e.textContent = "外部段需手机 4G 打开 " + d.healthUrl + "，显示 OK 即全链路通畅。";
      }
    });
    var meta = q(".page-meta", v);
    if (meta) meta.textContent = "上次诊断 " + d.at + " · 耗时 " + d.duration + " · 每段独立判定";
  }

  // ---------- 日志屏 ----------
  var logLv = 0, logKw = "", logFollow = true;
  function paintLogs(lines) {
    var v = q("#view-logs");
    if (!v) return;
    var host = q(".term", v) || q(".term__line", v)?.parentElement;
    if (!host) return;
    host.innerHTML = "";
    (lines || []).slice(-60).forEach(function (l) {
      var d = document.createElement("div");
      var up = l.toUpperCase();
      var cls = " term__dim";
      if (up.indexOf("ERROR") >= 0 || l.indexOf("失败") >= 0 || l.indexOf("错误") >= 0 || l.indexOf("拒绝") >= 0) cls = " term__hint";
      else if (up.indexOf("WARN") >= 0 || l.indexOf("重连") >= 0) cls = "";
      d.className = "term__line" + cls;
      d.textContent = l;
      host.appendChild(d);
    });
    if (logFollow) host.scrollTop = host.scrollHeight;
  }

  // ---------- 交互 ----------
  function wire() {
    // 无边框窗口：右上角 — ▢ ✕ 按钮 + 标题栏拖拽区
    try {
      var wc = q(".winctl");
      if (wc) {
        qa("button", wc).forEach(function (b) {
          b.addEventListener("click", function () {
            var w = window.mygo.window;
            var label = b.getAttribute("aria-label") || "";
            if (label.indexOf("最小化") >= 0) w.minimize();
            else if (label.indexOf("最大化") >= 0) w.toggleMaximize();
            else if (label.indexOf("关闭") >= 0) w.close();
          });
        });
        // 拖拽区：标题栏整体可拖，交互控件除外
        var bar = wc.parentElement;
        if (bar) {
          bar.style.setProperty("--app-region", "drag");
          wc.style.setProperty("--app-region", "no-drag");
          qa(".tabs, .tabs *, .brand, .brand *", bar).forEach(function (el) {
            el.style.setProperty("--app-region", "no-drag");
          });
        }
      }
    } catch (e) {}

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
        // button[role=switch] 不触发 change，必须接 click（v1.5 修死开关）
        if (input.tagName === "BUTTON") {
          input.addEventListener("click", function () {
            var on = input.getAttribute("aria-checked") !== "true";
            input.setAttribute("aria-checked", String(on));
            call(pair[1], on).catch(function () {});
          });
        }
      });
    }

    // 暂停 / 继续隧道（v1.5 修死按钮）
    function syncPause(d) {
      var pb = q("#btn-pause");
      if (!pb || !d) return;
      var paused = d.state === "已暂停";
      pb.setAttribute("data-paused", paused ? "1" : "0");
      pb.textContent = paused ? "继续隧道" : "暂停隧道";
    }
    var pauseBtn = q("#btn-pause");
    if (pauseBtn) {
      pauseBtn.addEventListener("click", function () {
        var paused = pauseBtn.getAttribute("data-paused") === "1";
        call(paused ? "Resume" : "Pause").then(function () {
          pauseBtn.setAttribute("data-paused", paused ? "0" : "1");
          pauseBtn.textContent = paused ? "暂停隧道" : "继续隧道";
        }).catch(function (e) {
          pauseBtn.textContent = String((e && e.message) || e);
          setTimeout(function () { pauseBtn.textContent = "暂停隧道"; }, 2000);
        });
      });
    }

    // 日志屏：级别筛选 / 搜索 / 清空 / 复制（v1.5 修死控件）
    function refreshLogs() {
      call("Logs", logLv, logKw).then(paintLogs).catch(function () {});
    }
    var LV = { "全部": 0, "INFO": 1, "WARN": 2, "ERROR": 3 };
    var segBtns = qa(".logbar .seg button");
    segBtns.forEach(function (b) {
      b.addEventListener("click", function () {
        segBtns.forEach(function (x) { x.setAttribute("aria-pressed", String(x === b)); });
        logLv = LV[b.textContent.trim()] || 0;
        refreshLogs();
      });
    });
    var logSearch = q(".logbar input[type=search]");
    if (logSearch) {
      var t0 = null;
      logSearch.addEventListener("input", function () {
        if (t0) clearTimeout(t0);
        t0 = setTimeout(function () { logKw = logSearch.value.trim(); refreshLogs(); }, 300);
      });
    }
    qa("#view-logs .page-actions .btn").forEach(function (b) {
      b.addEventListener("click", function () {
        if (b.textContent.indexOf("清空") >= 0) {
          call("ClearLogs").then(refreshLogs).catch(function () {});
        } else if (b.textContent.indexOf("复制") >= 0) {
          var txt = qa("#view-logs .term__line").map(function (el) { return el.textContent; }).join("\n");
          call("Copy", txt).then(function () {
            b.textContent = "已复制";
            setTimeout(function () { b.textContent = "复制"; }, 1500);
          }).catch(function () {});
        }
      });
    });
    var followSw = q(".logbar .follow .switch");
    if (followSw) {
      followSw.addEventListener("click", function () {
        logFollow = followSw.getAttribute("aria-checked") !== "true";
        followSw.setAttribute("aria-checked", String(logFollow));
      });
    }

    // 配对页返回主界面（v1.5：已连接时不再困在配对页）
    var backBtn = q("#back-home");
    if (backBtn) {
      backBtn.addEventListener("click", function () { location.href = "/"; });
    }

    // 更换配对码：回配对页重新粘贴链接（v1.3 恢复路径）
    document.addEventListener("click", function (e) {
      var t = e.target.closest('[data-act="relink"]');
      if (!t) return;
      location.href = "/pairing.html";
    });

    // 桌面快捷方式：误删后一键重建（v1.3）
    var scBtn = document.getElementById("btn-shortcut");
    if (scBtn) {
      scBtn.addEventListener("click", function () {
        scBtn.textContent = "创建中…";
        call("CreateShortcut").then(function () {
          scBtn.textContent = "已创建 ✓";
          setTimeout(function () { scBtn.textContent = "重新创建"; }, 2500);
        }).catch(function () {
          scBtn.textContent = "创建失败";
          setTimeout(function () { scBtn.textContent = "重新创建"; }, 2500);
        });
      });
    }

    // 检查更新（关于组的按钮）：点击检查 → 有新版则变为「更新并安装」
    var updBtn = null;
    qa("#view-settings .btn").forEach(function (b) {
      if (b.textContent.indexOf("检查更新") >= 0) updBtn = b;
    });
    if (updBtn) {
      var updState = 0; // 0 未检查 / 1 可更新 / 2 已最新或已装
      updBtn.addEventListener("click", function () {
        if (updState === 0 || updState === 3) {
          updState = 3;
          updBtn.textContent = "检查中…";
          call("CheckUpdate").then(function (u) {
            if (u.hasUpdate && u.assetUrl) {
              updState = 1;
              updBtn.textContent = "更新到 " + u.latest;
            } else if (u.hasUpdate) {
              updState = 2;
              updBtn.textContent = "去发布页";
              call("OpenURL", u.releaseUrl).catch(function () {});
            } else {
              updState = 2;
              updBtn.textContent = "已是最新版";
            }
          }).catch(function (e) {
            updState = 0;
            updBtn.textContent = "检查失败，重试";
            updBtn.title = String((e && e.message) || e || "");
          });
        } else if (updState === 1) {
          updState = 3;
          updBtn.textContent = "更新中…";
          call("DoUpdate").then(function (msg) {
            updState = 2;
            updBtn.textContent = msg.indexOf("重启") >= 0 ? "重启应用生效" : msg;
          }).catch(function () {
            updState = 0;
            updBtn.textContent = "更新失败，重试";
          });
        }
      });
    }

    // 日志屏：级别/搜索 → 重新拉取（简单轮询）
    setInterval(function () {
      if (!location.hash || location.hash.indexOf("logs") < 0) return;
      var level = 0;
      call("Logs", level, "").then(paintLogs).catch(function () {});
    }, 4000);

    // 配对页回填上次成功配对的链接（v1.5：重新配对不再「找不到配对码」）
    var pairField = q(".pair .field");
    if (pairField && !pairField.value) {
      call("LastURL").then(function (u) {
        if (u) pairField.value = u;
      }).catch(function () {});
    }

    // 配对页：从剪贴板读取（免手粘，杜绝截断丢 #token）
    var pasteBtn = q("#paste-clip");
    if (pasteBtn) {
      pasteBtn.addEventListener("click", function () {
        call("ClipboardText").then(function (t) {
          t = (t || "").trim();
          var f = q("input.field, input[type=text]");
          if (f) f.value = t;
          if (!t) pasteBtn.textContent = "剪贴板为空";
          else if (t.indexOf("tunnelport://REDACTED < 0) pasteBtn.textContent = "剪贴板里没有配对链接";
          else {
            pasteBtn.textContent = "已读取 ✓";
            setTimeout(function () { pasteBtn.textContent = "从剪贴板读取"; }, 2000);
          }
        }).catch(function () {});
      });
    }

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

  // 状态屏「链路台账」（.lrow）接真实五段数据
  function paintLedger(rows) {
    var sec = q('section[aria-label="链路台账"]');
    if (!sec) return;
    var ls = qa(".lrow", sec);
    (rows || []).forEach(function (r, i) {
      var row = ls[i]; if (!row) return;
      var k = q(".lrow__key", row); if (k) k.textContent = r.name;
      var v = q(".lrow__val", row); if (v) v.textContent = r.fact;
      var st = q(".lrow__state", row);
      if (st) {
        st.textContent = (r.state === "通过" ? "✔ " : r.state === "失败" ? "✘ " : "– ") + r.state;
        st.className = "lrow__state " + (r.state === "通过" ? "lrow__state--ok" : r.state === "失败" ? "lrow__state--bad" : "lrow__state--todo");
      }
      var mt = q(".lrow__meta", row); if (mt) mt.textContent = r.reading;
    });
  }

  // ---------- 轮询 ----------
  var tickN = 0;
  function tick() {
    tickN++;
    call("Status").then(function (d) { paintStatus(d); syncPause(d); }).catch(function () {});
    // 五段数据：状态屏台账 + 诊断屏同源（每 3 轮 = 9s 刷一次，探测不至过频）
    if (tickN % 3 === 1 || location.hash.indexOf("doctor") >= 0) {
      call("Doctor").then(function (d) { paintLedger(d.rows); paintDoctor(d); }).catch(function () {});
    }
    if (location.hash.indexOf("logs") >= 0) call("Logs", logLv, logKw).then(paintLogs).catch(function () {});
  }

  document.addEventListener("DOMContentLoaded", function () {
    try { wire(); } catch (e) {}
    tick();
    setInterval(tick, 3000);
    window.addEventListener("hashchange", tick);
  });
})();
