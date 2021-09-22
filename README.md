[![](https://godoc.org/github.com/robarchibald/gobounce?status.svg)](https://pkg.go.dev/github.com/robarchibald/gobounce)
[![Build Status](https://travis-ci.com/robarchibald/gobounce.svg)](https://travis-ci.com/robarchibald/gobounce)
# gobounce
A cross-platform Go file watcher and event debouncer utilizing github.com/radovskyb/watcher

## Example Usage

```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/robarchibald/gobounce"
)

func main() {
	// Create a debounced file watcher that will:
	// 1. Poll for updates every 100 milliseconds
	// 2. Debounce updates to only notify after no changes have occurred for 200 milliseconds
	// 3. Notify on FileChanged and FolderChanged channels when files and folders are ready for use
	options := gobounce.Options{RootFolders: []string{"folderToWatch"}}
	w, err := gobounce.New(options, 100*time.Millisecond)
	if err != nil {
		log.Fatal(err)
	}

	go handleChanges(w)
	w.Start()
}

func handleChanges(w *gobounce.Filewatcher) {
	for {
		select {
		case filename := <-w.FileChanged:
			fmt.Println("file changed", filename)
		case folder := <-w.FolderChanged:
			fmt.Println("folder changed", folder)
		case <-w.Closed:
			return
		}
	}
}
```
