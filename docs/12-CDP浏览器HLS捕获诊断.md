# CDP 浏览器 HLS 捕获诊断

这个工具用于排查“网页能看，但后端 HLS 录制拿到 401 或登录占位流”的主播。

它不会录制桌面，也不会改动正常录制链路。脚本只启动一个带 DevTools 调试端口的 Chrome/Edge 页面，监听真实网页播放器发出的 HLS 请求，并输出脱敏报告。

## 适用场景

- 某个主播网页能正常看，但录制文件显示“必须登录”。
- GUI 已设置 Cookie，ffmpeg 仍然在 `media.m3u8` 上返回 `401 Unauthorized`。
- 需要确认浏览器真实请求是否带 Cookie、请求头名称有哪些、HLS URL 是哪一类地址。

## 使用方式

在仓库根目录运行：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\cdp-capture-hls.ps1 -ScreenId example_user -Seconds 90
```

脚本会使用独立浏览器资料目录：

```text
apps\gui\build\browser-profile
```

第一次打开时如果没有登录 TwitCasting，请在这个新开的浏览器窗口里登录一次，然后重新运行脚本。

输出报告默认写入：

```text
apps\gui\build\cdp-capture
```

## 报告会包含什么

- 捕获到的 `.m3u8` / `livehls` 请求 URL。
- 请求和响应状态码。
- 请求头名称和响应头名称。
- 请求是否带 `Cookie` 头。
- 浏览器关联到请求的 Cookie 名称。
- 被浏览器阻止的 Cookie 名称。

报告不会保存 `Cookie`、`Authorization` 等敏感头的值。

## 结果怎么看

如果浏览器报告里 HLS 请求带 Cookie 且状态码是 `200`，而当前 ffmpeg 仍然 `401`，说明问题大概率在 ffmpeg 请求头/子请求 Cookie 传递方式。

如果浏览器报告里 HLS 请求也没有 Cookie 或状态码也是 `401`，说明独立浏览器资料目录没有有效登录，需要先在它里面登录 TwitCasting。

如果浏览器报告里出现额外的关键请求头名称，下一步可以把这些头安全地映射到后端录制链路，再做小范围验证。

如果捕获结果是 `0 HLS-related requests`，通常表示页面没有真正开始播放。请检查独立浏览器窗口是否已经登录、是否打开了正在直播的主播页、是否需要手动点击播放，然后重新运行脚本。
