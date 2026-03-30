package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"nofx/decision"
	"nofx/mcp"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GEPA 传给优化器的单笔理由长度：过短会导致「错因」传不到优化器，无法达成开仓级归因。
const (
	gepaReasonOpenMaxBytes  = 300
	gepaReasonCloseMaxBytes = 420
	gepaExampleReasonBytes  = 220
)

// truncateUTF8 按字节截断并避免切断多字节 UTF-8 字符。
func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	trunc := s[:maxBytes]
	for len(trunc) > 0 && (trunc[len(trunc)-1]&0xC0) == 0x80 {
		trunc = trunc[:len(trunc)-1]
	}
	return trunc + "..."
}

// buildPairedCloseRoundsBlock 将本窗口流水按标的把 open_* 与后续 close_* 配对，供优化器做「该点开仓为何亏」级归因（非结构化程序判错，而是把链条摆全）。
func buildPairedCloseRoundsBlock(ops []TradeOp) string {
	if len(ops) == 0 {
		return "\n## 本 batch 平仓回合（开仓→平仓配对）\n（本窗无开平仓流水，仅依据概览与轨迹优化。）\n\n"
	}
	pending := make(map[string][]TradeOp)
	var sb strings.Builder
	sb.WriteString("\n## 本 batch 平仓回合（开仓→平仓配对，归因优先读此节）\n")
	sb.WriteString("同一标的允许多次 open_* 合并持仓后再 close_*；下列反映本窗内**真实先后顺序**。\n\n")
	roundCount := 0
	for _, op := range ops {
		switch op.Action {
		case "open_long", "open_short":
			pending[op.Symbol] = append(pending[op.Symbol], op)
		case "close_long", "close_short":
			opens := pending[op.Symbol]
			if len(opens) == 0 {
				sb.WriteString(fmt.Sprintf("- ⚠️ **%s** %s @%.6f PnL=%.2f — 本窗流水内无前序开仓，无法配对\n\n",
					op.Symbol, op.Action, op.Price, op.PnL))
				continue
			}
			roundCount++
			first := opens[0]
			last := opens[len(opens)-1]
			dir := "多"
			if first.Action == "open_short" {
				dir = "空"
			}
			nOpen := len(opens)
			sb.WriteString(fmt.Sprintf("### 回合 %d · %s **方向=%s** → %s\n", roundCount, op.Symbol, dir, op.Action))
			sb.WriteString(fmt.Sprintf("- 首开: [%s] %s @ **%.6f** · 理由: %s\n",
				first.Time, first.Action, first.Price, truncateUTF8(strings.TrimSpace(first.Reasoning), gepaReasonCloseMaxBytes)))
			if nOpen > 1 {
				sb.WriteString(fmt.Sprintf("- 同窗加注: 共 **%d** 次同向开仓 · 末开 [%s] @ **%.6f** · 理由: %s\n",
					nOpen, last.Time, last.Price, truncateUTF8(strings.TrimSpace(last.Reasoning), gepaReasonCloseMaxBytes)))
			}
			sb.WriteString(fmt.Sprintf("- 平仓: [%s] @ **%.6f** · **PnL=%.2f** · 理由: %s\n",
				op.Time, op.Price, op.PnL, truncateUTF8(strings.TrimSpace(op.Reasoning), gepaReasonCloseMaxBytes)))
			if op.PnL < 0 {
				sb.WriteString("- **→ 亏损回合：改进版 system prompt 须用可迁移规则回应此类失败（建议+典型风险），不得略过不内化。**\n")
			}
			sb.WriteString("\n")
			delete(pending, op.Symbol)
		}
	}
	for sym, rest := range pending {
		if len(rest) > 0 {
			sb.WriteString(fmt.Sprintf("（窗口末 **%s** 仍有未平开仓 %d 笔 — 可与持仓管理规则一并考虑）\n", sym, len(rest)))
		}
	}
	if roundCount == 0 {
		return "\n## 本 batch 平仓回合（开仓→平仓配对）\n（本窗无平仓记录，仅依据轨迹与概览优化。）\n\n"
	}
	return sb.String()
}

// TrajectoryStep GEPA 反思链：单步轨迹（思维链+决策+结果）
type TrajectoryStep struct {
	Step        int
	Time        string
	CoTTrace    string
	Decisions   string // 决策摘要
	EquityAfter float64
	HadTrade    bool // 本步是否产生实际开/平仓（模拟器执行成功）；false 时纳入「错失机会」候选
}

// pretrainedBaselinePath 预训练融合 prompt 的固定路径，-base pretrained 时使用
const pretrainedBaselinePath = "prompt_simulator/training/pretrained/fused_baseline.txt"

// adaptiveRegimeHint Adaptive-OPRO：根据近期 PnL 动态生成策略倾向提示
func adaptiveRegimeHint(closedPnLs []float64, cumulativePnL float64, explore bool) string {
	if len(closedPnLs) < 3 {
		return ""
	}
	// 累计亏损 → 引导模型自主诊断多种可能原因
	if cumulativePnL < -30 {
		if explore {
			return "\n- **Adaptive-OPRO 当前倾向**：累计亏损，请**优先**从**扣费后净盈利**角度诊断：方向/周期错配、同质化亏损、手续费与无效换手等；改进**信号质量与规则可执行性**。**禁止**用「纯冷却时间」「每日笔数上限」代替质量改进；**允许**与信号质量绑定的门槛收紧。"
		}
		return "\n- **Adaptive-OPRO 当前倾向**：亏损可能由多种原因导致（如方向选择、持仓时间过短、策略周期与执行周期不匹配等），请结合数据**自主诊断**并针对性改进。**禁止**添加降低开仓频率的风控（如交易间隔、操作次数等）。"
	}
	// 盈利中 → 可保持
	if cumulativePnL > 20 {
		if explore {
			return "\n- **Adaptive-OPRO 当前倾向**：策略盈利中，可**巩固盈利逻辑**；探索模式下仍可优化结构，避免规则膨胀与矛盾表述。"
		}
		return "\n- **Adaptive-OPRO 当前倾向**：策略盈利中，可**保持当前风格**，微调即可，勿大改。"
	}
	return ""
}

// trajectoryDiagnosisFooter GEPA 反思链末尾诊断指引（探索模式优先净盈利，微调模式保留原「勿压频率」表述）
func trajectoryDiagnosisFooter(explore bool) string {
	if explore {
		return "\n请结合上述轨迹，诊断：① **策略类型与市场是否匹配**（最重要）：本段行情是趋势/震荡/冲突？所选策略（趋势跟随/区间高抛低吸/对冲价差）是否适配？若策略错配则**切换策略类型**，而非加更多限制 ② **盈利决策**的成功原因应强化（方向、时机、策略选择） ③ **亏损决策**的原因：**优先检查是否策略错配**（如震荡市用趋势策略），而非「信号不够强」 ④ 按 **A/B/C/D** 归纳有害模式——**优先**「切换策略 + 建议 + 好处/风险」；**避免**仅堆「禁止」「必须」导致过度保守；**禁止**亏损后唯一对策为「加更严格确认条件」 ⑤ **探索模式以扣费后净盈利为先**，但**小样本勿过拟合**；若长期无成交，几乎肯定是**策略类型错配而非信号门槛不够高** ⑥ **禁止**放宽入场条件至单项即可。\n"
	}
	return "\n请结合上述轨迹，诊断：① **策略类型与市场是否匹配**（趋势/震荡/冲突 → 趋势跟随/区间/对冲？错配则切换） ② **盈利决策**成功原因应强化 ③ **亏损决策**优先检查策略错配 ④ **避免**满篇硬禁令致零开仓；亏损对策**不是**「更保守」而是「换策略」 ⑤ 勿添加降低频率的规则 ⑥ **禁止**放宽至单项即可。\n"
}

// trainingDeadlockLikely 训练是否出现「越训越不敢开 / 长期无成交」迹象，用于注入解锁指令与软化全局硬套。
func trainingDeadlockLikely(o SessionOverview) bool {
	if o.StepsSinceLastOpen >= 10 {
		return true
	}
	if o.ConsecutiveNoTradeSteps >= 7 {
		return true
	}
	if o.GEPAWindowTotalSteps >= 8 && o.GEPAWindowNoTradeSteps >= 6 {
		return true
	}
	if o.GEPAWindowTotalSteps >= 5 && o.GEPAWindowNoTradeSteps == o.GEPAWindowTotalSteps {
		return true
	}
	return false
}

// buildTrainingDeadlockUnlockBlock 僵死时强制优化器：分场景化、禁止再叠全局 wait 条款。
func buildTrainingDeadlockUnlockBlock(o SessionOverview) string {
	if !trainingDeadlockLikely(o) {
		return ""
	}
	return fmt.Sprintf(`
## ⚠️ 训练僵死解锁（本轮与「防过拟合」同等优先）
**数据**：距上次开仓 **%d** 决策步；连续 **%d** 步无模拟器成交；本 GEPA 窗口 **%d/%d** 步无成交。

**本轮改 prompt 必须**：
1. **按场景拆分**：用 **A/B/C/D 或 趋势/震荡/弱趋势/数据差** 等分支，分别写清「该场景下第二项动能如何认定、仓位与杠杆倾向」；**禁止**再用**一条**全局过严条件套死所有状态（这是训练卡死主因之一）。
2. **至少一条分支**须允许在**有基本方向+可写清依据**时用 **低杠杆、小名义** 试错；**禁止**新增「除非多周期完美共振否则一律 wait」类全局句。
3. **亏损**：归因到「**该场景下**判断错/确认不足」，用建议+风险修补该分支；**禁止**用新的一刀切禁令让全场景陪葬。
4. 底线不变：**仍禁止**把任一场景写成「单项无第二证据即可满仓」；可用**分场景的第二项定义**解决僵死，**不等于**降为单项。
5. **0 开平仓窗口视为失败，不是中性结果**：若本轮/本窗长期无成交，改进版必须降低僵死，而不是把 wait 包装成“更优策略”。

`, o.StepsSinceLastOpen, o.ConsecutiveNoTradeSteps, o.GEPAWindowNoTradeSteps, o.GEPAWindowTotalSteps)
}

// buildGEPABatchMetaSuffix GEPA 元提示中「反思步骤 + 优化要求 + 硬约束 + JSON」整段（explore=探索默认，refine=微调）
func buildGEPABatchMetaSuffix(cfg *Config, curLen int, o SessionOverview) string {
	explore := cfg.GEPAPromptExplore()
	deadlock := trainingDeadlockLikely(o)
	var sb strings.Builder
	sb.WriteString("## 反思步骤（必做，GEPA 式）\n")
	if explore {
		sb.WriteString("在输出改进版之前，请先诊断：① **盈利交易**的成功原因（如方向选择、持仓时间、策略周期匹配等）应总结并强化 ② **亏损交易**的可能原因（如方向选择、持仓时间、策略周期与执行周期是否匹配等）应针对性改进 ③ **分状态有害模式**：相对 A/B/C/D，归纳「该 wait 却开仓」「该跟随却踏空」「震荡里追涨杀跌」「数据可疑仍交易」等——**优先**写成「**建议** + **典型好处 / 典型风险**」；**避免**大量「禁止」「必须」堆砌，以免训练期过度保守、长期 wait、无成交；**仅**对重复同质亏损写少量可检验阈值 ④ **探索阶段以扣费后净盈利为首要目标**，但**勿**因单笔亏损或小样本把具体币种/价位写死成永久硬规则（过拟合）⑤ **禁止**将任一场景的入场条件放宽至「单项即可」⑥ **无成交/长期 wait**：优先 **JSON 可解析**、**分场景**厘清第二项认定；若健康度差，**必须**拆分场景而非再叠全局禁令。**禁止**用「同一套第二项」卡死所有市场状态。⑦ **重复犯同一种错**时，必须先解释该信号在市场中代表什么（趋势启动 / 区间边界 / 相对强弱 / 噪音），以及为什么上次处理方式失效；改进方向应优先是**换处理方式**（区间/对冲/减仓/延后确认/持有更久），而不是简单「少开仓」「多等待」。思考后再输出。\n\n")
	} else {
		sb.WriteString("在输出改进版之前，请先诊断：① **盈利交易**的成功原因（如方向选择、持仓时间、策略周期匹配等）应总结并强化 ② **亏损交易**的可能原因（如方向选择、持仓时间、策略周期与执行周期是否匹配等）应针对性改进 ③ **分状态有害模式**：**优先**「建议 + 好处/风险」对照；**避免**满篇硬禁令致零开仓 ④ 勿添加降低开仓频率的规则 ⑤ **禁止**将任一场景放宽至单项即可 ⑥ **无成交步**：改进 JSON；健康度差时**分场景**定义第二项，**禁止**全局再加一刀切不可开仓。⑦ **若同一种错误重复出现**，改进版必须先解释信号含义与失效原因，再把修复写成**换处理方式**，而不是单纯减少交易。思考后再输出。\n\n")
	}
	sb.WriteString("## 优化要求：\n")
	if explore {
		sb.WriteString("1. **核心目标**：**优先**提升**扣费后净盈利**与**可执行规则**；在净盈利未企稳前，**交易笔数、开仓次数为次要**。① 盈利单的成功模式应强化写入规则 ② 亏损单须归类（逆势、BTC 主导性、周期错配、止损被扫、手续费损耗等）并针对性修复 ③ 若亏损单高度同质，优先修正**重复错误模式**；并**按 A/B/C/D** 区分有害决策。\n")
		sb.WriteString("2. **探索阶段**：**禁止**用「冷却期」「每日笔数上限」等**纯频率手段**代替质量改进；**允许**为提升净盈利而加入**与信号质量绑定**的门槛（如：同质化亏损时收紧第二项、减少无效反手），**区别于**「到点才能交易」的冷静期。修复方向应优先是**理解信号代表的市场含义**，并探索更合适的处理方式（趋势/区间/对冲/分批/减仓/延后确认），而不是简单把问题写成「少开仓」或「多等待」。\n")
		sb.WriteString("3. **重点调整**：方向选择、持仓管理、策略周期匹配、入场时机、手续费与换手结构。\n")
		sb.WriteString("4. **盈利操作**可强化相关策略；**亏损操作**应分析具体原因，优先用「建议 + 利弊」修复模式，**避免**笼统加硬禁令导致只 wait、不开仓。\n")
		sb.WriteString("4b. **防过拟合**：样本极少时**禁止**把单笔交易、本会话独有细节固化成永久「硬门槛」全文；应泛化为可迁移的利弊说明。\n")
		sb.WriteString("5. 保持输出格式（JSON、action 枚举等）不变\n")
		sb.WriteString("6. 直接输出完整系统提示词，不要解释或 markdown 标记\n")
		sb.WriteString(fmt.Sprintf("7. **探索模式·长度与结构（先删后加）**：当前约 %d 字。输出可为原 prompt 的 **%.2f～%.2f 倍**。**防膨胀铁律**：每次改进**先审视现有内容**，删除或合并以下类型内容：① 重复表述（同一规则在多处用不同措辞说了 2 遍以上）② 已被新规则覆盖的旧规则 ③ 过于具体的历史训练教训（应泛化或删除）④ 冗长的解释性文字（保留结论，删除推导过程）。**删完之后**再考虑是否需要新增内容。目标：**每轮 GEPA 后 prompt 长度不应净增长超过 10%%**；若超过，须在 reasoning 说明新增了什么、为什么必须加。\n", curLen, cfg.TrainGEPAExploreLenMin, cfg.TrainGEPAExploreLenMax))
		sb.WriteString("8. **必须全部使用中文**（技术标识如 open_long、JSON 等除外）\n")
		sb.WriteString("9. **加仓门槛**：同币种已有同向头寸时，新增开仓需**信号强度显著增强**（突破后回踩确认、新级别背离、量能二次放大），否则视为重复开仓\n")
		sb.WriteString("10. **JSON 输出（不可删减）**：必须明确「禁止只输出思维链不输出 JSON；思维链不超过 5 行；无操作也必须输出 []；每步结尾必须包含 json 代码块及决策数组，否则视为 0 操作」\n")
		sb.WriteString("11. **量能标准**：做空条件统一为「放量跌破」优先，「量能萎缩」仅作辅助；避免矛盾表述\n")
		sb.WriteString("12. **多策略框架（强制结构）**：改进版正文须显式包含「Step 0：先判市场类型再选策略」+ 「按 A/B/C/D 分支」的决策逻辑；**每种状态**分别给出**专属策略类型**（A趋势跟随 / B区间高抛低吸 / C对冲价差 / D不交易）和**对应入场标准**。**禁止**只写一套全局入场条件适用于所有状态。\n")
		sb.WriteString("\n## ⚠️ 锚段保护（以下段落必须原封不动保留，禁止修改、删除、改写或合并）\n")
		sb.WriteString("改进版输出中，以下段落必须**逐字保留**（只允许在这些段落之外进行修改）：\n")
		sb.WriteString("- **「## JSON 与字段」** 整段（从标题到 `---` 分隔符）——这是系统解析的技术硬约束\n")
		sb.WriteString("- **「## Step 0：先判市场类型，再选策略」** 整段——这是多策略框架的核心入口\n")
		sb.WriteString("- **「## 行情状态与对策」中的 A/B/C/D 表格**——策略类型映射不可更改（A趋势跟随/B区间高抛低吸/C对冲价差/D不交易）\n")
		sb.WriteString("- **仓位份数制规则**（「每份 = 100 USDT」「自行决定用几份（1-5 份）」）——仓位灵活性不可回退为固定值\n")
		sb.WriteString("- **「## 核心目标」中的反保守螺旋原则**——「出错不等于应该更保守，出错意味着当前策略不适合当前行情，正确反应是切换策略类型」\n")
		sb.WriteString("- **「止盈规则（按策略类型）」和「平仓纪律」**——ATR追踪止盈、EMA跌破平仓、区间对侧止盈、最大持仓步数15步等核心平仓逻辑不可删除（可微调数值）\n")
		sb.WriteString("- **「价格行为词典」与 A/B 入场法**——回踩均线、趋势线/支撑回踩、前高/箱体/颈线突破、假突破回归、箱体/布林带边界、中轨不新开仓等语义不可删除（可压缩表述）\n")
		sb.WriteString("如果改进版遗漏了上述任何锚段，视为无效输出。\n\n")
		sb.WriteString("## GEPA 输出风格（反保守螺旋、策略多样性）\n")
		sb.WriteString("- **亏损后正确反应是「切换策略类型」而非「加更多限制」**：趋势策略亏 → 检查是否该用区间/对冲；区间策略亏 → 检查区间是否被突破（切到趋势）。**禁止**所有亏损对策都是「更严格确认」。\n")
		sb.WriteString("- **等待是最后选项**：若信号还能被解释为区间边界、相对强弱、趋势延续或局部试探，则优先探索相应盈利方式；只有数据无效、收益空间不足覆盖 fee、或确实无可解释优势时才 wait。\n")
		sb.WriteString("- **执行稳定性**：若日志出现同价开平、开仓后 1-2 步立刻平仓、同类信号连续亏损，改进版必须加入“解释信号含义并改变处理方式”的指令，避免模型重复犯同一种错。\n")
		sb.WriteString("- **策略正文优先「建议 / 倾向」**，并配 **「好处 / 风险」** 对照；**避免**满篇「禁止」「必须」——易引发保守螺旋。\n")
		sb.WriteString("- **小样本警惕**：样本少时勿把具体币种、价位写死成永久条款。\n")
		sb.WriteString("- **若需收紧**：说明「**为何**改善净盈利」，**不要**只堆「不得开仓」类禁令。\n")
		sb.WriteString("- **长期无成交（>20步）**：几乎肯定是**策略类型错配**——改进版须包含多策略分支，而非更高入场门槛。\n")
		sb.WriteString("- **B 态执行要求**：若判断为震荡市但价格在中轨附近、暂无边界优势，则应明确输出 `[]` 或仅管理已有仓位；**禁止**识别出 B 态后只写分析却不给 JSON。\n")
		sb.WriteString("- 下文 **「硬约束」** 仅保留底线（JSON、分场景策略），**策略叙事**仍以建议为主。\n\n")
	} else {
		sb.WriteString("1. **核心目标**：找到更好的**盈利策略**（含策略类型选择），并**兼顾胜率与净盈利（扣费后 PnL）**：① **首先检查策略类型是否匹配市场**（趋势跟随/区间高抛低吸/对冲价差）——策略错配是亏损最常见根因 ② 盈利单的成功模式（方向、策略类型、持仓、周期一致）应强化 ③ 亏损单**优先检查策略错配**（如震荡市用趋势策略），而非笼统加「少开仓」\n")
		sb.WriteString("2. **禁止**添加降低开仓频率的风控；**禁止**亏损后唯一对策为「更严格确认条件」——应优先考虑**切换策略类型**与**改变处理方式**（区间/对冲/减仓/延后确认/持仓管理）\n")
		sb.WriteString("3. **重点调整**：策略类型选择 > 入场时机 > 方向选择 > 持仓管理\n")
		sb.WriteString("4. **亏损操作**优先检查策略错配，用「切换策略 + 建议 + 利弊」改进，**避免**只加硬禁令致零开仓\n")
		sb.WriteString("4b. **防保守螺旋**：亏损→加限制→更少交易→更少样本→更无法学习→更亏——**禁止**此循环。小样本勿过拟合。\n")
		sb.WriteString("5. 保持输出格式（JSON、action 枚举等）不变\n")
		sb.WriteString("6. 直接输出完整系统提示词，不要解释或 markdown 标记\n")
		sb.WriteString(fmt.Sprintf("7. **微调模式·长度（先删后加）**：当前约 %d 字。输出可为原 prompt 的 **%.2f～%.2f 倍**。**防膨胀铁律**：改进前**必须先扫描**现有 prompt，删除或合并：① 重复表述（同义不同措辞出现 2 次以上）② 被新规则取代的旧内容 ③ 过度具体的历史案例（应泛化）④ 冗长解释（保结论删推导）。**先瘦身再补丁**——若当前已 >20000 字，应**尽可能**精简以减少总长度。\n", curLen, cfg.TrainGEPARefineLenMin, cfg.TrainGEPARefineLenMax))
		sb.WriteString("8. **必须全部使用中文**（技术标识如 open_long、JSON 等除外）\n")
		sb.WriteString("9. **加仓门槛**：同币种已有同向头寸时，新增开仓需**信号强度显著增强**（突破后回踩确认、新级别背离、量能二次放大），否则视为重复开仓\n")
		sb.WriteString("\n## ⚠️ 锚段保护（以下段落必须原封不动保留）\n")
		sb.WriteString("- **「## JSON 与字段」** 整段——系统解析硬约束\n")
		sb.WriteString("- **「## Step 0」** 整段——多策略框架核心入口\n")
		sb.WriteString("- **A/B/C/D 表格中的策略类型映射**（A趋势跟随/B区间高抛低吸/C对冲价差/D不交易）\n")
		sb.WriteString("- **仓位份数制规则**（每份100U，自选1-5份）\n")
		sb.WriteString("- **反保守螺旋原则**（出错→换策略类型，不是→更保守）\n")
		sb.WriteString("- **止盈规则与平仓纪律**——ATR追踪、EMA跌破、区间对侧、最大持仓15步等核心逻辑不可删除\n")
		sb.WriteString("- **价格行为词典与 A/B 入场法**——回踩均线、趋势线/支撑回踩、前高/箱体/颈线突破、假突破回归、箱体/布林带边界、中轨不新开仓等语义不可删除\n")
		sb.WriteString("10. **JSON 输出（不可删减）**：必须明确「禁止只输出思维链不输出 JSON；思维链不超过 5 行；无操作也必须输出 []；每步结尾必须包含 json 代码块及决策数组，否则视为 0 操作」\n")
		sb.WriteString("11. **量能标准**：做空条件统一为「放量跌破」优先，「量能萎缩」仅作辅助（下跌趋势中量能萎缩表示抛压减弱，可配合使用）；避免矛盾表述\n")
		sb.WriteString("12. **技能调用（若使用技能库）**：若 prompt 含「检索到的相关技能」，应确保决策时**正确应用**相关技能：仅当技能直接匹配当前场景时应用，否则忽略；相关技能应融入决策逻辑，勿显式引用；硬约束（JSON 格式、止损距离）> 亏损规避 > 成功模式 > 一般规则\n")
		sb.WriteString("13. **分场景策略（强制结构）**：须含按状态分支的入场与风控；**禁止**单一全局条件套死所有场景。\n\n")
		sb.WriteString("## GEPA 输出风格（防过拟合、避免训练期零开仓）\n")
		sb.WriteString("- **策略正文优先「建议 / 倾向」**，并配 **「遵循的典型好处 / 忽视的典型风险」** 对照；**避免**把改进版写成满篇「禁止」「必须」「硬约束」——易引发过度保守、长期 wait、有效步减少。\n")
		sb.WriteString("- **小样本警惕**：样本少或单笔亏损时，**勿**把具体币种、价位、本会话步数写死成永久条款；应**泛化**为可迁移的利弊说明。\n")
		sb.WriteString("- **若需收紧**：说明「**为何**可能改善净盈利 / **为何**仍可能亏」，**不要**只堆「不得开仓」类无解释禁令。\n")
		sb.WriteString("- **等待是备选，不是默认**：改进版应要求模型在 wait 前先检查是否可转为趋势、区间、对冲、减仓试错或延后确认。\n")
		sb.WriteString("- **B 态中轨规则**：若处于区间中轨附近且无边界优势，改进版应明确要求输出 `[]` 或仅管理已有仓位，而不是重复输出无 JSON 的震荡分析。\n")
		sb.WriteString("- 下文 **「硬约束」** 仅保留项目底线（JSON、分场景下至少两项等），**策略叙事**仍以建议为主。\n\n")
	}
	sb.WriteString("**硬约束（禁止违反，但须分场景落实）**：\n")
	sb.WriteString("- **禁止**将任一场景的入场条件放宽至「单项即可」「无第二证据即可」——每一状态下仍须 **至少两项** 且第二项为**该场景下认可的动能/结构证据**；**允许且鼓励**在正文中**按 A/B/C/D（或趋势/震荡等）分别定义**「第二项」合格标准，**禁止**用**同一条**过严第二项描述套死所有状态（易导致训练全程 wait）。\n")
	sb.WriteString("- **禁止**删除「满足该场景条件即开、不满足即等」的分支逻辑。\n")
	sb.WriteString("- **禁止**删除止损、最小持仓时间、方向一致性等约束\n")
	sb.WriteString("- **禁止**添加「无依据乱开仓」「不分析即满仓」「模糊时重仓」等表述；**允许**写「在 **X 场景** 下积极用 **低杠杆、小名义** 试错（须写清依据）」。\n")
	sb.WriteString("- **同向再开（质量，非死数）**：应用「**新结构或显著强于平仓前**的第二项证据」约束同标的同向再开；**勿**用「固定间隔 N 步」类死数卡死，除非平仓回合已证明同质追单（即便如此也优先写**信号强度**而非步数）。\n")
	sb.WriteString("- **禁止**把修复重复亏损简单写成「减少开仓」或「更久等待」；应优先解释信号含义，并探索**换策略类型、换执行方式、换持仓管理、换风险表达**等替代处理。\n")
	if deadlock {
		sb.WriteString("- **【僵死轮追加】**：若上文已触发僵死解锁，本輪**优先**落实分场景第二项与低杠杆试错路径，**禁止**再叠加新的「全局一律 wait」句。\n")
	}
	sb.WriteString("\n**JSON 可解析率**：若本 batch 日志中大量步骤无决策，改进版须把 **JSON 铁律**前移至文首附近、缩短与 JSON 冲突的长段落，确保模型每步输出含 **json 代码块**（反引号+json 三字标记）。\n\n")
	sb.WriteString("改进后的系统提示词：")
	return sb.String()
}

// trainingPromptPrefix 训练模式专用前缀：强调在**有依据**的前提下积极评估机会、允许更高试错成本；
// 与 GEPA「平仓回合归因」配套——亏损应被总结为可迁移规则，而非「已知错不总结」。**并非**鼓励无规则乱开仓。
func trainingPromptPrefix(initialEquity float64, fixedPositionPct float64) string {
	minMargin := initialEquity * 0.01 // 最小约 1%，用于 fallback
	if minMargin < 20 {
		minMargin = 20
	}
	fixedMargin := initialEquity * (fixedPositionPct / 100)
	positionLine := ""
	examplePositionSize := initialEquity * 0.02 * 10 // 默认 2% 本金 10x
	if fixedPositionPct > 0 {
		positionLine = fmt.Sprintf("- **固定仓位（训练）**：每笔交易固定使用本金的 %.2f%% 作为保证金。保证金=%.0f USDT，**position_size_usd = %.0f × 杠杆**。示例：10x 杠杆 → position_size_usd=%.0f\n", fixedPositionPct, fixedMargin, fixedMargin, fixedMargin*10)
		examplePositionSize = fixedMargin * 10
	} else {
		positionLine = fmt.Sprintf("- **仓位由模型自行安排**：根据**预计算指标**（MACD、RSI、EMA等）、多周期一致性、可用余额**自主决定**单笔保证金与杠杆。建议：标准机会 1-3%% 本金，优质机会 3-5%%，强信号 5-10%%。单笔保证金不超过可用余额的 50%%。\n- **position_size_usd** = 保证金 × 杠杆。保证金 = 本金 × 你选择的百分比（如 2%% 则 %.0f×0.02=%.0f）。示例：2%% 本金、10x 杠杆 → position_size_usd=%.0f\n", initialEquity, initialEquity*0.02, initialEquity*0.02*10)
	}
	return fmt.Sprintf(`# 训练模式说明（覆盖下方冲突规则）

**当前为训练模式**：核心是**在规则内提高方向与时机判断能力**；训练允许更多**试错成本**，亏了多为**方向或归纳未到位**，下一版 prompt 经 GEPA 应吸收教训。**积极开仓 ≠ 乱开仓**：须有可写进 reasoning 的多周期/量价依据，宁可**小仓、低杠杆**试错，不可无依据重复试错。

- **每步复盘（帮助 prompt 学习）**：User Prompt 顶部可能含「训练专用·历史截面复盘」段落——仅用**当前时刻及以前**的已收盘 K 线算出的截面强弱参照（**无未来数据、非下单指令**）。请把它当作**复盘锚点**：对照你的思维链与决策，看是否漏判、是否与规则/JSON 冲突；**实盘不会出现本段**。该步骤等价于轻量复盘，便于后续 GEPA 从真实轨迹中学习；**平仓回合**会在 GEPA 侧被配对归因，**禁止**把训练亏损当成可忽略噪声而不进入规则迭代。
- **历史训练 composite（辅助线，可关）**：User Prompt 中可能含「历史最优参照」段落（仓库 baseline_state.json 自动晋升），**非下单指令**；**与下面「回放全知多段摆动」不同**。设置环境变量 **NOFX_TRAIN_BASELINE_HINT=0** 可关闭本段，仅保留全知参照与因果参照。
- **回放全知多段摆动参照（训练核心参照）**：User Prompt 含「回放全知多段摆动参照」——对**本段已发生**的 15m 收盘价路径，在**已知整段走势**下经 **δ 阈值**过滤极点后，**多段**主摆动毛收益合计（每段扣约 0.1%%×2 粗算）；含**自上次 GEPA / 近 1h / 4h / 12h** 等窗。**即使本步无成交**也会给出。**不是**实盘可复现承诺，**禁止**为追数字违反硬约束。**决策步**仅含机械块；「训练专用·AI 对「回放全知」的评估」仅在 **GEPA 改 prompt（doBatchTrainStep）** 时可选插入（**NOFX_TRAIN_HISTORICAL_AI=0** 或 **-train-hindsight-ai=false** 可关闭以省 API）。

- **训练效率（重要）**：在**有基本方向与波动可读**时**优先评估开仓**（含小杠杆试错），避免因过度胆怯而长期空步；**不是**无差别每步必开。此条仅用于训练，实盘可去除。

- **决策前仔细思考**：做出任何开仓/平仓决策前，必须**仔细分析**多周期数据、技术指标、持仓状态，**充分论证**决策理由。禁止冲动决策、草率平仓或盲目开仓。思维链中应体现：① 当前市场状态 ② 持仓盈亏与止盈止损距离 ③ 拟操作的理由与依据。

- **初始本金**：%.0f USDT
%s- **杠杆**：1-20 倍可自选，根据信号强度选择；手续费按币安合约 Taker 费率：单边 0.05%%（开仓+平仓合计 0.1%%），基于 position_size_usd 计算。
- **开仓积极、平仓耐心（铁律）**：**积极开仓**寻找机会，但**必须到开仓时设定的止盈点或止损点再平仓**。**禁止**在未触及止盈/止损时主动平仓——提前平仓会白白损失手续费，且无法让盈利奔跑。持仓期间用 hold 维持，等待市场触及你的计划。
- **全平+反向需确认反转**：全平并开反向仓是允许的，但**必须确认趋势已反转**（如多周期 MACD 反向、价格破关键 EMA、量能配合等）。**禁止**在未充分论证反转时冲动全平+反向——决策前仔细思考：① 大周期是否已转向？② 当前平仓理由是否充分？③ 反向开仓信号是否足够强？
- **平仓条件（必须满足其一）**：① 价格触及止损价 ② 价格触及止盈价 ③ 趋势明确反转（如 MACD 死叉+价格破关键 EMA）。**禁止**因「信号变化」「想换仓」等理由提前平仓。
- **最小持仓时间**：开仓后至少持仓 3 步（约 9 分钟）才可主动平仓，除非触及止损/止盈。
- **让盈利奔跑**：盈利单不要因「已盈利」而提前止盈，应持有至止盈点或趋势衰竭信号；亏损单严格按止损执行。
- **结合当前持仓判断**：决策前必须审视现有持仓（方向、盈亏、止盈止损距离），新开仓需与持仓方向一致性协调，避免冲突。
- **同币种同向合并**：模拟器与提示逻辑中，同币种同向为**一条合并仓位**；再次 open_* 为**加仓**（累加数量、重算均价），不会因「两条持仓满额」被拒。**加仓门槛**：已有同向头寸时，新增开仓需**信号强度显著增强**（突破后回踩确认、新级别背离形成、量能二次放大 200%%+），否则视为无效加仓，应 wait。
- **训练优势（测试环境）**：测试环境允许**更高频率的合规试错**，积极分析**在什么条件下值得开仓**；**禁止**用「交易间隔」「操作次数限制」等**实盘风控**当借口长期不开仓——但**仍须**写清 reasoning，且**亏损由后续 GEPA 归纳进规则**，不是「试了白试」。
- **冷静期禁用（训练铁律）**：训练阶段**不强制**「平仓后必须等 N 分钟再开」类**纯时间冷静期**（与**信号质量门槛**不同：后者允许且鼓励）。下方「平仓后等待15分钟」「强制冷静期」等**若仅按时间卡**则在训练中不适用；**若平仓后立刻同向再开，仍须满足加仓/再开门槛**（见同币种加仓与 GEPA 配对回合），避免追单式重复错。
- **开仓态度（重要）**：在**有可读信号**时**积极**给出开仓方案（含 1-3x 小杠杆试错）；多周期有基本方向即可尝试，不必等完美共振。**若**多周期严重背离、无任何技术信号，允许 **wait**，但**禁止**用空数组 [] 代替——须用非空 JSON 数组写 wait/hold 条目；**仅 D 态（数据可疑）**可单独输出仅含 [] 的数组。**并非**要求每步必开。
- **入场条件放宽（训练）**：实盘「至少两项信号」在训练中**放宽**——有一项基本方向（如 15m 趋势、1h 偏多/偏空）即可用 1-3x 小杠杆试错。**禁止**以「候选币种信号不足」「至少两项信号未满足」为由连续多步不开仓。
- **持仓与新开仓可并行**：若现有持仓未触发止损/止盈且趋势未破，可**同时**寻找新标的开仓，不必等全部平仓后再开。根据可用余额自行安排仓位。
- **训练 vs 实盘（关键）**：实盘中「信号模糊时观望」常更稳；**训练中**则要在模糊、冲突、震荡里**练习给出可论证的决策**——倾向**积极评估开仓**（小杠杆 1-3x）或**明确写出为何 wait**，二者都合法；**禁止**用「等完美结构」长期空步当借口。**禁止**在思维链中把「积极」偷换成「无依据乱开」——须始终能回答 **依据何在**；**亏损**由 GEPA 从真实轨迹与平仓回合中**归纳真因**，写入下一版规则，**不是**「方向错了就白亏」。
- **覆盖下方规则**：下方「不确定时观望」「任一回答否→选 wait」等保守规则在训练中**部分放宽**：有基本方向、有波动可读时，**优先尝试**用 small size/低杠杆表达观点；**仍**须满足 JSON 与 reasoning 可检验性。
- **分场景（与 GEPA 一致）**：实盘/基座 prompt 中应能通过 **A/B/C/D（或趋势/震荡等）** 区分策略；**禁止**用同一套过严条件解释所有市场——训练僵死多源于「一刀切」。若 GEPA 报告健康度差，下一版须**拆分场景**而非再叠全局禁令。
- **保证金与可用余额**：开仓前检查可用余额，确保 所需保证金+手续费 ≤ 可用余额。**禁止**以「保证金已达上限」为由不开仓——若可用余额充足，应积极开仓。
- **开仓 JSON 格式（必须遵守）**：开仓时 action 必须为 open_long 或 open_short（不能用 open），且**必填**：leverage、position_size_usd、stop_loss、take_profit、reasoning。**position_size_usd 禁止为 0 或省略**——根据上方仓位规则计算。**决策依据**：参考各币种预计算指标（MACD、RSI等）与历史回测（若有），不填 confidence。示例：
  {"symbol":"BTCUSDT","action":"open_long","leverage":10,"position_size_usd":%.0f,"stop_loss":65000,"take_profit":68000,"reasoning":"简要信号"}
  字段名必须用 position_size_usd（不是 size_usd），action 必须用 open_long/open_short（不是 open）。**JSON 铁律（必须遵守）**：**禁止**只输出思维链不输出 JSON。无操作**禁止**仅用单独的 []——须输出含 wait/hold 的非空数组；单独的 [] 仅 D 态例外。必须以 %sjson 开头输出决策数组，不能省略或夹杂在长段分析中。否则系统无法解析，本步将视为 0 操作。

---
`, initialEquity, positionLine, examplePositionSize, "```")
}

// loadLatestPrompt 优先加载上次训练完成时的最终 prompt，保证每次训练都基于上次最终结果
// 优先级：1) 完整训练结束文件 *_batch_final_*  2) 各 batch 的 _final_ 文件，按修改时间取最新
// 扫描范围：`candidates/` 根目录 + `candidates/run_*/` 子目录（与按会话归档一致）
func loadLatestPrompt(baseName, baselinePath string) (string, string, bool) {
	dir := filepath.Join("prompt_simulator", "training", "candidates")
	runCompletionPrefix := baseName + "_batch_final_"
	batchPrefix := baseName + "_batch"
	var candidates []struct {
		path         string
		info         os.FileInfo
		isRunFinal   bool
		isBatchFinal bool
	}
	addIfMatch := func(fullPath string) {
		base := filepath.Base(fullPath)
		if !strings.HasSuffix(base, ".txt") {
			return
		}
		if !strings.HasPrefix(base, batchPrefix) {
			return
		}
		info, err := os.Stat(fullPath)
		if err != nil || info == nil {
			return
		}
		name := base
		isRunFinal := strings.HasPrefix(name, runCompletionPrefix)
		isBatchFinal := strings.Contains(name, "_final_")
		candidates = append(candidates, struct {
			path         string
			info         os.FileInfo
			isRunFinal   bool
			isBatchFinal bool
		}{path: fullPath, info: info, isRunFinal: isRunFinal, isBatchFinal: isBatchFinal})
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", false
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "run_") {
			sub, err := os.ReadDir(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			for _, se := range sub {
				if se.IsDir() {
					continue
				}
				addIfMatch(filepath.Join(dir, e.Name(), se.Name()))
			}
			continue
		}
		if e.IsDir() {
			continue
		}
		addIfMatch(filepath.Join(dir, e.Name()))
	}
	if len(candidates) == 0 {
		return "", baselinePath, false
	}
	// 1) 完整训练结束文件优先  2) 否则 batch _final_ 按修改时间取最新
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].isRunFinal != candidates[j].isRunFinal {
			return candidates[i].isRunFinal
		}
		if candidates[i].isBatchFinal != candidates[j].isBatchFinal {
			return candidates[i].isBatchFinal
		}
		return candidates[i].info.ModTime().After(candidates[j].info.ModTime())
	})
	raw, err := os.ReadFile(candidates[0].path)
	if err != nil {
		return "", baselinePath, false
	}
	return string(raw), candidates[0].path, true
}

// writeTrainingRunMeta 会话目录元数据，便于事后对照参数与回溯
func writeTrainingRunMeta(runDir, runID string, cfg *Config) error {
	meta := map[string]interface{}{
		"run_id":                runID,
		"train_base":            cfg.TrainBase,
		"started_at":            time.Now().Format(time.RFC3339),
		"symbols":               cfg.Symbols,
		"step_interval":         cfg.StepInterval,
		"max_steps":             cfg.MaxSteps,
		"max_batches":           cfg.TrainMaxBatches,
		"train_every_steps":     cfg.TrainEverySteps,
		"train_extra_every_ops": cfg.TrainExtraEveryOps,
		"train_on_close":        cfg.TrainOnClose,
		"initial_equity":        cfg.InitialEquity,
		"train_position_pct":    cfg.TrainFixedPosition,
		"train_gepa_mode":       cfg.TrainGEPAPromptMode,
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "run_meta.json"), b, 0644)
}

// opsHasClose 本步是否发生平仓（用于「平仓后触发 GEPA」）
func opsHasClose(ops []TradeOp) bool {
	for _, op := range ops {
		if op.Action == "close_long" || op.Action == "close_short" {
			return true
		}
	}
	return false
}

// RunBatchTraining 步进式训练：每 N 次开平仓操作进行一次 batch 训练，或 -train-on-close 时每平仓一次训练
func RunBatchTraining(cfg *Config, mcpClient *mcp.Client) (bestPrompt string, err error) {
	var currentPrompt string
	var skipTrainPrefix bool
	if cfg.TrainBase == "pretrained" {
		// 优先使用最新 batch 候选，若无则用预训练融合基线
		content, usedPath, _ := loadLatestPrompt("pretrained", pretrainedBaselinePath)
		if content != "" {
			currentPrompt = content
			skipTrainPrefix = false // 始终注入强调积极开仓的前缀，覆盖候选中的保守描述
			if cfg.LogASCII {
				log.Printf("Using prompt: %s", usedPath)
			} else {
				log.Printf("📚 使用最新 prompt: %s", usedPath)
			}
		} else {
			raw, err := os.ReadFile(pretrainedBaselinePath)
			if err != nil {
				if cfg.LogASCII {
					log.Printf("Pretrain baseline missing, fallback to default. Run: ./simulator_bin -pretrain")
				} else {
					log.Printf("⚠️ 预训练基线不存在，回退到 default。建议先执行：./simulator_bin -pretrain")
				}
				baseTpl, fallbackErr := decision.GetPromptTemplate("default")
				if fallbackErr != nil {
					return "", fmt.Errorf("加载预训练基线失败且 default 回退失败: %w", err)
				}
				currentPrompt = baseTpl.Content
			} else {
				currentPrompt = string(raw)
				if cfg.LogASCII {
					log.Printf("Using pretrained baseline: %s", pretrainedBaselinePath)
				} else {
					log.Printf("📚 使用预训练融合 prompt 作为基线（%s）", pretrainedBaselinePath)
				}
			}
		}
	} else {
		// pretrained_batch_20260312：优先用上次训练的 batch_final，若无则用完整模板（prompts/pretrained_batch_20260312.txt）
		if cfg.TrainBase == "pretrained_batch_20260312" {
			content, usedPath, _ := loadLatestPrompt(cfg.TrainBase, "")
			if content != "" {
				currentPrompt = content
				if cfg.LogASCII {
					log.Printf("Using last trained prompt: %s", usedPath)
				} else {
					log.Printf("📚 使用上次训练产出作为基线: %s", usedPath)
				}
			} else {
				baseTpl, err := decision.GetPromptTemplate(cfg.TrainBase)
				if err != nil {
					return "", fmt.Errorf("加载基线模板 %s 失败: %w", cfg.TrainBase, err)
				}
				currentPrompt = baseTpl.Content
				skipTrainPrefix = true // 3.12 模板已含完整训练说明，不注入激进 prefix（避免覆盖「至少两项」等规则）
				if cfg.LogASCII {
					log.Printf("Using template: %s (no batch_final, skip train prefix)", cfg.TrainBase)
				} else {
					log.Printf("📚 使用 3.12 基线模板（无 batch_final，跳过训练前缀）: %s", cfg.TrainBase)
				}
			}
		} else if cfg.TrainBase == "pretrained_batch_profitable_20260319_train" {
			// 盈利训练基座：优先接续 candidates 里同名 batch_final / batchN_final
			content, usedPath, _ := loadLatestPrompt(cfg.TrainBase, "")
			if content != "" {
				currentPrompt = content
				skipTrainPrefix = true
				if cfg.LogASCII {
					log.Printf("Using last trained prompt: %s", usedPath)
				} else {
					log.Printf("📚 使用上次训练产出作为基线: %s", usedPath)
				}
			} else {
				baseTpl, err := decision.GetPromptTemplate(cfg.TrainBase)
				if err != nil {
					return "", fmt.Errorf("加载基线模板 %s 失败: %w", cfg.TrainBase, err)
				}
				currentPrompt = baseTpl.Content
				skipTrainPrefix = true
				if cfg.LogASCII {
					log.Printf("Using template: %s (no batch_final, skip train prefix)", cfg.TrainBase)
				} else {
					log.Printf("📚 使用盈利训练模板（无 batch_final，跳过训练前缀）: %s", cfg.TrainBase)
				}
			}
		} else if cfg.TrainBase == "训练-固定100U-基准-20260323" || cfg.TrainBase == "训练-固定100U-状态机完整-20260323" ||
			cfg.TrainBase == "训练-固定100U-基准-20260325" || cfg.TrainBase == "训练-固定100U-状态机完整-20260325" {
			// 20260323/20260325 训练基座（中文名）：默认仅用 prompts/<base>.txt；-train-from-prompts=false 或 -train-chain 时优先 candidates 最新 batch_final
			// 训练前缀（trainingPromptPrefix）含「训练放宽第二项、小仓试错、错误亦样本」等系统级约束；此前此处固定 skipTrainPrefix=true，导致仅靠正文 RevN 仍易全程 []。
			// 默认改为注入前缀；若与正文重复或需纯正文，设环境变量 NOFX_TRAIN_SKIP_PREFIX=1。
			skip20260325Prefix := os.Getenv("NOFX_TRAIN_SKIP_PREFIX") == "1"
			if cfg.TrainFromPromptsOnly && !cfg.TrainChain {
				baseTpl, err := decision.GetPromptTemplate(cfg.TrainBase)
				if err != nil {
					return "", fmt.Errorf("加载基线模板 %s 失败: %w", cfg.TrainBase, err)
				}
				currentPrompt = baseTpl.Content
				skipTrainPrefix = skip20260325Prefix
				if cfg.LogASCII {
					log.Printf("Using prompts only: %s (skip candidates batch_final)", cfg.TrainBase)
				} else {
					if skip20260325Prefix {
						log.Printf("📚 默认仅用 prompts 模板：%s，不读取 candidates；**已跳过**训练前缀（NOFX_TRAIN_SKIP_PREFIX=1）", cfg.TrainBase)
					} else {
						log.Printf("📚 默认仅用 prompts 模板：%s，不读取 candidates；**将注入**训练前缀（含试错/样本约束；设 NOFX_TRAIN_SKIP_PREFIX=1 可关闭）", cfg.TrainBase)
					}
				}
			} else {
				content, usedPath, _ := loadLatestPrompt(cfg.TrainBase, "")
				if content != "" {
					currentPrompt = content
					skipTrainPrefix = skip20260325Prefix
					if cfg.LogASCII {
						log.Printf("Using last trained prompt: %s", usedPath)
					} else {
						if skip20260325Prefix {
							log.Printf("📚 使用上次训练产出作为基线: %s（已跳过训练前缀）", usedPath)
						} else {
							log.Printf("📚 使用上次训练产出作为基线: %s（将注入训练前缀）", usedPath)
						}
					}
				} else {
					baseTpl, err := decision.GetPromptTemplate(cfg.TrainBase)
					if err != nil {
						return "", fmt.Errorf("加载基线模板 %s 失败: %w", cfg.TrainBase, err)
					}
					currentPrompt = baseTpl.Content
					skipTrainPrefix = skip20260325Prefix
					if cfg.LogASCII {
						log.Printf("Using template: %s (no batch_final)", cfg.TrainBase)
					} else {
						if skip20260325Prefix {
							log.Printf("📚 使用 20260323/20260325 baseline 训练模板（无 batch_final，已跳过训练前缀）: %s", cfg.TrainBase)
						} else {
							log.Printf("📚 使用 20260323/20260325 baseline 训练模板（无 batch_final，将注入训练前缀）: %s", cfg.TrainBase)
						}
					}
				}
			}
		} else {
			baseTpl, err := decision.GetPromptTemplate(cfg.TrainBase)
			if err != nil {
				return "", fmt.Errorf("加载基线模板 %s 失败: %w", cfg.TrainBase, err)
			}
			currentPrompt = baseTpl.Content
		}
	}
	// 训练模式：在基线前添加训练专用前缀（batch 候选已含则跳过）
	if !skipTrainPrefix {
		trainPrefix := trainingPromptPrefix(cfg.InitialEquity, cfg.TrainFixedPosition)
		currentPrompt = trainPrefix + currentPrompt
		if cfg.LogASCII {
			log.Printf("Train mode: injected 1%% equity(%.0f), leverage 1-20x, encourage open", cfg.InitialEquity*0.01)
		} else {
			log.Printf("📋 训练模式：已注入 1%% 本金(%.0f)、杠杆 1-20x、鼓励开仓 前缀", cfg.InitialEquity*0.01)
		}
	}

	batchSize := cfg.TrainBatchSize
	if batchSize <= 0 {
		batchSize = 5
	}

	acc := &SimAccount{
		Equity:           cfg.InitialEquity,
		AvailableBalance: cfg.InitialEquity,
		TotalPnL:         0,
		Positions:        nil,
		StartTime:        cfg.StartTime,
		BTCETHLeverage:   cfg.BTCETHLeverage,
		AltcoinLeverage:  cfg.AltcoinLeverage,
	}

	stepDur := cfg.StepDuration()
	tradeOps := make([]TradeOp, 0, batchSize*4)
	allTradeOps := make([]TradeOp, 0, 200)                     // 全量操作记录，训练结束输出
	trajectoryBuf := make([]TrajectoryStep, 0, 20)             // GEPA 轨迹缓冲，最多保留 20 步
	batchTrajectory := make([]TrajectoryStep, 0, batchSize*16) // 自上次 GEPA 起的完整步轨迹（含无成交步），用于错失机会反思
	callCount := 0
	batchNum := 0
	opCount := 0 // 开平仓操作数（开仓+平仓各算1次）
	stepsSinceLastStepTrain := 0
	stepsSinceLastOpen := 0        // 会话级：距上次开仓决策步
	consecutiveNoTrade := 0        // 会话级：连续无成交步
	consecutiveZeroDecision := 0   // 连续「JSON 解析成功但 decisions 为空 []」步数（与 engine 用户侧「无操作写 []」对抗）
	sessionOpenCount := 0          // 会话级：成功开仓次数
	successfulDecisionSteps := 0   // 成功拿到 FullDecision 的步数
	noJSONResponses := 0           // 原始响应缺少 ```json 的步数
	jsonFallbackCount := 0         // 触发 JSON 补调成功的步数
	zeroDecisionSteps := 0         // 最终 decisions 为空的步数
	allWaitSteps := 0              // 最终仅 wait/hold 的步数
	stepOpMark := 0                // allTradeOps 中下标：上次「按步数」GEPA 起算位置
	var trainWindowStart time.Time // 当前训练窗口起点（上次 GEPA 后首步），供 K 线因果参照
	useLegacyBatch := !cfg.TrainOnClose && cfg.TrainEverySteps <= 0 && cfg.TrainExtraEveryOps <= 0
	maxBatches := cfg.TrainMaxBatches
	if maxBatches <= 0 {
		maxBatches = 10
	}

	if err := os.MkdirAll(candidatesDir, 0755); err != nil {
		return "", err
	}
	runID := time.Now().Format("20060102_150405")
	runDir := filepath.Join(candidatesDir, "run_"+runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return "", fmt.Errorf("创建训练会话目录: %w", err)
	}
	outDir := runDir
	if err := writeTrainingRunMeta(outDir, runID, cfg); err != nil {
		log.Printf("⚠️ 写入 run_meta.json 失败: %v", err)
	}
	// 与会话目录一致的控制台日志（直接 go run 时也有落盘；train_skill.ps1 另有一份 tee 时可加 -NoLog 避免两套 run_*）
	logPath := filepath.Join(outDir, "training.log")
	logFile, errLog := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if errLog != nil {
		log.Printf("⚠️ 无法写入会话日志 %s: %v（仅控制台）", logPath, errLog)
	} else {
		prevW := log.Writer()
		log.SetOutput(io.MultiWriter(prevW, logFile))
		defer func() {
			log.SetOutput(prevW)
			_ = logFile.Close()
			if cfg.LogASCII {
				log.Printf("Session console log: %s", logPath)
			} else {
				log.Printf("📝 会话控制台日志已写入（与 run_meta 同目录）: %s", logPath)
			}
		}()
	}
	if cfg.LogASCII {
		log.Printf("Training run dir: %s", runDir)
	} else {
		log.Printf("📁 本次训练会话目录（含每轮 GEPA 中间 prompt）: %s", runDir)
	}

	if cfg.TrainOnClose {
		if cfg.LogASCII {
			log.Printf("Batch train start: train-on-close=true, max GEPA rounds=%d (each close_long/close_short triggers)", maxBatches)
		} else {
			log.Printf("🔧 Batch 训练启动: **每次平仓后**触发 GEPA，最多 %d 轮（忽略 -batch-size 累计）", maxBatches)
		}
	} else if useLegacyBatch {
		if cfg.LogASCII {
			log.Printf("Batch train start: batch-size=%d ops, max-GEPA-rounds=%d (open/close each=1; -max-batches is GEPA count)", batchSize, maxBatches)
		} else {
			log.Printf("🔧 Batch 训练启动: 每 %d 次开平仓操作触发 GEPA，最多 %d 轮（传统 -batch-size 模式；-max-batches=轮数）", batchSize, maxBatches)
		}
	} else {
		if cfg.LogASCII {
			log.Printf("Batch train start: GEPA trigger every %d decision steps + every %d open/close ops; max GEPA rounds=%d (not max decision steps)", cfg.TrainEverySteps, cfg.TrainExtraEveryOps, maxBatches)
		} else {
			log.Printf("🔧 Batch 训练启动: 每 **%d 决策步**一次 GEPA + 每 **%d 次开平仓**额外一次 GEPA，最多 **%d 轮 GEPA**（**-max-batches 是轮数，不是决策步上限**）",
				cfg.TrainEverySteps, cfg.TrainExtraEveryOps, maxBatches)
		}
	}

	for t := cfg.StartTime; t.Before(cfg.EndTime) || t.Equal(cfg.EndTime); t = t.Add(stepDur) {
		if batchNum >= maxBatches {
			if cfg.LogASCII {
				log.Printf("Reached max GEPA rounds %d, stopping (this is -max-batches, not decision-step count)", maxBatches)
			} else {
				log.Printf("📦 已达最大 GEPA 轮数 %d（-max-batches），停止训练（**不是**决策步上限）", maxBatches)
			}
			break
		}
		if cfg.MaxSteps > 0 && callCount >= cfg.MaxSteps {
			break
		}
		callCount++
		stepsSinceLastStepTrain++
		if stepsSinceLastStepTrain == 1 {
			trainWindowStart = t
		}

		ctx, err := BuildContextAtTime(t, cfg.Symbols, acc, callCount, cfg.MockMode, true, trainWindowStart, cfg.StepDuration())
		if err != nil {
			if cfg.LogASCII {
				log.Printf("DecStep %d: build context failed %v, skip (run -fetch first)", callCount, err)
			} else {
				log.Printf("⏭️ 步 %d: 构建上下文失败 %v，跳过（建议先执行 -fetch 预下载 K 线）", callCount, err)
			}
			continue
		}
		if consecutiveZeroDecision >= 3 {
			ctx.TrainingDeadlockNudge = fmt.Sprintf(
				"【训练系统·重申】你已连续 %d 步的 JSON 决策数组为空 `[]`。本步**禁止**再单独输出 `[]`；若无持仓可操作，须为每个候选币种至少一条 `wait`/`hold`（含 symbol、reasoning）。若市场可读且非 D 态，应优先考虑 1 份×1-3x 的 `open_long`/`open_short` 以产生可复盘样本。",
				consecutiveZeroDecision)
		}
		if cfg.LogASCII {
			log.Printf("DecStep %d: requesting AI decision (may take 1-10 min for large prompts)...", callCount)
		} else {
			log.Printf("⏳ 步 %d: 正在请求 AI 决策（prompt 较大时可能需 1-10 分钟，请耐心等待）...", callCount)
		}
		fd, err := decision.GetFullDecisionWithCustomPrompt(ctx, mcpClient, currentPrompt, true, "")
		if err != nil {
			if cfg.LogASCII {
				log.Printf("DecStep %d: AI decision failed %v, skip", callCount, err)
			} else {
				log.Printf("⏭️ 步 %d: AI 决策失败 %v，跳过", callCount, err)
			}
			continue
		}
		successfulDecisionSteps++
		if !fd.HadJSONCodeBlock {
			noJSONResponses++
		}
		if fd.UsedFallback {
			jsonFallbackCount++
		}
		if len(fd.Decisions) == 0 {
			zeroDecisionSteps++
			consecutiveZeroDecision++
		} else {
			consecutiveZeroDecision = 0
			if decisionsAllPassive(fd.Decisions) {
				allWaitSteps++
			}
		}

		// 输出 AI 决策详情，便于排查为何不开仓
		for _, d := range fd.Decisions {
			action := d.Action
			if action == "" {
				action = "wait"
			}
			reason := d.Reasoning
			if len(reason) > 100 {
				reason = reason[:100] + "..."
			}
			extra := ""
			if d.Action == "open_long" || d.Action == "open_short" {
				extra = fmt.Sprintf(" size=%.0f lev=%d", d.PositionSizeUSD, d.Leverage)
			}
			if cfg.LogASCII {
				log.Printf("   [OUT] %s %s%s | %s", d.Symbol, action, extra, reason)
			} else {
				log.Printf("   [输出] %s %s%s | %s", d.Symbol, action, extra, reason)
			}
		}
		if len(fd.Decisions) == 0 {
			cot := fd.CoTTrace
			if len(cot) > 200 {
				cot = cot[:200] + "..."
			}
			if cot != "" {
				if cfg.LogASCII {
					log.Printf("   [OUT] (no JSON) CoT: %s", cot)
				} else {
					log.Printf("   [输出] (无JSON) 思维链: %s", cot)
				}
			}
		} else {
			openCount := 0
			for _, d := range fd.Decisions {
				if d.Action == "open_long" || d.Action == "open_short" {
					openCount++
				}
			}
			if openCount == 0 && len(fd.CoTTrace) > 0 {
				cot := fd.CoTTrace
				if len(cot) > 150 {
					cot = cot[len(cot)-150:]
				}
				if cfg.LogASCII {
					log.Printf("   [OUT] (all wait) reasoning tail: %s", cot)
				} else {
					log.Printf("   [输出] (全wait) 思维链尾: %s", cot)
				}
			}
		}

		priceMap := GetPriceMapFromContext(ctx, cfg.Symbols)

		ops := make([]TradeOp, 0)
		ApplyDecisionsWithTradeLog(acc, fd.Decisions, priceMap, ctx.CurrentTime, &ops)

		// GEPA 轨迹收集：思维链 + 决策摘要
		decSummary := ""
		for _, d := range fd.Decisions {
			a := d.Action
			if a == "" {
				a = "wait"
			}
			decSummary += fmt.Sprintf("%s %s; ", d.Symbol, a)
		}
		if len(decSummary) > 200 {
			decSummary = decSummary[:200] + "..."
		}
		cot := fd.CoTTrace
		if len(cot) > 600 {
			cot = cot[:300] + "\n...[中略]...\n" + cot[len(cot)-300:]
		}
		tsStep := TrajectoryStep{
			Step:        callCount,
			Time:        ctx.CurrentTime,
			CoTTrace:    cot,
			Decisions:   strings.TrimSpace(decSummary),
			EquityAfter: acc.Equity,
			HadTrade:    len(ops) > 0,
		}
		trajectoryBuf = append(trajectoryBuf, tsStep)
		batchTrajectory = append(batchTrajectory, tsStep)
		if len(trajectoryBuf) > 20 {
			trajectoryBuf = trajectoryBuf[len(trajectoryBuf)-20:]
		}

		// 全量操作流水：每步追加，供 HTML/JSON 报告（与「何时触发 GEPA」解耦）
		allTradeOps = append(allTradeOps, ops...)

		// 训练健康度（会话级，模拟器实际成交）：检测「长期不开仓 / 长期无成交」
		hadExecOpen := false
		for _, op := range ops {
			if op.Action == "open_long" || op.Action == "open_short" {
				hadExecOpen = true
				sessionOpenCount++
			}
		}
		if hadExecOpen {
			stepsSinceLastOpen = 0
		} else {
			stepsSinceLastOpen++
		}
		if len(ops) == 0 {
			consecutiveNoTrade++
		} else {
			consecutiveNoTrade = 0
		}

		if !cfg.TrainOnClose {
			for _, op := range ops {
				tradeOps = append(tradeOps, op)
				opCount++
			}
		}
		maxStr := formatMaxStepsLabel(cfg)
		grossProfitStep := acc.Equity + acc.TotalFeesPaid - cfg.InitialEquity
		if cfg.LogASCII {
			if useLegacyBatch && !cfg.TrainOnClose {
				log.Printf("[DecStep %s/%s] decisions=%d trades_this=%d total_ops=%d/%d (GEPA %d/%d) | PnL=%.2f equity=%.0f fees=%.2f grossProfit=%.2f",
					fmt.Sprintf("%d", callCount), maxStr, len(fd.Decisions), len(ops), opCount, batchSize, batchNum, maxBatches, acc.TotalPnL, acc.Equity, acc.TotalFeesPaid, grossProfitStep)
			} else {
				log.Printf("[DecStep %s/%s] decisions=%d trades_this=%d total_ops=%d step_since=%d/%d | PnL=%.2f equity=%.0f fees=%.2f grossProfit=%.2f",
					fmt.Sprintf("%d", callCount), maxStr, len(fd.Decisions), len(ops), opCount, stepsSinceLastStepTrain, cfg.TrainEverySteps, acc.TotalPnL, acc.Equity, acc.TotalFeesPaid, grossProfitStep)
			}
		} else {
			if useLegacyBatch && !cfg.TrainOnClose {
				log.Printf("📌 步 %s/%s: 决策 %d 条 → 本步 %d 次操作，累计 %d/%d 次开平仓（GEPA %d/%d）| PnL=%.2f 权益=%.0f 手续费=%.2f 毛盈利=%.2f",
					fmt.Sprintf("%d", callCount), maxStr, len(fd.Decisions), len(ops), opCount, batchSize, batchNum, maxBatches, acc.TotalPnL, acc.Equity, acc.TotalFeesPaid, grossProfitStep)
			} else {
				log.Printf("📌 步 %s/%s: 决策 %d 条 → 本步 %d 次操作，累计开平仓 %d 次，距上次按步训练 %d/%d 步 | PnL=%.2f 权益=%.0f 手续费=%.2f 毛盈利=%.2f",
					fmt.Sprintf("%d", callCount), maxStr, len(fd.Decisions), len(ops), opCount, stepsSinceLastStepTrain, cfg.TrainEverySteps, acc.TotalPnL, acc.Equity, acc.TotalFeesPaid, grossProfitStep)
			}
		}

		type gepaTrigger struct {
			tag string
			ops []TradeOp
		}
		var triggers []gepaTrigger
		if cfg.TrainOnClose {
			if opsHasClose(ops) && batchNum < maxBatches {
				triggers = append(triggers, gepaTrigger{"train-on-close", append([]TradeOp(nil), ops...)})
			}
		} else if useLegacyBatch {
			if opCount >= batchSize && batchNum < maxBatches {
				triggers = append(triggers, gepaTrigger{"legacy-batch", append([]TradeOp(nil), tradeOps...)})
			}
		} else {
			if cfg.TrainExtraEveryOps > 0 && opCount >= cfg.TrainExtraEveryOps {
				triggers = append(triggers, gepaTrigger{"extra-ops", append([]TradeOp(nil), tradeOps...)})
			}
			if cfg.TrainEverySteps > 0 && stepsSinceLastStepTrain >= cfg.TrainEverySteps {
				if stepOpMark <= len(allTradeOps) {
					triggers = append(triggers, gepaTrigger{"every-steps", append([]TradeOp(nil), allTradeOps[stepOpMark:]...)})
				}
			}
		}

		for _, tr := range triggers {
			if batchNum >= maxBatches {
				break
			}
			opsForTrain := tr.ops
			batchNum++
			batchPnL := sumTradeOpsPnL(opsForTrain)
			switch tr.tag {
			case "train-on-close":
				if cfg.LogASCII {
					log.Printf("GEPA %d/%d (on-close): %d ops this step (PnL=%.2f), training", batchNum, maxBatches, len(opsForTrain), batchPnL)
				} else {
					log.Printf("📦 平仓触发 GEPA %d/%d：本步 %d 笔操作（样本 PnL=%.2f），进行训练", batchNum, maxBatches, len(opsForTrain), batchPnL)
				}
			case "legacy-batch":
				if cfg.LogASCII {
					log.Printf("GEPA %d/%d (legacy): %d ops cum (PnL=%.2f), training", batchNum, maxBatches, opCount, batchPnL)
				} else {
					log.Printf("📦 GEPA %d/%d（传统累计）：已累积 %d 次开平仓（本 batch PnL=%.2f），进行训练", batchNum, maxBatches, opCount, batchPnL)
				}
			case "extra-ops":
				if cfg.LogASCII {
					log.Printf("GEPA %d/%d (extra %d ops): %d ops (PnL=%.2f), training", batchNum, maxBatches, cfg.TrainExtraEveryOps, len(opsForTrain), batchPnL)
				} else {
					log.Printf("📦 GEPA %d/%d（**额外**：累计 %d 次开平仓）：%d 笔样本（PnL=%.2f），进行训练", batchNum, maxBatches, cfg.TrainExtraEveryOps, len(opsForTrain), batchPnL)
				}
			case "every-steps":
				if cfg.LogASCII {
					log.Printf("GEPA %d/%d (trigger every %d decision steps, not step limit): %d ops in window (PnL=%.2f), training", batchNum, maxBatches, cfg.TrainEverySteps, len(opsForTrain), batchPnL)
				} else {
					log.Printf("📦 GEPA %d/%d（**按步数** %d 步）：窗口内 %d 笔开平仓（可无成交；PnL=%.2f），进行训练", batchNum, maxBatches, cfg.TrainEverySteps, len(opsForTrain), batchPnL)
				}
			}

			totalSteps := cfg.MaxSteps
			if totalSteps <= 0 {
				totalSteps = callCount
			}
			gepaWinTotal := len(batchTrajectory)
			gepaWinNoTrade := 0
			for _, ts := range batchTrajectory {
				if !ts.HadTrade {
					gepaWinNoTrade++
				}
			}
			overview := SessionOverview{
				CurrentStep:             callCount,
				TotalSteps:              totalSteps,
				CumulativePnL:           acc.TotalPnL,
				InitialEquity:           cfg.InitialEquity,
				CurrentEquity:           acc.Equity,
				TotalFeesPaid:           acc.TotalFeesPaid,
				ClosedTradePnLs:         append([]float64{}, acc.ClosedTradePnLs...),
				BatchNum:                batchNum,
				StepsSinceLastOpen:      stepsSinceLastOpen,
				ConsecutiveNoTradeSteps: consecutiveNoTrade,
				OpensInSession:          sessionOpenCount,
				GEPAWindowTotalSteps:    gepaWinTotal,
				GEPAWindowNoTradeSteps:  gepaWinNoTrade,
			}
			adaptiveHint := adaptiveRegimeHint(overview.ClosedTradePnLs, overview.CumulativePnL, cfg.GEPAPromptExplore())
			trajStart := 0
			if len(trajectoryBuf) > 8 {
				trajStart = len(trajectoryBuf) - 8
			}
			trajectory := trajectoryBuf[trajStart:]
			missedSteps := filterMissedSteps(batchTrajectory)
			windowBenchBase := ""
			if !trainWindowStart.IsZero() {
				windowBenchBase = BuildTrainingBenchmarkHints(trainWindowStart, t, cfg.Symbols, cfg.StepDuration())
			}
			hindsightMech := ""
			if h := BuildTrainingHindsightOptimalHint(cfg.StartTime, trainWindowStart, t, cfg.Symbols); h != "" {
				hindsightMech = h
			}
			useMutation := (batchNum%3) == 0 && batchNum > 0
			var nextPrompt string
			var trainErr error
			if useMutation {
				if cfg.LogASCII {
					log.Printf("   [PromptBreeder] mutation round %d (calling LLM, may take 2-10 min)...", batchNum)
				} else {
					log.Printf("   🧬 PromptBreeder 变异轮 %d（调用 LLM 优化，可能需 2-10 分钟）...", batchNum)
				}
				nextPrompt, trainErr = doMutationStep(currentPrompt, overview, cfg, mcpClient)
			}
			if !useMutation || trainErr != nil || nextPrompt == "" {
				if !useMutation {
					if cfg.LogASCII {
						log.Printf("   [GEPA] round %d/%d LLM optimize (may take 2-10 min)...", batchNum, maxBatches)
					} else {
						log.Printf("   ⏳ GEPA 优化中 batch %d（调用 LLM，可能需 2-10 分钟）...", batchNum)
					}
				}
				windowBench := windowBenchBase
				if hindsightMech != "" {
					windowBench += enrichHindsightBlockForGEPA(hindsightMech, cfg, mcpClient, cfg.LogASCII, batchNum)
				}
				np, te := doBatchTrainStep(currentPrompt, opsForTrain, overview, trajectory, missedSteps, adaptiveHint, windowBench, cfg, mcpClient)
				if nextPrompt == "" {
					nextPrompt, trainErr = np, te
				}
			}
			if trainErr != nil {
				if cfg.LogASCII {
					log.Printf("   Train failed: %v, keep current prompt", trainErr)
				} else {
					log.Printf("   ⚠️ 训练失败: %v，保留当前 prompt", trainErr)
				}
			} else if nextPrompt != "" {
				currentPrompt = nextPrompt
				if cfg.LogASCII {
					log.Printf("   Prompt updated, len=%d", len(currentPrompt))
				} else {
					log.Printf("   ✓ Prompt 已更新，长度=%d", len(currentPrompt))
				}

				fname := filepath.Join(outDir, fmt.Sprintf("%s_batch%d_final_%s.txt", cfg.TrainBase, batchNum, time.Now().Format("150405")))
				if err := os.WriteFile(fname, []byte(currentPrompt), 0644); err == nil {
					if cfg.LogASCII {
						log.Printf("   Saved: %s", fname)
					} else {
						log.Printf("   已保存: %s", fname)
					}
				}
			}

			skillOps := make([]decision.TradeOpForSkill, 0, len(opsForTrain))
			for _, op := range opsForTrain {
				skillOps = append(skillOps, decision.TradeOpForSkill{
					Action: op.Action, Symbol: op.Symbol, PnL: op.PnL, Reasoning: op.Reasoning,
				})
			}
			if n := decision.ExtractSkillsFromTradeOps(skillOps, "training", mcpClient); n > 0 && !cfg.LogASCII {
				log.Printf("   📥 从本 batch 抽取 %d 条技能，已加入 SkillBank", n)
			}

			if !cfg.TrainOnClose {
				switch tr.tag {
				case "extra-ops", "legacy-batch":
					tradeOps = tradeOps[:0]
					opCount = 0
				case "every-steps":
					stepOpMark = len(allTradeOps)
				}
			}
			stepsSinceLastStepTrain = 0
			batchTrajectory = batchTrajectory[:0]
			time.Sleep(2 * time.Second)
			if batchNum >= maxBatches {
				break
			}
		}
	}

	// 保存训练会话摘要（交易详情、决策理由、胜率等）——与会话目录一致，便于回溯
	sessionPath := filepath.Join(outDir, fmt.Sprintf("training_session_%s.json", runID))
	var sessionSummary *TrainingSessionSummary
	var htmlPath string
	health := TrainingDecisionHealth{
		DecisionSteps:     callCount,
		SuccessfulSteps:   successfulDecisionSteps,
		NoJSONResponses:   noJSONResponses,
		JSONFallbackCount: jsonFallbackCount,
		ZeroDecisionSteps: zeroDecisionSteps,
		AllWaitSteps:      allWaitSteps,
	}
	if sum, err := writeTrainingSessionSummary(sessionPath, allTradeOps, acc, cfg.InitialEquity, cfg, health); err == nil {
		sessionSummary = sum
		if !cfg.LogASCII {
			log.Printf("📋 训练会话摘要已保存: %s (return=%.2f%% sharpe=%.3f maxDD=%.2f%% composite=%.4f)",
				sessionPath, sum.TotalReturnPct, sum.SharpeRatio, sum.MaxDrawdownPct, sum.CompositeScore)
		} else {
			log.Printf("Session JSON: %s return=%.2f%% sharpe=%.3f composite=%.4f", sessionPath, sum.TotalReturnPct, sum.SharpeRatio, sum.CompositeScore)
		}
		htmlPath = strings.TrimSuffix(sessionPath, ".json") + ".html"
		if err := writeTrainingReportHTML(htmlPath, cfg, allTradeOps, acc); err != nil {
			log.Printf("⚠️ 训练 HTML 报告生成失败: %v", err)
		} else if !cfg.LogASCII {
			log.Printf("📊 训练 HTML 报告: %s", htmlPath)
		} else {
			log.Printf("HTML report: %s", htmlPath)
		}
	} else {
		log.Printf("⚠️ 训练会话摘要写入失败: %v", err)
	}
	printTrainingSummary(allTradeOps, acc, cfg.InitialEquity, cfg.LogASCII, health)

	// 保存最终 prompt
	finalPath := filepath.Join(outDir, fmt.Sprintf("%s_batch_final_%s.txt", cfg.TrainBase, runID))
	if err := os.WriteFile(finalPath, []byte(currentPrompt), 0644); err != nil {
		if cfg.LogASCII {
			log.Printf("Save final prompt failed: %v", err)
		} else {
			log.Printf("⚠️ 保存最终 prompt 失败: %v", err)
		}
	} else {
		if cfg.LogASCII {
			log.Printf("Train done, final prompt saved: %s", finalPath)
		} else {
			log.Printf("✅ 训练完成，最终 prompt 已保存: %s", finalPath)
		}
	}
	if sessionSummary != nil {
		appendTrainingHistoryAndMaybePromote(cfg, sessionPath, htmlPath, finalPath, sessionSummary, currentPrompt, nil, runID)
	}
	// 毛盈利 = 权益 + 手续费 - 初始，表示趋势把握能力（若未扣费的总收益）
	// 毛盈利>0 说明策略方向正确；手续费会吃掉部分利润
	grossProfit := acc.Equity + acc.TotalFeesPaid - cfg.InitialEquity
	if cfg.LogASCII {
		log.Printf("Summary: PnL=%.2f equity=%.0f fees=%.2f grossProfit=%.2f",
			acc.TotalPnL, acc.Equity, acc.TotalFeesPaid, grossProfit)
	} else {
		log.Printf("📊 训练统计: 累计 PnL=%.2f 权益=%.0f 手续费=%.2f 毛盈利=%.2f (毛盈利>0=策略方向正确，手续费吃掉部分利润)",
			acc.TotalPnL, acc.Equity, acc.TotalFeesPaid, grossProfit)
	}

	return currentPrompt, nil
}

// SessionOverview 训练会话概览，供优化器参考
type SessionOverview struct {
	CurrentStep     int
	TotalSteps      int
	CumulativePnL   float64
	InitialEquity   float64
	CurrentEquity   float64
	TotalFeesPaid   float64   // 累计已支付手续费，权益+手续费=真实盈利（趋势把握能力）
	ClosedTradePnLs []float64 // 所有已平仓交易的盈亏
	BatchNum        int
	// 训练健康度：用于检测「越训越不敢开」并注入僵死解锁
	StepsSinceLastOpen      int // 距上一次成功执行的开仓已过多少决策步
	ConsecutiveNoTradeSteps int // 连续多少决策步模拟器侧无任何开平成交
	OpensInSession          int // 本会话累计开仓次数（执行成功）
	GEPAWindowTotalSteps    int // 本 GEPA 窗口内决策步数（自上次 GEPA 起）
	GEPAWindowNoTradeSteps  int // 上述步中无任何成交的步数
}

func computeWinRate(pnls []float64) (float64, int, int) {
	if len(pnls) == 0 {
		return 0, 0, 0
	}
	wins, losses := 0, 0
	for _, p := range pnls {
		if p > 0 {
			wins++
		} else if p < 0 {
			losses++
		}
	}
	total := wins + losses
	if total == 0 {
		return 0, 0, 0
	}
	return float64(wins) / float64(total) * 100, wins, losses
}

// doMutationStep PromptBreeder 变异：基于当前 prompt 生成变体，尝试不同策略方向
func doMutationStep(currentPrompt string, overview SessionOverview, cfg *Config, mcpClient *mcp.Client) (string, error) {
	winRate, wins, losses := computeWinRate(overview.ClosedTradePnLs)
	mutationPrompt := fmt.Sprintf(`你是 prompt 变异专家（PromptBreeder 风格）。根据当前表现生成策略变体以增加进化多样性。

**核心认知**：需**同时总结盈利与亏损原因**；亏损归因到**具体场景下的判断错误**，而非用一刀切禁令让全市场陪葬。**优先「建议 + 典型好处 / 典型风险」**；**必须**包含**分场景（A/B/C/D 或趋势/震荡等）**分支，**禁止**单一全局入场条件套死所有状态。**禁止**压频次类风控（交易间隔、每日笔数上限）。

会话概览：步数 %d/%d，累计 PnL=%.2f USDT，已平仓 %d 笔（盈利 %d/亏损 %d，胜率 %.1f%%）。
训练健康度：距上次开仓 %d 步；连续 %d 步无成交；本会话开仓 %d 次；本 GEPA 窗口 %d 步中 %d 步无成交。

%s

请基于当前系统提示词，生成一个**策略变体**，可从以下方向中选一个或组合调整（不限于此）：
1. **分场景**细化第二项动能认定（趋势 vs 震荡不同）
2. 方向与周期：多周期共振、拐点、BTC 过滤
3. 持仓与执行周期匹配
4. 在**已满足该场景下至少两项**时提高果断度；用**低杠杆试错**表达积极开仓（非乱开仓）

**硬约束（禁止违反）**：
- **禁止**将任一场景降为「单项即可」；每场景仍须至少两项+该场景认可的第二证据
- **禁止**删除分场景「满足即开、不满足即等」逻辑
- **禁止**删除止损、最小持仓时间、方向一致性
- **禁止**「无依据乱开仓」「不分析即满仓」；**允许**分场景下「低杠杆积极试错」
- 同向再开：**新结构或更强第二项**，**勿**用固定步数死卡

**长度（先删后加）**：当前长度 %d 字符。输出可为原 prompt 的 **%.2f～%.2f 倍**。**先审视删除**重复表述、旧规则、冗长解释，**再**考虑新增——每轮净增长不超过 10%%。

请直接输出变异后的完整系统提示词，保持格式不变。`,
		overview.CurrentStep, overview.TotalSteps, overview.CumulativePnL,
		len(overview.ClosedTradePnLs), wins, losses, winRate,
		overview.StepsSinceLastOpen, overview.ConsecutiveNoTradeSteps, overview.OpensInSession,
		overview.GEPAWindowTotalSteps, overview.GEPAWindowNoTradeSteps,
		strings.TrimSpace(buildTrainingDeadlockUnlockBlock(overview)),
		len(currentPrompt), cfg.TrainGEPAExploreLenMin, cfg.TrainGEPAExploreLenMax)
	mutationPrompt += fmt.Sprintf("\n- 累计手续费: %.2f USDT，真实盈利（权益+手续费-初始）= %.2f USDT（代表趋势把握能力）", overview.TotalFeesPaid, overview.CurrentEquity+overview.TotalFeesPaid-overview.InitialEquity)
	aiResp, _, _, err := mcpClient.CallWithMessagesMaxTokens(mutationPrompt+"\n\n当前系统提示词：\n"+currentPrompt,
		"请直接输出变异后的完整系统提示词，不要解释。", 20000)
	if err != nil {
		return "", err
	}
	return extractPromptFromResponse(aiResp), nil
}

// filterMissedSteps 从自上次 GEPA 以来的轨迹中筛出「无成交」步，供反思错失机会（条数上限防止 prompt 爆炸）
func filterMissedSteps(batch []TrajectoryStep) []TrajectoryStep {
	var out []TrajectoryStep
	for _, ts := range batch {
		if !ts.HadTrade {
			out = append(out, ts)
		}
	}
	const maxMissed = 12
	if len(out) > maxMissed {
		out = out[len(out)-maxMissed:]
	}
	return out
}

// buildMissedOpportunityBlock 将无成交步格式化为 GEPA 专用段落（关键机会未开/未平、仅观望、no JSON 等）
func buildMissedOpportunityBlock(missed []TrajectoryStep, sessionDeadlock bool) string {
	if len(missed) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n## 本 batch 内【无成交】步（错失机会候选，须反思）\n")
	sb.WriteString("以下时间步**未产生任何开/平仓**（模拟器层面无成交）。请对照「决策摘要」与「思维链」判断：\n")
	sb.WriteString("- 是否**该开未开**（信号已满足**该场景下**至少两项但因表述歧义、JSON 未输出、**全局过严**而 wait）\n")
	sb.WriteString("- 是否**该平未平**（持仓已恶化或趋势反转，但模型未输出平仓 JSON）\n")
	if sessionDeadlock {
		sb.WriteString("- **健康度已差**：优先 **分场景** 放宽「第二项」认定、强化 JSON 与可执行性；**禁止**再叠全局禁令。**仍禁止**把任一场景降为「单项即可」。\n\n")
	} else {
		sb.WriteString("- 改进方向：可执行性、JSON 铁律、**建议+利弊**；**避免**再堆硬禁令致长期不交易；**禁止**把「补错失」搞成删除全部门槛（应**分场景**细化第二项，而非降为单项）。\n\n")
	}
	for _, ts := range missed {
		cot := strings.TrimSpace(strings.ReplaceAll(ts.CoTTrace, "\n", " "))
		if len(cot) > 380 {
			cot = cot[:380] + "..."
		}
		sb.WriteString(fmt.Sprintf("### 步 %d [%s] 无成交\n- 决策摘要: %s\n- 思维链摘要: %s\n\n",
			ts.Step, ts.Time, ts.Decisions, cot))
	}
	return sb.String()
}

// doBatchTrainStep 基于本 batch 的操作记录、GEPA 轨迹、无成交步（错失候选）、Adaptive 倾向，调用 LLM 改进 prompt
// ops 可为空：用于「每 N 步」强制优化，仅依据轨迹与无成交步改进 prompt
func doBatchTrainStep(currentPrompt string, ops []TradeOp, overview SessionOverview, trajectory []TrajectoryStep, missedInBatch []TrajectoryStep, adaptiveHint string, windowKLineBenchmark string, cfg *Config, mcpClient *mcp.Client) (string, error) {
	explore := cfg.GEPAPromptExplore()
	parts := ""
	if len(ops) == 0 {
		parts = "（本段无实际开平仓记录；请依据会话概览、健康度、GEPA 轨迹与「无成交」专节：**分场景**恢复可执行开仓路径，强化 JSON；**优先**「建议+风险」；**禁止**再增加全局一律 wait；**禁止**把任一场景降为单项即可。）\n\n"
	}
	for i, op := range ops {
		line := fmt.Sprintf("%d. [%s] %s %s @ %.2f", i+1, op.Time, op.Action, op.Symbol, op.Price)
		if op.PnL != 0 {
			line += fmt.Sprintf(" PnL=%.2f", op.PnL)
		}
		if op.SizeUSD > 0 {
			line += fmt.Sprintf(" size=%.0fUSD x%d", op.SizeUSD, op.Leverage)
		}
		if op.Reasoning != "" {
			reason := op.Reasoning
			maxR := gepaReasonOpenMaxBytes
			if op.PnL != 0 {
				maxR = gepaReasonCloseMaxBytes
			}
			line += fmt.Sprintf(" | 理由: %s", truncateUTF8(strings.TrimSpace(reason), maxR))
		}
		parts += line + "\n"
	}

	winRate, wins, losses := computeWinRate(overview.ClosedTradePnLs)
	profitBeforeFees := overview.CurrentEquity + overview.TotalFeesPaid
	// 本 batch 同币种开仓统计（便于 GEPA 发现过度集中）
	symbolOpenCount := make(map[string]int)
	for _, op := range ops {
		if op.Action == "open_long" || op.Action == "open_short" {
			symbolOpenCount[op.Symbol]++
		}
	}
	symbolOpenStr := ""
	for sym, cnt := range symbolOpenCount {
		if cnt > 2 {
			symbolOpenStr += fmt.Sprintf(" %s(%d次⚠️)", sym, cnt)
		} else {
			symbolOpenStr += fmt.Sprintf(" %s(%d)", sym, cnt)
		}
	}
	if symbolOpenStr == "" {
		symbolOpenStr = " 无"
	}
	overviewText := fmt.Sprintf(`## 本次训练会话概览（截至第 %d 步）
- 已执行步数: %d / %d
- 累计 PnL: %.2f USDT（初始 %.0f → 当前权益 %.2f）
- 累计手续费: %.2f USDT
- 真实盈利（不含手续费）: %.2f USDT（权益+手续费-初始，代表趋势把握能力）
- 已平仓笔数: %d（盈利 %d 笔，亏损 %d 笔，胜率 %.1f%%）
- 本 batch 操作数: %d
- 本 batch 同币种开仓统计:%s（同币种>2次为过度集中，应强化加仓门槛）`,
		overview.CurrentStep, overview.CurrentStep, overview.TotalSteps,
		overview.CumulativePnL, overview.InitialEquity, overview.CurrentEquity,
		overview.TotalFeesPaid, profitBeforeFees-overview.InitialEquity,
		len(overview.ClosedTradePnLs), wins, losses, winRate, len(ops), symbolOpenStr)
	// 附加近期平仓盈亏明细（最近 15 笔），便于优化器分析盈亏模式
	if len(overview.ClosedTradePnLs) > 0 {
		n := len(overview.ClosedTradePnLs)
		if n > 15 {
			n = 15
		}
		recent := overview.ClosedTradePnLs[len(overview.ClosedTradePnLs)-n:]
		detail := ""
		for i, p := range recent {
			detail += fmt.Sprintf("  %d. %.2f USDT\n", i+1, p)
		}
		overviewText += fmt.Sprintf("\n- 近期平仓盈亏（最近 %d 笔）:\n%s", len(recent), detail)
	}
	if adaptiveHint != "" {
		overviewText += adaptiveHint
	}
	overviewText += fmt.Sprintf(`
- **训练健康度**：距上次成功开仓已 **%d** 决策步；连续 **%d** 步无模拟器成交；本会话累计开仓 **%d** 次；本 GEPA 窗口共 **%d** 步，其中 **%d** 步无成交。
`, overview.StepsSinceLastOpen, overview.ConsecutiveNoTradeSteps, overview.OpensInSession,
		overview.GEPAWindowTotalSteps, overview.GEPAWindowNoTradeSteps)

	deadlockUnlock := buildTrainingDeadlockUnlockBlock(overview)
	missedBlock := buildMissedOpportunityBlock(missedInBatch, trainingDeadlockLikely(overview))

	// GEPA 反思链：最近几步的思维链+决策轨迹，供诊断
	trajectoryBlock := ""
	if len(trajectory) > 0 {
		trajectoryBlock = "\n## GEPA 反思链：最近决策轨迹（思维链+决策）\n"
		for _, ts := range trajectory {
			cot := strings.TrimSpace(strings.ReplaceAll(ts.CoTTrace, "\n", " "))
			if len(cot) > 400 {
				cot = cot[:400] + "..."
			} else if cot != "" {
				cot = cot + "..."
			}
			trajectoryBlock += fmt.Sprintf("\n### 步 %d [%s]\n- 决策: %s\n- 思维链摘要: %s\n",
				ts.Step, ts.Time, ts.Decisions, cot)
		}
		trajectoryBlock += trajectoryDiagnosisFooter(explore)
	}
	if missedBlock != "" && len(trajectory) > 0 {
		trajectoryBlock += "\n（另见下方「无成交」专节，用于错失机会反思。）\n"
	}

	// 典型盈利/亏损案例摘要（APE 式示例，便于优化器学习）
	exampleBlock := ""
	var profitReasons, lossReasons []string
	for _, op := range ops {
		if op.Reasoning == "" || op.Reasoning == "(止盈/止损触发)" {
			continue
		}
		r := truncateUTF8(strings.TrimSpace(op.Reasoning), gepaExampleReasonBytes)
		if op.PnL > 0 {
			profitReasons = append(profitReasons, fmt.Sprintf("%s %s: %s", op.Action, op.Symbol, r))
		} else if op.PnL < 0 {
			lossReasons = append(lossReasons, fmt.Sprintf("%s %s PnL=%.1f: %s", op.Action, op.Symbol, op.PnL, r))
		}
	}
	const maxAPEExamples = 6
	if len(profitReasons) > 0 || len(lossReasons) > 0 {
		exampleBlock = "\n## 典型案例（平仓理由为主；须与「平仓回合」配对合读）\n"
		if len(profitReasons) > 0 {
			n := len(profitReasons)
			if n > maxAPEExamples {
				n = maxAPEExamples
			}
			exampleBlock += "- **盈利案例**：总结成功原因（方向/持仓时间/周期匹配等），强化类似决策\n"
			for i := len(profitReasons) - n; i < len(profitReasons); i++ {
				if i >= 0 {
					exampleBlock += "  - " + profitReasons[i] + "\n"
				}
			}
		}
		if len(lossReasons) > 0 {
			n := len(lossReasons)
			if n > maxAPEExamples {
				n = maxAPEExamples
			}
			exampleBlock += "- **亏损案例（优先逐条归因后再改 prompt）**：错在时机/追单/周期/延伸段等哪一类，应 wait 或加何确认\n"
			for i := len(lossReasons) - n; i < len(lossReasons); i++ {
				if i >= 0 {
					exampleBlock += "  - " + lossReasons[i] + "\n"
				}
			}
		}
	}

	windowBlock := ""
	if strings.TrimSpace(windowKLineBenchmark) != "" {
		windowBlock = "## 本窗口 K 线因果方向参照（简化、防过拟合）\n" + strings.TrimSpace(windowKLineBenchmark) + "\n\n"
	}

	pairedRoundsBlock := buildPairedCloseRoundsBlock(ops)

	attributionHardReq := `## 训练归因硬要求（内化进改进版全文，勿单独附件输出）
1. **先读「平仓回合（开仓→平仓配对）」**：对其中的 **PnL<0** 的每一回合，在脑中完成 **① 开仓当时属于哪类错误（追单、顺势末段追涨杀跌、周期错配、假突破、止盈/大幅减仓后立刻同向再开、布林极端外沿追涨等）② 更合理的是 wait 还是增加何种确认再开**。
2. 改进后的 system prompt **必须**用可迁移的「建议 + 忽视时的典型风险」写入对上述失败类型的修补；**禁止**只对具体价位/具体币种写永久硬门槛；**禁止**对亏损回合在改稿中完全无对应强化。
3. 仍须遵守文末硬约束（**分场景下**至少两项、JSON 铁律等）；探索模式仍防小样本过拟合，但「防过拟合」不等于「亏损回合可以不回应」。

`

	// 传完整 prompt 给优化器，加入会话概览、配对回合、GEPA 轨迹、无成交步、开平仓流水、案例
	metaSuffix := buildGEPABatchMetaSuffix(cfg, len(currentPrompt), overview)
	metaPrompt := fmt.Sprintf(`你是 prompt 优化专家。根据以下**训练会话概览**、**平仓回合配对**、**GEPA 决策轨迹**、**无成交步（错失机会候选）**和**实际开平仓操作记录**，改进系统提示词以提升盈利表现。

**重要**：改进版须**以「建议 / 倾向」为主**，配 **好处与风险** 对照；**避免**堆砌强禁令与小样本过拟合（如把单笔亏损写死成永久硬规则），以免训练期长期 wait、零开仓。

%s
%s
%s
%s
%s
%s
%s
## 本 batch 操作记录（含 AI 决策理由，与上节配对合读）：
%s
%s
## 当前系统提示词（完整内容）：
%s

%s`, attributionHardReq, overviewText, deadlockUnlock, pairedRoundsBlock, trajectoryBlock, missedBlock, windowBlock, parts, exampleBlock, currentPrompt, metaSuffix)

	aiResp, _, _, err := mcpClient.CallWithMessagesMaxTokens(metaPrompt, "请直接输出改进后的完整系统提示词，不要包含任何解释或前缀。", 20000)
	if err != nil {
		return "", err
	}

	next := extractPromptFromResponse(aiResp)
	return next, nil
}

func sumTradeOpsPnL(ops []TradeOp) float64 {
	sum := 0.0
	for _, op := range ops {
		sum += op.PnL
	}
	return sum
}

// TrainingSessionSummary 训练会话摘要（供 JSON 持久化与后续分析）
type TrainingSessionSummary struct {
	EndTime         string                 `json:"end_time"`
	InitialEquity   float64                `json:"initial_equity"`
	FinalEquity     float64                `json:"final_equity"`
	TotalPnL        float64                `json:"total_pnl"`
	TotalFeesPaid   float64                `json:"total_fees_paid"`
	RealProfit      float64                `json:"real_profit"` // 毛盈利 = equity + fees - initial，表示趋势把握能力；>0 说明策略方向正确
	TotalReturnPct  float64                `json:"total_return_pct"`
	SharpeRatio     float64                `json:"sharpe_ratio"`
	MaxDrawdownPct  float64                `json:"max_drawdown_pct"`
	CompositeScore  float64                `json:"composite_score"` // 用于基座晋升比较
	ClosedTradePnLs []float64              `json:"closed_trade_pnls"`
	WinRate         float64                `json:"win_rate"`
	Wins            int                    `json:"wins"`
	Losses          int                    `json:"losses"`
	TradeOps        []TradeOp              `json:"trade_ops"`
	DecisionHealth  TrainingDecisionHealth `json:"decision_health"`
}

type TrainingDecisionHealth struct {
	DecisionSteps     int `json:"decision_steps"`
	SuccessfulSteps   int `json:"successful_steps"`
	NoJSONResponses   int `json:"no_json_responses"`
	JSONFallbackCount int `json:"json_fallback_count"`
	ZeroDecisionSteps int `json:"zero_decision_steps"`
	AllWaitSteps      int `json:"all_wait_steps"`
}

func writeTrainingSessionSummary(path string, ops []TradeOp, acc *SimAccount, initialEquity float64, cfg *Config, health TrainingDecisionHealth) (*TrainingSessionSummary, error) {
	winRate, wins, losses := computeWinRate(acc.ClosedTradePnLs)
	realProfit := acc.Equity + acc.TotalFeesPaid - initialEquity
	curve := equityCurveFromOps(initialEquity, ops, cfg.BTCETHLeverage, cfg.AltcoinLeverage)
	totalReturn, sharpe, _, maxDD := ComputeMetrics(curve, initialEquity, acc.ClosedTradePnLs)
	composite := compositeTrainingScore(totalReturn, sharpe, maxDD)
	s := &TrainingSessionSummary{
		EndTime:         time.Now().Format("2006-01-02 15:04:05"),
		InitialEquity:   initialEquity,
		FinalEquity:     acc.Equity,
		TotalPnL:        acc.TotalPnL,
		TotalFeesPaid:   acc.TotalFeesPaid,
		RealProfit:      realProfit,
		TotalReturnPct:  totalReturn,
		SharpeRatio:     sharpe,
		MaxDrawdownPct:  maxDD,
		CompositeScore:  composite,
		ClosedTradePnLs: append([]float64{}, acc.ClosedTradePnLs...),
		WinRate:         winRate,
		Wins:            wins,
		Losses:          losses,
		TradeOps:        ops,
		DecisionHealth:  health,
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return s, os.WriteFile(path, b, 0644)
}

func decisionsAllPassive(decisions []decision.Decision) bool {
	if len(decisions) == 0 {
		return false
	}
	for _, d := range decisions {
		switch d.Action {
		case "", "wait", "hold":
			continue
		default:
			return false
		}
	}
	return true
}

func printTrainingSummary(ops []TradeOp, acc *SimAccount, initialEquity float64, logASCII bool, health TrainingDecisionHealth) {
	winRate, wins, losses := computeWinRate(acc.ClosedTradePnLs)
	realProfit := acc.Equity + acc.TotalFeesPaid - initialEquity
	if logASCII {
		log.Printf("--- Training Summary ---")
		log.Printf("Equity: %.0f -> %.0f | PnL: %.2f | Fees: %.2f | GrossProfit: %.2f", initialEquity, acc.Equity, acc.TotalPnL, acc.TotalFeesPaid, realProfit)
		log.Printf("Closed trades: %d (wins=%d losses=%d winRate=%.1f%%)", len(acc.ClosedTradePnLs), wins, losses, winRate)
		log.Printf("Decision health: steps=%d success=%d no_json=%d fallback=%d zero_decision=%d all_wait=%d",
			health.DecisionSteps, health.SuccessfulSteps, health.NoJSONResponses, health.JSONFallbackCount, health.ZeroDecisionSteps, health.AllWaitSteps)
	} else {
		log.Printf("")
		log.Printf("═══════════════════════════════════════════════════════════")
		log.Printf("📊 训练会话摘要")
		log.Printf("═══════════════════════════════════════════════════════════")
		log.Printf("  权益: %.0f → %.0f USDT | 累计 PnL: %.2f | 手续费: %.2f | 毛盈利: %.2f (毛盈利>0=策略方向正确)", initialEquity, acc.Equity, acc.TotalPnL, acc.TotalFeesPaid, realProfit)
		log.Printf("  已平仓: %d 笔 (盈利 %d / 亏损 %d, 胜率 %.1f%%)", len(acc.ClosedTradePnLs), wins, losses, winRate)
		log.Printf("  决策健康: 请求 %d 步 / 成功 %d 步 / 原始无JSON %d 步 / 补调 %d 步 / 最终空决策 %d 步 / 全wait %d 步",
			health.DecisionSteps, health.SuccessfulSteps, health.NoJSONResponses, health.JSONFallbackCount, health.ZeroDecisionSteps, health.AllWaitSteps)
		log.Printf("───────────────────────────────────────────────────────────")
		log.Printf("  交易明细 (共 %d 次操作):", len(ops))
	}
	for i, op := range ops {
		reason := op.Reasoning
		if len(reason) > 60 {
			reason = reason[:60] + "..."
		}
		if op.Action == "open_long" || op.Action == "open_short" {
			if logASCII {
				log.Printf("  %d. %s %s @ %.4f size=%.0f lev=%d | %s", i+1, op.Time, op.Action, op.Price, op.SizeUSD, op.Leverage, reason)
			} else {
				log.Printf("  %2d. %s %s %s @ %.4f 规模=%.0f 杠杆=%dx | %s", i+1, op.Time, op.Symbol, op.Action, op.Price, op.SizeUSD, op.Leverage, reason)
			}
		} else {
			pnlStr := ""
			if op.PnL != 0 {
				pnlStr = fmt.Sprintf(" PnL=%.2f", op.PnL)
			}
			if logASCII {
				log.Printf("  %d. %s %s %s @ %.4f%s | %s", i+1, op.Time, op.Symbol, op.Action, op.Price, pnlStr, reason)
			} else {
				log.Printf("  %2d. %s %s %s @ %.4f%s | %s", i+1, op.Time, op.Symbol, op.Action, op.Price, pnlStr, reason)
			}
		}
	}
	if !logASCII && len(ops) > 0 {
		log.Printf("═══════════════════════════════════════════════════════════")
	}
	if health.NoJSONResponses > 0 || health.JSONFallbackCount > 0 {
		log.Printf("⚠️ 决策协议异常：本次训练出现原始无 JSON 或 fallback 补救，禁止将此类结果当作稳定基座。")
	}
}
