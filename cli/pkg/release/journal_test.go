package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// journalRecs is the canonical full successful operation sequence used by the
// recovery fixtures: op-start → prepared → committing → mutated → committed →
// state-finalized → op-end.
func journalRecs() []JournalRecord {
	return []JournalRecord{
		{OpID: "op1", Phase: JournalOpStart, Tag: "config-v1.0.0", Digest: strings.Repeat("a", 64)},
		{OpID: "op1", Phase: JournalPrepared},
		{OpID: "op1", Phase: JournalCommitting},
		{OpID: "op1", Phase: JournalMutated, Payload: "home/.config/hypr/hyprland.conf"},
		{OpID: "op1", Phase: JournalCommitted, Payload: "home/.config/hypr/hyprland.conf"},
		{OpID: "op1", Phase: JournalStateFinalized},
		{OpID: "op1", Phase: JournalOpEnd},
	}
}

// writeJournalLines marshals the given records as newline-delimited JSON.
func writeJournalLines(t *testing.T, path string, recs []JournalRecord) {
	t.Helper()
	if err := os.WriteFile(path, journalLines(t, recs), 0o600); err != nil {
		t.Fatalf("write journal fixture: %v", err)
	}
}

func TestJournal_RecoveryConvergesState(t *testing.T) {
	full := journalRecs()
	cases := []struct {
		name    string
		records []JournalRecord
		want    JournalOutcome
	}{
		{
			name:    "missing journal has no pending work",
			records: nil,
			want:    JournalOutcomeCommitted,
		},
		{
			name:    "empty journal has no pending work",
			records: []JournalRecord{},
			want:    JournalOutcomeCommitted,
		},
		{
			name:    "crash immediately after op-start",
			records: full[:1],
			want:    JournalOutcomeUncommitted,
		},
		{
			name:    "crash after prepared",
			records: full[:2],
			want:    JournalOutcomeUncommitted,
		},
		{
			name:    "crash during committing",
			records: full[:3],
			want:    JournalOutcomeUncommitted,
		},
		{
			name:    "crash mid-mutation before commit",
			records: full[:4],
			want:    JournalOutcomeUncommitted,
		},
		{
			name:    "crash after commit before state update finalizes without reapply",
			records: full[:5],
			want:    JournalOutcomeCommitted,
		},
		{
			name:    "crash after state-finalized before op-end",
			records: full[:6],
			want:    JournalOutcomeCommitted,
		},
		{
			name:    "completed operation",
			records: full,
			want:    JournalOutcomeCommitted,
		},
		{
			name: "two completed operations back to back",
			records: append(append([]JournalRecord{}, full...),
				JournalRecord{OpID: "op2", Phase: JournalOpStart, Tag: "config-v1.1.0", Digest: strings.Repeat("b", 64)},
				JournalRecord{OpID: "op2", Phase: JournalPrepared},
				JournalRecord{OpID: "op2", Phase: JournalCommitting},
				JournalRecord{OpID: "op2", Phase: JournalCommitted},
				JournalRecord{OpID: "op2", Phase: JournalStateFinalized},
				JournalRecord{OpID: "op2", Phase: JournalOpEnd},
			),
			want: JournalOutcomeCommitted,
		},
		{
			name: "second operation interrupted mid-mutation",
			records: append(append([]JournalRecord{}, full...),
				JournalRecord{OpID: "op2", Phase: JournalOpStart, Tag: "config-v1.1.0", Digest: strings.Repeat("b", 64)},
				JournalRecord{OpID: "op2", Phase: JournalPrepared},
				JournalRecord{OpID: "op2", Phase: JournalCommitting},
				JournalRecord{OpID: "op2", Phase: JournalMutated, Payload: "home/.config/waybar/config.jsonc"},
			),
			want: JournalOutcomeUncommitted,
		},
		{
			name: "zero-target operation reaches state-finalized without target records",
			records: []JournalRecord{
				{OpID: "op3", Phase: JournalOpStart, Tag: "config-v1.2.0", Digest: strings.Repeat("c", 64)},
				{OpID: "op3", Phase: JournalPrepared},
				{OpID: "op3", Phase: JournalCommitting},
				{OpID: "op3", Phase: JournalStateFinalized},
				{OpID: "op3", Phase: JournalOpEnd},
			},
			want: JournalOutcomeCommitted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := NewJournal(filepath.Join(t.TempDir(), "journal.ndjson"))
			if tc.records != nil {
				writeJournalLines(t, j.path, tc.records)
			}
			outcome, recs, err := j.Recovery()
			if err != nil {
				t.Fatalf("Recovery: %v", err)
			}
			if outcome != tc.want {
				t.Fatalf("outcome = %q, want %q (last phase %q)", outcome, tc.want, lastPhase(recs))
			}
		})
	}
}

// lastPhase returns the phase of the final parsed record, or "" when empty.
func lastPhase(recs []JournalRecord) JournalPhase {
	if len(recs) == 0 {
		return ""
	}
	return recs[len(recs)-1].Phase
}

func TestJournal_TruncatedTailIsIndeterminate(t *testing.T) {
	fullLines := journalLines(t, journalRecs())
	offsets := journalLineOffsets(fullLines)

	// Truncate the byte stream inside the record that would follow every phase
	// boundary: the tail record is left partially written and the file no
	// longer ends in a newline, so recovery must refuse to infer success.
	for i, start := range offsets {
		lineEnd := len(fullLines)
		if i+1 < len(offsets) {
			lineEnd = offsets[i+1]
		}
		cuts := []int{start + 1, start + (lineEnd-start)/2, lineEnd - 1}
		for _, cut := range cuts {
			if cut <= start || cut >= lineEnd {
				continue
			}
			t.Run(fmt.Sprintf("record-%d-cut-%d", i, cut), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "journal.ndjson")
				if err := os.WriteFile(path, fullLines[:cut], 0o600); err != nil {
					t.Fatalf("write truncated journal: %v", err)
				}
				j := NewJournal(path)
				outcome, _, err := j.Recovery()
				if !errors.Is(err, ErrIndeterminateJournal) {
					t.Fatalf("expected ErrIndeterminateJournal, got outcome=%q err=%v", outcome, err)
				}
				if outcome != JournalOutcomeIndeterminate {
					t.Fatalf("outcome = %q, want indeterminate", outcome)
				}
			})
		}
	}
}

func TestJournal_TruncatedOpEndNeverCommits(t *testing.T) {
	// The op-end line itself is cut short: the operation otherwise looks
	// complete, but a truncated tail must NEVER be treated as success.
	fullLines := journalLines(t, journalRecs())
	path := filepath.Join(t.TempDir(), "journal.ndjson")
	if err := os.WriteFile(path, fullLines[:len(fullLines)-5], 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	outcome, _, err := NewJournal(path).Recovery()
	if !errors.Is(err, ErrIndeterminateJournal) {
		t.Fatalf("expected ErrIndeterminateJournal, got outcome=%q err=%v", outcome, err)
	}
	if outcome != JournalOutcomeIndeterminate {
		t.Fatalf("outcome = %q, want indeterminate", outcome)
	}
}

func TestJournal_IllegalOrderingIsIndeterminate(t *testing.T) {
	// A complete op-start line is required; "op1" is a single record payload.
	cases := []struct {
		name    string
		records []JournalRecord
	}{
		{
			name: "first record is not op-start",
			records: []JournalRecord{
				{OpID: "op1", Phase: JournalPrepared},
			},
		},
		{
			name: "op-end before committed",
			records: []JournalRecord{
				{OpID: "op1", Phase: JournalOpStart},
				{OpID: "op1", Phase: JournalPrepared},
				{OpID: "op1", Phase: JournalCommitting},
				{OpID: "op1", Phase: JournalOpEnd},
			},
		},
		{
			name: "state-finalized before committed",
			records: []JournalRecord{
				{OpID: "op1", Phase: JournalOpStart},
				{OpID: "op1", Phase: JournalPrepared},
				{OpID: "op1", Phase: JournalCommitting},
				{OpID: "op1", Phase: JournalMutated},
				{OpID: "op1", Phase: JournalStateFinalized},
			},
		},
		{
			name: "mutated after committed",
			records: []JournalRecord{
				{OpID: "op1", Phase: JournalOpStart},
				{OpID: "op1", Phase: JournalPrepared},
				{OpID: "op1", Phase: JournalCommitting},
				{OpID: "op1", Phase: JournalCommitted},
				{OpID: "op1", Phase: JournalMutated},
			},
		},
		{
			name: "second op starts before first closes",
			records: []JournalRecord{
				{OpID: "op1", Phase: JournalOpStart},
				{OpID: "op1", Phase: JournalPrepared},
				{OpID: "op1", Phase: JournalCommitting},
				{OpID: "op2", Phase: JournalOpStart},
			},
		},
		{
			name: "prepared after op-end starts nothing",
			records: []JournalRecord{
				{OpID: "op1", Phase: JournalOpStart},
				{OpID: "op1", Phase: JournalPrepared},
				{OpID: "op1", Phase: JournalCommitting},
				{OpID: "op1", Phase: JournalCommitted},
				{OpID: "op1", Phase: JournalStateFinalized},
				{OpID: "op1", Phase: JournalOpEnd},
				{OpID: "op2", Phase: JournalPrepared},
			},
		},
		{
			name: "unknown phase value",
			records: []JournalRecord{
				{OpID: "op1", Phase: JournalOpStart},
				{OpID: "op1", Phase: "banana"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "journal.ndjson")
			writeJournalLines(t, path, tc.records)
			outcome, _, err := NewJournal(path).Recovery()
			if !errors.Is(err, ErrIndeterminateJournal) {
				t.Fatalf("expected ErrIndeterminateJournal, got outcome=%q err=%v", outcome, err)
			}
			if outcome != JournalOutcomeIndeterminate {
				t.Fatalf("outcome = %q, want indeterminate", outcome)
			}
		})
	}
}

func TestJournal_InvalidLineIsIndeterminate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.ndjson")
	if err := os.WriteFile(path, []byte(`{"op_id":"op1","phase":"op-start"}`+"\n"+`not json at all`+"\n"), 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	outcome, recs, err := NewJournal(path).Recovery()
	if !errors.Is(err, ErrIndeterminateJournal) {
		t.Fatalf("expected ErrIndeterminateJournal, got outcome=%q err=%v", outcome, err)
	}
	if outcome != JournalOutcomeIndeterminate {
		t.Fatalf("outcome = %q, want indeterminate", outcome)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record parsed before failure, got %d", len(recs))
	}
}

func TestJournal_AppendEnforcesLegalOrdering(t *testing.T) {
	j := NewJournal(filepath.Join(t.TempDir(), "journal.ndjson"))
	legal := []JournalPhase{
		JournalOpStart, JournalPrepared, JournalCommitting,
		JournalMutated, JournalMutated, JournalCommitted, JournalStateFinalized, JournalOpEnd,
	}
	rec := JournalRecord{OpID: "op1"}
	for _, ph := range legal {
		rec.Phase = ph
		if err := j.Append(rec); err != nil {
			t.Fatalf("append %s: %v", ph, err)
		}
	}

	outcome, recs, err := j.Recovery()
	if err != nil {
		t.Fatalf("Recovery after legal appends: %v", err)
	}
	if outcome != JournalOutcomeCommitted {
		t.Fatalf("outcome = %q, want committed", outcome)
	}
	if len(recs) != len(legal) {
		t.Fatalf("recovered %d records, want %d", len(recs), len(legal))
	}

	illegal := []struct {
		name string
		next JournalPhase
	}{
		{"op-end after op-start", JournalOpEnd},
		{"state-finalized after committing", JournalStateFinalized},
		{"committed after prepared", JournalCommitted},
	}
	for _, tc := range illegal {
		t.Run(tc.name, func(t *testing.T) {
			fresh := NewJournal(filepath.Join(t.TempDir(), "journal.ndjson"))
			base := JournalRecord{OpID: "op1", Phase: JournalOpStart}
			if err := fresh.Append(base); err != nil {
				t.Fatalf("append op-start: %v", err)
			}
			bad := base
			bad.Phase = tc.next
			if err := fresh.Append(bad); err == nil {
				t.Fatalf("expected append of %s after op-start to fail", tc.next)
			}
		})
	}
}

func TestJournal_AppendRefusesTruncatedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.ndjson")
	// A valid op-start line followed by a partial record with no newline.
	if err := os.WriteFile(path, []byte(`{"op_id":"op1","phase":"op-start"}`+"\n"+`{"op_id":"op1","phase":"prep`), 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	err := NewJournal(path).Append(JournalRecord{OpID: "op2", Phase: JournalOpStart})
	if !errors.Is(err, ErrIndeterminateJournal) {
		t.Fatalf("expected ErrIndeterminateJournal appending to truncated tail, got %v", err)
	}
}

// journalLines returns the complete NDJSON bytes for the records.
func journalLines(t *testing.T, recs []JournalRecord) []byte {
	t.Helper()
	var sb strings.Builder
	for _, r := range recs {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

// journalLineOffsets returns the byte offset where each NDJSON line starts.
func journalLineOffsets(lines []byte) []int {
	var offsets []int
	for i := 0; i < len(lines); i++ {
		if i == 0 || lines[i-1] == '\n' {
			offsets = append(offsets, i)
		}
	}
	return offsets
}
