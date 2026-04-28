# Air Hot Reload Not Picking Up File Changes

## Symptoms

- `task dev_server` starts successfully but code changes in `internal/classifier/` and `internal/analysis/` are not reflected in the running binary
- Git version hash (e.g., `360c76e6`) remains unchanged across restarts
- New log lines added to Go source files do not appear in output
- `building...` may or may not appear in air output

## Environment

- **OS**: Linux (Ubuntu)
- **Build tool**: `air` (Cosmtrek/air) for hot reload
- **Config**: `.air.toml` in project root
- **Command**: `task dev_server` (runs `air realtime`)

## Root Cause

Under investigation. Possible causes:

1. **Air not detecting file changes**: The `.air.toml` config watches `include_dir = ["internal", "assets", "cmd", "pkg"]` with `include_ext = ["go"]`, which should cover our modified files. However, air may not detect changes made while a previous build is in progress, or may have stale file watches.

2. **Silent build failure**: The air build command uses CGO with TensorFlow Lite headers (`CGO_CFLAGS="-I${HOME}/src/tensorflow"`). If the build fails, air logs to `build-errors.log` and may stop restarting (`stop_on_error = true`).

3. **Binary caching**: Air compiles to `./tmp/main`. If the build fails silently, the old binary continues running.

4. **Version hash is commit-based**: The version comes from `git describe --tags --always`, so it stays the same between rebuilds when there are no new commits. This makes it unreliable for detecting whether a rebuild actually occurred.

## Reproduction Steps

1. Add a log line to `internal/analysis/birdnet_service.go` in the `Start()` method
2. Run `task dev_server`
3. Observe that the new log line does not appear in the output
4. Note the version hash stays the same

## Workaround

Stop air completely (`Ctrl+C`) and restart:

```bash
# Kill any lingering air processes
pkill -f air

# Clean and rebuild
rm -rf tmp/
task dev_server
```

If that doesn't work, build manually and run directly:

```bash
task
./birdnet-go realtime
```

## Diagnostic Steps

1. Check if air detected the file change — look for `building...` in the air output
2. Check `build-errors.log` for compilation errors:
   ```bash
   cat build-errors.log
   ```
3. Verify the binary was rebuilt by checking its timestamp:
   ```bash
   ls -la tmp/main
   ```
4. Run the build command from `.air.toml` manually to see errors:
   ```bash
   export CGO_ENABLED=1
   export CGO_CFLAGS="-I${HOME}/src/tensorflow"
   export CGO_LDFLAGS="-L/usr/lib -ltensorflowlite_c"
   go build -o ./tmp/main .
   ```

## Resolution

TBD.
