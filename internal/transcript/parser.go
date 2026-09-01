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
	agents  *pendingAgents
	offset  int64
}

// NewParser returns a Parser positioned at the start of a transcript.
func NewParser() *Parser {
	return &Parser{seen: map[string]bool{}, pending: newPendingTools(), agents: newPendingAgents()}
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
	case "assistant", "user":
		p.info.HasTurns = true // an exchange, as opposed to a metadata sidecar (#448)
	}
	switch l.Type {
	case "assistant":
		p.info.applyAssistant(l, p.seen)
		p.pending.addToolUses(l.Message.Content)
		p.agents.addLaunches(l.Message.Content)
		p.info.Interrupted = false // a real turn resumed (#351)
	case "user":
		// A user line carrying no tool_result is a new prompt, and it closes the
		// turn any older tool call belonged to (#483) — unless Claude Code wrote
		// it itself. It marks those isMeta (system reminders, skill preambles, the
		// "Continue from where you left off." resume), and they land in the middle
		// of live tool calls, so closing on them would break stalled detection.
		answered := p.pending.clearToolResults(l.Message.Content)
		notified := p.agents.clearNotifications(l.Message.Content) // <task-notification> closes an agent (#344)
		// A real prompt closes the turn every older tool call *and* subagent
		// belonged to (#483, #662). A line carrying a notification is not one,
		// however much it looks like plain text: it would retire the siblings of
		// the agent it announces.
		if !answered && !notified && !l.IsMeta {
			p.pending.closeTurn()
			p.agents.closeTurn()
		}
		p.info.Interrupted = isInterruptLine(l.Message.Content) // synthetic interrupt marker, else a real prompt clears it (#351)
	case "system":
		if l.Subtype == "compact_boundary" && l.Timestamp != "" {
			p.info.LastCompactBoundary = l.Timestamp // a compaction finished (#342)
		}
	}
}

// Info returns a snapshot of the cumulative state, finalizing the title and the
// pending-tool resolution. It does not mutate the parser, so it may be called
// after every Advance.
func (p *Parser) Info() *Info {
	info := p.info
	info.Title = p.titles.resolve()
	info.PendingTool, info.BackgroundActive = p.pending.resolve()
	info.AgentsActive, info.AgentActivity = p.agents.resolve()
	return &info
}
