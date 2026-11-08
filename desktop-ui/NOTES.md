# tunnelport Windows 桌面端 · UI 设计说明

> 交付物：`index.html`（主界面四屏 + 二维码弹窗）、`pairing.html`（首次配对引导）、`app.css`（设计系统实现）。
> 直接用浏览器打开即可（零外部资源，无 CDN / 无网络字体，符合 DESIGN.md 约束）。

## 方向（Direction）

```
机架前面板的诊断仪表，不是 SaaS 管理后台
· type   系统无衬线 800 字重标题 + 系统等宽 tabular-nums 读数
· palette 纸 #F5F6F4 / 墨 #141A17 / 琥珀 #E0A62B（标贴与焦点，小面积）
· signature 五段链路读数（✔ ✘ – 固定列），深色带上是发光读数，纸面上是台账行
```

两个设计师按此文件工作，应产出同一家族的界面：**标题栏导航 + 全宽台账行 + 唯一墨底地址条**。

## 变体收敛（.ux/drafts/）

| 变体 | 构图 | 判定 |
|---|---|---|
| A 侧栏仪表盘 | 左 208px 图标栏 + 两栏状态页 | 成熟但偏模板，浪费纵向空间 |
| **B 标题栏台账** | **Windows Terminal 式标题栏 tab + 全宽固定列行** | **胜出**：最贴合 DESIGN.md「行优于卡片、列对齐」 |
| C 配对引导 | 首屏粘贴配对链接 | 标题排版保留，并入成品 `pairing.html` |

## 屏幕清单与状态

| 屏 | 文件 / 入口 | 内容 | 状态覆盖 |
|---|---|---|---|
| 配对（首启） | `pairing.html` | 粘贴 `tunnelport://` 配对链接、三项预检、剪贴板读取 | 未连接、就绪 ✔ |
| 状态 | `index.html` | 已连接大标题、墨底入口地址条（复制/二维码）、五段台账、接入参数、最近事件 | 已连接、待验 –、复制反馈「已复制」 |
| 诊断 | `index.html#doctor` | **唯一深色带**：五段读数 + 判定 + 外部段待验提示；常见处理台账 | ✔ 通过 / – 待验 / 提示行 |
| 日志 | `index.html#logs` | 级别筛选 seg、搜索、跟随开关、全高终端、`tp logs -f` 等价命令 | INFO/WARN/ERROR 着色、开关 on/off |
| 设置 | `index.html#index#settings` | 通用/隧道/配对与安全/关于 四组行 | 开关 on/off、**引擎切换 rathole disabled（连接中不可改）**、空 placeholder |
| 二维码弹窗 | `index.html#qr` 或点「二维码」 | elev-3 模态 + 程序生成的 QR 占位、地址、Esc 关闭 | 打开/关闭、按钮 hover |

交互状态（app.css 全量声明）：`hover / focus-visible（琥珀 2px 环）/ active / disabled`，动效全部带 `prefers-reduced-motion` 兜底。

## 与 DESIGN.md 的偏差（均经 ux_audit 验证，建议回写 DESIGN.md）

1. **sticker 字重 600 → 700**：琥珀底 `#E0A62B` + 近黑字在 APCA 下 12/700 得 Lc 61.3（工具对 700 字重阈值 60），WCAG 9.21:1。颜色保持 DESIGN.md 原值。
2. **`::selection` 琥珀 → 墨底纸字**：选区是正文字号（阈值 75），琥珀对 Lc 61.3 不达标；墨底纸字 Lc 100，且与命令块气质一致。
3. **输入框边框 `--line` → `--ink-3`**：输入框边界属于组件识别信息，需 WCAG 1.4.11 的 3:1（`--line` 仅 1.29:1）；hover 提升到 `--ink`。行分隔线仍用 `--line`（结构装饰，非组件边界）。
4. **危险按钮字重 600 → 700**：`--fail` 文字 Lc 72（阈值 75 不足），700 字重下阈值 60，WCAG 5.26:1。
5. **语义色用法**：纸面上的状态文字用 700 字重（pass Lc 76 / warn Lc 69，均过 60 阈值）；深色带读数用 `-d` 变体（Lc 77+）。与站点实现一致。
6. **纸面温度**：`#F5F6F4` 在纯白对照下读作暖中性（视觉复核实测），DESIGN.md 文案称“冷纸白”；token 未改，仅记录该感知差异。

## 三轮视觉复核与修复（.ux/shots/）

| 轮次 | 发现 | 修复 |
|---|---|---|
| 1 | 日志终端 40% 空黑；诊断页底部 140px 空白；设置页末行贴边；弹窗孤行；组标签层级弱 | 补 6 行历史日志；诊断页加大节奏 + 脚注；设置组间距/留白调整；文案重写；标签升为 ink-2/700 |
| 2 | **设置页「关于」组在 800 视口被裁（加高截图才暴露）**；关于组间距过紧；诊断页大视口仍有 220px 空白 | 关于组合并为单行 + 全组间距统一 s4 + 行距收紧（9 行全部装入 800）；诊断页改为 flex 自适应均分 |
| 3 | 根因：`#view-doctor .view.is-active` 误写为后代选择器（应为 `#view-doctor.view.is-active`）；脚注内联 `margin-top` 压过 `auto` 规则，950 高度富余全堆到第一道缝 | 修正选择器 + 移除内联样式；800/950 双高度两道缝均分（57/57、132/132），脚注距底 29px |

复核方式：本会话读图工具故障（对所有文件返回同一张图，已用纯色探针证实），故三轮视觉复核均由独立子代理完成；我方以像素级程序化核验交叉验证。

## 验证记录

- `ux_audit`（app.css，34 对显式对比度配对 + Tokens + States + Sloptells）：**PASS**
- Chrome headless 1280×800 / 1280×950 截图（`.ux/shots/final-*.png`），像素级核验：状态栏条带、内容零溢出、各屏深色块位置、tab 琥珀下划线、墨底按钮文字可见性、弹窗遮罩、设置页 9 行边框计数 — 全部符合预期。
- 三轮独立子代理视觉复核（LOOK 清单）：问题清零后 FINAL_SIGNOFF。
- JS 语法 `node --check` 通过；tab 切换、hash 深链、开关、复制反馈、弹窗 Esc 均可交互。

## 复现截图

```sh
chrome --headless --disable-gpu --hide-scrollbars \
  --screenshot=out.png --window-size=1280,800 \
  "file://$(pwd)/index.html#doctor"   # #status #doctor #logs #settings #qr
```
