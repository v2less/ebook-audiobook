package model

import "time"

// SynthesisJob 一本书的合成任务
type SynthesisJob struct {
	ID          string         `json:"id"`
	BookID      string         `json:"book_id"`
	Status      JobStatus      `json:"status"`
	Progress    JobProgress    `json:"progress"`
	Config      JobConfig      `json:"config"`
	OutputPath  string         `json:"output_path"`
	OutputFormat string        `json:"output_format"` // mp3, wav, m4b
	Error       string         `json:"error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
}

// JobStatus 任务状态
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobMerging   JobStatus = "merging"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// JobProgress 任务进度
type JobProgress struct {
	TotalChapters     int `json:"total_chapters"`
	CompletedChapters int `json:"completed_chapters"`
	TotalSegments     int `json:"total_segments"`
	CompletedSegments int `json:"completed_segments"`
}

// JobConfig 任务配置
type JobConfig struct {
	ChapterVoiceMap map[int]string `json:"chapter_voice_map"` // chapter index → voice profile id
	DefaultVoiceID  string         `json:"default_voice_id"`
	TTSOptions      TTSOptions     `json:"tts_options"`
	OutputFormat    string         `json:"output_format"`
	ChapterGap      float64        `json:"chapter_gap"` // seconds of silence between chapters
	MergeChapters   bool           `json:"merge_chapters"`
}

// AudioSegment 一个合成片段
type AudioSegment struct {
	ID          string    `json:"id"`
	JobID       string    `json:"job_id"`
	ChapterIdx  int       `json:"chapter_idx"`
	SegmentIdx  int       `json:"segment_idx"`
	Text        string    `json:"text"`
	AudioPath   string    `json:"audio_path"`
	Format      string    `json:"format"`
	Duration    float64   `json:"duration"`
	Status      string    `json:"status"` // pending, synthesizing, done, failed
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
