package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chickenzord/zlaw/internal/llm"
)

// SessionStore persists per-session message history.
type SessionStore interface {
	// Append writes one message to the session's persistent log.
	Append(sessionID string, msg llm.Message) error
	// Load returns all messages previously appended for the session.
	// Returns nil, nil when the session has no history.
	Load(sessionID string) ([]llm.Message, error)
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
