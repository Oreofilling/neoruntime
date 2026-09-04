# 快速开始

五分钟上手：登录、调用受保护接口、订阅事件流。

示例均使用 `curl` 访问地址为 `https://192.168.1.50` 的设备——请按实际主机调整。由于 TLS 证书是自签名的，远程调用需要加 `-k`。

## 1. 登录并获取令牌

```bash
curl -k -X POST https://192.168.1.50/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username": "admin", "password": "admin"}'
```

::: warning
登录路由是 `/api/login`——**没有 `/v1`** 段。上面的账号密码是内置默认值；请通过 `POST /api/v1/system/password` 修改。
:::

响应在标准信封中携带令牌：

```json
{
  "code": 0,
  "message": "Success",
  "data": { "token": "Bearer eyJhbGciOi...", "username": "admin" }
}
```

注意 `data.token` **已自带 `Bearer ` 前缀**——请原样放入 `Authorization` 请求头，不要再叠加一个 `Bearer`。

## 2. 调用受保护接口

```bash
TOKEN='Bearer eyJhbGciOi...'   # 此处粘贴 data.token

curl -k https://192.168.1.50/api/v1/system/info \
  -H "Authorization: $TOKEN"
```

```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "version": "1.0.2",
    "services": { "ai-runtime": true, "event-bus": true, "device-control": true, "app-manager": true }
  }
}
```

## 3. 上传并加载 AI 模型

```bash
# 上传并注册
curl -k -X POST https://192.168.1.50/api/v1/ai/models/upload \
  -H "Authorization: $TOKEN" \
  -F 'model=@yolov8s.hef'

# 加载进 AI 运行时（model_id 使用上一步返回值）
curl -k -X POST https://192.168.1.50/api/v1/ai/models/<model_id>/load \
  -H "Authorization: $TOKEN"
```

## 4. 订阅事件流（WebSocket）

WebSocket 端点通过查询参数传递令牌：

```javascript
const ws = new WebSocket(
  'wss://192.168.1.50/api/v1/events/stream?token=eyJhbGciOi...'
) //                                     ^ 原始令牌，不带 "Bearer " 前缀

ws.onmessage = (ev) => console.log(JSON.parse(ev.data))
```

::: tip
在设备本机则使用 `ws://localhost:8080/api/v1/events/stream?token=...`。
:::

## 5. 接下来

- 完整接口清单：[API 参考](/api-reference/zh/)。
- 密码 RSA 加密、令牌生命周期与吊销：[认证](/zh/authentication)。
- 业务错误码与响应信封：[错误与状态码](/zh/errors)。
