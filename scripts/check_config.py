#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import sqlite3
import sys

try:
    conn = sqlite3.connect('config.db')
    conn.row_factory = sqlite3.Row
    cursor = conn.cursor()
    
    # 查询所有交易员
    cursor.execute('''
        SELECT id, name, scan_interval_minutes, trading_symbols, 
               use_coin_pool, use_oi_top, updated_at
        FROM traders
        ORDER BY updated_at DESC
    ''')
    
    rows = cursor.fetchall()
    print(f"找到 {len(rows)} 个交易员配置:\n")
    
    for row in rows:
        print(f"交易员ID: {row['id']}")
        print(f"名称: {row['name']}")
        print(f"扫描间隔: {row['scan_interval_minutes']} 分钟")
        
        symbols = row['trading_symbols'] or ''
        if symbols:
            coins = [s.strip() for s in symbols.split(',') if s.strip()]
            print(f"交易币种 ({len(coins)}个): {', '.join(coins)}")
        else:
            print("交易币种: (空 - 将使用默认币种)")
        
        print(f"使用Coin Pool: {bool(row['use_coin_pool'])}")
        print(f"使用OI Top: {bool(row['use_oi_top'])}")
        print(f"更新时间: {row['updated_at']}")
        print("-" * 60)
    
    conn.close()
except Exception as e:
    print(f"错误: {e}")
    sys.exit(1)

