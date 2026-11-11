状态：参考材料

# 主流 Agent 基准与 LiteAgent 的适配性

范围：公开 Agent 任务基准的规模、判分方式与接入路径，访问日期 2026-11-11。
不含性能与稳定性压测，那部分见 [PRESSURE.md](../../PRESSURE.md)。

## 1. 结论摘要

1. 先跑 Aider Polyglot 与 Multi-SWE-bench 的 Go 子集。两者都是“改仓库、跑测试”，与现有工具面重合。
2. 再跑 Terminal-Bench 2.1 或 4.0。它只要求终端，最接近 LiteAgent 的真实用法。
3. MCP-Universe 挑两个非浏览器域接入，作为 MCP 能力线。其余 MCP 基准多靠 LLM judge 判分。
4. 不跑 GAIA、WebArena、OSWorld、TheAgentCompany、Agents' Last Exam、GDPval。缺浏览器与 GUI 工具，判分也不可再生。
5. SWE-bench Verified 只作参考，不作主指标。BFCL 只做模型准入，不作框架分。

## 2. 评测对象与边界

LiteAgent 是本地优先的 Go Agent 平台。
Host 负责插件进程、Frame 路由与 WebUI 壳。
Agent 循环在 `plugins/agent`，入口是 `loop.turn`。

### 2.1 工具面

| 工具 | 行为 | 对评测的意义 |
|---|---|---|
| `file_read`、`file_write`、`file_edit` | 工作区读写与定点替换 | 覆盖补丁类任务 |
| `file_glob`、`file_grep` | 文件名与内容检索 | 影响定位效率 |
| `shell_execute` | Host 本机同步进程 | 能跑构建与测试，无 OS 隔离 |
| `python_execute` | 工作区 Python | 覆盖脚本与数据任务 |
| `skill_read` | 按需读取 SKILL.md | 不进入判分 |
| `mcp_call` | 调用 stdio MCP server | 只能接本地 server |
| `subagent` | 派生子会话 | 步数与成本会分叉 |

没有浏览器、GUI 与 computer-use 工具。
这一条直接决定 3.4 节的排除名单。

### 2.2 可观测事实

会话 Facts 足以支撑常规采集。

| 事实 | 用途 |
|---|---|
| `turn_start`、`turn_end` | 终态、耗时、错误码 |
| `usage` | 每次模型调用的 token |
| `role=tool` 消息 | 工具错误、截断标记 |
| `sandbox_decision` | 裁决轨迹 |
| `subagent_spawned`、`subagent_finished` | 子会话成本 |

采集口径：步数按模型调用计；错误率按 `is_error=true` 计。
Agent 不估算 token，供应商不返回 `usage` 时该步缺失。

### 2.3 Headless 驱动

`POST /bridge/call` 可以驱动 `loop.turn`。
`loop.turn` 对应 `capability=loop` 与 `method=turn`。
请求体带 `version`、`id`、`type`、`capability`、`method`、`payload`。

```json
{
  "version": 1,
  "id": "eval-1",
  "type": "req",
  "capability": "loop",
  "method": "turn",
  "payload": {
    "workspace": "/workspace/repo",
    "session_id": "eval-aider-001",
    "user_input": "…题面…",
    "model": "…"
  }
}
```

响应是 NDJSON，末尾必须有一条 Response。
同一个 `session_id` 连续调用，等价于多轮对话。
调用结束后可用 `/bridge/calls/{id}` 重放事件。
平台不提供 OpenAI 兼容的服务端。
因此只能接“能驱动任意外部进程”的 harness。

### 2.4 不在范围

| 项 | 归属 |
|---|---|
| 并发、内存、长会话、故障恢复 | [PRESSURE.md](../../PRESSURE.md) |
| 模型能力横评 | 模型方榜单 |
| 训练数据与 RL 配方 | 不讨论 |

本文结论只对当前工具面成立。

## 3. 分类对比

适配度三档：高指工具面与判分可直接对接；中指需适配器或外部服务；低指缺能力或判分不可复现。
另有观察档，指版本或口径未定，暂不纳入比较。
接入成本按人日粗估：低为 2 人日以内，中为 3～10 人日，高为 10 人日以上。

### 3.1 编码与软件工程

SWE-bench 家族同源，先看拆分。

| 基准 | 规模 | 任务形态 | 判分 | 环境 | 适配度 | 接入成本 |
|---|---|---|---|---|---|---|
| [SWE-bench](https://www.swebench.com/original.html) | 2,294 题 / 12 仓库 | Issue 修一个补丁 | 测试前后状态 | Docker | 中 | 中 |
| [Verified](https://www.swebench.com/verified.html) | 500 题 | 人工筛过的子集 | 同上 | 同上 | 中 | 中 |
| [Lite](https://www.swebench.com/lite.html) | 300 题 / 11 仓库 | 单文件为主 | 同上 | 同上 | 中 | 低 |
| [Multilingual](https://www.swebench.com/multilingual.html) | 300 题 / 9 语言 / 42 仓库 | 多语言修复 | 同上 | 多语言镜像 | 中 | 高 |
| [Pro](https://arxiv.org/abs/2509.16941) | 1,865 题 / 41 仓库，公开 731 | 长程多文件 | 测试 | Docker | 低 | 高 |
| [Live](https://swe-bench-live.github.io/) | 初始 1,319 题 / 93 仓库 | 新 Issue，持续更新 | 同上 | 实例镜像 | 中 | 高 |
| [SWE-rebench](https://arxiv.org/abs/2505.20411) | 21,000+ 题 | 自动挖掘 Python 题 | 同上 | 镜像 | 中 | 高 |
| [SWE-smith](https://swesmith.com/) | 50,000 题 / 128 仓库 | 合成缺陷 | 原测试 | 自建环境 | 高 | 中 |

Pro 平均改动 4.1 个文件、107.4 行，公开集只覆盖 11 个仓库。
其余分片落在 12 个 held-out 仓库与 18 个商业仓库。
有二手转述称，2026 年审计发现公开集约三成任务有缺陷。
原文未取到，见第 8 节。
Verified 头部分数已接近上限，区分度有限。
具体分数同样列为未确认项。

其他编码基准面向不同粒度。

| 基准 | 规模 | 任务形态 | 判分 | 环境 | 适配度 | 接入成本 |
|---|---|---|---|---|---|---|
| [Multi-SWE-bench](https://multi-swe-bench.github.io/) | 1,632 题 / 7 语言；Go 267 题 | 多语言仓库修复 | FAIL_TO_PASS | Docker | 高 | 中 |
| [Aider Polyglot](https://aider.chat/2024/12/21/polyglot.html) | 225 题 / 6 语言；Go 39 题 | 单文件改写 | 两轮尝试取通过 | 本地或容器 | 高 | 低 |
| [DeepSWE](https://deepswe.datacurve.ai/) | 113 题 / 91 仓库 | 原创长程任务 | 手写验证器 | 容器 | 中 | 高 |
| [NL2Repo-Bench](https://arxiv.org/abs/2512.12730) | 104 题 | 空仓库按文档生成 | pytest | Python | 低 | 高 |

Multi-SWE-bench 的语言分布：Java 350、TypeScript 318、JavaScript 294、Go 267、Rust 166、C 140、C++ 97。
另有 4,723 个 RL 实例，可作训练集，不作评测集。

### 3.2 终端与长程

| 基准 | 规模 | 任务形态 | 判分 | 环境 | 适配度 | 接入成本 |
|---|---|---|---|---|---|---|
| [Terminal-Bench 2.1](https://www.tbench.ai/) | 89 题 | 容器内终端任务 | 测试全过 | Docker | 高 | 中 |
| [Terminal-Bench 4.0](https://www.tbench.ai/) | 66 题 | 同上，限时 8 小时 | 测试，跑 5 次 | Docker | 高 | 中 |
| Terminal-Bench 3.0 | 74 题 | 滚动题集 | 测试 | Docker | 中 | 中 |
| [Terminal-Bench-Science](https://www.tbench.ai/) | 70 题 / 5 领域 | 科研流程 | 测试，pass@1 三次 | Docker | 中 | 高 |

版本变化很快，题数与口径都在改。

| 版本 | 时间 | 题数 | 说明 |
|---|---|---|---|
| 1.0 | 2025-05 | 80 | 初版 |
| 2.0 | 2025-11 | 89 | 提高难度，引入 Harbor |
| 2.1 | 2026-05 | 89 | 修正 28 个任务 |
| 3.0 | 2026-07 | 74 | 滚动题集 |
| 4.0 | 2026-08 | 66 | 移除 8 题，多题修订 |

2.1 是 2.0 的修正版，题数不变，改的是环境与指令。
4.0 换了题集，分数不能与 2.x 横比。
访问当日官网已出现 4.1，题数未确认。

### 3.3 工具调用、多轮与 MCP

| 基准 | 规模 | 任务形态 | 判分 | 环境 | 适配度 | 接入成本 |
|---|---|---|---|---|---|---|
| [BFCL V4](https://gorilla.cs.berkeley.edu/leaderboard.html) | 约 5,100 条 | 函数调用 | AST 与执行比对 | 无 | 低 | 低 |
| [τ²-bench](https://taubench.com/) | 原 115 / 50 / 114 | 多轮客服 | DB 状态加 pass^k | Python 模拟 | 中 | 高 |
| [MCP-Universe](https://github.com/SalesforceAIResearch/MCP-Universe) | 231 题 / 11 server / 6 域 | 真实 MCP 任务 | 84 个 evaluator | 远程服务 | 中 | 中 |
| [MCP-Bench](https://arxiv.org/abs/2508.20453) | 104 题 / 28 server / 250 工具 | 跨工具编排 | LLM judge | live server | 中 | 高 |
| [LiveMCPBench](https://arxiv.org/abs/2508.01780) | 95 题 / 70 server / 527 工具 | 多服务器路由 | LLM judge | 本地工具集 | 中 | 中 |
| [MCPToolBench++](https://arxiv.org/abs/2508.07575) | 4,000+ server / 40+ 类 | 单步与多步调用 | 结果比对 | 市场数据 | 低 | 高 |

BFCL V4 的构成：agentic 40%、多轮 30%、live 10%、非 live 10%、幻觉 10%。
agentic 共 665 条，其中 web search 200 条、memory 465 条。
多轮为 800 条，覆盖 8 个 API 域。

τ²-bench 的口径已变。

| 集合 | 题量 | 说明 |
|---|---|---|
| 论文原集 | retail 115、airline 50、telecom 114 | 2025 年发布 |
| 当前 text-v1 | 每域 50 题，6 种语言 | 冻结集，共 900 次调用 |
| voice-v1 | 50 / 114 / 114 | 英文语音帧 |

继任版本为 τ-bench 2.0，社区也称 τ³。
它增加 `banking_knowledge` 域，共 97 题，并支持全双工语音。
τ² 的关键指标是 pass^k：k 次运行全部通过才算成功。

### 3.4 通用、浏览器与桌面

| 基准 | 规模 | 任务形态 | 判分 | 环境 | 适配度 | 接入成本 |
|---|---|---|---|---|---|---|
| [GAIA](https://arxiv.org/abs/2311.12983) | 466 题，公开 166 | 检索加多模态 | 精确答案 | 浏览器 | 低 | 高 |
| [WebArena](https://webarena.dev/) | 812 题 | 网页操作 | 功能正确性 | 自建站点 | 低 | 高 |
| [OSWorld](https://os-world.github.io/) | 369 题 | 桌面 GUI | 执行脚本 | 虚拟机 | 低 | 高 |
| [TheAgentCompany](https://theagentcompany.com/) | 175 题 | 企业工作任务 | checkpoint | 模拟公司 | 低 | 高 |
| [Agents' Last Exam](https://agents-last-exam.org/) | 1,000+（论文）/ 1,500+（官网） | 跨 GUI 与 CLI | 确定性 grader | 沙箱 | 低 | 高 |
| [GDPval](https://arxiv.org/abs/2510.04374) | 1,320 题；AA v2 取 220 题 | 知识工作交付物 | 专家成对比较 | 无 | 低 | 高 |
| [MLE-bench](https://arxiv.org/abs/2410.07095) | 75 个赛题 | ML 工程 | 奖牌线 | GPU | 低 | 高 |
| [PaperBench](https://arxiv.org/abs/2504.01848) | 20 篇论文 | 复现研究 | rubric 加 judge | GPU | 低 | 高 |

排除理由集中在三点。
一是工具面缺口：没有浏览器与 GUI 工具，多数任务无法完成。
二是判分不可复现：GDPval 与 PaperBench 依赖专家或模型评审。
三是算力门槛：MLE-bench 与 PaperBench 需要 GPU 训练。

GAIA 的 466 题中，公开 166 题，其余答案保留在私有集。
ALE 论文写 1,000+ 题，官网写 1,500+ 题，两者口径不同。
ALE 公开子集约一成，具体数量以官方发布为准。

### 3.5 2026 新兴与观察

| 基准 | 规模 | 任务形态 | 判分 | 环境 | 适配度 | 接入成本 |
|---|---|---|---|---|---|---|
| [Terminal-Bench 4.1](https://www.tbench.ai/) | 未确认 | 终端长程 | 测试 | Docker | 观察 | 中 |
| τ-bench 2.0（τ³） | 每域 50 题 × 6 语言；banking 97 题 | 多语言与语音 | DB 状态加 judge | 音频链路 | 观察 | 高 |
| SWE-bench Pro 审计 | 公开 731 题 | 长程修复 | 测试 | Docker | 观察 | 高 |
| SWE-rebench V2 | 24,836 题 / 19 语言（二手） | 多语言修复 | 测试 | 镜像 | 观察 | 高 |
| [Terminal-Bench-Science](https://www.tbench.ai/) | 70 题 / 5 领域 | 科研流程 | 测试 | Docker | 观察 | 高 |

观察列表不进第一轮评测。
等版本稳定、题数与判分口径固定后再评估。
其中 4.1 与 τ³ 的变动最频繁，建议每季度看一次。

## 4. 重点接入说明

下面六个基准给出可执行路径。
其余基准的接入方式类似，不重复。

### 4.1 Aider Polyglot

形态：每题一个独立目录，改完跑官方测试。
题集共 225 题，Go 39、Python 34、Java 47、JavaScript 49、C++ 26、Rust 30。

接入步骤：

1. 取 polyglot 题集与对应语言的运行环境。
2. 每题新建会话，`workspace` 指向该题目录。
3. 第一轮 `loop.turn` 给题面与测试命令说明。
4. 跑测试，把失败输出作为第二轮 `user_input`。
5. 第二轮后再跑测试，记录 pass@2。
6. 从 Facts 采集步数、token 与工具错误。

主要风险：

- 官方分数用 Aider 自己的 edit format，与 `file_edit` 不等价，分数不能直接对比榜单。
- 第二次尝试带测试反馈，等于给模型额外信息，需在报告里写明。
- 6 种语言的工具链要预装，Windows 原生还需对应 SDK。

### 4.2 Multi-SWE-bench Go

形态：每实例一个镜像，含仓库、base commit 与 issue 正文。
判分用测试补丁，比较 FAIL_TO_PASS 与 PASS_TO_PASS。

接入步骤：

1. 只取 Go 子集 267 题，先抽 30 题跑通。
2. 镜像内启动 `liteagent-server`，仓库挂到 `/workspace`。
3. 经 `/bridge/call` 驱动 `loop.turn`，`user_input` 用 issue 正文。
4. 结束后用只读副本应用测试补丁，再跑目标测试。
5. 用 `git diff` 校验改动只落在源码。
6. 全量跑时固定并发为 1，避免模型端点限流。

主要风险：

- 测试补丁会改写测试文件，必须用只读副本判定。
- Go 版本与模块缓存要预置，否则联网拉依赖会污染结果。
- 镜像体积大，267 题的磁盘与时间成本高。

### 4.3 Terminal-Bench

形态：每任务一个容器，含指令、测试脚本与人工参考解。
2.1 与 4.0 都要求容器内完成全部操作。

接入步骤：

1. 用 Harbor 或 `tb` 起任务容器。
2. 容器内安装 Host，`workspace` 设为任务工作目录。
3. 单轮 `loop.turn` 执行，超时按任务声明设置。
4. 第三方对照口径为：每题 2 小时、最多 250 步、三次取均值。
5. 测试脚本在容器外执行，避免被测方改写判分。

主要风险：

- 版本多，2.1、3.0、4.0、4.1 的题集与口径都不同，报告必须写明版本。
- 联网策略按任务而定，允许联网时存在直接检索答案的风险。
- 4.0 单题限时 8 小时，全量跑的成本高。

### 4.4 MCP-Universe

形态：6 个域、11 个真实 MCP server、231 题、84 个 evaluator。
域分布：网页搜索 55、位置导航 45、财务分析 40、浏览器自动化 39、仓库管理 33、3D 设计 19。

接入步骤：

1. 先选仓库管理与网页搜索两域，二者不依赖浏览器。
2. Prompter 只支持 stdio，远程 server 需本地 stdio 代理。
3. 用任务 JSON 生成 `user_input`，工具经 `mcp_call` 暴露。
4. evaluator 在容器外运行，只读产物。
5. 记录 `mcp_call` 的错误率与重试次数。

主要风险：

- 浏览器自动化与 3D 设计两域需要 GUI 或 Blender，当前不适用。
- server 依赖外部 API Key 与配额，不可用时应记为阻塞。
- 论文口径为 133 个工具，工具描述会显著拉长上下文。

### 4.5 τ²-bench

形态：用户模拟器与 Agent 多轮对话，双方共享环境状态。
telecom 域为双控：用户也持有工具，可以主动改变状态。
判分比对数据库终态，并统计 pass^k。

接入步骤：

1. 把领域工具包成 stdio MCP server，经 `mcp_call` 暴露。
2. 每轮把用户模拟器的发言作为新的 `loop.turn` 输入，复用 `session_id`。
3. 对话结束后导出数据库状态，交给官方 evaluator 比对。
4. 每个配置至少跑 3 次，报 pass^3。

主要风险：

- 工具名与 schema 与官方定义不同，分数不能与论文横比。
- 用户模拟器要另一个模型，成本翻倍。
- 当前 text-v1 每域仅 50 题，统计波动大。
- 语音与 banking 域依赖实时音频链路，当前不适用。

### 4.6 BFCL

形态：纯模型层，考函数调用与参数是否正确。
V4 的 agentic 与多轮部分才是 Agent 相关。

接入步骤：

1. 在 BFCL harness 中配置同一个模型端点。
2. 不接入 LiteAgent，直接测模型。
3. 结果只用于模型准入与回归。

主要风险：

- 分数与框架无关，不能当 LiteAgent 的框架分。
- BFCL 假设原生 tool calling；`mcp_call` 的包装会改变模型可见 schema。
- 不同 provider 的提示模式与 FC 模式分数有差异，需固定模式。

## 5. 推荐组合与分阶段计划

| 阶段 | 目标 | 组合 | 规模 | 产出 |
|---|---|---|---|---|
| 第一周 | 打通驱动与判分 | Aider Polyglot 的 Python 与 Go | 73 题 × 3 次 | 基线表与采集脚本 |
| 2～4 周 | 真实仓库 | Multi-SWE-bench Go | 先 30 题，后 267 题 | 通过率与成本曲线 |
| 第 2 个月 | 终端与 MCP | Terminal-Bench 2.1 加 MCP-Universe 两域 | 89 题加约 88 题 | 长程能力线 |
| 持续 | 回归 | SWE-smith 自建集加固定题 | 约 85 题 | 每晚跑，环比不倒退 |

### 5.1 第一周

目标是验证链路，不是比分数。
先确认三件事：容器内 Host 能起、`loop.turn` 能跑完、测试结果能落盘。
Aider 题量小，失败能逐题定位。

### 5.2 2～4 周

进入真实仓库。
先抽 30 题，观察失败类型：定位错、改错文件、测试跑不起来。
对照基线用同模型加 mini-SWE-agent，隔离框架差异。

### 5.3 第 2 个月

加入终端长程与 MCP。
Terminal-Bench 优先 2.1，题集固定且第三方复现多。
MCP-Universe 只开两域，先把 stdio 代理跑通。

### 5.4 回归集

回归集要小、要快、要可解释。
用 SWE-smith 对自有仓库生成 40 题，再加 40 道固定题与 5 个 MCP 任务。
通过率下降 5 个百分点即阻断合入。

## 6. 评测方法与陷阱

### 6.1 驱动方式

驱动只用 `/bridge/call`，不模拟浏览器。
一次评测任务对应一个 `session_id`。
多轮任务复用同一 `session_id`，不新建会话。
采集走 Facts 与 `/bridge/calls/{id}`，不解析 UI。

### 6.2 隔离

`shell_execute` 在 Host 本机执行，`cwd` 以 workspace 为基准，工具层不检查越界。
`sandbox` 插件只做策略裁决，不是操作系统隔离。
跑公开基准必须用容器或虚拟机。
容器内启动 Host，宿主目录只读挂载。
禁止把用户主目录挂进容器。

### 6.3 评分证据

判分只看三样：测试结果、`git diff`、产物文件。
测试文件用只读副本，评测前后比对哈希。
检查 `git diff` 是否触碰测试、配置与判分脚本。
检查是否新增 `conftest.py`、`sitecustomize.py` 之类的旁路。
保存完整轨迹，失败样本可回放。

### 6.4 可复现性

`llm-openai` 当前不下发 `temperature`、`top_p`、`seed`。
同一配置必须跑 3 次以上，报均值与区间。
记录模型 ID、`base_url`、运行日期与宿主版本。
记录 `max_steps` 与超时设置，二者会直接改变通过率。
榜单分数是模型与 scaffold 的联合结果。
只能同 harness 比较，不能跨 harness 换算。

### 6.5 防作弊

四类作弊要分别防。

| 类型 | 表现 | 对策 |
|---|---|---|
| 改测试 | 改断言、跳过用例 | 只读测试副本，哈希比对 |
| 联网 oracle | 搜索题目与答案 | 断网或按域名白名单 |
| 数据污染 | 背下公开补丁 | 用 Live、SWE-rebench、SWE-smith |
| 验证器绕过 | 命中判分脚本漏洞 | 独立校验最终状态 |

公开题放久了就会被记住。
SWE-bench Verified 已公开多年，优先用新题集。
报告里写明运行日期，便于判断污染窗口。
2026 年有研究演示多个公开基准可被绕过，具体论文未定位。

### 6.6 成本控制

单题成本主要由步数与上下文长度决定。
先设 `max_steps` 上限，再按题调。
长任务用超时兜底，不用无限重试。
先用 10% 抽样估算单题成本，再决定全量。
MCP 与多轮任务要算上用户模拟器与 judge 的调用。

### 6.7 平台差异

多数基准假设 Linux 加 Docker。
Windows 原生只适合 Aider 与自建任务。
Terminal-Bench 的容器本身是 Linux，与宿主无关。
`shell_execute` 在 Windows 下走 Job Object 终止进程树，行为与 Linux 不同。

## 7. 参考来源

访问日期均为 2026-11-11。

SWE-bench 家族：

- [SWE-bench 官方站](https://www.swebench.com/)
- [原始集说明](https://www.swebench.com/original.html)
- [Lite 说明](https://www.swebench.com/lite.html)
- [Verified 说明](https://www.swebench.com/verified.html)
- [Multilingual 说明](https://www.swebench.com/multilingual.html)
- [SWE-bench 仓库](https://github.com/SWE-bench/SWE-bench)
- [SWE-bench Pro 论文](https://arxiv.org/abs/2509.16941)
- [SWE-bench Pro 公开榜](https://labs.scale.com/leaderboard/swe_bench_pro_public)
- [SWE-bench-Live](https://swe-bench-live.github.io/)
- [SWE-bench-Live 论文页](https://www.microsoft.com/en-us/research/publication/swe-bench-goes-live/)
- [SWE-rebench 论文](https://arxiv.org/abs/2505.20411)
- [SWE-smith 论文](https://arxiv.org/abs/2504.21798)
- [SWE-smith 官网](https://swesmith.com/)

多语言编码与终端：

- [Multi-SWE-bench 论文](https://arxiv.org/abs/2504.02605)
- [Multi-SWE-bench 官网](https://multi-swe-bench.github.io/)
- [Aider 榜单](https://aider.chat/docs/leaderboards/)
- [Aider Polyglot 说明](https://aider.chat/2024/12/21/polyglot.html)
- [Terminal-Bench 官网](https://www.tbench.ai/)
- [Harbor 仓库](https://github.com/laude-institute/harbor)
- [DeepSWE](https://deepswe.datacurve.ai/)
- [NL2Repo-Bench 论文](https://arxiv.org/abs/2512.12730)

工具调用与 MCP：

- [BFCL 榜单](https://gorilla.cs.berkeley.edu/leaderboard.html)
- [BFCL V3 多轮说明](https://gorilla.cs.berkeley.edu/blogs/13_bfcl_v3_multi_turn.html)
- [BFCL V4 agentic 说明](https://gorilla.cs.berkeley.edu/blogs/15_bfcl_v4_web_search.html)
- [τ²-bench 论文](https://arxiv.org/abs/2506.07982)
- [τ-bench 榜单](https://taubench.com/)
- [τ-bench 仓库](https://github.com/sierra-research/tau2-bench)
- [MCP-Universe 论文](https://arxiv.org/abs/2508.14704)
- [MCP-Universe 仓库](https://github.com/SalesforceAIResearch/MCP-Universe)
- [MCP-Bench 论文](https://arxiv.org/abs/2508.20453)
- [MCP-Bench 仓库](https://github.com/Accenture/mcp-bench)
- [LiveMCPBench 论文](https://arxiv.org/abs/2508.01780)
- [MCPToolBench++ 论文](https://arxiv.org/abs/2508.07575)

通用与桌面：

- [GAIA 论文](https://arxiv.org/abs/2311.12983)
- [WebArena 论文](https://arxiv.org/abs/2307.13854)
- [WebArena 站点](https://webarena.dev/)
- [OSWorld 论文](https://arxiv.org/abs/2404.07972)
- [OSWorld 站点](https://os-world.github.io/)
- [TheAgentCompany 论文](https://arxiv.org/abs/2412.14161)
- [TheAgentCompany 站点](https://theagentcompany.com/)
- [Agents' Last Exam 论文](https://arxiv.org/abs/2606.05405)
- [Agents' Last Exam 官网](https://agents-last-exam.org/)
- [GDPval 论文](https://arxiv.org/abs/2510.04374)
- [GDPval-AA 榜单](https://artificialanalysis.ai/evaluations/gdpval-aa)
- [MLE-bench 论文](https://arxiv.org/abs/2410.07095)
- [PaperBench 论文](https://arxiv.org/abs/2504.01848)

平台侧：

- [Agent 插件](../../plugins/agent/README.md)
- [Tools 插件](../../plugins/tools/README.md)
- [Prompter 插件](../../plugins/prompter/README.md)
- [Sandbox 插件](../../plugins/sandbox/README.md)
- [Browser Bridge](../platform/webui.md)
- [PRESSURE](../../PRESSURE.md)

## 8. 未确认项

下列数字或版本没有取到一手材料，使用时需复核。

| 项 | 现状 | 说明 |
|---|---|---|
| SWE-bench Pro 缺陷率 | 约 30% | 转述 OpenAI 2026-07 审计，原文未取到 |
| SWE-bench Verified 头部分数 | 96% 左右 | 二手榜单，未定具体日期与 harness |
| MCP-Universe 工具数 | 202 | 论文口径为 133，202 出处不明 |
| ALE 公开题数 | 约 150 | 公开比例约一成，具体数未取到 |
| Terminal-Bench 4.1 题数 | 未知 | 官网已上线，未公布规模 |
| SWE-rebench V2 规模 | 24,836 题 / 19 语言 | 二手来源，未见官方页 |
| MCPToolBench++ 任务数 | 未知 | 论文只给 server 与类别数 |
| SWE-bench Live 最新规模 | 1,565 题 / 164 仓库 | 2025-06 的二手记录 |
| 验证器绕过研究 | 未定位 | 2026 年有多篇，未确认具体论文 |
| τ³ 命名 | 社区俗称 | 官方文档称 τ-bench 2.0 |
