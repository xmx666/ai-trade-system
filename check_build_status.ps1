# Docker 构建状态检查脚本 (PowerShell)
# 适用于 Windows PowerShell

Write-Host "=== Docker 构建状态检查 ===" -ForegroundColor Cyan
Write-Host ""

# 检查 Docker 是否可用
try {
    docker --version | Out-Null
    Write-Host "✓ Docker 已安装" -ForegroundColor Green
} catch {
    Write-Host "✗ Docker 未找到，请确保 Docker Desktop 已启动" -ForegroundColor Red
    exit 1
}

# 检测 Docker Compose 命令
$composeCmd = $null
if (Get-Command docker -ErrorAction SilentlyContinue) {
    try {
        docker compose version | Out-Null
        $composeCmd = "docker compose"
        Write-Host "✓ 使用: docker compose" -ForegroundColor Green
    } catch {
        try {
            docker-compose --version | Out-Null
            $composeCmd = "docker-compose"
            Write-Host "✓ 使用: docker-compose" -ForegroundColor Green
        } catch {
            Write-Host "✗ Docker Compose 未找到" -ForegroundColor Red
            exit 1
        }
    }
}

Write-Host ""
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
Write-Host "1. 检查容器状态" -ForegroundColor Yellow
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
Invoke-Expression "$composeCmd ps"
Write-Host ""

Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
Write-Host "2. 检查镜像状态" -ForegroundColor Yellow
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
docker images | Select-String -Pattern "nofx|REPOSITORY"
Write-Host ""

Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
Write-Host "3. 最近 30 行构建日志" -ForegroundColor Yellow
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
Invoke-Expression "$composeCmd logs --tail=30" | Select-Object -Last 30
Write-Host ""

Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
Write-Host "4. 检查端口占用" -ForegroundColor Yellow
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
$ports = @(1666, 3001, 8080)
foreach ($port in $ports) {
    $result = Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue
    if ($result) {
        Write-Host "端口 $port 已被占用" -ForegroundColor Yellow
    } else {
        Write-Host "端口 $port 未被占用" -ForegroundColor Green
    }
}
Write-Host ""

Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
Write-Host "建议操作" -ForegroundColor Yellow
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Gray
Write-Host ""
Write-Host "如果构建正在进行：" -ForegroundColor Cyan
Write-Host "  - 查看实时日志: $composeCmd logs -f" -ForegroundColor White
Write-Host "  - 或运行: ./start.sh logs" -ForegroundColor White
Write-Host ""
Write-Host "如果构建已完成：" -ForegroundColor Cyan
Write-Host "  - 检查服务状态: $composeCmd ps" -ForegroundColor White
Write-Host "  - 访问前端: http://localhost:3001" -ForegroundColor White
Write-Host "  - 访问后端: http://localhost:1666" -ForegroundColor White
Write-Host ""
Write-Host "如果构建卡住：" -ForegroundColor Cyan
Write-Host "  - 按 Ctrl+C 中断" -ForegroundColor White
Write-Host "  - 重新运行: ./start.sh start --build" -ForegroundColor White
Write-Host ""

