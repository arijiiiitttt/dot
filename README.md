<div align="center">

<pre>
██████╗  ██████╗ ████████╗
██╔══██╗██╔═══██╗╚══██╔══╝
██║  ██║██║   ██║   ██║   
██║  ██║██║   ██║   ██║   
██████╔╝╚██████╔╝   ██║   
╚═════╝  ╚═════╝    ╚═╝   
</pre>

</div>

<p align="center">
  <b>dot</b>  a VS Code style code editor that runs in your <b>terminal</b>
</p>

<p align="center">
  <a href="#run-it"><img alt="Go" src="https://img.shields.io/badge/go-1.20-00ADD8?style=flat-square&logo=go&logoColor=white" /></a>
  <a href="#architecture"><img alt="Dependencies" src="https://img.shields.io/badge/dependencies-none-2ea44f?style=flat-square" /></a>
  <a href="#run-it"><img alt="Windows" src="https://img.shields.io/badge/windows-7%20%E2%86%92%2011-0078D6?style=flat-square&logo=windows&logoColor=white" /></a>
  <a href="#file-icons"><img alt="Icons" src="https://img.shields.io/badge/icons-vscode--icons-007ACC?style=flat-square" /></a>
</p>


> **⚠️ Honest framing.** Dot is a small, fast, single file terminal editor, not a replacement for VS Code. It has no plugins, no debugger, no language server and no git UI. The Windows builds are cross compiled; it is developed and tested on Linux in a pseudo terminal, so please report anything odd on your Windows version.


## Table of contents

1. [Run it](#run-it)
2. [Using Dot](#using-dot)
3. [LazyVim-style menus](#lazyvim-style-menus)
4. [Terminal panel](#terminal-panel)
5. [File icons](#file-icons)
6. [Colours and old Windows](#colours-and-old-windows)
7. [Architecture](#architecture)
8. [Building](#building)
9. [Extending](#extending)
10. [Troubleshooting](#troubleshooting)
11. [Credits](#credits)



## Run it

You need **Go 1.20.x**: the last Go version whose programs still run on Windows 7/8. On Windows 10/11 any newer Go also works, but the `.exe` will then need Windows 10+.

```bat
:: 1. check Go
go version

:: 2. go to the project folder (the one with go.mod)
cd Dot

:: 3a. run straight from source
go run .

:: 3b. ...or open a folder / file
go run . C:\Projects\my-app
go run . main.go

:: 4. build an exe and start it
go build -o dot.exe .
dot.exe
```

Run it in a real console (**cmd**, **PowerShell** or **Windows Terminal**), not inside another editor's output pane. Quit with **Ctrl+Q**.

### Install so that `.dot` works everywhere

```bat
build.bat        :: builds bin64\dot.exe and bin32\dot.exe
install.bat      :: copies Dot to %LOCALAPPDATA%\Dot, adds it to PATH, installs the icon font
```

Open a **new** terminal and type:

```bat
.dot                 :: open the current folder
.dot file.py         :: open a file
.dot C:\Projects\x   :: open a folder
```

| Setup | Result |
| --- | --- |
| Windows 10 1703+ / Windows Terminal | True-colour VS Code Dark+ |
| Windows 7 / 8 | Classic console API with a redefined 16-colour palette |
| Icon font not installed | Coloured squares instead of file icons |



## Using Dot

| Key | What it does |
| --- | --- |
| **Ctrl+N / Ctrl+S / Ctrl+W** | New file / save / close tab |
| **Ctrl+Z / Ctrl+Y** | Undo / redo |
| **Ctrl+C / X / V** | Copy / cut / paste (Windows clipboard) |
| **Ctrl+F / Ctrl+H** | Find / replace (case, whole word, regex) |
| **Ctrl+Shift+F** | Search in files |
| **Ctrl+G** | Go to line |
| **Ctrl+/** | Toggle line comment |
| **Alt+↑ / Alt+↓** | Move line |
| **Ctrl+Shift+K** | Delete line |
| **Tab / Shift+Tab** | Indent / outdent |
| **Ctrl+B / Ctrl+J** | Toggle side bar / panel |
| **Ctrl+PgUp / PgDn** | Switch tab |
| **F5 / Shift+F5** | Run current file / stop |
| **Ctrl+,** | Settings |
| **Ctrl+Q** | Quit |

**Things to try**

- Press **Ctrl+P**, type part of a file name: live preview on the right
- Press **F5** on a `.py` / `.js` / `.go` / `.bat` file: output appears in the panel, and anything you type goes to the program
- Drag the panel header with the mouse to resize it
- Click the menu bar (File, Edit, …) or right-click in the editor or explorer

Syntax colours: Go, Python, JavaScript, TypeScript, C, C++, C#, Java, Rust, Lua, Ruby, PHP, Shell, Batch, PowerShell, SQL, HTML, XML, CSS, JSON, YAML, INI/TOML, Markdown, VBScript, Makefile.



## LazyVim-style menus

| Key | Opens |
| --- | --- |
| **Ctrl+P** | Telescope-style finder: results above, prompt below, file preview on the right |
| **Ctrl+P**, then `>` | Commands |
| **Ctrl+P**, then `?` | Searchable list of every key binding |
| **Ctrl+P**, then `:` | Go to line |
| **Ctrl+Shift+P / F1 / Ctrl+Space / F4** | **Which-key** leader menu |

In the which-key menu press the letters shown:

| Keys | Action |
| --- | --- |
| `SPC` | Find file |
| `/` | Search in files |
| `e` | Explorer |
| `p` | Command palette |
| `,` | Settings |
| `` ` `` | Terminal |
| `r` | Run file |
| `f` `s` | **f**ile → **s**ave |
| `s` `g` | **s**earch → **g**rep |
| `b` `d` | **b**uffer → close |
| `u` `m` | **u**i → **m**aximize panel |
| `q` `q` | Quit |

`Backspace` goes back one level, `Esc` closes.



## Terminal panel

`` Ctrl+` `` toggles it. **OUTPUT** shows the result of **F5**, **TERMINAL** runs commands (`cmd` or PowerShell on Windows; `cd`, `cls` and `exit` are built in).

| Action | How |
| --- | --- |
| Resize | Drag the OUTPUT / TERMINAL header bar |
| Maximize / restore | Double-click the header, or **Ctrl+Shift+M** |
| Bigger / smaller | **Ctrl+Alt+↑ / Ctrl+Alt+↓** |
| Side bar width | Drag its right edge |
| Stop a running program | **Ctrl+C** (in the panel) or **Shift+F5** |
| Back to the editor | **Esc** |

Programs run with piped input and output, not a real TTY: interactive full-screen programs (REPLs, `vim`, …) will not work, but scripts that read lines from stdin do.

Run commands per file type (the language must be installed on the machine):

| Extension | Command |
| --- | --- |
| `.py` | `python` / `py` |
| `.js` | `node` |
| `.go` | `go run` |
| `.bat` `.cmd` | `cmd` |
| `.ps1` | `powershell` |
| `.lua` `.rb` `.php` `.vbs` | `lua` / `ruby` / `php` / `cscript` |
| `.java` | `java` |
| `.c` `.cpp` | `gcc` / `g++`, then run |
| `.html` | opens in the browser |



## File icons

A terminal can only draw text, so the real [vscode-icons](https://github.com/vscode-icons/vscode-icons) SVGs are packed into a **colour font** (`font/DotIcons.ttf`, ~1,240 icons) by `tools/gen_icons.py`.

1. `install.bat` installs the font (Windows 10 1809+ per-user install).
2. In **Windows Terminal** → *Settings → Profiles → Defaults → Appearance* set **Font face** to:

   ```
   Cascadia Mono, DotIcons
   ```

3. Restart the terminal.

Settings → *File icons* cycles **auto / on / off**. Without the font Dot shows coloured squares. Windows 7/8 consoles cannot show icon fonts.

Regenerate the font (needs Python and `pip install fonttools`):

```bash
git clone --depth 1 https://github.com/vscode-icons/vscode-icons
python tools/gen_icons.py vscode-icons out
# writes out/DotIcons.ttf and out/icons_data.go  (copy icons_data.go over the one in the repo root)
```



## Colours and old Windows

- **Windows 10 1703+ / Windows Terminal** → ANSI true-colour (VS Code Dark+ exact).
- **Windows 7 / 8** → the classic console API (`WriteConsoleOutputW`) with a redefined 16-colour palette, so the same colours appear without ANSI support.
- Force a mode: `.dot --vt` or `.dot --legacy`.
- Window too small? **Ctrl+Shift+P** → `u` `w` for size presets (classic console only).



## Architecture

No third-party Go modules: only the standard library and raw Win32 calls.

| File | Role |
| --- | --- |
| `main.go` | Arguments, start-up, main loop |
| `screen.go` | Palette, screen buffer, ANSI renderer |
| `term_windows.go` | Win32 console: output, keyboard, mouse, clipboard |
| `term_linux.go` / `term_other.go` | Linux backend (used for testing) / stub |
| `doc.go` | Text buffer, editing, undo, find |
| `syntax.go` | Languages and highlighting |
| `app.go` | Commands, key bindings, mouse |
| `draw.go` | Menu bar, explorer, tabs, editor, panel, status bar |
| `widgets.go` | Inputs, dialogs, menus |
| `lazy.go` | Telescope-style finder, which-key menu, Settings |
| `explorer.go` | File tree and search in files |
| `panel.go` | Output / terminal panel and process running |
| `icons.go` / `icons_data.go` | File icon lookup (data is generated) |
| `proc_windows.go` / `proc_other.go` | Hidden-window process start, kill tree |
| `tools/gen_icons.py` | SVG → colour font generator |



## Building

```bat
build.bat
```

or by hand:

```bat
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags="-s -w" -o bin64\dot.exe .
set GOARCH=386
go build -trimpath -ldflags="-s -w" -o bin32\dot.exe .
```

Cross-compiling from Linux/macOS: `GOOS=windows GOARCH=amd64 go build -o dot.exe .`



## Extending

- **New command** — `add("id", "Title", func(a *App) {...})` in `app.go`, then bind a key in the `keymap` table.
- **Leader menu entry** — add to `leaderTree` in `lazy.go`.
- **New language** — add a `*Lang` to the list in `syntax.go` (extensions, comment markers, keyword sets).
- **Run command for a file type** — `stepsFor` in `panel.go`.
- **Setting** — add an item in `openSettings` in `lazy.go`.



## Troubleshooting

| Problem | Fix |
| --- | --- |
| `.dot` is not recognized | Open a **new** terminal after `install.bat`; or use `dot`; or add `%LOCALAPPDATA%\Dot` to PATH |
| "must be started from a real console" | Run it from cmd / PowerShell / Windows Terminal |
| Colours look wrong | `.dot --legacy` (old consoles) or `.dot --vt` (Windows Terminal) |
| Icons are squares | Install `font\DotIcons.ttf`, set font face `Cascadia Mono, DotIcons`, restart the terminal |
| Terminal panel shows nothing | Update to the latest build (older builds drew output in the background colour) |
| `bun` / `npm` / `python` not found in the panel | The tool must be on your `PATH` |



## Credits

File icons are from [vscode-icons](https://github.com/vscode-icons/vscode-icons) (icons: CC BY-SA 4.0), packed into `DotIcons.ttf`. The look is inspired by VS Code (Dark+) and LazyVim / Telescope.
