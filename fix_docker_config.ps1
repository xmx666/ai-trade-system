# Docker镜像源配置脚本 (Windows PowerShell)

Write-Host "=========================================="
Write-Host "Docker镜像源配置修复"
Write-Host "=========================================="
Write-Host ""

# 获取用户配置目录
$dockerConfigDir = "$env:USERPROFILE\.docker"
$dockerConfigFile = "$dockerConfigDir\daemon.json"

Write-Host "[1/4] 检查Docker配置目录..."

# 创建配置目录（如果不存在）
if (-not (Test-Path $dockerConfigDir)) {
    Write-Host "创建配置目录: $dockerConfigDir"
    New-Item -ItemType Directory -Force -Path $dockerConfigDir | Out-Null
} else {
    Write-Host "配置目录已存在: $dockerConfigDir"
}

Write-Host "[2/4] 备份现有配置..."

# 备份现有配置
if (Test-Path $dockerConfigFile) {
    $backupFile = "$dockerConfigFile.backup.$(Get-Date -Format 'yyyyMMdd_HHmmss')"
    Copy-Item $dockerConfigFile $backupFile
    Write-Host "已备份到: $backupFile"
} else {
    Write-Host "未找到现有配置，将创建新配置"
}

Write-Host "[3/4] 创建新的镜像源配置..."

# 创建配置JSON内容
$configJson = @'
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ]
}
'@

# 保存配置
$configJson | Out-File -FilePath $dockerConfigFile -Encoding utf8 -Force

Write-Host "配置已创建: $dockerConfigFile"
Write-Host ""
Write-Host "配置内容:"
Get-Content $dockerConfigFile
Write-Host ""

Write-Host "[4/4] 配置完成！"
Write-Host ""
Write-Host "请执行以下操作使配置生效:"
Write-Host "   1. 打开Docker Desktop"
Write-Host "   2. 点击右上角设置图标（齿轮）"
Write-Host "   3. 进入 'Docker Engine' 设置"
Write-Host "   4. 点击 'Apply & Restart' 按钮"
Write-Host ""
Write-Host "或者直接重启Docker Desktop"
Write-Host ""
Write-Host "=========================================="
