# Source this script in your PowerShell profile to have Cue's file explorer
# follow your terminal's working directory.
#
#   . C:\path\to\cue-cd.ps1
#
# It overrides Set-Location (cd) with a proxy that writes the new directory
# to $env:CUE_CWD_FILE (set by Cue on startup) before delegating to the
# built-in command.

if ($env:CUE_CWD_FILE) {
  $originalSetLocation = Get-Command Set-Location -Type Cmdlet
  function Global:Set-Location {
    param(
      [Parameter(ValueFromPipeline=$true, Position=0)]
      [string]$Path
    )
    & $originalSetLocation @PSBoundParameters
    if ($?) {
      $PWD.ProviderPath | Out-File -FilePath $env:CUE_CWD_FILE -NoNewline -Encoding utf8
    }
  }
  Set-Alias -Name cd -Value Set-Location -Scope Global -Option AllScope
  Set-Alias -Name sl -Value Set-Location -Scope Global -Option AllScope
}
