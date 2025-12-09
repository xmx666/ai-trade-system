#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
查询交易员配置信息
"""
import sqlite3
import json
import sys
from datetime import datetime

def format_time(minutes):
    """格式化时间"""
    if minutes < 60:
        return f"{minutes}分钟"
    else:
        hours = minutes // 60
        mins = minutes % 60
        if mins == 0:
            return f"{hours}小时"
        else:
            return f"{hours}小时{mins}分钟"

def main():
    if len(sys.argv) < 2:
        print("用法: python3 check_trader_config.py <交易员ID>")
        print("示例: python3 check_trader_config.py binance_admin_deepseek_1764582109")
        sys.exit(1)

    trader_id = sys.argv[1]
    db_path = "config.db"

    try:
        import os
        if not os.path.exists(db_path):
            print(f"❌ 数据库文件不存在: {db_path}")
            sys.exit(1)
        
        conn = sqlite3.connect(db_path)
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()

        # 查询交易员配置
        cursor.execute("""
            SELECT 
                t.id, t.user_id, t.name, t.ai_model_id, t.exchange_id,
                t.initial_balance, t.scan_interval_minutes, t.is_running,
                t.btc_eth_leverage, t.altcoin_leverage, t.trading_symbols,
                t.use_coin_pool, t.use_oi_top, t.use_inside_coins,
                t.custom_prompt, t.override_base_prompt, t.system_prompt_template,
                t.is_cross_margin, t.created_at, t.updated_at,
                a.name as ai_model_name, a.provider as ai_provider, a.enabled as ai_enabled,
                e.name as exchange_name, e.type as exchange_type, e.enabled as exchange_enabled, e.testnet
            FROM traders t
            LEFT JOIN ai_models a ON t.ai_model_id = a.id AND t.user_id = a.user_id
            LEFT JOIN exchanges e ON t.exchange_id = e.id AND t.user_id = e.user_id
            WHERE t.id = ?
        """, (trader_id,))

        row = cursor.fetchone()
        if not row:
            print(f"❌ 未找到交易员: {trader_id}")
            sys.exit(1)

        # 解析交易币种
        trading_symbols = row['trading_symbols'] or ''
        trading_coins = []
        if trading_symbols:
            trading_coins = [s.strip() for s in trading_symbols.split(',') if s.strip()]

        # 如果没有指定交易币种，查询默认币种
        if not trading_coins:
            cursor.execute("SELECT value FROM system_config WHERE key = 'default_coins'")
            default_coins_row = cursor.fetchone()
            if default_coins_row and default_coins_row['value']:
                try:
                    trading_coins = json.loads(default_coins_row['value'])
                except:
                    pass

        # 打印配置信息
        print("╔════════════════════════════════════════════════════════════╗")
        print("║           交易员配置信息                                   ║")
        print("╚════════════════════════════════════════════════════════════╝")
        print()

        print(f"📋 交易员ID: {row['id']}")
        print(f"📝 交易员名称: {row['name']}")
        print(f"👤 用户ID: {row['user_id']}")
        print()

        print("🤖 AI模型配置:")
        print(f"   - 模型ID: {row['ai_model_id']}")
        if row['ai_model_name']:
            print(f"   - 模型名称: {row['ai_model_name']}")
        if row['ai_provider']:
            print(f"   - 提供商: {row['ai_provider']}")
        print(f"   - 是否启用: {bool(row['ai_enabled'])}")
        print()

        print("🏦 交易所配置:")
        print(f"   - 交易所ID: {row['exchange_id']}")
        if row['exchange_name']:
            print(f"   - 交易所名称: {row['exchange_name']}")
        if row['exchange_type']:
            print(f"   - 交易所类型: {row['exchange_type']}")
        print(f"   - 是否启用: {bool(row['exchange_enabled'])}")
        print(f"   - 测试网: {bool(row['testnet'])}")
        print()

        print("⏰ 时间配置:")
        scan_interval = row['scan_interval_minutes'] or 3
        print(f"   - 扫描间隔: {scan_interval} 分钟")
        print(f"   - 扫描间隔: {format_time(scan_interval)}")
        print(f"   - 扫描间隔: {scan_interval * 60} 秒")
        print()

        print("💰 账户配置:")
        print(f"   - 初始余额: {row['initial_balance']:.2f} USDT")
        print(f"   - 全仓模式: {bool(row['is_cross_margin'])}")
        print()

        print("⚖️  杠杆配置:")
        print(f"   - BTC/ETH杠杆: {row['btc_eth_leverage'] or 5}x")
        print(f"   - 山寨币杠杆: {row['altcoin_leverage'] or 5}x")
        print()

        print("🪙 币种配置:")
        if trading_coins:
            print(f"   - 交易币种数量: {len(trading_coins)} 个")
            print(f"   - 交易币种列表:")
            for i, coin in enumerate(trading_coins, 1):
                print(f"     {i}. {coin}")
        else:
            print(f"   - 未指定交易币种，将使用默认币种或信号源币种")
        print()

        print("📡 信号源配置:")
        print(f"   - 使用Coin Pool: {bool(row['use_coin_pool'])}")
        print(f"   - 使用OI Top: {bool(row['use_oi_top'])}")
        print(f"   - 使用内置评分: {bool(row['use_inside_coins'])}")
        print()

        print("📝 提示词配置:")
        print(f"   - 系统提示词模板: {row['system_prompt_template'] or 'default'}")
        custom_prompt = row['custom_prompt'] or ''
        if custom_prompt:
            if len(custom_prompt) > 100:
                print(f"   - 自定义提示词: {custom_prompt[:100]}...")
            else:
                print(f"   - 自定义提示词: {custom_prompt}")
        else:
            print(f"   - 自定义提示词: 无")
        print(f"   - 覆盖基础提示词: {bool(row['override_base_prompt'])}")
        print()

        print("🔄 运行状态:")
        print(f"   - 是否运行中: {bool(row['is_running'])}")
        if row['created_at']:
            print(f"   - 创建时间: {row['created_at']}")
        if row['updated_at']:
            print(f"   - 更新时间: {row['updated_at']}")
        print()

        conn.close()

    except sqlite3.Error as e:
        print(f"❌ 数据库错误: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"❌ 错误: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()

