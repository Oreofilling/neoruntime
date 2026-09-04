---
title: "认证"
---

API 使用 **JWT Bearer 令牌**。一个接口签发令牌；其余所有接口（下列公开接口除外）都要求在 `Authorization` 请求头携带令牌。

## 登录

`POST /api/login` ——注意完整路径：登录路由不在 `/api/v1` 分组内，**没有 `/v1`** 段。

```json
{
  "username": "admin",
  "password": "<密码>",
  "timestamp": 1725400000
}
```

- `username` / `password` ——必填。设备未配置凭据时，使用内置默认值 `admin` / `admin`。
- `timestamp` ——可选，Unix 秒，用于防重放。非零时，时钟偏差超出新鲜窗口的请求会被拒绝（`code 1005`）。省略或传 `0` 则跳过检查（兼容旧行为）。

成功时信封的 `data.token` 为 JWT，**已自带 `Bearer ` 前缀**：

```json
{ "code": 0, "message": "Success", "data": { "token": "Bearer eyJhbGciOi...", "username": "admin" } }
```

凭据错误返回 HTTP 401 与 `code 2000`。

## 使用令牌

将令牌原样放入 `Authorization` 请求头——**不要**再加一个 `Bearer`：

```bash
curl -k https://HOST/api/v1/system/stats -H "Authorization: Bearer eyJhbGciOi..."
```

**WebSocket** 与 **SSE** 端点不一定能设置请求头（`EventSource` 完全无法自定义请求头），因此改为在 `token` 查询参数中传递**原始令牌**（**不带** `Bearer ` 前缀）：

```
wss://HOST/api/v1/events/stream?token=eyJhbGciOi...
```

## 加密密码（可选，推荐）

客户端可以不通过 TLS 明文发送密码，而是用设备 RSA 公钥加密：

1. `GET /api/v1/auth/public-key`（无需认证）返回 PEM 公钥。
2. 使用 **RSA-2048 / PKCS#1 v1.5** 加密密码，密文 base64 编码。
3. 将 base64 密文放入 `password` 字段；`timestamp` 设为当前 Unix 时间，防止截获的密文被重放。

设备端的解密是宽松的：无法解密的值按明文处理，因此升级期间旧客户端仍可工作。

## 令牌生命周期与吊销

- JWT 签名密钥**每次启动重新生成**——设备重启后此前签发的所有令牌失效，重新登录即可。
- `POST /api/v1/logout` 在**服务端**吊销本次请求使用的令牌（令牌进入吊销列表，立即失效，而非等到过期）。

## 无需令牌的接口

| 接口 | 公开原因 |
| --- | --- |
| `GET /api/v1/system/health` | 存活探针。 |
| `GET /api/v1/auth/public-key` | 在会话令牌存在之前就要获取。 |
| `POST /api/login` | 签发令牌本身。 |
| `GET /api/v1/system/ota/status` | 升级进行期间，全新客户端也要能轮询安装进度。 |
| `GET /api/v1/system/os-upgrade/status` | 与 OTA 状态同理。 |

其他接口上缺失、无效或已吊销的令牌返回 HTTP 401 与业务码 `2000`（可区分时为 `2002` 过期 / `2003` 无效）。
