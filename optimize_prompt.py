#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
优化 trend_following.txt 文件，删除重复和不重要部分
"""

import re

def optimize_prompt():
    # 读取文件
    with open('prompts/trend_following.txt', 'r', encoding='utf-8') as f:
        lines = f.readlines()
    
    print(f"原始文件: {len(lines)} 行, {sum(len(l) for l in lines)} 字符")
    
    # 1. 删除重复的"周期内变化趋势的重要性"章节（只保留第一个）
    new_lines = []
    skip_until = None
    in_duplicate_section = False
    first_occurrence_kept = False
    
    i = 0
    while i < len(lines):
        line = lines[i]
        
        # 检测是否是"周期内变化趋势的重要性"章节开始
        if '**⚠️ 周期内变化趋势的重要性（关键策略）**：' in line:
            if not first_occurrence_kept:
                # 保留第一个出现
                first_occurrence_kept = True
                in_duplicate_section = True
                # 找到这个章节的结束（下一个章节或空行后跟其他内容）
                section_lines = [line]
                i += 1
                while i < len(lines):
                    next_line = lines[i]
                    # 检查是否是新的章节开始
                    if (next_line.strip().startswith('**⚠️') or 
                        next_line.strip().startswith('#') or
                        (next_line.strip() == '' and i + 1 < len(lines) and 
                         (lines[i+1].strip().startswith('**⚠️') or lines[i+1].strip().startswith('#')))):
                        break
                    section_lines.append(next_line)
                    i += 1
                new_lines.extend(section_lines)
                continue
            else:
                # 跳过所有后续重复的章节
                in_duplicate_section = True
                # 跳过直到下一个真正的章节
                i += 1
                while i < len(lines):
                    next_line = lines[i]
                    # 检查是否是新的章节开始（不是重复的周期内变化趋势）
                    if (next_line.strip().startswith('**⚠️') and 
                        '周期内变化趋势的重要性' not in next_line):
                        break
                    if (next_line.strip().startswith('#') or
                        (next_line.strip() == '' and i + 1 < len(lines) and 
                         lines[i+1].strip().startswith('#') and 
                         '周期内变化趋势的重要性' not in lines[i+1])):
                        break
                    i += 1
                continue
        
        new_lines.append(line)
        i += 1
    
    lines = new_lines
    print(f"删除重复章节后: {len(lines)} 行, {sum(len(l) for l in lines)} 字符")
    
    # 2. 精简技术指标详解部分（保留核心，删除过于详细的说明）
    # 3. 精简决策流程示例（保留核心格式，删除过长的示例）
    # 4. 删除重复的仓位控制说明（合并到一处）
    
    # 合并内容
    content = ''.join(lines)
    
    # 删除重复的仓位控制说明（保留第一个，删除后续重复）
    # 查找"⚠️ **重要提醒：仓位控制基于账户总余额百分比"的重复
    position_control_pattern = r'(⚠️ \*\*重要提醒：仓位控制基于账户总余额百分比.*?)(?=\n\n|\n# |\Z)'
    matches = list(re.finditer(position_control_pattern, content, re.DOTALL))
    if len(matches) > 1:
        # 保留第一个，删除后续
        for match in reversed(matches[1:]):
            content = content[:match.start()] + content[match.end():]
    
    # 精简决策流程示例（删除过长的示例，保留核心格式说明）
    # 查找"完整输出示例"部分，只保留简短版本
    example_pattern = r'\*\*完整输出示例.*?```\n```'
    content = re.sub(example_pattern, '**输出示例**: 参考上述格式要求', content, flags=re.DOTALL)
    
    # 删除过于详细的示例说明
    detailed_example_pattern = r'\*\*⚠️ 最终检查清单.*?不能只分析BTCUSDT就停止！\*\*'
    content = re.sub(detailed_example_pattern, '**检查清单**: 确保完成所有必要的分析步骤', content, flags=re.DOTALL)
    
    # 精简技术指标详解（保留核心说明，删除过于详细的公式和示例）
    # EMA、MACD、RSI等指标的计算方法可以简化
    ema_detail_pattern = r'\*\*计算方法\*\*：.*?初始值使用SMA.*?\n'
    content = re.sub(ema_detail_pattern, '**计算方法**: 使用指数移动平均公式计算\n', content, flags=re.DOTALL)
    
    # 删除重复的"找到好机会不容易"说明
    opportunity_pattern = r'\*\*找到好机会不容易.*?争取更大收益\*\*'
    content = re.sub(opportunity_pattern, '**重要**: 找到好机会不容易，应该勇敢使用高仓位，争取更大收益', content)
    
    # 删除重复的"必须避免"说明（合并）
    avoid_pattern = r'⚠️ \*\*必须避免的错误.*?❌ \*\*盈利空间小\*\*'
    matches = list(re.finditer(avoid_pattern, content, flags=re.DOTALL))
    if len(matches) > 1:
        for match in reversed(matches[1:]):
            content = content[:match.start()] + content[match.end():]
    
    # 保存优化后的文件
    with open('prompts/trend_following_optimized.txt', 'w', encoding='utf-8') as f:
        f.write(content)
    
    print(f"优化后文件: {len(content.split(chr(10)))} 行, {len(content)} 字符")
    print(f"减少: {sum(len(l) for l in lines) - len(content)} 字符 ({100 * (sum(len(l) for l in lines) - len(content)) / sum(len(l) for l in lines):.1f}%)")
    print("优化后的文件已保存到: prompts/trend_following_optimized.txt")

if __name__ == '__main__':
    optimize_prompt()

