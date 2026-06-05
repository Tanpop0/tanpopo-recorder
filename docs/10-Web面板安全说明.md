# Web 面板安全说明

Web 面板不是普通展示页，而是管理后台。

只要能打开 Web 面板，就可以：

- 添加主播
- 删除主播
- 开始监听
- 暂停监听
- 查看运行日志
- 修改当前使用的配置文件

所以它的安全边界等同于“谁能控制你的录制服务”。

## 默认安全姿势

默认命令：

```powershell
twitcasting-recorder web --config config.yaml --addr 127.0.0.1:8787
```

`127.0.0.1` 只允许本机访问。服务器部署时，可以用 SSH 隧道访问：

```bash
ssh -L 8787:127.0.0.1:8787 user@server
```

然后在自己电脑打开：

```text
http://127.0.0.1:8787
```

## 不建议的做法

不要直接这样暴露：

```powershell
twitcasting-recorder web --config config.yaml --addr 0.0.0.0:8787
```

除非你已经在前面加了 VPN、反向代理认证、HTTPS 或防火墙白名单。

## 推荐远程访问方式

按安全程度排序：

1. SSH 隧道
2. VPN / Tailscale / ZeroTier
3. Nginx / Caddy 反向代理 + Basic Auth + HTTPS
4. 只允许固定 IP 访问

## 后续可做的内置安全

如果以后需要公网或多人使用，可以加：

- `--web-token` 启动参数
- `web.token` 配置项
- 登录页
- CSRF token
- 只读访客模式
- 操作审计日志

目前第一版 Web 面板默认只监听本机，适合自用和服务器隧道访问。
