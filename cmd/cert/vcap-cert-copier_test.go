package main

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getAMSDependencyDir(t *testing.T) {
	tests := []struct {
		name           string
		dependencyRoot string
		want           string
		wantErr        bool
	}{
		{name: "AMS on pos 0", dependencyRoot: "./testdata/pos0", want: "testdata/pos0/0", wantErr: false},
		{name: "AMS on pos 1", dependencyRoot: "./testdata/pos1", want: "testdata/pos1/1", wantErr: false},
		{name: "AMS on pos 1, others have errors", dependencyRoot: "./testdata/pos1-others-have-errors", want: "testdata/pos1-others-have-errors/1", wantErr: false},
		{name: "AMS missing", dependencyRoot: "./testdata/ams-missing", want: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getAMSDependencyDir(tt.dependencyRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAMSDependencyDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getAMSDependencyDir() got = %v, want %v", got, tt.want)
			}
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
