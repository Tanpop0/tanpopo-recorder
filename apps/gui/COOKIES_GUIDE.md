# Cookie 使用说明

当某些 TwitCasting 直播或回放需要登录后才能观看时，普通 HLS 请求可能会录到登录提示画面，或在子播放列表上返回 `401 Unauthorized`。这时可以为对应主播启用 Cookie 鉴权。

## 获取 cookies.txt

1. 使用 Chrome 或 Edge 登录 TwitCasting，并确认当前账号可以观看目标直播。
2. 使用支持 Netscape 格式导出的 Cookie 工具，例如 `Get cookies.txt LOCALLY`。
3. 在 `twitcasting.tv` 页面导出 Cookie，并保存为 `cookies.txt`。
4. 在 GUI 的“设置 -> Cookie 设置”中点击“选择文件”，选择刚导出的文件。
5. 保存设置后，只对需要登录权限的主播设置“强制 Cookie”。

## 文件放在哪里

推荐通过 GUI 的“选择文件”按钮选择 Cookie 文件。也可以把 `cookies.txt` 放在程序运行目录，然后在设置里填写：

```text
cookies.txt
```

开发环境和正式打包环境的运行目录可能不同，所以不要在公开文档或配置里写自己的绝对路径。

## 安全提醒

- `cookies.txt` 等同于登录凭证，不要提交到 Git，也不要发给别人。
- 如果浏览器退出登录、账号权限变化，或 Cookie 过期，需要重新导出。
- 公开 issue、日志和截图时，请遮挡 Cookie 路径、账号信息和任何完整鉴权头。
