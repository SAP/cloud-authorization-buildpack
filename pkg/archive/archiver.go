package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/cloudfoundry/libbuildpack"
)

type archiver struct {
	tw   *tar.Writer
	log  *libbuildpack.Logger
	root string
}

func CreateArchive(log *libbuildpack.Logger, root string, relativeDirs []string) (io.Reader, error) {
	var buf bytes.Buffer
	zr := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zr)
	a := archiver{tw, log, root}
	for _, dir := range relativeDirs {
		if err := a.compressDir(path.Join(root, dir)); err != nil {
			return nil, err
		}
	}
	if err := a.addSchemaDCL(root); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zr.Close(); err != nil {
		return nil, err
	}

	a.log.Debug("uploaded tar: %s", base64.StdEncoding.EncodeToString(buf.Bytes()))
	return &buf, nil
}

func (a *archiver) addSchemaDCL(root string) error {
	fp := path.Join(root, "schema.dcl")
	fi, err := os.Stat(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return a.writeFile(fi, fp)
}

func (a *archiver) compressDir(dir string) error {
	return filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking file '%s': %w", file, err)
		}
		return a.writeFile(fi, file)
	})
}

func (a *archiver) writeFile(fi os.FileInfo, file string) error {
	relPath, err := filepath.Rel(a.root, file)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(fi, relPath)
	if err != nil {
		return err
	}

	header.Name = filepath.ToSlash(relPath)

	if err := a.tw.WriteHeader(header); err != nil {
		return err
	}

	if !fi.IsDir() {
		a.log.Info("adding file '%s' to policy bundle", file)
		data, err := os.Open(file)
		if err != nil {
			return err
		}
		if _, err := io.Copy(a.tw, data); err != nil {
			return err
		}
	}
	return nil
}
