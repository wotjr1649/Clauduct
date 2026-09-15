package auth

import "strings"

// readStoreSetting extracts one root setting from a Codex config file and nothing else.
//
// It is a bounded structural reader, not a TOML parser and not a config validator. The
// only question it answers is which credential store the user selected, because reading
// auth.json when the user's credentials live in the system keyring would report "no
// credentials" while they sit somewhere this build never looked.
//
// It is structural rather than a regular expression on purpose. A key name can be quoted,
// a value can be a multi-line string containing anything including the key name again, and
// a pattern match would find the wrong one. Skipping a value correctly requires
// understanding where it ends, which is most of a parser whether or not it is called one.
//
// Ported from the Node baseline, including which shapes it refuses.
func readStoreSetting(config string) (string, error) {
	if len(config) > maxConfigBytes {
		return "", errorOf(CategoryConfigUnsupported)
	}

	r := &tomlReader{text: config, rootTable: true}
	if err := r.scan(); err != nil {
		return "", err
	}
	if !r.found {
		return "", nil
	}
	switch r.store {
	case "file", "keyring", "auto", "ephemeral":
		return r.store, nil
	}
	// A value outside the known set is not something to guess at: it may name a store that
	// holds the real credentials.
	return "", errorOf(CategoryConfigUnsupported)
}

type tomlReader struct {
	text      string
	offset    int
	rootTable bool
	found     bool
	store     string
}

var errConfig = errorOf(CategoryConfigUnsupported)

func (r *tomlReader) at() byte {
	if r.offset >= len(r.text) {
		return 0
	}
	return r.text[r.offset]
}

func (r *tomlReader) space() {
	for r.at() == ' ' || r.at() == '\t' {
		r.offset++
	}
}

func (r *tomlReader) comment() {
	for r.offset < len(r.text) && r.text[r.offset] != '\n' {
		r.offset++
	}
}

func (r *tomlReader) scan() error {
	for r.offset < len(r.text) {
		r.space()
		switch c := r.at(); {
		case c == '#':
			r.comment()
		case c == '\r' || c == '\n':
			r.offset++
		case c == '[':
			if err := r.tableHeader(); err != nil {
				return err
			}
		default:
			if err := r.assignment(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *tomlReader) tableHeader() error {
	r.offset++
	array := r.at() == '['
	if array {
		r.offset++
	}
	path, err := r.keyPath()
	if err != nil {
		return err
	}
	// A table by this name would make the setting a sub-table rather than a root string,
	// which is a shape this reader does not claim to understand.
	if len(path) > 0 && path[0] == storeKey {
		return errConfig
	}
	if r.at() != ']' {
		return errConfig
	}
	r.offset++
	if array {
		if r.at() != ']' {
			return errConfig
		}
		r.offset++
	}
	if err := r.lineEnd(); err != nil {
		return err
	}
	// Everything after a table header belongs to that table, so the root key cannot appear
	// again from here on.
	r.rootTable = false
	return nil
}

func (r *tomlReader) assignment() error {
	path, err := r.keyPath()
	if err != nil {
		return err
	}
	if r.at() != '=' {
		return errConfig
	}
	r.offset++
	r.space()

	if r.rootTable && len(path) > 0 && path[0] == storeKey {
		// Exactly one root key, exactly once, and it must be a quoted string. A dotted
		// path or a second occurrence leaves two readings of the same setting.
		if r.found || len(path) != 1 || (r.at() != '"' && r.at() != '\'') {
			return errConfig
		}
		value, err := r.quoted(false, true)
		if err != nil {
			return err
		}
		r.found, r.store = true, value
		return r.lineEnd()
	}
	return r.skipValue()
}

func (r *tomlReader) keyPath() ([]string, error) {
	var parts []string
	for {
		r.space()
		if c := r.at(); c == '"' || c == '\'' {
			value, err := r.quoted(true, true)
			if err != nil {
				return nil, err
			}
			parts = append(parts, value)
		} else {
			start := r.offset
			for r.offset < len(r.text) && isBareKey(r.text[r.offset]) {
				r.offset++
			}
			if r.offset == start {
				return nil, errConfig
			}
			parts = append(parts, r.text[start:r.offset])
		}
		if len(parts) > maxKeyParts {
			return nil, errConfig
		}
		r.space()
		if r.at() != '.' {
			return parts, nil
		}
		r.offset++
	}
}

func isBareKey(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// quoted reads a string. decode says whether the characters are wanted or only skipped;
// key says the string is a key, where a multi-line form is not legal.
func (r *tomlReader) quoted(key, decode bool) (string, error) {
	quote := r.at()
	r.offset++
	multiline := strings.HasPrefix(r.text[r.offset:], string([]byte{quote, quote}))
	if multiline {
		if key {
			return "", errConfig
		}
		r.offset += 2
		// A newline immediately after the opening delimiter is not part of the value.
		if strings.HasPrefix(r.text[r.offset:], "\r\n") {
			r.offset += 2
		} else if r.at() == '\n' {
			r.offset++
		}
	}

	var value strings.Builder
	for r.offset < len(r.text) {
		c := r.text[r.offset]
		r.offset++

		switch {
		case c == quote:
			if !multiline {
				return value.String(), nil
			}
			count := 1
			for r.at() == quote {
				count++
				r.offset++
			}
			if count > 5 {
				return "", errConfig
			}
			if count >= 3 {
				if decode {
					value.WriteString(strings.Repeat(string(quote), count-3))
				}
				return value.String(), nil
			}
			if decode {
				value.WriteString(strings.Repeat(string(quote), count))
			}

		case c == '\\' && quote == '"':
			if err := r.escape(&value, multiline, decode); err != nil {
				return "", err
			}

		default:
			if !multiline && (c == '\r' || c == '\n') || isControl(c) {
				return "", errConfig
			}
			if decode {
				value.WriteByte(c)
			}
		}
		if decode && value.Len() > maxValueBytes {
			return "", errConfig
		}
	}
	// The file ended inside a string.
	return "", errConfig
}

func (r *tomlReader) escape(value *strings.Builder, multiline, decode bool) error {
	if r.offset >= len(r.text) {
		return errConfig
	}
	escaped := r.text[r.offset]
	r.offset++

	if multiline && isSpace(escaped) {
		// A line-ending backslash swallows the following whitespace, but only if a line
		// ending is actually part of it.
		newline := escaped == '\r' || escaped == '\n'
		for r.offset < len(r.text) && isSpace(r.text[r.offset]) {
			if r.text[r.offset] == '\r' || r.text[r.offset] == '\n' {
				newline = true
			}
			r.offset++
		}
		if !newline {
			return errConfig
		}
		return nil
	}

	if escaped == 'u' || escaped == 'U' {
		width := 4
		if escaped == 'U' {
			width = 8
		}
		if r.offset+width > len(r.text) {
			return errConfig
		}
		hex := r.text[r.offset : r.offset+width]
		point := 0
		for i := 0; i < width; i++ {
			digit := hexValue(hex[i])
			if digit < 0 {
				return errConfig
			}
			point = point*16 + digit
		}
		r.offset += width
		// Outside Unicode, or a surrogate half, which is not a character.
		if point > 0x10ffff || (point >= 0xd800 && point <= 0xdfff) {
			return errConfig
		}
		if decode {
			value.WriteRune(rune(point))
		}
		return nil
	}

	replacement, ok := map[byte]byte{
		'b': '\b', 't': '\t', 'n': '\n', 'f': '\f', 'r': '\r', '"': '"', '\\': '\\',
	}[escaped]
	if !ok {
		return errConfig
	}
	if decode {
		value.WriteByte(replacement)
	}
	return nil
}

// skipValue walks past a value without interpreting it, tracking bracket depth so that a
// string or a nested structure containing a newline does not end the line early.
func (r *tomlReader) skipValue() error {
	var stack []byte
	for r.offset < len(r.text) {
		c := r.at()
		switch {
		case c == '"' || c == '\'':
			if _, err := r.quoted(false, false); err != nil {
				return err
			}
		case c == '#':
			r.comment()
			if len(stack) == 0 {
				return nil
			}
		case c == '\n' && len(stack) == 0:
			r.offset++
			return nil
		case c == '[' || c == '{':
			if len(stack) >= maxNesting {
				return errConfig
			}
			closer := byte(']')
			if c == '{' {
				closer = '}'
			}
			stack = append(stack, closer)
			r.offset++
		case c == ']' || c == '}':
			if len(stack) == 0 || stack[len(stack)-1] != c {
				return errConfig
			}
			stack = stack[:len(stack)-1]
			r.offset++
		default:
			r.offset++
		}
	}
	if len(stack) > 0 {
		return errConfig
	}
	return nil
}

func (r *tomlReader) lineEnd() error {
	r.space()
	if r.at() == '#' {
		r.comment()
	}
	if r.at() == '\r' {
		r.offset++
	}
	if r.offset < len(r.text) && r.at() != '\n' {
		return errConfig
	}
	if r.at() == '\n' {
		r.offset++
	}
	return nil
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func isControl(c byte) bool {
	return c <= 0x08 || c == 0x0b || c == 0x0c || (c >= 0x0e && c <= 0x1f) || c == 0x7f
}

func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
