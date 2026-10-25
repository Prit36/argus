# Argus

A native Windows 11 Task Manager in Go, drawn with the
[mygo](https://github.com/egoist/mygo) native UI: Processes (grouped
apps / background / Windows processes, heat-colored cells, column
chooser), Performance (CPU, memory, disks, network adapters and GPUs with
60-second graphs), App history, Startup apps, Users, Details, Services
and Settings, plus the tray CPU meter.

## Quality of life

- Clicking a Processes column sorts across Apps, Background and Windows
  processes. Use View → Group by type to show category sections again.
  Processes, Details, Services, Startup apps and App history remember their
  sort column and direction between launches.
- Search filters the current data page, including Startup apps, App history
  and Users. Each page remembers its own search during the session. Escape
  or the clear button removes the filter.
- The title-bar pause button and Ctrl+P pause/resume updates at the previous
  speed. F5 refreshes once while paused and reloads the current page's
  services, startup entries, sessions or adapter addresses as applicable.
- Ctrl+C or Copy row copies the selected process, service or startup entry
  as tab-separated text. On Performance, Ctrl+C copies the machine summary.
- Switching directly to a process's details or services clears the target
  page's filter so the selection is visible.

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
release build, steady state:

| | |
|---|---:|
| CPU (one core) | ~0.5% |
| Working set | ~44 MB |
| Private bytes | ~68 MB |
| Threads | ~21 |

- A sample (all processes, rates, groups) takes ~1 ms and allocates
  nothing (`go test ./internal/sys -bench Sample -benchmem`).
- Frames take ~1–2 ms and redraw only what changed.
- The UI draws in software by default: the GPU's driver would add ~45 MB
  and two dozen threads for frames this small. `MYGO_GPU=1` draws on the
  GPU instead.
- `GOGC=50` keeps the small, long-lived heap (~8 MB live) close to what's
  in use.
- Collapsed process groups defer sorting their children until expanded.
- Startup, history and user row ordering is cached between data/filter
  changes instead of being rebuilt for mouse movement and other redraws.
- While minimized or hidden, sampling slows to at most once every four
  seconds. Restoring the window restores the chosen refresh interval;
  paused updates stay paused. GPU counters are disabled while hidden.

## Files

Settings (`settings.ini`) and app history (`history.tsv`) are kept in
`%LOCALAPPDATA%\argus`. mygo keeps the window's place in `%APPDATA%\Task Manager`.

## Testing hooks

| Variable | Effect |
|---|---|
| `TASKMAN_NO_ACTIVATE=1` | Show the window without taking the focus |
| `TASKMAN_PAGE=n` | Open on page `n` (its number in settings.ini) |
| `TASKMAN_CPUPROFILE=file` | Write a CPU profile of the run |
| `TASKMAN_MEMPROFILE=file` | Write a heap profile as the window closes |
| `TASKMAN_SNAPSHOTS=dir` | `go test ./internal/app` saves each tested frame there |
