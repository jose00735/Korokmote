# Working instructions for this repo

Korokmote is an open-source PC remote: a Go daemon for Windows (`daemon/`) and a Kotlin/Compose Android app (`android/`), talking over a documented protocol. Read `docs/PLAN.md` for the current milestone before starting work.

## Ground rules

- The docs in `docs/` are the specification. If code needs to differ, change the doc in the same commit and say why.
- Protocol changes start in `docs/PROTOCOL.md`, then `protocol/schema` and `protocol/vectors`, then Go, then Kotlin.
- Follow milestones in order. Do not polish animations before the milestone's functionality works end to end.
- Keep dependencies few. Android dependencies must be open source (no Play Services, ML Kit, Firebase).
- Never weaken a rule in `docs/SECURITY.md` to make something work; raise it instead.

## Daemon (Go)

- No cgo. Win32 access through `golang.org/x/sys/windows`.
- OS-specific code only under `internal/platform/windows` with `//go:build windows`; everything else must compile and test on Linux.
- All `SendInput` calls go through the single input worker.
- Run before committing: `gofmt -l .`, `go vet ./...`, `go test ./...`, and `GOOS=windows GOARCH=amd64 go build ./...`.
- Errors are wrapped with context (`fmt.Errorf("…: %w", err)`); logging uses `log/slog`; no secrets or typed text in logs.

## Android (Kotlin)

- Feature modules depend only on `core:*`.
- Colours, type, shapes and durations come from `core:design` tokens; no literals in feature code.
- Animations run in the draw phase and must not cause recomposition of content or delay input (see `docs/DESIGN.md`).
- Every animation honours the intensity setting and the system "remove animations" setting.
- Run before committing: `./gradlew ktlintCheck test`.

## Commits

- Small, one concern each. Prefix with the area: `daemon:`, `android:`, `protocol:`, `remotes:`, `docs:`, `ci:`.
- Add or update a test vector whenever a message is added or changed.
