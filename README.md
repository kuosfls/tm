# tgsms

监听 Telegram 指定用户 / 群组 / 频道的消息,并实时 POST 到你指定的 Webhook 地址。

基于 [gotd/td](https://github.com/gotd/td)(纯 Go 的 Telegram MTProto 客户端库),以**个人账号**身份登录(不是机器人),编译为单个二进制文件,无需 Docker、无需运行时依赖,一条命令即可部署到 Linux VPS。

## 功能特性

- 以个人账号登录 Telegram(支持两步验证 2FA),会话持久化保存,重启不用重新登录
- 按白名单监听:只转发指定会话 ID(用户 / 普通群 / 超级群 / 频道)的消息
- 收到消息立即以 JSON 格式 POST 到固定地址,失败自动重试(2s / 5s / 15s)
- 断线自动重连,systemd 守护 + 开机自启
- `tgsms` 数字菜单管理(类似 x-ui):启停、日志、登录、改配置、更新、卸载
- 内置 `tgsms chats` 一键查看最近会话的 ID,不用再到处找 ID
- 支持 socks5 代理(可选)
- 单二进制部署,支持 amd64 / arm64

## 一键安装

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kuosfls/tm/master/install.sh)
```

安装完成后,输入 `tgsms` 打开管理菜单:

```
tgsms 管理脚本 (Telegram 消息转发到 Webhook)
——————————————————————————————
  0. 退出脚本
——————————————————————————————
  1. 查看服务状态
  2. 启动 tgsms
  3. 停止 tgsms
  4. 重启 tgsms
  5. 查看实时日志
——————————————————————————————
  6. 登录 Telegram 账号
  7. 查看最近会话 ID
  8. 配置管理 (api / webhook / 监听ID / 代理)
  9. 测试 webhook 推送
——————————————————————————————
 10. 设置开机自启
 11. 取消开机自启
 12. 更新 tgsms
 13. 卸载 tgsms
——————————————————————————————
服务状态: 运行中    开机自启: 是    版本: v1.0.0
```

## 使用步骤

1. **申请 API 凭据**:打开 <https://my.telegram.org> → API development tools,创建应用后得到 `api_id` 和 `api_hash`
2. **填写配置**:输入 `tgsms` → 菜单 `[8]` 配置管理,依次设置 api_id / api_hash、webhook 推送地址
3. **登录账号**:菜单 `[6]`,按提示输入手机号(含区号,如 `+86138...`)、验证码、2FA 密码
4. **添加监听会话**:菜单 `[7]` 查看最近会话的 ID,再到菜单 `[8]` → `[6]` 添加
5. **启动服务**:菜单 `[2]`,然后可用菜单 `[5]` 观察日志、菜单 `[9]` 测试 webhook 连通性

也可以不进菜单直接使用子命令:

```bash
tgsms status                          # 查看状态
tgsms log                             # 实时日志
tgsms config show                     # 查看配置
tgsms config set webhook_url https://example.com/hook
tgsms config add-chat -1001234567890  # 添加监听会话
tgsms restart
```

## 会话 ID 格式

与 Bot API 风格一致,`tgsms chats`(菜单 `[7]`)输出的就是这种格式,可直接使用:

| 会话类型 | ID 形式 | 示例 |
|---|---|---|
| 用户(私聊) | 正数 | `777000`(Telegram 官方通知号) |
| 普通群 | 负数 | `-12345678` |
| 超级群 / 频道 | `-100` 开头 | `-1001234567890` |

## Webhook 数据格式

每收到一条被监听会话的消息,tgsms 会向 `webhook_url` 发送一个 POST 请求:

```
POST <webhook_url>
Content-Type: application/json
X-TGSMS-Secret: <webhook_secret>   # 配置了密钥才会携带
```

```json
{
  "event": "message",
  "message_id": 12345,
  "chat_id": -1001234567890,
  "chat_type": "channel",
  "chat_title": "某个频道",
  "sender_id": 777000,
  "sender_name": "Telegram",
  "sender_username": "telegram",
  "text": "消息正文内容",
  "media": "photo",
  "date": 1754000000,
  "received_at": 1754000001
}
```

说明:

- `chat_type`:`user` 私聊 / `group` 群组 / `channel` 频道
- `media`:消息带附件时为 `photo`、`document` 等,纯文本消息无此字段;附件本身不会上传,带图消息的 `text` 为其配文
- 目标端返回非 2xx 或超时会自动重试 3 次,仍失败则丢弃并记录日志
- 建议目标端校验 `X-TGSMS-Secret` 请求头,防止伪造请求

## 配置文件

位于 `/etc/tgsms/config.yaml`(含登录会话 `session.json`,注意保密):

```yaml
api_id: 0
api_hash: ""
phone: ""                  # 可选, 留空则登录时交互输入
webhook_url: ""            # 消息推送目标地址
webhook_secret: ""         # 可选, 请求头 X-TGSMS-Secret
chat_ids: []               # 监听的会话 ID 列表
proxy: ""                  # 可选, socks5://127.0.0.1:1080
include_outgoing: false    # 是否也转发自己发出的消息
```

改完配置后 `tgsms restart` 生效。

## 手动编译

```bash
git clone https://github.com/kuosfls/tm.git
cd tm
go build -o tgsms .
./tgsms config init -c ./config.yaml
./tgsms login -c ./config.yaml
./tgsms run -c ./config.yaml
```

## Fork 后发布自己的版本

1. Fork / 上传本仓库到你的 GitHub 账号
2. 把 `install.sh` 和 `scripts/tgsms.sh` 顶部的 `REPO="kuosfls/tm"` 改成你自己的 `用户名/仓库名`(如果默认分支不是 `master`,同时修改 `BRANCH`)
3. 推送一个 tag 即可自动构建并发布 Release:

   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

   GitHub Actions 会自动交叉编译 amd64 / arm64 并上传 `tgsms-linux-*.tar.gz`,之后一键安装脚本即可使用。

## 注意事项

- 本项目使用**个人账号**收取消息。请遵守 [Telegram 服务条款](https://telegram.org/tos),不要用于骚扰、爬取隐私等滥用行为;滥用 API 可能导致账号受限
- `api_id` / `api_hash` / `session.json` 等同于你的账号凭据,泄露后他人可直接登录你的账号,请勿分享
- 登录、查看会话 ID 时脚本会短暂暂停转发服务(避免两个进程争用同一份会话),完成后自动恢复

## License

MIT
