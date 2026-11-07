---
version: 1
name: tunnelport
description: Orca 手机端稳定接入工具的产品站与文档站设计系统。冷纸白基面 + 墨绿黑字 + 琥珀标贴色，仪表读数式排版。
colors:
  paper: "#F5F6F4"
  paper-2: "#ECEEE9"
  ink: "#141A17"
  ink-2: "#3E4A44"
  ink-3: "#5C6963"
  line: "#D7DBD5"
  ink-bg: "#101614"
  ink-bg-2: "#1B2320"
  on-ink: "#E8EBE6"
  on-ink-2: "#9AA6A0"
  amber: "#E0A62B"
  on-amber: "#241A05"
  pass: "#1E6B50"
  fail: "#B3402A"
  warn: "#8A6A12"
  pass-d: "#5FBF96"
  fail-d: "#E8765F"
  warn-d: "#D9AE45"
typography:
  sans: "系统栈：-apple-system / PingFang SC / Microsoft YaHei"
  mono: "系统栈：ui-monospace / SF Mono / Cascadia Mono / Consolas"
  display: "clamp(2.5rem, 6vw, 4.5rem) / 1.06 / 800 / -0.02em"
  h2: "2rem / 1.25 / 700 / -0.01em"
  h3: "1.25rem / 1.4 / 600"
  body: "1.0625rem / 1.75 / 400"
  small: "0.875rem / 1.6 / 400"
  mono-sm: "0.8125rem / 1.8 / 400"
rounded:
  r1: "2px"
  r2: "4px"
  r3: "8px"
spacing:
  s1: "4px"
  s2: "8px"
  s3: "12px"
  s4: "16px"
  s5: "24px"
  s6: "32px"
  s7: "48px"
  s8: "64px"
  s9: "96px"
  s10: "128px"
components:
  btn-primary:
    background: "ink"
    color: "paper"
    radius: "r2"
  btn-ghost:
    background: "transparent"
    color: "ink"
    border: "line"
  sticker:
    background: "amber"
    color: "on-amber"
    radius: "r1"
  band:
    background: "ink-bg"
    color: "on-ink"
---

# tunnelport 设计系统

## Overview

产品做的事是：把你的 VPS 变成手机访问桌面的入口，坏的时候用 `tp doctor` 告诉你卡在五段链路的哪一段。所以这个站不该长成 SaaS 落地页 —— 它应该像**一台仪表交付到你手上**。

视觉母题只有一个：**机柜前面板的诊断读数**。等宽字、对齐的列、状态符号（✔ ✘ –）、一个琥珀色端口标贴。整套系统围绕这一件东西展开，其余地方保持安静。

方向判定标准：两个设计师按这份文件工作，应该做出看起来同一个家族、但明显不是模板的页面。

## 零外部资源是设计约束，不是优化

不引入任何外部字体、CDN、图标库、分析脚本。理由有三条，都是产品性的：

1. 这是一个**网络稳定性**产品。首屏 200 KB 内加载完，本身就是在证明能力。
2. 国内网络访问境外 CDN（Google Fonts / jsdelivr / unpkg）不可靠，一个字体请求失败就会 FOUT 或空白。
3. 中文 webfont 动辄 5-10 MB，绝不可能自托管。既然中文必然走系统字体，就不要给拉丁部分制造不一致。

**因此字体个性不能来自字体文件，必须来自：字重对比、字号跨度、等宽处理、负字距、以及版式刻度。** 这不是妥协，是这套系统的核心手法。

## Colors

温度先行：整体**冷**，但不着蓝 —— 蓝是 SaaS 默认色。基面是一张略偏冷的纸 `#F5F6F4`，字是带绿调的墨 `#141A17`，形成"运维手册"而不是"营销页"的底色。

- **一个品牌色：琥珀 `#E0A62B`。** 它只做一件事 —— 当标贴。永远配深字 `#241A05`，永远小面积。它是机柜上的端口标签，不是按钮色。
- **三个语义色只在诊断读数里出现**：通过 `#1E6B50`、失败 `#B3402A`、警告 `#8A6A12`。深色带上换成 `-d` 变体（`pass-d` / `fail-d` / `warn-d`）以通过反相对比。
- **反相带只有一条**：`#101614`，用于 `tp doctor` 输出那一节。这是全页唯一的深色区域，作用是节奏标记 —— 前面是纸，这里是屏幕。不要加第二条。
- **分隔线 `#D7DBD5`**：只在表格行之间使用，不作为卡片边框。

## Typography

两个族，角色严格分离：

| 角色 | 字体 | 用法 |
|---|---|---|
| 正文与标题 | 系统无衬线（PingFang SC / Microsoft YaHei / -apple-system） | 所有中文、所有段落、所有标题 |
| 数据与命令 | 系统等宽（SF Mono / Cascadia Mono / Consolas） | 端口号、IP、RTT、命令、状态符号、品牌名 |

规则：

- **中文标题不靠字体靠字重**：`font-weight: 800`、`letter-spacing: -0.02em`、行高 1.06。中文标题在 800 字重下才有力，600 会显得退让。
- **所有数字一律等宽 + `tabular-nums`**。端口 30123 和延迟 41ms 必须在列里对齐，这是仪表的可信度来源。
- **等宽族只用于拉丁与数字**。中文进等宽族会掉进奇怪的 fallback，破坏对齐。
- 正文 45-75ch，`1.0625rem/1.75`。中文行高不能低于 1.7，否则密不透风。
- 禁止：`letter-spacing` 拉开的大写小标题（tracked eyebrow）、每段都用 `·` 连接的元信息串、`→` 挂在每个链接后面。

## Layout

- 12 列栅格，容器最大 `1120px`，左右留白 `--s5`（24px），桌面 `--s7`（48px）。
- **行优于卡片。** 承诺、现状对照、定价全部是"固定列 + 细分隔线"的行，不是圆角卡片网格。同一份数据在每行里占同一个 x 坐标。
- 首屏是两栏：左文案右读数。读数块**允许溢出右边缘**（`translateX(8%)`），并被容器裁切 —— 它读作气氛，不是插图。
- 纵向节奏：小节之间 `--s9`（96px），小节内部 `--s5`。不要在每节都套同一个 padding。

## Elevation

只有三级，且几乎不用。这套系统的层次靠**底色切换和刻度**，不靠阴影。如果一个元素需要阴影才能分层次，说明字号跨度不够。

| token | 用途 |
|---|---|
| `--elev-1` | 吸顶导航、命令块 |
| `--elev-2` | 仅浮层（本阶段无） |
| `--elev-3` | 仅模态（本阶段无） |

## Shapes

`--r1: 2px`（标贴）、`--r2: 4px`（按钮、命令块）、`--r3: 8px`（无）。小圆角是仪表感的一部分；`rounded-2xl` 那套会让它变成 SaaS 卡片。

## Components

- **btn-primary**：墨底纸字。只有一个，全页最多出现两次（首屏 + 定价）。
- **btn-ghost**：透明底 + `--line` 边。次要动作。
- **sticker**：琥珀底深字，2px 圆角，`0.75rem`，承载**真实状态信息**（如 `MVP 开发中`）。不做装饰性徽章。
- **readout**：五段诊断块。等宽、固定列、状态符号在左。这是全站唯一允许"复杂"的组件，也是唯一的签名元素。
- **command**：命令块，墨底，等宽，`user-select: all` 便于整段复制。

## States

每个可交互元素必须声明：`hover`、`focus-visible`、`active`、`disabled`。焦点环用 `--amber`，2px solid + 2px offset —— 琥珀是全站唯一的高饱和色，用在焦点上既醒目又不动用额外颜色。

有 `transition` 的地方必须有 `prefers-reduced-motion: reduce` 兜底。

## Do's & Don'ts

**Do**

- 用列对齐制造秩序，几乎不用边框
- 状态符号 `✔ ✘ –` 只表达真实判定结果
- 每个数字都用 `tabular-nums`
- 深色带只出现一次
- 琥珀只当标贴

**Don't**

- ❌ 渐变、光晕、`backdrop-filter` 玻璃拟态
- ❌ 圆角卡片网格（`rounded-2xl + shadow-lg + p-6` 那套 shadcn 反射）
- ❌ `1px` 灰色卡片边框
- ❌ 紫色 / 靛蓝渐变按钮
- ❌ 拉大字距的大写小标题
- ❌ 永久深色模式的整页配色（本系统是纸面为主，深色只是其中一条带）
- ❌ 引入任何外部字体、CDN、图标库
