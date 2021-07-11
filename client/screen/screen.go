package screen

import (
	"log"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"

	"bytes"
	"strconv"
)

type Screen struct {
	s        tcell.Screen
	channelM map[string]*channel
	channels []*channel
	cur      int
	entryC   chan Entry
}

type channel struct {
	name  string
	lines []string
	buf   bytes.Buffer
}

type Entry struct {
	Channel string
	Line    string
}

func New() (*Screen, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	sc := &Screen{
		s:        s,
		channels: []*channel{{name: "*status"}},
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	go sc.loop()

	return sc, nil
}

func (s *Screen) setString(x, y int, style tcell.Style, str string) int {
	var len int
	for _, r := range str {
		var comb []rune
		w := runewidth.RuneWidth(r)
		if w == 0 {
			comb = []rune{r}
			r = ' '
			w = 1
		}
		s.s.SetContent(x, y, r, comb, style)
		x += w
		len += w
	}

	return len
}

func (s *Screen) reDraw() {
	_, h := s.s.Size()
	s.s.Clear()

	// start drawing from the bottom, first list each channel
	y := h - 1
	x := 0
	for n := range s.channels {
		if s.cur == n {
			x += s.setString(x, y, tcell.StyleDefault, "["+strconv.Itoa(n)+"] ")
		} else {
			x += s.setString(x, y, tcell.StyleDefault, " "+strconv.Itoa(n)+"  ")
		}
	}
	// now draw the input line
	x = 0
	y--
	var c *channel
	if s.cur == -1 {
		c = &channel{name: "*no channel*"}
	} else {
		c = s.channels[s.cur]
	}
	s.setString(x, y, tcell.StyleDefault, "["+c.name+"] "+c.buf.String())

	// finally draw all the messages
	y--
	i := len(c.lines) - 1
	for ; y >= 0; y-- {
		if len(c.lines) <= 0 {
			break
		}
		// TODO: handle wrapping
		log.Printf("x %d y %d i %d", x, y, i)
		s.setString(x, y, tcell.StyleDefault, c.lines[i])
		i--
	}

	s.s.Sync()
}

func (s *Screen) loop() {
	s.reDraw()
	for {
		switch ev := s.s.PollEvent().(type) {
		case *tcell.EventResize:
			s.reDraw()
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyCtrlN:
				s.cur += 1
				if s.cur >= len(s.channels) {
					s.cur = 0
				}
				s.reDraw()
			case tcell.KeyCtrlP:
				s.cur -= 1
				if s.cur <= 0 {
					s.cur = len(s.channels) - 1
				}
				s.reDraw()
			case tcell.KeyBackspace:
				c := s.channels[s.cur]
				if c.buf.Len() != 0 {
					s.channels[s.cur].buf.UnreadRune()
				}
				s.reDraw()
			case tcell.KeyEnter:
				c := s.channels[s.cur]
				s.entryC <- Entry{
					Channel: c.name,
					Line:    c.buf.String(),
				}
				c.buf.Truncate(0)
				s.reDraw()
			case tcell.KeyRune:
				s.channels[s.cur].buf.WriteRune(ev.Rune())
				s.reDraw()
			}
		}
	}
}

func (s *Screen) JoinChannel(name string) {
}

func (s *Screen) PartChannel(name string) {
}

func (s *Screen) AddLine(channel, line string) {
}

func (s *Screen) GetEntry() Entry {
	return <-s.entryC
}
