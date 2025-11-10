# commandrunner

This package provides a way to run CLI commands, with support for modifying the
commands throughout their lifecycle.

Example usage:

```go
logger := slog.New(slog.NewTextHandler(io.Discard, nil))
hooks := []commandrunner.Hook{
    YourReplaceEchoWithPrintfHook(),
}

runner := exec.NewRunner(exec.WithLogger(logger), exec.WithHooks(hooks...))
for i := range 10 {
    // Will run `printf <i>`
    runner.Run(context.TODO(), "echo", strconv.Itoa(i))
}
```
