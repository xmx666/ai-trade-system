#!/usr/bin/env python3
import json
import sys
import os

# 从Docker容器读取最新的决策日志
log_file = sys.argv[1] if len(sys.argv) > 1 else None

if not log_file:
    print("用法: python3 check_krnos_in_logs.py <日志文件路径>")
    sys.exit(1)

try:
    with open(log_file, 'r', encoding='utf-8') as f:
        data = json.load(f)
    
    print("=" * 60)
    print(f"检查文件: {log_file}")
    print("=" * 60)
    
    # 检查各个字段
    print("\n1. 检查 input_prompt:")
    input_prompt = data.get('input_prompt', '')
    print(f"   长度: {len(input_prompt)}")
    print(f"   包含 'krnos': {'krnos' in input_prompt.lower()}")
    print(f"   包含 'kronos': {'kronos' in input_prompt.lower()}")
    print(f"   包含 '预测': {'预测' in input_prompt}")
    
    if 'krnos' in input_prompt.lower() or 'kronos' in input_prompt.lower():
        idx = input_prompt.lower().find('krnos')
        if idx < 0:
            idx = input_prompt.lower().find('kronos')
        print(f"\n   找到位置: {idx}")
        print(f"   上下文 (前后300字符):")
        print("   " + "-" * 56)
        print("   " + input_prompt[max(0, idx-150):idx+300])
        print("   " + "-" * 56)
    
    print("\n2. 检查 user_prompt:")
    user_prompt = data.get('user_prompt', '')
    print(f"   长度: {len(user_prompt)}")
    print(f"   包含 'krnos': {'krnos' in user_prompt.lower()}")
    
    print("\n3. 检查 context.PredictionMap:")
    context = data.get('context', {})
    if isinstance(context, dict):
        pred_map = context.get('PredictionMap', {})
        print(f"   PredictionMap keys: {list(pred_map.keys())}")
        if pred_map:
            for symbol, pred in pred_map.items():
                print(f"   {symbol}: {pred}")
    else:
        print("   context 不是字典类型")
    
    print("\n4. 检查系统提示词中是否提到krnos:")
    system_prompt = data.get('system_prompt', '')
    print(f"   包含 'krnos': {'krnos' in system_prompt.lower()}")
    print(f"   包含 'kronos': {'kronos' in system_prompt.lower()}")
    
except Exception as e:
    print(f"错误: {e}", file=sys.stderr)
    import traceback
    traceback.print_exc()

