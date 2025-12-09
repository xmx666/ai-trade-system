@echo off
chcp 65001 >nul
echo ==========================================
echo Docker镜像源配置修复
echo ==========================================
echo.

set "DOCKER_CONFIG_DIR=%USERPROFILE%\.docker"
set "DOCKER_CONFIG_FILE=%DOCKER_CONFIG_DIR%\daemon.json"

echo [1/4] 检查Docker配置目录...

if not exist "%DOCKER_CONFIG_DIR%" (
    echo 创建配置目录: %DOCKER_CONFIG_DIR%
    mkdir "%DOCKER_CONFIG_DIR%"
) else (
    echo 配置目录已存在: %DOCKER_CONFIG_DIR%
)

echo [2/4] 备份现有配置...

if exist "%DOCKER_CONFIG_FILE%" (
    set "BACKUP_FILE=%DOCKER_CONFIG_FILE%.backup.%date:~0,4%%date:~5,2%%date:~8,2%_%time:~0,2%%time:~3,2%%time:~6,2%"
    set "BACKUP_FILE=%BACKUP_FILE: =0%"
    copy "%DOCKER_CONFIG_FILE%" "%BACKUP_FILE%" >nul
    echo 已备份到: %BACKUP_FILE%
) else (
    echo 未找到现有配置，将创建新配置
)

echo [3/4] 创建新的镜像源配置...

(
echo {
echo   "registry-mirrors": [
echo     "https://docker.mirrors.ustc.edu.cn",
echo     "https://hub-mirror.c.163.com",
echo     "https://mirror.baidubce.com"
echo   ]
echo }
) > "%DOCKER_CONFIG_FILE%"

echo 配置已创建: %DOCKER_CONFIG_FILE%
echo.
echo 配置内容:
type "%DOCKER_CONFIG_FILE%"
echo.

echo [4/4] 配置完成！
echo.
echo 请执行以下操作使配置生效:
echo    1. 打开Docker Desktop
echo    2. 点击右上角设置图标（齿轮）
echo    3. 进入 'Docker Engine' 设置
echo    4. 点击 'Apply ^& Restart' 按钮
echo.
echo 或者直接重启Docker Desktop
echo.
echo ==========================================

pause

