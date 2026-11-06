#!/usr/bin/env bash
# 服务器端升级：替换二进制与前端产物并重启（root 执行）。
# 用法：upgrade.sh /tmp/jxc-server /tmp/web-dist.tar.gz
set -euo pipefail
BIN="${1:?usage: upgrade.sh <binary> <web.tar.gz>}"
WEB="${2:?usage: upgrade.sh <binary> <web.tar.gz>}"

install -m 755 "$BIN" /opt/jxc/server/jxc-server.new
mv -f /opt/jxc/server/jxc-server.new /opt/jxc/server/jxc-server

rm -rf /opt/jxc/web/*
tar -xzf "$WEB" -C /opt/jxc/web
chown -R root:jxc /opt/jxc/web
chmod -R a+rX /opt/jxc/web

systemctl restart jxc
sleep 2
systemctl --no-pager --lines=20 status jxc || true
