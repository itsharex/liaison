# WebSSH 文件 HTTP API

文件流使用自定义 HTTP handler，不走 protobuf JSON 转换。所有路由要求 Bearer
认证；会话 ID 不是认证凭据，不接受 URL 中的登录令牌。

WebSFTP 连接配置使用 `POST`（新建）/`PUT`（编辑）
`/api/v1/webssh/proxies/{id}/credential`，输入见 `api/websftp.proto`。
仅允许 WebSFTP 访问，按当前用户隔离。`remember_password=false` 清除密文但保留配置；
`true` 且密码为空仅允许保留已有密码。名称和密码可编辑，账号为连接标识，不能改名。
目标接口返回的 `credentials` 增加 `name`，`saved` 表示是否保存密码，不表示登录验证成功。

| 方法 | 路由 | 输入 / 输出 |
|---|---|---|
| POST | `/api/v1/webssh/proxies/{id}/files/sessions` | JSON `{username,password?,use_saved_credential?}` → `{id,home,can_upload,expires_at}` |
| DELETE | `/api/v1/webssh/files/sessions/{session}` | 关闭自己的文件会话并取消当前传输 |
| GET | `/api/v1/webssh/files/sessions/{session}/list?path=...` | 文件列表 |
| GET | `/api/v1/webssh/files/sessions/{session}/preview?path=...` | JSON `{text}`，UTF-8，256 KiB 上限 |
| GET | `/api/v1/webssh/files/sessions/{session}/download?path=...` | attachment 字节流，首版 64 MiB 上限 |
| PUT | `/api/v1/webssh/files/sessions/{session}/upload?path=...` | 原始文件字节流，64 MiB 上限，不覆盖 |

非流式响应为 `{code,message,data}`；错误额外含 `reason` 与 `request_id`。
会话绑定登录用户、登录凭据摘要、访问及 SSH 用户名，30 分钟到期，每用户最多 4 个，
全局最多 64 个，每会话同时只允许一项操作。每次操作重新鉴权；传输中每秒复查权限。
文件会话只保留加密后的 SSH 凭据，每次操作创建独立 SSH 连接并结束时关闭。
文件浏览要求 `webssh.files.read`，上传还要求 `webssh.files.upload`。
管理员默认拥有，普通用户默认不拥有。上传权限不能脱离浏览权限单独授予。

错误 reason 包括 `SFTP_FAILED`、`SFTP_NOT_FOUND`、`SFTP_FORBIDDEN`、
`SFTP_BUSY`、`SFTP_INVALID`、`SFTP_TOO_LARGE`、`SFTP_BINARY`、`SFTP_EXISTS`、
`SFTP_UPLOAD_UNSUPPORTED`、`SFTP_TIMEOUT`。不向调用者返回上游原始错误。
