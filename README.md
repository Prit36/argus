# Task Manager (Go + mygo)

A native Windows 11 Task Manager, ported from the Zig/nanoui version in
`../tt` to Go with the [mygo](https://github.com/egoist/mygo) native UI.
The pages, layout and behavior match the Zig app: Processes (grouped
apps / background / Windows processes, heat-colored cells, column
chooser), Performance (CPU, memory, disks, network adapters and GPUs with
60-second graphs), App history, Startup apps, Users, Details, Services
and Settings, plus the tray CPU meter.

## Build

```powershell
scripts\build.ps1          # build\windows-amd64\Task Manager.exe + installer
scripts\build.ps1 -Dev     # build\dev\taskmgr.exe, with a console for logs
scripts\build.ps1 -Test    # go vet + go test
```

Go 1.27 is needed (`go.mod`). `mygo build` embeds the icon, a
per-monitor-DPI manifest and version information, so the app lists itself
as "Task Manager".

## Layout

| Package | What it holds |
|---|---|
| `internal/winapi` | The Win32/NT calls, made with `syscall.SyscallN` so steady sampling allocates nothing |
| `internal/sys` | Reading the machine: processes (one `NtQuerySystemInformation` per tick), CPU, memory, disks, network, GPU (PDH, on a worker), services, startup items, sessions, ETW per-process network (as administrator), app history, icons |
| `internal/app` | State, sampling timer, commands and the mygo views |

Every field of `App` belongs to the UI thread. Blocking work (version
resources and account names on a pool of 4 resolvers, icons on a COM STA
thread, services, sessions, startup items, PDH) runs on goroutines that
hand their results back through `Window.Update`.

## Performance

Measured on an i5-12400F / RTX 5060, 1080×720 window, Processes page,
release builds, steady state:

| | Go (this) | Zig (`../tt`) |
|---|---:|---:|
| CPU (one core) | ~0.5% | ~0.3% |
| Working set | ~44 MB | ~61 MB |
| Private bytes | ~68 MB | ~70 MB |
| Threads | ~21 | ~19 |

- A sample (all processes, rates, groups) takes ~1 ms and allocates
  nothing (`go test ./internal/sys -bench Sample -benchmem`).
- Frames take ~1–2 ms and redraw only what changed.
- The UI draws in software by default: the GPU's driver would add ~45 MB
  and two dozen threads for frames this small. `MYGO_GPU=1` draws on the
  GPU instead.
- `GOGC=50` keeps the small, long-lived heap (~8 MB live) close to what's
  in use.

## Files

Settings (`settings.ini`) and app history (`history.tsv`) are kept in
`%LOCALAPPDATA%\nanotask`, in the same format as the Zig app, which shares
them. mygo keeps the window's place in `%APPDATA%\Task Manager`.

## Testing hooks

| Variable | Effect |
|---|---|
| `TASKMAN_NO_ACTIVATE=1` | Show the window without taking the focus |
| `TASKMAN_PAGE=n` | Open on page `n` (its number in settings.ini) |
| `TASKMAN_CPUPROFILE=file` | Write a CPU profile of the run |
| `TASKMAN_MEMPROFILE=file` | Write a heap profile as the window closes |
| `TASKMAN_SNAPSHOTS=dir` | `go test ./internal/app` saves each tested frame there |
