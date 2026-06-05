# Worker 进程隔离拆分计划

目标：把“一个 GUI 进程里同时管理多路录制”的模型，逐步改造成“主 GUI 管理多个单主播 worker 进程”。这样可以保留统一界面，同时获得“录画君多开窗口”式的故障隔离。

## 模块 1：独立 recorder-worker 入口

状态：已完成第一版。

内容：

- 新增 `apps/gui/cmd/recorder-worker`。
- worker 从 JSON 任务文件读取单次录制参数。
- worker 调用现有 `pkg/recorder.RecordLiveStreamWithOptions()`。
- worker 通过 stdout 输出 JSON Lines 状态事件。

边界：

- 暂不接入 GUI scheduler。
- 暂不负责直播检测，只录制一个已经解析好的 `stream_url`。
- 暂不做自动重连，自动重连仍由现有 scheduler/recorder 流程负责。

收益：

- 先得到一个可单独运行、可测试、可打包的单主播录制进程。
- 后续 GUI 调度器切换到 worker 时，不需要同时改 recorder 核心逻辑。

### worker job 示例

示例文件：`apps/gui/worker-job.example.json`

运行方式：

```powershell
cd apps\gui
go run .\cmd\recorder-worker --job .\worker-job.example.json
```

注意：第一版 worker 要求 `stream_url` 已经是解析好的 HLS 地址。后续模块会让 scheduler 负责生成 job，并自动传入真实直播流地址。

### stdout 事件协议

worker 通过 stdout 输出 JSON Lines，每行一个事件：

```json
{"type":"start","time":"2026-05-18T20:00:00+08:00","screen_id":"example_user","message":"recorder worker started"}
{"type":"status","time":"2026-05-18T20:00:01+08:00","screen_id":"example_user","status":"recording","message":"Recording 00:00:01"}
{"type":"log","time":"2026-05-18T20:00:02+08:00","message":"[example_user] Recording process starting"}
{"type":"result","time":"2026-05-18T20:10:00+08:00","screen_id":"example_user","duration":"00:10:00","file_path":"...","file_size":123456}
```

主 GUI 后续只需要读取这些行并映射到现有事件：

- `status` -> `NotifyStatus`
- `log` -> `NotifyAppLog`
- `result` -> `AddRecordingHistoryWithStatus`

## 模块 2：worker 管理器

状态：已完成第一版。

内容：

- 新增 `pkg/workerproc`。
- 主 GUI 可为每个主播启动一个 worker 进程。
- 管理 worker 生命周期：启动、停止、崩溃记录、进程清理。
- 读取 worker stdout JSON Lines 并转换成现有 `NotifyStatus` / `NotifyAppLog` / history 更新。

验收：

- 单个主播可通过 worker 录制。
- 停止时能优雅结束 worker 和 ffmpeg。
- worker 崩溃不会影响 GUI 主进程。

### worker manager 边界

第一版 manager 只提供通用进程管理能力：

- 生成临时 job 文件。
- 自动为 job 注入 `stop_file`。
- 启动 `recorder-worker.exe --job <job.json>`。
- 读取 stdout JSON Lines 并回调事件。
- 读取 stderr 并转成 `log` 事件。
- 停止时写入 `stop_file`，等待超时后再 kill worker。

它暂时不直接接 scheduler。模块 3 会把 scheduler 的录制启动路径切到 `pkg/workerproc.Manager`。

## 模块 3：scheduler 切换为 worker 模式

状态：已完成第一版。

内容：

- scheduler 检测到开播后，不再直接调用 recorder。
- scheduler 生成 worker job 并启动 worker。
- 每个主播最多一个 worker。
- 保留开关：`recording.worker_enabled`，便于回退到进程内录制。
- GUI 设置页可配置 worker 开关、worker 路径和开播检查间隔。
- 停止监控/删除主播/关闭 GUI 时，会尝试优雅停止对应 worker。

验收：

- 多主播同时开播时，主 GUI 只负责调度和展示。
- 单路 worker 异常不会让其他主播的录制状态乱掉。

## 模块 4：worker 自主状态机

状态：已完成第一版。

内容：

- worker 支持等待开播、录制、结束后等待下一次开播。
- worker 内部拥有自己的重连/轮询节奏。
- 主 GUI 只负责启动/暂停/展示，不再频繁参与单路状态机。
- worker 的 `monitor` 模式不再要求预先传入 `stream_url`，由 worker 自己轮询并解析直播地址。
- 每完成一段录制，worker 会输出 `result` 事件，GUI 据此写入历史记录。

验收：

- 行为接近录画君：一个 worker 长期盯一个主播。
- 直播结束后自动回到等待下一次开播。

## 模块 5：高级稳定性能力

状态：已完成第一版。

内容：

- 每主播最近日志缓存。
- worker 崩溃自动拉起策略。
- 每主播画质策略。
- 内置 ffmpeg 探测与下载/配置提示。
- `.ts` 兼容模式和 `.mp4` 后处理转封装。
- GUI 监控表展示每路最近错误和最近录制产物。
- 全局录制设置支持 `mkv`、`ts`、`mp4` 三种封装策略；主播可覆盖全局画质/封装。
- worker 异常退出后按退避节奏自动重启，手动暂停/删除/退出时不重启。
- 设置页支持 FFmpeg/FFprobe 检测，并提供 FFmpeg 下载页入口。
- 支持代理设置，直播检查、worker 检查和 ffmpeg 拉流会使用同一个代理地址。
- 支持 worker 连续失败熔断次数，避免网络或鉴权异常时无限重启。
- 支持每路日志落盘到输出目录的 `logs` 子目录，并在主播详情中查看最近日志。
- 支持录制完成后生成同名 `.txt` 直播信息文件，记录标题、主播、时间、产物、策略和结果。
- 监控表提供“详情”入口，用于查看单主播状态、最近错误、最近产物和日志目录。

验收：

- 多路录制长期运行时，主 GUI 事件压力保持低。
- 用户能清楚看到每一路 worker 的最后错误和录制产物。
