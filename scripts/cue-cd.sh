# Source this script in your shell rc file (e.g., ~/.bashrc, ~/.zshrc) to have
# Cue's file explorer follow your terminal's working directory.
#
#   source /path/to/cue-cd.sh
#
# It overrides cd with a function that writes the new directory to $CUE_CWD_FILE
# (set by Cue on startup) before actually changing directory.

if [ -n "$CUE_CWD_FILE" ]; then
  cd() {
    builtin cd "$@" || return
    printf '%s' "$PWD" > "$CUE_CWD_FILE"
  }
  pushd() {
    builtin pushd "$@" || return
    printf '%s' "$PWD" > "$CUE_CWD_FILE"
  }
  popd() {
    builtin popd "$@" || return
    printf '%s' "$PWD" > "$CUE_CWD_FILE"
  }
fi
