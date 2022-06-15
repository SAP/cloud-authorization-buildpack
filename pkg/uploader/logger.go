package uploader

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"unicode"
)

type Dclssue struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Message     string `json:"message"`
	SyntaxError bool   `json:"syntaxError"`
	Severity    string `json:"severity"`
	Code        int    `json:"code"`
}
type CompileError struct {
	Status       string   `json:"status"`
	Message      string   `json:"message"`
	Errors       []string `json:"errors"`
	CompileError struct {
		DclIssues map[string][]Dclssue `json:"dclIssues"`
	} `json:"compile_error"`
}

const severityError = "ERROR"
const severityWarning = "WARNING"

func (up *Uploader) logResponse(res *http.Response, reqURL string) error {
	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusNotModified {
		up.Log.Info("Base DCLs uploaded and compiled successfully")
		return nil
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("unexpected response on DCL upload to %s: status(%s)", reqURL, res.Status)
	}
	if res.StatusCode == http.StatusBadRequest {
		var ce CompileError
		if err = json.Unmarshal(b, &ce); err == nil {
			if err = up.printCompileError(ce); err == nil {
				return fmt.Errorf("DCL upload failed: status(%s) body(%s)", res.Status, string(b))
			}
		}
	}
	return fmt.Errorf("unexpected response on DCL upload to %s: status(%s) body(%s)", reqURL, res.Status, string(b))
}
func (up *Uploader) printCompileError(compileError CompileError) error {
	for file, issues := range compileError.CompileError.DclIssues {
		for _, issue := range issues {
			reader, err := os.Open(up.Root + file)
			if err != nil {
				return err
			}
			line, err := readLine(reader, issue.Line)
			if err != nil {
				return err
			}
			logFunc := up.Log.Info
			if issue.Severity == severityError {
				logFunc = up.Log.Error
			} else if issue.Severity == severityWarning {
				logFunc = up.Log.Warning
			}

			logFunc(createHeaderLine(issue, file))
			logFunc(line)
			logFunc(createMarkerLine(line, issue.Column))
		}
	}
	return nil
}
func createHeaderLine(issue Dclssue, file string) string {
	if issue.SyntaxError {
		return fmt.Sprintf("Syntax Error in %v line %v: %v ", file, issue.Line, issue.Message)
	}
	return fmt.Sprintf("%v in %v line %v: %v ", issue.Severity, file, issue.Line, issue.Message)
}
func createMarkerLine(line string, col int) (markerLine string) {
	for i, rune := range line {
		if i == col-1 {
			markerLine += "^"
		} else if unicode.IsSpace(rune) {
			markerLine += string(rune)
		} else {
			markerLine += " "
		}
	}
	return markerLine
}
func readLine(r io.Reader, lineNum int) (line string, err error) {
	sc := bufio.NewScanner(r)
	var lastLine int
	for sc.Scan() {
		lastLine++
		if lastLine == lineNum {
			return sc.Text(), sc.Err()
		}
	}
	return line, io.EOF
}
