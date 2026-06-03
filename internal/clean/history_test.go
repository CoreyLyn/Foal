package clean_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CoreyLyn/Foal/internal/history"
)

type recordingHistoryRecorder struct {
	sessions []history.SessionRecord
	items    []history.ItemRecord
	encoded  string
}

func (r *recordingHistoryRecorder) Record(ctx context.Context, session history.SessionRecord, items []history.ItemRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.sessions = append(r.sessions, session)
	r.items = append(r.items, items...)
	payload := struct {
		Session history.SessionRecord `json:"session"`
		Items   []history.ItemRecord  `json:"items"`
	}{Session: session, Items: items}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	r.encoded += string(data)
	return nil
}

func mustHaveIssue(t *testing.T, issue *history.Issue, code string) {
	t.Helper()
	if issue == nil {
		t.Fatalf("issue is nil, want %q", code)
	}
	if issue.Code != code || issue.Message == "" || !issue.Recoverable {
		t.Fatalf("issue = %#v, want recoverable %q", issue, code)
	}
}
