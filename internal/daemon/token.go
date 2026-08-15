package daemon

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/haribo/claude-vigie/internal/store"
)

// runToken prints the fleet auth token stored in the database, generating and
// persisting one if none exists yet. Used by tooling to connect clients.
func runToken(args []string) int {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	dbPath := fs.String("db", "vigie.db", "path to the SQLite database file")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "token: %v\n", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	token, err := resolveToken(context.Background(), st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "token: %v\n", err)
		return 1
	}
	fmt.Println(token)
	return 0
}
