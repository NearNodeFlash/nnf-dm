#!/bin/bash
# Verify that inline code examples in man pages match the corresponding
# .c files in daemons/lib-copy-offload/examples/.
#
# Each man page tags its example with a groff comment:
#   .\" example-file: example_foo.c
# immediately before the .nf block. This script extracts those blocks
# and diffs them against the .c files.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MAN_DIR="$REPO_ROOT/man/man3"
EXAMPLES_DIR="$REPO_ROOT/daemons/lib-copy-offload/examples"

TMPDIR=$(mktemp -d)
# shellcheck disable=SC2064
trap "rm -rf $TMPDIR" EXIT

ret=0

# Extract tagged example blocks from all man pages.
for manpage in "$MAN_DIR"/*.3; do
    awk '
    /^\.\\\"[[:space:]]+example-file:/ {
        filename = $NF
        getline  # consume the .nf line
        outfile = tmpdir "/" filename
        while (getline > 0) {
            if ($0 == ".fi") break
            print >> outfile
        }
        close(outfile)
    }
    ' tmpdir="$TMPDIR" "$manpage"
done

# Compare each extracted example with its .c file.
for extracted in "$TMPDIR"/*.c; do
    [ -f "$extracted" ] || continue
    filename=$(basename "$extracted")

    if [ ! -f "$EXAMPLES_DIR/$filename" ]; then
        echo "FAIL: $filename referenced in man page but not found in examples/"
        ret=1
        continue
    fi

    if diff -q "$extracted" "$EXAMPLES_DIR/$filename" > /dev/null 2>&1; then
        echo "OK: $filename"
    else
        echo "MISMATCH: $filename"
        diff -u "$EXAMPLES_DIR/$filename" "$extracted" || true
        ret=1
    fi
done

# Warn about .c files in examples/ not referenced by any man page.
for cfile in "$EXAMPLES_DIR"/*.c; do
    [ -f "$cfile" ] || continue
    filename=$(basename "$cfile")
    if [ ! -f "$TMPDIR/$filename" ]; then
        echo "WARN: $filename in examples/ not referenced by any man page"
        ret=1
    fi
done

if [ $ret -eq 0 ]; then
    echo "All man page examples match their .c files."
fi

exit $ret
