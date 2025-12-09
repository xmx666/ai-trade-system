#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
krnos预测服务测试脚本
用于验证krnos相关代码是否正确工作
"""

import os
import sys
import json
import time
from pathlib import Path
from datetime import datetime

# 添加项目路径
sys.path.insert(0, str(Path(__file__).parent))

from krnos_predictor import KrnosPredictor

# krnos标准预测参数
HISTORY_STEPS = 450  # 历史数据步数
PREDICTION_STEPS = 120  # 预测步数

def test_data_fetch():
    """测试1: 从真实币安数据文件加载数据"""
    print("\n" + "="*60)
    print("测试1: 加载真实币安数据")
    print("="*60)
    
    # 检查是否有Go程序生成的真实数据文件
    data_file = Path(__file__).parent / "test_data.json"
    
    if not data_file.exists():
        print("❌ 未找到真实数据文件 test_data.json")
        print("   请先运行: go run test_krnos_real.go")
        print("   这将从币安API获取真实数据")
        return None, None
    
    try:
        with open(data_file, 'r', encoding='utf-8') as f:
            data = json.load(f)
        
        price_history = data.get('price_history', [])
        volume_history = data.get('volume_history', [])
        symbol = data.get('symbol', 'UNKNOWN')
        interval = data.get('interval', 'UNKNOWN')
        history_steps = data.get('history_steps', 0)
        
        print(f"✓ 成功加载真实数据文件")
        print(f"  币种: {symbol}")
        print(f"  K线间隔: {interval}")
        print(f"  历史数据步数: {history_steps}")
        print(f"  价格数据: {len(price_history)} 条")
        print(f"  成交量数据: {len(volume_history)} 条")
        
        if len(price_history) > 0:
            print(f"  价格范围: [{min(price_history):.2f}, {max(price_history):.2f}]")
        if len(volume_history) > 0:
            print(f"  成交量范围: [{min(volume_history):.2f}, {max(volume_history):.2f}]")
        
        # 验证数据
        if len(price_history) < HISTORY_STEPS:
            print(f"⚠️  警告: 价格数据只有 {len(price_history)} 条，少于标准 {HISTORY_STEPS} 条")
        if len(volume_history) < HISTORY_STEPS:
            print(f"⚠️  警告: 成交量数据只有 {len(volume_history)} 条，少于标准 {HISTORY_STEPS} 条")
        if len(price_history) != len(volume_history):
            print(f"⚠️  警告: 价格和成交量数据长度不一致")
        
        return price_history, volume_history
        
    except Exception as e:
        print(f"❌ 加载数据文件失败: {e}")
        import traceback
        traceback.print_exc()
        return None, None

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
        return None

def test_prediction(predictor, price_history, volume_history):
    """测试3: 验证预测功能"""
    print("\n" + "="*60)
    print("测试3: 预测功能")
    print("="*60)
    
    if predictor is None:
        print("❌ 预测器未初始化，跳过测试")
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
            print(f"上次预测时间: {predictor.last_prediction_time}")
            print(f"上次预测趋势: {predictor.last_prediction.get('trend', 'unknown')}")
        
    except Exception as e:
        print(f"❌ 判断失败: {e}")

def test_data_validation():
    """测试6: 验证数据验证逻辑"""
    print("\n" + "="*60)
    print("测试6: 数据验证逻辑")
    print("="*60)
    
    # 测试数据不足的情况
    short_price = [90000.0] * 50  # 只有50条数据
    short_volume = [1000.0] * 50
    
    print(f"测试数据不足: 价格 {len(short_price)} 条, 成交量 {len(short_volume)} 条")
    if len(short_price) < 100:
        print("⚠️  数据不足，需要至少100条数据")
    
    # 测试数据充足的情况
    long_price = [90000.0] * 500  # 500条数据
    long_volume = [1000.0] * 500
    
    print(f"测试数据充足: 价格 {len(long_price)} 条, 成交量 {len(long_volume)} 条")
    if len(long_price) >= HISTORY_STEPS:
        print(f"✓ 数据充足，满足 {HISTORY_STEPS} 步历史数据要求")

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
    print("\n⚠️  重要: 请先运行 'go run test_krnos_real.go' 获取真实数据")
    print("="*60)
    
    # 测试1: 加载真实数据
    price_history, volume_history = test_data_fetch()
    
    if price_history is None or volume_history is None:
        print("\n❌ 无法加载真实数据，测试终止")
        print("   请先运行: go run test_krnos_real.go")
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
    
    # 测试6: 数据验证
    test_data_validation()
    
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
    print("1. ✓ 已使用真实币安数据（从币安API获取）")
    print("2. 需要根据etg_ai项目实际结构实现模型加载")
    print("3. 预测结果仅供参考，需要与实际市场数据对比验证")

if __name__ == '__main__':
    main()

