package main

import (
	"io"
	"os"
	"testing"
)

func captureRun(t *testing.T, args ...string) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	err = run(args)
	if closeErr := write.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	os.Stdout = oldStdout

	output, readErr := io.ReadAll(read)
	if readErr != nil && err == nil {
		err = readErr
	}
	return string(output), err
}

func stubCLIHooks(t *testing.T) func() {
	t.Helper()

	oldListAllTmuxPanes := listAllTmuxPanes
	oldListTargetTmuxPanes := listTargetTmuxPanes
	oldPasteTmuxText := pasteTmuxText
	oldSendTmuxKeys := sendTmuxKeys
	oldTmactNow := tmactNow

	return func() {
		listAllTmuxPanes = oldListAllTmuxPanes
		listTargetTmuxPanes = oldListTargetTmuxPanes
		pasteTmuxText = oldPasteTmuxText
		sendTmuxKeys = oldSendTmuxKeys
		tmactNow = oldTmactNow
	}
}
