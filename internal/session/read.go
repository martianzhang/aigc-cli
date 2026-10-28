package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// read parses the file line by line and rebuilds the projection state. A
// malformed final line is tolerated (a crash can leave a partial append);
// malformed records in the middle are an error.
func (s *Session) read() error {
	f, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("read session %s: %w", s.path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	lineNo := 0
	sawHeader := false
	var malformed error

	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var probe struct {
			Type string `json:"type"`
			Seq  int64  `json:"seq"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			if malformed == nil {
				malformed = fmt.Errorf("line %d: %w", lineNo, err)
			}
			continue
		}
		if malformed != nil {
			return fmt.Errorf("session %s: malformed line before line %d: %w", s.path, lineNo, malformed)
		}
		if !sawHeader && probe.Type != typeHeader {
			return fmt.Errorf("session %s: first record is not a header", s.path)
		}
		if probe.Seq > s.seq {
			s.seq = probe.Seq
		}

		switch probe.Type {
		case typeHeader:
			if sawHeader {
				return fmt.Errorf("session %s: duplicate header on line %d", s.path, lineNo)
			}
			if err := json.Unmarshal(line, &s.header); err != nil {
				return fmt.Errorf("session %s: parse header: %w", s.path, err)
			}
			if s.header.Schema != Schema {
				return fmt.Errorf("session %s: unsupported schema %q", s.path, s.header.Schema)
			}
			if s.header.ID == "" {
				return fmt.Errorf("session %s: header has no id", s.path)
			}
			sawHeader = true

		case typeSystem:
			var rec SystemRecord
			if err := json.Unmarshal(line, &rec); err != nil {
				return fmt.Errorf("session %s: parse system record: %w", s.path, err)
			}
			s.trackSeq(rec.Seq)
			s.system = rec.Content

		case typeMessage:
			var rec MessageRecord
			if err := json.Unmarshal(line, &rec); err != nil {
				return fmt.Errorf("session %s: parse message record: %w", s.path, err)
			}
			s.trackSeq(rec.Seq)
			s.messages = append(s.messages, seqMessage{seq: rec.Seq, msg: rec.toChatMessage()})

		case typeCompaction:
			var rec CompactionRecord
			if err := json.Unmarshal(line, &rec); err != nil {
				return fmt.Errorf("session %s: parse compaction record: %w", s.path, err)
			}
			s.trackSeq(rec.Seq)
			copy := rec
			s.lastCompaction = &copy
			s.dropMessagesUpTo(rec.UpToSeq)

		case typeSessionInfo:
			var rec SessionInfoRecord
			if err := json.Unmarshal(line, &rec); err != nil {
				return fmt.Errorf("session %s: parse session_info record: %w", s.path, err)
			}
			s.trackSeq(rec.Seq)
			s.name = rec.Name

		case typeClear:
			var rec ClearRecord
			if err := json.Unmarshal(line, &rec); err != nil {
				return fmt.Errorf("session %s: parse clear record: %w", s.path, err)
			}
			s.trackSeq(rec.Seq)
			copy := rec
			s.lastClear = &copy
			s.dropMessagesUpTo(rec.UpToSeq)

		case typeEnd:
			var rec EndRecord
			if err := json.Unmarshal(line, &rec); err != nil {
				return fmt.Errorf("session %s: parse end record: %w", s.path, err)
			}
			s.trackSeq(rec.Seq)

		default:
			// Unknown record types are ignored so older readers can open
			// files written by newer versions that add metadata records.
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read session %s: %w", s.path, err)
	}
	if !sawHeader {
		return fmt.Errorf("session %s: missing header", s.path)
	}

	// Legacy fallback: if no system record exists but the conversation starts
	// with a system message, treat that message as the system prompt.
	if s.system == "" && len(s.messages) > 0 && s.messages[0].msg.Role == "system" {
		s.system = s.messages[0].msg.Content
		s.messages = s.messages[1:]
	}
	return nil
}

func (s *Session) trackSeq(seq int64) {
	if seq > s.seq {
		s.seq = seq
	}
}
