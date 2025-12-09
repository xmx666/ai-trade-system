#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
分析历史决策文件的Token使用情况
"""

import json
import glob
import os

def estimate_tokens(text):
    """
    粗略估算token数量：
    - 中文：约1.5 token/字符
    - 英文：约0.25 token/字符
    - 数字和符号：约0.5 token/字符
    简化：按平均1.2 token/字符估算（保守）
    """
    if not text:
        return 0
    # 简单估算：字符数 * 1.2（保守）
    return int(len(text) * 1.2)

# 查找最新的决策文件
decision_files = sorted(glob.glob('decision_logs/binance_admin_deepseek_1764582109/decision_*.json'), reverse=True)

if not decision_files:
    print("未找到决策文件")
    exit(1)

# 分析最新的几个文件
print("=" * 80)
print("分析历史决策文件的Token使用情况")
print("=" * 80)

for file_path in decision_files[:3]:  # 分析最近3个文件
    print(f"\n文件: {os.path.basename(file_path)}")
    print("-" * 80)
    
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            data = json.load(f)
        
        # 提取输入
        system_prompt = data.get('system_prompt', '')
        input_prompt = data.get('input_prompt', '')
        
        # 提取输出
        cot_trace = data.get('cot_trace', '')
        decision_json = data.get('decision_json', '')
        
        # 计算token数量
        input_tokens = estimate_tokens(system_prompt) + estimate_tokens(input_prompt)
        output_tokens = estimate_tokens(cot_trace) + estimate_tokens(decision_json)
        total_tokens = input_tokens + output_tokens
        
        # 检查finish_reason
        finish_reason = data.get('finish_reason', 'NOT_FOUND')
        
        print(f"输入Token估算:")
        print(f"  system_prompt: {estimate_tokens(system_prompt):,} tokens ({len(system_prompt):,} 字符)")
        print(f"  input_prompt: {estimate_tokens(input_prompt):,} tokens ({len(input_prompt):,} 字符)")
        print(f"  输入总计: {input_tokens:,} tokens")
        
        print(f"\n输出Token估算:")
        print(f"  cot_trace: {estimate_tokens(cot_trace):,} tokens ({len(cot_trace):,} 字符)")
        print(f"  decision_json: {estimate_tokens(decision_json):,} tokens ({len(decision_json):,} 字符)")
        print(f"  输出总计: {output_tokens:,} tokens")
        
        print(f"\n总Token估算: {total_tokens:,} tokens")
        print(f"finish_reason: {finish_reason}")
        
        # 检查是否超过限制
        if output_tokens > 8000:
            print(f"⚠️  警告：输出Token ({output_tokens:,}) 超过8000限制！")
        elif output_tokens > 4000:
            print(f"⚠️  注意：输出Token ({output_tokens:,}) 超过默认4k限制，但未超过8k最大限制")
        
        # 检查是否被截断
        if cot_trace and (cot_trace.endswith('置信区间') or cot_trace.endswith('价格范围') or len(cot_trace) < 1000):
            print(f"⚠️  警告：cot_trace可能被截断（长度: {len(cot_trace)} 字符）")
            print(f"  最后100字符: {cot_trace[-100:]}")
        
        if finish_reason == 'length':
            print(f"⚠️  确认：finish_reason='length'，确实因token限制被截断")
        elif finish_reason == 'NOT_FOUND':
            print(f"⚠️  注意：finish_reason字段不存在（可能是修改前的文件）")
        
    except Exception as e:
        print(f"错误：{e}")

print("\n" + "=" * 80)

