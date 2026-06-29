<template>
  <div class="settings-container">
    <div class="settings-header">
      <h2>应用设置</h2>
      <p>配置录制输出、鉴权模式（OAuth/Cookie）以及授权信息。</p>
    </div>
    <div class="settings-summary">
      <div>
        <span>鉴权</span>
        <strong>{{ authSummary }}</strong>
      </div>
      <div>
        <span>评论</span>
        <strong>{{ form.recording.saveCommentsText ? '保存' : '关闭' }}</strong>
      </div>
      <div>
        <span>推送</span>
        <strong>{{ telegramReady ? '已配置' : '未启用' }}</strong>
      </div>
      <div>
        <span>Worker</span>
        <strong>{{ form.recording.workerEnabled ? '启用' : '关闭' }}</strong>
      </div>
    </div>

    <div class="settings-content">
      <el-form :model="form" label-position="top" class="settings-form">
        <div class="settings-section">
          <h3>录制设置</h3>
          <el-form-item label="视频保存路径">
            <div class="path-selector">
              <el-input v-model="form.outputDirectory" readonly placeholder="选择保存目录..." size="large">
                <template #prefix>
                  <el-icon><Folder /></el-icon>
                </template>
              </el-input>
              <el-button type="primary" size="large" @click="selectDir">浏览...</el-button>
            </div>
          </el-form-item>
          <el-form-item label="画质策略">
            <el-select v-model="form.recording.qualityMode" style="width: 100%">
              <el-option label="中档稳定 (推荐)" value="stable" />
              <el-option label="原始/最高" value="original" />
              <el-option label="高画质" value="high" />
              <el-option label="低画质" value="low" />
              <el-option label="自动尝试高/中/低" value="auto" />
              <el-option label="仅保存音频" value="audio" />
            </el-select>
            <div class="form-tip">“仅保存音频”会减小成品文件，但不保证减少下载流量。</div>
          </el-form-item>
          <el-form-item label="封装格式">
            <el-select v-model="form.recording.containerMode" style="width: 100%">
              <el-option label="MKV 稳定默认" value="mkv" />
              <el-option label="TS 兼容模式" value="ts" />
              <el-option label="MP4 后处理转封装" value="mp4" />
            </el-select>
          </el-form-item>
          <el-form-item>
            <el-checkbox v-model="form.recording.saveInfoText">每次录制旁边保存直播信息 txt</el-checkbox>
          </el-form-item>
          <el-form-item>
            <el-checkbox v-model="form.recording.saveCommentsText">每次录制旁边保存评论 JSONL（播放器回放数据）</el-checkbox>
          </el-form-item>
          <el-form-item>
            <el-checkbox v-model="form.recording.saveCommentsTextFile">额外保存评论 TXT（给人直接阅读，默认关闭）</el-checkbox>
          </el-form-item>
          <el-form-item v-if="form.recording.saveCommentsTextFile" label="评论 txt 格式模板">
            <el-input
              v-model="form.recording.commentTextTemplate"
              placeholder="[{offset}] {display_name}: {message}"
            />
            <div class="form-tip">可用变量：{offset}、{display_name}、{name}、{screen_id}、{user_id}、{message}、{created}、{id}</div>
          </el-form-item>
          <el-form-item label="短录制阈值 (秒)">
            <el-input-number v-model="form.recording.minDurationSeconds" :min="1" :max="3600" />
          </el-form-item>
          <el-form-item label="小文件阈值 (MB，0 为关闭)">
            <el-input-number v-model="form.recording.minFileSizeMB" :min="0" :max="1024" />
          </el-form-item>
          <el-form-item label="全部开始错峰间隔 (秒)">
            <el-input-number v-model="form.recording.startupStaggerSeconds" :min="0" :max="30" />
          </el-form-item>
          <el-form-item label="FFmpeg 路径">
            <el-input v-model="form.recording.ffmpegPath" placeholder="可填 ffmpeg.exe、bin 目录或 FFmpeg 解压目录；留空则自动查找" />
          </el-form-item>
          <el-form-item label="FFprobe 路径">
            <el-input v-model="form.recording.ffprobePath" placeholder="可填 ffprobe.exe、bin 目录或 FFmpeg 解压目录；留空则自动查找" />
          </el-form-item>
          <el-form-item>
            <el-button :loading="toolChecking" @click="checkTools">检测 FFmpeg / FFprobe</el-button>
            <el-button :loading="healthChecking" @click="runHealthCheck">一键健康检查</el-button>
            <el-button @click="openFFmpegDownload">打开 FFmpeg 下载页</el-button>
            <span class="tool-status" :class="{ ok: toolStatus.ffmpeg_ok, bad: toolStatus.message && !toolStatus.ffmpeg_ok }">
              {{ toolStatus.message }}
            </span>
          </el-form-item>
          <div v-if="healthReport.items.length > 0" class="health-panel">
            <div class="health-panel-head">
              <strong>健康检查</strong>
              <span :class="healthReport.ok ? 'ok' : 'bad'">{{ healthReport.ok ? '通过' : '需要处理' }}</span>
            </div>
            <div class="health-grid">
              <div v-for="item in healthReport.items" :key="item.name" class="health-item" :class="item.status">
                <span>{{ item.name }}</span>
                <strong>{{ statusText(item.status) }}</strong>
                <p>{{ item.message }}</p>
              </div>
            </div>
          </div>
          <el-form-item>
            <el-checkbox v-model="form.recording.workerEnabled">启用单主播 worker 进程隔离（实验）</el-checkbox>
          </el-form-item>
          <el-form-item label="Worker 路径">
            <el-input v-model="form.recording.workerPath" placeholder="留空则使用当前程序内置 worker；也可指定 recorder-worker.exe" />
          </el-form-item>
          <el-form-item label="Worker 等待开播检查间隔 (秒)">
            <el-input-number v-model="form.recording.workerCheckIntervalSeconds" :min="5" :max="300" />
          </el-form-item>
          <el-form-item label="Worker 连续失败熔断次数">
            <el-input-number v-model="form.recording.workerMaxRestarts" :min="1" :max="100" />
          </el-form-item>
        </div>

        <div class="settings-section">
          <h3>代理设置</h3>
          <el-form-item>
            <el-checkbox v-model="form.proxy.enabled">启用代理</el-checkbox>
          </el-form-item>
          <el-form-item label="代理地址">
            <el-input v-model="form.proxy.url" placeholder="例如: http://127.0.0.1:7890 或 socks5://127.0.0.1:1080" />
          </el-form-item>
        </div>

        <div class="settings-section">
          <h3>Telegram 推送</h3>
          <el-form-item>
            <el-checkbox v-model="form.notifications.telegram.enabled">启用 Telegram Bot 配置</el-checkbox>
          </el-form-item>
          <el-form-item label="Bot Token">
            <el-input v-model="form.notifications.telegram.botToken" type="password" show-password placeholder="123456:ABC-DEF..." />
          </el-form-item>
          <el-form-item label="Chat ID">
            <el-input v-model="form.notifications.telegram.chatId" placeholder="个人、群组或频道 chat_id" />
          </el-form-item>
          <div class="notify-options">
            <el-checkbox v-model="form.notifications.telegram.notifyOnStart">开录</el-checkbox>
            <el-checkbox v-model="form.notifications.telegram.notifyOnFinish">完成</el-checkbox>
            <el-checkbox v-model="form.notifications.telegram.notifyOnError">失败</el-checkbox>
          </div>
          <div class="notify-actions">
            <el-button :loading="telegramTesting" @click="testTelegram">测试推送</el-button>
            <span>录制事件只会发送给右侧“TG 推送”已开启的主播。</span>
          </div>
        </div>

        <div class="settings-section">
          <h3>鉴权模式</h3>
          <div class="auth-overview">
            <div :class="{ ready: hasOAuthToken }">
              <span>OAuth</span>
              <strong>{{ hasOAuthToken ? '已配置' : '未配置' }}</strong>
            </div>
            <div :class="{ ready: form.cookies.enabled }">
              <span>Cookie</span>
              <strong>{{ form.cookies.enabled ? '已启用' : '未启用' }}</strong>
            </div>
            <div :class="{ ready: form.proxy.enabled }">
              <span>代理</span>
              <strong>{{ form.proxy.enabled ? '已启用' : '未启用' }}</strong>
            </div>
          </div>
          <el-form-item label="鉴权策略">
            <el-select v-model="form.authMode" style="width: 100%">
              <el-option label="自动 (推荐)" value="auto" />
              <el-option label="仅 OAuth" value="oauth" />
              <el-option label="仅 Cookie" value="cookie" />
            </el-select>
            <div class="form-tip">自动模式：优先走 OAuth API 链路，会员/受限场景可叠加 Cookie。</div>
          </el-form-item>
        </div>

        <div class="settings-section">
          <h3>OAuth 设置</h3>
          <el-form-item label="Client ID">
            <el-input v-model="form.oauth.clientId" placeholder="TwitCasting App Client ID" />
          </el-form-item>
          <el-form-item label="Client Secret">
            <el-input v-model="form.oauth.clientSecret" type="password" show-password placeholder="TwitCasting App Client Secret" />
          </el-form-item>
          <el-form-item label="Redirect URI">
            <el-input v-model="form.oauth.redirectUri" placeholder="例如: urn:ietf:wg:oauth:2.0:oob" />
          </el-form-item>
          <el-form-item label="Access Token">
            <el-input v-model="form.oauth.accessToken" type="textarea" :rows="2" placeholder="授权成功后自动写入，也可手动粘贴" />
          </el-form-item>

          <div class="oauth-guide">
            <div class="oauth-guide-title">OAuth 获取教程（3 分钟）</div>
            <ol class="oauth-guide-steps">
              <li>在 TwitCasting 开发者平台创建应用，拿到 <code>Client ID</code> 和 <code>Client Secret</code>。</li>
              <li>填入本页；<code>Redirect URI</code> 建议使用 <code>urn:ietf:wg:oauth:2.0:oob</code>。</li>
              <li>先点“保存设置”，再点“打开授权页面”。</li>
              <li>浏览器登录并授权后，复制返回的 <code>code</code>（或回调 URL 的 <code>?code=...</code>）。</li>
              <li>把 <code>code</code> 粘贴到“Authorization Code (回填)”，点“用 Code 换 Token”。</li>
              <li>点“校验 Token”确认可用，完成。</li>
            </ol>
            <div class="oauth-guide-tip">
              失败排查：优先检查 <code>Client ID / Client Secret / Redirect URI</code> 与开发者平台配置是否完全一致。
            </div>
          </div>

          <div class="oauth-actions">
            <el-button type="primary" @click="openOAuthPage">打开授权页面</el-button>
            <el-button type="success" @click="verifyToken">校验 Token</el-button>
          </div>

          <el-form-item label="Authorization Code (回填)">
            <el-input v-model="form.oauth.code" placeholder="从授权回调页复制 code 填入这里" />
          </el-form-item>
          <el-button type="warning" @click="exchangeCode">用 Code 换 Token</el-button>
        </div>

        <div class="settings-section">
          <h3>Cookie 设置</h3>
          <el-form-item>
            <el-checkbox v-model="form.cookies.enabled">启用 cookies.txt（会员/受限内容建议开启）</el-checkbox>
          </el-form-item>

          <div class="oauth-guide cookie-guide">
            <div class="oauth-guide-title">Cookie 获取教程（2 分钟）</div>
            <ol class="oauth-guide-steps">
              <li>在浏览器登录 TwitCasting，并确认这个账号能正常观看目标直播。</li>
              <li>使用浏览器扩展导出 Netscape 格式的 <code>cookies.txt</code>。</li>
              <li>点击下面的“选择文件”，选中刚导出的 <code>cookies.txt</code>。</li>
              <li>保存设置；只对需要登录的主播，在右侧单主播策略里设为“强制 Cookie”。</li>
            </ol>
            <div class="oauth-guide-tip">
              <code>cookies.txt</code> 等同于登录凭证，请不要分享；如果浏览器退出登录或观看权限变化，需要重新导出。
            </div>
          </div>

          <el-form-item label="Cookie 文件路径">
            <div class="path-selector">
              <el-input v-model="form.cookies.filePath" placeholder="默认: cookies.txt">
                <template #prefix>
                  <el-icon><Document /></el-icon>
                </template>
              </el-input>
              <el-button type="primary" @click="selectCookieFile">选择文件</el-button>
            </div>
            <div class="form-tip">相对路径会按程序运行目录解析；打包版建议直接选择绝对路径，避免换目录后读错文件。</div>
          </el-form-item>
        </div>

        <div class="form-actions">
          <el-button type="primary" size="large" @click="saveSettings" :loading="saving">保存设置</el-button>
        </div>
      </el-form>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, onMounted, ref } from 'vue'
import { Document, Folder } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { BrowserOpenURL } from '../../wailsjs/runtime'
import {
  GetConfig,
  SaveSettings,
  SelectDirectory,
  SelectCookieFile,
  GetOAuthAuthorizeURL,
  ExchangeOAuthCode,
  VerifyOAuthToken,
  CheckRecordingToolsWithPaths,
  RunHealthCheckWithSettings,
  TestTelegramNotification,
} from '../../wailsjs/go/main/App'

const saving = ref(false)
const toolChecking = ref(false)
const healthChecking = ref(false)
const telegramTesting = ref(false)
const toolStatus = reactive({
  ffmpeg_ok: false,
  ffprobe_ok: false,
  message: '',
})
const healthReport = reactive({
  ok: false,
  items: [],
})

const form = reactive({
  outputDirectory: '.',
  authMode: 'auto',
  oauth: {
    clientId: '',
    clientSecret: '',
    redirectUri: '',
    accessToken: '',
    code: '',
  },
  cookies: {
    enabled: false,
    filePath: 'cookies.txt',
  },
  proxy: {
    enabled: false,
    url: '',
  },
  recording: {
    qualityMode: 'stable',
    containerMode: 'mkv',
    saveInfoText: true,
    saveCommentsText: true,
    saveCommentsTextFile: false,
    commentTextTemplate: '[{offset}] {display_name}: {message}',
    minDurationSeconds: 10,
    minFileSizeMB: 0,
    startupStaggerSeconds: 2,
    ffmpegPath: '',
    ffprobePath: '',
    workerEnabled: false,
    workerPath: '',
    workerCheckIntervalSeconds: 30,
    workerMaxRestarts: 8,
  },
  notifications: {
    telegram: {
      enabled: false,
      botToken: '',
      chatId: '',
      notifyOnStart: true,
      notifyOnFinish: true,
      notifyOnError: true,
    },
  },
})

const hasOAuthToken = computed(() => !!form.oauth.accessToken.trim())
const telegramReady = computed(() => (
  form.notifications.telegram.enabled &&
  !!form.notifications.telegram.botToken.trim() &&
  !!form.notifications.telegram.chatId.trim()
))
const authSummary = computed(() => {
  if (hasOAuthToken.value && form.cookies.enabled) return 'OAuth + Cookie'
  if (hasOAuthToken.value) return 'OAuth'
  if (form.cookies.enabled) return 'Cookie'
  return '未配置'
})

const statusText = (status) => {
  switch (status) {
    case 'ok': return '正常'
    case 'warn': return '注意'
    case 'error': return '错误'
    default: return status || '-'
  }
}

const loadSettings = async () => {
  try {
    const cfg = await GetConfig()
    if (!cfg) return

    form.outputDirectory = cfg.output_directory || '.'
    form.authMode = cfg.auth_mode || 'auto'

    const oauth = cfg.oauth || {}
    form.oauth.clientId = oauth.client_id || ''
    form.oauth.clientSecret = oauth.client_secret || ''
    form.oauth.redirectUri = oauth.redirect_uri || ''
    form.oauth.accessToken = oauth.access_token || ''

    const cookies = cfg.cookies || {}
    form.cookies.enabled = !!cookies.enabled
    form.cookies.filePath = cookies.file_path || 'cookies.txt'

    const proxy = cfg.proxy || {}
    form.proxy.enabled = !!proxy.enabled
    form.proxy.url = proxy.url || ''

    const recording = cfg.recording || {}
    form.recording.qualityMode = recording.quality_mode || 'stable'
    form.recording.containerMode = recording.container_mode || 'mkv'
    form.recording.saveInfoText = !!recording.save_info_text
    form.recording.saveCommentsText = recording.save_comments_text !== false
    form.recording.saveCommentsTextFile = !!recording.save_comments_text_file
    form.recording.commentTextTemplate = recording.comment_text_template || '[{offset}] {display_name}: {message}'
    form.recording.minDurationSeconds = Number(recording.min_duration_seconds ?? 10)
    form.recording.minFileSizeMB = Number(recording.min_file_size_mb ?? 0)
    form.recording.startupStaggerSeconds = Number(recording.startup_stagger_seconds ?? 2)
    form.recording.ffmpegPath = recording.ffmpeg_path || ''
    form.recording.ffprobePath = recording.ffprobe_path || ''
    form.recording.workerEnabled = !!recording.worker_enabled
    form.recording.workerPath = recording.worker_path || ''
    form.recording.workerCheckIntervalSeconds = Number(recording.worker_check_interval_seconds ?? 30)
    form.recording.workerMaxRestarts = Number(recording.worker_max_restarts ?? 8)

    const telegram = (cfg.notifications && cfg.notifications.telegram) || {}
    form.notifications.telegram.enabled = !!telegram.enabled
    form.notifications.telegram.botToken = telegram.bot_token || ''
    form.notifications.telegram.chatId = telegram.chat_id || ''
    form.notifications.telegram.notifyOnStart = telegram.notify_on_start !== false
    form.notifications.telegram.notifyOnFinish = telegram.notify_on_finish !== false
    form.notifications.telegram.notifyOnError = telegram.notify_on_error !== false
  } catch (e) {
    console.error(e)
    ElMessage.error('加载设置失败')
  }
}

const selectDir = async () => {
  try {
    const path = await SelectDirectory()
    if (path) {
      form.outputDirectory = path
    }
  } catch (e) {
    console.error(e)
  }
}

const selectCookieFile = async () => {
  try {
    const path = await SelectCookieFile()
    if (path) {
      form.cookies.filePath = path
      form.cookies.enabled = true
    }
  } catch (e) {
    console.error(e)
    ElMessage.error(`选择 Cookie 文件失败: ${e}`)
  }
}

const buildSettingsPayload = () => ({
  output_directory: form.outputDirectory,
  auth_mode: form.authMode,
  oauth: {
    client_id: form.oauth.clientId,
    client_secret: form.oauth.clientSecret,
    redirect_uri: form.oauth.redirectUri,
    access_token: form.oauth.accessToken,
  },
  cookies: {
    enabled: form.cookies.enabled,
    file_path: form.cookies.filePath,
  },
  proxy: {
    enabled: form.proxy.enabled,
    url: form.proxy.url,
  },
  recording: {
    quality_mode: form.recording.qualityMode,
    container_mode: form.recording.containerMode,
    save_info_text: form.recording.saveInfoText,
    save_comments_text: form.recording.saveCommentsText,
    save_comments_text_file: form.recording.saveCommentsTextFile,
    comment_text_template: form.recording.commentTextTemplate,
    min_duration_seconds: Number(form.recording.minDurationSeconds || 0),
    min_file_size_mb: Number(form.recording.minFileSizeMB || 0),
    startup_stagger_seconds: Number(form.recording.startupStaggerSeconds || 0),
    ffmpeg_path: form.recording.ffmpegPath,
    ffprobe_path: form.recording.ffprobePath,
    worker_enabled: form.recording.workerEnabled,
    worker_path: form.recording.workerPath,
    worker_check_interval_seconds: Number(form.recording.workerCheckIntervalSeconds || 0),
    worker_max_restarts: Number(form.recording.workerMaxRestarts || 0),
  },
  notifications: {
    telegram: {
      enabled: form.notifications.telegram.enabled,
      bot_token: form.notifications.telegram.botToken,
      chat_id: form.notifications.telegram.chatId,
      notify_on_start: form.notifications.telegram.notifyOnStart,
      notify_on_finish: form.notifications.telegram.notifyOnFinish,
      notify_on_error: form.notifications.telegram.notifyOnError,
    },
  },
})

const saveSettings = async (silent = false) => {
 saving.value = true
 try {
    const payload = buildSettingsPayload()
    const err = await SaveSettings(payload)
    if (err) {
      if (!silent) ElMessage.error(`保存失败: ${err}`)
      return false
    } else {
      if (!silent) ElMessage.success('设置已保存')
      return true
    }
  } catch (e) {
    if (!silent) ElMessage.error(`保存失败: ${e}`)
    return false
  } finally {
    saving.value = false
  }
}

const testTelegram = async () => {
  telegramTesting.value = true
  try {
    const err = await TestTelegramNotification({
      enabled: true,
      bot_token: form.notifications.telegram.botToken,
      chat_id: form.notifications.telegram.chatId,
      notify_on_start: true,
      notify_on_finish: true,
      notify_on_error: true,
    })
    if (err) {
      ElMessage.error(`测试推送失败: ${err}`)
      return
    }
    ElMessage.success('测试推送已发送')
  } catch (e) {
    ElMessage.error(`测试推送失败: ${e}`)
  } finally {
    telegramTesting.value = false
  }
}

const checkTools = async () => {
  toolChecking.value = true
  try {
    const status = await CheckRecordingToolsWithPaths(
      form.recording.ffmpegPath,
      form.recording.ffprobePath,
    )
    toolStatus.ffmpeg_ok = !!status.ffmpeg_ok
    toolStatus.ffprobe_ok = !!status.ffprobe_ok
    toolStatus.message = status.message || ''
    if (status.ffmpeg_ok) {
      ElMessage.success(status.message || 'FFmpeg 可用')
    } else {
      ElMessage.error(status.message || 'FFmpeg 不可用')
    }
  } catch (e) {
    toolStatus.message = `检测失败: ${e}`
    ElMessage.error(toolStatus.message)
  } finally {
    toolChecking.value = false
  }
}

const openFFmpegDownload = () => {
  BrowserOpenURL('https://ffmpeg.org/download.html')
}

const runHealthCheck = async () => {
  healthChecking.value = true
  try {
    const report = await RunHealthCheckWithSettings(buildSettingsPayload())
    healthReport.ok = !!report.ok
    healthReport.items = report.items || []
    const lines = (report.items || []).map(item => `${item.status.toUpperCase()} ${item.name}: ${item.message}`)
    const message = lines.join('\n')
    if (report.ok) {
      ElMessage.success(message || '健康检查通过')
    } else {
      ElMessage.error(message || '健康检查发现错误')
    }
  } catch (e) {
    ElMessage.error(`健康检查失败: ${e}`)
  } finally {
    healthChecking.value = false
  }
}

const openOAuthPage = async () => {
  try {
    const saved = await saveSettings(true)
    if (!saved) {
      ElMessage.error('保存 OAuth 设置失败，暂不能打开授权页')
      return
    }
    const url = await GetOAuthAuthorizeURL()
    if (!url) {
      ElMessage.error('请先填写 Client ID（和可选 Redirect URI）并保存')
      return
    }
    BrowserOpenURL(url)
  } catch (e) {
    ElMessage.error(`打开授权页失败: ${e}`)
  }
}

const exchangeCode = async () => {
  try {
    if (!form.oauth.code) {
      ElMessage.error('请先填入 authorization code')
      return
    }
    const saved = await saveSettings(true)
    if (!saved) {
      ElMessage.error('保存 OAuth 设置失败，暂不能换取 Token')
      return
    }
    const err = await ExchangeOAuthCode(form.oauth.code)
    if (err) {
      ElMessage.error(`换取 Token 失败: ${err}`)
      return
    }
    ElMessage.success('Token 更新成功')
    await loadSettings()
  } catch (e) {
    ElMessage.error(`换取 Token 失败: ${e}`)
  }
}

const verifyToken = async () => {
  try {
    if (!form.oauth.accessToken.trim()) {
      ElMessage.error('请先填写 Access Token')
      return
    }
    const saved = await saveSettings(true)
    if (!saved) {
      ElMessage.error('保存 OAuth 设置失败，暂不能校验 Token')
      return
    }
    const err = await VerifyOAuthToken()
    if (err) {
      ElMessage.error(`Token 校验失败: ${err}`)
      return
    }
    ElMessage.success('Token 校验通过')
  } catch (e) {
    ElMessage.error(`Token 校验失败: ${e}`)
  }
}

onMounted(() => {
  loadSettings()
})
</script>

<style scoped>
.settings-container {
  height: 100%;
  min-height: 0;
  padding: 4px 4px 20px;
  box-sizing: border-box;
  overflow-y: auto;
}

.settings-header {
  margin-bottom: 14px;
}

.settings-header h2 {
  font-size: 20px;
  font-weight: 700;
  color: #1e293b;
  margin: 0 0 5px 0;
}

.settings-header p {
  color: #64748b;
  font-size: 13px;
  margin: 0;
}

.settings-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 14px;
}

.settings-summary div {
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  padding: 10px 12px;
}

.settings-summary span {
  display: block;
  color: #64748b;
  font-size: 12px;
}

.settings-summary strong {
  display: block;
  margin-top: 3px;
  overflow: hidden;
  color: #111827;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.settings-content {
  background: white;
  border-radius: 8px;
  padding: 18px;
  border: 1px solid #e5e7eb;
}

.settings-section {
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid #eef2f7;
}

.settings-section:last-child {
  border-bottom: none;
}

.settings-section h3 {
  font-size: 16px;
  font-weight: 600;
  color: #334155;
  margin: 0 0 12px 0;
}

.path-selector {
  display: flex;
  gap: 12px;
}

.oauth-guide {
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 12px 14px;
  background: #f8fafc;
  margin-bottom: 14px;
}

.oauth-guide-title {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
  margin-bottom: 8px;
}

.oauth-guide-steps {
  margin: 0 0 8px 18px;
  padding: 0;
  color: #334155;
  line-height: 1.7;
  font-size: 13px;
}

.oauth-guide-tip {
  font-size: 12px;
  color: #475569;
}

.oauth-actions {
  display: flex;
  gap: 12px;
  margin-bottom: 12px;
}

.form-tip {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 6px;
}

.tool-status {
  margin-left: 12px;
  font-size: 12px;
  color: #64748b;
}

.tool-status.ok {
  color: #059669;
}

.tool-status.bad {
  color: #dc2626;
}

.notify-options {
  display: flex;
  gap: 18px;
  flex-wrap: wrap;
}

.notify-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
  color: #64748b;
  font-size: 12px;
}

.health-panel {
  margin: -6px 0 18px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #f8fafc;
  padding: 12px;
}

.health-panel-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
  color: #111827;
  font-size: 13px;
}

.health-panel-head .ok {
  color: #059669;
  font-weight: 700;
}

.health-panel-head .bad {
  color: #dc2626;
  font-weight: 700;
}

.health-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 8px;
}

.health-item {
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-left-width: 4px;
  border-radius: 8px;
  background: #ffffff;
  padding: 8px 10px;
}

.health-item.ok {
  border-left-color: #10b981;
}

.health-item.warn {
  border-left-color: #f59e0b;
}

.health-item.error {
  border-left-color: #ef4444;
}

.health-item span {
  color: #64748b;
  font-size: 12px;
}

.health-item strong {
  display: block;
  margin-top: 3px;
  color: #111827;
  font-size: 13px;
}

.health-item p {
  margin: 5px 0 0;
  color: #475569;
  font-size: 12px;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.auth-overview {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 14px;
}

.auth-overview div {
  border: 1px solid #e5e7eb;
  border-left: 4px solid #cbd5e1;
  border-radius: 8px;
  background: #ffffff;
  padding: 8px 10px;
}

.auth-overview div.ready {
  border-left-color: #10b981;
}

.auth-overview span {
  color: #64748b;
  font-size: 12px;
}

.auth-overview strong {
  display: block;
  margin-top: 3px;
  color: #111827;
  font-size: 13px;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 24px;
}

@media (max-width: 780px) {
  .settings-summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>



