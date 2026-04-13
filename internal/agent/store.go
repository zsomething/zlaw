package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zsomething/zlaw/internal/llm"
)

// SessionMeta holds lightweight metadata about a session stored alongside the
// JSONL message log as <sessionID>.meta.json.
type SessionMeta struct {
	SessionID   string    `json:"session_id"`
	AgentName   string    `json:"agent_name"`
	Channel     string    `json:"channel"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	MessageCount int      `json:"message_count"`
	Title       string    `json:"title"`
	TotalInputTokens  int `json:"total_input_tokens"`
	TotalOutputTokens int `json:"total_output_tokens"`
}

// SessionStore persists per-session message history and metadata.
type SessionStore interface {
	// Append writes one message to the session's persistent log.
	Append(sessionID string, msg llm.Message) error
	// Load returns all messages previously appended for the session.
	// Returns nil, nil when the session has no history.
	Load(sessionID string) ([]llm.Message, error)
	// LoadMeta returns the metadata for the session.
	// Returns a zero-value SessionMeta (no error) when no metadata exists yet.
	LoadMeta(sessionID string) (SessionMeta, error)
	// UpdateMeta loads the current metadata (or zero value), calls fn to mutate
	// it, then persists the result. fn must not be nil.
	UpdateMeta(sessionID string, fn func(*SessionMeta)) error
	// Archive moves the session's JSONL and metadata files to an archived/
	// subdirectory, preserving history without polluting the active session
	// directory. A subsequent Load returns nil, nil (as if the session is new).
	// Missing files are silently ignored.
	Archive(sessionID string) error
}

// JSONLFileStore stores each session as a JSONL file under baseDir.
// File path: <baseDir>/<sessionID>.jsonl
// Each line is a JSON-encoded llm.Message.
type JSONLFileStore struct {
	baseDir string
}

// NewJSONLFileStore returns a store that writes to baseDir.
// The directory is created on first use.
func NewJSONLFileStore(baseDir string) *JSONLFileStore {
	return &JSONLFileStore{baseDir: baseDir}
}

func (s *JSONLFileStore) filePath(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID+".jsonl")
}

func (s *JSONLFileStore) metaFilePath(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID+".meta.json")
}

// Append encodes msg as JSON and appends it as a new line to the session file.
func (s *JSONLFileStore) Append(sessionID string, msg llm.Message) error {
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return fmt.Errorf("session store: mkdir %s: %w", s.baseDir, err)
	}

	f, err := os.OpenFile(s.filePath(sessionID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("session store: open %s: %w", sessionID, err)
	}
	defer f.Close()

	line, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("session store: marshal message: %w", err)
	}
	line = append(line, '\n')

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("session store: write %s: %w", sessionID, err)
	}
	return nil
}

// Load reads all lines from the session file and decodes each as an llm.Message.
// Returns nil, nil if the file does not exist.
func (s *JSONLFileStore) Load(sessionID string) ([]llm.Message, error) {
	f, err := os.Open(s.filePath(sessionID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session store: open %s: %w", sessionID, err)
	}
	defer f.Close()

	var msgs []llm.Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB per line
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg llm.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("session store: decode line %d of %s: %w", lineNum, sessionID, err)
		}
		msgs = append(msgs, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("session store: scan %s: %w", sessionID, err)
	}
	return msgs, nil
}

// LoadMeta reads the metadata file for the session.
// Returns a zero-value SessionMeta when the file does not exist.
func (s *JSONLFileStore) LoadMeta(sessionID string) (SessionMeta, error) {
	data, err := os.ReadFile(s.metaFilePath(sessionID))
	if os.IsNotExist(err) {
		return SessionMeta{}, nil
	}
	if err != nil {
		return SessionMeta{}, fmt.Errorf("session meta: read %s: %w", sessionID, err)
	}
	var m SessionMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return SessionMeta{}, fmt.Errorf("session meta: decode %s: %w", sessionID, err)
	}
	return m, nil
}

// Archive moves the session's JSONL and metadata files into an archived/
// subdirectory under baseDir. A timestamp suffix is appended to the filename
// so repeated clears of the same session ID do not overwrite each other.
// Subsequent Load calls return nil, nil (no active file), so the next Append
// starts a fresh log. Missing files are silently ignored.
func (s *JSONLFileStore) Archive(sessionID string) error {
	archiveDir := filepath.Join(s.baseDir, "archived")
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return fmt.Errorf("session store: mkdir archived: %w", err)
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	suffix := sessionID + "-" + ts
	for _, pair := range [][2]string{
		{s.filePath(sessionID), filepath.Join(archiveDir, suffix+".jsonl")},
		{s.metaFilePath(sessionID), filepath.Join(archiveDir, suffix+".meta.json")},
	} {
		src, dst := pair[0], pair[1]
		if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("session store: archive %s: %w", sessionID, err)
		}
	}
	return nil
}

// UpdateMeta loads metadata, applies fn, and writes the result back.
func (s *JSONLFileStore) UpdateMeta(sessionID string, fn func(*SessionMeta)) error {
	m, err := s.LoadMeta(sessionID)
	if err != nil {
		return err
	}
	fn(&m)
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return fmt.Errorf("session meta: mkdir %s: %w", s.baseDir, err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("session meta: encode %s: %w", sessionID, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.metaFilePath(sessionID), data, 0o600); err != nil {
		return fmt.Errorf("session meta: write %s: %w", sessionID, err)
	}
	return nil
}
