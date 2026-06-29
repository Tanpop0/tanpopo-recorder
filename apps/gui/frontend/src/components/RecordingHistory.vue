<template>
  <div class="history-container">
    <div class="history-header">
      <div>
        <h2>录制历史</h2>
        <p>最近 {{ filteredHistory.length }} / {{ history.length }} 条，支持按主播、文件名和状态筛选。</p>
      </div>
      <div class="actions">
        <el-button type="danger" :icon="Delete" plain @click="confirmClear">清空记录</el-button>
        <el-button :icon="Refresh" circle :loading="loading" @click="refreshHistory" />
      </div>
    </div>

    <div class="history-filters">
      <el-input v-model="query" placeholder="搜索主播 ID 或文件名" clearable />
      <div class="status-filter">
        <button
          v-for="item in statusOptions"
          :key="item.value"
          type="button"
          class="status-filter-btn"
          :class="{ active: statusFilter === item.value }"
          @click="statusFilter = item.value"
        >
          {{ item.label }}
        </button>
      </div>
    </div>

    <div class="history-scroll">
      <el-empty v-if="filteredHistory.length === 0" description="暂无匹配录制记录" />

      <div v-else class="history-grid">
        <div v-for="record in filteredHistory" :key="record.id" class="history-card" :class="recordHealth(record).level">
          <button class="avatar-button" type="button" :title="`筛选 ${record.streamer_id || 'unknown'}`" @click="filterByStreamer(record.streamer_id)">
            <el-avatar :size="38" :src="record.avatar || 'https://twitcasting.tv/img/user_default.png'" />
          </button>

          <div class="card-content">
            <div class="file-name" :title="getFileName(record.file_path)">
              {{ getFileName(record.file_path) }}
            </div>
            <div class="meta-info">
              <span class="streamer-id">@{{ record.streamer_id || 'unknown' }}</span>
              <span v-if="record.nickname">{{ record.nickname }}</span>
              <span>{{ formatSize(record.file_size) }}</span>
              <span>{{ record.duration || '--:--:--' }}</span>
              <span v-if="formatAverageBitrate(record)">{{ formatAverageBitrate(record) }}</span>
              <span class="status-label" :class="recordHealth(record).level">
                {{ recordHealth(record).text }}
              </span>
            </div>
            <div v-if="mediaSummary(record)" class="media-info">
              {{ mediaSummary(record) }}
            </div>
            <div class="comment-info">
              <span v-if="record.comment_text_exists" class="comment-badge">评论 TXT</span>
              <span v-if="record.comment_jsonl_exists" class="comment-badge">JSONL</span>
              <span v-if="!record.comment_text_exists && !record.comment_jsonl_exists" class="comment-muted">未保存评论</span>
            </div>
            <div class="time-info">{{ formatDate(record.start_time) }}</div>
          </div>

          <div class="card-actions">
            <el-tooltip content="标记正常" placement="top">
              <el-button circle type="success" plain @click="markStatus(record, 'completed')">正</el-button>
            </el-tooltip>
            <el-tooltip content="标记异常" placement="top">
              <el-button circle type="warning" plain @click="markStatus(record, 'failed')">异</el-button>
            </el-tooltip>
            <el-tooltip content="打开评论 TXT" placement="top">
              <el-button circle :icon="Document" :disabled="!record.comment_text_exists" @click="openFile(record.comment_text_path)" />
            </el-tooltip>
            <el-tooltip content="打开评论 JSONL" placement="top">
              <el-button circle :icon="Tickets" :disabled="!record.comment_jsonl_exists" @click="openFile(record.comment_jsonl_path)" />
            </el-tooltip>
            <el-tooltip content="复制路径" placement="top">
              <el-button circle :icon="CopyDocument" @click="copyPath(record.file_path)" />
            </el-tooltip>
            <el-tooltip content="打开文件位置" placement="top">
              <el-button circle :icon="FolderOpened" @click="openFolder(record.file_path)" />
            </el-tooltip>
            <el-tooltip content="选择删除方式" placement="top">
              <el-button circle type="danger" :icon="Delete" plain @click="deleteRecord(record)" />
            </el-tooltip>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, onMounted } from 'vue'
import { CopyDocument, Delete, Document, FolderOpened, Refresh, Tickets } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  GetRecordingHistory,
  ClearRecordingHistory,
  DeleteHistoryRecord,
  DeleteHistoryRecordAndFile,
  OpenFile,
  OpenFolder,
  UpdateHistoryRecordStatus,
} from '../../wailsjs/go/main/App'
import { ClipboardSetText } from '../../wailsjs/runtime'

const history = ref([])
const loading = ref(false)
const query = ref('')
const statusFilter = ref('all')
const emit = defineEmits(['changed'])

const statusOptions = [
  { value: 'all', label: '全部' },
  { value: 'completed', label: '完成' },
  { value: 'warning', label: '偏短/中断' },
  { value: 'error', label: '异常' },
]

const refreshHistory = async () => {
  loading.value = true
  try {
    const list = await GetRecordingHistory()
    history.value = (list || []).sort((a, b) => new Date(b.start_time) - new Date(a.start_time))
    emit('changed', history.value.length)
  } catch (e) {
    console.error(e)
    ElMessage.error(`刷新历史失败: ${e}`)
  } finally {
    loading.value = false
  }
}

const getFileName = (path) => {
  if (!path) return 'Unknown File'
  return path.split(/[/\\]/).pop()
}

const formatSize = (bytes) => {
  const value = Number(bytes || 0)
  if (value <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(value) / Math.log(k)), sizes.length - 1)
  return `${parseFloat((value / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`
}

const formatDate = (dateStr) => {
  if (!dateStr) return '未知时间'
  return new Date(dateStr).toLocaleString()
}

const parseDurationSeconds = (value) => {
  const text = String(value || '').trim()
  const match = text.match(/^(\d+):(\d{1,2}):(\d{1,2})$/)
  if (!match) return 0
  return Number(match[1]) * 3600 + Number(match[2]) * 60 + Number(match[3])
}

const averageBitrate = (record) => {
  const probed = Number(record.media_bitrate || 0)
  if (probed > 0) return probed
  const seconds = parseDurationSeconds(record.duration)
  const bytes = Number(record.file_size || 0)
  if (seconds <= 0 || bytes <= 0) return 0
  return (bytes * 8) / seconds
}

const formatBitrate = (bitsPerSecond) => {
  const value = Number(bitsPerSecond || 0)
  if (value <= 0) return ''
  if (value >= 1000 * 1000) return `${(value / 1000 / 1000).toFixed(2)} Mbps`
  return `${Math.round(value / 1000)} kbps`
}

const formatAverageBitrate = (record) => {
  const value = formatBitrate(averageBitrate(record))
  return value ? `平均 ${value}` : ''
}

const mediaSummary = (record) => {
  const parts = []
  if (record.width && record.height) {
    const fps = Number(record.frame_rate || 0)
    parts.push(`${record.width}x${record.height}${fps > 0 ? ` ${fps.toFixed(fps >= 10 ? 0 : 2)}fps` : ''}`)
  }
  if (record.video_codec) {
    parts.push(`视频 ${record.video_codec}`)
  }
  if (record.audio_codec) {
    parts.push(`音频 ${record.audio_codec}`)
  }
  const videoBitrate = formatBitrate(record.video_bitrate)
  if (videoBitrate) {
    parts.push(`视频码率 ${videoBitrate}`)
  }
  const audioBitrate = formatBitrate(record.audio_bitrate)
  if (audioBitrate) {
    parts.push(`音频码率 ${audioBitrate}`)
  }
  return parts.join(' / ')
}

const recordHealth = (record) => {
  const status = String(record.status || 'completed').toLowerCase()
  if (status === 'manual_stopped') {
    return { level: 'warning', text: '手动停止' }
  }
  if (['short', 'too_short', 'small', 'too_small', 'failed_short', 'interrupted'].includes(status)) {
    return { level: 'warning', text: status === 'interrupted' ? '中断' : '偏短' }
  }
  if (status.startsWith('failed_') || ['failed', 'error'].includes(status)) {
    return { level: 'error', text: '异常' }
  }
  return { level: 'completed', text: '完成' }
}

const filteredHistory = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return history.value.filter((record) => {
    const health = recordHealth(record)
    if (statusFilter.value !== 'all') {
      if (statusFilter.value === 'warning' && health.level !== 'warning') return false
      if (statusFilter.value === 'error' && health.level !== 'error') return false
      if (statusFilter.value === 'completed' && health.level !== 'completed') return false
    }
    if (!keyword) return true
    const haystack = `${record.streamer_id || ''} ${getFileName(record.file_path)} ${record.file_path || ''}`.toLowerCase()
    return haystack.includes(keyword)
  })
})

const copyPath = async (path) => {
  if (!path) return
  try {
    await ClipboardSetText(path)
    ElMessage.success('路径已复制')
  } catch (e) {
    ElMessage.error(`复制失败: ${e}`)
  }
}

const openFolder = (path) => {
  OpenFolder(path)
}

const filterByStreamer = (streamerID) => {
  if (!streamerID) return
  query.value = streamerID
  statusFilter.value = 'all'
}

const openFile = async (path) => {
  if (!path) return
  const err = await OpenFile(path)
  if (err) {
    ElMessage.error(err)
  }
}

const removeRecordLocally = (id) => {
  history.value = history.value.filter(record => record.id !== id)
  emit('changed', history.value.length)
}

const deleteRecord = async (record) => {
  if (!record?.id) return
  let deleteFile = false
  try {
    await ElMessageBox.confirm(
      `请选择删除方式。\n\n只删记录：保留磁盘文件。\n删除文件和记录：同时删除 ${getFileName(record.file_path)} 及同名评论/信息文件。`,
      '删除历史',
      {
        confirmButtonText: '删除文件和记录',
        cancelButtonText: '只删记录',
        distinguishCancelAndClose: true,
        type: 'warning',
      }
    )
    deleteFile = true
  } catch (action) {
    if (action !== 'cancel') return
  }

  if (deleteFile) {
    const err = await DeleteHistoryRecordAndFile(record.id)
    if (err) {
      ElMessage.error(`删除文件失败: ${err}`)
      return
    }
    ElMessage.success('文件和记录已删除')
  } else {
    await DeleteHistoryRecord(record.id)
    ElMessage.success('已删除记录，文件已保留')
  }
  removeRecordLocally(record.id)
}

const markStatus = async (record, status) => {
  await UpdateHistoryRecordStatus(record.id, status)
  record.status = status
  ElMessage.success(status === 'completed' ? '已标记为正常' : '已标记为异常')
}

const confirmClear = () => {
  ElMessageBox.confirm(
    '确定清空所有录制记录吗？文件本身不会被删除。',
    '清空记录',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  ).then(async () => {
    await ClearRecordingHistory()
    history.value = []
    emit('changed', 0)
    ElMessage.success('录制记录已清空，文件已保留')
  })
}

onMounted(() => {
  refreshHistory()
})

defineExpose({ refreshHistory, filterByStreamer })
</script>

<style scoped>
.history-container {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 4px;
  box-sizing: border-box;
}

.history-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 10px;
}

.history-header h2 {
  margin: 0;
  color: #111827;
  font-size: 18px;
  line-height: 1.25;
}

.history-header p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
}

.actions {
  display: flex;
  gap: 8px;
  flex: 0 0 auto;
}

.history-filters {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) auto;
  gap: 10px;
  margin-bottom: 10px;
}

.status-filter {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  justify-content: flex-end;
}

.status-filter-btn {
  height: 32px;
  padding: 0 10px;
  border: 1px solid #dbe3ed;
  border-radius: 6px;
  background: #ffffff;
  color: #475569;
  cursor: pointer;
}

.status-filter-btn.active {
  border-color: #93c5fd;
  background: #eff6ff;
  color: #2563eb;
  font-weight: 700;
}

.history-scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 0 4px 16px 0;
}

.history-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(460px, 1fr));
  gap: 12px;
}

.history-card {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  padding: 10px;
  border: 1px solid #e5e7eb;
  border-left-width: 4px;
  border-radius: 8px;
  background: #ffffff;
}

.history-card.completed {
  border-left-color: #10b981;
}

.history-card.warning {
  border-left-color: #f59e0b;
}

.history-card.error {
  border-left-color: #ef4444;
}

.avatar-button {
  width: 38px;
  height: 38px;
  padding: 0;
  border: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
}

.avatar-button:hover {
  box-shadow: 0 0 0 3px #dbeafe;
}

.card-content {
  flex: 1 1 auto;
  min-width: 0;
}

.file-name {
  margin-bottom: 4px;
  overflow: hidden;
  color: #111827;
  font-size: 13px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meta-info,
.comment-info {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  color: #64748b;
  font-size: 12px;
}

.comment-info {
  margin-top: 5px;
}

.media-info {
  margin-top: 5px;
  color: #475569;
  font-size: 12px;
  line-height: 1.4;
}

.streamer-id {
  color: #2563eb;
  font-weight: 700;
}

.status-label,
.comment-badge {
  display: inline-flex;
  align-items: center;
  height: 22px;
  padding: 0 7px;
  border-radius: 999px;
  font-weight: 700;
}

.status-label {
  background: #ecfdf5;
  color: #047857;
}

.status-label.warning {
  background: #fffbeb;
  color: #b45309;
}

.status-label.error {
  background: #fef2f2;
  color: #b91c1c;
}

.comment-badge {
  background: #eef2ff;
  color: #4338ca;
}

.comment-muted {
  color: #94a3b8;
}

.time-info {
  margin-top: 4px;
  color: #94a3b8;
  font-size: 12px;
}

.card-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
  flex: 0 0 178px;
}

@media (max-width: 760px) {
  .history-header,
  .history-filters {
    align-items: stretch;
    grid-template-columns: 1fr;
  }

  .history-header {
    flex-direction: column;
  }

  .status-filter {
    justify-content: flex-start;
  }

  .history-grid {
    grid-template-columns: 1fr;
  }

  .history-card {
    align-items: flex-start;
  }

  .card-actions {
    flex-basis: 96px;
  }
}
</style>
