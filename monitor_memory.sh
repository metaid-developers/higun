#!/bin/bash
# Docker容器内存监控脚本

CONTAINER_NAME="higun_btc"  # 根据实际容器名修改

echo "=== Docker容器内存监控 ==="
echo ""

# 检查容器是否运行
if ! docker ps | grep -q $CONTAINER_NAME; then
    echo "❌ 容器 $CONTAINER_NAME 未运行"
    exit 1
fi

echo "✓ 容器运行中"
echo ""

# 获取容器内存限制
MEM_LIMIT=$(docker inspect $CONTAINER_NAME | grep -i "memory\":" | head -1 | awk '{print $2}' | tr -d ',')
if [ -z "$MEM_LIMIT" ] || [ "$MEM_LIMIT" = "0" ]; then
    MEM_LIMIT="unlimited"
else
    MEM_LIMIT_GB=$((MEM_LIMIT / 1024 / 1024 / 1024))
    MEM_LIMIT="${MEM_LIMIT_GB}GB"
fi

echo "内存限制: $MEM_LIMIT"
echo ""
echo "实时内存使用情况 (每5秒刷新)："
echo "----------------------------------------"

while true; do
    # 获取容器内存统计
    STATS=$(docker stats $CONTAINER_NAME --no-stream --format "{{.MemUsage}}")
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    
    # 提取使用量和限制
    MEM_USED=$(echo $STATS | awk '{print $1}')
    MEM_TOTAL=$(echo $STATS | awk '{print $3}')
    
    # 获取最近的日志（检查是否有OOM或GC）
    RECENT_LOG=$(docker logs $CONTAINER_NAME --tail 3 2>&1 | grep -E "\[Memory\]|OOM|killed|restart")
    
    printf "\r[$TIMESTAMP] 使用: %s / %s" "$MEM_USED" "$MEM_TOTAL"
    
    # 如果有内存相关日志，显示警告
    if [ ! -z "$RECENT_LOG" ]; then
        echo ""
        echo "⚠️  内存告警:"
        echo "$RECENT_LOG"
    fi
    
    sleep 5
done
