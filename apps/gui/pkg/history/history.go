package history

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// RecordingRecord represents a single recording history entry
type RecordingRecord struct {
	ID         string    `json:"id"`
	StreamerID string    `json:"streamer_id"`
	Title      string    `json:"title"` // Optional, if available
	Nickname   string    `json:"nickname,omitempty"`
	Avatar     string    `json:"avatar,omitempty"`
	FilePath   string    `json:"file_path"`
	FileSize   int64     `json:"file_size"` // in bytes
	Duration   string    `json:"duration"`  // formatted string or seconds
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Status     string    `json:"status"` // "completed", "failed", "interrupted"

	CommentTextPath    string `json:"comment_text_path,omitempty"`
	CommentJSONLPath   string `json:"comment_jsonl_path,omitempty"`
	CommentTextExists  bool   `json:"comment_text_exists,omitempty"`
	CommentJSONLExists bool   `json:"comment_jsonl_exists,omitempty"`
}

// HistoryManager handles reading and writing recording history
type HistoryManager struct {
	records []RecordingRecord
	mu      sync.RWMutex
	path    string
}

// NewHistoryManager creates a new manager
func NewHistoryManager(path string) *HistoryManager {
	hm := &HistoryManager{
		records: []RecordingRecord{},
		path:    path,
	}
	hm.Load()
	return hm
}

// AddRecord adds a new record to history and saves
func (hm *HistoryManager) AddRecord(record RecordingRecord) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Prepend to list (newest first)
	hm.records = append([]RecordingRecord{record}, hm.records...)
	return hm.save()
}

// GetRecords returns all records
func (hm *HistoryManager) GetRecords() []RecordingRecord {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	// Return a copy
	result := make([]RecordingRecord, len(hm.records))
	copy(result, hm.records)
	return result
}

// GetRecord returns a single record by ID.
func (hm *HistoryManager) GetRecord(id string) (RecordingRecord, bool) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	for _, record := range hm.records {
		if record.ID == id {
			return record, true
		}
	}
	return RecordingRecord{}, false
}

// ClearHistory removes all records
func (hm *HistoryManager) ClearHistory() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.records = []RecordingRecord{}
	return hm.save()
}

// DeleteRecord removes a record by ID
func (hm *HistoryManager) DeleteRecord(id string) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	for i, r := range hm.records {
		if r.ID == id {
			hm.records = append(hm.records[:i], hm.records[i+1:]...)
			return hm.save()
		}
	}
	return nil
}

// UpdateRecordStatus updates a record status by ID.
func (hm *HistoryManager) UpdateRecordStatus(id string, status string) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	for i := range hm.records {
		if hm.records[i].ID == id {
			hm.records[i].Status = status
			return hm.save()
		}
	}
	return nil
}

// Load reads history from disk
func (hm *HistoryManager) Load() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	data, err := os.ReadFile(hm.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &hm.records)
}

func (hm *HistoryManager) save() error {
	data, err := json.MarshalIndent(hm.records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(hm.path, data, 0644)
}
