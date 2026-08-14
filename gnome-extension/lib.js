// Pure helpers for the vigie indicator: no GNOME imports, no state, no I/O.
// extension.js keeps everything that touches Shell; this file is what a test can
// exercise under node (#430).

// groupOrder takes the known order as an argument rather than reading a
// module-level constant: the list itself stays in extension.js, where a Go test
// checks it against the design document (#423), and this stays a pure function a
// node test can state its own inputs for (#430).
//
// groupOrder returns the statuses to render: the known ones in their documented
// order, then anything else the server actually sent. Keeping the list in step is
// what the Go test enforces, but a menu must not depend on being in step — a
// status added on the server side has to *appear*, unstyled and unlabelled if
// need be, rather than take its sessions off the screen (#422).
export function groupOrder(sessions, statusOrder) {
    const seen = new Set(sessions.map(s => s.status).filter(Boolean));
    const unknown = [...seen].filter(s => !statusOrder.includes(s)).sort();
    return [...statusOrder, ...unknown];
}

export function basename(path) {
    if (!path)
        return '';
    const parts = path.replace(/\/+$/, '').split('/');
    return parts[parts.length - 1] || path;
}
