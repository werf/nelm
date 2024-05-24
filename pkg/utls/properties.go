package utls

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/looplab/fsm"
	"github.com/samber/lo"
)

const (
	commaRune       = ','
	equalsRune      = '='
	singleQuoteRune = '\''
	doubleQuoteRune = '"'
)

func ParseProperties(ctx context.Context, input string) (map[string]any, error) {
	result := make(map[string]any)

	machine := fsm.NewFSM("start",
		fsm.Events{
			{
				Name: "foundKeyChar",
				Src: []string{
					"start",
					"entriesSeparator",
				},
				Dst: "key",
			},
			{
				Name: "foundValueChar",
				Src: []string{
					"KVSeparator",
				},
				Dst: "value",
			},
			{
				Name: "foundQuotedValueChar",
				Src: []string{
					"openingValueQuote",
				},
				Dst: "quotedValue",
			},
			{
				Name: "foundKVSeparator",
				Src: []string{
					"key",
				},
				Dst: "KVSeparator",
			},
			{
				Name: "foundEntriesSeparator",
				Src: []string{
					"start",
					"key",
					"value",
					"KVSeparator",
					"closingValueQuote",
				},
				Dst: "entriesSeparator",
			},
			{
				Name: "foundOpeningValueQuote",
				Src: []string{
					"KVSeparator",
				},
				Dst: "openingValueQuote",
			},
			{
				Name: "foundClosingValueQuote",
				Src: []string{
					"quotedValue",
				},
				Dst: "closingValueQuote",
			},
		},
		fsm.Callbacks{},
	)

	var key []rune
	var value []rune
	var valQuote rune
	var valPresent bool
	reader := bufio.NewReader(strings.NewReader(input))
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				if len(key) > 0 {
					k, v := convertKeyValue(key, value, valPresent, machine.Is("closingValueQuote"))
					result[k] = v
				}

				break
			} else {
				return nil, fmt.Errorf("error reading input: %w", err)
			}
		}

		switch r {
		case commaRune:
			if machine.Can("foundEntriesSeparator") {
				if len(key) == 0 {
					lo.Must0(machine.Event(ctx, "foundEntriesSeparator"))
					break
				}

				k, v := convertKeyValue(key, value, valPresent, machine.Is("closingValueQuote"))
				result[k] = v

				key = []rune{}
				value = []rune{}
				valPresent = false

				lo.Must0(machine.Event(ctx, "foundEntriesSeparator"))
			} else if machine.Is("key") {
				key = append(key, r)
			} else if machine.Is("value") || machine.Is("quotedValue") {
				value = append(value, r)
			}
		case equalsRune:
			if machine.Can("foundKVSeparator") {
				valPresent = true
				lo.Must0(machine.Event(ctx, "foundKVSeparator"))
			} else if machine.Is("key") {
				key = append(key, r)
			} else if machine.Is("value") || machine.Is("quotedValue") {
				value = append(value, r)
			}
		case doubleQuoteRune, singleQuoteRune:
			if machine.Can("foundOpeningValueQuote") {
				valQuote = r
				lo.Must0(machine.Event(ctx, "foundOpeningValueQuote"))
			} else if machine.Can("foundClosingValueQuote") && valQuote == r && (len(value) == 0 || value[len(value)-1] != '\\') {
				lo.Must0(machine.Event(ctx, "foundClosingValueQuote"))
			} else if machine.Is("key") {
				key = append(key, r)
			} else if machine.Is("value") || machine.Is("quotedValue") {
				value = append(value, r)
			}
		default:
			if unicode.IsSpace(r) &&
				(machine.Is("KVSeparator") || machine.Is("entriesSeparator")) {
				break
			}

			if machine.Can("foundKeyChar") {
				lo.Must0(machine.Event(ctx, "foundKeyChar"))
			} else if machine.Can("foundValueChar") {
				lo.Must0(machine.Event(ctx, "foundValueChar"))
			} else if machine.Can("foundQuotedValueChar") {
				lo.Must0(machine.Event(ctx, "foundQuotedValueChar"))
			}

			if machine.Is("key") {
				key = append(key, r)
			} else if machine.Is("value") || machine.Is("quotedValue") {
				value = append(value, r)
			}
		}
	}

	return result, nil
}

func convertKeyValue(key, value []rune, valPresent, valQuoted bool) (string, any) {
	k := string(key)
	k = strings.TrimSpace(k)
	k = strings.ToLower(k)

	var v any
	if !valPresent {
		if strings.HasPrefix(strings.ToLower(k), "no") {
			k = trimLeftChars(k, 2)
			v = false
		} else {
			v = true
		}
	} else {
		v = string(value)
		if !valQuoted {
			v = strings.TrimSpace(v.(string))
		}
	}

	return k, v
}

func trimLeftChars(s string, n int) string {
	m := 0
	for i := range s {
		if m >= n {
			return s[i:]
		}
		m++
	}

	return s[:0]
}
