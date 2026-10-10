//go:build linux

package linux

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"

	ui "github.com/banditmoscow1337/nativeForms"
)

type composeNode struct {
	next map[uint32]*composeNode
	text string
}
type composeState struct{ node *composeNode }

var composeOnce sync.Once
var composeRoot composeNode
var composeKey = regexp.MustCompile(`<([^>]+)>`)
var composeText = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)

func loadCompose() {
	composeOnce.Do(func() {
		composeRoot.next = make(map[uint32]*composeNode)
		locale := ""
		for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
			if locale = os.Getenv(key); locale != "" {
				break
			}
		}
		system := "/usr/share/X11/locale"
		local := filepath.Join(system, "en_US.UTF-8", "Compose")
		if f, err := os.Open(filepath.Join(system, "compose.dir")); err == nil {
			s := bufio.NewScanner(f)
			for s.Scan() {
				fields := strings.Fields(strings.SplitN(s.Text(), "#", 2)[0])
				if len(fields) >= 2 && fields[1] == locale {
					local = filepath.Join(system, strings.TrimSuffix(fields[0], ":"))
					break
				}
			}
			f.Close()
		}
		home, _ := os.UserHomeDir()
		root := os.Getenv("XCOMPOSEFILE")
		if root == "" {
			root = filepath.Join(home, ".XCompose")
			if _, e := os.Stat(root); e != nil {
				root = local
			}
		}
		seen := map[string]bool{}
		total := 0
		var read func(string, int)
		read = func(path string, depth int) {
			if depth > 8 || seen[path] || total > 32<<20 {
				return
			}
			seen[path] = true
			f, e := os.Open(path)
			if e != nil {
				return
			}
			defer f.Close()
			info, e := f.Stat()
			if e != nil || info.Size() > 8<<20 {
				return
			}
			total += int(info.Size())
			s := bufio.NewScanner(f)
			s.Buffer(make([]byte, 4096), 64<<10)
			for s.Scan() {
				line := strings.TrimSpace(s.Text())
				if line == "" || line[0] == '#' {
					continue
				}
				if strings.HasPrefix(line, "include") {
					q := composeText.FindString(line)
					p, e := strconv.Unquote(q)
					if e == nil {
						p = strings.NewReplacer("%H", home, "%L", local, "%S", system, "%%", "%").Replace(p)
						if !filepath.IsAbs(p) {
							p = filepath.Join(filepath.Dir(path), p)
						}
						read(p, depth+1)
					}
					continue
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				matches := composeKey.FindAllStringSubmatch(parts[0], -1)
				if len(matches) < 2 || len(matches) > 8 {
					continue
				}
				value, e := strconv.Unquote(composeText.FindString(parts[1]))
				if e != nil {
					continue
				}
				node := &composeRoot
				valid := true
				for _, m := range matches {
					sym := keysym(m[1])
					if sym == 0 {
						valid = false
						break
					}
					if node.next == nil {
						node.next = map[uint32]*composeNode{}
					}
					if node.next[sym] == nil {
						node.next[sym] = &composeNode{}
					}
					node = node.next[sym]
				}
				if valid {
					node.text = value
				}
			}
		}
		read(root, 0)
	})
}
func (c *composeState) reset(m *ui.Manager) {
	if c.node != nil {
		m.HandleEvent(ui.Event{Type: ui.CompositionEnd})
	}
	c.node = nil
}
func (c *composeState) input(m *ui.Manager, sym uint32) {
	loadCompose()
	if sym >= 0xffe1 && sym <= 0xffee {
		return
	}
	if c.node != nil && (sym == 0xff1b || sym == 0xff08) {
		c.reset(m)
		return
	}
	node := c.node
	if node == nil {
		node = &composeRoot
	}
	next := node.next[sym]
	if next != nil {
		if c.node == nil {
			m.HandleEvent(ui.Event{Type: ui.CompositionStart})
		}
		c.node = next
		if next.text != "" {
			m.HandleEvent(ui.Event{Type: ui.CompositionEnd, Text: next.text})
			c.node = nil
		}
		return
	}
	if c.node != nil {
		c.reset(m)
		return
	}
	if r := keysymRune(sym); r >= 32 && unicode.IsPrint(r) {
		m.HandleEvent(ui.Event{Type: ui.TextInput, Rune: r})
	}
}
