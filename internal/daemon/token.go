package daemon

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/haribo/claude-vigie/internal/store"
)

// lookupToken reports the token an operator could hand to a client: the one in
// this process's environment, else the one the daemon persisted. It never
// creates one — see runToken for why that matters.
func lookupToken(ctx context.Context, st *store.Store) (string, bool, error) {
	if env := os.Getenv(tokenEnv); env != "" {
		return env, true, nil
	}
	v, ok, err := st.GetMeta(ctx, "token")
	if err != nil {
		return "", false, err
	}
	return v, ok && v != "", nil
}

// runToken prints the fleet auth token, so an operator can connect a client.
//
// It reads. It used to share `resolveToken` with `serve`, which *mints* a token
// when it finds none — correct for a daemon starting for the first time, wrong
// for a question. A daemon given its token through the environment persists
// nothing, so this command, run from an operator shell that does not carry the
// variable, found an empty store, generated a fresh secret, wrote it to the
// database and printed it. The operator got a token no running server had ever
// heard of, handed it to a machine, and the machine was refused — with nothing
// to suggest the answer was wrong rather than the setup (#657).
//
// So: no token to print is an answer, not a blank to fill. It names the two
// places one comes from, because when the daemon holds it in its environment
// there is nothing here to read and saying so is the whole help there is.
func runToken(args []string) int {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	dbPath := fs.String("db", "vigie.db", "path to the SQLite database file")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Opening creates the file. For a command that only reads, a mistyped path
	// would leave an empty database behind and then report it holds no token —
	// true, and about a database it had just invented.
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "token: %v\n", err)
		return 1
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "token: %v\n", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	token, ok, err := lookupToken(context.Background(), st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "token: %v\n", err)
		return 1
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "token: no token in %s.\n"+
			"The daemon writes one there when it generates its own on first start.\n"+
			"If it was given one through $%s instead, the token is only in the daemon's\n"+
			"environment and this command cannot read it.\n", *dbPath, tokenEnv)
		return 1
	}
	fmt.Println(token)
	return 0
}
