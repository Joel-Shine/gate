<div align="center">
    
# GATE

**One-stop solution for your file searching nightmares!**

</div>

## ✨ Features
* **Fuzzy Jumping:** Type a partial name, hit enter, and instantly teleport to the file or directory.
* **Frecency Engine:** The more you visit a path, the higher it scores. Top-scoring paths bypass the menu and auto-jump.
* **Content Sniper:** Use the `-c` flag to search inside text files to find exact strings.
* **Extension Filtering:** Use the `-e` flag to strictly filter searches by file type.
* **Instant Previews:** Directories show a peek of their contents; files display their size.
* **Cross-Platform:** Works seamlessly on Linux, macOS, and Windows.

---

## 🖼️ Demo
![Demo for gate](https://github.com/Joel-Shine/gate/blob/main/g-demo.png)

## 🚀 Installation & Setup

Because `g` changes your active terminal directory, it requires two parts to work: the compiled binary, and a tiny shell wrapper.

### Step 1: Compile the Binary
Clone this repository and compile the Go code.
```bash
go build -o g main.go
```

### Step 2: Add the Shell Wrapper
A child process (the Go binary) cannot change the directory of a parent process (your terminal). To fix this, add the following wrapper to your shell profile. It captures the path found by G.A.T.E. and uses native OS commands to jump you there.

**For macOS / Linux (Add to `~/.bashrc` or `~/.zshrc`):**
```bash
g() {
    local res
    # UPDATE THIS PATH to wherever you saved the compiled binary!
    res=$(~/scripts/g "$@")
    
    if [ -z "$res" ]; then return; fi

    local target_dir=""
    if [ -d "$res" ]; then target_dir="$res"
    elif [ -f "$res" ]; then target_dir=$(dirname "$res")
    fi

    if [ -n "$target_dir" ]; then
        cd "$target_dir" || return
        
        # Skip opening the GUI if we just used 'back' or '..'
        if [[ "$1" == "back" || "$1" == ".." ]]; then return; fi
        
        # Open in native file manager
        if [[ "$OSTYPE" == "darwin"* ]]; then open .
        elif [[ "$OSTYPE" == "linux-gnu"* ]]; then xdg-open .
        fi
    fi
}
```

**For Windows (Add to your PowerShell `$PROFILE`):**
```powershell
function g {
    # UPDATE THIS PATH to wherever you saved g.exe!
    $res = C:\path\to\your\g.exe $args
    
    if (-not $res) { return }

    $targetDir = ""
    if (Test-Path -Path $res -PathType Container) { $targetDir = $res } 
    elseif (Test-Path -Path $res -PathType Leaf) { $targetDir = Split-Path -Path $res -Parent }

    if ($targetDir) {
        Set-Location $targetDir
        
        if ($args[0] -eq "back" -or $args[0] -eq "..") { return }
        Invoke-Item .
    }
}
```
*Restart your terminal or source your profile, and you are ready to jump!*

---

## 📖 Command Reference

The default search depth is **5**. If you need to dig deeper, provide a number as the very last argument. Wrap multi-word search targets in `"double quotes"`.

| Command Combo | What it does | Example |
| :--- | :--- | :--- |
| `g <target>` | Fuzzy search files & folders. | `g config` |
| `g <target> <depth>` | Search with a custom depth limit. | `g project 8` |
| `g -e <exts> <target>` | Filter strictly by file extensions (comma separated). | `g -e html,css,js index` |
| `g -c <text>` | Search inside file contents for a specific string. | `g -c auth` |
| `g -c <text> <depth>` | Content search with a custom depth limit. | `g -c "api key" 3` |
| `g -c -e <exts> <text>` | Content search restricted to specific extensions. | `g -c -e go,py main` |
| `g back` | Jump seamlessly to your previous directory. | `g back` |
| `g ..` | Go up one parent directory level. | `g ..` |
| `g -help` | Print the in-terminal help table. | `g -help` |
