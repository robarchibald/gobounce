// This is a generalized filesystem watcher with ideas taken from the https://github.com/6degreeshealth/autotest package
package gobounce

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/radovskyb/watcher"
)

type Filewatcher struct {
	FileChanged   chan string
	FolderChanged chan string
	Closed        chan struct{}

	watcher          *watcher.Watcher
	options          Options
	watchFolders     []string
	pollDuration     time.Duration
	fileDebounce     map[string]*time.Timer
	folderDebounce   map[string]*time.Timer
	debounceDuration time.Duration
	mutex            sync.Mutex
}

type Options struct {
	RootFolders      []string
	FolderExclusions []string
	IncludeHidden    bool
	ExcludeSubdirs   bool
}

// New creates a debounced file watcher. It will watch for changes to the filesystem every `pollDuration` duration
// and notify of changes after no change has been seen in that file or folder in 2x the `pollDuration`. For example,
// if the pollDuration is set to 1 second, the debounceDuration will automatically be set to 2 seconds. This would be
// the timeline then for an example file:
// (0 seconds)     poll for changes: none found
// (0.3 seconds)   folder1/file1 updated
// (1 second)      poll for changes: 1 folder1/file1 and 1 folder1 change found
//                 debounce timer for folder1 created for 2 seconds due to change
//                 debounce timer for folder1/file1 created for 2 seconds due to change
// (1.1 second)    folder1/file2 updated
// (2 seconds)     poll for changes - 1 folder1/file2 and 1 folder1 change found
// 	               debounce timer for folder1 reset to 2 seconds due to new change to folder1
//                 debounce timer for folder1/file2 created for 2 seconds due to change
// (3 seconds)     poll for changes - no new changes found
//                 debounce timer finishes for folder1/file1. FileChanged channel publishes the filename
// (4 seconds)     poll for changes - no new changes found
//                 debounce timer finishes for folder1/file2. FileChanged channel publishes the filename
//                 debounce timer finishes for folder1. FileChanged channel publishes the folder name
func New(options Options, pollDuration time.Duration) (*Filewatcher, error) {
	w := &Filewatcher{
		FileChanged:      make(chan string),
		FolderChanged:    make(chan string),
		watcher:          watcher.New(),
		options:          options,
		pollDuration:     pollDuration,
		debounceDuration: 2 * pollDuration, // note that the debounceDuration must always be > pollDuration for debounce to work
		fileDebounce:     make(map[string]*time.Timer),
		folderDebounce:   make(map[string]*time.Timer),
	}
	w.Closed = w.watcher.Closed
	if !w.options.IncludeHidden {
		w.watcher.IgnoreHiddenFiles(true)
	}

	var err error
	if !options.ExcludeSubdirs {
		w.watchFolders, err = w.getWatchFolders()
		if err != nil {
			return nil, fmt.Errorf("error determining watch folders: %w", err)
		}
	} else {
		w.watchFolders = options.RootFolders
	}
	for _, folder := range w.watchFolders {
		if err := w.watcher.Add(folder); err != nil {
			return nil, fmt.Errorf("error adding watch folder: %w", err)
		}
	}
	return w, nil
}

func (w *Filewatcher) getWatchFolders() ([]string, error) {
	watchFolders := []string{}
	exclusions := prepareFolders(w.options.FolderExclusions)
	for _, rootFolder := range w.options.RootFolders {
		err := filepath.WalkDir(rootFolder, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			pathWithSlashes := string(filepath.Separator) + path + string(filepath.Separator)
			for _, excludedFolder := range exclusions {
				if strings.Contains(pathWithSlashes, excludedFolder) { // match against full folder name or subdir. partial names not allowed
					return nil
				}
			}
			watchFolders = append(watchFolders, path)
			return nil
		})
		if err != nil {
			return watchFolders, err
		}
	}
	return watchFolders, nil
}

func prepareFolders(folders []string) []string {
	for i := 0; i < len(folders); i++ {
		folder := strings.Trim(folders[i], `/\`) // trim leading and trailing folder separators for consistency
		if len(folder) == 0 {                    // remove empty folder from slice
			folders = append(folders[:i], folders[i+1:]...)
			i--
			continue
		}
		folders[i] = fmt.Sprintf("%c%s%c", filepath.Separator, folder, filepath.Separator) // add leading and trailing separator
	}
	return folders
}

func (w *Filewatcher) WatchFolders() []string {
	return w.watchFolders
}

func (w *Filewatcher) Start() {
	go w.listen()

	w.watcher.Start(w.pollDuration)
}

func (w *Filewatcher) listen() {
	for {
		select {
		case e := <-w.watcher.Event:
			w.debounce(e)
		case <-w.watcher.Closed:
			return
		}
	}
}

func (w *Filewatcher) Close() {
	w.watcher.Close()
	close(w.FileChanged)
	close(w.FolderChanged)
}

func (w *Filewatcher) debounce(e watcher.Event) {
	path, _ := filepath.Abs(getWatcherPath(e.Path))
	if path == "" {
		return
	}

	w.mutex.Lock()
	w.debounceItem(w.fileDebounce, path, w.FileChanged)
	w.debounceItem(w.folderDebounce, filepath.Dir(path), w.FolderChanged)
	w.mutex.Unlock()
}

func (w *Filewatcher) debounceItem(debounceMap map[string]*time.Timer, path string, notifyChannel chan string) {
	timer, ok := debounceMap[path]
	if !ok {
		timer = time.NewTimer(w.debounceDuration)
		debounceMap[path] = timer
		go w.waitDebounceTimer(timer, debounceMap, path, notifyChannel)
	} else {
		timer.Reset(w.debounceDuration)
	}
}

func (w *Filewatcher) waitDebounceTimer(timer *time.Timer, debounceMap map[string]*time.Timer, path string, notifyChannel chan string) {
	<-timer.C
	timer.Stop()

	w.mutex.Lock()
	delete(debounceMap, path)
	w.mutex.Unlock()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return // file has been deleted since we started the timer, so ignore
	}
	notifyChannel <- path
}

func getWatcherPath(path string) string {
	// Rename and Move events path is in the format of fromPath -> toPath according to https://github.com/radovskyb/watcher
	toPathIndex := strings.Index(path, "-> ")
	if toPathIndex != -1 {
		return path[toPathIndex+3:]
	}

	return path
}
