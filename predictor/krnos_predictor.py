#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
krnos模型预测服务
使用etg_ai项目的krnos模型进行市场趋势预测
支持蒙特卡罗方法进行范围预测
"""

import os
import sys
import json
import time
try:
    import numpy as np
except ImportError:
    # 如果numpy未安装，尝试使用miniforge3的Python
    import subprocess
    miniforge_python = os.path.expanduser("~/miniforge3/bin/python3")
    if os.path.exists(miniforge_python):
        sys.executable = miniforge_python
        import numpy as np
    else:
        raise ImportError("numpy未安装，请运行: pip install numpy")

from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Tuple, Optional
import logging

# 配置日志（默认输出到stderr，避免污染stdout的JSON输出）
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    stream=sys.stderr  # 默认输出到stderr
)
logger = logging.getLogger(__name__)

# 尝试导入Kronos模型相关库
KRONOS_AVAILABLE = False
torch = None  # 全局变量，如果导入失败则为None
try:
    import torch
    from transformers import AutoModel, AutoTokenizer
    KRONOS_AVAILABLE = True
    logger.info("Kronos模型库导入成功")
except ImportError as e:
    logger.error(f"无法导入Kronos模型库: {e}")
    logger.error("请安装: pip install torch transformers huggingface_hub")
    KRONOS_AVAILABLE = False
    torch = None
except OSError as e:
    # Alpine Linux兼容性问题：torch在musl libc上可能无法加载
    logger.warning(f"PyTorch加载失败（可能是Alpine Linux兼容性问题）: {e}")
    logger.warning("krnos预测功能将不可用，但系统将继续运行")
    KRONOS_AVAILABLE = False
    torch = None
except Exception as e:
    logger.error(f"导入Kronos模型库时发生未知错误: {e}")
    logger.error("krnos预测功能将不可用，但系统将继续运行")
    KRONOS_AVAILABLE = False
    torch = None

# 尝试导入Kronos模型（如果GitHub仓库可用）
KRONOS_GITHUB_AVAILABLE = False
try:
    # 优先尝试从Kronos官方项目导入
    # 注意：__file__是 /app/predictor/krnos_predictor.py，所以 parent.parent 是 /app
    kronos_path = Path(__file__).parent.parent / "Kronos"
    if kronos_path.exists() and (kronos_path / "model").exists():
        sys.path.insert(0, str(kronos_path))
        try:
            from model import Kronos, KronosTokenizer, KronosPredictor as KronosPredictorClass
            KRONOS_GITHUB_AVAILABLE = True
            logger.info(f"成功从Kronos官方项目导入: {kronos_path}")
        except ImportError as e:
            logger.warning(f"从Kronos官方项目导入失败: {e}，尝试etg_ai项目")
            KRONOS_GITHUB_AVAILABLE = False
    else:
        KRONOS_GITHUB_AVAILABLE = False
    
    # 如果Kronos项目不可用，尝试etg_ai项目
    if not KRONOS_GITHUB_AVAILABLE:
        etg_ai_path = Path(__file__).parent.parent / "etg_ai"
        if etg_ai_path.exists():
            sys.path.insert(0, str(etg_ai_path))
            try:
                from model import Kronos, KronosTokenizer
                # 尝试导入KronosPredictor，如果不存在则使用简化版
                try:
                    from predictor import KronosPredictor as KronosPredictorClass
                except ImportError:
                    KronosPredictorClass = None  # 将在_load_model中处理
                KRONOS_GITHUB_AVAILABLE = True
                logger.info(f"成功从etg_ai项目导入Kronos: {etg_ai_path}")
            except ImportError as e:
                KRONOS_GITHUB_AVAILABLE = False
                logger.warning(f"etg_ai项目存在但无法导入Kronos模型: {e}，将使用Hugging Face版本")
        else:
            KRONOS_GITHUB_AVAILABLE = False
            logger.info("未找到Kronos项目或etg_ai项目，将使用Hugging Face版本")
except Exception as e:
    KRONOS_GITHUB_AVAILABLE = False
    logger.warning(f"检查Kronos项目时出错: {e}，将使用Hugging Face版本")


class KrnosPredictor:
    """krnos模型预测器"""
    
    def __init__(self, model_path: Optional[str] = None, cache_dir: str = "./predictor_cache", device: Optional[str] = None):
        """
        初始化预测器
        
        Args:
            model_path: 模型文件路径（本地路径或Hugging Face模型ID）
            cache_dir: 缓存目录，用于存储预测结果
            device: 设备（'cuda:0', 'cpu'等），如果为None则自动选择
        """
        self.model_path = model_path or "NeoQuasar/Kronos-base"
        self.cache_dir = Path(cache_dir)
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        
        # 预测结果缓存
        self.last_prediction = None
        self.last_prediction_time = None
        self.prediction_interval = 30 * 60  # 30分钟（秒）
        
        # 设备选择（优先使用GPU，但如果torch不可用则使用CPU）
        if device is None:
            if torch is not None and hasattr(torch, 'cuda') and torch.cuda.is_available():
                self.device = "cuda:0"
            else:
                self.device = "cpu"
        else:
            self.device = device
        
        # 模型和tokenizer
        self.model = None
        self.tokenizer = None
        self.predictor = None
        
        # 模型加载
        self._load_model()
    
    def _load_model(self):
        """加载Kronos模型（必须成功，否则禁止预测）"""
        if not KRONOS_AVAILABLE:
            error_msg = "Kronos模型库不可用（可能是Alpine Linux兼容性问题），预测功能将不可用"
            logger.warning(error_msg)
            self.model = None
            return
        
        try:
            # 优先使用GitHub版本的Kronos（如果可用）
            if KRONOS_GITHUB_AVAILABLE:
                logger.info("使用GitHub版本的Kronos模型")
                self._load_model_from_github()
            else:
                # 使用Hugging Face版本（简化实现）
                logger.info("使用Hugging Face版本的Kronos模型（简化实现）")
                self._load_model_from_huggingface_simple()
            
            # 如果模型加载失败，直接失败，不使用任何备用方案
            if self.model is None or self.predictor is None:
                error_msg = "Kronos模型加载失败，禁止使用备用预测方案以避免误导交易系统"
                logger.error(error_msg)
                self.model = None
                self.tokenizer = None
                self.predictor = None
                raise RuntimeError(error_msg)
            
            logger.info(f"✓ Kronos模型加载成功，设备: {self.device}")
            
        except Exception as e:
            error_msg = f"模型加载失败: {e}，预测功能已禁用"
            logger.error(error_msg, exc_info=True)
            self.model = None
            self.tokenizer = None
            self.predictor = None
    
    def _load_model_from_github(self):
        """从GitHub项目加载Kronos模型"""
        try:
            # 检查本地模型路径
            model_dir = Path(__file__).parent.parent / "models" / "kronos"
            local_model_path = model_dir / "Kronos-base"
            local_tokenizer_path = model_dir / "Kronos-Tokenizer-base"
            
            # 优先使用本地模型（如果存在且完整）
            if local_model_path.exists() and local_tokenizer_path.exists():
                # 检查关键文件是否存在
                model_safetensors = local_model_path / "model.safetensors"
                tokenizer_safetensors = local_tokenizer_path / "model.safetensors"
                
                if model_safetensors.exists() and tokenizer_safetensors.exists():
                    logger.info(f"✓ 使用本地模型: {local_model_path}")
                    try:
                        # 尝试从本地加载
                        tokenizer = KronosTokenizer.from_pretrained(str(local_tokenizer_path))
                        model = Kronos.from_pretrained(str(local_model_path))
                        logger.info("✓ 本地模型加载成功")
                    except Exception as e:
                        logger.warning(f"本地模型加载失败: {e}，尝试从Hugging Face加载...")
                        # 如果本地加载失败，fallback到在线加载
                        tokenizer = KronosTokenizer.from_pretrained("NeoQuasar/Kronos-Tokenizer-base")
                        model = Kronos.from_pretrained("NeoQuasar/Kronos-base")
                else:
                    logger.info("本地模型文件不完整，从Hugging Face加载...")
                    tokenizer = KronosTokenizer.from_pretrained("NeoQuasar/Kronos-Tokenizer-base")
                    model = Kronos.from_pretrained("NeoQuasar/Kronos-base")
            else:
                logger.info("本地模型不存在，从Hugging Face加载...")
                tokenizer = KronosTokenizer.from_pretrained("NeoQuasar/Kronos-Tokenizer-base")
                model = Kronos.from_pretrained("NeoQuasar/Kronos-base")
            
            # 创建预测器（如果KronosPredictorClass可用）
            # 尝试从Kronos官方项目导入KronosPredictor
            try:
                # 尝试从Kronos项目导入
                # 注意：__file__是 /app/predictor/krnos_predictor.py，所以 parent.parent 是 /app
                kronos_path = Path(__file__).parent.parent / "Kronos"
                if kronos_path.exists():
                    sys.path.insert(0, str(kronos_path))
                    from model import KronosPredictor
                    self.predictor = KronosPredictor(
                        model, 
                        tokenizer, 
                        device=self.device, 
                        max_context=512
                    )
                    logger.info("✓ 使用Kronos官方KronosPredictor")
                else:
                    raise ImportError("Kronos项目不存在")
            except (ImportError, NameError) as e:
                # 如果predictor不可用，直接失败，禁止使用备用方案
                error_msg = f"KronosPredictor类不可用: {e}，禁止使用备用预测方案以避免误导交易系统"
                logger.error(error_msg)
                raise RuntimeError(error_msg)
            
            self.model = model
            self.tokenizer = tokenizer
            
        except Exception as e:
            logger.error(f"从GitHub项目加载模型失败: {e}")
            raise
    
    def _load_model_from_huggingface_simple(self):
        """从Hugging Face加载Kronos模型（简化实现，优先使用本地）"""
        try:
            from transformers import AutoModel, AutoTokenizer
            
            # 检查本地模型路径
            model_dir = Path(__file__).parent.parent / "models" / "kronos"
            local_model_path = model_dir / "Kronos-base"
            local_tokenizer_path = model_dir / "Kronos-Tokenizer-base"
            
            # 优先使用本地模型
            if local_model_path.exists() and local_tokenizer_path.exists():
                model_safetensors = local_model_path / "model.safetensors"
                tokenizer_safetensors = local_tokenizer_path / "model.safetensors"
                
                if model_safetensors.exists() and tokenizer_safetensors.exists():
                    logger.info(f"✓ 使用本地模型: {local_model_path}")
                    model_path = str(local_model_path)
                    tokenizer_path = str(local_tokenizer_path)
                else:
                    logger.info("本地模型文件不完整，从Hugging Face加载...")
                    model_path = "NeoQuasar/Kronos-base"
                    tokenizer_path = "NeoQuasar/Kronos-Tokenizer-base"
            else:
                logger.info("本地模型不存在，从Hugging Face加载...")
                model_path = "NeoQuasar/Kronos-base"
                tokenizer_path = "NeoQuasar/Kronos-Tokenizer-base"
            
            # 尝试从etg_ai导入（如果可用）
            etg_ai_path = Path(__file__).parent.parent.parent / "etg_ai"
            if etg_ai_path.exists():
                sys.path.insert(0, str(etg_ai_path))
                try:
                    from model import Kronos
                    from tokenizer import KronosTokenizer
                    from predictor import KronosPredictor as KronosPredictorClass
                    
                    logger.info("使用etg_ai中的Kronos类加载模型")
                    tokenizer = KronosTokenizer.from_pretrained(tokenizer_path)
                    model = Kronos.from_pretrained(model_path)
                    self.predictor = KronosPredictorClass(
                        model, 
                        tokenizer, 
                        device=self.device, 
                        max_context=512
                    )
                    self.model = model
                    self.tokenizer = tokenizer
                    return
                except ImportError as e:
                    logger.warning(f"无法从etg_ai导入: {e}，使用transformers直接加载")
                except Exception as e:
                    logger.warning(f"使用etg_ai类加载失败: {e}，尝试transformers直接加载")
            
            # 使用transformers直接加载（Kronos可能需要特殊处理）
            logger.info("使用transformers直接加载模型")
            try:
                tokenizer = AutoTokenizer.from_pretrained(tokenizer_path, trust_remote_code=True, local_files_only=True)
                model = AutoModel.from_pretrained(model_path, trust_remote_code=True, local_files_only=True)
            except Exception as e:
                logger.warning(f"本地加载失败: {e}，尝试在线加载...")
                # 如果本地加载失败，尝试在线加载
                tokenizer = AutoTokenizer.from_pretrained("NeoQuasar/Kronos-Tokenizer-base", trust_remote_code=True)
                model = AutoModel.from_pretrained("NeoQuasar/Kronos-base", trust_remote_code=True)
            
            # 必须使用真实的KronosPredictor，禁止使用任何备用方案
            # 如果无法创建真实的predictor，直接失败
            error_msg = "无法创建Kronos预测器，必须使用真实的Kronos模型，禁止使用备用方案以避免误导交易系统"
            logger.error(error_msg)
            raise RuntimeError(error_msg)
            
        except Exception as e:
            logger.error(f"从Hugging Face加载模型失败: {e}")
            raise
    
    def should_repredict(self, current_price: float, current_trend: str) -> bool:
        """
        判断是否需要重新预测
        
        Args:
            current_price: 当前价格
            current_trend: 当前趋势（'up', 'down', 'sideways'）
        
        Returns:
            bool: 是否需要重新预测
        """
        # 如果距离上次预测超过30分钟，需要重新预测
        if self.last_prediction_time is None:
            return True
        
        time_since_last = time.time() - self.last_prediction_time
        if time_since_last >= self.prediction_interval:
            logger.info(f"距离上次预测已超过30分钟，需要重新预测")
            return True
        
        # 如果预测结果与当前市场有较大差异，需要重新预测
        if self.last_prediction:
            predicted_trend = self.last_prediction.get('trend', '')
            predicted_price_range = self.last_prediction.get('price_range', [])
            
            # 检查价格是否超出预测范围
            if predicted_price_range:
                min_price = min(predicted_price_range)
                max_price = max(predicted_price_range)
                if current_price < min_price * 0.95 or current_price > max_price * 1.05:
                    logger.info(f"当前价格 {current_price} 超出预测范围 [{min_price}, {max_price}]，需要重新预测")
                    return True
            
            # 检查趋势是否相反
            if predicted_trend == 'up' and current_trend == 'down':
                logger.info("预测趋势与当前趋势相反，需要重新预测")
                return True
            if predicted_trend == 'down' and current_trend == 'up':
                logger.info("预测趋势与当前趋势相反，需要重新预测")
                return True
        
        return False
    
    def predict_with_monte_carlo(
        self,
        price_history: List[float],
        volume_history: List[float],
        n_simulations: int = 10,
        prediction_horizon: int = 120  # 预测未来120步（标准预测方式）
    ) -> Dict:
        """
        使用蒙特卡罗方法进行范围预测
        
        Args:
            price_history: 历史价格序列（建议450步）
            volume_history: 历史成交量序列（建议450步）
            n_simulations: 蒙特卡罗模拟次数（默认10次，不超过10次以降低GPU压力和预测时间）
            prediction_horizon: 预测步数（默认120步，标准预测方式）
        
        Returns:
            Dict: 包含预测曲线、置信区间、趋势等信息
        """
        # 严格检查：模型必须可用，否则禁止预测
        if self.model is None or self.model == "placeholder":
            error_msg = "krnos模型未加载，禁止进行预测以避免误导交易系统"
            logger.error(error_msg)
            raise RuntimeError(error_msg)
        
        # 检查predictor是否可用
        if self.predictor is None:
            error_msg = "krnos预测器未初始化，禁止进行预测以避免误导交易系统"
            logger.error(error_msg)
            raise RuntimeError(error_msg)
        
        try:
            # 限制蒙特卡罗模拟次数（不超过10次以降低GPU压力和预测时间）
            if n_simulations > 10:
                logger.warning(f"蒙特卡罗模拟次数 {n_simulations} 超过限制，已调整为10次")
                n_simulations = 10
            
            logger.info(f"✓ 使用真实krnos模型进行预测: 价格数据 {len(price_history)} 条, 成交量数据 {len(volume_history)} 条")
            logger.info(f"  价格范围: [{min(price_history):.2f}, {max(price_history):.2f}]")
            logger.info(f"  成交量范围: [{min(volume_history):.2f}, {max(volume_history):.2f}]")
            logger.info(f"  预测步数: {prediction_horizon}, 蒙特卡罗模拟: {n_simulations}次")
            
            # 1. 使用真实的krnos模型进行基础预测
            base_prediction = self._call_real_krnos_model(price_history, volume_history, prediction_horizon)
            
            # 2. 蒙特卡罗模拟（使用真实模型预测结果）
            predictions = []
            for i in range(n_simulations):
                # 对模型预测结果添加适当的噪声进行蒙特卡罗模拟
                simulated_prediction = self._monte_carlo_simulation(base_prediction, price_history)
                predictions.append(simulated_prediction)
            
            # 3. 计算统计量
            predictions_array = np.array(predictions)
            mean_prediction = np.mean(predictions_array, axis=0)
            std_prediction = np.std(predictions_array, axis=0)
            confidence_interval_lower = mean_prediction - 1.96 * std_prediction
            confidence_interval_upper = mean_prediction + 1.96 * std_prediction
            
            # 判断趋势
            if mean_prediction[-1] > price_history[-1] * 1.01:
                trend = 'up'
            elif mean_prediction[-1] < price_history[-1] * 0.99:
                trend = 'down'
            else:
                trend = 'sideways'
            
            result = {
                'timestamp': datetime.now().isoformat(),
                'trend': trend,
                'mean_prediction': mean_prediction.tolist() if isinstance(mean_prediction, np.ndarray) else mean_prediction,
                'confidence_interval_lower': confidence_interval_lower.tolist() if isinstance(confidence_interval_lower, np.ndarray) else confidence_interval_lower,
                'confidence_interval_upper': confidence_interval_upper.tolist() if isinstance(confidence_interval_upper, np.ndarray) else confidence_interval_upper,
                'price_range': [
                    float(min(confidence_interval_lower)),
                    float(max(confidence_interval_upper))
                ],
                'trend_strength': self._calculate_trend_strength(mean_prediction),
                'prediction_horizon': prediction_horizon,
                'n_simulations': n_simulations,
                # 保存输入数据信息，用于验证是否使用了最新数据
                'input_data_info': {
                    'data_points': len(price_history),
                    'latest_price': float(price_history[-1]) if price_history else None,
                    'earliest_price': float(price_history[0]) if price_history else None,
                    'data_timestamp': datetime.now().isoformat()  # 数据获取时间
                }
            }
            
            # 保存预测结果
            self.last_prediction = result
            self.last_prediction_time = time.time()
            self._save_prediction(result)
            
            return result
            
        except Exception as e:
            error_msg = f"预测失败: {e}，禁止返回任何预测结果以避免误导交易系统"
            logger.error(error_msg, exc_info=True)
            # 不返回任何预测结果，直接抛出错误
            raise RuntimeError(error_msg) from e
    
    def _call_real_krnos_model(
        self, 
        price_history: List[float], 
        volume_history: List[float], 
        prediction_horizon: int
    ) -> np.ndarray:
        """
        调用真实的Kronos模型进行预测
        
        Args:
            price_history: 历史价格序列（收盘价）
            volume_history: 历史成交量序列
            prediction_horizon: 预测步数
        
        Returns:
            np.ndarray: 模型预测结果（收盘价序列）
        
        Raises:
            RuntimeError: 如果模型调用失败
        """
        if self.predictor is None:
            raise RuntimeError("Kronos模型未加载，无法进行预测")
        
        try:
            import pandas as pd
            from datetime import datetime, timedelta
            
            # 准备输入数据
            # Kronos需要OHLCV数据，但我们只有收盘价和成交量
            # 使用收盘价作为OHLC的近似值
            lookback = len(price_history)
            
            # 创建DataFrame（Kronos需要OHLCV格式）
            df_data = {
                'open': price_history,
                'high': price_history,  # 使用收盘价作为近似
                'low': price_history,   # 使用收盘价作为近似
                'close': price_history,
                'volume': volume_history if len(volume_history) == len(price_history) else [0] * len(price_history)
            }
            
            # 如果成交量数据不足，使用默认值
            if len(volume_history) < len(price_history):
                df_data['volume'] = [volume_history[0] if volume_history else 0] * len(price_history)
            
            df = pd.DataFrame(df_data)
            
            # 创建时间戳
            base_time = datetime.now()
            x_timestamp = pd.Series([base_time - timedelta(minutes=3*(lookback-i)) for i in range(lookback)])
            y_timestamp = pd.Series([base_time + timedelta(minutes=3*(i+1)) for i in range(prediction_horizon)])
            
            # 调用Kronos预测器
            logger.info(f"调用Kronos模型进行预测: 历史数据 {lookback} 步, 预测 {prediction_horizon} 步")
            
            pred_df = self.predictor.predict(
                df=df,
                x_timestamp=x_timestamp,
                y_timestamp=y_timestamp,
                pred_len=prediction_horizon,
                T=1.0,          # Temperature for sampling
                top_p=0.9,      # Nucleus sampling probability
                sample_count=1  # Number of forecast paths to generate and average
            )
            
            # 记录pred_df的信息用于调试
            logger.info(f"pred_df列名: {list(pred_df.columns)}")
            logger.info(f"pred_df形状: {pred_df.shape}")
            if len(pred_df.columns) > 0:
                logger.info(f"pred_df第一列前5个值: {pred_df.iloc[:5, 0].values}")
            
            # 提取预测的收盘价
            prediction = None
            if 'close' in pred_df.columns:
                prediction = pred_df['close'].values
                logger.info(f"使用'close'列提取预测数据")
            else:
                # 如果没有close列，尝试查找其他价格相关的列
                price_columns = [col for col in pred_df.columns if any(keyword in col.lower() for keyword in ['close', 'price', 'mid', 'value'])]
                if price_columns:
                    prediction = pred_df[price_columns[0]].values
                    logger.info(f"使用'{price_columns[0]}'列提取预测数据")
                else:
                    # 如果找不到价格列，使用第一列
                    prediction = pred_df.iloc[:, 0].values
            logger.warning(f"⚠️ 未找到价格列，使用第一列（列名: {pred_df.columns[0]}）")
            
            # 验证预测数据的合理性
            if prediction is not None and len(prediction) > 0:
                first_pred = prediction[0]
                last_price = price_history[-1] if price_history else None
                
                # 检查预测的第一个值是否与当前价格接近（允许±50%的偏差，因为这是预测值）
                if last_price is not None:
                    price_ratio = abs(first_pred / last_price) if last_price != 0 else float('inf')
                    if price_ratio < 0.1 or price_ratio > 10:
                        error_msg = (
                            f"⚠️ 预测数据异常：预测第一个值 {first_pred:.2f} 与当前价格 {last_price:.2f} 差异过大（比例: {price_ratio:.2f}）\n"
                            f"   pred_df列名: {list(pred_df.columns)}\n"
                            f"   pred_df形状: {pred_df.shape}\n"
                            f"   历史价格范围: [{min(price_history):.2f}, {max(price_history):.2f}]\n"
                            f"   预测数据范围: [{min(prediction):.2f}, {max(prediction):.2f}]"
                        )
                        logger.error(error_msg)
                        raise RuntimeError(f"预测数据异常，可能提取了错误的列: {error_msg}")
                    else:
                        logger.info(f"✓ 预测数据验证通过：预测第一个值 {first_pred:.2f}，当前价格 {last_price:.2f}，比例 {price_ratio:.2f}")
            
            logger.info(f"✓ Kronos模型预测完成，返回 {len(prediction)} 步预测结果")
            
            return np.array(prediction, dtype=np.float64)
            
        except Exception as e:
            error_msg = f"调用Kronos模型失败: {e}，禁止使用备用预测方案"
            logger.error(error_msg, exc_info=True)
            raise RuntimeError(error_msg) from e
    
    def _monte_carlo_simulation(self, base_prediction: np.ndarray, price_history: List[float]) -> np.ndarray:
        """
        对模型预测结果进行蒙特卡罗模拟
        
        Args:
            base_prediction: 模型基础预测结果
            price_history: 历史价格（用于计算波动率）
        
        Returns:
            np.ndarray: 模拟后的预测结果
        """
        # 计算历史波动率
        prices = np.array(price_history)
        if len(prices) > 1:
            returns = np.diff(prices) / prices[:-1]
            volatility = np.std(returns)
        else:
            volatility = 0.01
        
        # 添加基于波动率的随机噪声
        noise = np.random.normal(0, volatility * prices[-1] * 0.1, size=len(base_prediction))
        simulated = base_prediction + noise
        
        return simulated
    
    def _calculate_trend_strength(self, prediction: np.ndarray) -> float:
        """计算趋势强度（0-1）"""
        if len(prediction) < 2:
            return 0.5
        
        # 计算预测曲线的斜率
        slope = (prediction[-1] - prediction[0]) / len(prediction)
        # 归一化到0-1
        strength = min(abs(slope) / prediction[0] * 100, 1.0)
        return float(strength)
    
    def _save_prediction(self, prediction: Dict):
        """保存预测结果到缓存"""
        cache_file = self.cache_dir / f"prediction_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
        try:
            with open(cache_file, 'w', encoding='utf-8') as f:
                json.dump(prediction, f, indent=2, ensure_ascii=False)
            
            # 保存最新预测
            latest_file = self.cache_dir / "latest_prediction.json"
            with open(latest_file, 'w', encoding='utf-8') as f:
                json.dump(prediction, f, indent=2, ensure_ascii=False)
            
            logger.info(f"预测结果已保存: {cache_file}")
        except Exception as e:
            logger.error(f"保存预测结果失败: {e}")
    
    def get_latest_prediction(self) -> Optional[Dict]:
        """获取最新预测结果"""
        latest_file = self.cache_dir / "latest_prediction.json"
        if latest_file.exists():
            try:
                with open(latest_file, 'r', encoding='utf-8') as f:
                    return json.load(f)
            except Exception as e:
                logger.error(f"读取最新预测结果失败: {e}")
        return None

# SimpleKronosPredictor类已完全移除
# 系统现在严格使用真实的Kronos模型进行预测
# 如果Kronos模型不可用，预测将失败，不会使用任何备用方案以避免误导交易系统

def main():
    """主函数：处理命令行参数并运行预测"""
    import argparse
    
    parser = argparse.ArgumentParser(description='Kronos模型预测服务')
    parser.add_argument('--input', type=str, help='输入JSON文件路径')
    args = parser.parse_args()
    
    # 日志已经在模块级别配置为输出到stderr，无需重复配置
    
    try:
        predictor = KrnosPredictor()
        
        if args.input:
            # 从文件读取输入数据
            with open(args.input, 'r', encoding='utf-8') as f:
                input_data = json.load(f)
            
            price_history = input_data.get('price_history', [])
            volume_history = input_data.get('volume_history', [])
            n_simulations = input_data.get('n_simulations', 10)
            prediction_horizon = input_data.get('prediction_horizon', 120)
            
            # 进行预测
            result = predictor.predict_with_monte_carlo(
                price_history, 
                volume_history, 
                n_simulations=n_simulations,
                prediction_horizon=prediction_horizon
            )
        else:
            # 测试模式：使用模拟数据
            price_history = [90000 + i * 10 + np.random.randn() * 100 for i in range(100)]
            volume_history = [1000 + np.random.randn() * 100 for _ in range(100)]
            result = predictor.predict_with_monte_carlo(price_history, volume_history, n_simulations=10)
        
        # 输出JSON到stdout（这是唯一输出到stdout的内容）
        print(json.dumps(result, indent=2, ensure_ascii=False))
        
    except Exception as e:
        # 错误信息输出到stderr
        error_result = {
            "error": str(e),
            "timestamp": datetime.now().isoformat()
        }
        # 错误也以JSON格式输出到stdout，方便Go代码解析
        print(json.dumps(error_result, indent=2, ensure_ascii=False))
        sys.exit(1)


if __name__ == '__main__':
    main()

