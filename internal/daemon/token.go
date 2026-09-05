package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/haribo/claude-vigie/internal/store"
)

// tokenFingerprintKey holds a hash of the token the daemon last started with.
//
// It exists so `token` can tell "the stored token is the live one" from "the
// stored token is a leftover". A daemon given its token through the environment
// persists nothing — deliberately — so a token generated on an earlier run stays
// in the store, and this command would print it as though it were in use. The
// operator hands it over and the machine is refused, by a daemon holding a secret
// this command never saw (#720).
//
// A hash rather than the token: the point is to compare, and writing the secret
// would undo the reason it was kept out of the file in the first place. It is
// rewritten on every start, so a daemon later restarted without the variable
// corrects it rather than leaving a stale warning behind.
const tokenFingerprintKey = "token_fingerprint"

// fingerprint identifies a token without carrying it.
func fingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:16]
}

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
	if !ok || v == "" {
		return "", false, nil
	}
	// A fingerprint that does not match the stored token means the daemon started
	// with a different one — supplied through its environment, and not written
	// anywhere this command can read. Printing the stored value would answer
	// confidently and wrongly.
	//
	// No fingerprint at all is not a mismatch: a daemon that has never run has
	// left none, and the stored token is then the only answer there is.
	fp, hasFP, err := st.GetMeta(ctx, tokenFingerprintKey)
	if err != nil {
		return "", false, err
	}
	if hasFP && fp != "" && fp != fingerprint(v) {
		return "", false, nil
	}
	return v, true, nil
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
		fmt.Fprintf(os.Stderr, "token: no token to print from %s.\n"+
			"The daemon writes one there when it generates its own on first start.\n"+
			"If it was given one through $%s instead, the token is only in the daemon's\n"+
			"environment and this command cannot read it — a value left in the database\n"+
			"by an earlier run is not the one in use, so it is not printed.\n", *dbPath, tokenEnv)
		return 1
	}
	fmt.Println(token)
	return 0
}
