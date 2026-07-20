# mustclose

mustclose is a Go static analyzer that flags values implementing io.Closer whose Close method is never called.

## Install

```
go install github.com/renzoarr/mustclose@latest
```

## Use

To check all packages in current subdirectories

```
mustclose ./...
```

## How it works

The analyzer is built on SSA (via `golang.org/x/tools/go/analysis/passes/buildssa`).

> For every closer **created** in a function, that function must either **close**
> it or let its **ownership leave** the function. Ownership leaves when the closer
> is returned, or stored into something that outlives the call (a global, a
> receiver field, or an aggregate that is itself returned). Anything else — a
> closer that is created and then dropped, discarded in a local aggregate, or
> passed somewhere untraceable — is reported.

Two consequences worth stating up front:

- Closers you *receive* (parameters, struct fields) are never flagged — you didn't
  create them, so closing them is not your responsibility.
- A callee is never assumed to close what it receives, matching idiomatic Go
  (`io.Copy`, decoders, and friends do not close their arguments).

Diagnostics currently use a uniform message (`Close is not called`). An optional
overlay that names the offending value — the bound variable (`Close is not called
on f`) or, for a discarded call result, the callee (`Close is not called on the
result of Open`) — lives in `refine_messages.go` but is disabled by default, as it
reverse-maps SSA positions back to the AST (a heuristic). Enable it by uncommenting
the `refineMessages` call in `run`.

## How to run with golangci-lint
TODO

## Limitations

- **Path-insensitive close detection (by design).** A `Close` on any path is
  treated as closing the value. This is deliberate: requiring `Close` on every
  path would flag common guarded patterns (`if x != nil { x.Close() }`) and produce
  noisy false positives.

  ```go
  x, _ := os.Open("/tmp/foo")
  if cond {
      x.Close()
  }
  // analyzer sees x as closed
  ```

- **A callee is never credited with closing.** Because ownership only leaves the
  function through a return or a persistent store (see the rule above), passing a
  created closer to another function does **not** clear the warning:

  ```go
  f, _ := os.Open("/tmp/foo")
  process(f)                 // reported: process is not assumed to close f
  defer fmt.Println(f)       // reported: f enters a variadic slice
  s := []io.Closer{f}        // reported: f stored in local aggregate, not returned
  b := box{closer: f}; _ = b // reported: b is discarded, f with it
  return box{closer: f}      // not reported: ownership returned to caller
  ```

  The deliberate trade-off is a false positive when a callee genuinely takes
  ownership and closes the value (e.g.
  `func consume(rc io.ReadCloser) { defer rc.Close() }`).

