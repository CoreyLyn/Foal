package history

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

type QueryResult struct {
	Status   string          `json:"status"`
	Sessions []SessionResult `json:"sessions"`
	Errors   []QueryIssue    `json:"errors"`
}

type SessionResult struct {
	ID        string            `json:"id"`
	Command   CommandParameters `json:"command_parameters"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at"`
	Mode      string            `json:"mode"`
	Aggregate AggregateOutcomes `json:"aggregate_outcomes"`
	Items     []ItemRecord      `json:"items"`
}

type QueryIssue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	Path        string `json:"path,omitempty"`
}

type FileRecorder struct {
	dir string
}

type FileQuery struct {
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

func NewDefaultFileQuery() (FileQuery, error) {
	dir := os.Getenv("FOAL_HISTORY_DIR")
	if dir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return FileQuery{}, err
		}
		dir = filepath.Join(configDir, "Foal", "history")
	}
	return NewFileQuery(dir), nil
}

func NewFileQuery(dir string) FileQuery {
	return FileQuery{dir: dir}
}

func (q FileQuery) Recent(ctx context.Context) QueryResult {
	if ctx == nil {
		ctx = context.Background()
	}
	result := QueryResult{
		Status:   "ok",
		Sessions: []SessionResult{},
		Errors:   []QueryIssue{},
	}
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return result
		}
		result.Status = "partial"
		result.Errors = append(result.Errors, queryIssue("history_read_failed", err.Error(), true, q.dir))
		return result
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			result.Status = "partial"
			result.Errors = append(result.Errors, queryIssue("context_canceled", err.Error(), true, q.dir))
			return result
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		session, err := readHistorySessionFile(ctx, filepath.Join(q.dir, entry.Name()))
		if err != nil {
			result.Status = "partial"
			result.Errors = append(result.Errors, queryIssue("history_decode_failed", err.Error(), true, filepath.Join(q.dir, entry.Name())))
			continue
		}
		result.Sessions = append(result.Sessions, session)
	}

	sort.SliceStable(result.Sessions, func(i, j int) bool {
		return result.Sessions[i].EndedAt.After(result.Sessions[j].EndedAt)
	})
	return result
}

func readHistorySessionFile(ctx context.Context, path string) (SessionResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return SessionResult{}, err
	}
	defer file.Close()

	var session *SessionRecord
	var items []ItemRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return SessionResult{}, err
		}
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return SessionResult{}, err
		}
		switch record.Type {
		case "session":
			if record.Session == nil {
				return SessionResult{}, fmt.Errorf("history session record is empty")
			}
			copied := *record.Session
			session = &copied
		case "item":
			if record.Item == nil {
				return SessionResult{}, fmt.Errorf("history item record is empty")
			}
			items = append(items, *record.Item)
		default:
			return SessionResult{}, fmt.Errorf("unknown history record type: %s", record.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		return SessionResult{}, err
	}
	if session == nil {
		return SessionResult{}, fmt.Errorf("history session record is missing")
	}
	return SessionResult{
		ID:        session.ID,
		Command:   session.Command,
		StartedAt: session.StartedAt,
		EndedAt:   session.EndedAt,
		Mode:      session.Mode,
		Aggregate: session.Aggregate,
		Items:     items,
	}, nil
}

func queryIssue(code, message string, recoverable bool, path string) QueryIssue {
	return QueryIssue{
		Code:        code,
		Message:     message,
		Recoverable: recoverable,
		Path:        path,
	}
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
