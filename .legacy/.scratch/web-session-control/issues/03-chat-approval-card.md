# 03 — 聊天确认卡 + policy_decision

Status: resolved

**What to build:** Session View 用页内确认卡替换 `window.confirm`；保持 POST `/api/tool-approval`；多 Session 非当前仅徽标；超时与 Host 对齐；最终裁决写入 `policy_decision`（含 allow）。

**Acceptance:**
- [x] Web 无浏览器 confirm 弹窗
- [x] 允许/拒绝/超时卡片状态正确
- [x] allow 路径 Session Log 可见 policy_decision

**Answer:** `plugins/session/ui/main.js` renderApprovalCard；agent `logPolicyDecisionMeta` 对 deny/ask/allow 均落盘；Host 契约未改。
