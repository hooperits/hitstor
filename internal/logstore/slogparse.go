package logstore

import (
	"regexp"
	"strings"
	"time"
)

var (
	slogTextLineRe         = regexp.MustCompile(`^time=(\S+)\s+level=(\S+)\s+msg="((?:\\.|[^"\\])*)"(?:\s+(.+))?$`)
	slogTextLineUnquotedRe = regexp.MustCompile(`^time=(\S+)\s+level=(\S+)\s+msg=(\S+)(?:\s+(.+))?$`)
)

// ParseSlogTextLine extracts time, level, and message from journal/slog text output.
func ParseSlogTextLine(line string) (Entry, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Entry{}, false
	}
	if m := slogTextLineRe.FindStringSubmatch(line); len(m) >= 4 {
		return entryFromSlogParts(m[1], m[2], unescapeSlogMsg(m[3])), true
	}
	if m := slogTextLineUnquotedRe.FindStringSubmatch(line); len(m) >= 4 {
		return entryFromSlogParts(m[1], m[2], m[3]), true
	}
	return Entry{}, false
}

func entryFromSlogParts(timeStr, level, msg string) Entry {
	ts, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, timeStr)
	}
	if err != nil {
		ts = time.Now().UTC()
	}
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		level = "info"
	}
	return Entry{
		Time:    ts.UTC(),
		Level:   level,
		Message: strings.TrimSpace(msg),
	}
}

func unescapeSlogMsg(s string) string {
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}
