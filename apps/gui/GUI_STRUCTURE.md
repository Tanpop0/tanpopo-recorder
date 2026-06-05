# TwitCasting Recorder GUI 项目结构说明

本文件用来帮助你快速看懂 `twitcasting-recorder-gui` 这个子项目的结构，方便以后维护和重新打开 GUI。

## 1. 顶层文件和目录

- `main.go`
  - Wails 程序入口。
  - 嵌入前端打包后的静态文件（`frontend/dist`），创建 `App` 实例并启动窗口 + 托盘图标。

- `app.go`
  - 定义 `App` 结构体：
    - 保存 `config.Config`（配置）、`scheduler.ValidationManager`（定时监控 + 录制）、`history.HistoryManager`（录制历史）。
  - 提供一批导出方法给前端调用，例如：
    - `GetStreamers()` / `AddStreamer()` / `RemoveStreamer()`
    - `ToggleMonitoring()` / `SetAllMonitoring()`
    - `GetRecordingHistory()` / `AddRecordingHistory()` / `ClearRecordingHistory()` / `DeleteHistoryRecord()`
    - `GetConfig()` / `SaveSettings()` / `SelectDirectory()`
  - 通过 `NotifyStatus()` 把当前各主播的监控/录制状态推送给前端（`runtime.EventsEmit("streamer-status", ...)`）。

- `config.yaml`
  - GUI 使用的主配置文件。
  - 包含：
    - `streamers`: 需要监控的 TwitCasting 账号列表（`screen_id` + `schedule` + 昵称 + 头像）。
    - `output_directory`: 录制文件输出目录（目前指向仓库根目录下的 `download` 目录）。

- `cookies.txt`
  - 浏览器导出的登录 cookies。
  - 如果启用，录制时会读取 Netscape cookies 并转换成 Cookie 请求头交给 `ffmpeg`，用来解决“需要登录才能完整观看”的直播/回放。

- `COOKIES_GUIDE.md`
  - 说明如何从浏览器导出 cookies，并放到正确位置。

- `history.json`
  - GUI 中“录制历史”的持久化文件。
  - 由 `pkg/history` 自动读写，不需要手动编辑。

- `go.mod` / `go.sum`
  - Go 模块依赖定义。

- `wails.json`
  - Wails 项目配置：如何构建前端、输出 exe 名称等。

- `build/`
  - Wails 模板自动生成的构建相关文件和图标（`appicon.png` 等）。
  - 一般不需要手动修改。

- `frontend/`
  - 前端工程（基于 Vite + Vue 模板）：
    - `src/`：前端源码（组件、页面、API 调用）。
    - `dist/`：前端打包后的静态资源，Wails 会把这里嵌入 Go 程序。（`go:embed all:frontend/dist`）
    - `node_modules/`：前端依赖，体积大，可以通过 npm 重新安装。

- `pkg/`
  - 后端业务逻辑（纯 Go）：
    - `pkg/config`：配置读写。
    - `pkg/checker`：定时请求 TwitCasting 接口 + 页面，用来判断是否开播、获取直播标题。
    - `pkg/recorder`：调用 `ffmpeg` 录制直播流，支持稳定画质策略、Cookie 请求头、低频进度和日志过滤。
    - `pkg/scheduler`：基于 `robfig/cron` 的定时任务调度，按 `schedule` 周期性检查主播是否开播，决定是否启动录制。
    - `pkg/history`：录制历史记录的读写，数据保存在 `history.json`。
    - `pkg/metadata`：拉取主播昵称、头像等基础资料。

- 若干以 `screen_id` 命名的目录（例如 `fkuase/`、`kn_0_0_ng/` 等）
  - 这是旧版本或测试时生成的录制输出目录，每个目录下面是若干 `*_temp_时间戳.ts` 文件。
  - 这些只是录制结果样本，对程序逻辑没有影响。
  - 如果你已经把真正的输出目录改到仓库根目录的 `download/`，并且确认不再需要这些旧文件，可以手动备份或删除它们，让项目根目录更干净。

## 2. 程序运行流程（后端大致流程）

1. 启动：
   - `main.go` 中 `func main()` 创建 `App`，启动托盘 + 窗口，并绑定 `App` 的方法给前端。

2. 初始化：
   - `App.startup()`：
     - 读取 `config.yaml`，如果不存在则创建空配置。
     - 初始化 `scheduler.ValidationManager`，把 `App` 作为 `StatusNotifier` 注入。
     - 初始化 `history.HistoryManager`，指向 `history.json`。
     - 遍历配置里的每个 `streamer`，加入调度器，并默认设置为“监控暂停”（Idle），需要你在 GUI 手动开启监控。

3. 定时检查：
   - `pkg/scheduler/scheduler.go` 使用 `cron` 按各自 `Schedule` 周期触发 `checkAndRecord(screenID)`：
     - 如果该 `screenID` 被“暂停监控”，直接返回。
     - 如果已经在录制，同样直接返回，避免重复启动。
     - 否则调用 `pkg/checker.CheckStreamStatus()`：
       - 请求 `https://twitcasting.tv/streamserver.php?target=...&mode=client` 判断是否开播。
       - 再通过 HTML 抓取当前直播标题。

4. 发现开播后：
   - 调度器：
     - 通过 `App.NotifyStatus()` 把状态变更推给前端（显示为“正在录制 + 标题”）。
     - 在后台启动一个 goroutine 调用 `pkg/recorder.RecordLiveStreamWithOptions()` 真正录制。

5. 录制逻辑（`pkg/recorder/recorder.go`）：
   - 根据 `output_directory` + `screenID` 生成专属目录，例如 `download/kn_0_0_ng/`。
   - 构造临时文件名：`<screenID>_temp_<时间戳>.ts`，为避免 Python/Windows 编码问题先写到 ASCII 文件名。
   - 组合 `ffmpeg` 命令行参数，并附加 Referer、Origin、Cookie 请求头。
   - 启动 `ffmpeg`，实时读取 stdout/stderr：
     - 解析 `progress/out_time`，按秒更新“录制进度”状态给前端。
     - 高频 ffmpeg 噪声只在后端消费，不逐行推送给前端。
   - 命令结束后：
     - 尝试把临时文件重命名为：`<screenID>_<title>_<时间戳>.ts`。
     - 计算持续时间字符串、文件大小，并返回给调度器。

6. 录制结束后：
   - 调度器接收返回的 `duration`、`filePath`、`fileSize`：
     - 调用 `App.AddRecordingHistory()` 把这条记录写入 `history.json`。
     - 根据当前是否仍在监控，更新状态为“监控中，等待下次直播”。

## 3. 如何重新运行 / 打开 GUI

你现在有两种常用方式：

### 方式 A：直接运行已有 exe

- 文件：`twitcasting-recorder-gui.exe`
- 操作：
  - 直接双击运行；或
  - 在 PowerShell 中：
    ```powershell
    cd <repo>\apps\gui
    .\\twitcasting-recorder-gui.exe
    ```
- 注意：
  - 如果这个 exe 是很早之前构建的，而你后来改动了前端或后端代码，那么 exe 的行为可能和当前源码不一致。
  - 如果 exe 已经无法正常启动，建议采用“方式 B”重新构建一个新的 exe。

### 方式 B：使用 Wails 重新构建

前提：本机已安装 Wails v2 + Node.js（前端依赖）。

1. 在项目目录打开终端：
   ```powershell
   cd <repo>\apps\gui
   ```

2. （可选）开发模式运行：
   ```powershell
   wails dev
   ```
   - Wails 会：
     - 启动 Vite 前端 dev server；
     - 启动 Go 后端；
     - 打开一个带有热重载的开发窗口，适合调试前端和后端交互。

3. 构建正式 exe：
   ```powershell
   wails build
   ```
   - 构建完成后会在当前目录生成新的 `twitcasting-recorder-gui.exe`。
   - 之后就可以像“方式 A”那样直接双击 exe 使用。

---

如果你之后想继续优化或精简代码，可以以本文件为“地图”，优先关注：

- `pkg/checker`：判断直播是否开播、获取标题；
- `pkg/recorder`：具体录制逻辑和 ffmpeg 参数；
- `pkg/scheduler`：何时检查、何时启动/停止录制；
- `app.go`：GUI 与这些后端逻辑的“桥梁”。

其他目录（如历史输出的 `fkuase/`、`kn_0_0_ng/` 等）更多属于数据，不影响程序结构，可按需要清理。
