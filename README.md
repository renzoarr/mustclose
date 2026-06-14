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

## Limitations 
Some of these will be fixed

1. empty variable declarations will be flaged

```go
var i io.Closer  // will return a warning
```


2. Reassingments are not retracked

```go
a, _ := os.Open("/tmp/foo") // tracked
a, _ = os.Open("/tmp/foo")  // reassignment to a fresh closer — *not* re-tracked
a.Close()                   // closes only the second one, analyzer won't notice if the user
                            // forgot to close the first value before reassigning.
```

3. path-insensitive close detection (no plan to fix)
```go 
x, _ := os.Open("/tmp/foo")
if cond { 
    x.Close()
}
// analyzer will always see x as closed
```

4. missing warning on type switch with io.Closer case

```go
var v any = example2{} // example2 implements io.Closer
switch a := v.(type) {
case example2:
    fmt.Println(a) // we don't return a warning here. We do return a warning if a is assigned to a new variable `b := a`
default:
}
```