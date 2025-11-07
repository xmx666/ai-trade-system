# Windows时间同步脚本
# 需要以管理员权限运行

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "Windows时间同步工具" -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan
Write-Host ""

# 检查管理员权限
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "⚠️  需要管理员权限才能同步时间" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "请按以下步骤操作：" -ForegroundColor Cyan
    Write-Host "1. 右键点击 PowerShell 图标" -ForegroundColor White
    Write-Host "2. 选择 '以管理员身份运行'" -ForegroundColor White
    Write-Host "3. 重新运行此脚本" -ForegroundColor White
    Write-Host ""
    Write-Host "或者手动同步：" -ForegroundColor Cyan
    Write-Host "1. 按 Win+I 打开设置" -ForegroundColor White
    Write-Host "2. 进入 时间和语言 > 日期和时间" -ForegroundColor White
    Write-Host "3. 点击 '立即同步' 按钮" -ForegroundColor White
    Write-Host ""
    exit 1
}

Write-Host "✓ 已获得管理员权限" -ForegroundColor Green
Write-Host ""

# 检查时间服务状态
Write-Host "检查Windows时间服务..." -ForegroundColor Cyan
$timeService = Get-Service -Name "W32Time" -ErrorAction SilentlyContinue
if ($timeService) {
    Write-Host "  状态: $($timeService.Status)" -ForegroundColor $(if ($timeService.Status -eq 'Running') { 'Green' } else { 'Yellow' })
    Write-Host "  启动类型: $($timeService.StartType)" -ForegroundColor White
    
    if ($timeService.Status -ne 'Running') {
        Write-Host "  正在启动时间服务..." -ForegroundColor Yellow
        Start-Service -Name "W32Time"
    }
} else {
    Write-Host "  ⚠️  时间服务未找到" -ForegroundColor Yellow
}

Write-Host ""

# 显示当前时间
Write-Host "当前系统时间:" -ForegroundColor Cyan
$currentTime = Get-Date
Write-Host "  $($currentTime.ToString('yyyy-MM-dd HH:mm:ss'))" -ForegroundColor White
Write-Host ""

# 尝试同步时间
Write-Host "正在同步时间..." -ForegroundColor Cyan
try {
    $result = w32tm /resync 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ 时间同步成功" -ForegroundColor Green
        Write-Host $result
    } else {
        Write-Host "⚠️  时间同步失败: $result" -ForegroundColor Yellow
        Write-Host ""
        Write-Host "尝试手动同步：" -ForegroundColor Cyan
        Write-Host "1. 按 Win+I 打开设置" -ForegroundColor White
        Write-Host "2. 进入 时间和语言 > 日期和时间" -ForegroundColor White
        Write-Host "3. 点击 '立即同步' 按钮" -ForegroundColor White
    }
} catch {
    Write-Host "⚠️  同步失败: $_" -ForegroundColor Yellow
}

Write-Host ""

# 查询时间源信息
Write-Host "时间源信息:" -ForegroundColor Cyan
try {
    $source = w32tm /query /source 2>&1
    Write-Host "  时间源: $source" -ForegroundColor White
    
    $status = w32tm /query /status 2>&1 | Select-String -Pattern "Last Successful|Last Error" | Select-Object -First 2
    if ($status) {
        $status | ForEach-Object { Write-Host "  $_" -ForegroundColor White }
    }
} catch {
    Write-Host "  无法查询时间源信息" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "完成！请重启Docker容器以应用新时间" -ForegroundColor Green
Write-Host "运行: docker compose restart nofx" -ForegroundColor Yellow
Write-Host "=========================================" -ForegroundColor Cyan

