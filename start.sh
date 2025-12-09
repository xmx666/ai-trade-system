#!/bin/bash

# ═══════════════════════════════════════════════════════════════
# NOFX AI Trading System - Docker Quick Start Script
# Usage: ./start.sh [command]
# ═══════════════════════════════════════════════════════════════

set -e

# ------------------------------------------------------------------------
# Color Definitions
# ------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ------------------------------------------------------------------------
# Utility Functions: Colored Output
# ------------------------------------------------------------------------
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# ------------------------------------------------------------------------
# Detection: Docker Compose Command (Backward Compatible)
# ------------------------------------------------------------------------
detect_compose_cmd() {
    if command -v docker compose &> /dev/null; then
        COMPOSE_CMD="docker compose"
    elif command -v docker-compose &> /dev/null; then
        COMPOSE_CMD="docker-compose"
    else
        print_error "Docker Compose 未安装！请先安装 Docker Compose"
        exit 1
    fi
    print_info "使用 Docker Compose 命令: $COMPOSE_CMD"
}

# ------------------------------------------------------------------------
# Validation: Docker Installation
# ------------------------------------------------------------------------
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker 未安装！请先安装 Docker: https://docs.docker.com/get-docker/"
        exit 1
    fi

    detect_compose_cmd
    print_success "Docker 和 Docker Compose 已安装"
}

# ------------------------------------------------------------------------
# Validation: Environment File (.env)
# ------------------------------------------------------------------------
check_env() {
    if [ ! -f ".env" ]; then
        print_warning ".env 不存在，从模板复制..."
        cp .env.example .env
        print_info "✓ 已使用默认环境变量创建 .env"
        print_info "💡 如需修改端口等设置，可编辑 .env 文件"
    fi
    print_success "环境变量文件存在"
}

# ------------------------------------------------------------------------
# Validation: Configuration File (config.json) - BASIC SETTINGS ONLY
# ------------------------------------------------------------------------
check_config() {
    if [ ! -f "config.json" ]; then
        print_warning "config.json 不存在，从模板复制..."
        cp config.json.example config.json
        print_info "✓ 已使用默认配置创建 config.json"
        print_info "💡 如需修改基础设置（杠杆大小、开仓币种、管理员模式、JWT密钥等），可编辑 config.json"
        print_info "💡 模型/交易所/交易员配置请使用Web界面"
    fi
    print_success "配置文件存在"
}

# ------------------------------------------------------------------------
# Utility: Read Environment Variables
# ------------------------------------------------------------------------
read_env_vars() {
    if [ -f ".env" ]; then
        # 读取端口配置，设置默认值
        NOFX_FRONTEND_PORT=$(grep "^NOFX_FRONTEND_PORT=" .env 2>/dev/null | cut -d'=' -f2 || echo "3000")
        NOFX_BACKEND_PORT=$(grep "^NOFX_BACKEND_PORT=" .env 2>/dev/null | cut -d'=' -f2 || echo "8080")
        
        # 去除可能的引号和空格
        NOFX_FRONTEND_PORT=$(echo "$NOFX_FRONTEND_PORT" | tr -d '"'"'" | tr -d ' ')
        NOFX_BACKEND_PORT=$(echo "$NOFX_BACKEND_PORT" | tr -d '"'"'" | tr -d ' ')
        
        # 如果为空则使用默认值
        NOFX_FRONTEND_PORT=${NOFX_FRONTEND_PORT:-3000}
        NOFX_BACKEND_PORT=${NOFX_BACKEND_PORT:-8080}
    else
        # 如果.env不存在，使用默认端口
        NOFX_FRONTEND_PORT=3000
        NOFX_BACKEND_PORT=8080
    fi
}

# ------------------------------------------------------------------------
# Validation: Database File (config.db)
# ------------------------------------------------------------------------
check_database() {
    if [ ! -f "config.db" ]; then
        print_warning "数据库文件不存在，创建空数据库文件..."
        # 创建空文件以避免Docker创建目录
        touch config.db
        print_info "✓ 已创建空数据库文件，系统将在启动时初始化"
    else
        print_success "数据库文件存在"
    fi
}

# ------------------------------------------------------------------------
# Build: Frontend (Node.js Based)
# ------------------------------------------------------------------------
# build_frontend() {
#     print_info "检查前端构建环境..."

#     if ! command -v node &> /dev/null; then
#         print_error "Node.js 未安装！请先安装 Node.js"
#         exit 1
#     fi

#     if ! command -v npm &> /dev/null; then
#         print_error "npm 未安装！请先安装 npm"
#         exit 1
#     fi

#     print_info "正在构建前端..."
#     cd web

#     print_info "安装 Node.js 依赖..."
#     npm install

#     print_info "构建前端应用..."
#     npm run build

#     cd ..
#     print_success "前端构建完成"
# }

# ------------------------------------------------------------------------
# Service Management: Start
# ------------------------------------------------------------------------
start() {
    print_info "正在启动 NOFX AI Trading System..."

    # 读取环境变量
    read_env_vars

    # Auto-build frontend if missing or forced
    # if [ ! -d "web/dist" ] || [ "$1" == "--build" ]; then
    #     build_frontend
    # fi

    # Rebuild images if flag set
    if [ "$1" == "--build" ]; then
        print_info "重新构建镜像..."
        print_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        print_info "构建过程会显示详细进度，请耐心等待"
        print_info "首次构建可能需要 10-25 分钟"
        print_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        
        # 先构建镜像（显示详细进度）
        print_info "步骤 1/2: 构建镜像（显示详细进度）..."
        
        # 读取代理配置（如果存在）
        BUILD_ARGS=""
        if [ -f ".env" ]; then
            HTTP_PROXY_VAL=$(grep "^HTTP_PROXY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d ' ')
            HTTPS_PROXY_VAL=$(grep "^HTTPS_PROXY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d ' ')
            NO_PROXY_VAL=$(grep "^NO_PROXY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d ' ')
            
            if [ -n "$HTTP_PROXY_VAL" ]; then
                BUILD_ARGS="$BUILD_ARGS --build-arg HTTP_PROXY=$HTTP_PROXY_VAL"
            fi
            if [ -n "$HTTPS_PROXY_VAL" ]; then
                BUILD_ARGS="$BUILD_ARGS --build-arg HTTPS_PROXY=$HTTPS_PROXY_VAL"
            fi
            if [ -n "$NO_PROXY_VAL" ]; then
                BUILD_ARGS="$BUILD_ARGS --build-arg NO_PROXY=$NO_PROXY_VAL"
            fi
        fi
        
        # 设置BuildKit代理环境变量（如果.env中有代理配置）
        if [ -f .env ]; then
            export HTTP_PROXY=$(grep "^HTTP_PROXY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d ' ' || echo "")
            export HTTPS_PROXY=$(grep "^HTTPS_PROXY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d ' ' || echo "")
            export NO_PROXY=$(grep "^NO_PROXY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d ' ' || echo "")
            if [ -n "$HTTP_PROXY" ]; then
                print_info "设置BuildKit代理环境变量: HTTP_PROXY=$HTTP_PROXY"
            fi
        fi
        
        # --progress 是全局标志，需要放在 compose 后面
        if [[ "$COMPOSE_CMD" == "docker compose" ]]; then
            if [ -n "$BUILD_ARGS" ]; then
                print_info "使用代理配置进行构建..."
                docker compose --progress=plain build $BUILD_ARGS
            else
                docker compose --progress=plain build
            fi
        else
            if [ -n "$BUILD_ARGS" ]; then
                print_info "使用代理配置进行构建..."
                $COMPOSE_CMD build $BUILD_ARGS
            else
                $COMPOSE_CMD build
            fi
        fi
        
        if [ $? -ne 0 ]; then
            print_error "镜像构建失败！"
            exit 1
        fi
        
        print_success "镜像构建完成！"
        echo ""
        
        # 然后启动容器
        print_info "步骤 2/2: 启动容器..."
        $COMPOSE_CMD up -d
        
        if [ $? -ne 0 ]; then
            print_error "容器启动失败！"
            exit 1
        fi
    else
        print_info "启动容器..."
        $COMPOSE_CMD up -d
    fi

    print_success "服务已启动！"
    print_info "Web 界面: http://localhost:${NOFX_FRONTEND_PORT}"
    print_info "API 端点: http://localhost:${NOFX_BACKEND_PORT}"
    print_info ""
    print_info "查看日志: ./start.sh logs"
    print_info "停止服务: ./start.sh stop"
}

# ------------------------------------------------------------------------
# Service Management: Stop
# ------------------------------------------------------------------------
stop() {
    print_info "正在停止服务..."
    $COMPOSE_CMD stop
    print_success "服务已停止"
}

# ------------------------------------------------------------------------
# Service Management: Restart
# ------------------------------------------------------------------------
restart() {
    print_info "正在重启服务..."
    $COMPOSE_CMD restart
    print_success "服务已重启"
}

# ------------------------------------------------------------------------
# Monitoring: Logs
# ------------------------------------------------------------------------
logs() {
    if [ -z "$2" ]; then
        print_info "查看所有服务的完整实时日志（按 Ctrl+C 退出）..."
        print_info "提示：使用 './start.sh logs nofx' 只查看后端日志"
        print_info "提示：使用 './start.sh logs nofx-frontend' 只查看前端日志"
        echo ""
        $COMPOSE_CMD logs -f
    else
        print_info "查看服务 '$2' 的完整实时日志（按 Ctrl+C 退出）..."
        $COMPOSE_CMD logs -f "$2"
    fi
}

# ------------------------------------------------------------------------
# Monitoring: Status
# ------------------------------------------------------------------------
status() {
    # 读取环境变量
    read_env_vars
    
    print_info "服务状态:"
    $COMPOSE_CMD ps
    echo ""
    print_info "健康检查:"
    curl -s "http://localhost:${NOFX_BACKEND_PORT}/api/health" | jq '.' || echo "后端未响应"
}

# ------------------------------------------------------------------------
# Maintenance: Clean (Destructive)
# ------------------------------------------------------------------------
clean() {
    print_warning "这将删除所有容器和数据！"
    read -p "确认删除？(yes/no): " confirm
    if [ "$confirm" == "yes" ]; then
        print_info "正在清理..."
        $COMPOSE_CMD down -v
        print_success "清理完成"
    else
        print_info "已取消"
    fi
}

# ------------------------------------------------------------------------
# Maintenance: Update
# ------------------------------------------------------------------------
update() {
    print_info "正在更新..."
    git pull
    
    print_info "重新构建镜像（显示详细进度）..."
    
    # 读取代理配置（如果存在）
    BUILD_ARGS=""
    if [ -f ".env" ]; then
        HTTP_PROXY_VAL=$(grep "^HTTP_PROXY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d ' ')
        HTTPS_PROXY_VAL=$(grep "^HTTPS_PROXY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d ' ')
        NO_PROXY_VAL=$(grep "^NO_PROXY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d ' ')
        
        if [ -n "$HTTP_PROXY_VAL" ]; then
            BUILD_ARGS="$BUILD_ARGS --build-arg HTTP_PROXY=$HTTP_PROXY_VAL"
        fi
        if [ -n "$HTTPS_PROXY_VAL" ]; then
            BUILD_ARGS="$BUILD_ARGS --build-arg HTTPS_PROXY=$HTTPS_PROXY_VAL"
        fi
        if [ -n "$NO_PROXY_VAL" ]; then
            BUILD_ARGS="$BUILD_ARGS --build-arg NO_PROXY=$NO_PROXY_VAL"
        fi
    fi
    
    # --progress 是全局标志，需要放在 compose 后面
    if [[ "$COMPOSE_CMD" == "docker compose" ]]; then
        if [ -n "$BUILD_ARGS" ]; then
            print_info "使用代理配置进行构建..."
            docker compose --progress=plain build $BUILD_ARGS
        else
            docker compose --progress=plain build
        fi
    else
        if [ -n "$BUILD_ARGS" ]; then
            print_info "使用代理配置进行构建..."
            $COMPOSE_CMD build $BUILD_ARGS
        else
            $COMPOSE_CMD build
        fi
    fi
    
    if [ $? -ne 0 ]; then
        print_error "镜像构建失败！"
        exit 1
    fi
    
    print_info "启动容器..."
    $COMPOSE_CMD up -d
    
    print_success "更新完成"
}

# ------------------------------------------------------------------------
# Help: Usage Information
# ------------------------------------------------------------------------
show_help() {
    echo "NOFX AI Trading System - Docker 管理脚本"
    echo ""
    echo "用法: ./start.sh [command] [options]"
    echo ""
    echo "命令:"
    echo "  start [--build]    启动服务（可选：重新构建，会显示详细构建进度）"
    echo "  stop               停止服务"
    echo "  restart            重启服务"
    echo "  logs [service]     查看日志（可选：指定服务名 nofx/nofx-frontend）"
    echo "  status             查看服务状态"
    echo "  clean              清理所有容器和数据"
    echo "  update             更新代码并重启（会显示详细构建进度）"
    echo "  help               显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  ./start.sh start --build    # 构建并启动"
    echo "  ./start.sh logs backend     # 查看后端日志"
    echo "  ./start.sh status           # 查看状态"
}

# ------------------------------------------------------------------------
# Main: Command Dispatcher
# ------------------------------------------------------------------------
main() {
    check_docker

    case "${1:-start}" in
        start)
            check_env
            check_config
            check_database
            start "$2"
            ;;
        stop)
            stop
            ;;
        restart)
            restart
            ;;
        logs)
            logs "$@"
            ;;
        status)
            status
            ;;
        clean)
            clean
            ;;
        update)
            update
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            print_error "未知命令: $1"
            show_help
            exit 1
            ;;
    esac
}

# Execute Main
main "$@"