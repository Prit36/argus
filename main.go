// Argus is an independent Windows process monitor in Go on mygo: processes grouped as
// apps, background and Windows processes; the performance of the CPU,
// memory, disks, network and GPUs over the last minute; app history,
// startup apps, users, every process's details, and services.
//
// One window of native UI, drawn by mygo. Sampling runs on its
// own timer (once a second, Task Manager's "Normal"), and each sample is a
// handful of system calls: one for every process, one for every processor,
// one per disk and adapter. Nothing else runs between samples; frames are
// drawn only when something changed.
package main

import (
	"log"

	"taskmgr/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
