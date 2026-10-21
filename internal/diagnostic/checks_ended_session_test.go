package diagnostic

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The safe next step and docs/DOCTOR.md both name the release that binds
// resumed sessions to their live continuation, so they must not drift.
func TestEndedSessionPostEndActivityDocsNameResumedSessionBindingRelease(t *testing.T) {
	doc, err := os.ReadFile("../../docs/DOCTOR.md")
	if err != nil {
		t.Fatalf("read docs/DOCTOR.md: %v", err)
	}
	for _, line := range strings.Split(string(doc), "\n") {
		if !strings.HasPrefix(line, "- `"+CheckEndedSessionPostEndActivity+"` —") {
			continue
		}
		if !strings.Contains(line, "predates "+resumedSessionBindingRelease+".") {
			t.Fatalf("docs/DOCTOR.md entry for %s does not name release %s: %s", CheckEndedSessionPostEndActivity, resumedSessionBindingRelease, line)
		}
		return
	}
	t.Fatalf("docs/DOCTOR.md has no entry for %s", CheckEndedSessionPostEndActivity)
}

func TestEndedSessionPostEndActivityCheck(t *testing.T) {
	type session struct {
		id, project, endedAt string // empty endedAt keeps the session open
	}
	type prompt struct {
		sessionID, createdAt string
	}
	type wantFinding struct {
		SessionID             string `json:"session_id"`
		Project               string `json:"project"`
		EndedAt               string `json:"ended_at"`
		LatestPostEndPromptAt string `json:"latest_post_end_prompt_at"`
		PostEndPromptCount    int64  `json:"post_end_prompt_count"`
		ContinuationState     string `json:"continuation_state"`
		ContinuationCount     int    `json:"continuation_count"`
	}
	tests := []struct {
		name     string
		project  string
		sessions []session
		prompts  []prompt
		want     []wantFinding
	}{
		{
			name:     "ended root with post-end prompts and no continuation warns",
			project:  "engram",
			sessions: []session{{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"}},
			prompts: []prompt{
				{sessionID: "root-a", createdAt: "2026-01-01 09:59:00"},
				{sessionID: "root-a", createdAt: "2026-01-01 10:01:00"},
				{sessionID: "root-a", createdAt: "2026-01-01 10:05:00"},
			},
			want: []wantFinding{{SessionID: "root-a", Project: "engram", EndedAt: "2026-01-01 10:00:00", LatestPostEndPromptAt: "2026-01-01 10:05:00", PostEndPromptCount: 2, ContinuationState: "none"}},
		},
		{
			name:    "open continuation is ok",
			project: "engram",
			sessions: []session{
				{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"},
				{id: "root-a:resume:2", project: "engram"},
			},
			prompts: []prompt{{sessionID: "root-a", createdAt: "2026-01-01 10:05:00"}},
		},
		{
			name:     "ended root without post-end activity is ok",
			project:  "engram",
			sessions: []session{{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"}},
			prompts:  []prompt{{sessionID: "root-a", createdAt: "2026-01-01 09:59:00"}},
		},
		{
			name:    "closed continuations report the conversation once after its latest end",
			project: "engram",
			sessions: []session{
				{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"},
				{id: "root-a:resume:2", project: "engram", endedAt: "2026-01-01 11:00:00"},
			},
			prompts: []prompt{
				{sessionID: "root-a", createdAt: "2026-01-01 10:05:00"},
				{sessionID: "root-a:resume:2", createdAt: "2026-01-01 11:10:00"},
				{sessionID: "root-a", createdAt: "2026-01-01 11:20:00"},
			},
			want: []wantFinding{{SessionID: "root-a", Project: "engram", EndedAt: "2026-01-01 11:00:00", LatestPostEndPromptAt: "2026-01-01 11:20:00", PostEndPromptCount: 2, ContinuationState: "ended", ContinuationCount: 1}},
		},
		{
			name:    "activity picked up by a later continuation is ok",
			project: "engram",
			sessions: []session{
				{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"},
				{id: "root-a:resume:2", project: "engram", endedAt: "2026-01-01 11:00:00"},
			},
			prompts: []prompt{
				{sessionID: "root-a", createdAt: "2026-01-01 10:05:00"},
				{sessionID: "root-a:resume:2", createdAt: "2026-01-01 10:30:00"},
			},
		},
		{
			name:     "prompt timestamps in RFC 3339 layout compare by time",
			project:  "engram",
			sessions: []session{{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"}},
			prompts: []prompt{
				{sessionID: "root-a", createdAt: "2026-01-01T09:30:00Z"},
				{sessionID: "root-a", createdAt: "2026-01-01T10:30:00.123456789Z"},
			},
			want: []wantFinding{{SessionID: "root-a", Project: "engram", EndedAt: "2026-01-01 10:00:00", LatestPostEndPromptAt: "2026-01-01 10:30:00", PostEndPromptCount: 1, ContinuationState: "none"}},
		},
		{
			name:     "activity in another project is not reported",
			project:  "engram",
			sessions: []session{{id: "root-b", project: "other", endedAt: "2026-01-01 10:00:00"}},
			prompts:  []prompt{{sessionID: "root-b", createdAt: "2026-01-01 10:05:00"}},
		},
		{
			name:    "conversation rooted in another project is not reported",
			project: "engram",
			sessions: []session{
				{id: "root-b", project: "other", endedAt: "2026-01-01 10:00:00"},
				{id: "root-b:resume:2", project: "engram", endedAt: "2026-01-01 11:00:00"},
			},
			prompts: []prompt{{sessionID: "root-b:resume:2", createdAt: "2026-01-01 11:10:00"}},
		},
		{
			name:    "conversation rooted in the scoped project is reported with its root project",
			project: "engram",
			sessions: []session{
				{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"},
				{id: "root-a:resume:2", project: "other", endedAt: "2026-01-01 11:00:00"},
			},
			prompts: []prompt{{sessionID: "root-a:resume:2", createdAt: "2026-01-01 11:10:00"}},
			want:    []wantFinding{{SessionID: "root-a", Project: "engram", EndedAt: "2026-01-01 11:00:00", LatestPostEndPromptAt: "2026-01-01 11:10:00", PostEndPromptCount: 1, ContinuationState: "ended", ContinuationCount: 1}},
		},
		{
			name:    "unparseable end in the conversation is not reported",
			project: "engram",
			sessions: []session{
				{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"},
				{id: "root-a:resume:2", project: "engram", endedAt: "not-a-time"},
			},
			prompts: []prompt{{sessionID: "root-a", createdAt: "2026-01-01 10:05:00"}},
		},
		{
			name: "unscoped run reports every conversation in root order",
			sessions: []session{
				{id: "root-b", project: "other", endedAt: "2026-01-01 10:00:00"},
				{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"},
			},
			prompts: []prompt{
				{sessionID: "root-b", createdAt: "2026-01-01 10:05:00"},
				{sessionID: "root-a", createdAt: "2026-01-01 10:06:00"},
			},
			want: []wantFinding{
				{SessionID: "root-a", Project: "engram", EndedAt: "2026-01-01 10:00:00", LatestPostEndPromptAt: "2026-01-01 10:06:00", PostEndPromptCount: 1, ContinuationState: "none"},
				{SessionID: "root-b", Project: "other", EndedAt: "2026-01-01 10:00:00", LatestPostEndPromptAt: "2026-01-01 10:05:00", PostEndPromptCount: 1, ContinuationState: "none"},
			},
		},
		{
			name:    "missing root takes the lowest-ordinal continuation's project",
			project: "engram",
			sessions: []session{
				{id: "root-a:resume:10", project: "other", endedAt: "2026-01-01 11:00:00"},
				{id: "root-a:resume:2", project: "engram", endedAt: "2026-01-01 10:00:00"},
			},
			prompts: []prompt{{sessionID: "root-a:resume:2", createdAt: "2026-01-01 11:30:00"}},
			want:    []wantFinding{{SessionID: "root-a", Project: "engram", EndedAt: "2026-01-01 11:00:00", LatestPostEndPromptAt: "2026-01-01 11:30:00", PostEndPromptCount: 1, ContinuationState: "ended", ContinuationCount: 2}},
		},
		{
			name:    "nested continuation belongs to the outer conversation",
			project: "engram",
			sessions: []session{
				{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"},
				{id: "root-a:resume:2", project: "engram", endedAt: "2026-01-01 11:00:00"},
				{id: "root-a:resume:2:resume:2", project: "engram", endedAt: "2026-01-01 12:00:00"},
			},
			prompts: []prompt{{sessionID: "root-a:resume:2:resume:2", createdAt: "2026-01-01 12:10:00"}},
			want:    []wantFinding{{SessionID: "root-a", Project: "engram", EndedAt: "2026-01-01 12:00:00", LatestPostEndPromptAt: "2026-01-01 12:10:00", PostEndPromptCount: 1, ContinuationState: "ended", ContinuationCount: 2}},
		},
		{
			name:    "open nested continuation is ok",
			project: "engram",
			sessions: []session{
				{id: "root-a", project: "engram", endedAt: "2026-01-01 10:00:00"},
				{id: "root-a:resume:2", project: "engram", endedAt: "2026-01-01 11:00:00"},
				{id: "root-a:resume:2:resume:2", project: "engram"},
			},
			prompts: []prompt{{sessionID: "root-a", createdAt: "2026-01-01 11:30:00"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newDiagnosticTestStore(t)
			projects := make(map[string]string, len(tt.sessions))
			for _, session := range tt.sessions {
				projects[session.id] = session.project
				if err := s.CreateSession(session.id, session.project, "/work/"+session.project); err != nil {
					t.Fatalf("create session %q: %v", session.id, err)
				}
				if session.endedAt == "" {
					continue
				}
				if err := s.EndSession(session.id, "done"); err != nil {
					t.Fatalf("EndSession(%q): %v", session.id, err)
				}
				if _, err := s.DB().Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, session.endedAt, session.id); err != nil {
					t.Fatalf("set ended_at for %q: %v", session.id, err)
				}
			}
			for _, p := range tt.prompts {
				if _, err := s.DB().Exec(`INSERT INTO user_prompts (session_id, content, project, created_at) VALUES (?, 'prompt', ?, ?)`, p.sessionID, projects[p.sessionID], p.createdAt); err != nil {
					t.Fatalf("insert prompt for %q: %v", p.sessionID, err)
				}
			}

			report, err := NewRunner().RunOne(context.Background(), Scope{Store: s, Project: tt.project}, CheckEndedSessionPostEndActivity)
			if err != nil {
				t.Fatalf("RunOne: %v", err)
			}
			check := report.Checks[0]
			var summary map[string]int
			if err := json.Unmarshal(check.Evidence, &summary); err != nil {
				t.Fatalf("decode check evidence: %v", err)
			}
			if len(tt.want) == 0 {
				if report.Status != StatusOK || len(check.Findings) != 0 || summary["conversations_reported"] != 0 {
					t.Fatalf("status=%q findings=%+v evidence=%s, want ok with no findings and conversations_reported 0", report.Status, check.Findings, check.Evidence)
				}
				return
			}
			if report.Status != StatusWarning || len(check.Findings) != len(tt.want) || summary["finding_count"] != len(tt.want) {
				t.Fatalf("status=%q findings=%+v evidence=%s, want %d warning(s)", report.Status, check.Findings, check.Evidence, len(tt.want))
			}
			for i, finding := range check.Findings {
				if finding.Severity != SeverityWarning || finding.ReasonCode != CheckEndedSessionPostEndActivity {
					t.Fatalf("finding %d severity=%q reason=%q, want warning %q", i, finding.Severity, finding.ReasonCode, CheckEndedSessionPostEndActivity)
				}
				if !strings.Contains(finding.SafeNextStep, "predates "+resumedSessionBindingRelease+", update it first") {
					t.Fatalf("finding %d safe next step %q does not name release %s", i, finding.SafeNextStep, resumedSessionBindingRelease)
				}
				var got wantFinding
				if err := json.Unmarshal(finding.Evidence, &got); err != nil {
					t.Fatalf("decode finding %d evidence: %v", i, err)
				}
				if got != tt.want[i] {
					t.Fatalf("finding %d evidence=%+v, want %+v", i, got, tt.want[i])
				}
			}
		})
	}
}
