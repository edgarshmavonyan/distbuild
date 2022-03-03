// +build !solution

package filecache

import (
	"errors"
	"gitlab.com/slon/shad-go/distbuild/pkg/artifact"
	"io"
	"os"
	"path/filepath"

	"gitlab.com/slon/shad-go/distbuild/pkg/build"
)

var (
	ErrNotFound    = errors.New("file not found")
	ErrExists      = errors.New("file exists")
	ErrWriteLocked = errors.New("file is locked for write")
	ErrReadLocked  = errors.New("file is locked for read")
)

const DefaultFileName = "data"

type Cache struct {
	artCache *artifact.Cache
}

func retypeArtifactError(err error) error {
	switch {
	case errors.Is(err, artifact.ErrWriteLocked):
		return ErrWriteLocked
	case errors.Is(err, artifact.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, artifact.ErrReadLocked):
		return ErrReadLocked
	case errors.Is(err, artifact.ErrExists):
		return ErrExists
	default:
		return err
	}
}

func New(rootDir string) (*Cache, error) {
	cache, err := artifact.NewCache(rootDir)
	if err != nil {
		return nil, err
	}
	return &Cache{cache}, nil
}

func (c *Cache) Range(fileFn func(file build.ID) error) error {
	err := c.artCache.Range(fileFn)
	return retypeArtifactError(err)
}

func (c *Cache) Remove(file build.ID) error {
	err := c.artCache.Remove(file)
	return retypeArtifactError(err)
}

type cacheFileWriter struct {
	fileWriter io.WriteCloser
	commitFunc func() error
	closed     bool
}

func (f *cacheFileWriter) Close() error {
	if f.closed {
		return errors.New("close called second time")
	}
	f.closed = true
	_ = f.Close()
	err := f.commitFunc()
	return err
}

func (f *cacheFileWriter) Write(p []byte) (int, error) {
	return f.fileWriter.Write(p)
}

func (c *Cache) Write(file build.ID) (w io.WriteCloser, abort func() error, err error) {
	var path string
	var commit func() error
	path, commit, abort, err = c.artCache.Create(file)
	if err != nil {
		err = retypeArtifactError(err)
		return
	}
	path = filepath.Join(path, DefaultFileName)
	f, err := os.Create(path)
	if err != nil {
		return
	}
	return &cacheFileWriter{
		fileWriter: f,
		commitFunc: commit,
		closed:     false,
	}, abort, err
}

func (c *Cache) Get(file build.ID) (path string, unlock func(), err error) {
	path, unlock, err = c.artCache.Get(file)
	if err != nil {
		err = retypeArtifactError(err)
		return
	}
	path = filepath.Join(path, DefaultFileName)
	return
}
