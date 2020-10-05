package readline

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

var ZeroWidthJoinSequenceOk = os.Getenv("WT_SESSION") != "" && os.Getenv("WT_PROFILE_ID") != ""

type Moji interface {
	Width() width_t
	WriteTo(io.Writer) (int64, error)
	Put(io.Writer)
	IsSpace() bool
}

type CodePoint rune

func (c CodePoint) Width() width_t {
	return GetCharWidth(rune(c))
}

func writeRune(w io.Writer, r rune) (int, error) {
	var b [8]byte
	n := utf8.EncodeRune(b[:], r)
	return w.Write(b[:n])
}

func (c CodePoint) WriteTo(w io.Writer) (int64, error) {
	n, err := writeRune(w, rune(c))
	return int64(n), err
}

func (c CodePoint) Put(w io.Writer) {
	ch := rune(c)
	if ch < ' ' {
		w.Write([]byte{'^', byte('A' + (ch - 1))})
	} else if (ch >= 0x10000 && !SurrogatePairOk) || runewidth.RuneWidth(ch) == 0 {
		fmt.Fprintf(w, "<%X>", ch)
	} else {
		writeRune(w, rune(ch))
	}
}

func (c CodePoint) IsSpace() bool {
	return unicode.IsSpace(rune(c))
}

type ZeroWidthJoinSequence string

func (s ZeroWidthJoinSequence) Width() width_t {
	return width_t(runewidth.StringWidth(string(s))) + 1
}

func (s ZeroWidthJoinSequence) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, string(s))
	return int64(n), err
}

func (s ZeroWidthJoinSequence) Put(w io.Writer) {
	io.WriteString(w, string(s))
}

func (s ZeroWidthJoinSequence) IsSpace() bool {
	return false
}

const (
	zeroWidthJoinRune = '\u200D'
	zeroWidthJoinStr  = "\u200D"
)

func string2moji(s string) []Moji {
	runes := []rune(s)
	mojis := make([]Moji, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		if ZeroWidthJoinSequenceOk && runes[i] == zeroWidthJoinRune && i > 0 && i+1 < len(runes) {
			mojis[len(mojis)-1] = ZeroWidthJoinSequence(string(runes[i-1 : i+2]))
			i++
		} else {
			mojis = append(mojis, CodePoint(runes[i]))
		}
	}
	return mojis
}

func moji2string(m []Moji) string {
	var buffer strings.Builder
	for _, m1 := range m {
		m1.WriteTo(&buffer)
	}
	return buffer.String()
}