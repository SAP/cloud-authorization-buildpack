package main

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getAMSDependencyDir(t *testing.T) {
	tests := []struct {
		name           string
		dependencyRoot string
		want           []string
		wantErr        assert.ErrorAssertionFunc
	}{
		{name: "AMS on pos 0", dependencyRoot: "./testdata/pos0", want: []string{"testdata/pos0/0"}, wantErr: assert.NoError},
		{name: "AMS on pos 1", dependencyRoot: "./testdata/pos1", want: []string{"testdata/pos1/1"}, wantErr: assert.NoError},
		{name: "AMS on pos 1, others have errors", dependencyRoot: "./testdata/pos1-others-have-errors", want: []string{"testdata/pos1-others-have-errors/1"}, wantErr: assert.NoError},
		{name: "AMS missing", dependencyRoot: "./testdata/ams-missing", want: nil, wantErr: assert.Error},
		{name: "AMS buildpack supplied twice", dependencyRoot: "./testdata/duplicate-opa-mitigation", want: []string{"testdata/duplicate-opa-mitigation/0", "testdata/duplicate-opa-mitigation/1"}, wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run(tt.name, func(t *testing.T) {
				got, err := getAMSDependencyDirs(tt.dependencyRoot)
				if !tt.wantErr(t, err, fmt.Sprintf("getAMSDependencyDirs(%v)", tt.dependencyRoot)) {
					return
				}
				assert.Equalf(t, tt.want, got, "getAMSDependencyDirs(%v)", tt.dependencyRoot)
			})
		})
	}
}

func Test_copyCertToDisk(t *testing.T) {
	depsDir, err := os.MkdirTemp("", "test")
	assert.NoError(t, err)

	crtBytes := []byte("hello cert")
	keyBytes := []byte("hello key")
	err = copyCertToDisk(depsDir, crtBytes, keyBytes)
	assert.NoError(t, err)

	crt, err := os.ReadFile(path.Join(depsDir, "ias.crt"))
	assert.NoError(t, err)
	assert.Equal(t, crtBytes, crt)
	key, err := os.ReadFile(path.Join(depsDir, "ias.key"))
	assert.Equal(t, keyBytes, key)
	assert.NoError(t, err)
}
