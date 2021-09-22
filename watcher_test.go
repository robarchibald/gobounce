package gobounce

import (
	"io/ioutil"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetWatchFolders(t *testing.T) {
	w := &Filewatcher{options: Options{RootFolders: []string{filepath.Join("testdata", "dir"), filepath.Join("testdata", "dir2")}, FolderExclusions: []string{"exclude", ""}}}
	folders, err := w.getWatchFolders()
	assert.NoError(t, err)
	assert.Equal(t, []string{
		filepath.Join("testdata", "dir"),
		filepath.Join("testdata", "dir", "subdir"),
		filepath.Join("testdata", "dir2"),
		filepath.Join("testdata", "dir2", "subdir2"),
		filepath.Join("testdata", "dir2", "subdir3"),
	}, folders)

	w = &Filewatcher{options: Options{RootFolders: []string{"//bogusPath"}}}
	_, err = w.getWatchFolders()
	assert.Error(t, err)
}

func TestNew(t *testing.T) {
	_, err := New(Options{RootFolders: []string{"//bogusPath"}}, time.Millisecond)
	assert.Error(t, err)
}

func TestWatch(t *testing.T) {
	w, err := New(Options{RootFolders: []string{"testdata"}}, 1*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	var called int
	var mutex sync.Mutex
	go func() {
		for {
			select {
			case <-w.FileChanged:
				mutex.Lock()
				called++
				mutex.Unlock()
			case <-w.FolderChanged:
			case <-w.watcher.Closed:
				return
			}
		}
	}()

	go w.Start()
	ioutil.WriteFile("testdata/test", []byte(time.Now().Format(time.RFC3339Nano)), 0644)
	ioutil.WriteFile("testdata/test2", []byte(time.Now().Format(time.RFC3339Nano)), 0644)
	for i := 0; i < 10; i++ {
		time.Sleep(time.Millisecond) // sleep so writes can complete
	}

	w.Close()

	assert.Equal(t, 2, called)
}

func TestGetWatcherPath(t *testing.T) {
	assert.Equal(t, "myNewFile", getWatcherPath("myFile -> myNewFile")) // simulate move or rename event
}
