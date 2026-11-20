#!/usr/bin/env bash
# 在目标主机以 root 运行，从发布包（release/）安装 CMDB。
set -euo pipefail

PREFIX=/opt/cmdb
LOGDIR=/var/log/cmdb
SRC=$(cd "$(dirname "$0")" && pwd)

if [ "$(id -u)" != "0" ]; then
  echo "请以 root 运行此脚本" >&2
  exit 1
fi

echo ">> 创建 cmdb 系统用户与目录..."
if ! id cmdb >/dev/null 2>&1; then
  NOLOGIN=$(command -v nologin || echo /usr/sbin/nologin)
  useradd --system --no-create-home --shell "$NOLOGIN" cmdb
fi
mkdir -p "$PREFIX" "$LOGDIR"

echo ">> 安装后端二进制..."
install -m 0755 "$SRC/cmdb-server" "$PREFIX/cmdb-server"

echo ">> 安装配置文件（已存在则跳过，避免覆盖）..."
if [ ! -f "$PREFIX/config.yaml" ]; then
  install -m 0640 "$SRC/config.yaml" "$PREFIX/config.yaml"
  echo "   已生成 $PREFIX/config.yaml，请修改数据库密码与 JWT 密钥！"
else
  echo "   $PREFIX/config.yaml 已存在，保留原配置。"
fi

echo ">> 部署前端静态文件..."
rm -rf "$PREFIX/frontend"
cp -r "$SRC/frontend" "$PREFIX/frontend"

chown -R cmdb:cmdb "$PREFIX" "$LOGDIR"

echo ">> 安装 systemd 服务..."
install -m 0644 "$SRC/cmdb.service" /etc/systemd/system/cmdb.service
systemctl daemon-reload
systemctl enable cmdb.service

cat <<EOF

================ 安装完成 ================
后续步骤：
  1) 确认已创建数据库并授权：
       CREATE DATABASE cmdb DEFAULT CHARSET utf8mb4;
  2) 编辑配置：$PREFIX/config.yaml（数据库连接、JWT 密钥）
  3) 启动后端：  systemctl start cmdb
     查看状态：  systemctl status cmdb
     查看日志：  tail -f $LOGDIR/server.log
  4) 配置 Nginx：参考 $SRC/nginx.conf
       cp $SRC/nginx.conf /etc/nginx/conf.d/cmdb.conf
       nginx -t && nginx -s reload
=========================================
EOF
