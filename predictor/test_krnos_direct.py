#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
krnos预测服务测试脚本（直接从币安API获取真实数据）
用于验证krnos相关代码是否正确工作
"""

import os
import sys
import json
import time
import requests
from pathlib import Path
from datetime import datetime

# 添加项目路径
sys.path.insert(0, str(Path(__file__).parent))

from krnos_predictor import KrnosPredictor

# krnos标准预测参数
HISTORY_STEPS = 450  # 历史数据步数
PREDICTION_STEPS = 120  # 预测步数

# 币安API配置
BINANCE_API_BASE = "https://fapi.binance.com"

# 代理配置（从环境变量或.env文件读取，与nofx项目保持一致）
def get_proxy_config():
    """获取代理配置，与nofx项目保持一致"""
    # 首先从环境变量读取
    http_proxy = os.environ.get('HTTP_PROXY') or os.environ.get('http_proxy')
    https_proxy = os.environ.get('HTTPS_PROXY') or os.environ.get('https_proxy')
    
    # 如果环境变量中没有，尝试从.env文件读取
    if not http_proxy or not https_proxy:
        env_file = Path(__file__).parent.parent / ".env"
        if env_file.exists():
            try:
                with open(env_file, 'r', encoding='utf-8') as f:
                    for line in f:
                        line = line.strip()
                        if line.startswith('HTTP_PROXY=') and not http_proxy:
                            http_proxy = line.split('=', 1)[1].strip().strip('"').strip("'")
                        elif line.startswith('HTTPS_PROXY=') and not https_proxy:
                            https_proxy = line.split('=', 1)[1].strip().strip('"').strip("'")
            except Exception as e:
                print(f"⚠️  读取.env文件失败: {e}")
    
    proxies = {}
    if http_proxy:
        proxies['http'] = http_proxy
    if https_proxy:
        proxies['https'] = https_proxy
    
    return proxies if proxies else None

def fetch_real_binance_data(symbol="BTCUSDT", interval="3m", limit=450):
    """直接从币安API获取真实K线数据"""
    print(f"\n从币安API获取真实数据...")
    print(f"  币种: {symbol}")
    print(f"  K线间隔: {interval}")
    print(f"  数据条数: {limit}")
    
    url = f"{BINANCE_API_BASE}/fapi/v1/klines"
    params = {
        "symbol": symbol,
        "interval": interval,
        "limit": limit
    }
    
    try:
        # 获取代理配置（与nofx项目保持一致）
        proxies = get_proxy_config()
        if proxies:
            print(f"  使用代理: {proxies.get('https', proxies.get('http', 'N/A'))}")
        
        response = requests.get(url, params=params, timeout=30, proxies=proxies)
        response.raise_for_status()
        klines = response.json()
        
        if not klines:
            raise ValueError("币安API返回空数据")
        
        print(f"✓ 成功获取 {len(klines)} 条K线数据")
        
        # 提取价格和成交量
        price_history = []
        volume_history = []
        
        for kline in klines:
            # 币安K线数据格式: [开盘时间, 开盘价, 最高价, 最低价, 收盘价, 成交量, ...]
            close_price = float(kline[4])  # 收盘价
            volume = float(kline[5])       # 成交量
            
            price_history.append(close_price)
            volume_history.append(volume)
        
        if len(price_history) > 0:
            print(f"  价格范围: [{min(price_history):.2f}, {max(price_history):.2f}]")
            print(f"  成交量范围: [{min(volume_history):.2f}, {max(volume_history):.2f}]")
            print(f"  最新价格: {price_history[-1]:.2f}")
            print(f"  最新成交量: {volume_history[-1]:.2f}")
        
        return price_history, volume_history
        
    except requests.exceptions.RequestException as e:
        print(f"❌ 网络请求失败: {e}")
        return None, None
    except Exception as e:
        print(f"❌ 获取数据失败: {e}")
        import traceback
        traceback.print_exc()
        return None, None

def test_data_fetch():
    """测试1: 从币安API获取真实数据"""
    print("\n" + "="*60)
    print("测试1: 从币安API获取真实数据")
    print("="*60)
    
    price_history, volume_history = fetch_real_binance_data(
        symbol="BTCUSDT",
        interval="3m",
        limit=HISTORY_STEPS
    )
    
    if price_history is None or volume_history is None:
        print("❌ 无法获取真实数据")
        return None, None
    
    if len(price_history) < 100:
        print(f"⚠️  警告: 数据只有 {len(price_history)} 条，少于标准 {HISTORY_STEPS} 条")
    
    if len(price_history) != len(volume_history):
        print(f"⚠️  警告: 价格和成交量数据长度不一致")
    
    return price_history, volume_history

def test_predictor_initialization():
    """测试2: 验证预测器初始化"""
    print("\n" + "="*60)
    print("测试2: 预测器初始化")
    print("="*60)
    
    try:
        predictor = KrnosPredictor()
        print("✓ 预测器初始化成功")
        print(f"  缓存目录: {predictor.cache_dir}")
        print(f"  预测间隔: {predictor.prediction_interval / 60} 分钟")
        return predictor
    except Exception as e:
        print(f"❌ 预测器初始化失败: {e}")
        import traceback
        traceback.print_exc()
        return None

def test_prediction(predictor, price_history, volume_history):
    """测试3: 验证预测功能"""
    print("\n" + "="*60)
    print("测试3: 预测功能")
    print("="*60)
    
    if predictor is None:
        print("❌ 预测器未初始化，跳过测试")
        return None
    
    if price_history is None or volume_history is None:
        print("❌ 数据未加载，跳过测试")
        return None
    
    try:
        print(f"开始预测: 历史数据 {len(price_history)} 步, 预测 {PREDICTION_STEPS} 步")
        print(f"蒙特卡罗模拟次数: 10次（限制）")
        
        start_time = time.time()
        result = predictor.predict_with_monte_carlo(
            price_history=price_history,
            volume_history=volume_history,
            n_simulations=10,  # 限制在10次以内
            prediction_horizon=PREDICTION_STEPS  # 预测120步
        )
        elapsed_time = time.time() - start_time
        
        print(f"✓ 预测完成，耗时: {elapsed_time:.2f} 秒")
        print(f"\n预测结果:")
        print(f"  趋势: {result.get('trend', 'unknown')}")
        print(f"  趋势强度: {result.get('trend_strength', 0):.4f}")
        print(f"  预测步数: {result.get('prediction_horizon', 0)}")
        print(f"  模拟次数: {result.get('n_simulations', 0)}")
        
        if 'price_range' in result:
            price_range = result['price_range']
            print(f"  价格范围: [{price_range[0]:.2f}, {price_range[1]:.2f}]")
        
        if 'mean_prediction' in result:
            mean_pred = result['mean_prediction']
            print(f"  预测曲线长度: {len(mean_pred)}")
            if len(mean_pred) > 0:
                print(f"  起始预测价格: {mean_pred[0]:.2f}")
                print(f"  结束预测价格: {mean_pred[-1]:.2f}")
                price_change = ((mean_pred[-1] - mean_pred[0]) / mean_pred[0]) * 100
                print(f"  预测价格变化: {price_change:+.2f}%")
        
        if 'confidence_interval_lower' in result and 'confidence_interval_upper' in result:
            lower = result['confidence_interval_lower']
            upper = result['confidence_interval_upper']
            if len(lower) > 0 and len(upper) > 0:
                print(f"  置信区间: [{lower[0]:.2f}, {upper[-1]:.2f}]")
        
        return result
        
    except Exception as e:
        print(f"❌ 预测失败: {e}")
        import traceback
        traceback.print_exc()
        return None

def test_prediction_cache(predictor):
    """测试4: 验证预测缓存功能"""
    print("\n" + "="*60)
    print("测试4: 预测缓存功能")
    print("="*60)
    
    if predictor is None:
        print("❌ 预测器未初始化，跳过测试")
        return
    
    try:
        latest = predictor.get_latest_prediction()
        if latest:
            print("✓ 获取最新预测结果成功")
            print(f"  时间戳: {latest.get('timestamp', 'unknown')}")
            print(f"  趋势: {latest.get('trend', 'unknown')}")
        else:
            print("⚠️  暂无缓存的预测结果")
    except Exception as e:
        print(f"❌ 获取缓存失败: {e}")

def test_should_repredict(predictor):
    """测试5: 验证重新预测判断逻辑"""
    print("\n" + "="*60)
    print("测试5: 重新预测判断逻辑")
    print("="*60)
    
    if predictor is None:
        print("❌ 预测器未初始化，跳过测试")
        return
    
    try:
        current_price = 91000.0
        current_trend = "up"
        
        should_repredict = predictor.should_repredict(current_price, current_trend)
        print(f"当前价格: {current_price}")
        print(f"当前趋势: {current_trend}")
        print(f"是否需要重新预测: {should_repredict}")
        
        if predictor.last_prediction:
            if predictor.last_prediction_time:
                print(f"上次预测时间: {datetime.fromtimestamp(predictor.last_prediction_time)}")
            print(f"上次预测趋势: {predictor.last_prediction.get('trend', 'unknown')}")
        
    except Exception as e:
        print(f"❌ 判断失败: {e}")

def main():
    """主测试函数"""
    print("\n" + "="*60)
    print("krnos预测服务测试（使用真实币安数据）")
    print("="*60)
    print(f"标准预测参数:")
    print(f"  历史数据步数: {HISTORY_STEPS}")
    print(f"  预测步数: {PREDICTION_STEPS}")
    print(f"  蒙特卡罗模拟: 10次（限制）")
    print("="*60)
    
    # 测试1: 从币安获取真实数据
    price_history, volume_history = test_data_fetch()
    
    if price_history is None or volume_history is None:
        print("\n❌ 无法获取真实数据，测试终止")
        print("   请检查网络连接和币安API可访问性")
        return
    
    # 验证数据是否足够
    if len(price_history) < 100 or len(volume_history) < 100:
        print(f"\n❌ 数据不足: 价格 {len(price_history)} 条, 成交量 {len(volume_history)} 条")
        print("   至少需要 100 条数据")
        return
    
    # 测试2: 预测器初始化
    predictor = test_predictor_initialization()
    if predictor is None:
        print("\n❌ 预测器初始化失败，测试终止")
        return
    
    # 测试3: 预测功能
    result = test_prediction(predictor, price_history, volume_history)
    
    # 测试4: 预测缓存
    test_prediction_cache(predictor)
    
    # 测试5: 重新预测判断
    test_should_repredict(predictor)
    
    # 总结
    print("\n" + "="*60)
    print("测试总结")
    print("="*60)
    
    if result:
        print("✓ 核心功能测试通过")
        print(f"  预测趋势: {result.get('trend', 'unknown')}")
        print(f"  预测步数: {len(result.get('mean_prediction', []))}")
        print(f"  使用真实币安数据: ✓")
    else:
        print("❌ 核心功能测试失败")
    
    print("\n注意:")
    print("1. ✓ 已使用真实币安数据（从币安API直接获取）")
    print("2. 需要根据etg_ai项目实际结构实现模型加载")
    print("3. 预测结果仅供参考，需要与实际市场数据对比验证")

if __name__ == '__main__':
    main()

