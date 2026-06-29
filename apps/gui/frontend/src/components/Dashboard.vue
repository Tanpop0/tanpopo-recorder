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
          <section class="task-panel">
            <div class="task-toolbar">
              <el-input v-model="searchQuery" placeholder="搜索主播 ID" clearable />
              <el-button icon="Refresh" :loading="refreshLoading" @click="refreshList">刷新</el-button>
            </div>

            <div v-if="bulkState.active || bulkState.done > 0" class="queue-banner" :class="{ done: !bulkState.active }">
              <div>
                <strong>{{ bulkState.title }}</strong>
                <span>{{ bulkState.message }}</span>
              </div>
              <div class="queue-progress">
                {{ bulkState.done }} / {{ bulkState.total }}
              </div>
            </div>

            <div class="task-list">
              <button
                v-for="streamer in filterStreamers"
                :key="streamer.screen_id"
                class="streamer-row"
                :class="{ active: selectedID === streamer.screen_id, recording: streamer.current_status === 'recording', 'has-error': streamer.lastError || streamer.consecutiveFailures > 0 }"
                type="button"
                @click="selectStreamer(streamer)"
              >
                <el-avatar :size="42" :src="streamer.avatar || 'https://twitcasting.tv/img/user_default.png'" />
                <div class="streamer-main">
                  <div class="streamer-title-line">
                    <span class="streamer-id">{{ streamer.screen_id }}</span>
                    <span class="status-pill" :class="streamer.current_status">
                      <span class="status-dot"></span>{{ getStatusText(streamer.current_status) }}
                    </span>
                  </div>
                  <div class="streamer-subtitle">{{ streamer.nickname || '未获取昵称' }}</div>
                  <div class="streamer-message">{{ renderRealtimeMessage(streamer) }}</div>
                  <div class="streamer-meta">
                    <span>更新 {{ formatShortTime(streamer.lastCheckAt) }}</span>
                    <span v-if="streamer.consecutiveFailures > 0" class="danger-meta">异常 {{ streamer.consecutiveFailures }} 次</span>
                    <span v-else-if="streamer.lastSuccessAt">成功 {{ formatShortTime(streamer.lastSuccessAt) }}</span>
                  </div>
                </div>
                <div class="row-actions" @click.stop>
                  <el-button
                    size="small"
                    :type="streamer.current_status === 'idle' ? 'success' : 'warning'"
                    :loading="isStreamerBusy(streamer.screen_id)"
                    :disabled="bulkState.active"
                    @click="setMonitoringByRow(streamer)"
                  >
                    {{ streamer.current_status === 'idle' ? '开始' : '暂停' }}
                  </el-button>
                  <el-button size="small" @click="openDetails(streamer)">详情</el-button>
                </div>
              </button>
              <div v-if="filterStreamers.length === 0" class="empty-state">暂无匹配主播</div>
            </div>
          </section>

          <aside class="detail-panel">
            <template v-if="selectedStreamer">
              <div class="detail-head">
                <el-avatar :size="52" :src="selectedStreamer.avatar || 'https://twitcasting.tv/img/user_default.png'" />
                <div>
                  <h2>{{ selectedStreamer.nickname || selectedStreamer.screen_id }}</h2>
                  <p>{{ selectedStreamer.screen_id }}</p>
                </div>
                <span class="status-pill large" :class="selectedStreamer.current_status">
                  <span class="status-dot"></span>{{ getStatusText(selectedStreamer.current_status) }}
                </span>
              </div>

              <div class="detail-stats">
                <div v-for="item in detailStatItems(selectedStreamer)" :key="item.key">
                  <span>{{ item.label }}</span>
                  <strong>{{ item.value }}</strong>
                </div>
              </div>

              <div class="detail-section policy-section">
                <div class="section-label">单主播策略</div>
                <div class="policy-control">
                  <span>画质</span>
                  <el-select
                    :model-value="selectedStreamer.quality_mode || ''"
                    size="small"
                    @change="value => updateStreamerQualityMode(selectedStreamer, value)"
                  >
                    <el-option v-for="item in qualityModeOptions" :key="item.value" :label="item.label" :value="item.value" />
                  </el-select>
                </div>
                <div class="policy-control">
                  <span>鉴权</span>
                  <el-select
                    :model-value="selectedStreamer.auth_mode || ''"
                    size="small"
                    @change="value => updateStreamerAuthMode(selectedStreamer, value)"
                  >
                    <el-option v-for="item in authModeOptions" :key="item.value" :label="item.label" :value="item.value" />
                  </el-select>
                </div>
                <div class="policy-control">
                  <span>TG 推送</span>
                  <el-switch
                    :model-value="!!selectedStreamer.telegram_enabled"
                    size="small"
                    inline-prompt
                    active-text="开"
                    inactive-text="关"
                    @change="value => updateStreamerTelegramEnabled(selectedStreamer, value)"
                  />
                </div>
              </div>

              <div class="detail-section">
                <div class="section-label">当前状态</div>
                <p>{{ renderRealtimeMessage(selectedStreamer) }}</p>
              </div>

              <div class="detail-section">
                <div class="section-label">诊断</div>
                <p>{{ renderDiagnosticsMessage(selectedStreamer) }}</p>
              </div>

              <div class="detail-actions">
                <el-button size="small" @click="openLink(selectedStreamer.screen_id)">打开主页</el-button>
                <el-button size="small" type="primary" plain @click="showStreamerHistory(selectedStreamer)">历史录播</el-button>
                <el-button size="small" type="danger" plain @click="removeStreamer(selectedStreamer.screen_id)">删除主播</el-button>
                <el-button
                  size="small"
                  :type="selectedStreamer.current_status === 'idle' ? 'success' : 'warning'"
                  :loading="isStreamerBusy(selectedStreamer.screen_id)"
                  :disabled="bulkState.active"
                  @click="setMonitoringByRow(selectedStreamer)"
                >
                  {{ selectedStreamer.current_status === 'idle' ? '开始监听' : '暂停监听' }}
                </el-button>
              </div>

              <div class="recent-log">
                <div class="section-label">最近日志</div>
                <div v-if="!selectedStreamer.recentLogs || selectedStreamer.recentLogs.length === 0" class="log-empty">暂无日志</div>
                <div v-for="(line, index) in selectedStreamer.recentLogs.slice(0, 6)" :key="index" class="log-line">{{ line }}</div>
              </div>
            </template>
            <div v-else class="detail-empty">
              <h2>选择一个主播</h2>
              <p>在左侧任务列表中选择主播后，这里会显示录制状态、诊断和最近日志。</p>
            </div>
          </aside>
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
import StreamerDetailDialog from './StreamerDetailDialog.vue'
import { GetStreamers, AddStreamer, RemoveStreamer, SetMonitoring, SetAllMonitoring, GetRecordingHistory, UpdateStreamerOptions } from '../../wailsjs/go/main/App'
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

const simplifyAppLogLine = (raw) => {
  const text = String(raw || '').trim()
  if (!text) return ''
  const lower = text.toLowerCase()

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
      if (status === 'recording') return 3
      if (status === 'monitoring' || status === 'restricted') return 2
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
    parts.push(`最近错误: ${lastError}`)
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
    if (elapsed) {
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

.task-panel,
.detail-panel {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  min-height: 0;
}

.task-panel {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.task-toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  padding: 12px;
  border-bottom: 1px solid #eef2f7;
}

.queue-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid #dbeafe;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 12px;
}

.queue-banner.done {
  border-bottom-color: #dcfce7;
  background: #f0fdf4;
  color: #047857;
}

.queue-banner strong,
.queue-banner span {
  display: block;
}

.queue-banner span {
  margin-top: 2px;
  color: inherit;
  opacity: 0.82;
}

.queue-progress {
  flex: 0 0 auto;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.8);
  padding: 4px 8px;
  font-weight: 800;
}

.task-list {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 8px;
}

.streamer-row {
  width: 100%;
  min-height: 78px;
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr) auto;
  gap: 12px;
  align-items: center;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  padding: 10px;
  text-align: left;
  cursor: pointer;
  color: inherit;
}

.streamer-row:hover,
.streamer-row.active {
  background: #f8fafc;
  border-color: #dbe3ed;
}

.streamer-row.recording {
  border-color: #fecaca;
  background: #fff7f7;
}

.streamer-row.has-error {
  border-left-color: #f59e0b;
  border-left-width: 4px;
}

.streamer-main {
  min-width: 0;
}

.streamer-title-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.streamer-id {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #111827;
  font-size: 14px;
  font-weight: 800;
}

.streamer-subtitle {
  margin-top: 3px;
  color: #64748b;
  font-size: 12px;
}

.streamer-message {
  margin-top: 8px;
  color: #475569;
  font-size: 12px;
  line-height: 1.45;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.streamer-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 5px;
  color: #94a3b8;
  font-size: 11px;
}

.streamer-meta .danger-meta {
  color: #b45309;
  font-weight: 700;
}

.row-actions {
  display: flex;
  gap: 6px;
  align-items: center;
  flex: 0 0 auto;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex: 0 0 auto;
  height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  color: #475569;
  background: #f1f5f9;
  white-space: nowrap;
}

.status-pill.large {
  margin-left: auto;
  max-width: 92px;
  overflow: hidden;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: #94a3b8;
}

.status-pill.recording {
  color: #b91c1c;
  background: #fef2f2;
}

.status-pill.recording .status-dot {
  background: #ef4444;
}

.status-pill.monitoring {
  color: #047857;
  background: #ecfdf5;
}

.status-pill.monitoring .status-dot {
  background: #10b981;
}

.status-pill.error {
  color: #b45309;
  background: #fffbeb;
}

.status-pill.error .status-dot {
  background: #f59e0b;
}

.status-pill.restricted {
  color: #7c3aed;
  background: #f5f3ff;
}

.status-pill.restricted .status-dot {
  background: #8b5cf6;
}

.detail-panel {
  width: 360px;
  box-sizing: border-box;
  padding: 14px;
  display: flex;
  flex-direction: column;
  overflow: auto;
  min-width: 0;
}

.detail-head {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.detail-head > div {
  flex: 1 1 auto;
  min-width: 0;
}

.detail-head h2 {
  margin: 0;
  color: #111827;
  font-size: 16px;
  line-height: 1.25;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-head p {
  margin: 3px 0 0;
  color: #64748b;
  font-size: 12px;
}

.detail-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-top: 14px;
}

.detail-stats div {
  border: 1px solid #edf2f7;
  border-radius: 8px;
  padding: 8px;
  min-width: 0;
}

.detail-stats span,
.section-label {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.detail-stats strong {
  display: block;
  margin-top: 5px;
  color: #111827;
  font-size: 13px;
  overflow-wrap: anywhere;
}

.detail-section {
  margin-top: 14px;
}

.policy-section {
  border-top: 1px solid #eef2f7;
  padding-top: 12px;
}

.policy-control {
  display: grid;
  grid-template-columns: 46px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.policy-control > span {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.detail-section p {
  margin: 7px 0 0;
  color: #334155;
  font-size: 13px;
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.detail-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 14px;
}

.detail-actions :deep(.el-button) {
  width: 100%;
  margin-left: 0;
}

.recent-log {
  margin-top: 16px;
  border-top: 1px solid #eef2f7;
  padding-top: 12px;
}

.log-line,
.log-empty {
  margin-top: 6px;
  color: #475569;
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.empty-state,
.detail-empty {
  color: #64748b;
  padding: 24px;
  text-align: center;
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

  .detail-panel {
    width: auto;
    max-height: 360px;
  }

  .status-pill.large {
    max-width: none;
  }
}

</style>

