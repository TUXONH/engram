package store

import (
	"errors"
	"strings"
	"testing"
)

func TestListEndedSessionPostEndActivityReturnsQueryErrors(t *testing.T) {
	tests := []struct {
		name, marker string
	}{
		{name: "discovery", marker: "SELECT DISTINCT s.id"},
		{name: "conversation rows", marker: "ended_at FROM sessions"},
		{name: "post-end prompts", marker: "latest_end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			if err := s.CreateSession("root-a", "engram", "/work/engram"); err != nil {
				t.Fatal(err)
			}
			if err := s.EndSession("root-a", "done"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB().Exec(`UPDATE sessions SET ended_at = '2026-01-01 10:00:00' WHERE id = 'root-a'`); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB().Exec(`INSERT INTO user_prompts (session_id, content, project, created_at) VALUES ('root-a', 'prompt', 'engram', '2026-01-01 10:05:00')`); err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected query failure")
			s.hooks.queryIt = func(db queryer, query string, args ...any) (rowScanner, error) {
				if strings.Contains(query, tt.marker) {
					return nil, injected
				}
				rows, err := db.Query(query, args...)
				if err != nil {
					return nil, err
				}
				return sqlRowScanner{rows: rows}, nil
			}

			evidence, err := s.ListEndedSessionPostEndActivity("engram")
			if !errors.Is(err, injected) || evidence != nil {
				t.Fatalf("evidence=%+v err=%v, want nil evidence and the injected error", evidence, err)
			}
		})
	}
}
