package daemon

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/haribo/claude-vigie/internal/store"
)

// `stats_daily` is never pruned and never recomputed — a day cannot be rebuilt,
// because the session rows and events it came from are long gone. So a figure
// corrupted before the rollup was made safe (#432) can only be corrected by an
// operator who decides what the right number is. That is what this command is:
// deliberate surgery on one row, printing what it replaced.
//
// There is deliberately no automatic repair. A large day is not, by itself,
// wrong, and a tool that quietly rewrote history would be a worse defect than the
// one it fixes. See docs/design/token-rollup.md.

var dayPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func runStatsRepair(args []string) int {
	fs := flag.NewFlagSet("stats-repair", flag.ContinueOnError)
	dbPath := fs.String("db", "vigie.db", "path to the SQLite database file")
	day := fs.String("day", "", "UTC day to correct, YYYY-MM-DD")
	model := fs.String("model", "", "model bucket to correct (empty is a real bucket: reports with no model)")
	tokens := fs.Int64("output-tokens", -1, "value to write into that bucket")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: vigied stats-repair -day YYYY-MM-DD -model NAME -output-tokens N [-db PATH]\n\n"+
			"Replaces one (day, model) bucket's output-token figure and prints what it held\n"+
			"before. Daily stats are never recomputed, so a value corrupted by an earlier\n"+
			"defect can only be corrected deliberately — decide the right figure yourself.\n")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	switch {
	case !dayPattern.MatchString(*day):
		fmt.Fprintln(os.Stderr, "stats-repair: -day must be a UTC day, YYYY-MM-DD")
		return 2
	case *tokens < 0:
		fmt.Fprintln(os.Stderr, "stats-repair: -output-tokens must be zero or more")
		return 2
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stats-repair: opening %s: %v\n", *dbPath, err)
		return 1
	}
	defer func() { _ = st.Close() }()

	before, existed, err := st.SetDailyTokens(context.Background(), *day, *model, *tokens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stats-repair: %v\n", err)
		return 1
	}
	if !existed {
		fmt.Printf("%s / %q had no row; created it with output_tokens = %d\n", *day, *model, *tokens)
		return 0
	}
	fmt.Printf("%s / %q output_tokens: %d -> %d\n", *day, *model, before, *tokens)
	return 0
}
