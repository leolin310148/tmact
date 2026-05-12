package statusd

import "tmact/internal/panestatus"

func RuntimeTag(runtime string) string {
	switch runtime {
	case panestatus.RuntimeClaude:
		return "cc"
	case panestatus.RuntimeCodex:
		return "cx"
	case panestatus.RuntimeCopilot:
		return "cp"
	case panestatus.RuntimeGemini:
		return "g"
	case panestatus.RuntimeTmact:
		return "tm"
	case panestatus.RuntimeShell, panestatus.RuntimeUnknown:
		return "$"
	default:
		return "$"
	}
}

func RunningGlyph(running bool) string {
	if running {
		return "▸"
	}
	return ""
}

func AskingGlyph(asking bool) string {
	if asking {
		return "!"
	}
	return ""
}
