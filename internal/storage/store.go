package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/google/uuid"

	"ebook-audiobook/internal/model"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS books (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		author TEXT DEFAULT '',
		format TEXT NOT NULL,
		file_name TEXT NOT NULL,
		meta_json TEXT DEFAULT '{}',
		chapters_json TEXT DEFAULT '[]',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS voice_profiles (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		source TEXT NOT NULL,
		engine TEXT NOT NULL,
		voice_id TEXT DEFAULT '',
		sample_path TEXT DEFAULT '',
		design_prompt TEXT DEFAULT '',
		description TEXT DEFAULT '',
		language TEXT DEFAULT 'zh-CN',
		gender TEXT DEFAULT 'neutral',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS synthesis_jobs (
		id TEXT PRIMARY KEY,
		book_id TEXT NOT NULL,
		status TEXT DEFAULT 'pending',
		progress_json TEXT DEFAULT '{}',
		config_json TEXT DEFAULT '{}',
		output_path TEXT DEFAULT '',
		output_format TEXT DEFAULT 'mp3',
		error TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		finished_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS audio_segments (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		chapter_idx INTEGER NOT NULL,
		segment_idx INTEGER NOT NULL,
		text TEXT NOT NULL,
		audio_path TEXT DEFAULT '',
		format TEXT DEFAULT 'wav',
		duration REAL DEFAULT 0,
		status TEXT DEFAULT 'pending',
		error TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// ---- Book CRUD ----

func (s *Store) SaveBook(b *model.Book) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	b.CreatedAt = time.Now()
	metaJSON, _ := json.Marshal(b.Meta)
	chaptersJSON, _ := json.Marshal(b.Chapters)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO books (id, title, author, format, file_name, meta_json, chapters_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.Title, b.Author, b.Format, b.FileName, string(metaJSON), string(chaptersJSON), b.CreatedAt,
	)
	return err
}

func (s *Store) GetBook(id string) (*model.Book, error) {
	row := s.db.QueryRow(`SELECT id, title, author, format, file_name, meta_json, chapters_json, created_at FROM books WHERE id = ?`, id)
	b := &model.Book{}
	var metaJSON, chaptersJSON string
	err := row.Scan(&b.ID, &b.Title, &b.Author, &b.Format, &b.FileName, &metaJSON, &chaptersJSON, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(metaJSON), &b.Meta)
	json.Unmarshal([]byte(chaptersJSON), &b.Chapters)
	return b, nil
}

func (s *Store) ListBooks() ([]*model.Book, error) {
	rows, err := s.db.Query(`SELECT id, title, author, format, file_name, meta_json, chapters_json, created_at FROM books ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books = make([]*model.Book, 0)
	for rows.Next() {
		b := &model.Book{}
		var metaJSON, chaptersJSON string
		if err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Format, &b.FileName, &metaJSON, &chaptersJSON, &b.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(metaJSON), &b.Meta)
		json.Unmarshal([]byte(chaptersJSON), &b.Chapters)
		books = append(books, b)
	}
	return books, nil
}

func (s *Store) DeleteBook(id string) error {
	_, err := s.db.Exec(`DELETE FROM books WHERE id = ?`, id)
	return err
}

// ---- VoiceProfile CRUD ----

func (s *Store) SaveVoiceProfile(vp *model.VoiceProfile) error {
	if vp.ID == "" {
		vp.ID = uuid.New().String()
	}
	vp.UpdatedAt = time.Now()
	if vp.CreatedAt.IsZero() {
		vp.CreatedAt = vp.UpdatedAt
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO voice_profiles (id, name, source, engine, voice_id, sample_path, design_prompt, description, language, gender, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		vp.ID, vp.Name, vp.Source, vp.Engine, vp.VoiceID, vp.SamplePath, vp.DesignPrompt, vp.Description, vp.Language, vp.Gender, vp.CreatedAt, vp.UpdatedAt,
	)
	return err
}

func (s *Store) GetVoiceProfile(id string) (*model.VoiceProfile, error) {
	row := s.db.QueryRow(`SELECT id, name, source, engine, voice_id, sample_path, design_prompt, description, language, gender, created_at, updated_at FROM voice_profiles WHERE id = ?`, id)
	vp := &model.VoiceProfile{}
	err := row.Scan(&vp.ID, &vp.Name, &vp.Source, &vp.Engine, &vp.VoiceID, &vp.SamplePath, &vp.DesignPrompt, &vp.Description, &vp.Language, &vp.Gender, &vp.CreatedAt, &vp.UpdatedAt)
	return vp, err
}

func (s *Store) ListVoiceProfiles() ([]*model.VoiceProfile, error) {
	rows, err := s.db.Query(`SELECT id, name, source, engine, voice_id, sample_path, design_prompt, description, language, gender, created_at, updated_at FROM voice_profiles ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var profiles = make([]*model.VoiceProfile, 0)
	for rows.Next() {
		vp := &model.VoiceProfile{}
		if err := rows.Scan(&vp.ID, &vp.Name, &vp.Source, &vp.Engine, &vp.VoiceID, &vp.SamplePath, &vp.DesignPrompt, &vp.Description, &vp.Language, &vp.Gender, &vp.CreatedAt, &vp.UpdatedAt); err != nil {
			return nil, err
		}
		profiles = append(profiles, vp)
	}
	return profiles, nil
}

func (s *Store) DeleteVoiceProfile(id string) error {
	_, err := s.db.Exec(`DELETE FROM voice_profiles WHERE id = ?`, id)
	return err
}

// ---- SynthesisJob CRUD ----

func (s *Store) SaveJob(job *model.SynthesisJob) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	job.UpdatedAt = time.Now()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = job.UpdatedAt
	}
	progressJSON, _ := json.Marshal(job.Progress)
	configJSON, _ := json.Marshal(job.Config)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO synthesis_jobs (id, book_id, status, progress_json, config_json, output_path, output_format, error, created_at, updated_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.BookID, string(job.Status), string(progressJSON), string(configJSON),
		job.OutputPath, job.OutputFormat, job.Error, job.CreatedAt, job.UpdatedAt, job.FinishedAt,
	)
	return err
}

func (s *Store) GetJob(id string) (*model.SynthesisJob, error) {
	row := s.db.QueryRow(`SELECT id, book_id, status, progress_json, config_json, output_path, output_format, error, created_at, updated_at, finished_at FROM synthesis_jobs WHERE id = ?`, id)
	job := &model.SynthesisJob{}
	var progressJSON, configJSON string
	var statusStr string
	err := row.Scan(&job.ID, &job.BookID, &statusStr, &progressJSON, &configJSON,
		&job.OutputPath, &job.OutputFormat, &job.Error, &job.CreatedAt, &job.UpdatedAt, &job.FinishedAt)
	job.Status = model.JobStatus(statusStr)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(progressJSON), &job.Progress)
	json.Unmarshal([]byte(configJSON), &job.Config)
	return job, nil
}

func (s *Store) ListJobs() ([]*model.SynthesisJob, error) {
	rows, err := s.db.Query(`SELECT id, book_id, status, progress_json, config_json, output_path, output_format, error, created_at, updated_at, finished_at FROM synthesis_jobs ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs = make([]*model.SynthesisJob, 0)
	for rows.Next() {
		job := &model.SynthesisJob{}
		var progressJSON, configJSON string
		var statusStr string
		if err := rows.Scan(&job.ID, &job.BookID, &statusStr, &progressJSON, &configJSON, &job.OutputPath, &job.OutputFormat, &job.Error, &job.CreatedAt, &job.UpdatedAt, &job.FinishedAt); err != nil {
			return nil, err
		}
		job.Status = model.JobStatus(statusStr)
		json.Unmarshal([]byte(progressJSON), &job.Progress)
		json.Unmarshal([]byte(configJSON), &job.Config)
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *Store) UpdateJobStatus(id string, status model.JobStatus, errMsg string) error {
	var finished *time.Time
	if status == model.JobCompleted || status == model.JobFailed || status == model.JobCancelled {
		now := time.Now()
		finished = &now
	}
	_, dbErr := s.db.Exec(
		`UPDATE synthesis_jobs SET status = ?, error = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
		string(status), errMsg, finished, time.Now(), id,
	)
	return dbErr
}

func (s *Store) UpdateJobProgress(id string, progress model.JobProgress) error {
	progressJSON, _ := json.Marshal(progress)
	_, err := s.db.Exec(`UPDATE synthesis_jobs SET progress_json = ?, updated_at = ? WHERE id = ?`,
		string(progressJSON), time.Now(), id)
	return err
}

// ---- AudioSegment CRUD ----

func (s *Store) SaveSegment(seg *model.AudioSegment) error {
	if seg.ID == "" {
		seg.ID = uuid.New().String()
	}
	seg.CreatedAt = time.Now()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO audio_segments (id, job_id, chapter_idx, segment_idx, text, audio_path, format, duration, status, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		seg.ID, seg.JobID, seg.ChapterIdx, seg.SegmentIdx, seg.Text, seg.AudioPath, seg.Format, seg.Duration, seg.Status, seg.Error, seg.CreatedAt,
	)
	return err
}

func (s *Store) GetJobSegments(jobID string) ([]*model.AudioSegment, error) {
	rows, err := s.db.Query(`SELECT id, job_id, chapter_idx, segment_idx, text, audio_path, format, duration, status, error, created_at FROM audio_segments WHERE job_id = ? ORDER BY chapter_idx, segment_idx`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var segs = make([]*model.AudioSegment, 0)
	for rows.Next() {
		seg := &model.AudioSegment{}
		if err := rows.Scan(&seg.ID, &seg.JobID, &seg.ChapterIdx, &seg.SegmentIdx, &seg.Text, &seg.AudioPath, &seg.Format, &seg.Duration, &seg.Status, &seg.Error, &seg.CreatedAt); err != nil {
			return nil, err
		}
		segs = append(segs, seg)
	}
	return segs, nil
}
