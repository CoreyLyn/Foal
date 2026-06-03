package history

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Recorder interface {
	Record(ctx context.Context, session SessionRecord, items []ItemRecord) error
}

type CommandParameters struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type SessionRecord struct {
	ID        string            `json:"id"`
	Command   CommandParameters `json:"command_parameters"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at"`
	Mode      string            `json:"mode"`
	Aggregate AggregateOutcomes `json:"aggregate_outcomes"`
}

type AggregateOutcomes struct {
	CandidateCount int   `json:"candidate_count"`
	DeletedCount   int   `json:"deleted_count"`
	SkippedCount   int   `json:"skipped_count"`
	ErrorCount     int   `json:"error_count"`
	CandidateBytes int64 `json:"candidate_bytes"`
	AffectedBytes  int64 `json:"affected_bytes"`
}

type ItemRecord struct {
	SessionID     string `json:"session_id"`
	Path          string `json:"path"`
	Rule          string `json:"rule,omitempty"`
	PlannedAction string `json:"planned_action,omitempty"`
	Action        string `json:"action,omitempty"`
	Bytes         *int64 `json:"bytes,omitempty"`
	Result        string `json:"result"`
	SkippedReason *Issue `json:"skipped_reason,omitempty"`
	Error         *Issue `json:"error,omitempty"`
}

type Issue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
}

type Record struct {
	Type    string         `json:"type"`
	Session *SessionRecord `json:"session,omitempty"`
	Item    *ItemRecord    `json:"item,omitempty"`
}

type FileRecorder struct {
	dir string
}

func NewDefaultFileRecorder() (FileRecorder, error) {
	dir := os.Getenv("FOAL_HISTORY_DIR")
	if dir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return FileRecorder{}, err
		}
		dir = filepath.Join(configDir, "Foal", "history")
	}
	return NewFileRecorder(dir), nil
}

func NewFileRecorder(dir string) FileRecorder {
	return FileRecorder{dir: dir}
}

func (r FileRecorder) Record(ctx context.Context, session SessionRecord, items []ItemRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if session.ID == "" {
		return fmt.Errorf("history session id is required")
	}
	if err := os.MkdirAll(r.dir, 0700); err != nil {
		return err
	}

	path := filepath.Join(r.dir, session.ID+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(Record{Type: "session", Session: &session}); err != nil {
		return err
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		item.SessionID = session.ID
		if err := encoder.Encode(Record{Type: "item", Item: &item}); err != nil {
			return err
		}
	}
	return nil
}
