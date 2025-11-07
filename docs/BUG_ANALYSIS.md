# 历史成交显示±0%问题分析报告

## 📋 Docker启动配置

### 启动命令
```bash
docker-compose up -d
```

### 服务配置

#### 后端服务 (nofx-trading)
- **容器名**: `nofx-trading`
- **端口映射**: `8080:8080`
- **挂载卷**:
  - `./config.json:/app/config.json:ro` (只读)
  - `./config.db:/app/config.db` (可读写)
  - `./decision_logs:/app/decision_logs` (可读写)
  - `./prompts:/app/prompts` (可读写)
  - `/etc/localtime:/etc/localtime:ro` (时区同步)

#### 前端服务 (nofx-frontend)
- **容器名**: `nofx-frontend`
- **端口映射**: `3000:80`
- **依赖**: 后端服务

---

## 🐛 问题描述

前端交易面板中显示的历史成交记录，所有的盈亏百分比都是 **±0%**，但其他数据（如USDT盈亏）正常。

---

## 🔍 问题定位

### 1. 前端显示代码

**文件**: `web/src/components/AILearning.tsx`

**第555行**显示盈亏百分比：
```typescript
{trade.pn_l_pct.toFixed(2)}%
```

数据来源：`performance.recent_trades` 数组中的 `TradeOutcome` 对象。

### 2. 后端API接口

**文件**: `api/server.go`

**第1107-1132行** - `handlePerformance` 函数：
```go
func (s *Server) handlePerformance(c *gin.Context) {
    // ...
    performance, err := trader.GetDecisionLogger().AnalyzePerformance(100)
    // ...
    c.JSON(http.StatusOK, performance)
}
```

### 3. 核心问题代码

**文件**: `logger/decision_logger.go`

**第422-428行** - 盈亏百分比计算：
```go
// 计算盈亏百分比（相对保证金）
positionValue := quantity * openPrice
marginUsed := positionValue / float64(leverage)
pnlPct := 0.0
if marginUsed > 0 {
    pnlPct = (pnl / marginUsed) * 100
}
```

**第415-420行** - 盈亏（USDT）计算：
```go
var pnl float64
if side == "long" {
    pnl = quantity * (action.Price - openPrice)
} else {
    pnl = quantity * (openPrice - action.Price)
}
```

### 4. 问题根源

**关键发现**：

1. **平仓价格记录时机问题**：
   - **文件**: `trader/auto_trader.go`
   - **第731行** (`executeCloseLongWithRecord`) 和 **第757行** (`executeCloseShortWithRecord`)：
   ```go
   // 获取当前价格
   marketData, err := market.Get(decision.Symbol)
   if err != nil {
       return err
   }
   actionRecord.Price = marketData.CurrentPrice
   ```
   
   **问题**：这里使用的是**当前市场价格**，而不是**实际平仓执行价格**。

2. **开仓价格记录**：
   - **第627行** (`executeOpenLongWithRecord`) 和 **第686行** (`executeOpenShortWithRecord`)：
   ```go
   actionRecord.Price = marketData.CurrentPrice
   ```
   
   同样使用的是市场价格，不是实际执行价格。

3. **盈亏计算问题**：
   - 如果开仓价格和平仓价格都使用市场价格，且价格相同（或非常接近），则：
   - `pnl = quantity * (action.Price - openPrice)` 会接近0
   - 导致 `pnlPct = (pnl / marginUsed) * 100` 也接近0

### 5. 更深入的问题

**实际执行价格 vs 市场价格**：

- 平仓时，`CloseLong/CloseShort` 返回的订单信息中可能包含实际执行价格
- 但代码中没有使用这个实际执行价格，而是使用了获取订单前的市场价格
- 如果市场价格和实际执行价格有差异，会导致盈亏计算不准确

**数量问题**：

- 平仓时使用的是 `quantity = 0`（全部平仓）
- 但 `AnalyzePerformance` 中使用的 `quantity` 来自开仓记录
- 如果开仓时记录的 `quantity` 为0或未正确记录，会导致计算错误

### 6. 可能的原因

**最可能的原因**：

1. **价格获取时机问题**：
   - 平仓时，先获取市场价格，再执行平仓
   - 如果市场价格和实际执行价格相同，且开仓价格也相同，则 `pnl = 0`
   - 导致 `pnlPct = 0%`

2. **数量记录问题**：
   - 开仓时记录的 `quantity` 可能未正确保存到 `openPositions` map中
   - 或者在匹配开仓和平仓记录时出现问题

3. **杠杆或保证金计算问题**：
   - 如果 `marginUsed = 0`，则 `pnlPct` 会被设置为 `0.0`
   - 如果 `leverage = 0` 或未正确记录，会导致 `marginUsed` 计算错误

---

## 📊 代码流程分析

### 正常流程

```
1. 开仓 (executeOpenLongWithRecord)
   ↓
   获取市场价格 → actionRecord.Price = CurrentPrice
   计算数量 → actionRecord.Quantity = PositionSizeUSD / CurrentPrice
   执行开仓 → trader.OpenLong()
   ↓
   记录到 openPositions map:
   {
     "openPrice": actionRecord.Price,
     "quantity": actionRecord.Quantity,
     "leverage": decision.Leverage,
     ...
   }

2. 平仓 (executeCloseLongWithRecord)
   ↓
   获取市场价格 → actionRecord.Price = CurrentPrice
   执行平仓 → trader.CloseLong()
   ↓
   记录到 DecisionRecord.Decisions[]

3. 分析表现 (AnalyzePerformance)
   ↓
   匹配开仓和平仓记录 (通过 symbol_side key)
   ↓
   计算盈亏:
   pnl = quantity * (closePrice - openPrice)
   marginUsed = (quantity * openPrice) / leverage
   pnlPct = (pnl / marginUsed) * 100
```

### 问题流程

```
如果开仓价格 = 平仓价格 = 市场价格
   ↓
pnl = quantity * (相同价格 - 相同价格) = 0
   ↓
pnlPct = (0 / marginUsed) * 100 = 0%
```

---

## 🔧 需要检查的地方

1. **实际执行价格**：
   - 检查 `CloseLong/CloseShort` 返回的订单信息是否包含实际执行价格
   - 如果包含，应该使用实际执行价格而不是市场价格

2. **数量记录**：
   - 检查开仓时 `actionRecord.Quantity` 是否正确记录
   - 检查 `openPositions` map 中存储的 `quantity` 是否正确

3. **价格差异**：
   - 检查开仓和平仓时的市场价格是否真的相同
   - 如果相同，说明可能是在同一周期内开平仓，或者价格没有变化

4. **订单执行结果**：
   - 检查平仓订单的返回结果中是否包含实际成交价格
   - 应该使用实际成交价格而不是市场价格

---

## 📝 总结

**问题本质**：
- 历史成交记录显示 ±0% 是因为盈亏百分比计算使用了市场价格而不是实际执行价格
- 如果开仓和平仓时获取的市场价格相同（或非常接近），会导致 `pnl = 0`，进而 `pnlPct = 0%`

**解决方案方向**：
1. 使用实际订单执行价格（从订单返回结果中获取）而不是市场价格
2. 确保开仓和平仓时记录的价格是实际执行价格
3. 检查数量记录是否正确

**需要修改的文件**：
- `trader/auto_trader.go` - 修改平仓时价格记录逻辑，使用实际执行价格
- 可能需要修改 `trader/interface.go` 中的交易接口，确保返回实际执行价格

---

**报告生成时间**: 2025-11-04

