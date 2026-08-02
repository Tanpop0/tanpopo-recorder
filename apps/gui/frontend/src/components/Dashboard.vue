<template>
  <div class="dashboard-container">
    <div class="app-header">
      <div class="title-block">
        <div class="eyebrow">TwitCasting Recorder</div>
        <h1>录制监控</h1>
      </div>
      <div class="status-strip">
        <div class="metric recording">
          <span>录制中</span>
          <strong>{{ statusCounts.recording }}</strong>
        </div>
        <div class="metric monitoring">
          <span>监听中</span>
          <strong>{{ statusCounts.monitoring }}</strong>
        </div>
        <div class="metric error">
          <span>异常</span>
          <strong>{{ statusCounts.error }}</strong>
        </div>
        <div class="metric idle">
          <span>空闲</span>
          <strong>{{ statusCounts.idle }}</strong>
        </div>
      </div>
      <div class="top-actions">
        <el-button type="success" icon="VideoPlay" :loading="bulkState.active && bulkState.type === 'start'" :disabled="hasBlockingOperation" @click="startAll">全部开始</el-button>
        <el-button type="warning" icon="VideoPause" :loading="bulkState.active && bulkState.type === 'stop'" :disabled="hasBlockingOperation" @click="stopAll">全部暂停</el-button>
        <el-button type="primary" icon="Plus" :disabled="hasBlockingOperation" @click="showAddDialog = true">添加主播</el-button>
      </div>
    </div>

    <el-tabs v-model="currentTab" class="dashboard-tabs">
      <el-tab-pane label="监控" name="dashboard" class="dashboard-pane">
        <div class="monitor-layout">
          <StreamerTaskPanel
            v-model:search-query="searchQuery"
            :streamers="filterStreamers"
            :selected-id="selectedID"
            :refresh-loading="refreshLoading"
            :bulk-state="bulkState"
            :status-text="getStatusText"
            :realtime-message="renderRealtimeMessage"
            :short-time="formatShortTime"
            :is-busy="isStreamerBusy"
            @refresh="refreshList"
            @select="selectStreamer"
            @toggle-monitoring="setMonitoringByRow"
            @open-details="openDetails"
            @avatar-error="refreshAvatar"
          />

          <StreamerSidePanel
            :streamer="selectedStreamer"
            :status-text="getStatusText"
            :stat-items="detailStatItems"
            :realtime-message="renderRealtimeMessage"
            :diagnostics-message="renderDiagnosticsMessage"
            :display-log-line="localizeAppLogLine"
            :auth-mode-options="authModeOptions"
            :quality-mode-options="qualityModeOptions"
            :is-busy="isStreamerBusy"
            :bulk-active="bulkState.active"
            @open-link="openLink"
            @show-history="showStreamerHistory"
            @remove="removeStreamer"
            @toggle-monitoring="setMonitoringByRow"
            @update-auth="updateStreamerAuthMode"
            @update-quality="updateStreamerQualityMode"
            @update-telegram="updateStreamerTelegramEnabled"
            @avatar-error="refreshAvatar"
          />
        </div>
      </el-tab-pane>

      <el-tab-pane :label="`历史 (${historyCount})`" name="history" class="content-pane">
        <RecordingHistory ref="historyRef" @changed="handleHistoryChanged" />
      </el-tab-pane>

      <el-tab-pane label="设置" name="settings" class="content-pane">
        <Settings />
      </el-tab-pane>
    </el-tabs>

    <OperationLog v-if="currentTab === 'dashboard'" :logs="operationLogs" @clear="clearOperationLogs" />

    <AddStreamerDialog v-model="showAddDialog" @add="addStreamer" />
    <StreamerDetailDialog v-model="showDetailDialog" :streamer="detailRow" :status-text="getStatusText" />
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed, watch, nextTick } from 'vue'
import RecordingHistory from './RecordingHistory.vue'
import Settings from './Settings.vue'
import OperationLog from './OperationLog.vue'
import AddStreamerDialog from './AddStreamerDialog.vue'
import StreamerTaskPanel from './StreamerTaskPanel.vue'
import StreamerSidePanel from './StreamerSidePanel.vue'
import StreamerDetailDialog from './StreamerDetailDialog.vue'
import { GetStreamers, AddStreamer, RemoveStreamer, SetMonitoring, SetAllMonitoring, GetRecordingHistory, UpdateStreamerOptions, RefreshStreamerMetadata } from '../../wailsjs/go/main/App'
import { BrowserOpenURL, EventsOn } from '../../wailsjs/runtime'
import { ElMessage } from 'element-plus'

const streamers = ref([])
const showAddDialog = ref(false)
const searchQuery = ref('')
const currentTab = ref('dashboard')
const historyCount = ref(0)
const historyRef = ref(null)
const operationLogs = ref([])
const nowTick = ref(Date.now())
const showDetailDialog = ref(false)
const detailRow = ref(null)
const selectedID = ref('')
const refreshLoading = ref(false)
const busyStreamers = ref(new Set())
const bulkState = ref({
  active: false,
  type: '',
  title: '',
  message: '',
  total: 0,
  done: 0,
})
let ticker = null
let bulkHideTimer = null
const avatarRefreshPending = new Set()

const authModeOptions = [
  { value: '', label: '继承全局' },
  { value: 'cookie', label: '强制 Cookie' },
  { value: 'no_cookie', label: '禁用 Cookie' },
]

const qualityModeOptions = [
  { value: '', label: '跟随全局' },
  { value: 'original', label: '原始/最高' },
  { value: 'high', label: '高画质' },
  { value: 'stable', label: '中档稳定' },
  { value: 'low', label: '低画质' },
  { value: 'auto', label: '自动尝试' },
  { value: 'audio', label: '仅保存音频' },
]

const appendOpLog = (msg) => {
  const text = String(msg || '').trim()
  if (!text) return
  const t = new Date().toLocaleTimeString()
  operationLogs.value.unshift(`[${t}] ${text}`)
  if (operationLogs.value.length > 500) {
    operationLogs.value = operationLogs.value.slice(0, 500)
  }
}

const localizeAppLogLine = (raw) => {
  const text = String(raw || '').trim()
  const lower = text.toLowerCase()
  const prefixMatch = text.match(/^((?:\[[^\]]+\]\s*)+)/)
  const cleanPrefix = (prefixMatch?.[1].match(/\[[^\]]+\]/g) || [])
    .filter(group => !/^\[(?:stderr|stdout|in#|out#|tcp\s@|hls\s@|https\s@|tls\s@|null\s@|aist#|vist#)/i.test(group))
    .join(' ')
  const prefix = cleanPrefix ? `${cleanPrefix} ` : ''

  if (lower.includes('error opening input') && lower.includes('404 not found')) {
    return `${prefix}录制流不可用：直播可能已结束、流地址已失效，或当前账号无观看权限`
  }
  if (lower.includes('401 unauthorized') || lower.includes('authorization failed')) {
    return `${prefix}录制鉴权失败：请刷新 Cookie，或确认当前账号具有观看权限`
  }
  if (lower.includes('403 forbidden')) {
    return `${prefix}服务器拒绝访问录制流：当前账号、Cookie 或代理出口可能无权限`
  }
  if (lower.includes('check live status failed') && (lower.includes('context deadline exceeded') || lower.includes('client.timeout exceeded') || lower.includes('tls handshake timeout'))) {
    return `${prefix}直播状态检查超时：通常由代理或网络连接波动导致，程序将在下次检查时重试`
  }
  if (lower.includes('lacked sufficient buffer space') || lower.includes('queue was full')) {
    return `${prefix}本机或代理连接资源不足：连接队列已满，请检查代理负载`
  }
  if (lower.includes('proxyconnect') && (lower.includes('connection refused') || lower.includes('dial tcp'))) {
    return `${prefix}无法连接本地代理：请确认代理程序和端口正在运行`
  }
  if (lower.includes('forcibly closed by the remote host') || lower.includes('connection reset')) {
    return `${prefix}网络连接被中途重置：通常由代理节点或线路波动导致`
  }
  if (lower.includes('media progress stalled')) {
    return `${prefix}媒体进度长时间未更新：当前录制将停止并自动重新连接`
  }
  if (lower.includes('no usable cookies were found')) {
    return `${prefix}未找到可用 Cookie：请检查 Cookie 文件路径和内容`
  }
  if (lower.includes('login-required placeholder recording detected')) {
    return `${prefix}检测到登录提示占位录像：Cookie 已失效或当前账号没有观看权限`
  }
  if (lower.includes('error during demuxing') && lower.includes('error number -138')) {
    return `${prefix}录制连接中断：FFmpeg 正在等待恢复或重新连接直播流`
  }
  if (lower.includes('failed to start ffmpeg') || lower.includes('executable file not found')) {
    return `${prefix}无法启动 FFmpeg：请检查设置中的 FFmpeg 可执行文件路径`
  }
  return text
}

const simplifyAppLogLine = (raw) => {
  const text = String(raw || '').trim()
  if (!text) return ''
  const lower = text.toLowerCase()
  const localized = localizeAppLogLine(text)
  if (localized !== text) return localized

  // First drop ffmpeg high-frequency noise, even if it contains "error=".
  const noisyPatterns = [
    '[stdout] progress=',
    '[stdout] frame=',
    '[stdout] fps=',
    '[stdout] bitrate=',
    '[stdout] total_size=',
    '[stdout] out_time',
    '[stdout] speed=',
    '[stdout] drop_frames=',
    '[stdout] dup_frames=',
    '[stdout] stream_0_0_q=',
    '[stderr] [hls @',
    '[stderr] [https @',
    '[stderr] [tls @',
    '[stderr] [null @',
    '[stderr] [mov,mp4,m4a,3gp,3g2,mj2 @',
    'opening \'https://',
    'skip (\'#ext-x-version',
    'skipping ',
    'will reconnect at ',
    'last message repeated',
    'found duplicated moov atom'
  ]
  if (noisyPatterns.some(k => lower.includes(k))) {
    return ''
  }

  const noisyFfmpegMetaPrefixes = [
    '[stderr] stream mapping:',
    '[stderr] metadata:',
    '[stderr] duration: n/a',
    '[stderr] program 0',
    '[stderr] stream #',
    '[stderr] compatible_brands:',
    '[stderr] major_brand',
    '[stderr] minor_version',
    '[stderr] variant_bitrate',
    '[stderr] press [q] to stop'
  ]
  if (noisyFfmpegMetaPrefixes.some(k => lower.includes(k))) {
    return ''
  }

  // Keep all actionable/problem lines.
  const strongKeep = [
    'error',
    'failed',
    'unauthorized',
    'forbidden',
    'rate limited',
    'auto reconnect',
    'reconnecting',
    'pause requested',
    'monitoring started',
    'live started',
    'recording finished',
    'stopped by user',
    'check live status failed',
    'stream reconnecting',
    'recording failed',
    'recording process starting'
  ]
  if (strongKeep.some(k => lower.includes(k))) {
    return text
  }

  // Keep concise lifecycle lines.
  if (lower.includes('[q] command received. exiting.')) {
    return text
  }

  // By default keep short app-level lines, drop long ffmpeg detail lines.
  if (text.length > 120 && (lower.includes('[stderr]') || lower.includes('[stdout]'))) {
    return ''
  }
  return text
}

const clearOperationLogs = () => {
  operationLogs.value = []
}

const refreshAvatar = async (row) => {
  const screenID = row?.screen_id
  if (!screenID || avatarRefreshPending.has(screenID)) return
  avatarRefreshPending.add(screenID)
  try {
    const err = await RefreshStreamerMetadata(screenID)
    if (err) {
      appendOpLog(`${screenID} 头像刷新失败：${err}`)
      return
    }
    await refreshList()
  } catch (e) {
    appendOpLog(`${screenID} 头像刷新失败：${e?.message || e}`)
  } finally {
    setTimeout(() => avatarRefreshPending.delete(screenID), 30000)
  }
}

const findStreamer = (screenID) => streamers.value.find(s => s.screen_id === screenID)

const hasBlockingOperation = computed(() => bulkState.value.active)

const markStreamerBusy = (screenID, busy) => {
  const next = new Set(busyStreamers.value)
  if (busy) {
    next.add(screenID)
  } else {
    next.delete(screenID)
  }
  busyStreamers.value = next
}

const isStreamerBusy = (screenID) => busyStreamers.value.has(screenID)

const extractMediaElapsed = (message) => {
  const text = String(message || '')
  const m = text.match(/(\d{2}:\d{2}:\d{2}(?:\.\d+)?)/)
  if (!m) return ''
  return m[1].split('.')[0]
}

const formatRunElapsed = (sinceTs) => {
  if (!sinceTs) return '--:--:--'
  const sec = Math.max(0, Math.floor((nowTick.value - sinceTs) / 1000))
  const h = Math.floor(sec / 3600).toString().padStart(2, '0')
  const m = Math.floor((sec % 3600) / 60).toString().padStart(2, '0')
  const s = Math.floor(sec % 60).toString().padStart(2, '0')
  return `${h}:${m}:${s}`
}

const shouldShowMediaElapsed = (row) => {
  return !!row?.mediaElapsed
}

const formatShortTime = (value) => {
  if (!value) return '-'
  return new Date(value).toLocaleTimeString()
}

const estimateNextCheckAt = (row, base = Date.now()) => {
  if (!row?.is_monitoring && row?.current_status === 'idle') return null
  const schedule = String(row?.schedule || '')
  const m = schedule.match(/^\*\/(\d+)\s+\*\s+\*\s+\*\s+\*$/)
  const minutes = m ? Math.max(1, Number(m[1] || 1)) : 1
  return base + minutes * 60 * 1000
}

const applyHistoryStats = (list) => {
  const stats = new Map()
  ;(list || []).forEach((record) => {
    const id = record.streamer_id
    if (!id) return
    const current = stats.get(id) || { lastSuccessAt: null, historyWarnings: 0, historyErrors: 0 }
    const status = String(record.status || 'completed').toLowerCase()
    const ts = record.end_time || record.start_time
    if (['completed'].includes(status) && (!current.lastSuccessAt || new Date(ts) > new Date(current.lastSuccessAt))) {
      current.lastSuccessAt = ts
    }
    if (['short', 'too_short', 'small', 'too_small'].includes(status)) {
      current.historyWarnings += 1
    }
    if (status.startsWith('failed_') || ['failed', 'error', 'interrupted'].includes(status)) {
      current.historyErrors += 1
    }
    stats.set(id, current)
  })
  streamers.value = streamers.value.map((row) => ({
    ...row,
    ...(stats.get(row.screen_id) || {}),
  }))
}

const refreshCounts = async () => {
  try {
    const list = await GetRecordingHistory()
    historyCount.value = list ? list.length : 0
    applyHistoryStats(list)
  } catch (e) {
    console.error(e)
  }
}

const handleHistoryChanged = (count) => {
  historyCount.value = Number(count || 0)
  refreshCounts()
}

watch(currentTab, (newVal) => {
  if (newVal === 'history' && historyRef.value) {
    historyRef.value.refreshHistory()
  }
  refreshCounts()
})

const filterStreamers = computed(() => {
  let list = [...streamers.value]
  if (searchQuery.value) {
    list = list.filter(s => s.screen_id.toLowerCase().includes(searchQuery.value.toLowerCase()))
  }
  return list.sort((a, b) => {
    const getScore = (status) => {
      if (status === 'recording') return 4
      if (status === 'restricted') return 3
      if (status === 'monitoring') return 2
      return 1
    }
    const scoreA = getScore(a.current_status)
    const scoreB = getScore(b.current_status)
    if (scoreA !== scoreB) return scoreB - scoreA
    return a.screen_id.localeCompare(b.screen_id)
  })
})

const selectedStreamer = computed(() => {
  if (!streamers.value.length) return null
  return streamers.value.find(s => s.screen_id === selectedID.value) || streamers.value[0]
})

const statusCounts = computed(() => {
  return streamers.value.reduce((acc, item) => {
    const status = item.current_status || 'idle'
    const countStatus = status === 'restricted' ? 'monitoring' : status
    acc[countStatus] = (acc[countStatus] || 0) + 1
    return acc
  }, { recording: 0, monitoring: 0, error: 0, idle: 0 })
})

const selectStreamer = (streamer) => {
  selectedID.value = streamer?.screen_id || ''
}

const getStatusText = (status) => {
  switch (status) {
    case 'recording': return '录制中'
    case 'monitoring': return '监听中'
    case 'restricted': return '受限直播'
    case 'error': return '异常'
    default: return '空闲'
  }
}

const getAuthModeText = (mode) => {
  switch (String(mode || '').toLowerCase()) {
    case 'cookie': return '强制 Cookie'
    case 'no_cookie': return '禁用 Cookie'
    default: return '继承全局'
  }
}

const getQualityModeText = (mode) => {
  switch (String(mode || '').toLowerCase()) {
    case 'stable':
    case 'medium': return '中档稳定'
    case 'auto': return '自动画质'
    case 'original': return '原始/最高'
    case 'high': return '高画质'
    case 'low': return '低画质'
    case 'audio': return '仅保存音频'
    default: return ''
  }
}

const getContainerModeText = (mode) => {
  switch (String(mode || '').toLowerCase()) {
    case 'mkv': return 'MKV'
    case 'ts': return 'TS'
    case 'mp4': return 'MP4'
    default: return ''
  }
}

const customRecordingPolicyText = (row) => {
  const parts = []
  const quality = getQualityModeText(row?.quality_mode)
  const container = getContainerModeText(row?.container_mode)
  if (quality) parts.push(quality)
  if (container) parts.push(container)
  if (row?.telegram_enabled) parts.push('TG 推送')
  return parts.join(' / ')
}

const detailStatItems = (row) => {
  if (!row) return []
  const items = []
  const status = row.current_status || 'idle'

  if (status === 'recording') {
    items.push({
      key: 'recording',
      label: '录制时长',
      value: row.recordingSince ? formatRunElapsed(row.recordingSince) : '--:--:--',
    })
    if (shouldShowMediaElapsed(row)) {
      items.push({ key: 'media', label: '媒体时长', value: row.mediaElapsed || '--:--:--' })
    }
    if (row.lastSuccessAt) {
      items.push({ key: 'last-success', label: '最后成功', value: formatShortTime(row.lastSuccessAt) })
    }
  } else if (status === 'monitoring') {
    items.push({ key: 'next-check', label: '下次检查', value: formatShortTime(row.nextCheckAt) })
    items.push({ key: 'last-check', label: '最后检查', value: formatShortTime(row.lastCheckAt) })
    if (row.lastSuccessAt) {
      items.push({ key: 'last-success', label: '最后成功', value: formatShortTime(row.lastSuccessAt) })
    }
  } else {
    items.push({ key: 'status', label: '状态', value: getStatusText(status) })
    if (row.lastSuccessAt) {
      items.push({ key: 'last-success', label: '最后成功', value: formatShortTime(row.lastSuccessAt) })
    }
    if (row.lastCheckAt) {
      items.push({ key: 'last-check', label: '最后检查', value: formatShortTime(row.lastCheckAt) })
    }
  }

  const recordingPolicy = customRecordingPolicyText(row)
  if (recordingPolicy) {
    items.push({ key: 'record-policy', label: '录制策略', value: recordingPolicy })
  }
  if (row.auth_mode) {
    items.push({ key: 'auth-policy', label: '鉴权策略', value: getAuthModeText(row.auth_mode) })
  }
  if ((row.consecutiveFailures || 0) > 0) {
    items.push({ key: 'failures', label: '连续异常', value: `${row.consecutiveFailures} 次` })
  }
  if (items.length < 2 && row.nextCheckAt) {
    items.push({ key: 'next-check', label: '下次检查', value: formatShortTime(row.nextCheckAt) })
  }
  return items.slice(0, 6)
}

const renderRealtimeMessage = (row) => {
  const msg = row.last_message || '等待更新...'
  if (row.current_status === 'recording') {
    const title = row.currentTitle || '未知标题'
    const elapsed = row.mediaElapsed || '--:--:--'
    const runElapsed = formatRunElapsed(row.recordingSince)
    const timeText = shouldShowMediaElapsed(row)
      ? `录制时长 ${runElapsed} | 媒体时长 ${elapsed}`
      : `录制时长 ${runElapsed}`
    return `标题: ${title} | ${timeText}`
  }
  return msg
}

const mediaElapsedSeconds = (value) => {
  const match = String(value || '').trim().match(/^(\d+):(\d{1,2}):(\d{1,2})$/)
  if (!match) return null
  return Number(match[1]) * 3600 + Number(match[2]) * 60 + Number(match[3])
}

const shouldAcceptMediaElapsed = (current, next) => {
  const currentSeconds = mediaElapsedSeconds(current)
  const nextSeconds = mediaElapsedSeconds(next)
  if (nextSeconds === null) return false
  return currentSeconds === null || nextSeconds >= currentSeconds
}

const basename = (path) => {
  const text = String(path || '')
  if (!text) return ''
  return text.split(/[/\\]/).pop() || text
}

const renderDiagnosticsMessage = (row) => {
  const parts = []
  if (row.consecutiveFailures > 0) {
    parts.push(`连续异常 ${row.consecutiveFailures} 次`)
  }
  if (row.historyWarnings > 0 || row.historyErrors > 0) {
    parts.push(`历史异常 ${row.historyErrors || 0} / 可疑 ${row.historyWarnings || 0}`)
  }
  const recordingPolicy = customRecordingPolicyText(row)
  if (recordingPolicy) {
    parts.push(`录制策略 ${recordingPolicy}`)
  }
  if (row.auth_mode) {
    parts.push(`鉴权 ${getAuthModeText(row.auth_mode)}`)
  }
  const lastError = row.last_error || row.lastError
  const lastFilePath = row.last_file_path || row.lastFilePath
  if (lastError) {
    parts.push(`最近错误: ${localizeAppLogLine(lastError)}`)
  }
  if (lastFilePath) {
    parts.push(`产物: ${basename(lastFilePath)}`)
  }
  return parts.length ? parts.join(' | ') : '暂无异常'
}

const refreshList = async () => {
  refreshLoading.value = true
  try {
    const list = await GetStreamers()
    const now = Date.now()

    const stateMap = new Map()
    streamers.value.forEach(s => {
      stateMap.set(s.screen_id, {
        current_status: s.current_status,
        last_message: s.last_message,
        mediaElapsed: s.mediaElapsed,
        currentTitle: s.currentTitle,
        recordingSince: s.recordingSince,
        lastError: s.lastError,
        lastFilePath: s.lastFilePath,
        recentLogs: s.recentLogs,
        lastCheckAt: s.lastCheckAt,
        nextCheckAt: s.nextCheckAt,
        consecutiveFailures: s.consecutiveFailures,
        lastSuccessAt: s.lastSuccessAt,
        historyWarnings: s.historyWarnings,
        historyErrors: s.historyErrors,
      })
    })

    streamers.value = (list || []).map(s => {
      let status = s.current_status || (s.is_monitoring ? 'monitoring' : 'idle')
      let message = s.last_message || (s.is_monitoring ? '监听中，等待开播...' : '已暂停监听')
      let mediaElapsed = null
      let currentTitle = null
      let recordingSince = null
      let lastError = s.last_error || ''
      let lastFilePath = s.last_file_path || ''
      let recentLogs = s.recent_logs || []
      let lastCheckAt = now
      let nextCheckAt = estimateNextCheckAt({ ...s, current_status: status }, now)
      let consecutiveFailures = 0
      let lastSuccessAt = null
      let historyWarnings = 0
      let historyErrors = 0

      if (stateMap.has(s.screen_id)) {
        const old = stateMap.get(s.screen_id)
        if (!s.current_status && s.is_monitoring && ['recording', 'error', 'restricted'].includes(old.current_status)) {
          status = old.current_status
          message = old.last_message
        }
        if (status === 'recording' && old.current_status === 'recording') {
          mediaElapsed = old.mediaElapsed
          currentTitle = old.currentTitle
          recordingSince = old.recordingSince
        }
        lastError = old.lastError || lastError
        lastFilePath = old.lastFilePath || lastFilePath
        recentLogs = old.recentLogs || recentLogs
        lastCheckAt = old.lastCheckAt || lastCheckAt
        nextCheckAt = old.nextCheckAt || nextCheckAt
        consecutiveFailures = old.consecutiveFailures || 0
        lastSuccessAt = old.lastSuccessAt || null
        historyWarnings = old.historyWarnings || 0
        historyErrors = old.historyErrors || 0
      }

      return {
        ...s,
        current_status: status,
        last_message: message,
        mediaElapsed,
        currentTitle,
        recordingSince,
        lastError,
        lastFilePath,
        recentLogs,
        lastCheckAt,
        nextCheckAt,
        consecutiveFailures,
        lastSuccessAt,
        historyWarnings,
        historyErrors,
      }
    })
    if (!selectedID.value && streamers.value.length > 0) {
      selectedID.value = streamers.value[0].screen_id
    }
    if (selectedID.value && !streamers.value.some(s => s.screen_id === selectedID.value)) {
      selectedID.value = streamers.value[0]?.screen_id || ''
    }
  } catch (e) {
    appendOpLog(`刷新列表失败: ${e?.message || e}`)
    console.error(e)
  } finally {
    refreshLoading.value = false
  }
}

const setMonitoringByRow = async (row) => {
  if (!row || !row.screen_id) return
  if (isStreamerBusy(row.screen_id) || bulkState.value.active) return
  const shouldMonitor = row.current_status === 'idle'
  const action = shouldMonitor ? '开始监听' : '暂停监听'
  appendOpLog(`请求: ${row.screen_id} ${action}`)
  markStreamerBusy(row.screen_id, true)
  try {
    await SetMonitoring(row.screen_id, shouldMonitor)
    row.current_status = shouldMonitor ? 'monitoring' : 'idle'
    row.last_message = shouldMonitor ? '监听中，等待开播...' : '已暂停监听'
    row.lastCheckAt = Date.now()
    row.nextCheckAt = shouldMonitor ? estimateNextCheckAt(row, row.lastCheckAt) : null
    await refreshList()
    appendOpLog(`${row.screen_id} 操作成功: ${shouldMonitor ? '监听中' : '已暂停'}`)
  } catch (e) {
    appendOpLog(`${row.screen_id} 操作失败: ${e?.message || e}`)
    console.error(e)
  } finally {
    markStreamerBusy(row.screen_id, false)
  }
}

const finishBulkStateSoon = (message) => {
  bulkState.value = { ...bulkState.value, active: false, message }
  if (bulkHideTimer) clearTimeout(bulkHideTimer)
  bulkHideTimer = setTimeout(() => {
    bulkState.value = { active: false, type: '', title: '', message: '', total: 0, done: 0 }
  }, 5000)
}

const runMonitoringQueue = async (shouldMonitor) => {
  if (bulkState.value.active) return
  const targets = streamers.value.filter((row) => {
    if (shouldMonitor) return row.current_status === 'idle'
    return row.current_status !== 'idle'
  })
  if (targets.length === 0) {
    appendOpLog(shouldMonitor ? '没有需要开始监听的主播' : '没有需要暂停的主播')
    return
  }
  const type = shouldMonitor ? 'start' : 'stop'
  bulkState.value = {
    active: true,
    type,
    title: shouldMonitor ? '开始监听队列' : '暂停监听队列',
    message: shouldMonitor ? '按顺序提交监听任务，避免瞬间并发冲击。' : '按顺序暂停任务。',
    total: targets.length,
    done: 0,
  }
  appendOpLog(`请求: ${shouldMonitor ? '全部开始' : '全部暂停'} (${targets.length} 个任务)`)

  try {
    bulkState.value = {
      ...bulkState.value,
      message: shouldMonitor ? '已交给后端按设置的错峰间隔启动。' : '正在暂停所有任务。',
    }
    await SetAllMonitoring(shouldMonitor)
    for (const row of targets) {
      markStreamerBusy(row.screen_id, true)
      if (!shouldMonitor) {
        row.current_status = 'idle'
        row.last_message = '已暂停监听'
        row.lastCheckAt = Date.now()
        row.nextCheckAt = null
      }
      bulkState.value = { ...bulkState.value, done: bulkState.value.done + 1 }
      markStreamerBusy(row.screen_id, false)
    }
    await refreshList()
    const doneText = shouldMonitor ? '全部开始任务已提交，后端将按错峰间隔执行' : '全部暂停队列已完成'
    appendOpLog(doneText)
    finishBulkStateSoon(doneText)
  } catch (e) {
    targets.forEach(row => markStreamerBusy(row.screen_id, false))
    const failText = `${shouldMonitor ? '全部开始' : '全部暂停'}失败: ${e?.message || e}`
    appendOpLog(failText)
    finishBulkStateSoon(failText)
    console.error(e)
  }
}

const startAll = async () => {
  await runMonitoringQueue(true)
}

const stopAll = async () => {
  await runMonitoringQueue(false)
}

const addStreamer = async (form) => {
  if (!form.id) return
  appendOpLog(`请求: 添加主播 ${form.id}`)
  try {
    const err = await AddStreamer(form.id, form.schedule, form.qualityMode, form.containerMode, form.authMode)
    if (err) {
      appendOpLog(`添加失败: ${err}`)
      return
    }
    showAddDialog.value = false
    await refreshList()
    appendOpLog('添加主播成功')
  } catch (e) {
    appendOpLog(`添加失败: ${e?.message || e}`)
    console.error(e)
  }
}

const updateStreamerAuthMode = async (row, authMode) => {
  if (!row?.screen_id) return
  const previous = row.auth_mode || ''
  row.auth_mode = authMode || ''
  try {
    const err = await UpdateStreamerOptions(row.screen_id, row.quality_mode || '', row.container_mode || '', row.auth_mode || '', !!row.telegram_enabled)
    if (err) {
      row.auth_mode = previous
      ElMessage.error(`更新策略失败: ${err}`)
      return
    }
    appendOpLog(`${row.screen_id} 鉴权策略: ${getAuthModeText(row.auth_mode)}`)
  } catch (e) {
    row.auth_mode = previous
    ElMessage.error(`更新策略失败: ${e?.message || e}`)
  }
}

const updateStreamerQualityMode = async (row, qualityMode) => {
  if (!row?.screen_id) return
  const previous = row.quality_mode || ''
  row.quality_mode = qualityMode || ''
  try {
    const err = await UpdateStreamerOptions(row.screen_id, row.quality_mode || '', row.container_mode || '', row.auth_mode || '', !!row.telegram_enabled)
    if (err) {
      row.quality_mode = previous
      ElMessage.error(`更新画质策略失败: ${err}`)
      return
    }
    appendOpLog(`${row.screen_id} 画质策略: ${getQualityModeText(row.quality_mode) || '跟随全局'}`)
  } catch (e) {
    row.quality_mode = previous
    ElMessage.error(`更新画质策略失败: ${e?.message || e}`)
  }
}

const updateStreamerTelegramEnabled = async (row, enabled) => {
  if (!row?.screen_id) return
  const previous = !!row.telegram_enabled
  row.telegram_enabled = !!enabled
  try {
    const err = await UpdateStreamerOptions(row.screen_id, row.quality_mode || '', row.container_mode || '', row.auth_mode || '', !!row.telegram_enabled)
    if (err) {
      row.telegram_enabled = previous
      ElMessage.error(`更新 TG 推送失败: ${err}`)
      return
    }
    appendOpLog(`${row.screen_id} TG 推送: ${row.telegram_enabled ? '开启' : '关闭'}`)
  } catch (e) {
    row.telegram_enabled = previous
    ElMessage.error(`更新 TG 推送失败: ${e?.message || e}`)
  }
}

const removeStreamer = async (id) => {
  appendOpLog(`请求: 删除主播 ${id}`)
  try {
    const err = await RemoveStreamer(id)
    if (err) {
      appendOpLog(`删除失败: ${err}`)
      return
    }
    await refreshList()
    appendOpLog(`已删除主播 ${id}`)
  } catch (e) {
    appendOpLog(`删除失败: ${e?.message || e}`)
    console.error(e)
  }
}

const openLink = (id) => {
  BrowserOpenURL(`https://twitcasting.tv/${id}`)
}

const openDetails = (row) => {
  detailRow.value = row
  selectStreamer(row)
  showDetailDialog.value = true
}

const showStreamerHistory = async (row) => {
  if (!row?.screen_id) return
  currentTab.value = 'history'
  await nextTick()
  if (historyRef.value?.filterByStreamer) {
    historyRef.value.filterByStreamer(row.screen_id)
  } else if (historyRef.value?.refreshHistory) {
    historyRef.value.refreshHistory()
  }
}

onMounted(() => {
  refreshList()
  refreshCounts()
  appendOpLog('面板已加载')

  EventsOn('streamer-status', (data) => {
    const row = findStreamer(data.screen_id)
    if (!row) return

    const previousStatus = row.current_status
    row.current_status = data.status
    row.lastCheckAt = Date.now()
    row.nextCheckAt = data.status === 'idle' ? null : estimateNextCheckAt(row, row.lastCheckAt)
    if (previousStatus !== 'recording' && data.status === 'recording') {
      row.recordingSince = Date.now()
    }
    let displayMessage = data.message || ''

    const elapsed = extractMediaElapsed(displayMessage)
    if (elapsed && shouldAcceptMediaElapsed(row.mediaElapsed, elapsed)) {
      row.mediaElapsed = elapsed
    }

    if (displayMessage.startsWith('Live! ')) {
      row.currentTitle = displayMessage.substring(6).trim()
      displayMessage = '已开播'
    }

    const writtenMatch = displayMessage.match(/Written\s+([\d.]+\s*[KMG]i?B)/)
    if (writtenMatch) {
      displayMessage = `已写入 ${writtenMatch[1]}`
    } else if (displayMessage.includes('Opening stream')) {
      displayMessage = '正在打开直播流...'
    } else if (displayMessage.includes('Writing output to')) {
      const pathParts = displayMessage.split('Writing output to')
      if (pathParts.length > 1) {
        const fileName = pathParts[1].trim().split(/[/\\]/).pop()
        displayMessage = fileName ? `开始录制: ${fileName}` : '开始写入文件...'
      } else {
        displayMessage = '开始写入文件...'
      }
    } else if (displayMessage.includes('Found matching plugin')) {
      displayMessage = '发现直播源，准备录制...'
    } else if (displayMessage.includes('Available streams')) {
      displayMessage = '解析直播流画质中...'
    } else if (displayMessage.includes('Stream ended') || displayMessage.includes('Closing')) {
      displayMessage = '直播结束，录制完成'
    } else if (data.status === 'error') {
      displayMessage = `错误: ${displayMessage}`
      row.consecutiveFailures = (row.consecutiveFailures || 0) + 1
      row.lastError = displayMessage
    } else if (data.status === 'restricted') {
      row.consecutiveFailures = 0
    } else if (displayMessage.length > 120) {
      displayMessage = displayMessage.substring(0, 117) + '...'
    }

    row.last_message = displayMessage

    if (data.status !== 'recording') {
      row.mediaElapsed = null
      row.recordingSince = null
      if (data.status === 'monitoring' || data.status === 'idle') {
        row.currentTitle = null
      }
    }

    if (data.status === 'recording') {
      row.consecutiveFailures = 0
    }

    if (previousStatus !== data.status) {
      appendOpLog(`${data.screen_id} 状态变化: ${getStatusText(previousStatus)} -> ${getStatusText(data.status)}`)
    }

    if (previousStatus === 'recording' && data.status !== 'recording') {
      setTimeout(() => {
        refreshCounts()
        if (currentTab.value === 'history' && historyRef.value) {
          historyRef.value.refreshHistory()
        }
      }, 1200)
    }
  })

  EventsOn('history-updated', (payload) => {
    refreshCounts()
    if (currentTab.value === 'history' && historyRef.value) {
      historyRef.value.refreshHistory()
    }
    if (payload && payload.streamer_id) {
      const row = findStreamer(payload.streamer_id)
      if (row) {
        if ((payload.status || 'completed') === 'completed') {
          row.lastSuccessAt = Date.now()
          row.consecutiveFailures = 0
        } else {
          row.historyWarnings = (row.historyWarnings || 0) + 1
        }
      }
      appendOpLog(`历史已更新: ${payload.streamer_id} (${payload.status || 'completed'})`)
    } else if (payload && payload.action === 'clear') {
      historyCount.value = 0
      appendOpLog('历史记录已清空，文件已保留')
    } else {
      appendOpLog('历史已更新')
    }
  })

  EventsOn('streamer-diagnostics', (payload) => {
    if (!payload || !payload.screen_id) return
    const row = findStreamer(payload.screen_id)
    if (!row) return
    row.lastError = payload.last_error || ''
    row.lastFilePath = payload.last_file_path || ''
    row.recentLogs = payload.recent_logs || []
    if (row.lastError) {
      row.consecutiveFailures = Math.max(row.consecutiveFailures || 0, 1)
    }
  })

  EventsOn('streamer-metadata-updated', () => {
    refreshList()
  })

  EventsOn('app-log', (payload) => {
    const msg = payload && payload.message ? payload.message : String(payload || '')
    const line = simplifyAppLogLine(msg)
    if (line) appendOpLog(line)
  })

  ticker = setInterval(() => {
    nowTick.value = Date.now()
  }, 1000)
})

onUnmounted(() => {
  if (ticker) {
    clearInterval(ticker)
    ticker = null
  }
  if (bulkHideTimer) {
    clearTimeout(bulkHideTimer)
    bulkHideTimer = null
  }
})
</script>

<style scoped>
.dashboard-container {
  padding: 14px;
  height: 100vh;
  box-sizing: border-box;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: #f6f7f9;
}

.app-header {
  display: flex;
  align-items: center;
  gap: 14px;
  flex: 0 0 auto;
  min-width: 0;
}

.title-block {
  flex: 0 0 210px;
  min-width: 0;
}

.eyebrow {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0;
  margin-bottom: 2px;
}

.title-block h1 {
  margin: 0;
  color: #111827;
  font-size: 24px;
  line-height: 1.2;
}

.status-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(84px, 1fr));
  gap: 8px;
  flex: 1 1 auto;
  min-width: 0;
  max-width: 740px;
}

.metric {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  padding: 8px 10px;
}

.metric span {
  display: block;
  color: #6b7280;
  font-size: 12px;
  line-height: 1.2;
}

.metric strong {
  display: block;
  margin-top: 3px;
  color: #111827;
  font-size: 20px;
  line-height: 1;
}

.metric.recording strong {
  color: #dc2626;
}

.metric.monitoring strong {
  color: #059669;
}

.metric.error strong {
  color: #d97706;
}

.top-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
  flex: 0 0 auto;
}

.monitor-layout {
  display: grid;
  grid-template-columns: minmax(520px, 1fr) 360px;
  gap: 12px;
  min-height: 0;
  height: 100%;
}

.dashboard-tabs {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.dashboard-tabs :deep(.el-tabs__content) {
  flex: 1 1 auto;
  min-height: 0;
  height: auto;
  overflow: hidden;
}

.dashboard-tabs :deep(.el-tab-pane) {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

.dashboard-tabs :deep(.dashboard-pane),
.dashboard-tabs :deep(.content-pane) {
  flex: 1 1 auto;
  min-height: 0;
}

@media (max-width: 1100px) {
  .app-header {
    align-items: stretch;
    flex-direction: column;
  }

  .title-block {
    flex: 0 0 auto;
  }

  .status-strip {
    min-width: 0;
    width: 100%;
  }

  .top-actions {
    justify-content: flex-start;
  }

  .monitor-layout {
    grid-template-columns: 1fr;
  }

}

</style>

