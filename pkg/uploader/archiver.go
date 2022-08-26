package uploader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry/libbuildpack"
)

type archiveContent struct {
	header *tar.Header
	file   string
}

func CreateArchive(log *libbuildpack.Logger, root string) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	zr := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zr)

	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}

	content, err := crawlDCLs(rootInfo, root, root)
	if err != nil {
		return nil, err
	}
	for _, c := range *content {
		if err := tw.WriteHeader(c.header); err != nil {
			return nil, err
		}
		if c.file != "" {
			log.Info("adding file '%s' to policy upload archive", c.header.Name)
			data, err := os.Open(c.file)
			if err != nil {
				return nil, err
			}
			if _, err := io.Copy(tw, data); err != nil {
				return nil, err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zr.Close(); err != nil {
		return nil, err
	}

	log.Debug("uploaded tar: %s", base64.StdEncoding.EncodeToString(buf.Bytes()))
	return &buf, nil
}

func crawlDCLs(fi os.FileInfo, file, root string) (*[]archiveContent, error) {
	var archive []archiveContent
	if fi.IsDir() {
		content, err := ioutil.ReadDir(file) //nolint
		if err != nil {
			return nil, err
		}
		for _, cfi := range content {
			carchive, err := crawlDCLs(cfi, path.Join(file, cfi.Name()), root)
			if err != nil {
				return nil, err
			}
			archive = append(archive, *carchive...)
		}
		if len(archive) > 0 && file != root {
			ce, err := createContentEntry(fi, file, root)
			if err != nil {
				return nil, err
			}
			archive = append(*ce, archive...)
		}
		return &archive, nil
	}
	return createContentEntry(fi, file, root)
}

func createContentEntry(fi os.FileInfo, file, root string) (*[]archiveContent, error) {
	var result archiveContent
	if !fi.IsDir() && !strings.HasSuffix(file, ".dcl") {
		return &[]archiveContent{}, nil
	}

	relPath, err := filepath.Rel(root, file)
	if err != nil {
		return nil, err
	}
	result.header, err = tar.FileInfoHeader(fi, relPath)
	if err != nil {
		return nil, err
	}

	result.header.Name = filepath.ToSlash(relPath)
	if !fi.IsDir() {
		result.file = file
	}
	return &[]archiveContent{result}, nil
}
