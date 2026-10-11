package src

import (
	"path/filepath"
	"strings"
	"unicode"
)

type hlState struct {
	mode   int  // 0 normal, 1 block comment, 2 multiline string, 5 html tag, 6 html attr string, 7 css (depth in aux), 8 md fence
	delim  rune // string delimiter (mode 2/6)
	triple bool
	aux    int
}

type Lang struct {
	Name        string
	Exts        []string
	LineC       []string
	BlockS      string
	BlockE      string
	Quotes      string
	Backtick    bool
	Triple      bool
	Kw, Ctl     map[string]bool
	Types, Bi   map[string]bool
	Special     string
	NoCase      bool
	Preproc     bool
	Dollar      bool
	Dash        bool
	KeyColon    bool
	ColonIndent bool
	TypeCap     bool
	Icon        Color
	Run         string
}

func set(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

var langs []*Lang

func init() {
	cBlock := func(l *Lang) *Lang { l.BlockS, l.BlockE = "/*", "*/"; return l }
	langs = []*Lang{
		cBlock(&Lang{Name: "Go", Exts: []string{".go"}, LineC: []string{"//"}, Quotes: `"'`, Backtick: true, TypeCap: false, Icon: cKw,
			Kw:    set("func var const type struct interface map chan package import"),
			Ctl:   set("if else for range switch case default return break continue goto go defer select fallthrough"),
			Types: set("int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 uintptr float32 float64 complex64 complex128 string bool byte rune error any"),
			Bi:    set("true false nil iota make new len cap append copy delete panic recover print println close")}),
		{Name: "Python", Exts: []string{".py", ".pyw"}, LineC: []string{"#"}, Quotes: `"'`, Triple: true, ColonIndent: true, Icon: cType,
			Kw:    set("def class lambda global nonlocal None True False and or not in is as with import from pass del async await yield"),
			Ctl:   set("if elif else for while try except finally raise return break continue assert match case"),
			Types: set("int str float bool list dict set tuple bytes object type"),
			Bi:    set("print len range open input enumerate zip map filter sorted sum min max abs isinstance super self cls")},
		cBlock(&Lang{Name: "JavaScript", Exts: []string{".js", ".mjs", ".cjs", ".jsx"}, LineC: []string{"//"}, Quotes: `"'`, Backtick: true, Dollar: true, Icon: cFunc,
			Kw:    set("var let const function class extends new this super typeof instanceof void delete in of async await yield static get set import export from as null undefined true false NaN Infinity"),
			Ctl:   set("if else for while do switch case default return break continue throw try catch finally"),
			Types: set("Object Array String Number Boolean Map Set Promise Date RegExp Error Symbol JSON Math"),
			Bi:    set("console require module exports parseInt parseFloat setTimeout setInterval document window process")}),
		cBlock(&Lang{Name: "TypeScript", Exts: []string{".ts", ".tsx"}, LineC: []string{"//"}, Quotes: `"'`, Backtick: true, Dollar: true, Icon: cKw,
			Kw:    set("var let const function class extends implements new this super typeof instanceof void delete in of async await yield static get set import export from as null undefined true false interface type enum namespace declare readonly public private protected abstract keyof"),
			Ctl:   set("if else for while do switch case default return break continue throw try catch finally"),
			Types: set("string number boolean any unknown never object void Array Map Set Promise Record Partial Date Error"),
			Bi:    set("console require module exports parseInt parseFloat setTimeout setInterval document window process")}),
		cBlock(&Lang{Name: "C", Exts: []string{".c", ".h"}, LineC: []string{"//"}, Quotes: `"'`, Preproc: true, Icon: cKw,
			Kw:    set("struct union enum typedef static extern const volatile register inline sizeof auto unsigned signed NULL true false"),
			Ctl:   set("if else for while do switch case default return break continue goto"),
			Types: set("int char short long float double void size_t bool FILE uint8_t uint16_t uint32_t uint64_t int8_t int16_t int32_t int64_t"),
			Bi:    set("printf scanf malloc free memcpy memset strlen strcmp strcpy puts fopen fclose")}),
		cBlock(&Lang{Name: "C++", Exts: []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"}, LineC: []string{"//"}, Quotes: `"'`, Preproc: true, Icon: cKw,
			Kw:    set("class struct union enum typedef static extern const volatile inline sizeof auto unsigned signed nullptr true false namespace using template typename public private protected virtual override final new delete this operator constexpr noexcept explicit friend mutable"),
			Ctl:   set("if else for while do switch case default return break continue goto try catch throw"),
			Types: set("int char short long float double void bool size_t string vector map set unique_ptr shared_ptr wchar_t"),
			Bi:    set("cout cin endl printf std make_shared make_unique")}),
		cBlock(&Lang{Name: "C#", Exts: []string{".cs"}, LineC: []string{"//"}, Quotes: `"'`, Preproc: true, TypeCap: true, Icon: cType,
			Kw:    set("using namespace class struct interface enum public private protected internal static readonly const new this base override virtual abstract sealed void var null true false get set async await partial ref out in is as typeof"),
			Ctl:   set("if else for foreach while do switch case default return break continue throw try catch finally"),
			Types: set("int long short byte float double decimal bool char string object List Dictionary Task"),
			Bi:    set("Console")}),
		cBlock(&Lang{Name: "Java", Exts: []string{".java"}, LineC: []string{"//"}, Quotes: `"'`, TypeCap: true, Icon: cStr,
			Kw:    set("package import class interface enum extends implements public private protected static final abstract new this super void null true false synchronized volatile transient native instanceof throws"),
			Ctl:   set("if else for while do switch case default return break continue throw try catch finally"),
			Types: set("int long short byte float double boolean char String Object List Map Set"),
			Bi:    set("System")}),
		cBlock(&Lang{Name: "Rust", Exts: []string{".rs"}, LineC: []string{"//"}, Quotes: `"`, Icon: cStr,
			Kw:    set("fn let mut const static struct enum impl trait type use mod pub crate self Self super as where move ref dyn unsafe async await true false"),
			Ctl:   set("if else for while loop match return break continue"),
			Types: set("i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize f32 f64 bool char str String Vec Option Result Box"),
			Bi:    set("println print format vec panic Some None Ok Err")}),
		{Name: "Lua", Exts: []string{".lua"}, LineC: []string{"--"}, BlockS: "--[[", BlockE: "]]", Quotes: `"'`, Icon: cKw,
			Kw:  set("function local end then do nil true false and or not in"),
			Ctl: set("if elseif else for while repeat until return break goto"),
			Bi:  set("print require pairs ipairs tostring tonumber type table string math os io setmetatable getmetatable pcall error assert")},
		{Name: "Ruby", Exts: []string{".rb"}, LineC: []string{"#"}, Quotes: `"'`, Icon: cRed,
			Kw:  set("def class module end self nil true false and or not begin rescue ensure do lambda proc yield require include attr_accessor attr_reader"),
			Ctl: set("if elsif else unless while until for case when then return break next redo retry"),
			Bi:  set("puts print p require_relative")},
		cBlock(&Lang{Name: "PHP", Exts: []string{".php"}, LineC: []string{"//", "#"}, Quotes: `"'`, Dollar: true, Icon: cCtl,
			Kw:  set("function class extends implements new public private protected static const echo namespace use null true false array abstract interface trait"),
			Ctl: set("if else elseif for foreach while do switch case default return break continue try catch finally throw"),
			Bi:  set("isset empty die print_r var_dump strlen count")}),
		{Name: "Shell", Exts: []string{".sh", ".bash", ".zsh"}, LineC: []string{"#"}, Quotes: `"'`, Dollar: true, Icon: cNum,
			Kw:  set("function local export readonly declare echo cd source alias unset"),
			Ctl: set("if then else elif fi for while until do done case esac in return exit break continue"),
			Bi:  set("ls cat grep sed awk mkdir rm cp mv chmod chown find sort uniq head tail curl wget")},
		{Name: "Batch", Exts: []string{".bat", ".cmd"}, Special: "bat", NoCase: true, Quotes: `"`, Icon: cNum,
			Kw:  set("echo set setlocal endlocal call cd chdir copy del dir md mkdir move rd rmdir ren rename type cls title pause color start path prompt exit errorlevel not exist defined"),
			Ctl: set("if else for goto in do rem")},
		{Name: "PowerShell", Exts: []string{".ps1", ".psm1", ".psd1"}, LineC: []string{"#"}, BlockS: "<#", BlockE: "#>", Quotes: `"'`, Dollar: true, Dash: true, NoCase: true, Icon: cKw,
			Kw:  set("function param begin process end filter workflow class using $true $false $null"),
			Ctl: set("if else elseif switch for foreach while do until return break continue throw try catch finally in"),
			Bi:  set("write-host write-output get-childitem set-location get-content set-content remove-item new-item")},
		{Name: "SQL", Exts: []string{".sql"}, LineC: []string{"--"}, BlockS: "/*", BlockE: "*/", Quotes: `"'`, NoCase: true, Icon: cStr,
			Kw:    set("select from where insert into values update set delete create table drop alter add index view join inner left right outer on as and or not null is in like between group by order having limit offset union distinct primary key foreign references default"),
			Ctl:   set("case when then else end"),
			Types: set("int integer varchar char text date datetime timestamp boolean float double decimal"),
			Bi:    set("count sum avg min max coalesce now")},
		{Name: "HTML", Exts: []string{".html", ".htm", ".xhtml", ".vue", ".svelte"}, Special: "html", Icon: cStr},
		{Name: "XML", Exts: []string{".xml", ".svg", ".xaml", ".csproj", ".config", ".plist"}, Special: "html", Icon: cStr},
		{Name: "CSS", Exts: []string{".css", ".scss", ".less"}, Special: "css", Icon: cKw},
		{Name: "JSON", Exts: []string{".json", ".jsonc", ".webmanifest"}, Quotes: `"`, KeyColon: true, Kw: set("true false null"), Icon: cFunc},
		{Name: "Markdown", Exts: []string{".md", ".markdown"}, Special: "md", Icon: cKw},
		{Name: "YAML", Exts: []string{".yml", ".yaml"}, LineC: []string{"#"}, Quotes: `"'`, KeyColon: true, Dash: true, Kw: set("true false null yes no"), Icon: cRed},
		{Name: "INI", Exts: []string{".ini", ".cfg", ".conf", ".toml", ".properties", ".env"}, Special: "ini", LineC: []string{";", "#"}, Icon: cDim},
		{Name: "VBScript", Exts: []string{".vbs", ".bas", ".vb"}, LineC: []string{"'"}, Quotes: `"`, NoCase: true, Icon: cKw,
			Kw:  set("dim set sub function end class const private public new nothing true false and or not mod option explicit call byval byref"),
			Ctl: set("if then else elseif for each next while wend do loop until select case exit return"),
			Bi:  set("msgbox wscript createobject len mid left right")},
		{Name: "Makefile", Exts: []string{".mk"}, LineC: []string{"#"}, Quotes: `"'`, Dollar: true, Icon: cNum,
			Kw: set("ifeq ifneq else endif include define endef export"), Ctl: set("")},
		{Name: "Plain Text", Exts: []string{".txt", ".log", ".text"}, Icon: cDim},
	}
}

func langByName(n string) *Lang {
	for _, l := range langs {
		if l.Name == n {
			return l
		}
	}
	return langs[len(langs)-1]
}

func langForFile(path string) *Lang {
	base := strings.ToLower(filepath.Base(path))
	if base == "makefile" || base == "gnumakefile" {
		return langByName("Makefile")
	}
	if base == ".gitignore" || base == ".env" {
		return langByName("INI")
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, l := range langs {
		for _, e := range l.Exts {
			if e == ext {
				return l
			}
		}
	}
	return langs[len(langs)-1]
}

func hasAt(line []rune, i int, s string) bool {
	sr := []rune(s)
	if i+len(sr) > len(line) {
		return false
	}
	for k, r := range sr {
		if line[i+k] != r {
			return false
		}
	}
	return true
}

func paint(out []Color, a, b int, c Color) {
	for i := a; i < b && i < len(out); i++ {
		out[i] = c
	}
}

func isIdentStart(r rune, l *Lang) bool {
	return r == '_' || unicode.IsLetter(r) || (l.Dollar && r == '$')
}
func isIdentPart(r rune, l *Lang) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || (l.Dollar && r == '$') || (l.Dash && r == '-')
}

// scan colours one line and returns the state at the start of the next line.
func (l *Lang) scan(line []rune, st hlState, out []Color) hlState {
	for i := range out {
		out[i] = cFg
	}
	switch l.Special {
	case "html":
		return scanHTML(line, st, out)
	case "css":
		return scanCSS(line, st, out)
	case "md":
		return scanMD(line, st, out)
	case "ini":
		scanINI(l, line, out)
		return st
	case "bat":
		scanBat(l, line, out)
		return st
	}
	n := len(line)
	i := 0
	// continuing multi-line constructs
	if st.mode == 1 {
		j := indexRunes(line, 0, l.BlockE)
		if j < 0 {
			paint(out, 0, n, cComment)
			return st
		}
		paint(out, 0, j+len([]rune(l.BlockE)), cComment)
		i = j + len([]rune(l.BlockE))
		st.mode = 0
	} else if st.mode == 2 {
		end, closed := scanString(line, 0, st.delim, st.triple)
		paint(out, 0, end, cStr)
		if !closed {
			return st
		}
		i = end
		st.mode = 0
	}
	for i < n {
		r := line[i]
		switch {
		case r == ' ' || r == '\t':
			i++
			continue
		}
		// line comment
		matched := false
		for _, lc := range l.LineC {
			if hasAt(line, i, lc) {
				paint(out, i, n, cComment)
				return st
			}
		}
		if matched {
			break
		}
		if l.BlockS != "" && hasAt(line, i, l.BlockS) {
			bs := len([]rune(l.BlockS))
			j := indexRunes(line, i+bs, l.BlockE)
			if j < 0 {
				paint(out, i, n, cComment)
				st.mode = 1
				return st
			}
			e := j + len([]rune(l.BlockE))
			paint(out, i, e, cComment)
			i = e
			continue
		}
		if l.Preproc && r == '#' && strings.TrimSpace(string(line[:i])) == "" {
			j := i + 1
			for j < n && (unicode.IsLetter(line[j]) || line[j] == ' ') {
				j++
			}
			paint(out, i, j, cCtl)
			i = j
			continue
		}
		// strings
		if strings.ContainsRune(l.Quotes, r) || (l.Backtick && r == '`') {
			if l.Name == "Rust" && r == '\'' {
				i++
				continue
			}
			triple := l.Triple && hasAt(line, i, string([]rune{r, r, r}))
			start := i
			if triple {
				end, closed := scanString(line, i+3, r, true)
				paint(out, start, end, cStr)
				if !closed {
					st.mode, st.delim, st.triple = 2, r, true
					return st
				}
				i = end
				continue
			}
			end, closed := scanString(line, i+1, r, false)
			col := cStr
			if l.KeyColon && closed {
				k := end
				for k < n && line[k] == ' ' {
					k++
				}
				if k < n && line[k] == ':' {
					col = cVar
				}
			}
			paint(out, start, end, col)
			if !closed && r == '`' {
				st.mode, st.delim, st.triple = 2, r, false
				return st
			}
			i = end
			continue
		}
		// numbers
		if unicode.IsDigit(r) || (r == '.' && i+1 < n && unicode.IsDigit(line[i+1]) && (i == 0 || !isIdentPart(line[i-1], l))) {
			if i == 0 || !isIdentPart(line[i-1], l) {
				j := i + 1
				for j < n && (unicode.IsLetter(line[j]) || unicode.IsDigit(line[j]) || line[j] == '.' || line[j] == '_') {
					j++
				}
				paint(out, i, j, cNum)
				i = j
				continue
			}
		}
		if isIdentStart(r, l) {
			j := i + 1
			for j < n && isIdentPart(line[j], l) {
				j++
			}
			// trailing dash from Dash langs (e.g. "a -")
			for j > i+1 && line[j-1] == '-' {
				j--
			}
			w := string(line[i:j])
			key := w
			if l.NoCase {
				key = strings.ToLower(w)
			}
			k := j
			for k < n && line[k] == ' ' {
				k++
			}
			var col Color = cVar
			switch {
			case l.Ctl[key]:
				col = cCtl
			case l.Kw[key]:
				col = cKw
			case l.Types[key]:
				col = cType
			case l.Bi[key]:
				col = cFunc
			case l.KeyColon && k < n && line[k] == ':':
				col = cVar
			case k < n && line[k] == '(':
				col = cFunc
			case l.Dash && strings.Contains(w, "-") && unicode.IsUpper(line[i]):
				col = cFunc
			case l.TypeCap && unicode.IsUpper(line[i]):
				col = cType
			case i > 0 && line[i-1] == '$':
				col = cVar
			case l.Name == "Go" && unicode.IsUpper(line[i]) && k < n && (line[k] == '{' || line[k] == ')' || line[k] == ',' || line[k] == ']'):
				col = cType
			}
			if l.Dollar && line[i] == '$' && l.Name != "JavaScript" && l.Name != "TypeScript" {
				col = cVar
			}
			paint(out, i, j, col)
			i = j
			continue
		}
		i++
	}
	return st
}

func indexRunes(line []rune, from int, s string) int {
	for i := from; i < len(line); i++ {
		if hasAt(line, i, s) {
			return i
		}
	}
	return -1
}

// scanString returns end index (exclusive) and whether the closing delimiter was found.
func scanString(line []rune, i int, delim rune, triple bool) (int, bool) {
	n := len(line)
	for i < n {
		c := line[i]
		if c == '\\' && delim != '`' {
			i += 2
			continue
		}
		if triple {
			if c == delim && i+2 < n+0 && hasAt(line, i, string([]rune{delim, delim, delim})) {
				return i + 3, true
			}
			if c == delim && i+2 == n-0 && false {
				return n, true
			}
		} else if c == delim {
			return i + 1, true
		}
		i++
	}
	if i > n {
		i = n
	}
	return i, false
}

func scanINI(l *Lang, line []rune, out []Color) {
	s := strings.TrimSpace(string(line))
	if s == "" {
		return
	}
	if s[0] == ';' || s[0] == '#' {
		paint(out, 0, len(line), cComment)
		return
	}
	if s[0] == '[' {
		paint(out, 0, len(line), cType)
		return
	}
	for i, r := range line {
		if r == '=' || r == ':' {
			paint(out, 0, i, cVar)
			paint(out, i+1, len(line), cStr)
			return
		}
	}
}

var batCtl = map[string]bool{"if": true, "else": true, "for": true, "goto": true, "call": true, "exit": true, "in": true, "do": true}

func scanBat(l *Lang, line []rune, out []Color) {
	s := strings.ToLower(strings.TrimSpace(string(line)))
	if strings.HasPrefix(s, "rem ") || s == "rem" || strings.HasPrefix(s, "::") || strings.HasPrefix(s, "@rem") {
		paint(out, 0, len(line), cComment)
		return
	}
	if strings.HasPrefix(s, ":") {
		paint(out, 0, len(line), cFunc)
		return
	}
	n := len(line)
	for i := 0; i < n; {
		r := line[i]
		switch {
		case r == '"':
			e, _ := scanString(line, i+1, '"', false)
			paint(out, i, e, cStr)
			i = e
		case r == '%' || r == '!':
			j := i + 1
			for j < n && line[j] != r && line[j] != ' ' {
				j++
			}
			if j < n && line[j] == r {
				j++
			}
			paint(out, i, j, cVar)
			i = j
		case unicode.IsLetter(r):
			j := i
			for j < n && (unicode.IsLetter(line[j]) || unicode.IsDigit(line[j])) {
				j++
			}
			w := strings.ToLower(string(line[i:j]))
			if batCtl[w] {
				paint(out, i, j, cCtl)
			} else if l.Kw[w] {
				paint(out, i, j, cKw)
			}
			i = j
		case unicode.IsDigit(r):
			paint(out, i, i+1, cNum)
			i++
		default:
			i++
		}
	}
}

func scanHTML(line []rune, st hlState, out []Color) hlState {
	n := len(line)
	i := 0
	for i < n {
		switch st.mode {
		case 1:
			j := indexRunes(line, i, "-->")
			if j < 0 {
				paint(out, i, n, cComment)
				return st
			}
			paint(out, i, j+3, cComment)
			i = j + 3
			st.mode = 0
		case 6:
			e, closed := scanString(line, i, st.delim, false)
			paint(out, i, e, cStr)
			i = e
			if closed {
				st.mode = 5
			}
		case 5:
			r := line[i]
			switch {
			case r == '>':
				paint(out, i, i+1, cDim)
				st.mode = 0
				i++
			case r == '/' || r == '?':
				paint(out, i, i+1, cDim)
				i++
			case r == '"' || r == '\'':
				e, closed := scanString(line, i+1, r, false)
				paint(out, i, e, cStr)
				i = e
				if !closed {
					st.mode, st.delim = 6, r
				}
			case unicode.IsLetter(r) || r == '_' || r == ':':
				j := i
				for j < n && (unicode.IsLetter(line[j]) || unicode.IsDigit(line[j]) || strings.ContainsRune("-_:.", line[j])) {
					j++
				}
				paint(out, i, j, cVar)
				i = j
			default:
				i++
			}
		default:
			if line[i] == '<' {
				if hasAt(line, i, "<!--") {
					st.mode = 1
					paint(out, i, i+4, cComment)
					i += 4
					continue
				}
				j := i + 1
				for j < n && (line[j] == '/' || line[j] == '!' || line[j] == '?') {
					j++
				}
				paint(out, i, j, cDim)
				k := j
				for k < n && (unicode.IsLetter(line[k]) || unicode.IsDigit(line[k]) || strings.ContainsRune("-_:.", line[k])) {
					k++
				}
				paint(out, j, k, cKw)
				st.mode = 5
				i = k
			} else if line[i] == '&' {
				j := i + 1
				for j < n && line[j] != ';' && line[j] != ' ' && j-i < 10 {
					j++
				}
				if j < n && line[j] == ';' {
					paint(out, i, j+1, cNum)
					i = j + 1
				} else {
					i++
				}
			} else {
				i++
			}
		}
	}
	return st
}

func scanCSS(line []rune, st hlState, out []Color) hlState {
	n := len(line)
	i := 0
	depth := st.aux
	inVal := false
	if st.mode == 1 {
		j := indexRunes(line, 0, "*/")
		if j < 0 {
			paint(out, 0, n, cComment)
			return st
		}
		paint(out, 0, j+2, cComment)
		i = j + 2
		st.mode = 0
	}
	for i < n {
		r := line[i]
		switch {
		case hasAt(line, i, "/*"):
			j := indexRunes(line, i+2, "*/")
			if j < 0 {
				paint(out, i, n, cComment)
				st.mode = 1
				st.aux = depth
				return st
			}
			paint(out, i, j+2, cComment)
			i = j + 2
		case r == '{':
			depth++
			inVal = false
			i++
		case r == '}':
			if depth > 0 {
				depth--
			}
			inVal = false
			i++
		case r == ';':
			inVal = false
			i++
		case r == ':' && depth > 0 && !inVal:
			inVal = true
			i++
		case r == '"' || r == '\'':
			e, _ := scanString(line, i+1, r, false)
			paint(out, i, e, cStr)
			i = e
		case r == '@':
			j := i + 1
			for j < n && (unicode.IsLetter(line[j]) || line[j] == '-') {
				j++
			}
			paint(out, i, j, cCtl)
			i = j
		case r == '#' && (depth > 0 && inVal):
			j := i + 1
			for j < n && (unicode.IsDigit(line[j]) || (line[j] >= 'a' && line[j] <= 'f') || (line[j] >= 'A' && line[j] <= 'F')) {
				j++
			}
			paint(out, i, j, cNum)
			i = j
		case unicode.IsDigit(r) || (r == '.' && i+1 < n && unicode.IsDigit(line[i+1]) && inVal):
			j := i + 1
			for j < n && (unicode.IsDigit(line[j]) || unicode.IsLetter(line[j]) || line[j] == '.' || line[j] == '%') {
				j++
			}
			paint(out, i, j, cNum)
			i = j
		case unicode.IsLetter(r) || r == '-' || r == '_' || r == '.' || r == '#':
			j := i + 1
			for j < n && (unicode.IsLetter(line[j]) || unicode.IsDigit(line[j]) || line[j] == '-' || line[j] == '_') {
				j++
			}
			var col Color = cFg
			switch {
			case depth > 0 && !inVal:
				col = cVar
			case depth > 0 && inVal:
				if j < n && line[j] == '(' {
					col = cFunc
				}
			case r == '.' || r == '#':
				col = cFunc
			default:
				col = cKw
			}
			paint(out, i, j, col)
			i = j
		default:
			i++
		}
	}
	st.aux = depth
	return st
}

func scanMD(line []rune, st hlState, out []Color) hlState {
	s := string(line)
	t := strings.TrimLeft(s, " ")
	if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
		paint(out, 0, len(line), cStr)
		if st.mode == 8 {
			st.mode = 0
		} else {
			st.mode = 8
		}
		return st
	}
	if st.mode == 8 {
		paint(out, 0, len(line), cStr)
		return st
	}
	if strings.HasPrefix(t, "#") {
		paint(out, 0, len(line), cKw)
		return st
	}
	if strings.HasPrefix(t, ">") {
		paint(out, 0, len(line), cComment)
		return st
	}
	off := len(line) - len([]rune(t))
	if len(t) > 1 && (t[0] == '-' || t[0] == '*' || t[0] == '+') && t[1] == ' ' {
		paint(out, off, off+1, cCtl)
	} else {
		k := 0
		for k < len(t) && t[k] >= '0' && t[k] <= '9' {
			k++
		}
		if k > 0 && k+1 < len(t) && t[k] == '.' && t[k+1] == ' ' {
			paint(out, off, off+k+1, cCtl)
		}
	}
	n := len(line)
	for i := 0; i < n; i++ {
		switch line[i] {
		case '`':
			j := i + 1
			for j < n && line[j] != '`' {
				j++
			}
			if j < n {
				paint(out, i, j+1, cStr)
				i = j
			}
		case '*', '_':
			if i+1 < n && line[i+1] == line[i] {
				j := indexRunes(line, i+2, string([]rune{line[i], line[i]}))
				if j > 0 {
					paint(out, i, j+2, cFunc)
					i = j + 1
				}
			}
		case '[':
			j := i + 1
			for j < n && line[j] != ']' {
				j++
			}
			if j+1 < n && line[j+1] == '(' {
				e := j + 2
				for e < n && line[e] != ')' {
					e++
				}
				paint(out, i, j+1, cType)
				if e < n {
					paint(out, j+1, e+1, cDim)
					i = e
				}
			}
		}
	}
	return st
}
