package uploader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/cloudfoundry/libbuildpack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateArchive(t *testing.T) {
	writtenLogs := new(bytes.Buffer)
	logger := libbuildpack.NewLogger(writtenLogs)

	tests := []struct {
		name    string
		rootDir string
		input   map[string]string
		wantErr assert.ErrorAssertionFunc
		want    map[string]string // value can be _DIR_ to indicate a directory
	}{
		{name: "one dcl file in root", rootDir: "/", input: map[string]string{
			"schema.dcl":        "SCHEMA {}",
			"policies/dcl1.dcl": "POLICIES 123 {}",
		}, wantErr: assert.NoError, want: map[string]string{
			"schema.dcl":        "SCHEMA {}",
			"policies":          "_DIR_",
			"policies/dcl1.dcl": "POLICIES 123 {}",
		}},
		{name: "dcl files in subdir", rootDir: "/dcl", input: map[string]string{
			"otherfile.txt":         "hello world",
			"dcl/schema.dcl":        "SCHEMA {}",
			"dcl/policies/dcl1.dcl": "POLICIES 123 {}",
		}, wantErr: assert.NoError, want: map[string]string{
			"schema.dcl":        "SCHEMA {}",
			"policies":          "_DIR_",
			"policies/dcl1.dcl": "POLICIES 123 {}",
		}},
		{name: "non dcl files in dcl dir", rootDir: "/dcl", input: map[string]string{
			"dcl/schema.dcl":        "SCHEMA {}",
			"dcl/otherfile.txt":     "hello world",
			"dcl/policies/dcl1.dcl": "POLICIES 123 {}",
		}, wantErr: assert.NoError, want: map[string]string{
			"schema.dcl":        "SCHEMA {}",
			"policies":          "_DIR_",
			"policies/dcl1.dcl": "POLICIES 123 {}",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "dcl-temp-dir-")
			require.NoError(t, err)

			for name, content := range tt.input {
				if strings.Contains(name, "/") {
					dir := path.Join(tempDir, path.Dir(name))
					err := os.MkdirAll(dir, 0700)
					require.NoError(t, err)
				}
				createFile(t, path.Join(tempDir, name), content)
			}

			buf, err := CreateArchive(logger, path.Join(tempDir, tt.rootDir))
			tt.wantErr(t, err)

			if err == nil {
				assertBundleContent(t, buf, tt.want)
			}
		})
	}
}

func TestCreateArchiveJavaLog(t *testing.T) {
	tests := []struct {
		name        string
		rootDir     string
		existingDir string
		wantErr     require.ErrorAssertionFunc
		wantLog     string
	}{
		{name: "BOOT-INF configured instead of existing WEB-INF", rootDir: "/BOOT-INF/classes", existingDir: "/WEB-INF/classes", wantErr: require.Error, wantLog: "'/WEB-INF/classes' instead?"},
		{name: "WEB-INF configured instead of existing BOOT-INF", rootDir: "/WEB-INF/classes", existingDir: "/BOOT-INF/classes", wantErr: require.Error, wantLog: "'/BOOT-INF/classes' instead?"},
		{name: "WEB-INF configured correctly", rootDir: "/WEB-INF/classes", existingDir: "/WEB-INF/classes", wantErr: require.NoError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writtenLogs := new(bytes.Buffer)
			logger := libbuildpack.NewLogger(writtenLogs)

			tempDir, err := os.MkdirTemp("", "dcl-temp-dir-")
			require.NoError(t, err)

			dirs := path.Join(tempDir, tt.existingDir)
			err = os.MkdirAll(dirs, 0700)
			require.NoError(t, err)

			_, err = CreateArchive(logger, path.Join(tempDir, tt.rootDir))
			tt.wantErr(t, err)

			assert.Contains(t, writtenLogs.String(), tt.wantLog)
		})
	}
}

func createFile(t *testing.T, name, content string) {
	err := os.WriteFile(name, []byte(content), 0600)
	require.NoError(t, err)
}

func assertBundleContent(t *testing.T, buffer *bytes.Buffer, expected map[string]string) {
	gzReader, err := gzip.NewReader(buffer)
	require.NoError(t, err)

	defer gzReader.Close()

	tarGzReader := tar.NewReader(gzReader)

	actualFiles := []string{}
	actualDirs := []string{}
	for {
		header, err := tarGzReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		switch header.Typeflag {
		case tar.TypeReg:
			actualFiles = append(actualFiles, header.Name)
			var buf bytes.Buffer

			_, err := io.CopyN(&buf, tarGzReader, header.Size)
			require.NoError(t, err)

			expectedContent, fileIsExpected := expected[header.Name]
			assert.Truef(t, fileIsExpected, "unexpected file '%s' in archive", header.Name)
			assert.Equal(t, expectedContent, buf.String())
		case tar.TypeDir:
			actualDirs = append(actualDirs, header.Name)
			expectedContent, dirIsExpected := expected[header.Name]
			assert.Truef(t, dirIsExpected && expectedContent == "_DIR_", "unexpected dir '%s' in archive", header.Name)
		default:
			require.Error(t, fmt.Errorf("unsupported entry type '%d' for '%s'", header.Typeflag, header.Name))
		}
	}

	assert.Equalf(t, len(expected), len(actualFiles)+len(actualDirs), "unexpected amount of files/dirs in archive. files: %s, dirs: %s", actualFiles, actualDirs)
}
