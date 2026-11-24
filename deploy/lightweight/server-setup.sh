#!/usr/bin/env bash
# 一次性服务器初始化（Anolis 3 / RHEL 8 系，root 执行）。先读 docs/35-服务器部署与演示.md。
# 用法：JXC_DB_PASSWORD='强密码' bash server-setup.sh
#      登录用户是 admin 等普通账号时：sudo JXC_DB_PASSWORD='强密码' bash server-setup.sh
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "请用 root 执行。普通账号（如 admin）请用 sudo，注意密码写在 sudo 后面："
  echo "  sudo JXC_DB_PASSWORD='强密码' bash $0"
  exit 1
fi
HERE="$(cd "$(dirname "$0")" && pwd)"
APP_USER=jxc
APP_DIR=/opt/jxc
DB_NAME=jxc
DB_USER=jxc
: "${JXC_DB_PASSWORD:?请把密码写在 sudo 后面：sudo JXC_DB_PASSWORD=xxx bash server-setup.sh}"

echo "==> 安装依赖（核心四件，缺一即停）"
# --disableexcludes=main：宝塔等面板镜像会在 /etc/dnf/dnf.conf 里 exclude nginx/mysql，
# 不关掉全局排除就会报「All matches were filtered out by exclude filtering」。
# 普通镜像上没有 exclude，这个参数是空操作，可以一直带着。
for pkg in nginx mysql-server tar openssl; do
  if ! dnf install -y --disableexcludes=main "$pkg"; then
    echo "!! 安装 $pkg 失败。依次排查："
    echo "   1) grep -n exclude /etc/dnf/dnf.conf   # 被全局排除（宝塔镜像常见）"
    echo "   2) dnf module list $pkg                 # 被 module 化，需先 module enable"
    echo "   3) dnf repolist --all                   # 仓库没启用"
    exit 1
  fi
done

# python3 只有演示数据脚本（seed-demo.py）用。精简镜像常常没装 python3，
# 但系统自带的 platform-python 也是 3.6，脚本只用标准库，可直接复用，避免为它卡住部署。
if ! command -v python3 >/dev/null 2>&1; then
  dnf install -y python3 || true
fi
if command -v python3 >/dev/null 2>&1; then
  PYTHON_BIN="$(command -v python3)"
elif [ -x /usr/libexec/platform-python ]; then
  PYTHON_BIN=/usr/libexec/platform-python
  echo "   未装 python3，第 7 步请用 $PYTHON_BIN 跑 seed-demo.py"
else
  PYTHON_BIN=""
  echo "!! 既无 python3 也无 platform-python：第 7 步的 seed-demo.py 需要自行安装 python3"
fi

echo "==> 系统用户与目录"
id -u "$APP_USER" >/dev/null 2>&1 || useradd --system --home "$APP_DIR" --shell /sbin/nologin "$APP_USER"
mkdir -p "$APP_DIR"/{bin,server,web,backups}
chown -R root:"$APP_USER" "$APP_DIR"
chmod 750 "$APP_DIR" "$APP_DIR/server"

# jxc.service 用 ProtectSystem=strict，只有 ReadWritePaths 里的路径可写；
# 这些路径若不存在，systemd 会直接以 NAMESPACE 错误启动失败，所以必须先建好并交给 jxc。
mkdir -p "$APP_DIR/server"/{log,uploads}
chown -R "$APP_USER":"$APP_USER" "$APP_DIR/server/log" "$APP_DIR/server/uploads"
chmod 750 "$APP_DIR/server/log" "$APP_DIR/server/uploads"

echo "==> MySQL 调优"
install -m 644 "$HERE/my.cnf" /etc/my.cnf.d/jxc.cnf
systemctl enable --now mysqld

echo "==> 建库与账号"
# 注意：heredoc 未加引号，反引号会被 bash 当命令替换执行（库名会被吃掉），必须写成 \` 。
mysql -uroot <<SQL
CREATE DATABASE IF NOT EXISTS \`$DB_NAME\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
CREATE USER IF NOT EXISTS '$DB_USER'@'127.0.0.1' IDENTIFIED BY '$JXC_DB_PASSWORD';
ALTER USER '$DB_USER'@'127.0.0.1' IDENTIFIED BY '$JXC_DB_PASSWORD';
GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL

echo "==> nginx 与 systemd"
install -m 644 "$HERE/nginx-jxc.conf" /etc/nginx/conf.d/jxc.conf
install -m 644 "$HERE/jxc.service" /etc/systemd/system/jxc.service
install -m 755 "$HERE/upgrade.sh" /opt/jxc/bin/upgrade.sh
install -m 755 "$HERE/reset-demo.sh" /opt/jxc/bin/reset-demo.sh

# nginx 的 worker 以 nginx 用户运行，必须能「穿过」/opt/jxc（750 root:jxc）才能读 web/；
# 只靠 upgrade.sh 里的 chmod -R a+rX 不够——父目录不可穿过，子文件权限再松也没用。
# 少了这一步，首页会 500（try_files 回落到读不到的 index.html → 内部重定向循环）。
if id -u nginx >/dev/null 2>&1; then
  usermod -aG "$APP_USER" nginx
fi

systemctl daemon-reload
systemctl enable --now nginx
nginx -t && systemctl restart nginx

if command -v getenforce >/dev/null 2>&1 && [ "$(getenforce)" = "Enforcing" ]; then
  echo "!! SELinux = Enforcing：nginx 会读不到 /opt/jxc/web、也连不上 127.0.0.1:8888。"
  echo "   按 docs/35 第六节放行后重启 nginx 与 jxc，或确认镜像本就该关 SELinux。"
fi

echo "==> 完成。下一步："
echo "  1) 上传二进制与前端到 /tmp，再执行 /opt/jxc/bin/upgrade.sh"
echo "  2) 写 /opt/jxc/server/config.yaml（chmod 640 root:jxc）"
echo "  3) 初始化数据库与权限，见 docs/35 第 4 节"
