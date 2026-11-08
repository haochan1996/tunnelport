#!/bin/sh
# tunnelport 容器入口
# 角色：TP_ROLE=serve | connect（默认 connect）
set -e

TP="/usr/local/bin/tp"
RATHOLE="/usr/local/bin/rathole"

case "${TP_ROLE}" in
  serve)
    # 容器内端口映射必须预先确定：优先基座三连 TP_PORT_BASE（推荐，防火墙放行一段），
    # 或单独指定 TP_PORT。
    if [ -z "${TP_PORT_BASE}" ] && [ -z "${TP_PORT}" ]; then
      echo "错误：容器模式必须指定端口。推荐基座三连（防火墙放行 30123-30125 一段）："
      echo "  docker run -d --restart unless-stopped \\"
      echo "    -e TP_ROLE=serve -e TP_PORT_BASE=30123 -e TP_TOKEN=<52位token> \\"
      echo "    -p 30123-30125:30123-30125 -v tp-state:/root/.tunnelport tunnelport"
      exit 1
    fi
    set -- serve --bind "${TP_BIND:-0.0.0.0}" --rathole "$RATHOLE"
    if [ -n "${TP_PORT_BASE}" ]; then
      set -- "$@" --port-base "${TP_PORT_BASE}"
    else
      set -- "$@" --port "${TP_PORT}"
    fi
    [ -n "${TP_TOKEN}" ] && set -- "$@" --token "${TP_TOKEN}"
    [ -n "${TP_ENTRY_HOST}" ] && set -- "$@" --entry-host "${TP_ENTRY_HOST}"
    exec "$TP" "$@"
    ;;
  connect)
    if [ -z "${TP_TOKEN}" ]; then
      echo "错误：需要 TP_TOKEN（完整配对 URL：tunnelport://host:port#token）"
      exit 1
    fi
    set -- connect --token "${TP_TOKEN}" --rathole "$RATHOLE"
    [ -n "${TP_LOCAL_PORT}" ] && set -- "$@" --local-port "${TP_LOCAL_PORT}"
    [ -n "${TP_LOCAL_HOST}" ] && set -- "$@" --local-host "${TP_LOCAL_HOST}"
    [ -n "${TP_HOST}" ] && set -- "$@" --host "${TP_HOST}"
    [ -n "${TP_CONTROL_PORT}" ] && set -- "$@" --port "${TP_CONTROL_PORT}"
    exec "$TP" "$@"
    ;;
  *)
    echo "错误：TP_ROLE 只能是 serve 或 connect（当前：${TP_ROLE}）"
    exit 1
    ;;
esac
