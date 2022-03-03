// +build !solution

package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"gitlab.com/slon/shad-go/distbuild/pkg/build"
)

var (
	ErrNotFound    = errors.New("artifact not found")
	ErrExists      = errors.New("artifact exists")
	ErrWriteLocked = errors.New("artifact is locked for write")
	ErrReadLocked  = errors.New("artifact is locked for read")
)

type entry struct {
	mu        sync.Mutex
	rCounter  int
	wCounter  int
	path      string
	committed bool
}

type Cache struct {
	root    string
	mu      sync.RWMutex
	entries map[string]*entry
}

const DirectoryPerm = os.FileMode(0777)

func NewCache(root string) (*Cache, error) {
	err := os.MkdirAll(root, DirectoryPerm)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	return &Cache{
		root:    root,
		mu:      sync.RWMutex{},
		entries: make(map[string]*entry),
	}, nil
}

func (c *Cache) Range(artifactFn func(artifact build.ID) error) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var id build.ID
	for k, curEntry := range c.entries {
		err := id.UnmarshalText([]byte(k))
		if err != nil {
			return err
		}
		curEntry.mu.Lock()
		curEntry.rCounter++
		curEntry.mu.Unlock()
		err = artifactFn(id)
		curEntry.mu.Lock()
		curEntry.rCounter--
		curEntry.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Cache) Remove(artifact build.ID) error {
	aString := artifact.String()
	c.mu.Lock()
	defer c.mu.Unlock()
	curEntry, ok := c.entries[aString]
	if !ok {
		return ErrNotFound
	}
	curEntry.mu.Lock()
	defer curEntry.mu.Unlock()
	if !curEntry.committed {
		return ErrWriteLocked
	}
	if curEntry.rCounter > 0 {
		return ErrReadLocked
	}
	delete(c.entries, aString)
	return os.RemoveAll(curEntry.path)
}

func (c *Cache) Create(artifact build.ID) (path string, commit, abort func() error, err error) {
	aString := artifact.String()
	c.mu.Lock()
	curEntry, ok := c.entries[aString]
	if ok {
		curEntry.mu.Lock()
		defer curEntry.mu.Unlock()
		if curEntry.committed {
			err = ErrExists
		} else {
			err = ErrWriteLocked
		}
		c.mu.Unlock()
		return
	}
	curEntry = &entry{
		path:      filepath.Join(c.root, aString),
		committed: false,
	}
	c.entries[aString] = curEntry
	c.mu.Unlock()
	path = curEntry.path
	commit = func() error {
		c.mu.Lock()
		curEntry, ok := c.entries[aString]
		c.mu.Unlock()
		if !ok {
			return nil
		}
		curEntry.mu.Lock()
		defer curEntry.mu.Unlock()
		if curEntry.committed {
			return nil
		}
		curEntry.committed = true
		return nil
	}
	abort = func() error {
		c.mu.Lock()
		curEntry, ok := c.entries[aString]
		if !ok {
			c.mu.Unlock()
			return nil
		}
		delete(c.entries, aString)
		c.mu.Unlock()
		curEntry.mu.Lock()
		defer curEntry.mu.Unlock()
		if curEntry.committed {
			return nil
		}
		return os.RemoveAll(curEntry.path)
	}
	err = os.Mkdir(path, DirectoryPerm)
	return
}

func (c *Cache) Get(artifact build.ID) (path string, unlock func(), err error) {
	aString := artifact.String()
	c.mu.RLock()
	curEntry, ok := c.entries[aString]
	if !ok {
		c.mu.RUnlock()
		err = ErrNotFound
		return
	}
	curEntry.mu.Lock()
	c.mu.RUnlock()
	if !curEntry.committed {
		curEntry.mu.Unlock()
		err = ErrWriteLocked
		return
	}
	curEntry.rCounter++
	curEntry.mu.Unlock()
	path = curEntry.path
	unlock = func() {
		curEntry.mu.Lock()
		curEntry.rCounter--
		curEntry.mu.Unlock()
	}
	return
}
