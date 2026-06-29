<template>
  <el-dialog :model-value="modelValue" title="主播状态详情" width="720px" @update:model-value="$emit('update:modelValue', $event)">
    <div v-if="streamer" class="detail-panel">
      <div class="detail-grid">
        <div><span>主播 ID</span><strong>{{ streamer.screen_id }}</strong></div>
        <div><span>昵称</span><strong>{{ streamer.nickname || '-' }}</strong></div>
        <div><span>状态</span><strong>{{ statusText(streamer.current_status) }}</strong></div>
        <div><span>策略</span><strong>{{ qualityText(streamer.quality_mode) }} / {{ containerText(streamer.container_mode) }}</strong></div>
        <div><span>上次更新</span><strong>{{ formatTime(streamer.lastCheckAt) }}</strong></div>
        <div><span>下次预估</span><strong>{{ formatTime(streamer.nextCheckAt) }}</strong></div>
        <div><span>最后成功</span><strong>{{ formatTime(streamer.lastSuccessAt) }}</strong></div>
        <div><span>连续异常</span><strong>{{ streamer.consecutiveFailures || 0 }} 次</strong></div>
        <div><span>最后产物</span><strong>{{ basename(streamer.lastFilePath) || '-' }}</strong></div>
        <div><span>最后错误</span><strong>{{ streamer.lastError || '-' }}</strong></div>
      </div>
      <div class="detail-actions">
        <el-button size="small" @click="openLogFolder">打开日志目录</el-button>
        <el-button size="small" :disabled="!streamer.lastFilePath" @click="openRecordingFolder">打开产物目录</el-button>
      </div>
      <div class="detail-log">
        <div class="detail-log-title">最近日志</div>
        <div v-if="!streamer.recentLogs || streamer.recentLogs.length === 0" class="detail-empty">暂无日志</div>
        <div v-for="(line, index) in streamer.recentLogs" :key="index" class="detail-log-line">{{ line }}</div>
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { OpenFolder, OpenStreamerLogFolder } from '../../wailsjs/go/main/App'

const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
  streamer: {
    type: Object,
    default: null,
  },
  statusText: {
    type: Function,
    required: true,
  },
})

defineEmits(['update:modelValue'])

const basename = (path) => {
  const text = String(path || '')
  if (!text) return ''
  return text.split(/[/\\]/).pop() || text
}

const formatTime = (value) => {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

const qualityText = (mode) => {
  switch (String(mode || '').toLowerCase()) {
    case 'stable':
    case 'medium':
      return '中档稳定'
    case 'original':
      return '原始/最高'
    case 'high':
      return '高画质'
    case 'low':
      return '低画质'
    case 'auto':
      return '自动画质'
    case 'audio':
      return '仅保存音频'
    default:
      return '全局'
  }
}

const containerText = (mode) => {
  switch (String(mode || '').toLowerCase()) {
    case 'mkv':
      return 'MKV'
    case 'ts':
      return 'TS'
    case 'mp4':
      return 'MP4'
    default:
      return '全局'
  }
}

const openLogFolder = async () => {
  if (!props.streamer?.screen_id) return
  await OpenStreamerLogFolder(props.streamer.screen_id)
}

const openRecordingFolder = async () => {
  if (!props.streamer?.lastFilePath) return
  await OpenFolder(props.streamer.lastFilePath)
}
</script>

<style scoped>
.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 14px;
}

.detail-grid div {
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 8px 10px;
  min-width: 0;
}

.detail-grid span {
  display: block;
  color: #64748b;
  font-size: 12px;
  margin-bottom: 4px;
}

.detail-grid strong {
  display: block;
  color: #0f172a;
  font-size: 13px;
  overflow-wrap: anywhere;
}

.detail-actions {
  display: flex;
  gap: 8px;
}

.detail-log {
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 10px;
  max-height: 260px;
  overflow: auto;
  background: #f8fafc;
}

.detail-log-title {
  font-size: 13px;
  font-weight: 700;
  margin-bottom: 8px;
  color: #334155;
}

.detail-empty,
.detail-log-line {
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  color: #475569;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
</style>
