package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Parser folds a transcript's JSONL lines into cumulative Info incrementally.
// Transcripts are append-only, so the watcher can keep one Parser per file and
// feed it only the newly-appended bytes (Advance from the last Offset), turning an
// O(file) re-parse into O(delta) — issue #257. The result is byte-for-byte
// identical to a full Parse of the same input.
type Parser struct {
	info    Info
	titles  titleTracker
	seen    map[string]bool
	pending *pendingTools
	offset  int64
}

// NewParser returns a Parser positioned at the start of a transcript.
func NewParser() *Parser {
	return &Parser{seen: map[string]bool{}, pending: newPendingTools()}
}

// Offset is the number of bytes consumed so far — the position the reader passed
// to the next Advance must start at.
func (p *Parser) Offset() int64 { return p.offset }

// Advance folds every complete (newline-terminated) line available from r into the
// cumulative state, advancing Offset by the bytes consumed. A trailing partial
// line (a record still being written) is left unconsumed, so it is re-read once
// complete on the next Advance.
func (p *Parser) Advance(r io.Reader) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		raw, err := br.ReadBytes('\n')
		if err == nil { // a full line (terminated by '\n')
			p.offset += int64(len(raw))
			p.foldLine(raw)
			continue
		}
		if err == io.EOF {
			return nil // any bytes in `raw` are a partial line: leave them unconsumed
		}
		return fmt.Errorf("reading transcript: %w", err)
	}
}

func (p *Parser) foldLine(raw []byte) {
	var l line
	if json.Unmarshal(raw, &l) != nil {
		return // skip a malformed line rather than fail the whole parse
	}
	p.info.applyMeta(l, &p.titles)
	switch l.Type {
	case "assistant":
		p.info.applyAssistant(l, p.seen)
		p.pending.addToolUses(l.Message.Content)
	case "user":
		p.pending.clearToolResults(l.Message.Content)
	}
}

// Info returns a snapshot of the cumulative state, finalizing the title and the
// pending-tool resolution. It does not mutate the parser, so it may be called
// after every Advance.
func (p *Parser) Info() *Info {
	info := p.info
	info.Title = p.titles.resolve()
	info.PendingTool, info.BackgroundActive = p.pending.resolve()
	return &info
}
