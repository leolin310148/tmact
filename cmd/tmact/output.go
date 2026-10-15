package main

import (
	"encoding/json"
	"os"
)

func printJSON(value interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// reportedError marks a failure already reported inside the command's JSON
// document. main exits non-zero without echoing it to stderr, so callers that
// merge stdout and stderr still read exactly one JSON document.
type reportedError struct{ err error }

func (e reportedError) Error() string { return e.err.Error() }

func (e reportedError) Unwrap() error { return e.err }

// jsonFailure prints report (which must already carry err) as the command's
// only stdout document and returns err marked as reported.
func jsonFailure(report interface{}, err error) error {
	if printErr := printJSON(report); printErr != nil {
		return printErr
	}
	return reportedError{err: err}
}
