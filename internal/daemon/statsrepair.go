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
	del := fs.Bool("delete", false, "remove the bucket entirely, figures and all")
	into := fs.String("into", "", "fold the bucket into this model's bucket on the same day")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: vigied stats-repair -day YYYY-MM-DD -model NAME <one of>\n"+
			"         -output-tokens N   replace that bucket's output-token figure\n"+
			"         -into MODEL        fold the bucket into MODEL's bucket, same day\n"+
			"         -delete            remove the bucket entirely\n\n"+
			"Daily stats are never recomputed, so anything corrupted by an earlier defect can\n"+
			"only be corrected deliberately — you decide what is right, and each operation\n"+
			"prints what it replaced.\n\n"+
			"-into is the one to reach for when the bucket is not a model at all, such as the\n"+
			"`<synthetic>` marker Claude Code writes on lines it generated itself: the tokens\n"+
			"in it are real output and belong to whichever model produced them. -delete throws\n"+
			"them away, which is right only when there is nothing to return.\n")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Exactly one operation. Two at once would be an operator who meant one of
	// them, on a table where the wrong write cannot be undone.
	chosen := 0
	for _, on := range []bool{*tokens >= 0, *del, *into != ""} {
		if on {
			chosen++
		}
	}
	switch {
	case !dayPattern.MatchString(*day):
		fmt.Fprintln(os.Stderr, "stats-repair: -day must be a UTC day, YYYY-MM-DD")
		return 2
	case chosen == 0:
		fmt.Fprintln(os.Stderr, "stats-repair: give one of -output-tokens, -into or -delete")
		return 2
	case chosen > 1:
		fmt.Fprintln(os.Stderr, "stats-repair: -output-tokens, -into and -delete are alternatives; give one")
		return 2
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stats-repair: opening %s: %v\n", *dbPath, err)
		return 1
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	switch {
	case *del:
		return reportRepair(st.DeleteDailyBucket(ctx, *day, *model))(
			fmt.Sprintf("%s / %q had no row; nothing to delete\n", *day, *model),
			func(b store.DailyStat) string {
				return fmt.Sprintf("%s / %q deleted: %d tokens, %ds working, %ds waiting, %ds idle\n",
					*day, *model, b.OutputTokens, b.WorkingSeconds, b.WaitingSeconds, b.IdleSeconds)
			})
	case *into != "":
		return reportRepair(st.MoveDailyBucket(ctx, *day, *model, *into))(
			fmt.Sprintf("%s / %q had no row; nothing to move\n", *day, *model),
			func(b store.DailyStat) string {
				return fmt.Sprintf("%s / %q folded into %q: %d tokens, %ds working, %ds waiting, %ds idle\n",
					*day, *model, *into, b.OutputTokens, b.WorkingSeconds, b.WaitingSeconds, b.IdleSeconds)
			})
	}

	before, existed, err := st.SetDailyTokens(ctx, *day, *model, *tokens)
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

// reportRepair turns one of the bucket operations into an exit code, printing what
// it replaced — the property the whole command rests on, since nothing here can be
// recomputed afterwards to check it.
func reportRepair(before store.DailyStat, existed bool, err error) func(absent string, done func(store.DailyStat) string) int {
	return func(absent string, done func(store.DailyStat) string) int {
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "stats-repair: %v\n", err)
			return 1
		case !existed:
			fmt.Print(absent)
			return 0
		}
		fmt.Print(done(before))
		return 0
	}
}
