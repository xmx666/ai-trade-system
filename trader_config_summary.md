# 交易员配置信息

## 交易员ID
`binance_admin_deepseek_1764582109`

## 币种配置

根据决策日志文件 `decision_20251205_024513_cycle115.json`，当前交易员配置的候选币种为：

1. **BTCUSDT**
2. **ETHUSDT**
3. **SOLUSDT**
4. **BNBUSDT**
5. **XRPUSDT**
6. **DOGEUSDT**
7. **ADAUSDT**
8. **HYPEUSDT**

**总计：8个币种**

## 时间配置

从决策日志的时间戳可以看出：
- **最后更新时间**: 2025-12-05 02:45:13 (UTC+8)
- **周期编号**: 115

## 账户状态

- **总余额**: 116.88 USDT
- **可用余额**: 116.88 USDT
- **未实现盈亏**: 7.88 USDT
- **持仓数量**: 0

## 注意事项

1. 这些币种是从决策日志中提取的候选币种列表
2. 实际的交易币种配置存储在数据库 `config.db` 的 `traders` 表中
3. 扫描间隔等时间配置也需要从数据库中查询

## 查询完整配置的方法

要查看完整的配置信息（包括扫描间隔、杠杆等），可以：

1. **使用Python脚本**（如果数据库可访问）:
   ```bash
   python3 check_trader_config.py binance_admin_deepseek_1764582109
   ```

2. **直接查询数据库**:
   ```bash
   sqlite3 config.db "SELECT * FROM traders WHERE id = 'binance_admin_deepseek_1764582109';"
   ```

3. **通过Web界面**: 访问交易员配置页面查看完整信息

