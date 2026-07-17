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
Working on the SSA form lets it follow values across assignments, aliases and
interface conversions, which resolves most of the earlier AST-based limitations.

Everything below follows from a single rule:

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

## Limitations

### Resolved

1. **Zero-value declarations are ignored.** A nil closer has no resource to close,
   so declarations like `var i io.Closer` are no longer flagged.

2. **Reassignments are tracked.** Each assignment produces a distinct SSA value, so
   a closer that is overwritten before being closed is still reported.

3. **Aliases are followed.** Closing through an alias clears the warning:

   ```go
   a, _ := os.Open("/tmp/foo")
   b := a
   b.Close() // a is considered closed
   ```

4. **Type-switch and comma-ok bindings are tracked.** A closer bound in a
   `switch x := v.(type)` case or via `c, ok := x.(T)` is analyzed like any other
   value.

5. **Closers that escape in a struct are not flagged.** A direct application of the
   ownership rule: returning a struct that holds the closer transfers ownership to
   the caller. Storing it into a struct that is instead *discarded* is reported.

   ```go
   func newThing() thing {
       a := &closerImpl{}
       return thing{field: a} // ok, escapes to the caller
   }
   ```

6. **Method values count as closing.** `closeFn := a.Close; closeFn()` clears the
   warning.

### Remaining limitations

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
  fmt.Println(c)             // reported: c enters a variadic slice
  s := []io.Closer{c}        // reported
  b := box{c: c}; _ = b      // reported: b is discarded
  return box{c: c}           // not reported: ownership returned to caller
  ```

  The deliberate trade-off is a false positive when a callee genuinely takes
  ownership and closes the value (e.g.
  `func consume(rc io.ReadCloser) { defer rc.Close() }`).

