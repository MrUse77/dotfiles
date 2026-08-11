package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// JournalPhase is one transition of a config-release operation. Records are
// appended as newline-delimited JSON to XDG_STATE_HOME/moonarch/journal.ndjson,
// one line per phase transition, under the exclusive release lock.
type JournalPhase string

const (
	JournalOpStart        JournalPhase = "op-start"
	JournalPrepared       JournalPhase = "prepared"
	JournalCommitting     JournalPhase = "committing"
	JournalMutated        JournalPhase = "mutated"
	JournalCommitted      JournalPhase = "committed"
	JournalStateFinalized JournalPhase = "state-finalized"
	JournalOpEnd          JournalPhase = "op-end"
)

// JournalOutcome is the recovery verdict for the journal tail.
type JournalOutcome string

const (
	JournalOutcomeCommitted     JournalOutcome = "committed"
	JournalOutcomeUncommitted   JournalOutcome = "uncommitted"
	JournalOutcomeIndeterminate JournalOutcome = "indeterminate"
)

// JournalRecord is one NDJSON line. Payload carries per-target detail for
// mutated/committed records (the target path).
type JournalRecord struct {
	OpID    string       `json:"op_id"`
	Phase   JournalPhase `json:"phase"`
	Tag     string       `json:"tag,omitempty"`
	Digest  string       `json:"digest,omitempty"`
	Payload any          `json:"payload,omitempty"`
	Ts      time.Time    `json:"ts"`
}

// journalNext is the legal phase transition table. An empty last phase means
// "no records yet": only op-start may begin a journal or a new operation after
// op-end. state-finalized may follow committing directly (a zero-target op)
// but otherwise every mutation must land before committed, and committed is
// required before state-finalized. Once committed is seen, mutated may not
// reappear: mutation records are written during Prepare, strictly before the
// per-target committed records.
var journalNext = map[JournalPhase][]JournalPhase{
	"":                    {JournalOpStart},
	JournalOpStart:        {JournalPrepared},
	JournalPrepared:       {JournalCommitting},
	JournalCommitting:     {JournalMutated, JournalCommitted, JournalStateFinalized},
	JournalMutated:        {JournalMutated, JournalCommitted},
	JournalCommitted:      {JournalCommitted, JournalStateFinalized},
	JournalStateFinalized: {JournalOpEnd},
	JournalOpEnd:          {JournalOpStart},
}

// JournalError wraps a journal read/append/recovery failure. Offset is the
// byte offset of the offending record when known. Recovery failures satisfy
// errors.Is(err, ErrIndeterminateJournal).
type JournalError struct {
	Op     string
	Offset int64
	Cause  error
}

func (e *JournalError) Error() string {
	if e.Offset >= 0 {
		return fmt.Sprintf("journal %s at byte %d: %v", e.Op, e.Offset, e.Cause)
	}
	return fmt.Sprintf("journal %s: %v", e.Op, e.Cause)
}
func (e *JournalError) Unwrap() error { return e.Cause }

// Journal appends NDJSON records to a single file and reduces the tail during
// recovery. One writer exists at a time because every caller holds the release
// lock.
type Journal struct {
	path string
}

// NewJournal creates a journal backed by path.
func NewJournal(path string) *Journal {
	return &Journal{path: path}
}

// Append validates the phase transition against the journal tail, appends the
// record as one NDJSON line, and fsyncs before returning. It refuses to touch
// a truncated or illegal tail: writing into an indeterminate journal would
// destroy the evidence recovery needs.
func (j *Journal) Append(rec JournalRecord) error {
	if !journalPhaseKnown(rec.Phase) {
		return &JournalError{Op: "append", Offset: -1, Cause: fmt.Errorf("unknown phase %q", rec.Phase)}
	}
	if rec.OpID == "" {
		return &JournalError{Op: "append", Offset: -1, Cause: fmt.Errorf("op_id required")}
	}
	last, err := j.lastPhase()
	if err != nil {
		return err
	}
	if !journalTransitionAllowed(last, rec.Phase) {
		return &JournalError{Op: "append", Offset: -1, Cause: fmt.Errorf("illegal phase transition %q -> %q", last, rec.Phase)}
	}

	dir := filepath.Dir(j.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &JournalError{Op: "append", Offset: -1, Cause: err}
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return &JournalError{Op: "append", Offset: -1, Cause: err}
	}
	line, err := json.Marshal(rec)
	if err != nil {
		_ = f.Close()
		return &JournalError{Op: "append", Offset: -1, Cause: err}
	}
	line = append(line, '\n')
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return &JournalError{Op: "append", Offset: -1, Cause: err}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return &JournalError{Op: "append", Offset: -1, Cause: err}
	}
	if err := f.Close(); err != nil {
		return &JournalError{Op: "append", Offset: -1, Cause: err}
	}
	return nil
}

// Recovery reduces the journal tail under the lock into exactly one of
// Committed, Uncommitted, or Indeterminate. It never infers success from a
// truncated tail: any record that is not complete JSON terminated by a newline,
// or any transition violating the legal ordering, yields Indeterminate with an
// error satisfying errors.Is(err, ErrIndeterminateJournal), and the records
// parsed before the failure. A missing or empty journal means no operation has
// ever been recorded: nothing is pending, so the verdict is Committed.
func (j *Journal) Recovery() (JournalOutcome, []JournalRecord, error) {
	data, err := os.ReadFile(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return JournalOutcomeCommitted, nil, nil
		}
		return "", nil, &JournalError{Op: "recovery", Offset: -1, Cause: err}
	}
	if len(data) == 0 {
		return JournalOutcomeCommitted, nil, nil
	}
	if data[len(data)-1] != '\n' {
		return JournalOutcomeIndeterminate, nil, &JournalError{
			Op:     "recovery",
			Offset: int64(len(data) - 1),
			Cause:  fmt.Errorf("%w: tail record truncated (file does not end in a newline)", ErrIndeterminateJournal),
		}
	}

	var (
		records []JournalRecord
		last    JournalPhase
		offset  int64
	)
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			offset += 1
			continue
		}
		lineStart := offset
		var rec JournalRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return JournalOutcomeIndeterminate, records, &JournalError{
				Op:     "recovery",
				Offset: lineStart,
				Cause:  fmt.Errorf("%w: line %d: %v", ErrIndeterminateJournal, len(records)+1, err),
			}
		}
		if !journalPhaseKnown(rec.Phase) {
			return JournalOutcomeIndeterminate, records, &JournalError{
				Op:     "recovery",
				Offset: lineStart,
				Cause:  fmt.Errorf("%w: unknown phase %q", ErrIndeterminateJournal, rec.Phase),
			}
		}
		if !journalTransitionAllowed(last, rec.Phase) {
			return JournalOutcomeIndeterminate, records, &JournalError{
				Op:     "recovery",
				Offset: lineStart,
				Cause:  fmt.Errorf("%w: illegal phase transition %q -> %q", ErrIndeterminateJournal, last, rec.Phase),
			}
		}
		records = append(records, rec)
		last = rec.Phase
		offset += int64(len(line)) + 1
	}

	switch last {
	case JournalOpEnd, JournalStateFinalized, JournalCommitted:
		// state-finalized is authoritative: the identity rotation is done. A
		// committed record without state-finalized means the transaction
		// committed but the identity update is still pending; recovery
		// finalizes it without ever re-applying. op-end is the closing marker.
		return JournalOutcomeCommitted, records, nil
	default:
		// op-start/prepared/committing/mutated: the operation never reached a
		// commit point, so prior state must be restored.
		return JournalOutcomeUncommitted, records, nil
	}
}

// lastPhase reads the final complete record's phase. A truncated tail or an
// unparsable last line is refused with ErrIndeterminateJournal so Append never
// writes into an indeterminate journal.
func (j *Journal) lastPhase() (JournalPhase, error) {
	data, err := os.ReadFile(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", &JournalError{Op: "append", Offset: -1, Cause: err}
	}
	if len(data) == 0 {
		return "", nil
	}
	if data[len(data)-1] != '\n' {
		return "", &JournalError{
			Op:     "append",
			Offset: int64(len(data) - 1),
			Cause:  fmt.Errorf("%w: cannot append to a truncated journal tail", ErrIndeterminateJournal),
		}
	}
	trimmed := bytes.TrimRight(data, "\n")
	if len(trimmed) == 0 {
		return "", nil
	}
	lastIdx := bytes.LastIndexByte(trimmed, '\n')
	var line []byte
	if lastIdx < 0 {
		line = trimmed
	} else {
		line = trimmed[lastIdx+1:]
	}
	var rec JournalRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return "", &JournalError{
			Op:     "append",
			Offset: -1,
			Cause:  fmt.Errorf("%w: cannot append after an unparsable tail line: %v", ErrIndeterminateJournal, err),
		}
	}
	return rec.Phase, nil
}

func journalPhaseKnown(phase JournalPhase) bool {
	_, ok := journalNext[phase]
	return ok
}

func journalTransitionAllowed(last, next JournalPhase) bool {
	for _, allowed := range journalNext[last] {
		if allowed == next {
			return true
		}
	}
	return false
}
