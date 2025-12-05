import useSWR from 'swr';
import { useEffect, useMemo } from 'react';
import { useLanguage } from '../contexts/LanguageContext';
import { t } from '../i18n/translations';
import { api } from '../lib/api';
import { Brain, BarChart3, TrendingUp, TrendingDown, Sparkles, Coins, Trophy, ScrollText, Lightbulb } from 'lucide-react';

interface TradeOutcome {
  symbol: string;
  side: string;
  quantity: number;
  leverage: number;
  open_price: number;
  close_price: number;
  position_value: number;
  margin_used: number;
  pn_l: number;
  pn_l_pct: number;
  net_pn_l?: number;  // 净盈亏（扣除手续费后）
  net_pn_l_pct?: number;  // 净盈亏百分比
  duration: string;
  open_time: string;
  close_time: string;
  was_stop_loss: boolean;
}

interface SymbolPerformance {
  symbol: string;
  total_trades: number;
  winning_trades: number;
  losing_trades: number;
  win_rate: number;
  total_pn_l: number;
  avg_pn_l: number;
}

interface PerformanceAnalysis {
  total_trades: number;
  winning_trades: number;
  losing_trades: number;
  win_rate: number;
  avg_win: number;
  avg_loss: number;
  profit_factor: number;
  sharpe_ratio: number;
  recent_trades: TradeOutcome[];
  symbol_stats: { [key: string]: SymbolPerformance };
  best_symbol?: string;
  worst_symbol?: string;
}

interface AILearningProps {
  traderId: string;
}

export default function AILearning({ traderId }: AILearningProps) {
  const { language } = useLanguage();
  const cacheKey = traderId ? `performance-${traderId}` : 'performance';
  const storageKey = traderId ? `ai-learning-trades-${traderId}` : 'ai-learning-trades';
  const binanceTradesKey = traderId ? `binance-trades-${traderId}` : 'binance-trades';

  const { data: performance, error } = useSWR<PerformanceAnalysis>(
    cacheKey,
    () => api.getPerformance(traderId),
    {
      refreshInterval: 30000, // 30秒刷新（AI学习分析数据更新频率较低）
      revalidateOnFocus: false,
      dedupingInterval: 20000,
    }
  );

  // 获取币安真实交易记录（优先使用）
  const { data: binanceTradesData } = useSWR(
    traderId ? binanceTradesKey : null,
    async () => {
      if (!traderId) return null;
      try {
        // 获取所有交易对的交易记录（币安API要求指定symbol，我们获取BTCUSDT作为主要交易对）
        const result = await api.getBinanceTrades(traderId, 'BTCUSDT', 100);
        const trades = result.trades || [];
        console.log('获取到币安交易记录:', trades.length, '条');
        return trades;
      } catch (err) {
        console.error('获取币安交易记录失败:', err);
        return null;
      }
    },
    {
      refreshInterval: 60000, // 60秒刷新一次（币安交易记录更新频率较低）
      revalidateOnFocus: false,
      dedupingInterval: 30000,
    }
  );

  // 持久化recent_trades到localStorage
  useEffect(() => {
    if (performance?.recent_trades && performance.recent_trades.length > 0) {
      try {
        // 合并已保存的交易记录和新的交易记录，去重并保持最新
        const saved = localStorage.getItem(storageKey);
        let existingTrades: TradeOutcome[] = [];
        
        if (saved) {
          try {
            existingTrades = JSON.parse(saved);
          } catch (e) {
            console.error('Failed to parse saved trades:', e);
          }
        }

        // 创建一个Map，以close_time为key去重，保留最新的记录
        const tradesMap = new Map<string, TradeOutcome>();
        
        // 先添加已保存的记录
        existingTrades.forEach((trade: TradeOutcome) => {
          if (trade.close_time) {
            tradesMap.set(trade.close_time, trade);
          }
        });

        // 再添加新的记录（会覆盖旧的）
        performance.recent_trades.forEach((trade: TradeOutcome) => {
          if (trade.close_time) {
            tradesMap.set(trade.close_time, trade);
          }
        });

        // 转换为数组并按close_time降序排序（最新的在前）
        const mergedTrades = Array.from(tradesMap.values()).sort((a, b) => {
          const timeA = new Date(a.close_time).getTime();
          const timeB = new Date(b.close_time).getTime();
          return timeB - timeA;
        });

        // 只保留最近500条记录，避免localStorage过大（增加显示数量）
        const limitedTrades = mergedTrades.slice(0, 500);

        localStorage.setItem(storageKey, JSON.stringify(limitedTrades));
      } catch (e) {
        console.error('Failed to save trades to localStorage:', e);
      }
    }
  }, [performance?.recent_trades, storageKey]);

  // 将币安交易数据转换为前端格式
  const convertedBinanceTrades = useMemo(() => {
    if (!binanceTradesData || binanceTradesData.length === 0) return null;
    
    // 币安API返回的是逐笔成交记录，需要按持仓方向和时间正确组合开平仓记录
    // 策略：按symbol和position_side分组，然后按时间顺序配对开仓和平仓
    
    // 按symbol和position_side分组
    const tradesByPosition = new Map<string, any[]>();
    binanceTradesData.forEach((trade: any) => {
      const key = `${trade.symbol}_${trade.position_side}`;
      if (!tradesByPosition.has(key)) {
        tradesByPosition.set(key, []);
      }
      tradesByPosition.get(key)!.push(trade);
    });
    
    // 将币安交易记录转换为TradeOutcome格式
    const convertedTrades: TradeOutcome[] = [];
    
    // 按持仓方向分组处理
    tradesByPosition.forEach((trades) => {
      if (trades.length === 0) return;
      
      // 按时间排序（从早到晚）
      trades.sort((a, b) => a.time - b.time);
      
      // 按持仓方向配对开仓和平仓
      // LONG: BUY开仓，SELL平仓
      // SHORT: SELL开仓，BUY平仓
      const positionSide = trades[0].position_side;
      let openTrades: any[] = [];
      
      for (let i = 0; i < trades.length; i++) {
        const trade = trades[i];
        const isOpenTrade = (positionSide === 'LONG' && trade.side === 'BUY') || 
                           (positionSide === 'SHORT' && trade.side === 'SELL');
        const isCloseTrade = (positionSide === 'LONG' && trade.side === 'SELL') || 
                            (positionSide === 'SHORT' && trade.side === 'BUY');
        
        if (isOpenTrade) {
          // 开仓：累积数量
          openTrades.push(trade);
        } else if (isCloseTrade && openTrades.length > 0) {
          // 平仓：匹配开仓记录
          // 使用加权平均价格计算开仓价
          let totalOpenQty = 0;
          let totalOpenValue = 0;
          let totalOpenCommission = 0;
          
          openTrades.forEach(ot => {
            totalOpenQty += ot.qty;
            totalOpenValue += ot.price * ot.qty;
            totalOpenCommission += ot.commission || 0;
          });
          
          const avgOpenPrice = totalOpenQty > 0 ? totalOpenValue / totalOpenQty : 0;
          const closePrice = trade.price;
          const closeQty = trade.qty;
          const closeCommission = trade.commission || 0;
          const realizedPnl = trade.realized_pnl || 0;
          
          // 计算实际使用的数量（取开仓和平仓的最小值）
          const actualQty = Math.min(totalOpenQty, closeQty);
          
          if (actualQty > 0 && avgOpenPrice > 0 && closePrice > 0) {
            const side = positionSide === 'LONG' ? 'long' : 'short';
            const totalCommission = totalOpenCommission + closeCommission;
            const netPnl = realizedPnl - totalCommission;
            
            // 计算持仓时长（使用最早的开仓时间和平仓时间）
            const openTime = new Date(Math.min(...openTrades.map(ot => ot.time)));
            const closeTime = new Date(trade.time);
            const durationMs = Math.max(0, closeTime.getTime() - openTime.getTime());
            const hours = Math.floor(durationMs / (1000 * 60 * 60));
            const minutes = Math.floor((durationMs % (1000 * 60 * 60)) / (1000 * 60));
            const seconds = Math.floor((durationMs % (1000 * 60)) / 1000);
            const duration = hours > 0 ? `${hours}小时${minutes}分` : minutes > 0 ? `${minutes}分${seconds}秒` : `${seconds}秒`;
            
            // 计算盈亏百分比（基于保证金）
            const positionValue = avgOpenPrice * actualQty;
            // 从realized_pnl反推杠杆倍数（如果可能）
            const estimatedLeverage = realizedPnl !== 0 && avgOpenPrice !== closePrice 
              ? Math.abs(realizedPnl / ((closePrice - avgOpenPrice) * actualQty * (side === 'long' ? 1 : -1)))
              : 15; // 默认15倍杠杆
            const marginUsed = positionValue / estimatedLeverage;
            const pnlPct = marginUsed > 0 ? (netPnl / marginUsed) * 100 : 0;
            
            convertedTrades.push({
              symbol: trade.symbol,
              side,
              quantity: actualQty,
              leverage: Math.round(estimatedLeverage),
              open_price: avgOpenPrice,
              close_price: closePrice,
              position_value: positionValue,
              margin_used: marginUsed,
              pn_l: realizedPnl,
              pn_l_pct: marginUsed > 0 ? (realizedPnl / marginUsed) * 100 : 0,
              net_pn_l: netPnl,
              net_pn_l_pct: pnlPct,
              duration,
              open_time: openTime.toISOString(),
              close_time: closeTime.toISOString(),
              was_stop_loss: false, // 币安API不直接提供止损信息
            });
            
            // 减少已匹配的开仓数量
            let remainingQty = closeQty;
            openTrades = openTrades.filter(ot => {
              if (remainingQty <= 0) return true;
              if (ot.qty <= remainingQty) {
                remainingQty -= ot.qty;
                return false;
              } else {
                ot.qty -= remainingQty;
                remainingQty = 0;
                return true;
              }
            });
          }
        }
      }
    });
    
    // 按close_time降序排序（最新的在前）
    convertedTrades.sort((a, b) => new Date(b.close_time).getTime() - new Date(a.close_time).getTime());
    
    console.log('转换后的币安交易记录:', convertedTrades.length, '条', convertedTrades.slice(0, 3));
    
    return convertedTrades;
  }, [binanceTradesData]);

  // 合并API数据和localStorage数据（优先使用币安真实数据）
  const mergedPerformance = useMemo(() => {
    try {
      const saved = localStorage.getItem(storageKey);
      let savedTrades: TradeOutcome[] = [];
      
      if (saved) {
        try {
          savedTrades = JSON.parse(saved);
        } catch (e) {
          console.error('Failed to parse saved trades:', e);
        }
      }

      // 优先使用币安真实交易数据
      if (convertedBinanceTrades && convertedBinanceTrades.length > 0) {
        // 使用币安真实数据，合并performance的其他统计信息
        const binanceTrades = convertedBinanceTrades;
        const winningTrades = binanceTrades.filter(t => (t.net_pn_l ?? t.pn_l) >= 0);
        const losingTrades = binanceTrades.filter(t => (t.net_pn_l ?? t.pn_l) < 0);
        const winCount = winningTrades.length;
        const lossCount = losingTrades.length;
        
        const avgWin = winCount > 0 
          ? winningTrades.reduce((sum, t) => sum + (t.net_pn_l ?? t.pn_l), 0) / winCount 
          : 0;
        const avgLoss = lossCount > 0 
          ? losingTrades.reduce((sum, t) => sum + (t.net_pn_l ?? t.pn_l), 0) / lossCount 
          : 0;
        
        return {
          ...(performance || {}),
          total_trades: binanceTrades.length,
          winning_trades: winCount,
          losing_trades: lossCount,
          win_rate: binanceTrades.length > 0 ? (winCount / binanceTrades.length) * 100 : 0,
          avg_win: avgWin,
          avg_loss: avgLoss,
          profit_factor: lossCount > 0 && winCount > 0 && avgLoss !== 0
            ? avgWin / Math.abs(avgLoss)
            : 0,
          recent_trades: binanceTrades,
        } as PerformanceAnalysis;
      }

      // 如果API返回了数据，使用API数据；否则使用保存的数据
      if (performance) {
        const apiTrades = performance.recent_trades || [];
        
        if (apiTrades.length > 0) {
          // API有数据，使用API数据（已经在useEffect中保存到localStorage）
          return performance;
        } else if (savedTrades.length > 0) {
          // API没有数据但localStorage有，使用localStorage的数据
          return {
            ...performance,
            recent_trades: savedTrades,
          };
        }
        return performance;
      } else if (savedTrades.length > 0) {
        // 如果API还没有数据，但localStorage有数据，返回一个临时的performance对象
        // 注意：这只是一个占位符，用于显示历史交易记录
        // 使用 net_pn_l（净盈亏）而不是 pn_l（毛盈亏）来计算统计
        const winningTrades = savedTrades.filter(t => (t.net_pn_l ?? t.pn_l) >= 0);
        const losingTrades = savedTrades.filter(t => (t.net_pn_l ?? t.pn_l) < 0);
        const winCount = winningTrades.length;
        const lossCount = losingTrades.length;
        
        // 计算各币种的统计
        const symbolMap = new Map<string, { total: number; wins: number; pnl: number }>();
        savedTrades.forEach(trade => {
          const netPnl = trade.net_pn_l ?? trade.pn_l;  // 优先使用净盈亏
          const existing = symbolMap.get(trade.symbol) || { total: 0, wins: 0, pnl: 0 };
          symbolMap.set(trade.symbol, {
            total: existing.total + 1,
            wins: existing.wins + (netPnl >= 0 ? 1 : 0),
            pnl: existing.pnl + netPnl,
          });
        });
        
        const symbolStats: { [key: string]: SymbolPerformance } = {};
        symbolMap.forEach((stats, symbol) => {
          symbolStats[symbol] = {
            symbol,
            total_trades: stats.total,
            winning_trades: stats.wins,
            losing_trades: stats.total - stats.wins,
            win_rate: (stats.wins / stats.total) * 100,
            total_pn_l: stats.pnl,
            avg_pn_l: stats.pnl / stats.total,
          };
        });
        
        // 找到最佳和最差币种
        let bestSymbol: string | undefined;
        let worstSymbol: string | undefined;
        let bestPnl = -Infinity;
        let worstPnl = Infinity;
        
        symbolMap.forEach((stats, symbol) => {
          if (stats.pnl > bestPnl) {
            bestPnl = stats.pnl;
            bestSymbol = symbol;
          }
          if (stats.pnl < worstPnl) {
            worstPnl = stats.pnl;
            worstSymbol = symbol;
          }
        });
        
        // 计算平均盈利和平均亏损（使用净盈亏）
        const avgWin = winCount > 0 
          ? winningTrades.reduce((sum, t) => sum + (t.net_pn_l ?? t.pn_l), 0) / winCount 
          : 0;
        const avgLoss = lossCount > 0 
          ? losingTrades.reduce((sum, t) => sum + (t.net_pn_l ?? t.pn_l), 0) / lossCount 
          : 0;
        
        return {
          total_trades: savedTrades.length,
          winning_trades: winCount,
          losing_trades: lossCount,
          win_rate: savedTrades.length > 0 ? (winCount / savedTrades.length) * 100 : 0,
          avg_win: avgWin,
          avg_loss: avgLoss,
          profit_factor: lossCount > 0 && winCount > 0 && avgLoss !== 0
            ? avgWin / Math.abs(avgLoss)
            : 0,
          sharpe_ratio: 0, // 需要更多数据才能计算夏普比率
          recent_trades: savedTrades,
          symbol_stats: symbolStats,
          best_symbol: bestSymbol,
          worst_symbol: worstSymbol,
        } as PerformanceAnalysis;
      }
    } catch (e) {
      console.error('Failed to merge performance data:', e);
    }

    return performance;
  }, [performance, storageKey, convertedBinanceTrades]);

  if (error) {
    return (
      <div className="rounded p-6" style={{ background: '#1E2329', border: '1px solid #2B3139' }}>
        <div style={{ color: '#F6465D' }}>{t('loadingError', language)}</div>
      </div>
    );
  }

  // 使用合并后的数据
  const displayPerformance = mergedPerformance || performance;

  if (!displayPerformance) {
    return (
      <div className="rounded p-6" style={{ background: '#1E2329', border: '1px solid #2B3139' }}>
        <div className="flex items-center gap-2" style={{ color: '#848E9C' }}>
          <BarChart3 className="w-4 h-4" /> {t('loading', language)}
        </div>
      </div>
    );
  }

  if (!displayPerformance || displayPerformance.total_trades === 0) {
    return (
      <div className="rounded p-6" style={{ background: '#1E2329', border: '1px solid #2B3139' }}>
        <div className="flex items-center gap-2 mb-2">
          <Brain className="w-5 h-5" style={{ color: '#8B5CF6' }} />
          <h2 className="text-lg font-bold" style={{ color: '#EAECEF' }}>{t('aiLearning', language)}</h2>
        </div>
        <div style={{ color: '#848E9C' }}>
          {t('noCompleteData', language)}
        </div>
      </div>
    );
  }

  const symbolStats = displayPerformance.symbol_stats || {};
  const symbolStatsList = Object.values(symbolStats).filter(stat => stat != null).sort(
    (a, b) => (b.total_pn_l || 0) - (a.total_pn_l || 0)
  );

  return (
    <div className="space-y-8">
      {/* 标题区 - 优化设计 */}
      <div className="relative rounded-2xl p-6 overflow-hidden" style={{
        background: 'linear-gradient(135deg, rgba(139, 92, 246, 0.15) 0%, rgba(99, 102, 241, 0.1) 50%, rgba(30, 35, 41, 0.8) 100%)',
        border: '1px solid rgba(139, 92, 246, 0.3)',
        boxShadow: '0 8px 32px rgba(139, 92, 246, 0.2)'
      }}>
        <div className="absolute top-0 right-0 w-96 h-96 rounded-full opacity-10" style={{
          background: 'radial-gradient(circle, #8B5CF6 0%, transparent 70%)',
          filter: 'blur(60px)'
        }} />
        <div className="relative flex items-center gap-4">
          <div className="w-16 h-16 rounded-2xl flex items-center justify-center" style={{
            background: 'linear-gradient(135deg, #8B5CF6 0%, #6366F1 100%)',
            boxShadow: '0 8px 24px rgba(139, 92, 246, 0.5)',
            border: '2px solid rgba(255, 255, 255, 0.1)'
          }}>
            <Brain className="w-8 h-8" style={{ color: '#FFF' }} />
          </div>
          <div>
            <h2 className="text-3xl font-bold mb-1" style={{
              color: '#EAECEF',
              textShadow: '0 2px 8px rgba(139, 92, 246, 0.3)'
            }}>
              {t('aiLearning', language)}
            </h2>
            <p className="text-base" style={{ color: '#A78BFA' }}>
              {t('tradesAnalyzed', language, { count: displayPerformance.total_trades })}
            </p>
          </div>
        </div>
      </div>

      {/* 核心指标卡片 - 4列网格 */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {/* 总交易数 */}
        <div className="rounded-2xl p-5 relative overflow-hidden group hover:scale-105 transition-transform" style={{
          background: 'linear-gradient(135deg, rgba(99, 102, 241, 0.2) 0%, rgba(30, 35, 41, 0.8) 100%)',
          border: '1px solid rgba(99, 102, 241, 0.3)',
          boxShadow: '0 4px 16px rgba(99, 102, 241, 0.2)'
        }}>
          <div className="absolute top-0 right-0 w-24 h-24 rounded-full opacity-20" style={{
            background: 'radial-gradient(circle, #6366F1 0%, transparent 70%)',
            filter: 'blur(20px)'
          }} />
          <div className="relative">
            <div className="text-xs font-semibold mb-3 uppercase tracking-wider" style={{ color: '#A5B4FC' }}>
              {t('totalTrades', language)}
            </div>
            <div className="text-4xl font-bold mono mb-1" style={{ color: '#E0E7FF' }}>
              {displayPerformance.total_trades}
            </div>
            <div className="text-xs flex items-center gap-1" style={{ color: '#6366F1' }}>
              <BarChart3 className="w-3 h-3" /> Trades
            </div>
          </div>
        </div>

        {/* 胜率 */}
        <div className="rounded-2xl p-5 relative overflow-hidden group hover:scale-105 transition-transform" style={{
          background: (displayPerformance.win_rate || 0) >= 50
            ? 'linear-gradient(135deg, rgba(16, 185, 129, 0.2) 0%, rgba(30, 35, 41, 0.8) 100%)'
            : 'linear-gradient(135deg, rgba(248, 113, 113, 0.2) 0%, rgba(30, 35, 41, 0.8) 100%)',
          border: `1px solid ${(displayPerformance.win_rate || 0) >= 50 ? 'rgba(16, 185, 129, 0.4)' : 'rgba(248, 113, 113, 0.4)'}`,
          boxShadow: `0 4px 16px ${(displayPerformance.win_rate || 0) >= 50 ? 'rgba(16, 185, 129, 0.2)' : 'rgba(248, 113, 113, 0.2)'}`
        }}>
          <div className="absolute top-0 right-0 w-24 h-24 rounded-full opacity-20" style={{
            background: `radial-gradient(circle, ${(displayPerformance.win_rate || 0) >= 50 ? '#10B981' : '#F87171'} 0%, transparent 70%)`,
            filter: 'blur(20px)'
          }} />
          <div className="relative">
            <div className="text-xs font-semibold mb-3 uppercase tracking-wider" style={{
              color: (displayPerformance.win_rate || 0) >= 50 ? '#6EE7B7' : '#FCA5A5'
            }}>
              {t('winRate', language)}
            </div>
            <div className="text-4xl font-bold mono mb-1" style={{
              color: (displayPerformance.win_rate || 0) >= 50 ? '#10B981' : '#F87171'
            }}>
              {(displayPerformance.win_rate || 0).toFixed(1)}%
            </div>
            <div className="text-xs" style={{ color: '#94A3B8' }}>
              {displayPerformance.winning_trades || 0}W / {displayPerformance.losing_trades || 0}L
            </div>
          </div>
        </div>

        {/* 平均盈利 */}
        <div className="rounded-2xl p-5 relative overflow-hidden group hover:scale-105 transition-transform" style={{
          background: 'linear-gradient(135deg, rgba(14, 203, 129, 0.2) 0%, rgba(30, 35, 41, 0.8) 100%)',
          border: '1px solid rgba(14, 203, 129, 0.3)',
          boxShadow: '0 4px 16px rgba(14, 203, 129, 0.2)'
        }}>
          <div className="absolute top-0 right-0 w-24 h-24 rounded-full opacity-20" style={{
            background: 'radial-gradient(circle, #0ECB81 0%, transparent 70%)',
            filter: 'blur(20px)'
          }} />
          <div className="relative">
            <div className="text-xs font-semibold mb-3 uppercase tracking-wider" style={{ color: '#6EE7B7' }}>
              {t('avgWin', language)}
            </div>
            <div className="text-4xl font-bold mono mb-1" style={{ color: '#10B981' }}>
              +{(displayPerformance.avg_win || 0).toFixed(2)}
            </div>
            <div className="text-xs flex items-center gap-1" style={{ color: '#6EE7B7' }}>
              <TrendingUp className="w-3 h-3" /> USDT Average
            </div>
          </div>
        </div>

        {/* 平均亏损 */}
        <div className="rounded-2xl p-5 relative overflow-hidden group hover:scale-105 transition-transform" style={{
          background: 'linear-gradient(135deg, rgba(246, 70, 93, 0.2) 0%, rgba(30, 35, 41, 0.8) 100%)',
          border: '1px solid rgba(246, 70, 93, 0.3)',
          boxShadow: '0 4px 16px rgba(246, 70, 93, 0.2)'
        }}>
          <div className="absolute top-0 right-0 w-24 h-24 rounded-full opacity-20" style={{
            background: 'radial-gradient(circle, #F6465D 0%, transparent 70%)',
            filter: 'blur(20px)'
          }} />
          <div className="relative">
            <div className="text-xs font-semibold mb-3 uppercase tracking-wider" style={{ color: '#FCA5A5' }}>
              {t('avgLoss', language)}
            </div>
            <div className="text-4xl font-bold mono mb-1" style={{ color: '#F87171' }}>
              {(displayPerformance.avg_loss || 0).toFixed(2)}
            </div>
            <div className="text-xs flex items-center gap-1" style={{ color: '#FCA5A5' }}>
              <TrendingDown className="w-3 h-3" /> USDT Average
            </div>
          </div>
        </div>
      </div>

      {/* 关键指标：夏普比率 & 盈亏比 - 2列网格 */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* 夏普比率 */}
        <div className="rounded-2xl p-6 relative overflow-hidden" style={{
          background: 'linear-gradient(135deg, rgba(139, 92, 246, 0.25) 0%, rgba(99, 102, 241, 0.15) 50%, rgba(30, 35, 41, 0.9) 100%)',
          border: '2px solid rgba(139, 92, 246, 0.5)',
          boxShadow: '0 12px 40px rgba(139, 92, 246, 0.3)'
        }}>
          <div className="absolute top-0 right-0 w-48 h-48 rounded-full opacity-20" style={{
            background: 'radial-gradient(circle, #8B5CF6 0%, transparent 70%)',
            filter: 'blur(40px)'
          }} />
          <div className="relative">
            <div className="flex items-center gap-3 mb-4">
              <div className="w-12 h-12 rounded-xl flex items-center justify-center" style={{
                background: 'rgba(139, 92, 246, 0.3)',
                border: '1px solid rgba(139, 92, 246, 0.5)'
              }}>
                <Sparkles className="w-6 h-6" style={{ color: '#A78BFA' }} />
              </div>
              <div>
                <div className="text-lg font-bold" style={{ color: '#C4B5FD' }}>夏普比率</div>
                <div className="text-xs" style={{ color: '#94A3B8' }}>风险调整后收益 · AI自我进化指标</div>
              </div>
            </div>

            <div className="flex items-end justify-between mb-4">
              <div className="text-6xl font-bold mono" style={{
                color: (displayPerformance.sharpe_ratio || 0) >= 2 ? '#10B981' :
                       (displayPerformance.sharpe_ratio || 0) >= 1 ? '#22D3EE' :
                       (displayPerformance.sharpe_ratio || 0) >= 0 ? '#F0B90B' : '#F87171',
                textShadow: '0 4px 12px rgba(0, 0, 0, 0.3)'
              }}>
                {displayPerformance.sharpe_ratio ? displayPerformance.sharpe_ratio.toFixed(2) : 'N/A'}
              </div>

              {displayPerformance.sharpe_ratio !== undefined && (
                <div className="text-right mb-2">
                  <div className="text-sm font-bold px-3 py-1 rounded-lg" style={{
                    color: (displayPerformance.sharpe_ratio || 0) >= 2 ? '#10B981' :
                           (displayPerformance.sharpe_ratio || 0) >= 1 ? '#22D3EE' :
                           (displayPerformance.sharpe_ratio || 0) >= 0 ? '#F0B90B' : '#F87171',
                    background: (displayPerformance.sharpe_ratio || 0) >= 2 ? 'rgba(16, 185, 129, 0.2)' :
                               (displayPerformance.sharpe_ratio || 0) >= 1 ? 'rgba(34, 211, 238, 0.2)' :
                               (displayPerformance.sharpe_ratio || 0) >= 0 ? 'rgba(240, 185, 11, 0.2)' : 'rgba(248, 113, 113, 0.2)'
                  }}>
                    {displayPerformance.sharpe_ratio >= 2 ? '🟢 卓越表现' :
                     displayPerformance.sharpe_ratio >= 1 ? '🟢 良好表现' :
                     displayPerformance.sharpe_ratio >= 0 ? '🟡 波动较大' : '🔴 需要调整'}
                  </div>
                </div>
              )}
            </div>

            {displayPerformance.sharpe_ratio !== undefined && (
              <div className="rounded-xl p-4" style={{
                background: 'rgba(0, 0, 0, 0.4)',
                border: '1px solid rgba(139, 92, 246, 0.3)'
              }}>
                <div className="text-sm leading-relaxed" style={{ color: '#DDD6FE' }}>
                  {displayPerformance.sharpe_ratio >= 2 && '✨ AI策略非常有效！风险调整后收益优异，可适度扩大仓位但保持纪律。'}
                  {displayPerformance.sharpe_ratio >= 1 && displayPerformance.sharpe_ratio < 2 && '✅ 策略表现稳健，风险收益平衡良好，继续保持当前策略。'}
                  {displayPerformance.sharpe_ratio >= 0 && displayPerformance.sharpe_ratio < 1 && '⚠️ 收益为正但波动较大，AI正在优化策略，降低风险。'}
                  {displayPerformance.sharpe_ratio < 0 && '🚨 当前策略需要调整！AI已自动进入保守模式，减少仓位和交易频率。'}
                </div>
              </div>
            )}
          </div>
        </div>

        {/* 盈亏比 */}
        <div className="rounded-2xl p-6 relative overflow-hidden" style={{
          background: 'linear-gradient(135deg, rgba(240, 185, 11, 0.25) 0%, rgba(252, 213, 53, 0.15) 50%, rgba(30, 35, 41, 0.9) 100%)',
          border: '2px solid rgba(240, 185, 11, 0.5)',
          boxShadow: '0 12px 40px rgba(240, 185, 11, 0.3)'
        }}>
          <div className="absolute top-0 right-0 w-48 h-48 rounded-full opacity-20" style={{
            background: 'radial-gradient(circle, #F0B90B 0%, transparent 70%)',
            filter: 'blur(40px)'
          }} />
          <div className="relative">
            <div className="flex items-center gap-3 mb-4">
              <div className="w-12 h-12 rounded-xl flex items-center justify-center" style={{
                background: 'rgba(240, 185, 11, 0.3)',
                border: '1px solid rgba(240, 185, 11, 0.5)'
              }}>
                <Coins className="w-6 h-6" style={{ color: '#FCD34D' }} />
              </div>
              <div>
                <div className="text-lg font-bold" style={{ color: '#FCD34D' }}>
                  {t('profitFactor', language)}
                </div>
                <div className="text-xs" style={{ color: '#94A3B8' }}>
                  {t('avgWinDivLoss', language)}
                </div>
              </div>
            </div>

            <div className="flex items-end justify-between mb-4">
              <div className="text-6xl font-bold mono" style={{
                color: (displayPerformance.profit_factor || 0) >= 2.0 ? '#10B981' :
                       (displayPerformance.profit_factor || 0) >= 1.5 ? '#F0B90B' :
                       (displayPerformance.profit_factor || 0) >= 1.0 ? '#FB923C' : '#F87171',
                textShadow: '0 4px 12px rgba(0, 0, 0, 0.3)'
              }}>
                {(displayPerformance.profit_factor || 0) > 0 ? (displayPerformance.profit_factor || 0).toFixed(2) : 'N/A'}
              </div>

              <div className="text-right mb-2">
                <div className="text-sm font-bold px-3 py-1 rounded-lg" style={{
                  color: (displayPerformance.profit_factor || 0) >= 2.0 ? '#10B981' :
                         (displayPerformance.profit_factor || 0) >= 1.5 ? '#F0B90B' : '#94A3B8',
                  background: (displayPerformance.profit_factor || 0) >= 2.0 ? 'rgba(16, 185, 129, 0.2)' :
                             (displayPerformance.profit_factor || 0) >= 1.5 ? 'rgba(240, 185, 11, 0.2)' : 'rgba(148, 163, 184, 0.2)'
                }}>
                  {(displayPerformance.profit_factor || 0) >= 2.0 && t('excellent', language)}
                  {(displayPerformance.profit_factor || 0) >= 1.5 && (displayPerformance.profit_factor || 0) < 2.0 && t('good', language)}
                  {(displayPerformance.profit_factor || 0) >= 1.0 && (displayPerformance.profit_factor || 0) < 1.5 && t('fair', language)}
                  {(displayPerformance.profit_factor || 0) > 0 && (displayPerformance.profit_factor || 0) < 1.0 && t('poor', language)}
                </div>
              </div>
            </div>

            <div className="rounded-xl p-4" style={{
              background: 'rgba(0, 0, 0, 0.4)',
              border: '1px solid rgba(240, 185, 11, 0.3)'
            }}>
              <div className="text-sm leading-relaxed" style={{ color: '#FEF3C7' }}>
                {(displayPerformance.profit_factor || 0) >= 2.0 && '🔥 盈利能力出色！每亏1元能赚' + (displayPerformance.profit_factor || 0).toFixed(1) + '元，AI策略表现优异。'}
                {(displayPerformance.profit_factor || 0) >= 1.5 && (displayPerformance.profit_factor || 0) < 2.0 && '✓ 策略稳定盈利，盈亏比健康，继续保持纪律性交易。'}
                {(displayPerformance.profit_factor || 0) >= 1.0 && (displayPerformance.profit_factor || 0) < 1.5 && '⚠️ 策略略有盈利但需优化，AI正在调整仓位和止损策略。'}
                {(displayPerformance.profit_factor || 0) > 0 && (displayPerformance.profit_factor || 0) < 1.0 && '❌ 平均亏损大于盈利，需要调整策略或降低交易频率。'}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* 最佳/最差币种 - 独立行 */}
      {(displayPerformance.best_symbol || displayPerformance.worst_symbol) && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {displayPerformance.best_symbol && (
            <div className="rounded-2xl p-6 backdrop-blur-sm" style={{
              background: 'linear-gradient(135deg, rgba(16, 185, 129, 0.15) 0%, rgba(14, 203, 129, 0.05) 100%)',
              border: '1px solid rgba(16, 185, 129, 0.3)',
              boxShadow: '0 4px 16px rgba(16, 185, 129, 0.1)'
            }}>
              <div className="flex items-center gap-2 mb-3">
                <Trophy className="w-6 h-6" style={{ color: '#10B981' }} />
                <span className="text-sm font-semibold" style={{ color: '#6EE7B7' }}>{t('bestPerformer', language)}</span>
              </div>
              <div className="text-3xl font-bold mono mb-1" style={{ color: '#10B981' }}>
                {displayPerformance.best_symbol}
              </div>
              {symbolStats[displayPerformance.best_symbol] && (
                <div className="text-lg font-semibold" style={{ color: '#6EE7B7' }}>
                  {symbolStats[displayPerformance.best_symbol].total_pn_l > 0 ? '+' : ''}
                  {symbolStats[displayPerformance.best_symbol].total_pn_l.toFixed(2)} USDT {t('pnl', language)}
                </div>
              )}
            </div>
          )}

          {displayPerformance.worst_symbol && (
            <div className="rounded-2xl p-6 backdrop-blur-sm" style={{
              background: 'linear-gradient(135deg, rgba(248, 113, 113, 0.15) 0%, rgba(246, 70, 93, 0.05) 100%)',
              border: '1px solid rgba(248, 113, 113, 0.3)',
              boxShadow: '0 4px 16px rgba(248, 113, 113, 0.1)'
            }}>
              <div className="flex items-center gap-2 mb-3">
                <TrendingDown className="w-6 h-6" style={{ color: '#F87171' }} />
                <span className="text-sm font-semibold" style={{ color: '#FCA5A5' }}>{t('worstPerformer', language)}</span>
              </div>
              <div className="text-3xl font-bold mono mb-1" style={{ color: '#F87171' }}>
                {displayPerformance.worst_symbol}
              </div>
              {symbolStats[displayPerformance.worst_symbol] && (
                <div className="text-lg font-semibold" style={{ color: '#FCA5A5' }}>
                  {symbolStats[displayPerformance.worst_symbol].total_pn_l > 0 ? '+' : ''}
                  {symbolStats[displayPerformance.worst_symbol].total_pn_l.toFixed(2)} USDT {t('pnl', language)}
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* 币种表现 & 历史成交 - 左右分屏 2列布局 */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* 左侧：币种表现统计表格 */}
        {symbolStatsList.length > 0 && (
          <div className="rounded-2xl overflow-hidden" style={{
            background: 'rgba(30, 35, 41, 0.4)',
            border: '1px solid rgba(99, 102, 241, 0.2)',
            boxShadow: '0 4px 16px rgba(0, 0, 0, 0.2)',
            maxHeight: 'calc(100vh - 200px)'
          }}>
            <div className="p-5 border-b sticky top-0 z-10" style={{
              borderColor: 'rgba(99, 102, 241, 0.2)',
              background: 'rgba(30, 35, 41, 0.95)',
              backdropFilter: 'blur(10px)'
            }}>
              <h3 className="font-bold flex items-center gap-2 text-lg" style={{ color: '#E0E7FF' }}>
                <BarChart3 className="w-5 h-5" /> {t('symbolPerformance', language)}
              </h3>
            </div>
            <div className="overflow-y-auto" style={{ maxHeight: 'calc(100vh - 280px)' }}>
              <table className="w-full">
                <thead className="sticky top-0 z-10">
                  <tr style={{ background: 'rgba(15, 23, 42, 0.95)', backdropFilter: 'blur(10px)' }}>
                    <th className="text-left px-4 py-3 text-xs font-semibold" style={{ color: '#94A3B8' }}>Symbol</th>
                    <th className="text-right px-4 py-3 text-xs font-semibold" style={{ color: '#94A3B8' }}>Trades</th>
                    <th className="text-right px-4 py-3 text-xs font-semibold" style={{ color: '#94A3B8' }}>Win Rate</th>
                    <th className="text-right px-4 py-3 text-xs font-semibold" style={{ color: '#94A3B8' }}>Total P&L (USDT)</th>
                    <th className="text-right px-4 py-3 text-xs font-semibold" style={{ color: '#94A3B8' }}>Avg P&L (USDT)</th>
                  </tr>
                </thead>
                <tbody>
                  {symbolStatsList.map((stat, idx) => (
                    <tr key={stat.symbol} className="transition-colors hover:bg-white/5" style={{
                      borderTop: idx > 0 ? '1px solid rgba(99, 102, 241, 0.1)' : 'none'
                    }}>
                      <td className="px-4 py-3">
                        <span className="font-bold mono text-sm" style={{ color: '#E0E7FF' }}>{stat.symbol}</span>
                      </td>
                      <td className="px-4 py-3 text-right mono text-sm" style={{ color: '#CBD5E1' }}>
                        {stat.total_trades}
                      </td>
                      <td className="px-4 py-3 text-right mono text-sm font-semibold" style={{
                        color: (stat.win_rate || 0) >= 50 ? '#10B981' : '#F87171'
                      }}>
                        {(stat.win_rate || 0).toFixed(1)}%
                      </td>
                      <td className="px-4 py-3 text-right mono text-sm font-bold" style={{
                        color: (stat.total_pn_l || 0) > 0 ? '#10B981' : '#F87171'
                      }}>
                        {(stat.total_pn_l || 0) > 0 ? '+' : ''}{(stat.total_pn_l || 0).toFixed(2)}
                      </td>
                      <td className="px-4 py-3 text-right mono text-sm" style={{
                        color: (stat.avg_pn_l || 0) > 0 ? '#10B981' : '#F87171'
                      }}>
                        {(stat.avg_pn_l || 0) > 0 ? '+' : ''}{(stat.avg_pn_l || 0).toFixed(2)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* 右侧：历史成交记录 */}
        <div className="rounded-2xl overflow-hidden" style={{
          background: 'rgba(30, 35, 41, 0.4)',
          border: '1px solid rgba(240, 185, 11, 0.2)',
          maxHeight: 'calc(100vh - 200px)'
        }}>
          <div className="p-5 border-b sticky top-0 z-10" style={{
            background: 'rgba(240, 185, 11, 0.1)',
            borderColor: 'rgba(240, 185, 11, 0.3)',
            backdropFilter: 'blur(10px)'
          }}>
            <div className="flex items-center gap-2">
              <ScrollText className="w-6 h-6" style={{ color: '#FCD34D' }} />
              <div>
                <h3 className="font-bold text-lg" style={{ color: '#FCD34D' }}>{t('tradeHistory', language)}</h3>
                <p className="text-xs" style={{ color: '#94A3B8' }}>
                  {displayPerformance?.recent_trades && displayPerformance.recent_trades.length > 0
                    ? t('completedTrades', language, { count: displayPerformance.recent_trades.length })
                    : t('completedTradesWillAppear', language)}
                </p>
              </div>
            </div>
          </div>

          <div className="overflow-y-auto p-4 space-y-3" style={{ maxHeight: 'calc(100vh - 280px)' }}>
            {displayPerformance?.recent_trades && displayPerformance.recent_trades.length > 0 ? (
              displayPerformance.recent_trades.map((trade: TradeOutcome, idx: number) => {
                // 使用净盈亏（net_pn_l）来判断是否盈利，如果没有则使用毛盈亏（pn_l）
                const netPnl = trade.net_pn_l ?? trade.pn_l;
                const netPnlPct = trade.net_pn_l_pct ?? trade.pn_l_pct;
                const isProfitable = netPnl >= 0;
                const isRecent = idx === 0;

                return (
                  <div key={idx} className="rounded-xl p-4 backdrop-blur-sm transition-all hover:scale-[1.02]" style={{
                    background: isRecent
                      ? isProfitable
                        ? 'linear-gradient(135deg, rgba(16, 185, 129, 0.15) 0%, rgba(14, 203, 129, 0.05) 100%)'
                        : 'linear-gradient(135deg, rgba(248, 113, 113, 0.15) 0%, rgba(246, 70, 93, 0.05) 100%)'
                      : 'rgba(30, 35, 41, 0.4)',
                    border: isRecent
                      ? isProfitable ? '1px solid rgba(16, 185, 129, 0.4)' : '1px solid rgba(248, 113, 113, 0.4)'
                      : '1px solid rgba(71, 85, 105, 0.3)',
                    boxShadow: isRecent
                      ? '0 4px 16px rgba(139, 92, 246, 0.2)'
                      : '0 2px 8px rgba(0, 0, 0, 0.1)'
                  }}>
                    <div className="flex items-center justify-between mb-3">
                      <div className="flex items-center gap-2">
                        <span className="text-base font-bold mono" style={{ color: '#E0E7FF' }}>
                          {trade.symbol}
                        </span>
                        <span className="text-xs px-2 py-1 rounded font-bold" style={{
                          background: trade.side === 'long' ? 'rgba(14, 203, 129, 0.2)' : 'rgba(246, 70, 93, 0.2)',
                          color: trade.side === 'long' ? '#10B981' : '#F87171'
                        }}>
                          {trade.side.toUpperCase()}
                        </span>
                        {isRecent && (
                          <span className="text-xs px-2 py-0.5 rounded font-semibold" style={{
                            background: 'rgba(240, 185, 11, 0.2)',
                            color: '#FCD34D'
                          }}>
                            {t('latest', language)}
                          </span>
                        )}
                      </div>
                      <div className="text-lg font-bold mono" style={{
                        color: isProfitable ? '#10B981' : '#F87171'
                      }}>
                        {isProfitable ? '+' : ''}{netPnlPct.toFixed(2)}%
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-2 mb-3 text-xs">
                      <div>
                        <div style={{ color: '#94A3B8' }}>{t('entry', language)}</div>
                        <div className="font-mono font-semibold" style={{ color: '#CBD5E1' }}>
                          {trade.open_price.toFixed(4)}
                        </div>
                      </div>
                      <div className="text-right">
                        <div style={{ color: '#94A3B8' }}>{t('exit', language)}</div>
                        <div className="font-mono font-semibold" style={{ color: '#CBD5E1' }}>
                          {trade.close_price.toFixed(4)}
                        </div>
                      </div>
                    </div>

                    {/* Position Details */}
                    <div className="grid grid-cols-2 gap-2 mb-3 text-xs">
                      <div>
                        <div style={{ color: '#94A3B8' }}>Quantity</div>
                        <div className="font-mono font-semibold" style={{ color: '#CBD5E1' }}>
                          {trade.quantity ? trade.quantity.toFixed(4) : '-'}
                        </div>
                      </div>
                      <div className="text-right">
                        <div style={{ color: '#94A3B8' }}>Leverage</div>
                        <div className="font-mono font-semibold" style={{ color: '#FCD34D' }}>
                          {trade.leverage ? `${trade.leverage}x` : '-'}
                        </div>
                      </div>
                      <div>
                        <div style={{ color: '#94A3B8' }}>Position Value</div>
                        <div className="font-mono font-semibold" style={{ color: '#CBD5E1' }}>
                          {trade.position_value ? `$${trade.position_value.toFixed(2)}` : '-'}
                        </div>
                      </div>
                      <div className="text-right">
                        <div style={{ color: '#94A3B8' }}>Margin Used</div>
                        <div className="font-mono font-semibold" style={{ color: '#A78BFA' }}>
                          {trade.margin_used ? `$${trade.margin_used.toFixed(2)}` : '-'}
                        </div>
                      </div>
                    </div>

                    <div className="rounded-lg p-2 mb-2" style={{
                      background: isProfitable ? 'rgba(16, 185, 129, 0.1)' : 'rgba(248, 113, 113, 0.1)'
                    }}>
                      <div className="flex items-center justify-between text-xs">
                        <span style={{ color: '#94A3B8' }}>P&L</span>
                        <span className="font-bold mono" style={{
                          color: isProfitable ? '#10B981' : '#F87171'
                        }}>
                          {isProfitable ? '+' : ''}{netPnl.toFixed(2)} USDT
                        </span>
                      </div>
                    </div>

                    <div className="flex items-center justify-between text-xs" style={{ color: '#94A3B8' }}>
                      <span>⏱️ {formatDuration(trade.duration)}</span>
                      {trade.was_stop_loss && (
                        <span className="px-2 py-0.5 rounded font-semibold" style={{
                          background: 'rgba(248, 113, 113, 0.2)',
                          color: '#FCA5A5'
                        }}>
                          {t('stopLoss', language)}
                        </span>
                      )}
                    </div>

                    <div className="text-xs mt-2 pt-2 border-t" style={{
                      color: '#64748B',
                      borderColor: 'rgba(71, 85, 105, 0.3)'
                    }}>
                      {new Date(trade.close_time).toLocaleString('en-US', {
                        month: 'short',
                        day: '2-digit',
                        hour: '2-digit',
                        minute: '2-digit'
                      })}
                    </div>
                  </div>
                );
              })
            ) : (
              <div className="p-6 text-center">
                <div className="mb-2 flex justify-center opacity-50">
                  <ScrollText className="w-10 h-10" style={{ color: '#94A3B8' }} />
                </div>
                <div style={{ color: '#94A3B8' }}>{t('noCompletedTrades', language)}</div>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* AI学习说明 - 现代化设计 */}
      <div className="rounded-2xl p-6 backdrop-blur-sm" style={{
        background: 'linear-gradient(135deg, rgba(240, 185, 11, 0.1) 0%, rgba(252, 213, 53, 0.05) 100%)',
        border: '1px solid rgba(240, 185, 11, 0.2)',
        boxShadow: '0 4px 16px rgba(240, 185, 11, 0.1)'
      }}>
        <div className="flex items-start gap-4">
          <div className="w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0" style={{
            background: 'rgba(240, 185, 11, 0.2)',
            border: '1px solid rgba(240, 185, 11, 0.3)'
          }}>
            <Lightbulb className="w-5 h-5" style={{ color: '#FCD34D' }} />
          </div>
          <div>
            <h3 className="font-bold mb-3 text-base" style={{ color: '#FCD34D' }}>{t('howAILearns', language)}</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 text-sm">
              <div className="flex items-start gap-2">
                <span style={{ color: '#F0B90B' }}>•</span>
                <span style={{ color: '#CBD5E1' }}>{t('aiLearningPoint1', language)}</span>
              </div>
              <div className="flex items-start gap-2">
                <span style={{ color: '#F0B90B' }}>•</span>
                <span style={{ color: '#CBD5E1' }}>{t('aiLearningPoint2', language)}</span>
              </div>
              <div className="flex items-start gap-2">
                <span style={{ color: '#F0B90B' }}>•</span>
                <span style={{ color: '#CBD5E1' }}>{t('aiLearningPoint3', language)}</span>
              </div>
              <div className="flex items-start gap-2">
                <span style={{ color: '#F0B90B' }}>•</span>
                <span style={{ color: '#CBD5E1' }}>{t('aiLearningPoint4', language)}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// 格式化持仓时长
function formatDuration(duration: string | undefined): string {
  if (!duration) return '-';

  const match = duration.match(/(\d+h)?(\d+m)?(\d+\.?\d*s)?/);
  if (!match) return duration;

  const hours = match[1] || '';
  const minutes = match[2] || '';
  const seconds = match[3] || '';

  let result = '';
  if (hours) result += hours.replace('h', '小时');
  if (minutes) result += minutes.replace('m', '分');
  if (!hours && seconds) result += seconds.replace(/(\d+)\.?\d*s/, '$1秒');

  return result || duration;
}
