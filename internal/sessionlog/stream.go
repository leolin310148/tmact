package sessionlog

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
)

// MaxRecordBytes bounds memory consumed by one JSONL record. Oversized records
// are counted and discarded while streaming continues at the next line.
const MaxRecordBytes = 16 * 1024 * 1024

// Stream opens source and visits each successfully decoded record in order.
func Stream(source Source, visit func(Record) error) (Stats, error) {
	file, err := os.Open(source.Path)
	if err != nil {
		return Stats{}, err
	}
	defer file.Close()
	return StreamReader(file, source, visit)
}

// StreamReader is Stream's reader-based form, useful to callers that already
// own a stream and to deterministic tests.
func StreamReader(input io.Reader, source Source, visit func(Record) error) (Stats, error) {
	if source.Provider != ProviderClaude && source.Provider != ProviderCodex {
		return Stats{}, fmt.Errorf("unsupported session-log provider %q", source.Provider)
	}
	if visit == nil {
		return Stats{}, fmt.Errorf("session-log visitor is required")
	}

	reader := bufio.NewReaderSize(input, 64*1024)
	state := parseState{
		provider:  source.Provider,
		sessionID: sessionIDFromPath(source),
	}
	var stats Stats
	for {
		line, oversized, err := readBoundedLine(reader)
		if len(line) > 0 || oversized {
			stats.Lines++
			if oversized {
				stats.Oversized++
			} else if len(bytes.TrimSpace(line)) > 0 {
				record, known, parseErr := normalize(line, &state)
				if parseErr != nil {
					stats.Malformed++
				} else {
					stats.Records++
					if !known {
						stats.Unknown++
					}
					if visitErr := visit(record); visitErr != nil {
						return stats, visitErr
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return stats, nil
			}
			return stats, err
		}
	}
}

func readBoundedLine(reader *bufio.Reader) ([]byte, bool, error) {
	line := make([]byte, 0, 64*1024)
	oversized := false
	for {
		part, prefix, err := reader.ReadLine()
		if !oversized {
			if len(line)+len(part) > MaxRecordBytes {
				line = nil
				oversized = true
			} else {
				line = append(line, part...)
			}
		}
		if err != nil {
			return line, oversized, err
		}
		if !prefix {
			return line, oversized, nil
		}
	}
}
