# Source this script in your Nushell env.nu to have Cue's file explorer
# follow your terminal's working directory.
#
#   source /path/to/cue-cd.nu
#
# It overrides the built-in `cd` command with a custom command that writes
# the new directory to $env.CUE_CWD_FILE (set by Cue on startup).

if "CUE_CWD_FILE" in $env {
  def --env cue-cd [dir?: string] {
    cd $dir
    $env.PWD | save -f $env.CUE_CWD_FILE
  }
}
