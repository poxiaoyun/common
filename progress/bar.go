package progress

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
	"xiaoshiai.cn/common/units"
)

var SpinnerDefault = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

type Bar struct {
	Name       string
	MaxNameLen int    // max name length
	Total      int64  // total bytes, -1 for indeterminate
	Width      int    // width of the bar
	Status     string // status text
	Done       bool   // if the bar is done
	Fragments  map[string]*BarFragment

	nameindex int // scroll name index
	cnt       int // refresh count for scroll name
	changed   bool
	mu        sync.RWMutex
	mp        *MultiBar
}

func NewSingleBar(ctx context.Context) *Bar {
	width := 60
	w, _, err := term.GetSize(0)
	if err == nil {
		if w < 40 {
			w = 40
		}
		width = w - 20 // min 20 chars for status
	}
	if width > 80 {
		width = 80
	}
	bar := &Bar{
		MaxNameLen: 8,
		Width:      width,
	}
	go runSigngleBar(ctx, bar)
	return bar
}

func runSigngleBar(ctx context.Context, bar *Bar) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	// initial print
	bar.Print(os.Stdout)
	for {
		select {
		case <-ctx.Done():
			// print once
			bar.SetStatus("canceld", false)
			fmt.Fprint(os.Stdout, "\033[1A\033[J")
			bar.Print(os.Stdout)
			return
		case <-t.C:
			if changed := bar.changed; changed {
				bar.changed = false
				fmt.Fprint(os.Stdout, "\033[1A\033[J")
				bar.Print(os.Stdout)
			}
		}
	}
}

type BarFragment struct {
	Offset int64  // offset of the fragment
	Size   int64  // processed bytes
	uid    string // uid of the fragment, for delete
	idx    int    // index when no total
}

func (b *Bar) SetNameStatus(name, status string, done bool) {
	b.Name, b.Status, b.Done = name, status, done
	b.Notify()
}

func (b *Bar) SetStatus(status string, done bool) {
	b.Status = status
	b.Done = done
	b.Notify()
}

func (b *Bar) SetDone() {
	b.Done = true
	b.Notify()
}

func (r *Bar) Notify() {
	r.changed = true
	if r.mp != nil {
		r.mp.haschange = true
	}
}

func (b *Bar) Print(w io.Writer) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	name := b.calcname()
	proc, status := b.calcFragments(b.Width - len(name) - 12)
	fmt.Fprintf(w, "%s [%s] %s\n", name, proc, status)
}

func (b *Bar) calcname() string {
	maxlen := b.MaxNameLen

	showname := b.Name
	// scroll name
	if len(b.Name) > maxlen {
		showname += "  "
		lowptr := b.nameindex % len(showname)
		maxptr := lowptr + maxlen
		if maxptr < len(showname) {
			showname = showname[lowptr:maxptr]
		} else {
			showname = showname[lowptr:] + showname[:maxptr-len(showname)]
		}
		// 3x speed low than fps
		if b.cnt%3 == 0 {
			b.nameindex++
			b.Notify()
		}
		b.cnt++
	} else if len(showname) < maxlen {
		// fill space
		showname += strings.Repeat(" ", maxlen-len(showname))
	}

	return showname
}

func (b *Bar) calcFragments(maxlen int) (string, string) {
	buf := bytes.Repeat([]byte("-"), maxlen)
	processed := int64(0)
	for _, f := range b.Fragments {
		processed += f.Size
		if b.Total > 0 {
			start := int(float64(maxlen) * float64(f.Offset) / float64(b.Total))
			end := int(float64(maxlen) * float64(f.Offset+f.Size) / float64(b.Total))
			if end > maxlen {
				end = maxlen
			}
			if start < 0 {
				start = 0
			}
			for i := start; i < end; i++ {
				buf[i] = '+'
			}
		} else {
			//------------+----------
			//-------------+---------
			//--------------+--------
			//---------------+-------
			//----------------+------
			buf[f.idx%maxlen] = '+'
			f.idx++
		}
	}
	if processed <= 0 {
		return string(buf), b.Status
	}
	if b.Total <= 0 {
		return string(buf), units.HumanSize(float64(processed))
	} else {
		return string(buf), units.HumanSize(float64(processed)) + "/" + units.HumanSize(float64(b.Total))
	}
}
