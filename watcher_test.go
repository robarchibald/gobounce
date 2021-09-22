package gobounce

import (
	"io/ioutil"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetWatchFolders(t *testing.T) {
	w := &Filewatcher{options: Options{RootFolders: []string{filepath.Join("testdata", "dir")}, FolderExclusions: []string{"exclude", ""}}}
	folders, err := w.getWatchFolders()
	assert.NoError(t, err)
	assert.Equal(t, []string{
		filepath.Join("testdata", "dir"),
		filepath.Join("testdata", "dir", "subdir"),
	}, folders)

	w = &Filewatcher{options: Options{RootFolders: []string{"//bogusPath"}}}
	_, err = w.getWatchFolders()
	assert.Error(t, err)

	w = &Filewatcher{options: Options{RootFolders: []string{"/path"}, ExcludeSubdirs: true}}
	folders, _ = w.getWatchFolders()
	assert.Equal(t, []string{"/path"}, folders)
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

func TestWatchFolders(t *testing.T) {
	dir, _ := filepath.Abs("testdata/dir")
	hidden, _ := filepath.Abs("testdata/dir/.hidden")
	exclude, _ := filepath.Abs("testdata/dir/exclude")
	excludeSubdir, _ := filepath.Abs("testdata/dir/exclude/othersubdir")
	subdir, _ := filepath.Abs("testdata/dir/subdir")
	tests := []struct {
		name    string
		options Options
		want    []string
	}{
		{"no subdirs",
			Options{
				RootFolders:    []string{"testdata/dir"},
				ExcludeSubdirs: true,
			},
			[]string{dir}},
		{"with subdirs & hidden",
			Options{
				RootFolders:   []string{"testdata/dir"},
				IncludeHidden: true,
			},
			[]string{dir, hidden, exclude, excludeSubdir, subdir}},
		{"no hidden",
			Options{
				RootFolders: []string{"testdata/dir"},
			},
			[]string{dir, exclude, excludeSubdir, subdir}},
		{"without exclude",
			Options{
				RootFolders:      []string{"testdata/dir"},
				ExcludeSubdirs:   false,
				IncludeHidden:    false,
				FolderExclusions: []string{"exclude"},
			},
			[]string{dir, subdir}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := New(tt.options, time.Millisecond)
			require.NoError(t, err)
			assert.Equal(t, tt.want, w.WatchFolders())
		})
	}
}
