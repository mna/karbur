// Package errors provides functions and types to define and manipulate
// errors. It should generally be used instead of the standard library's
// errors package, as it is a strict superset.
package errors

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// ErrUnsupported indicates that a requested operation cannot be performed,
// because it is unsupported. See the stdlib's errors.ErrUnsupported
// documentation for more details.
var ErrUnsupported = errors.ErrUnsupported

// New returns an error with s as error message.
// See the stdlib's errors.New documentation for more details.
func New(s string) error { return errors.New(s) }

// Errorf formats according to a format specifier and returns the string as a
// value that satisfies error.
//
// If the format specifier includes a %w verb with an error operand, the
// returned error will implement an Unwrap method returning the operand. It is
// invalid to include more than one %w verb or to supply it with an operand
// that does not implement the error interface. The %w verb is otherwise a
// synonym for %v.
//
// See the stdlib's fmt.Errorf documentation for more details.
func Errorf(f string, args ...any) error { return fmt.Errorf(f, args...) }

// Is reports whether any error in err's chain matches target.
// See the stdlib's errors.Is documentation for more details.
func Is(err, target error) bool { return errors.Is(err, target) }

// As finds the first error in err's chain that matches target,
// and if so, sets target to that error value and returns true.
// Otherwise, it returns false.
//
// See the stdlib's errors.As documentation for more details.
func As(err error, target any) bool { return errors.As(err, target) }

// AsType finds the first error in err's tree that matches the
// type E, and if one is found, returns that error value and true.
// Otherwise, it returns the zero value of E and false.
//
// See the stdlib's errors.As documentation for more details.
func AsType[E error](err error) (E, bool) { return errors.AsType[E](err) }

// Join returns an error that wraps the given errors. See the stdlib's
// errors.Join documentation for more details.
func Join(errs ...error) error { return errors.Join(errs...) }

// Unwrap returns the result of calling the Unwrap method on err,
// if err's type contains an Unwrap method returning error. Otherwise,
// Unwrap returns nil.
//
// See the stdlib's errors.Unwrap documentation for more details.
func Unwrap(err error) error { return errors.Unwrap(err) }

// ConstError is an error string that can be defined as constant.
type ConstError string

// Error returns the error message of the ConstError, which is the
// constant string value itself.
func (e ConstError) Error() string {
	return string(e)
}

// ErrorTag is the type of a tag that can be applied to an error using
// errors.Tag. It is expected that the main program defines its own predefined
// and shared list of tags to be used throughout the code.
//
// It is conceptually similar to the error flags mentioned in this blog post:
// https://npf.io/2021/04/errorflags/, except that ErrorTag is a string that is
// also used as prefix to the error message.
type ErrorTag string

type taggedError struct {
	tag  ErrorTag
	err  error
	meta map[string]string
}

func (e taggedError) Error() string {
	var buf strings.Builder

	if len(e.meta) > 0 {
		keys := slices.Sorted(maps.Keys(e.meta))

		buf.WriteString(" [")
		for i, k := range keys {
			if i > 0 {
				buf.WriteString(", ")
			}
			fmt.Fprintf(&buf, "%s: %s", k, e.meta[k])
		}
		buf.WriteByte(']')
	}
	return string(e.tag) + ": " + e.err.Error() + buf.String()
}

func (e taggedError) Unwrap() error {
	return e.err
}

// Tag returns an error that wraps e and tags it with the provided error tag.
// Errors can be queried for tags with IsTag. An arbitrary set of key-value
// pairs can also be provided and will be stored in the error and printed in
// the error message. It is possible to query an error for presence of a key
// using HasKey and presence of a specific key-value pair with HasKeyValue. If
// the number of key-value arguments is not even, the final key is associated
// with an empty string value.
//
// The special key "code" should be set to an integer value (as a string) when
// provided, and if so it can be queried with HasCode and extracted as integer
// with Code.
//
// Example uses of key-value metadata could be to identify the argument that
// failed validation, or the (stringified) status code of an HTTP request.
//
// If e is nil, it returns nil.
func Tag(e error, tag ErrorTag, kvpairs ...string) error {
	if e == nil {
		return e
	}

	var m map[string]string
	if len(kvpairs) > 0 {
		m = make(map[string]string, len(kvpairs)/2)
		for i := 0; i < len(kvpairs); i += 2 {
			k := kvpairs[i]
			var v string
			if i+1 < len(kvpairs) {
				v = kvpairs[i+1]
			}
			m[k] = v
		}
	}
	return taggedError{tag: tag, err: e, meta: m}
}

// CodeKey is the key used to store an error code in the error metadata.
// If the associated value is a valid integer, it can be queried and
// extracted via HasCode and Code.
const CodeKey = "code"

// IsTag returns true if e or any error in its chain is tagged with the
// provided tag.
func IsTag(e error, tag ErrorTag) bool {
	var te taggedError
	if As(e, &te) {
		if te.tag == tag {
			return true
		}
		return IsTag(te.Unwrap(), tag)
	}
	return false
}

// HasKey returns true if e or any error in its chain has been tagged with the
// specified key, regardless of its value.
func HasKey(e error, key string) bool {
	var te taggedError
	if As(e, &te) {
		if _, ok := te.meta[key]; ok {
			return true
		}
		return HasKey(te.Unwrap(), key)
	}
	return false
}

// HasKeyValue returns true if e or any error in its chain has been tagged with
// the specified key and value.
func HasKeyValue(e error, key, value string) bool {
	var te taggedError
	if As(e, &te) {
		if v, ok := te.meta[key]; ok && v == value {
			return true
		}
		return HasKeyValue(te.Unwrap(), key, value)
	}
	return false
}

// KeyValue returns the value associated with the specified key if any error in
// the chain has been tagged with it. If no such error exists, it returns an
// empty string.
func KeyValue(e error, key string) string {
	var te taggedError
	if As(e, &te) {
		if v, ok := te.meta[key]; ok {
			return v
		}
		return KeyValue(te.Unwrap(), key)
	}
	return ""
}

// HasCode returns true if e or any error in its chain has been tagged with any
// of the provided code values. The "code" key is a special key that, if
// present and its value is an integer, is interpreted as the error code.
func HasCode(e error, codes ...int) bool {
	if e == nil || len(codes) == 0 {
		return false
	}

	// naive implementation would be to call HasKeyValue for each code,
	// but it would be slow to walk the error chain each time, so we implement
	// an optimized version.
	lookup := make(map[string]bool, len(codes))
	for _, code := range codes {
		lookup[fmt.Sprint(code)] = true
	}
	return hasCode(e, lookup)
}

func hasCode(e error, lookup map[string]bool) bool {
	var te taggedError
	if As(e, &te) {
		if v, ok := te.meta[CodeKey]; ok && lookup[v] {
			return true
		}
		return hasCode(te.Unwrap(), lookup)
	}
	return false
}

// Code returns the integer value of the "code" key in the error metadata, if
// it is present and its value is a valid integer. Otherwise it returns 0.
func Code(e error) int {
	v := KeyValue(e, CodeKey)
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return 0
}
