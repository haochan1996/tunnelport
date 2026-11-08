# tunnelport 容器镜像（FR-2.5）
# 构建上下文需要预置三个文件（CI 打镜像时由流水线准备）：
#   tp-linux        编译好的静态二进制
#   rathole         对应架构的 rathole 可执行文件（D-1：将来由 CI 自构建）
#   entrypoint.sh   入口脚本（本目录）
#
# 用法（serve，中继端）:
#   docker run -d --name tp-serve --restart unless-stopped \
#     -e TP_ROLE=serve -e TP_PORT=30123 -e TP_TOKEN=<52位token> \
#     -p 30123:30123 -p 6768:6768 -p 6769:6769 \
#     -v tp-state:/root/.tunnelport \
#     tunnelport:latest
#   （TP_TOKEN 留空则自动生成，从 docker logs 里读）
#
# 用法（connect，客户端）:
#   docker run -d --name tp-connect --restart unless-stopped \
#     -e TP_TOKEN=<tunnelport://REDACTED 完整配对URL> \
#     -v tp-state:/root/.tunnelport \
#     tunnelport:latest
FROM ubuntu:24.04

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY tp-linux /usr/local/bin/tp
COPY rathole /usr/local/bin/rathole
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /usr/local/bin/tp /usr/local/bin/rathole /entrypoint.sh

# 容器内角色默认 connect（PRD §6.1）；serve 必须显式 TP_ROLE=serve
ENV TP_ROLE=connect

EXPOSE 30123 6768 6769
ENTRYPOINT ["/entrypoint.sh"]
