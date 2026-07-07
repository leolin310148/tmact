package shellhook

import (
	"fmt"
	"strings"
)

// Shells lists the shells InitScript supports.
var Shells = []string{"zsh", "bash", "fish"}

// InitScript returns a shell snippet that installs preexec/precmd hooks
// emitting events through `tmact hook emit`. The snippet is opt-in: the
// operator sources it (or eval's `tmact hook init <shell>`) themselves —
// tmact never edits shell rc files. Every emit is backgrounded with output
// discarded so a slow or missing daemon can never block the prompt.
func InitScript(shell string) (string, error) {
	switch shell {
	case "zsh":
		return zshInitScript, nil
	case "bash":
		return bashInitScript, nil
	case "fish":
		return fishInitScript, nil
	default:
		return "", fmt.Errorf("unsupported shell %q (expected %s)", shell, strings.Join(Shells, ", "))
	}
}

const zshInitScript = `# tmact shell hook (zsh). Opt-in: add to ~/.zshrc yourself:
#   eval "$(tmact hook init zsh)"
# Emits preexec/precmd events to a running tmact statusd. Never blocks the
# prompt: emits are backgrounded, output discarded, and skipped outside tmux.
if [[ -n ${TMUX_PANE-} ]] && command -v tmact >/dev/null 2>&1; then
  autoload -Uz add-zsh-hook

  _tmact_hook_preexec() {
    typeset -g TMACT_HOOK_CMD_ID="$$-$SECONDS-$RANDOM"
    command tmact hook emit --quiet --type preexec --command-id "$TMACT_HOOK_CMD_ID" --command "$1" >/dev/null 2>&1 &!
  }

  _tmact_hook_precmd() {
    local tmact_exit=$?
    [[ -n ${TMACT_HOOK_CMD_ID-} ]] || return 0
    command tmact hook emit --quiet --type precmd --command-id "$TMACT_HOOK_CMD_ID" --exit-code "$tmact_exit" >/dev/null 2>&1 &!
    unset TMACT_HOOK_CMD_ID
    return 0
  }

  add-zsh-hook preexec _tmact_hook_preexec
  add-zsh-hook precmd _tmact_hook_precmd
fi
`

const bashInitScript = `# tmact shell hook (bash). Opt-in: add to ~/.bashrc yourself:
#   eval "$(tmact hook init bash)"
# Emits preexec/precmd events to a running tmact statusd via a DEBUG trap and
# PROMPT_COMMAND. Never blocks the prompt: emits run in backgrounded subshells
# with output discarded, and everything is skipped outside tmux.
# Composes with your existing setup instead of clobbering it: any existing
# DEBUG trap is preserved and run first, and precmd is appended to
# PROMPT_COMMAND (scalar or bash 5.1+ array form) without flattening it.
# Best-effort limitation: the hook appends itself to the END of
# PROMPT_COMMAND, so with other PROMPT_COMMAND entries the reported exit code
# may be theirs rather than the user command's.
if [[ -n ${TMUX_PANE-} ]] && command -v tmact >/dev/null 2>&1; then
	  _tmact_hook_preexec() {
	    [[ -n ${COMP_LINE-} ]] && return 0
	    [[ -n ${TMACT_HOOK_CMD_ID-} ]] && return 0
	    # Skip DEBUG firings for our own functions and for other PROMPT_COMMAND
	    # entries (an empty Enter reruns PROMPT_COMMAND without a user command).
	    [[ ${BASH_COMMAND} == _tmact_hook_* ]] && return 0
	    _tmact_hook_is_prompt_command "$BASH_COMMAND" && return 0
	    TMACT_HOOK_CMD_ID="$$-${SECONDS}-${RANDOM}"
	    (command tmact hook emit --quiet --type preexec --command-id "$TMACT_HOOK_CMD_ID" --command "$BASH_COMMAND" >/dev/null 2>&1 &)
	    return 0
	  }

	  _tmact_hook_is_prompt_command() {
	    local tmact_cmd="$1"
	    if [[ $(declare -p PROMPT_COMMAND 2>/dev/null) == 'declare -a'* ]]; then
	      local tmact_pc
	      for tmact_pc in "${PROMPT_COMMAND[@]}"; do
	        [[ "$tmact_pc" == "$tmact_cmd" ]] && return 0
	      done
	      return 1
	    fi
	    local tmact_pc_list="${PROMPT_COMMAND-}"
	    [[ -z "$tmact_pc_list" ]] && return 1
	    case "$tmact_pc_list" in
	      "$tmact_cmd"|"$tmact_cmd;"*|*";$tmact_cmd"|*";$tmact_cmd;"*) return 0 ;;
	    esac
	    case "$tmact_pc_list" in
	      "$tmact_cmd"$'\n'*|*$'\n'"$tmact_cmd"|*$'\n'"$tmact_cmd"$'\n'*) return 0 ;;
	    esac
	    return 1
	  }

	  _tmact_hook_precmd() {
	    local tmact_exit=$?
    [[ -n ${TMACT_HOOK_CMD_ID-} ]] || return 0
    (command tmact hook emit --quiet --type precmd --command-id "$TMACT_HOOK_CMD_ID" --exit-code "$tmact_exit" >/dev/null 2>&1 &)
    unset TMACT_HOOK_CMD_ID
    return 0
  }

  # Compose with any existing DEBUG trap. trap -p prints
  #   trap -- 'CMD' DEBUG
  # so strip that wrapper to recover CMD (best effort: a CMD literally
  # containing "' DEBUG" would confuse the strip, which is rare).
  _tmact_prev_debug="$(trap -p DEBUG)"
  _tmact_prev_debug="${_tmact_prev_debug#trap -- \'}"
  _tmact_prev_debug="${_tmact_prev_debug%\' DEBUG}"
  case "${_tmact_prev_debug}" in
    *_tmact_hook_preexec*) : ;;                                   # already installed
    "") trap '_tmact_hook_preexec' DEBUG ;;                       # no prior trap
    *) trap "${_tmact_prev_debug}; _tmact_hook_preexec" DEBUG ;;  # run prior trap first
  esac
  unset _tmact_prev_debug

  # Append precmd to PROMPT_COMMAND without coercing its type. bash 5.1+
  # allows PROMPT_COMMAND to be an array; appending a string would flatten
  # it and break other entries, so branch on the actual variable type.
  if [[ $(declare -p PROMPT_COMMAND 2>/dev/null) == 'declare -a'* ]]; then
    case " ${PROMPT_COMMAND[*]} " in
      *' _tmact_hook_precmd '*) : ;;
      *) PROMPT_COMMAND+=('_tmact_hook_precmd') ;;
    esac
  elif [[ ${PROMPT_COMMAND-} != *_tmact_hook_precmd* ]]; then
    PROMPT_COMMAND="${PROMPT_COMMAND:+$PROMPT_COMMAND$'\n'}_tmact_hook_precmd"
  fi
fi
`

const fishInitScript = `# tmact shell hook (fish). Opt-in: add to ~/.config/fish/config.fish yourself:
#   tmact hook init fish | source
# Emits preexec/precmd events to a running tmact statusd. Never blocks the
# prompt: emits are backgrounded and disowned with output discarded, and
# skipped outside tmux.
if set -q TMUX_PANE; and type -q tmact
  function _tmact_hook_preexec --on-event fish_preexec
    set -g TMACT_HOOK_CMD_ID (echo %self)-(random)
    command tmact hook emit --quiet --type preexec --command-id "$TMACT_HOOK_CMD_ID" --command "$argv[1]" >/dev/null 2>&1 &
    disown 2>/dev/null
  end

  function _tmact_hook_precmd --on-event fish_postexec
    set -l tmact_exit $status
    if set -q TMACT_HOOK_CMD_ID
      command tmact hook emit --quiet --type precmd --command-id "$TMACT_HOOK_CMD_ID" --exit-code "$tmact_exit" >/dev/null 2>&1 &
      disown 2>/dev/null
      set -e TMACT_HOOK_CMD_ID
    end
  end
end
`
