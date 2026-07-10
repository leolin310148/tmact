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
	type captureResult struct {
		output []byte
		err    error
	}
	captured := make(chan captureResult, 1)
	go func() {
		output, readErr := io.ReadAll(read)
		captured <- captureResult{output: output, err: readErr}
	}()
	err = run(args)
	if closeErr := write.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	os.Stdout = oldStdout

	result := <-captured
	if result.err != nil && err == nil {
		err = result.err
	}
	return string(result.output), err
}

func stubCLIHooks(t *testing.T) func() {
	t.Helper()

	oldListAllTmuxPanes := listAllTmuxPanes
	oldListTargetTmuxPanes := listTargetTmuxPanes
	oldListSessionTmuxPanes := listSessionTmuxPanes
	oldNewTmuxSession := newTmuxSession
	oldNewTmuxWindow := newTmuxWindow
	oldPasteTmuxText := pasteTmuxText
	oldSendTmuxKeys := sendTmuxKeys
	oldTmactNow := tmactNow
	oldTmactSleep := tmactSleep
	oldTmactExecutable := tmactExecutable
	oldTrustFolderRun := trustFolderRun
	oldDispatchRun := dispatchRun
	oldDispatchRemoteRun := dispatchRemoteRun
	oldSendPeerPaneInput := sendPeerPaneInput

	return func() {
		listAllTmuxPanes = oldListAllTmuxPanes
		listTargetTmuxPanes = oldListTargetTmuxPanes
		listSessionTmuxPanes = oldListSessionTmuxPanes
		newTmuxSession = oldNewTmuxSession
		newTmuxWindow = oldNewTmuxWindow
		pasteTmuxText = oldPasteTmuxText
		sendTmuxKeys = oldSendTmuxKeys
		tmactNow = oldTmactNow
		tmactSleep = oldTmactSleep
		tmactExecutable = oldTmactExecutable
		trustFolderRun = oldTrustFolderRun
		dispatchRun = oldDispatchRun
		dispatchRemoteRun = oldDispatchRemoteRun
		sendPeerPaneInput = oldSendPeerPaneInput
	}
}
