# CMDB 系统需求文档（v0.1）

> 对标蓝鲸 CMDB，团队生产自用。本文档为 MVP 阶段需求与技术设计基线，随开发迭代更新。

## 1. 项目目标

构建一个**元数据驱动（"一切皆模型"）**的配置管理数据库（CMDB），统一管理团队的各类 IT 资源（主机、网络设备、应用、数据库等），支持自定义模型、实例管理、业务拓扑组织，并具备生产级的权限控制与操作审计能力。

### 设计原则
- **模型驱动**：资源类型不硬编码，由用户通过"模型管理"自定义；实例是模型的具体化。
- **关系型底座 + 动态字段**：用 MySQL 保证事务、权限、审计的可靠性，用 JSON 字段承载动态属性。
- **可追溯**：所有写操作均产生审计记录。
- **可扩展**：MVP 聚焦四大模块，架构预留云同步、进程管理、自动发现等扩展点。

## 2. 技术栈

| 层 | 选型 |
|----|------|
| 后端 | Go + Gin（HTTP） + GORM（ORM） |
| 数据库 | MySQL 8.0（利用 JSON 类型 + 生成列索引） |
| 缓存 | Redis（会话、权限缓存，可选，二期） |
| 前端 | Vue 3 + Vite + Element Plus + Pinia + Vue Router |
| 认证 | JWT（自建用户体系，预留 LDAP/OAuth2 对接） |
| 部署 | 两种方式：① 主机（裸机）部署 ② Docker Compose，详见第 9 节 |

## 3. 核心概念

```
模型分组(ModelGroup)
   └─ 模型(Model)  例如：主机、交换机、MySQL实例、应用
        ├─ 字段(Attribute)  例如：IP、CPU核数、负责人、状态
        └─ 实例(Instance)   模型的具体记录
模型关联(Association)  模型A 与 模型B 的关系定义（如 "应用 运行于 主机"）
实例关联(InstanceRelation)  具体实例之间的关联数据

业务拓扑（内置模型）：
   业务(Business) → 集群(Set) → 模块(Module) → 主机(Host)
```

## 4. 功能需求（MVP）

### 4.1 模型管理
- **模型分组**：对模型分类（如"主机资源""网络""中间件"），可增删改查、排序。
- **模型定义**：
  - 创建/编辑/删除/停用模型，含名称、唯一标识(bk_obj_id)、图标、所属分组。
  - 内置模型（主机、业务、集群、模块）不可删除，字段可扩展。
- **字段管理**（模型的属性）：
  - 字段类型：单行文本、长文本、数字、浮点、枚举（单选/多选）、日期、时间、布尔、用户、列表、关联。
  - 字段属性：显示名、唯一标识、是否必填、是否唯一、默认值、是否可编辑、校验正则、分组（字段分栏）、排序。
  - 唯一校验：支持单字段唯一和联合唯一键。
- **模型关联定义**：
  - 定义模型间关系类型（1:1 / 1:N / N:N）与关系语义（如 belong / run / connect）。
  - 关联约束：源模型、目标模型、关系名称。

### 4.2 资源实例管理
- 基于任意模型的实例 **增 / 删 / 改 / 查**。
- **列表页**：动态列（按模型字段渲染）、分页、排序、字段筛选、关键字搜索、自定义显示列。
- **详情页**：按字段分组展示；展示该实例的关联关系（关联拓扑）。
- **批量能力**：批量导入（Excel/CSV）、批量导出、批量编辑、批量删除。
- **唯一性校验**：写入时按模型定义的唯一键去重。
- **实例关联**：在实例间建立/解除关联，查看关联链路。

### 4.3 业务拓扑
- 内置拓扑层级：**业务 → 集群(Set) → 模块(Module) → 主机**。
- 业务管理：创建业务、业务下维护集群与模块树（可视化树形操作）。
- **主机管理**：
  - 主机资源池（空闲机）：未分配到业务的主机集合。
  - 主机分配：资源池 → 业务模块（上架）。
  - 主机转移：业务内模块间转移、转移回资源池、主机回收。
  - 主机快照字段（IP、OS、CPU、内存、磁盘等内置字段）。
- 拓扑树检索：按业务查看完整拓扑，统计各节点主机数。

### 4.4 权限与审计
- **用户管理**：用户增删改查、启用/禁用、密码管理。
- **RBAC 权限模型**：
  - 角色（Role）：可配置的权限集合（如 管理员、运维、只读）。
  - 权限维度：按**模型**（对象级）× **操作**（查看/新增/编辑/删除）授权；业务按**业务实例**授权（用户只能看到被授权的业务）。
  - 用户 ↔ 角色多对多。
- **操作审计**：
  - 记录所有写操作：操作人、时间、操作类型、资源类型、资源ID、变更前后内容(diff)、来源IP。
  - 审计日志查询：按时间、操作人、资源类型、操作类型筛选。

## 5. 数据模型设计（关键）

### 5.1 动态字段落地方案：JSON 字段
采用"**元数据表 + 实例表(JSON 属性列)**"方案，而非每模型一张物理表或 EAV：

- `model` / `attribute`：存模型与字段的元数据定义。
- `instance`：所有实例统一存储，`data` 列为 MySQL JSON 类型，承载动态字段值。
- 高频查询/筛选字段：通过 MySQL **生成列(generated column)** 从 JSON 抽取并建索引，兼顾灵活性与性能。

### 5.2 核心表（初稿）

| 表 | 关键字段 | 说明 |
|----|----------|------|
| `model_group` | id, name, code, index | 模型分组 |
| `model` | id, group_id, bk_obj_id, name, icon, is_builtin, enabled | 模型定义 |
| `attribute` | id, model_id, bk_property_id, name, type, required, unique, options(JSON), default, editable, regex, group, index | 字段定义 |
| `association_kind` | id, name, src_to_dst_note, dst_to_src_note | 关联类型 |
| `model_association` | id, kind_id, src_model_id, dst_model_id, mapping(1:1/1:n/n:n) | 模型间关联定义 |
| `instance` | id, model_id, data(JSON), creator, updater, created_at, updated_at | 实例（动态属性） |
| `instance_relation` | id, association_id, src_inst_id, dst_inst_id | 实例关联数据 |
| `business` / `set` / `module` | 层级字段 + parent_id | 业务拓扑节点 |
| `host` | 内置字段 + business_id/module 归属 | 主机（也是一种 instance，内置模型） |
| `user` | id, username, password_hash, display_name, email, status | 用户 |
| `role` | id, name, code, description | 角色 |
| `role_permission` | role_id, model_id, actions(JSON) | 角色对象级权限 |
| `user_role` | user_id, role_id | 用户角色关联 |
| `audit_log` | id, user, action, resource_type, resource_id, before(JSON), after(JSON), ip, created_at | 审计日志 |

## 6. 非功能需求
- **安全**：密码加盐哈希(bcrypt)；JWT 鉴权；接口级权限校验中间件。
- **性能**：列表接口分页 + 索引优化；审计写入异步化（二期）。
- **可维护**：统一错误码、统一响应结构、请求日志。
- **部署**：支持主机部署与 docker-compose 两种方式（详见第 9 节）；数据库迁移工具（GORM AutoMigrate 或 golang-migrate）。

## 7. MVP 不包含（二期及以后）
- 云资源自动发现与同步（云厂商 API）
- 进程管理、服务实例
- 动态分组
- 操作审计异步化与归档
- IAM 对接 / 第三方 SSO / LDAP
- 拓扑关系图高级可视化（力导向图）

## 8. 已确认的关键决策
1. **认证方式**：MVP 采用自建用户名密码体系（JWT），LDAP/SSO 对接放二期。
2. **权限粒度**：MVP 做到"角色 × 模型对象 × 操作"+"业务实例级"授权；字段级权限放二期。
3. **动态字段方案**：采用"JSON 列 + 生成列索引"方案（实例统一存储，高频字段抽生成列建索引）。
4. **主机建模**：主机作为内置模型的实例，统一纳入 `instance` 体系；拓扑归属关系（业务/集群/模块）单独维护。

## 9. 部署方案

系统同时支持两种部署方式，共用同一套配置规范（环境变量 + `config.yaml`，环境变量优先级更高）。

### 9.1 主机（裸机）部署
适用于已有服务器 / 已有 MySQL 实例、不希望引入容器的场景。利用 Go 静态编译无运行时依赖的优势。

- **后端**：交叉编译为单个静态二进制 `cmdb-server`，随附 `config.yaml`；通过 `systemd` 托管（提供 `cmdb.service` 单元文件，支持开机自启、日志、自动重启）。
- **前端**：`npm run build` 产出静态文件，由 **Nginx** 托管并反向代理 `/api` 到后端；提供示例 `nginx.conf`。
- **数据库**：使用用户已有的 MySQL，或按文档单独安装；启动时执行迁移建表。
- **交付物**：
  - `deploy/host/` 目录：`cmdb.service`、`nginx.conf`、`config.example.yaml`、安装脚本 `install.sh`（建用户、放二进制、装 service、初始化 DB）。
  - 构建脚本 `scripts/build.sh`：一键产出后端二进制 + 前端静态包。

### 9.2 Docker Compose 部署
适用于快速起全套、开发测试或没有现成 MySQL 的场景。

- **服务**：`mysql`、`backend`、`frontend`(nginx 托管静态文件并反代后端)，可选 `redis`（二期）。
- **交付物**：
  - `backend/Dockerfile`（多阶段构建，产出最小镜像）、`frontend/Dockerfile`（build + nginx）。
  - 根目录 `docker-compose.yml`：一键 `docker compose up -d` 起全套，含数据卷持久化、健康检查、网络编排。
  - `.env.example`：端口、数据库账密、JWT 密钥等配置。

### 9.3 配置与迁移统一
- 配置来源优先级：环境变量 > `config.yaml` > 内置默认值，两种部署方式共用。
- 数据库迁移：后端启动时自动执行建表与内置数据初始化（内置模型、超级管理员账号），两种方式一致。
