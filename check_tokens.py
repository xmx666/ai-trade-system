import json

file_path = 'decision_logs/binance_admin_deepseek_1764582109/decision_20251204_101942_cycle2.json'
with open(file_path, 'r', encoding='utf-8') as f:
    data = json.load(f)

system_prompt = data.get('system_prompt', '')
input_prompt = data.get('input_prompt', '')
cot_trace = data.get('cot_trace', '')
decision_json = data.get('decision_json', '')
finish_reason = data.get('finish_reason', 'NOT_FOUND')

def estimate_tokens(text):
    return int(len(text) * 1.2) if text else 0

input_tokens = estimate_tokens(system_prompt) + estimate_tokens(input_prompt)
output_tokens = estimate_tokens(cot_trace) + estimate_tokens(decision_json)

print('=' * 80)
print('Token使用情况分析')
print('=' * 80)
print(f'输入Token:')
print(f'  system_prompt: {estimate_tokens(system_prompt):,} tokens ({len(system_prompt):,} 字符)')
print(f'  input_prompt: {estimate_tokens(input_prompt):,} tokens ({len(input_prompt):,} 字符)')
print(f'  输入总计: {input_tokens:,} tokens')
print(f'')
print(f'输出Token:')
print(f'  cot_trace: {estimate_tokens(cot_trace):,} tokens ({len(cot_trace):,} 字符)')
print(f'  decision_json: {estimate_tokens(decision_json):,} tokens ({len(decision_json):,} 字符)')
print(f'  输出总计: {output_tokens:,} tokens')
print(f'')
print(f'总Token: {input_tokens + output_tokens:,} tokens')
print(f'finish_reason: {finish_reason}')
print(f'')
if output_tokens > 8000:
    print('⚠️  警告：输出Token超过8000限制！')
elif output_tokens > 4000:
    print('⚠️  注意：输出Token超过默认4k限制')
if cot_trace.endswith('置信区间') or cot_trace.endswith('价格范围'):
    print('⚠️  警告：cot_trace被截断')
    print(f'最后100字符: {cot_trace[-100:]}')

