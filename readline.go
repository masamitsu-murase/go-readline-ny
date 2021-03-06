package readline

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Result is the type for readline's result.
type Result int

const (
	CONTINUE Result = iota
	ENTER    Result = iota
	ABORT    Result = iota
	INTR     Result = iota
)

// String makes Result to fmt.Stringer
func (this Result) String() string {
	switch this {
	case CONTINUE:
		return "CONTINUE"
	case ENTER:
		return "ENTER"
	case ABORT:
		return "ABORT"
	case INTR:
		return "INTR"
	default:
		return "ERROR"
	}
}

type KeyFuncT interface {
	Call(ctx context.Context, buffer *Buffer) Result
}

type KeyGoFuncT struct {
	Func func(ctx context.Context, buffer *Buffer) Result
	Name string
}

func (this *KeyGoFuncT) Call(ctx context.Context, buffer *Buffer) Result {
	if this.Func == nil {
		return CONTINUE
	}
	return this.Func(ctx, buffer)
}

func (this KeyGoFuncT) String() string {
	return this.Name
}

var defaultKeyMap = map[string]KeyFuncT{
	name2char[K_CTRL_A]:        name2func(F_BEGINNING_OF_LINE),
	name2char[K_CTRL_B]:        name2func(F_BACKWARD_CHAR),
	name2char[K_BACKSPACE]:     name2func(F_BACKWARD_DELETE_CHAR),
	name2char[K_CTRL_C]:        name2func(F_INTR),
	name2char[K_CTRL_D]:        name2func(F_DELETE_OR_ABORT),
	name2char[K_CTRL_E]:        name2func(F_END_OF_LINE),
	name2char[K_CTRL_F]:        name2func(F_FORWARD_CHAR),
	name2char[K_CTRL_H]:        name2func(F_BACKWARD_DELETE_CHAR),
	name2char[K_CTRL_K]:        name2func(F_KILL_LINE),
	name2char[K_CTRL_L]:        name2func(F_CLEAR_SCREEN),
	name2char[K_CTRL_M]:        name2func(F_ACCEPT_LINE),
	name2char[K_CTRL_R]:        name2func(F_ISEARCH_BACKWARD),
	name2char[K_CTRL_U]:        name2func(F_UNIX_LINE_DISCARD),
	name2char[K_CTRL_Y]:        name2func(F_YANK),
	name2char[K_DELETE]:        name2func(F_DELETE_CHAR),
	name2char[K_ENTER]:         name2func(F_ACCEPT_LINE),
	name2char[K_ESCAPE]:        name2func(F_KILL_WHOLE_LINE),
	name2char[K_CTRL_N]:        name2func(F_HISTORY_DOWN),
	name2char[K_CTRL_P]:        name2func(F_HISTORY_UP),
	name2char[K_CTRL_Q]:        name2func(F_QUOTED_INSERT),
	name2char[K_CTRL_T]:        name2func(F_SWAPCHAR),
	name2char[K_CTRL_V]:        name2func(F_QUOTED_INSERT),
	name2char[K_CTRL_W]:        name2func(F_UNIX_WORD_RUBOUT),
	name2char[K_CTRL]:          name2func(F_PASS),
	name2char[K_DELETE]:        name2func(F_DELETE_CHAR),
	name2char[K_END]:           name2func(F_END_OF_LINE),
	name2char[K_HOME]:          name2func(F_BEGINNING_OF_LINE),
	name2char[K_LEFT]:          name2func(F_BACKWARD_CHAR),
	name2char[K_RIGHT]:         name2func(F_FORWARD_CHAR),
	name2char[K_SHIFT]:         name2func(F_PASS),
	name2char[K_DOWN]:          name2func(F_HISTORY_DOWN),
	name2char[K_UP]:            name2func(F_HISTORY_UP),
	name2char[K_ALT_V]:         name2func(F_YANK),
	name2char[K_ALT_Y]:         name2func(F_YANK_WITH_QUOTE),
	name2char[K_ALT_B]:         name2func(F_BACKWARD_WORD),
	name2char[K_ALT_F]:         name2func(F_FORWARD_WORD),
	name2char[K_CTRL_LEFT]:     name2func(F_BACKWARD_WORD),
	name2char[K_CTRL_RIGHT]:    name2func(F_FORWARD_WORD),
	name2char[K_CTRL_Z]:        name2func(F_UNDO),
	name2char[K_CTRL_UNDERBAR]: name2func(F_UNDO),
}

func normWord(src string) string {
	return strings.Replace(strings.ToUpper(src), "-", "_", -1)
}

func (editor *KeyMap) BindKeyFunc(key string, f KeyFuncT) error {
	key = normWord(key)
	if char, ok := name2char[key]; ok {
		if editor.KeyMap == nil {
			editor.KeyMap = map[string]KeyFuncT{}
		}
		editor.KeyMap[char] = f
		return nil
	}
	return fmt.Errorf("%s: no such keyname", key)
}

var GlobalKeyMap KeyMap

func (editor *KeyMap) BindKeyClosure(name string, f func(context.Context, *Buffer) Result) error {
	return editor.BindKeyFunc(name, &KeyGoFuncT{Func: f, Name: "annonymous"})
}

func (editor *KeyMap) GetBindKey(key string) KeyFuncT {
	key = normWord(key)
	if char, ok := name2char[key]; ok {
		if editor.KeyMap != nil {
			if f, ok := editor.KeyMap[char]; ok {
				return f
			}
		}
		return editor.KeyMap[char]
	} else {
		return nil
	}
}

func GetFunc(funcName string) (KeyFuncT, error) {
	rc := name2func(normWord(funcName))
	if rc != nil {
		return rc, nil
	} else {
		return nil, fmt.Errorf("%s: not found in the function-list", funcName)
	}
}

func (editor *KeyMap) BindKeySymbol(keyName, funcName string) error {
	funcValue := name2func(normWord(funcName))
	if funcValue == nil {
		return fmt.Errorf("%s: no such function.", funcName)
	}
	return editor.BindKeyFunc(keyName, funcValue)
}

const (
	ansiCursorOff = "\x1B[?25l"
	ansiCursorOn  = "\x1B[?25h\x1B[s\x1B[u"
)

var CtrlC = errors.New("^C")

var mu sync.Mutex

func getKeyFunction(editor *Editor, key1 string) KeyFuncT {
	if editor.KeyMap.KeyMap != nil {
		if f, ok := editor.KeyMap.KeyMap[key1]; ok {
			return f
		}
	}
	if f, ok := GlobalKeyMap.KeyMap[key1]; ok {
		return f
	}
	if f, ok := defaultKeyMap[key1]; ok {
		return f
	}
	return &KeyGoFuncT{
		Func: func(ctx context.Context, this *Buffer) Result {
			return keyFuncInsertSelf(ctx, this, key1)
		},
		Name: key1,
	}
}

// Call LineEditor
// - ENTER typed -> returns TEXT and nil
// - CTRL-C typed -> returns "" and readline.CtrlC
// - CTRL-D typed -> returns "" and io.EOF
func (editor *Editor) ReadLine(ctx context.Context) (string, error) {
	if editor.Writer == nil {
		editor.Writer = os.Stdout
	}
	if editor.Out == nil {
		editor.Out = bufio.NewWriter(editor.Writer)
	}
	defer func() {
		editor.Out.WriteString(ansiCursorOn)
		editor.Out.Flush()
	}()

	if editor.Prompt == nil {
		editor.Prompt = func() (int, error) {
			editor.Out.WriteString("\n> ")
			return 2, nil
		}
	}
	if editor.History == nil {
		editor.History = new(EmptyHistory)
	}
	if editor.LineFeed == nil {
		editor.LineFeed = func(Result) {
			editor.Out.WriteByte('\n')
		}
	}
	if editor.OpenKeyGetter == nil {
		editor.OpenKeyGetter = NewDefaultTty
	}
	buffer := Buffer{
		Editor:         editor,
		Buffer:         make([]Moji, 0, 20),
		HistoryPointer: editor.History.Len(),
	}

	tty1, err := editor.OpenKeyGetter()
	if err != nil {
		return "", fmt.Errorf("go-tty.Open: %s", err.Error())
	}
	buffer.TTY = tty1
	defer tty1.Close()

	buffer.termWidth, _, err = tty1.Size()
	if err != nil {
		return "", fmt.Errorf("go-tty.Size: %s", err.Error())
	}

	var err1 error
	buffer.topColumn, err1 = editor.Prompt()
	if err1 != nil {
		// unable to get prompt-string.
		fmt.Fprintf(buffer.Out, "%s\n$ ", err1.Error())
		buffer.topColumn = 2
	} else if buffer.topColumn >= buffer.termWidth-3 {
		// ViewWidth is too narrow to edit.
		io.WriteString(buffer.Out, "\n")
		buffer.topColumn = 0
	}
	buffer.InsertString(0, editor.Default)
	if buffer.Cursor > len(buffer.Buffer) {
		buffer.Cursor = len(buffer.Buffer)
	}
	buffer.RepaintAfterPrompt()

	cursorOnSwitch := false

	buffer.startChangeWidthEventLoop(buffer.termWidth, tty1.GetResizeNotifier())

	for {
		mu.Lock()
		if !cursorOnSwitch {
			io.WriteString(buffer.Out, ansiCursorOn)
			cursorOnSwitch = true
		}
		buffer.Out.Flush()

		mu.Unlock()
		key1, err := buffer.GetKey()
		if err != nil {
			return "", err
		}
		mu.Lock()

		f := getKeyFunction(editor, key1)

		if fg, ok := f.(*KeyGoFuncT); !ok || fg.Func != nil {
			io.WriteString(buffer.Out, ansiCursorOff)
			cursorOnSwitch = false
			buffer.Out.Flush()
		}
		rc := f.Call(ctx, &buffer)
		if rc != CONTINUE {
			buffer.LineFeed(rc)

			if !cursorOnSwitch {
				io.WriteString(buffer.Out, ansiCursorOn)
			}
			buffer.Out.Flush()
			result := buffer.String()
			mu.Unlock()
			if rc == ENTER {
				return result, nil
			} else if rc == INTR {
				return result, CtrlC
			} else {
				return result, io.EOF
			}
		}
		mu.Unlock()
	}
}
