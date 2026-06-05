# GitHub 开源发布清单

开源前不要急着把当前开发目录直接上传。建议按下面顺序检查。

## 必须确认

- [ ] 没有提交 `config.yaml`
- [ ] 没有提交 `cookies.txt`
- [ ] 没有提交 `history.json`
- [ ] 没有提交 `download/`
- [ ] 没有提交 `logs/`
- [ ] 没有提交 OAuth token、Cookie、个人路径
- [ ] 示例配置使用相对路径
- [ ] README 能说明 GUI、CLI、Web 三种入口
- [ ] SECURITY.md 说明 Web 面板和 Cookie 风险
- [ ] PRIVACY.md 说明本地数据和录制内容风险
- [ ] 选择并添加 LICENSE

## 建议确认

- [ ] release 包和源码仓库分开
- [ ] 旧的实验文件不出现在首版 release
- [ ] `streamserver.php` 等 legacy fallback 在文档中标注为兼容路径
- [ ] README 明确推荐 OAuth 官方 API
- [ ] Web 面板默认只绑定 `127.0.0.1`
- [ ] CLI 和 GUI 都能跑 `doctor` 或健康检查

## 推荐 release 结构

```text
TwitCastingRecorder-vX.Y.Z-windows/
  twitcasting-recorder-gui.exe
  twitcasting-recorder.exe
  recorder-worker.exe
  config.example.yaml
  README.txt
```

不要把自己的 `config.yaml`、`cookies.txt`、`history.json` 放进去。

## License 提醒

GitHub 上没有 LICENSE 时，默认并不等于真正开源。别人通常不能明确复用、修改或分发。

常见选择：

- MIT：最宽松，适合个人工具。
- Apache-2.0：宽松，并带专利条款。
- GPL-3.0：要求衍生项目也开源。

选择许可证前先确认你希望别人怎么使用这个项目。
