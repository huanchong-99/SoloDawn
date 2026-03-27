# SoloDawn 运维手册

> 版本: 0.0.153
> 更新日期: 2026-01-30

## 目录

1. [部署架构](#部署架构)
2. [配置参数](#配置参数)
3. [监控与告警](#监控与告警)
4. [备份与恢复](#备份与恢复)
5. [故障排查](#故障排查)
6. [性能调优](#性能调优)
7. [安全运维](#安全运维)

---

## 部署架构

### 系统架构图

```
┌─────────────────────────────────────────────────────────────┐
│                        Load Balancer                         │
│                    (Nginx / Traefik)                        │
└─────────────────────────┬───────────────────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          │               │               │
          ▼               ▼               ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│  SoloDawn      │ │  SoloDawn      │ │  SoloDawn      │
│  Server #1      │ │  Server #2      │ │  Server #N      │
│  (Port 3001)    │ │  (Port 3001)    │ │  (Port 3001)    │
└────────┬────────┘ └────────┬────────┘ └────────┬────────┘
         │                   │                   │
         └───────────────────┼───────────────────┘
                             │
                             ▼
                   ┌─────────────────┐
                   │    SQLite DB    │
                   │  (共享存储)      │
                   └─────────────────┘
```

### 单机部署

```bash
# 1. 准备目录
mkdir -p /opt/solodawn/{bin,data,logs,config}

# 2. 复制二进制文件
cp target/release/server /opt/solodawn/bin/

# 3. 创建配置文件
cat > /opt/solodawn/config/.env << EOF
SOLODAWN_ENCRYPTION_KEY=your-32-character-encryption-key
DATABASE_URL=sqlite:/opt/solodawn/data/solodawn.db
SERVER_PORT=3001
LOG_LEVEL=info
RUST_LOG=server=info,tower_http=debug
EOF

# 4. 创建 systemd 服务
cat > /etc/systemd/system/solodawn.service << EOF
[Unit]
Description=SoloDawn Server
After=network.target

[Service]
Type=simple
User=solodawn
Group=solodawn
WorkingDirectory=/opt/solodawn
EnvironmentFile=/opt/solodawn/config/.env
ExecStart=/opt/solodawn/bin/server
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 5. 启动服务
systemctl daemon-reload
systemctl enable solodawn
systemctl start solodawn
```

### Docker 部署

```dockerfile
# Dockerfile
FROM rust:1.75-slim as builder
WORKDIR /app
COPY . .
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/target/release/server /usr/local/bin/
COPY --from=builder /app/frontend/dist /app/frontend/dist
WORKDIR /app
EXPOSE 3001
CMD ["server"]
```

```yaml
# docker-compose.yml
version: '3.8'
services:
  solodawn:
    build: .
    ports:
      - "3001:3001"
    environment:
      - SOLODAWN_ENCRYPTION_KEY=${SOLODAWN_ENCRYPTION_KEY}
      - DATABASE_URL=sqlite:/data/solodawn.db
      - LOG_LEVEL=info
    volumes:
      - solodawn-data:/data
      - /var/run/docker.sock:/var/run/docker.sock
    restart: unless-stopped

volumes:
  solodawn-data:
```

---

## 配置参数

### 环境变量

| 变量名 | 必需 | 默认值 | 说明 |
|--------|------|--------|------|
| `SOLODAWN_ENCRYPTION_KEY` | ✓ | - | 32字符加密密钥 |
| `DATABASE_URL` | - | `sqlite:./data/solodawn.db` | 数据库连接字符串 |
| `SERVER_PORT` | - | `3001` | 服务端口 |
| `SERVER_HOST` | - | `0.0.0.0` | 监听地址 |
| `LOG_LEVEL` | - | `info` | 日志级别 |
| `RUST_LOG` | - | - | Rust 日志过滤器 |
| `SENTRY_DSN` | - | - | Sentry 错误追踪 |

### 日志级别

- `error`: 仅错误
- `warn`: 警告和错误
- `info`: 常规信息（推荐生产环境）
- `debug`: 调试信息
- `trace`: 详细追踪

### 数据库配置

SQLite 连接参数：

```
sqlite:/path/to/db?mode=rwc&cache=shared&_journal_mode=WAL
```

| 参数 | 说明 |
|------|------|
| `mode=rwc` | 读写创建模式 |
| `cache=shared` | 共享缓存 |
| `_journal_mode=WAL` | WAL 日志模式（推荐） |
| `_busy_timeout=5000` | 忙等待超时（毫秒） |

---

## 监控与告警

### 健康检查

```bash
# HTTP 健康检查
curl -f http://localhost:3001/api/health

# 响应示例
{
  "status": "healthy",
  "version": "0.0.153",
  "uptime_seconds": 3600,
  "database": "connected"
}
```

### Prometheus 指标（可选）

> 注意：当前版本默认未暴露 `/metrics` 端点。如需 Prometheus 指标，请先启用对应的 metrics 中间件或在网关层采集。

```bash
# 指标端点（如已启用）
curl http://localhost:3001/metrics
```

关键指标：

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `solodawn_http_requests_total` | Counter | HTTP 请求总数 |
| `solodawn_http_request_duration_seconds` | Histogram | 请求延迟 |
| `solodawn_websocket_connections` | Gauge | WebSocket 连接数 |
| `solodawn_workflows_total` | Counter | 工作流总数 |
| `solodawn_terminals_active` | Gauge | 活跃终端数 |
| `solodawn_db_query_duration_seconds` | Histogram | 数据库查询延迟 |

### 告警规则

```yaml
# prometheus-alerts.yml
groups:
  - name: solodawn
    rules:
      - alert: SoloDawnDown
        expr: up{job="solodawn"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "SoloDawn 服务不可用"

      - alert: HighErrorRate
        expr: rate(solodawn_http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "HTTP 5xx 错误率过高"

      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(solodawn_http_request_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P95 延迟超过 1 秒"

      - alert: TooManyConnections
        expr: solodawn_websocket_connections > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "WebSocket 连接数过多"
```

### 日志聚合

```yaml
# filebeat.yml
filebeat.inputs:
  - type: log
    enabled: true
    paths:
      - /opt/solodawn/logs/*.log
    json.keys_under_root: true
    json.add_error_key: true

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
  index: "solodawn-%{+yyyy.MM.dd}"
```

---

## 备份与恢复

### 数据库备份

```bash
#!/bin/bash
# backup.sh

BACKUP_DIR="/opt/solodawn/backups"
DB_PATH="/opt/solodawn/data/solodawn.db"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/solodawn_${DATE}.db"

# 创建备份目录
mkdir -p ${BACKUP_DIR}

# 使用 SQLite 在线备份
sqlite3 ${DB_PATH} ".backup '${BACKUP_FILE}'"

# 压缩备份
gzip ${BACKUP_FILE}

# 保留最近 30 天的备份
find ${BACKUP_DIR} -name "*.db.gz" -mtime +30 -delete

echo "Backup completed: ${BACKUP_FILE}.gz"
```

### 定时备份

```bash
# crontab -e
# 每天凌晨 2 点备份
0 2 * * * /opt/solodawn/scripts/backup.sh >> /var/log/solodawn-backup.log 2>&1
```

### 恢复流程

```bash
#!/bin/bash
# restore.sh

BACKUP_FILE=$1
DB_PATH="/opt/solodawn/data/solodawn.db"

if [ -z "$BACKUP_FILE" ]; then
    echo "Usage: restore.sh <backup_file>"
    exit 1
fi

# 停止服务
systemctl stop solodawn

# 备份当前数据库
mv ${DB_PATH} ${DB_PATH}.old

# 解压并恢复
if [[ $BACKUP_FILE == *.gz ]]; then
    gunzip -c ${BACKUP_FILE} > ${DB_PATH}
else
    cp ${BACKUP_FILE} ${DB_PATH}
fi

# 设置权限
chown solodawn:solodawn ${DB_PATH}

# 启动服务
systemctl start solodawn

echo "Restore completed from: ${BACKUP_FILE}"
```

### 配置备份

```bash
# 备份配置文件
tar -czvf /opt/solodawn/backups/config_$(date +%Y%m%d).tar.gz \
    /opt/solodawn/config/
```

---

## 故障排查

### 常见问题

#### 1. 服务无法启动

```bash
# 检查日志
journalctl -u solodawn -n 100 --no-pager

# 常见原因
# - 端口被占用
netstat -tlnp | grep 3001

# - 数据库文件权限
ls -la /opt/solodawn/data/

# - 环境变量未设置
cat /opt/solodawn/config/.env
```

#### 2. 数据库锁定

```bash
# 检查数据库状态
sqlite3 /opt/solodawn/data/solodawn.db "PRAGMA integrity_check;"

# 检查 WAL 文件
ls -la /opt/solodawn/data/solodawn.db*

# 强制检查点
sqlite3 /opt/solodawn/data/solodawn.db "PRAGMA wal_checkpoint(TRUNCATE);"
```

#### 3. WebSocket 连接失败

```bash
# 检查连接数限制
ulimit -n

# 增加文件描述符限制
echo "solodawn soft nofile 65535" >> /etc/security/limits.conf
echo "solodawn hard nofile 65535" >> /etc/security/limits.conf

# 检查防火墙
iptables -L -n | grep 3001
```

#### 4. 内存使用过高

```bash
# 检查内存使用
ps aux | grep server

# 检查数据库缓存
sqlite3 /opt/solodawn/data/solodawn.db "PRAGMA cache_size;"

# 减少缓存大小
sqlite3 /opt/solodawn/data/solodawn.db "PRAGMA cache_size = -2000;"  # 2MB
```

### 日志分析

```bash
# 查看错误日志
grep -i error /opt/solodawn/logs/server.log | tail -50

# 查看慢查询
grep "slow_query" /opt/solodawn/logs/server.log

# 统计请求状态码
grep "status=" /opt/solodawn/logs/server.log | \
    sed 's/.*status=\([0-9]*\).*/\1/' | sort | uniq -c | sort -rn
```

### 性能诊断

```bash
# CPU 分析
perf top -p $(pgrep server)

# 内存分析
valgrind --tool=massif /opt/solodawn/bin/server

# 网络分析
ss -s
ss -tnp | grep 3001
```

---

## 性能调优

### 系统参数

```bash
# /etc/sysctl.conf

# 网络优化
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_tw_reuse = 1

# 文件描述符
fs.file-max = 2097152
fs.nr_open = 2097152

# 应用配置
sysctl -p
```

### 数据库优化

```sql
-- 优化 SQLite 配置
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = -64000;  -- 64MB
PRAGMA temp_store = MEMORY;
PRAGMA mmap_size = 268435456;  -- 256MB

-- 分析表统计信息
ANALYZE;

-- 清理碎片
VACUUM;
```

### 连接池配置

```rust
// 推荐配置
SqlitePoolOptions::new()
    .max_connections(10)
    .min_connections(2)
    .acquire_timeout(Duration::from_secs(30))
    .idle_timeout(Duration::from_secs(600))
    .max_lifetime(Duration::from_secs(1800))
```

---

## 安全运维

### 密钥管理

```bash
# 生成新的加密密钥
openssl rand -base64 24 | tr -d '\n' | head -c 32

# 密钥轮换步骤
# 1. 生成新密钥
# 2. 停止服务
# 3. 运行密钥迁移脚本
# 4. 更新环境变量
# 5. 重启服务
```

### 访问控制

```bash
# 限制服务用户权限
useradd -r -s /bin/false solodawn
chown -R solodawn:solodawn /opt/solodawn
chmod 700 /opt/solodawn/config
chmod 600 /opt/solodawn/config/.env
```

### 日志审计

```bash
# 启用审计日志
export RUST_LOG="server=info,tower_http=info,audit=info"

# 审计日志格式
# {"timestamp":"2026-01-30T10:00:00Z","event":"workflow.created","user":"...","resource":"..."}
```

### 安全检查清单

- [ ] 加密密钥已设置且安全存储
- [ ] 数据库文件权限正确 (600)
- [ ] 配置文件权限正确 (600)
- [ ] 服务以非 root 用户运行
- [ ] 防火墙规则已配置
- [ ] TLS/SSL 已启用（生产环境）
- [ ] 日志不包含敏感信息
- [ ] 定期备份已配置
- [ ] 监控告警已配置

---

## 版本升级

### 升级步骤

```bash
# 1. 备份数据
/opt/solodawn/scripts/backup.sh

# 2. 下载新版本
wget https://releases.solodawn.io/v0.0.154/server -O /tmp/server

# 3. 停止服务
systemctl stop solodawn

# 4. 替换二进制文件
cp /tmp/server /opt/solodawn/bin/server
chmod +x /opt/solodawn/bin/server

# 5. 运行数据库迁移
/opt/solodawn/bin/server migrate

# 6. 启动服务
systemctl start solodawn

# 7. 验证
curl http://localhost:3001/api/health
```

### 回滚步骤

```bash
# 1. 停止服务
systemctl stop solodawn

# 2. 恢复旧版本二进制
cp /opt/solodawn/bin/server.backup /opt/solodawn/bin/server

# 3. 恢复数据库（如需要）
/opt/solodawn/scripts/restore.sh /opt/solodawn/backups/latest.db.gz

# 4. 启动服务
systemctl start solodawn
```

---

*本文档最后更新于 2026-01-30*
