# Playground Backend

这是一个按 `DDD-lite` 思路搭的 Go 后端雏形。现在持久化已经从本地 JSON 切到 MySQL，适合继续往“多用户账号密码管理系统”方向扩展。

## 配置源

当前支持三层配置源，优先级从高到低是：

1. 系统环境变量
2. 本地 `.env`
3. Nacos 远程配置

也就是说，Nacos 适合做中心化默认配置；你本地临时覆盖时，`.env` 仍然能盖住它。

## 目录结构

```text
cmd/
  auth-service/
  app-service/
internal/
  application/
  config/
  domain/
  infrastructure/
    persistence/mysql/
  interfaces/http/
  platform/
migrations/
  001_init_accounts.sql
  002_init_credential_records.sql
```

## 设计取舍

- 先保持 `DDD-lite` 分层，不上过重框架。
- 对齐你现有的 Traefik 路由：
  - `auth-service` 提供 `/auth/login` 和 `/internal/auth/verify`
  - `app-service` 提供 `/api/v1/accounts` 和 `/api/v1/credentials`
- 持久化改为 MySQL，当前运行时会自动建表；同时我也补了一份 PostgreSQL 迁移参考，后面切库会顺很多。
- `accounts.id` 使用应用侧生成的字符串业务 ID，而不是数据库自增主键，这样更适合跨库兼容、对外暴露和后续扩展。
- 后台登录用户和业务账号凭据已经拆成两个领域，避免把“谁能登录后台”和“后台里管理的账号密码”混在一起。
- 日志沿用你提到的 `logMan` 路线适配版，默认彩色终端输出。

## 数据库准备

先创建数据库：

```sql
CREATE DATABASE playground
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;
```

默认配置会在启动时自动执行 `migrations/001_init_accounts.sql` 对应的建表逻辑。

如果你后面要切 PostgreSQL，可以参考：

- [001_init_accounts.postgresql.sql](/C:/Users/1/Desktop/projects/Playground/backend/playground/migrations/001_init_accounts.postgresql.sql:1)

## Nacos

已经接入了 Nacos 配置拉取，配置入口在：

- [internal/config/nacos.go](/C:/Users/1/Desktop/projects/Playground/backend/playground/internal/config/nacos.go:1)

我在本机实际探测到的结果是：

- `http://127.0.0.1:9000` 是 MinIO，不是 Nacos
- `http://127.0.0.1:8848` 才是可用的 Nacos
- `nacos / nacos` 登录在当前机器上可用

所以示例里默认写的是：

```powershell
PLAYGROUND_NACOS_ENABLED=false
PLAYGROUND_NACOS_SERVER_ADDRS=http://127.0.0.1:8848
PLAYGROUND_NACOS_GROUP=DEFAULT_GROUP
PLAYGROUND_NACOS_DATA_ID=playground-backend.properties
PLAYGROUND_NACOS_USERNAME=nacos
PLAYGROUND_NACOS_PASSWORD=nacos
PLAYGROUND_NACOS_FORMAT=properties
```

如果你要启用，把 `PLAYGROUND_NACOS_ENABLED` 改成 `true` 即可。

Nacos 配置内容建议直接存成 env 风格的 `properties`，示例文件在：

- [examples/nacos/playground-backend.properties](/C:/Users/1/Desktop/projects/Playground/backend/playground/examples/nacos/playground-backend.properties:1)

例如：

```properties
PLAYGROUND_DB_DSN=root:root@tcp(127.0.0.1:3306)/playground?parseTime=true&charset=utf8mb4&loc=Local
PLAYGROUND_TOKEN_SECRET=change-me-before-production
PLAYGROUND_LOG_FORMAT=text
```

## 环境变量

复制一份：

```powershell
Copy-Item .env.example .env
```

核心数据库配置：

```powershell
PLAYGROUND_DB_DRIVER=mysql
PLAYGROUND_DB_DSN=root:root@tcp(127.0.0.1:3306)/playground?parseTime=true&charset=utf8mb4&loc=Local
PLAYGROUND_DB_AUTO_MIGRATE=true
PLAYGROUND_APP_HTTP_ADDR=:8090
PLAYGROUND_CREDENTIAL_SECRET_KEY=change-me-before-production
```

## 启动

认证服务：

```powershell
go run ./cmd/auth-service
```

业务服务：

```powershell
go run ./cmd/app-service
```

第一次启动时，如果 `accounts` 表为空，会自动创建管理员账号：

- 用户名：`PLAYGROUND_BOOTSTRAP_ADMIN_USERNAME`
- 邮箱：`PLAYGROUND_BOOTSTRAP_ADMIN_EMAIL`
- 密码：`PLAYGROUND_BOOTSTRAP_ADMIN_PASSWORD`

## 已提供接口

### auth-service

- `GET /healthz`
- `POST /auth/login`
- `GET /auth/me`
- `POST /auth/logout`
- `ANY /internal/auth/verify`

登录请求体示例：

```json
{
  "login": "admin",
  "passwordBase64": "Q2hhbmdlTWUxMjMh"
}
```

### app-service

- `GET /healthz`
- `GET /api/v1/accounts`
- `POST /api/v1/accounts`
- `GET /api/v1/accounts/{id}`
- `PUT /api/v1/accounts/{id}`
- `PUT /api/v1/accounts/{id}/password`
- `DELETE /api/v1/accounts/{id}`
- `GET /api/v1/credentials?page=1&pageSize=10&keyword=github`
- `POST /api/v1/credentials`
- `GET /api/v1/credentials/{id}`
- `PUT /api/v1/credentials/{id}`
- `DELETE /api/v1/credentials/{id}`
- `GET /api/v1/credentials/{id}/secret`

## 日志

默认就是彩色文本终端日志：

```powershell
PLAYGROUND_LOG_FORMAT=text
PLAYGROUND_LOG_COLOR=true
```

如果要额外落盘：

```powershell
PLAYGROUND_LOG_FILE_ENABLED=true
PLAYGROUND_LOG_FILE_DIR=storage/logs
```

## 和前端/网关的对应关系

- 前端登录页默认发到 `/auth/login`
- Traefik 已把 `/auth` 路由给 `auth-service`
- Traefik 的 `forwardAuth` 会调用 `/internal/auth/verify`
- 业务接口放在 `/api/...`，由 `app-service` 提供

## 后续建议

1. 给凭据补标签、归档、收藏和批量操作。
2. 补 refresh token、登出、审计日志。
3. 如果你后面明确更偏 PostgreSQL，我可以再给你切一版 pg 仓储。
