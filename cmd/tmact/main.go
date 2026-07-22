package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

type globalOptions struct {
	Target string
}

type repeatedStrings []string

func (r *repeatedStrings) String() string {
	return strings.Join(*r, ",")
}

func (r *repeatedStrings) Set(value string) error {
	if value == "" {
		return errors.New("value cannot be empty")
	}
	*r = append(*r, value)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var (
	listAllTmuxPanes     = tmux.ListAllPanes
	listTargetTmuxPanes  = tmux.ListPanes
	listSessionTmuxPanes = tmux.ListSessionPanes
	captureTmuxPane      = tmux.CapturePane
	captureTmuxPaneInfo  = tmux.CapturePaneInfoForTarget
	newTmuxSession       = tmux.NewSession
	newTmuxWindow        = tmux.NewWindow
	pasteTmuxText        = tmux.PasteText
	sendTmuxKeys         = tmux.SendKeys
	tmactNow             = time.Now
	tmactSleep           = time.Sleep
	tmactExecutable      = os.Executable
)

func run(args []string) error {
	globals, args, err := parseGlobalArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return usage()
	}

	switch args[0] {
	case "ls":
		if globals.Target != "" {
			return errors.New("global -t/--target is not valid with ls")
		}
		return runList(args[1:])
	case "send":
		return runSend(args[1:], globals)
	case "capture":
		return runCapture(args[1:], globals)
	case "wait":
		return runWait(args[1:], globals)
	case "session":
		if globals.Target != "" {
			return errors.New("global -t/--target is not valid with session; pass one exact session name")
		}
		return runSession(args[1:])
	case "detect":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runDetect(args[1:])
	case "inspect":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runInspect(args[1:])
	case "status":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runStatus(args[1:])
	case "statusd":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runStatusd(args[1:])
	case "usage":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runUsage(args[1:])
	case "log":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runLog(args[1:])
	case "human-active":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runHumanActive(args[1:])
	case "stt-set":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runSTTSet(args[1:])
	case "inbox":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runInbox(args[1:])
	case "summarize":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runSummarize(args[1:])
	case "broadcast":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runBroadcast(args[1:])
	case "panels":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runPanels(args[1:])
	case "loop":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runLoop(args[1:])
	case "workflow":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runWorkflow(args[1:])
	case "watch":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runWatch(args[1:])
	case "dispatch-work":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runDispatch(args[1:])
	case "trust-folder":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runTrustFolder(args[1:])
	case "hook":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runHook(args[1:])
	case "commands":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runCommands(args[1:])
	case "llm":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runLLM(args[1:])
	case "help":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runHelp(args[1:])
	case "version", "-v", "--version", "-version":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send, capture, and wait")
		}
		return runVersion(args[1:])
	case "-h", "--help":
		return usage()
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func parseGlobalArgs(args []string) (globalOptions, []string, error) {
	var opts globalOptions
	for len(args) > 0 {
		arg := args[0]
		switch {
		case arg == "-t" || arg == "--target":
			if len(args) < 2 || args[1] == "" {
				return opts, args, fmt.Errorf("%s requires a value", arg)
			}
			opts.Target = args[1]
			args = args[2:]
		case strings.HasPrefix(arg, "-t="):
			opts.Target = strings.TrimPrefix(arg, "-t=")
			args = args[1:]
		case strings.HasPrefix(arg, "--target="):
			opts.Target = strings.TrimPrefix(arg, "--target=")
			args = args[1:]
		default:
			return opts, args, nil
		}
		if opts.Target == "" {
			return opts, args, errors.New("target cannot be empty")
		}
	}
	return opts, args, nil
}

func wantsHelp(args []string) bool {
	return len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help")
}
