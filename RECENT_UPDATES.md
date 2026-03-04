# 近期更新汇总 (2025-03)

本文档汇总近期对 NOFX 项目的主要更新。

---

## 一、前端展示

### 1. DecisionCard 决策卡片
- **Input Prompt**：保留并可折叠，便于调试与审计
- **AI 思维链**：完整展示 Chain of Thought 推理过程
- **决策动作**：开平仓、币种、杠杆、止损止盈等
- **账户状态**：净值、保证金、持仓数
- **执行日志**：实际成交价、订单 ID、成功/失败

### 2. 后端不可用时的体验
- 显示「启动中」状态，避免空白或报错
- 每 5 秒自动重试连接
- 提供「立即重试」按钮

### 3. 登录流程简化
- 删除 LandingPage 及 `components/landing/` 下所有组件
- 未登录时直接显示 LoginPage

**涉及文件**：`web/src/App.tsx`、`web/src/hooks/useSystemConfig.ts`、`web/src/lib/config.ts`、`web/src/i18n/translations.ts`

---

## 二、交易策略 Prompt 规则

### 1. 多空方向一致性
- **原则**：避免「一个币多、一个币空」的背离格局
- **转多先平空**：确认做多时，先检查空仓，优先 `close_short` 再 `open_long`
- **转空先平多**：确认做空时，先检查多仓，优先 `close_long` 再 `open_short`
- 多币种信号分歧时，优先怀疑噪音，可等待方向趋同

### 2. 策略定位
- 明确为**趋势策略**，以趋势为主
- 严禁「多空双开赚波动」
- 若新开单与现有持仓方向完全相反，视为决策错误

### 3. 基本处理逻辑与常识
- 在 `decision/engine.go` 中增加行业常识（加密相关性、同时多空风险）
- 通用决策流程：先平后开、方向一致

**涉及文件**：`decision/engine.go`、`prompts/alphagpt_factor_test.txt`

---

## 三、决策反思机制

- **Reflection Pass**：第一次决策后进行第二轮反思
- 将第一次决策 + 思维链作为输入，检查是否正确、有漏洞、是否鲁莽
- 输出最终决策，提高决策质量

**配置**：`decision/engine.go` 中 `enableReflectionPass = true`

---

## 四、交易分析脚本

### `scripts/analyze_all_trades.py`

分析指定交易员的所有已平仓交易，结合决策日志生成 Markdown 报告。

**功能**：
- 读取 `trade_logs/{trader_id}/` 下已平仓交易
- 匹配 `decision_logs` 中的开仓原因
- **平仓后价格走势**：通过币安 API 获取平仓后 1h、4h、24h 价格，评估是否过早止盈/过早止损
- 代理配置与项目一致：从项目根目录 `.env` 加载 `HTTP_PROXY`、`HTTPS_PROXY`

**用法**：
```bash
python scripts/analyze_all_trades.py [trader_id]
python scripts/analyze_all_trades.py binance_admin_deepseek_1771898732 --no-post-price --limit 5
```

**输出**：`trade_analysis_report_{trader_id}_{timestamp}.md`

---

## 五、项目文档与架构

### ARCHITECTURE_FOR_LLM.md
- 面向大模型的架构与修改指南
- 目录结构、启动方式、存储、调用流程
- 常见修改点说明

### 交易记录存储
- `trades.csv` 表格化存储
- `trade_logger` 统一管理

---

## 六、冗余清理

- 删除未使用的脚本、文档
- 删除过期的 decision_logs
- 更新 `baseline-browser-mapping` 消除相关警告

---

## 重要路径速查

| 用途       | 路径 |
|------------|------|
| 决策引擎   | `decision/engine.go` |
| 主策略 Prompt | `prompts/alphagpt_factor_test.txt` |
| 交易分析脚本 | `scripts/analyze_all_trades.py` |
| 前端 App   | `web/src/App.tsx` |
| 代理配置   | `.env`（HTTP_PROXY、HTTPS_PROXY） |
| 架构文档   | `docs/ARCHITECTURE_FOR_LLM.md` |

---

*最后更新：2025-03*
