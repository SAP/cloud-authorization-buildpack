package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"strings"

	"io/ioutil"
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

type archiveContent struct {
	header *tar.Header
	file   string
}

func CreateArchive(log *libbuildpack.Logger, root string) (io.Reader, error) {
	var buf bytes.Buffer
	zr := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zr)
	a := archiver{tw, log, root}

	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}

	content, err := a.crawDCLs(rootInfo, root)
	if err != nil {
		return nil, err
	}
	for _, c := range *content {
		if err := a.tw.WriteHeader(c.header); err != nil {
			return nil, err
		}
		if c.file != "" {
			a.log.Info("adding file '%s' to policy bundle", c.file)
			data, err := os.Open(c.file)
			if err != nil {
				return nil, err
			}
			if _, err := io.Copy(a.tw, data); err != nil {
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

	a.log.Debug("uploaded tar: %s", base64.StdEncoding.EncodeToString(buf.Bytes()))
	return &buf, nil
}

func (a *archiver) crawDCLs(fi os.FileInfo, file string) (*[]archiveContent, error) {
	var archive []archiveContent
	if fi.IsDir() {
		content, err := ioutil.ReadDir(file)
		if err != nil {
			return nil, err
		}
		for _, cfi := range content {

			carchive, err := a.crawDCLs(cfi, path.Join(file, cfi.Name()))
			if err != nil {
				return nil, err
			}
			archive = append(archive, *carchive...)
		}
		if len(archive) > 0 && file != a.root {
			ce, err := a.createContentEntry(fi, file)
			if err != nil {
				return nil, err
			}
			archive = append(*ce, archive...)
		}
		return &archive, nil
	} else {
		return a.createContentEntry(fi, file)
	}

}

func (a *archiver) createContentEntry(fi os.FileInfo, file string) (*[]archiveContent, error) {
	var resultLine archiveContent
	if !fi.IsDir() && !strings.HasSuffix(file, ".dcl") && !strings.HasSuffix(file, ".json") {
		return &[]archiveContent{}, nil
	}

	relPath, err := filepath.Rel(a.root, file)
	if err != nil {
		return nil, err
	}
	resultLine.header, err = tar.FileInfoHeader(fi, relPath)
	if err != nil {
		return nil, err
	}

	resultLine.header.Name = filepath.ToSlash(relPath)
	if !fi.IsDir() {
		resultLine.file = file
	}
	return &[]archiveContent{resultLine}, nil
}
