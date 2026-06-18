package synthesis

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ebook-audiobook/internal/model"
	"ebook-audiobook/internal/storage"
	"ebook-audiobook/internal/tts"
)

// Manager orchestrates synthesis jobs — scheduling, synthesis, and assembly
type Manager struct {
	store     *storage.Store
	engineReg *tts.EngineRegistry
	splitter  *tts.TextSplitter
	assembler AudioAssembler
	retry     *RetryPolicy

	maxWorkers   int
	outputDir    string
	defaultFmt   string
	sampleRate   int
	chapterGap   float64
	apiThrottle  chan struct{} // global rate limiter: 1 API call at a time

	mu        sync.Mutex
	jobs      map[string]context.CancelFunc
	listeners map[string][]chan model.JobProgress
}

func NewManager(store *storage.Store, engineReg *tts.EngineRegistry, outputDir string, maxWorkers int, defaultFmt string, sampleRate int, chapterGap float64) *Manager {
	throttle := make(chan struct{}, 1)
	throttle <- struct{}{} // initialize with one token
	return &Manager{
		store:       store,
		engineReg:   engineReg,
		splitter:    tts.NewTextSplitter(1500),
		assembler:   NewFFmpegAssembler(),
		retry:       DefaultRetryPolicy(),
		maxWorkers:  maxWorkers,
		outputDir:   outputDir,
		defaultFmt:  defaultFmt,
		sampleRate:  sampleRate,
		chapterGap:  chapterGap,
		apiThrottle: throttle,
		jobs:        make(map[string]context.CancelFunc),
		listeners:   make(map[string][]chan model.JobProgress),
	}
}

// StartJob launches an async synthesis job
func (m *Manager) StartJob(bookID string, cfg model.JobConfig) (*model.SynthesisJob, error) {
	book, err := m.store.GetBook(bookID)
	if err != nil {
		return nil, fmt.Errorf("get book: %w", err)
	}

	job := &model.SynthesisJob{
		BookID: bookID, Status: model.JobPending, Config: cfg,
		OutputFormat: cfg.OutputFormat,
	}
	if job.OutputFormat == "" {
		job.OutputFormat = m.defaultFmt
	}
	job.Progress = model.JobProgress{TotalChapters: len(book.Chapters)}

	if err := m.store.SaveJob(job); err != nil {
		return nil, fmt.Errorf("save job: %w", err)
	}

	jobDir := filepath.Join(m.outputDir, job.ID)
	os.MkdirAll(jobDir, 0755)

	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.jobs[job.ID] = cancel
	m.listeners[job.ID] = make([]chan model.JobProgress, 0)
	m.mu.Unlock()

	go m.run(ctx, job, book, jobDir)
	return job, nil
}

// CancelJob cancels a running job
func (m *Manager) CancelJob(jobID string) error {
	m.mu.Lock()
	if cancel, ok := m.jobs[jobID]; ok {
		cancel()
		delete(m.jobs, jobID)
	}
	m.mu.Unlock()
	return m.store.UpdateJobStatus(jobID, model.JobCancelled, "cancelled")
}

// SubscribeProgress returns a channel for job progress updates (SSE)
func (m *Manager) SubscribeProgress(jobID string) chan model.JobProgress {
	ch := make(chan model.JobProgress, 50)
	m.mu.Lock()
	m.listeners[jobID] = append(m.listeners[jobID], ch)
	m.mu.Unlock()
	return ch
}

// UnsubscribeProgress removes a listener
func (m *Manager) UnsubscribeProgress(jobID string, ch chan model.JobProgress) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.listeners[jobID]
	for i, c := range list {
		if c == ch {
			m.listeners[jobID] = append(list[:i], list[i+1:]...)
			break
		}
	}
}

func (m *Manager) notify(jobID string, progress model.JobProgress) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.listeners[jobID] {
		select {
		case ch <- progress:
		default:
		}
	}
}

// ---- orchestration ----

type segTask struct {
	seg    *model.AudioSegment
	vp     *model.VoiceProfile
	opts   model.TTSOptions
}

// run is the main synthesis pipeline: split → synthesize → assemble
func (m *Manager) run(ctx context.Context, job *model.SynthesisJob, book *model.Book, jobDir string) {
	job.Status = model.JobRunning
	m.store.SaveJob(job)

	// Phase 1: split book into segment tasks
	tasks, totalSegments := m.splitBook(job, book)
	job.Progress.TotalSegments = totalSegments
	m.store.UpdateJobProgress(job.ID, job.Progress)

	// Phase 2: synthesize with worker pool
	sem := make(chan struct{}, m.maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0
	allFailed := true

	for _, task := range tasks {
		select {
		case <-ctx.Done():
			m.store.UpdateJobStatus(job.ID, model.JobCancelled, "cancelled")
			return
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(t segTask) {
			defer wg.Done()
			defer func() { <-sem }()

			t.seg.Status = "synthesizing"
			m.store.SaveSegment(t.seg)

			audioData, format, err := m.synthesizeOne(ctx, t)

			if err != nil {
				t.seg.Status = "failed"
				t.seg.Error = err.Error()
				m.store.SaveSegment(t.seg)
				log.Printf("❌ ch%d.seg%d failed: %v", t.seg.ChapterIdx, t.seg.SegmentIdx, err)
				return
			}

			// Save audio
			fname := fmt.Sprintf("ch%03d_seg%03d.%s", t.seg.ChapterIdx, t.seg.SegmentIdx, format)
			audioPath := filepath.Join(jobDir, fname)
			if err := os.WriteFile(audioPath, audioData, 0644); err != nil {
				t.seg.Status = "failed"
				t.seg.Error = err.Error()
				m.store.SaveSegment(t.seg)
				return
			}

			t.seg.Status = "done"
			t.seg.AudioPath = audioPath
			t.seg.Format = format
			m.store.SaveSegment(t.seg)

			mu.Lock()
			completed++
			allFailed = false
			job.Progress.CompletedSegments = completed
			if completed >= totalSegments {
				job.Progress.CompletedChapters = job.Progress.TotalChapters
			} else {
				job.Progress.CompletedChapters = completed * job.Progress.TotalChapters / totalSegments
			}
			mu.Unlock()

			m.store.UpdateJobProgress(job.ID, job.Progress)
			m.notify(job.ID, job.Progress)
		}(task)
	}

	wg.Wait()

	if allFailed {
		m.store.UpdateJobStatus(job.ID, model.JobFailed, "all segments failed")
		return
	}

	// Phase 3: assemble
	job.Status = model.JobMerging
	m.store.SaveJob(job)

	segments, _ := m.store.GetJobSegments(job.ID)
	var segFiles []SegmentFile
	for _, s := range segments {
		if s.Status == "done" {
			segFiles = append(segFiles, SegmentFile{Path: s.AudioPath, ChapterIdx: s.ChapterIdx})
		}
	}

	outputPath, err := m.assembler.Assemble(jobDir, segFiles, m.chapterGap, m.sampleRate, job.OutputFormat)
	if err != nil {
		m.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("assemble: %v", err))
		return
	}

	job.OutputPath = outputPath
	job.Progress.CompletedSegments = totalSegments
	job.Progress.CompletedChapters = job.Progress.TotalChapters
	job.Status = model.JobCompleted
	m.store.UpdateJobProgress(job.ID, job.Progress)
	m.store.SaveJob(job)
	m.notify(job.ID, job.Progress)
	log.Printf("✅ Job %s complete → %s", job.ID[:8], outputPath)
}

// splitBook divides all chapters into segment tasks
func (m *Manager) splitBook(job *model.SynthesisJob, book *model.Book) ([]segTask, int) {
	var tasks []segTask
	total := 0

	for _, ch := range book.Chapters {
		voiceID := job.Config.DefaultVoiceID
		if v, ok := job.Config.ChapterVoiceMap[ch.Index]; ok {
			voiceID = v
		}
		vp, err := m.store.GetVoiceProfile(voiceID)
		if err != nil {
			// Not in DB — try to match a preset voice by ID or VoiceID
			vp = lookupPresetVoice(voiceID)
		}

		texts := m.splitter.Split(ch.Content)
		for si, text := range texts {
			seg := &model.AudioSegment{
				JobID: job.ID, ChapterIdx: ch.Index, SegmentIdx: si,
				Text: text, Status: "pending",
			}
			m.store.SaveSegment(seg)
			tasks = append(tasks, segTask{
				seg:  seg,
				vp:   vp,
				opts: job.Config.TTSOptions,
			})
		}
		total += len(texts)
	}
	return tasks, total
}

// synthesizeOne synthesizes a single segment with retry and global rate limiting
func (m *Manager) synthesizeOne(ctx context.Context, t segTask) ([]byte, string, error) {
	var audio []byte
	var format string

	err := m.retry.Do(ctx, func() error {
		// Acquire API throttle — serialize all TTS API calls globally
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.apiThrottle:
			// got the token, proceed
		}

		var synthErr error
		audio, format, synthErr = m.engineReg.SynthesizeWithEngine(ctx, t.seg.Text, t.vp, t.opts)

		// Release token after a cool-down delay to respect API rate limits
		go func() {
			time.Sleep(3 * time.Second)
			m.apiThrottle <- struct{}{}
		}()

		return synthErr
	})
	if err != nil {
		return nil, "", err
	}
	return audio, format, nil

}

// lookupPresetVoice finds a preset voice by ID or VoiceID, translating our
// internal IDs (e.g. "moli") to MiMo API voice IDs (e.g. "茉莉")
func lookupPresetVoice(id string) *model.VoiceProfile {
	for _, p := range tts.MiMoPresetVoices {
		if p.ID == id || p.VoiceID == id {
			c := p
			return &c
		}
	}
	return &model.VoiceProfile{ID: id, Engine: "mimo", Source: "preset", VoiceID: id}
}
