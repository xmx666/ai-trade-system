#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
优化 trend_following.txt 文件，删除重复和不重要部分
"""

def optimize_prompt():
    # 读取文件
    with open('prompts/trend_following.txt', 'r', encoding='utf-8') as f:
        lines = f.readlines()
    
    original_len = sum(len(l) for l in lines)
    print(f"原始文件: {len(lines)} 行, {original_len} 字符")
    
    # 1. 删除重复的"周期内变化趋势的重要性"章节（只保留第一个）
    new_lines = []
    first_occurrence_kept = False
    skip_duplicate = False
    duplicate_section_depth = 0
    
    i = 0
    while i < len(lines):
        line = lines[i]
        
        # 检测是否是"周期内变化趋势的重要性"章节开始
        if '**⚠️ 周期内变化趋势的重要性（关键策略）**：' in line:
            if not first_occurrence_kept:
                # 保留第一个出现
                first_occurrence_kept = True
                skip_duplicate = False
                new_lines.append(line)
                i += 1
                # 继续添加这个章节的内容，直到下一个章节
                while i < len(lines):
                    next_line = lines[i]
                    # 检查是否是新的重复章节开始
                    if '**⚠️ 周期内变化趋势的重要性（关键策略）**：' in next_line:
                        skip_duplicate = True
                        break
                    # 检查是否是其他章节开始
                    if (next_line.strip().startswith('#') or 
                        (next_line.strip().startswith('**⚠️') and '周期内变化趋势的重要性' not in next_line)):
                        break
                    new_lines.append(next_line)
                    i += 1
                continue
            else:
                # 跳过所有后续重复的章节
                skip_duplicate = True
                i += 1
                # 跳过直到下一个真正的章节
                while i < len(lines):
                    next_line = lines[i]
                    # 检查是否是新的重复章节开始
                    if '**⚠️ 周期内变化趋势的重要性（关键策略）**：' in next_line:
                        i += 1
                        continue
                    # 检查是否是其他章节开始
                    if (next_line.strip().startswith('#') or 
                        (next_line.strip().startswith('**⚠️') and '周期内变化趋势的重要性' not in next_line)):
                        skip_duplicate = False
                        break
                    i += 1
                continue
        
        if not skip_duplicate:
            new_lines.append(line)
        i += 1
    
    lines = new_lines
    after_dup_removal = sum(len(l) for l in lines)
    print(f"删除重复章节后: {len(lines)} 行, {after_dup_removal} 字符 (减少 {original_len - after_dup_removal} 字符)")
    
    # 2. 合并内容并进一步优化
    content = ''.join(lines)
    
    # 删除重复的"找到好机会不容易"说明（只保留一处）
    opportunity_pattern = r'(\*\*找到好机会不容易.*?争取更大收益\*\*)'
    matches = list(re.finditer(opportunity_pattern, content, re.DOTALL))
    if len(matches) > 1:
        # 保留第一个，删除后续
        for match in reversed(matches[1:]):
            content = content[:match.start()] + content[match.end():]
    
    # 精简过长的决策流程示例（保留核心，删除详细示例）
    # 查找"完整输出示例"部分
    import re
    example_start = content.find('**完整输出示例（成功格式）**：')
    if example_start != -1:
        # 找到示例结束位置（下一个```或章节）
        example_end = content.find('```\n\n⚠️ **关键要求**：', example_start)
        if example_end != -1:
            # 替换为简短说明
            content = content[:example_start] + '**输出示例**: 参考上述格式要求，输出思维链分析和JSON数组\n\n⚠️ **关键要求**：' + content[example_end + len('```\n\n⚠️ **关键要求**：'):]
    
    # 删除过于详细的检查清单说明
    checklist_pattern = r'\*\*⚠️ 最终检查清单.*?不能只分析BTCUSDT就停止！\*\*'
    content = re.sub(checklist_pattern, '**检查清单**: 确保完成所有必要的分析步骤', content, flags=re.DOTALL)
    
    # 精简技术指标的计算方法说明（保留核心，删除详细公式）
    # EMA计算方法
    ema_calc = re.search(r'\*\*计算方法\*\*：.*?初始值使用SMA.*?\n', content, re.DOTALL)
    if ema_calc:
        content = content[:ema_calc.start()] + '**计算方法**: 使用指数移动平均公式计算\n' + content[ema_calc.end():]
    
    # 保存优化后的文件
    with open('prompts/trend_following_optimized.txt', 'w', encoding='utf-8') as f:
        f.write(content)
    
    final_len = len(content)
    print(f"优化后文件: {len(content.split(chr(10)))} 行, {final_len} 字符")
    print(f"总减少: {original_len - final_len} 字符 ({100 * (original_len - final_len) / original_len:.1f}%)")
    print("优化后的文件已保存到: prompts/trend_following_optimized.txt")

if __name__ == '__main__':
    import re
    optimize_prompt()

