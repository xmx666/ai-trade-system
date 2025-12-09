# Docker 镜像源修复脚本 (PowerShell)
# 用于 Windows 上的 Docker Desktop

Write-Host "正在检查 Docker Desktop 配置..." -ForegroundColor Yellow

$dockerConfigPath = "$env:APPDATA\Docker\settings.json"

if (Test-Path $dockerConfigPath) {
    Write-Host "找到 Docker 配置文件: $dockerConfigPath" -ForegroundColor Green
    
    # 读取当前配置
    $config = Get-Content $dockerConfigPath | ConvertFrom-Json
    
    # 检查是否有 registry-mirrors 配置
    if (-not $config.registryMirrors) {
        $config | Add-Member -MemberType NoteProperty -Name "registryMirrors" -Value @()
    }
    
    # 更新镜像源为可用的源
    $config.registryMirrors = @(
        "https://docker.mirrors.ustc.edu.cn",
        "https://hub-mirror.c.163.com",
        "https://mirror.baidubce.com"
    )
    
    # 保存配置
    $config | ConvertTo-Json -Depth 10 | Set-Content $dockerConfigPath
    
    Write-Host "已更新 Docker 镜像源配置" -ForegroundColor Green
    Write-Host "请重启 Docker Desktop 使配置生效" -ForegroundColor Yellow
} else {
    Write-Host "未找到 Docker Desktop 配置文件" -ForegroundColor Red
    Write-Host "请手动在 Docker Desktop Settings -> Docker Engine 中配置镜像源" -ForegroundColor Yellow
}
