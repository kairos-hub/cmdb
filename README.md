# CMDB 配置管理平台

对标蓝鲸 CMDB 的轻量级配置管理数据库，采用**元数据驱动（"一切皆模型"）**设计，统一管理团队的各类 IT 资源。

> 当前为 **阶段 0：项目骨架**。后续按模型管理 → 资源实例 → 业务拓扑 → 权限审计迭代。需求详见 [`docs/requirements.md`](docs/requirements.md)。

## 技术栈

| 层 | 选型 |
|----|------|
| 后端 | Go 1.25 + Gin + GORM |
| 数据库 | MySQL 8.0（JSON 字段承载动态属性） |
| 前端 | Vue 3 + Vite + Element Plus + Pinia + Vue Router |
| 认证 | JWT |
| 部署 | Docker Compose / 主机（systemd + Nginx） |

## 项目结构

```
cmdb/
├── backend/              # Go 后端
│   ├── cmd/server/       # 程序入口
│   ├── internal/
│   │   ├── config/       # 配置加载（env > yaml > 默认）
│   │   ├── database/     # GORM 连接与迁移
│   │   ├── handler/      # HTTP handler
│   │   ├── middleware/   # 日志、CORS 等中间件
│   │   ├── model/        # 数据模型（随业务模块填充）
│   │   └── router/       # 路由注册
│   ├── pkg/              # response、logger 等通用包
│   ├── config.yaml       # 配置文件
│   └── Dockerfile
├── frontend/             # Vue3 前端
│   ├── src/
│   │   ├── api/          # axios 封装
│   │   ├── layouts/      # 主布局
│   │   ├── router/       # 路由与守卫
│   │   ├── store/        # Pinia
│   │   └── views/        # 页面
│   ├── nginx.conf        # 容器内 nginx 配置
│   └── Dockerfile
├── deploy/host/          # 主机部署：systemd、nginx、install.sh
├── scripts/build.sh      # 构建主机发布包
├── docker-compose.yml
├── .env.example
└── docs/requirements.md  # 需求文档
```

## 快速开始

### 方式一：Docker Compose（推荐）

```bash
cp .env.example .env        # 按需修改数据库密码、端口、JWT 密钥
docker compose up -d --build
```

启动后访问 `http://<服务器IP>:<HTTP_PORT>`（默认 80）。健康检查：`curl http://localhost/api/health`。

### 方式二：主机部署

在有 Go 与 Node 的构建机上打包，再上传到目标主机安装：

```bash
# 1. 构建发布包（产出到 release/）
bash scripts/build.sh

# 2. 将 release/ 上传到目标主机，以 root 运行
cd release && ./install.sh

# 3. 按提示配置数据库，启动服务
vi /opt/cmdb/config.yaml
systemctl start cmdb

# 4. 配置 Nginx
cp release/nginx.conf /etc/nginx/conf.d/cmdb.conf
nginx -t && nginx -s reload
```

### 开发模式

```bash
# 后端（需本地 MySQL，配置见 backend/config.yaml）
cd backend && go run ./cmd/server

# 前端（自动代理 /api 到 127.0.0.1:8080）
cd frontend && npm install && npm run dev
```

## 配置说明

配置优先级：**环境变量 > 配置文件 > 内置默认值**，两种部署方式共用。常用环境变量：

| 变量 | 说明 |
|------|------|
| `CMDB_DB_HOST` / `CMDB_DB_PORT` | 数据库地址 |
| `CMDB_DB_USER` / `CMDB_DB_PASSWORD` / `CMDB_DB_NAME` | 数据库账号 |
| `CMDB_SERVER_PORT` / `CMDB_SERVER_MODE` | 服务端口 / 模式(debug\|release) |
| `CMDB_JWT_SECRET` | JWT 密钥（生产必改） |
| `CMDB_LOG_LEVEL` / `CMDB_LOG_FORMAT` | 日志级别 / 格式(text\|json) |
