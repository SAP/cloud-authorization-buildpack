package compressor

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

func CreateArchive(root string, relativeDirs []string) (io.Reader, error) {
	var buf bytes.Buffer
	zr := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zr)
	for _, dir := range relativeDirs {
		if err := compressDir(path.Join(root, dir), tw); err != nil {
			return nil, err
		}
	}
	if err := addSchemaDCL(tw, root); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zr.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}

func addSchemaDCL(tw *tar.Writer, root string) error {
	fp := path.Join(root, "schema.dcl")
	fi, err := os.Stat(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return writeFile(tw, fi, fp)
}

func compressDir(dir string, tw *tar.Writer) error {
	return filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking file '%s': %w", file, err)
		}
		return writeFile(tw, fi, file)
	})
}

func writeFile(tw *tar.Writer, fi os.FileInfo, file string) error {
	header, err := tar.FileInfoHeader(fi, file)
	if err != nil {
		return err
	}

	// must provide real name
	// (see https://golang.org/src/archive/tar/common.go?#L626)
	header.Name = filepath.ToSlash(file)

	// write header
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	// if not a dir, write file content
	if !fi.IsDir() {
		data, err := os.Open(file)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, data); err != nil {
			return err
		}
	}
	return nil
}
