package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook <bash|zsh|fish|powershell>",
	Short: "Generate shell hook for automatic .clim.yaml checking",
	Long: `Generate a shell hook that automatically runs 'clim check' when you
cd into a directory with a .clim.yaml file.

Usage:

  bash:
    eval "$(clim shell hook bash)"
    # To load on every session:
    echo 'eval "$(clim shell hook bash)"' >> ~/.bashrc

  zsh:
    eval "$(clim shell hook zsh)"
    # To load on every session:
    echo 'eval "$(clim shell hook zsh)"' >> ~/.zshrc

  fish:
    clim shell hook fish | source
    # To load on every session:
    clim shell hook fish > ~/.config/fish/conf.d/clim-hook.fish

  powershell:
    clim shell hook powershell | Out-String | Invoke-Expression
    # To load on every session, add to your $PROFILE:
    Add-Content $PROFILE 'clim shell hook powershell | Out-String | Invoke-Expression'`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(requireArgs(1, "clim shell hook <bash|zsh|fish|powershell>"), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			_, _ = os.Stdout.WriteString(hookBash)
		case "zsh":
			_, _ = os.Stdout.WriteString(hookZsh)
		case "fish":
			_, _ = os.Stdout.WriteString(hookFish)
		case "powershell":
			_, _ = os.Stdout.WriteString(hookPowerShell)
		}
		return nil
	},
}

func init() {
	// Registered under shellCmd in shell.go.
}

const hookBash = `# clim shell hook — auto-check .clim.yaml on cd
__clim_hook() {
  local prev_dir="$OLDPWD"
  builtin cd "$@" || return $?
  if [ "$PWD" != "$prev_dir" ]; then
    __clim_check_dir
  fi
}

__clim_check_dir() {
  local dir="$PWD"
  while [ "$dir" != "/" ] && [ "$dir" != "" ]; do
    if [ -f "$dir/.clim.yaml" ]; then
      local output
      output=$(clim check --file "$dir/.clim.yaml" 2>&1)
      local rc=$?
      if [ $rc -ne 0 ]; then
        echo "$output" | grep -E '^[[:space:]]+[✗⚠]' | head -5
        echo "  Run 'clim check' for details or 'clim import' to install missing tools."
      fi
      return
    fi
    dir=$(dirname "$dir")
  done
}

alias cd='__clim_hook'
`

const hookZsh = `# clim shell hook — auto-check .clim.yaml on cd
autoload -U add-zsh-hook

__clim_check_dir() {
  local dir="$PWD"
  while [[ "$dir" != "/" && -n "$dir" ]]; do
    if [[ -f "$dir/.clim.yaml" ]]; then
      local output
      output=$(clim check --file "$dir/.clim.yaml" 2>&1)
      local rc=$?
      if (( rc != 0 )); then
        echo "$output" | grep -E '^[[:space:]]+[✗⚠]' | head -5
        echo "  Run 'clim check' for details or 'clim import' to install missing tools."
      fi
      return
    fi
    dir=$(dirname "$dir")
  done
}

add-zsh-hook chpwd __clim_check_dir
`

const hookFish = `# clim shell hook — auto-check .clim.yaml on cd
function __clim_check_dir --on-variable PWD
  set -l dir $PWD
  while test "$dir" != /
    if test -f "$dir/.clim.yaml"
      set -l output (clim check --file "$dir/.clim.yaml" 2>&1)
      set -l rc $status
      if test $rc -ne 0
        printf '%s\n' $output | grep -E '^[[:space:]]+[✗⚠]' | head -5
        echo "  Run 'clim check' for details or 'clim import' to install missing tools."
      end
      return
    end
    set dir (dirname "$dir")
  end
end
`

const hookPowerShell = "# clim shell hook \u2014 auto-check .clim.yaml on cd\n" +
	"function __clim_check_dir {\n" +
	"    $dir = $PWD.Path\n" +
	"    while ($dir -and $dir -ne [System.IO.Path]::GetPathRoot($dir)) {\n" +
	"        $candidate = Join-Path $dir \".clim.yaml\"\n" +
	"        if (Test-Path $candidate -PathType Leaf) {\n" +
	"            $output = clim check --file \"$candidate\" 2>&1 | Out-String\n" +
	"            if ($LASTEXITCODE -ne 0) {\n" +
	"                $output -split \"`n\" | Select-String -Pattern '^\\s+[\\u2717\\u26A0]' | Select-Object -First 5 | ForEach-Object { $_.Line }\n" +
	"                Write-Host \"  Run 'clim check' for details or 'clim import' to install missing tools.\"\n" +
	"            }\n" +
	"            return\n" +
	"        }\n" +
	"        $dir = Split-Path $dir -Parent\n" +
	"    }\n" +
	"}\n\n" +
	"# Override Set-Location to trigger check on directory change\n" +
	"$__clim_original_cd = Get-Command Set-Location -CommandType Cmdlet\n" +
	"function Set-Location {\n" +
	"    $prevDir = $PWD.Path\n" +
	"    & $__clim_original_cd @args\n" +
	"    if ($PWD.Path -ne $prevDir) {\n" +
	"        __clim_check_dir\n" +
	"    }\n" +
	"}\n" +
	"Set-Alias -Name cd -Value Set-Location -Option AllScope -Force\n"
