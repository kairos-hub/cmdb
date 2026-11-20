#!/usr/bin/env bash
# CMDB 主机信息采集上报脚本（Linux）
# 用法：
#   CMDB_URL=http://<cmdb>:8080 CMDB_USER=collector CMDB_PASS=xxxxx bash collect_host.sh
# 定时（每小时上报一次）：
#   crontab -e 加一行：
#   0 * * * * CMDB_URL=http://<cmdb> CMDB_USER=collector CMDB_PASS=xxx bash /opt/cmdb/collect_host.sh >/var/log/cmdb_collect.log 2>&1
# 说明：
#   - 按「内网IP」做 upsert：主机已存在则更新采集字段，不存在则新建；备注/业务等手工字段不受影响。
#   - 采集账号需拥有「主机」模型的新增/编辑数据权限（可在角色管理里建个采集角色并绑定）。
set -euo pipefail

CMDB_URL="${CMDB_URL:-http://127.0.0.1:8080}"
CMDB_USER="${CMDB_USER:-collector}"
CMDB_PASS="${CMDB_PASS:-}"

# ---- 采集本机信息 ----
ip="$(ip -4 route get 1 2>/dev/null | awk '{print $7; exit}')"
[ -z "${ip:-}" ] && ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
name="$(hostname)"
os="$( . /etc/os-release 2>/dev/null; echo "${PRETTY_NAME:-$(uname -s)}" )"
cpu="$(nproc 2>/dev/null || echo 0)"
# 内存，单位 MB（主机模型字段「内存(MB)」）
mem="$(awk '/MemTotal/{printf "%d", $2/1024}' /proc/meminfo 2>/dev/null || echo 0)"
# 磁盘总量，单位 GB（所有物理盘之和）
disk="$(lsblk -bdno SIZE 2>/dev/null | awk '{s+=$1} END{printf "%d", s/1024/1024/1024}')"
[ -z "${disk:-}" ] || [ "${disk}" = "0" ] && disk="$(df -BG --total 2>/dev/null | awk '/total/{gsub("G","",$2); print $2}')"

if [ -z "${ip:-}" ]; then
  echo "无法获取内网IP，退出"; exit 1
fi

# ---- 登录获取 token ----
token="$(curl -s "${CMDB_URL%/}/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"${CMDB_USER}\",\"password\":\"${CMDB_PASS}\"}" \
  | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
if [ -z "${token:-}" ]; then
  echo "登录失败，请检查 CMDB_URL / CMDB_USER / CMDB_PASS"; exit 1
fi

# ---- 上报 ----
payload="{\"data\":{\"bk_host_innerip\":\"${ip}\",\"bk_host_name\":\"${name}\",\"bk_os_name\":\"${os}\",\"bk_cpu\":${cpu:-0},\"bk_mem\":${mem:-0},\"bk_disk\":${disk:-0}}}"
echo "上报：${payload}"
curl -s "${CMDB_URL%/}/api/instances/collect" \
  -H "Authorization: Bearer ${token}" \
  -H 'Content-Type: application/json' \
  -d "${payload}"
echo
