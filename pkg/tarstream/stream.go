// +build !solution

package tarstream

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func Send(dir string, w io.Writer) error {
	tw := tar.NewWriter(w)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		if hdr.Name, err = filepath.Rel(dir, filepath.ToSlash(path)); err != nil {
			return err
		}
		if err = tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}
		return nil
	})
	return err
}

func Receive(dir string, r io.Reader) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		path := filepath.Join(dir, hdr.Name)
		if hdr.Typeflag == tar.TypeDir {
			err = os.Mkdir(path, os.FileMode(hdr.Mode).Perm())
			if err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			continue
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, os.FileMode(hdr.Mode).Perm())
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
