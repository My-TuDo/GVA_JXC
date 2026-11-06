#!/usr/bin/env bash
# 把演示库恢复到 golden.sql（root/cron 执行）。
# 用法：reset-demo.sh [golden.sql]
set -euo pipefail
GOLDEN="${1:-/opt/jxc/backups/golden.sql}"
DB="${JXC_DB:-jxc}"
[ -f "$GOLDEN" ] || { echo "缺少 $GOLDEN，先按 docs/35 生成金库快照"; exit 1; }
mysql -uroot "$DB" < "$GOLDEN"
echo "$(date "+%F %T") 已恢复 $DB <- $GOLDEN"
