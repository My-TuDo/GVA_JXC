#!/usr/bin/env bash
# 一次性服务器初始化（Anolis 3 / RHEL 8 系，root 执行）。先读 docs/35-服务器部署与演示.md。
# 用法：JXC_DB_PASSWORD='强密码' bash server-setup.sh
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then echo "请用 root 执行"; exit 1; fi
HERE="$(cd "$(dirname "$0")" && pwd)"
APP_USER=jxc
APP_DIR=/opt/jxc
DB_NAME=jxc
DB_USER=jxc
: "${JXC_DB_PASSWORD:?请先 export JXC_DB_PASSWORD=<数据库密码>}"

echo "==> 安装依赖"
dnf install -y nginx mysql-server tar python3

echo "==> 系统用户与目录"
id -u "$APP_USER" >/dev/null 2>&1 || useradd --system --home "$APP_DIR" --shell /sbin/nologin "$APP_USER"
mkdir -p "$APP_DIR"/{bin,server,web,backups,logs}
chown -R root:"$APP_USER" "$APP_DIR"
chmod 750 "$APP_DIR" "$APP_DIR/server"

echo "==> MySQL 调优"
install -m 644 "$HERE/my.cnf" /etc/my.cnf.d/jxc.cnf
systemctl enable --now mysqld

echo "==> 建库与账号"
mysql -uroot <<SQL
CREATE DATABASE IF NOT EXISTS `$DB_NAME` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
CREATE USER IF NOT EXISTS '$DB_USER'@'127.0.0.1' IDENTIFIED BY '$JXC_DB_PASSWORD';
ALTER USER '$DB_USER'@'127.0.0.1' IDENTIFIED BY '$JXC_DB_PASSWORD';
GRANT ALL PRIVILEGES ON `$DB_NAME`.* TO '$DB_USER'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL

echo "==> nginx 与 systemd"
install -m 644 "$HERE/nginx-jxc.conf" /etc/nginx/conf.d/jxc.conf
install -m 644 "$HERE/jxc.service" /etc/systemd/system/jxc.service
install -m 755 "$HERE/upgrade.sh" /opt/jxc/bin/upgrade.sh
install -m 755 "$HERE/reset-demo.sh" /opt/jxc/bin/reset-demo.sh
systemctl daemon-reload
systemctl enable --now nginx
nginx -t && systemctl reload nginx

echo "==> 完成。下一步："
echo "  1) 上传二进制与前端到 /tmp，再执行 /opt/jxc/bin/upgrade.sh"
echo "  2) 写 /opt/jxc/server/config.yaml（chmod 640 root:jxc）"
echo "  3) 初始化数据库与权限，见 docs/35 第 4 节"
