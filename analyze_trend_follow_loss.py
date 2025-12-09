#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
分析 trend_follow 策略的交易记录，找出从 240u 亏到 100u 的原因
"""

import json
import os
from datetime import datetime
from collections import defaultdict
from pathlib import Path

def find_trend_follow_logs(log_dir="decision_logs"):
    """查找所有使用 trend_following 策略的交易记录"""
    trend_follow_files = []
    
    for root, dirs, files in os.walk(log_dir):
        for file in files:
            if file.endswith('.json'):
                filepath = os.path.join(root, file)
                try:
                    with open(filepath, 'r', encoding='utf-8') as f:
                        data = json.load(f)
                        system_prompt = data.get('system_prompt', '')
                        if 'trend_following' in system_prompt or '趋势跟踪' in system_prompt:
                            trend_follow_files.append((filepath, data))
                except Exception as e:
                    continue
    
    # 按时间戳排序
    trend_follow_files.sort(key=lambda x: x[1].get('timestamp', ''))
    return trend_follow_files

def analyze_trades(files):
    """分析交易记录"""
    account_history = []  # 账户余额历史
    trades = []  # 所有交易
    open_positions = {}  # 当前持仓
    
    for filepath, data in files:
        timestamp = data.get('timestamp', '')
        account_state = data.get('account_state', {})
        total_balance = account_state.get('total_balance', 0)
        total_unrealized = account_state.get('total_unrealized_profit', 0)
        decisions = data.get('decisions', [])
        
        # 记录账户状态
        account_history.append({
            'timestamp': timestamp,
            'total_balance': total_balance,
            'total_unrealized': total_unrealized,
            'available_balance': account_state.get('available_balance', 0),
            'position_count': account_state.get('position_count', 0),
            'cycle': data.get('cycle_number', 0)
        })
        
        # 分析交易决策
        for decision in decisions:
            action = decision.get('action', '')
            symbol = decision.get('symbol', '')
            price = decision.get('price', 0)
            quantity = decision.get('quantity', 0)
            leverage = decision.get('leverage', 0)
            success = decision.get('success', False)
            timestamp = decision.get('timestamp', '')
            
            if not success:
                continue
            
            pos_key = f"{symbol}_{'long' if 'long' in action else 'short'}"
            
            if action in ['open_long', 'open_short']:
                # 开仓
                open_positions[pos_key] = {
                    'symbol': symbol,
                    'side': 'long' if 'long' in action else 'short',
                    'open_price': price,
                    'quantity': quantity,
                    'leverage': leverage,
                    'open_time': timestamp,
                    'open_balance': total_balance
                }
            elif action in ['close_long', 'close_short']:
                # 平仓
                if pos_key in open_positions:
                    open_pos = open_positions[pos_key]
                    # 计算盈亏
                    if open_pos['side'] == 'long':
                        pnl = quantity * (price - open_pos['open_price'])
                    else:
                        pnl = quantity * (open_pos['open_price'] - price)
                    
                    # 计算保证金
                    position_value = quantity * open_pos['open_price']
                    margin_used = position_value / open_pos['leverage'] if open_pos['leverage'] > 0 else 0
                    
                    # 计算手续费（开仓0.04% + 平仓0.04% = 0.08%）
                    fee = position_value * 0.0008
                    net_pnl = pnl - fee
                    
                    pnl_pct = (pnl / margin_used * 100) if margin_used > 0 else 0
                    net_pnl_pct = (net_pnl / margin_used * 100) if margin_used > 0 else 0
                    
                    # 计算持仓时长
                    open_time = datetime.fromisoformat(open_pos['open_time'].replace('Z', '+00:00'))
                    close_time = datetime.fromisoformat(timestamp.replace('Z', '+00:00'))
                    duration = (close_time - open_time).total_seconds() / 60  # 分钟
                    
                    trades.append({
                        'symbol': symbol,
                        'side': open_pos['side'],
                        'open_price': open_pos['open_price'],
                        'close_price': price,
                        'quantity': quantity,
                        'leverage': open_pos['leverage'],
                        'pnl': pnl,
                        'net_pnl': net_pnl,
                        'pnl_pct': pnl_pct,
                        'net_pnl_pct': net_pnl_pct,
                        'fee': fee,
                        'margin_used': margin_used,
                        'duration_minutes': duration,
                        'open_time': open_pos['open_time'],
                        'close_time': timestamp,
                        'open_balance': open_pos['open_balance'],
                        'close_balance': total_balance
                    })
                    
                    del open_positions[pos_key]
    
    return account_history, trades

def generate_report(account_history, trades):
    """生成分析报告"""
    print("=" * 80)
    print("TREND_FOLLOW 策略亏损分析报告")
    print("=" * 80)
    print()
    
    # 1. 账户余额变化
    print("## 1. 账户余额变化")
    print("-" * 80)
    if account_history:
        initial_balance = account_history[0]['total_balance']
        final_balance = account_history[-1]['total_balance']
        total_loss = final_balance - initial_balance
        loss_pct = (total_loss / initial_balance * 100) if initial_balance > 0 else 0
        
        print(f"初始余额: {initial_balance:.2f} USDT")
        print(f"最终余额: {final_balance:.2f} USDT")
        print(f"总亏损: {total_loss:.2f} USDT ({loss_pct:.2f}%)")
        print()
        
        # 找出余额变化的关键节点
        print("关键余额变化节点:")
        prev_balance = initial_balance
        for i, record in enumerate(account_history[::max(1, len(account_history)//20)]):  # 采样显示
            balance = record['total_balance']
            change = balance - prev_balance
            change_pct = (change / prev_balance * 100) if prev_balance > 0 else 0
            if abs(change) > 5 or abs(change_pct) > 3:  # 只显示变化较大的
                print(f"  {record['timestamp'][:19]} | 余额: {balance:.2f} USDT | 变化: {change:+.2f} USDT ({change_pct:+.2f}%) | 周期: {record['cycle']}")
            prev_balance = balance
        print()
    
    # 2. 交易统计
    print("## 2. 交易统计")
    print("-" * 80)
    if trades:
        total_trades = len(trades)
        winning_trades = [t for t in trades if t['net_pnl'] > 0]
        losing_trades = [t for t in trades if t['net_pnl'] < 0]
        
        win_rate = (len(winning_trades) / total_trades * 100) if total_trades > 0 else 0
        
        total_pnl = sum(t['net_pnl'] for t in trades)
        total_win = sum(t['net_pnl'] for t in winning_trades) if winning_trades else 0
        total_loss = sum(t['net_pnl'] for t in losing_trades) if losing_trades else 0
        
        avg_win = (total_win / len(winning_trades)) if winning_trades else 0
        avg_loss = (total_loss / len(losing_trades)) if losing_trades else 0
        
        profit_factor = abs(total_win / total_loss) if total_loss != 0 else 0
        
        print(f"总交易数: {total_trades}")
        print(f"盈利交易: {len(winning_trades)} ({win_rate:.1f}%)")
        print(f"亏损交易: {len(losing_trades)} ({100-win_rate:.1f}%)")
        print(f"总盈亏: {total_pnl:.2f} USDT")
        print(f"总盈利: {total_win:.2f} USDT")
        print(f"总亏损: {total_loss:.2f} USDT")
        print(f"平均盈利: {avg_win:.2f} USDT")
        print(f"平均亏损: {avg_loss:.2f} USDT")
        print(f"盈亏比: {profit_factor:.2f}")
        print()
        
        # 按币种统计
        print("## 3. 按币种统计")
        print("-" * 80)
        symbol_stats = defaultdict(lambda: {'trades': 0, 'wins': 0, 'total_pnl': 0, 'total_win': 0, 'total_loss': 0})
        
        for trade in trades:
            symbol = trade['symbol']
            symbol_stats[symbol]['trades'] += 1
            symbol_stats[symbol]['total_pnl'] += trade['net_pnl']
            if trade['net_pnl'] > 0:
                symbol_stats[symbol]['wins'] += 1
                symbol_stats[symbol]['total_win'] += trade['net_pnl']
            else:
                symbol_stats[symbol]['total_loss'] += trade['net_pnl']
        
        for symbol, stats in sorted(symbol_stats.items(), key=lambda x: x[1]['total_pnl']):
            win_rate = (stats['wins'] / stats['trades'] * 100) if stats['trades'] > 0 else 0
            print(f"{symbol}:")
            print(f"  交易数: {stats['trades']} | 胜率: {win_rate:.1f}% | 总盈亏: {stats['total_pnl']:.2f} USDT")
            print(f"  总盈利: {stats['total_win']:.2f} USDT | 总亏损: {stats['total_loss']:.2f} USDT")
        print()
        
        # 4. 最大亏损交易
        print("## 4. 最大亏损交易（Top 10）")
        print("-" * 80)
        losing_trades_sorted = sorted(losing_trades, key=lambda x: x['net_pnl'])[:10]
        for i, trade in enumerate(losing_trades_sorted, 1):
            print(f"{i}. {trade['symbol']} {trade['side'].upper()}")
            print(f"   开仓: {trade['open_price']:.4f} | 平仓: {trade['close_price']:.4f}")
            print(f"   盈亏: {trade['net_pnl']:.2f} USDT ({trade['net_pnl_pct']:.2f}%)")
            print(f"   杠杆: {trade['leverage']}x | 持仓时长: {trade['duration_minutes']:.1f} 分钟")
            print(f"   时间: {trade['open_time'][:19]} → {trade['close_time'][:19]}")
            print()
        
        # 5. 持仓时长分析
        print("## 5. 持仓时长分析")
        print("-" * 80)
        short_trades = [t for t in trades if t['duration_minutes'] < 15]
        medium_trades = [t for t in trades if 15 <= t['duration_minutes'] < 60]
        long_trades = [t for t in trades if t['duration_minutes'] >= 60]
        
        print(f"短持仓 (<15分钟): {len(short_trades)} 笔")
        if short_trades:
            short_pnl = sum(t['net_pnl'] for t in short_trades)
            print(f"  总盈亏: {short_pnl:.2f} USDT")
        
        print(f"中持仓 (15-60分钟): {len(medium_trades)} 笔")
        if medium_trades:
            medium_pnl = sum(t['net_pnl'] for t in medium_trades)
            print(f"  总盈亏: {medium_pnl:.2f} USDT")
        
        print(f"长持仓 (≥60分钟): {len(long_trades)} 笔")
        if long_trades:
            long_pnl = sum(t['net_pnl'] for t in long_trades)
            print(f"  总盈亏: {long_pnl:.2f} USDT")
        print()
        
        # 6. 亏损原因分析
        print("## 6. 亏损原因分析")
        print("-" * 80)
        
        # 分析亏损的主要原因
        reasons = {
            '频繁止损': len([t for t in losing_trades if t['duration_minutes'] < 15]),
            '持仓时间过长': len([t for t in losing_trades if t['duration_minutes'] > 60]),
            '高杠杆亏损': len([t for t in losing_trades if t['leverage'] >= 15]),
            '手续费侵蚀': len([t for t in trades if abs(t['pnl']) > 0 and t['net_pnl'] < 0 and abs(t['fee']) > abs(t['pnl']) * 0.3])
        }
        
        print("可能的亏损原因:")
        for reason, count in sorted(reasons.items(), key=lambda x: x[1], reverse=True):
            if count > 0:
                print(f"  - {reason}: {count} 笔交易")
        
        # 计算手续费总额
        total_fees = sum(t['fee'] for t in trades)
        print(f"\n手续费总额: {total_fees:.2f} USDT")
        print(f"手续费占总亏损比例: {abs(total_fees / total_loss * 100):.1f}%" if total_loss < 0 else "N/A")
        print()
        
        # 7. 最近10笔交易详情
        print("## 7. 最近10笔交易详情")
        print("-" * 80)
        recent_trades = sorted(trades, key=lambda x: x['close_time'], reverse=True)[:10]
        for i, trade in enumerate(recent_trades, 1):
            status = "✅" if trade['net_pnl'] > 0 else "❌"
            print(f"{i}. {status} {trade['symbol']} {trade['side'].upper()}")
            print(f"   开仓: {trade['open_price']:.4f} → 平仓: {trade['close_price']:.4f}")
            print(f"   盈亏: {trade['net_pnl']:.2f} USDT ({trade['net_pnl_pct']:+.2f}%) | 手续费: {trade['fee']:.2f} USDT")
            print(f"   杠杆: {trade['leverage']}x | 时长: {trade['duration_minutes']:.1f} 分钟")
            print(f"   时间: {trade['close_time'][:19]}")
            print()
    else:
        print("未找到交易记录")
        print()

def main():
    print("正在查找 trend_follow 策略的交易记录...")
    files = find_trend_follow_logs()
    print(f"找到 {len(files)} 个交易记录文件")
    print()
    
    if not files:
        print("未找到使用 trend_following 策略的交易记录")
        return
    
    print("正在分析交易记录...")
    account_history, trades = analyze_trades(files)
    
    print("生成分析报告...")
    generate_report(account_history, trades)
    
    # 保存详细数据到文件
    output_file = "trend_follow_analysis.json"
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump({
            'account_history': account_history,
            'trades': trades,
            'summary': {
                'total_files': len(files),
                'total_trades': len(trades),
                'initial_balance': account_history[0]['total_balance'] if account_history else 0,
                'final_balance': account_history[-1]['total_balance'] if account_history else 0
            }
        }, f, indent=2, ensure_ascii=False, default=str)
    
    print(f"\n详细数据已保存到: {output_file}")

if __name__ == '__main__':
    main()

