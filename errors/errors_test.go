package errors

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	constErr ConstError = "an error"
	testTag  ErrorTag   = "test"
	nopeTag  ErrorTag   = "nope"
)

func TestTag(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		err := Tag(nil, testTag)
		require.Nil(t, err)
	})

	t.Run("no key value", func(t *testing.T) {
		err := Tag(constErr, testTag)
		require.NotNil(t, err)
		require.ErrorContains(t, err, string(testTag))
		require.ErrorContains(t, err, constErr.Error())
	})

	t.Run("even key value", func(t *testing.T) {
		err := Tag(constErr, testTag, "x", "1", "y", "2")
		require.NotNil(t, err)
		require.ErrorContains(t, err, "[x: 1, y: 2]")
	})

	t.Run("odd key value", func(t *testing.T) {
		err := Tag(constErr, testTag, "x", "1", "y")
		require.NotNil(t, err)
		require.ErrorContains(t, err, "[x: 1, y: ]")
	})

	t.Run("single key", func(t *testing.T) {
		err := Tag(constErr, testTag, "x")
		require.NotNil(t, err)
		require.ErrorContains(t, err, "[x: ]")
	})
}

func TestIsTag(t *testing.T) {
	cases := []struct {
		desc string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"simple", Tag(constErr, testTag), true},
		{"wrapped", Errorf("error: %w", Tag(constErr, testTag)), true},
		{"mismatch", Errorf("error: %w", Tag(constErr, nopeTag)), false},
		{"multiple tags", Tag(Errorf("error: %w", Tag(constErr, testTag)), nopeTag), true},
		{"multiple tags mismatch", Tag(Errorf("error: %w", Tag(constErr, ErrorTag("other"))), nopeTag), false},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ok := IsTag(tc.err, testTag)
			require.Equal(t, ok, tc.want)
		})
	}
}

func TestIs(t *testing.T) {
	cases := []struct {
		desc string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"tag", Tag(constErr, testTag), true},
		{"mismatch", Tag(io.EOF, testTag), false},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ok := Is(tc.err, constErr)
			require.Equal(t, ok, tc.want)
		})
	}
}

func TestHasKey(t *testing.T) {
	const wantKey = "x"
	cases := []struct {
		desc string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not tagged", Errorf("notag: %w", constErr), false},
		{"no key", Tag(constErr, testTag), false},
		{"key", Tag(constErr, testTag, "x"), true},
		{"multiple keys match", Tag(constErr, testTag, "v", "1", "w", "2", "x", "3", "y", "4"), true},
		{"multiple keys mismatch", Tag(constErr, testTag, "v", "1", "w", "2"), false},
		{"wrapped", Errorf("wrap: %w", Tag(constErr, testTag, "x")), true},
		{"multi wrapped match", Tag(Errorf("wrap: %w", Tag(constErr, testTag, "x")), testTag, "y"), true},
		{"multi wrapped mismatch", Tag(Errorf("wrap: %w", Tag(constErr, testTag, "z")), testTag, "y"), false},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ok := HasKey(tc.err, wantKey)
			require.Equal(t, ok, tc.want)
		})
	}
}

func TestHasKeyValue(t *testing.T) {
	const wantKey, wantValue = "x", "1"
	cases := []struct {
		desc string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not tagged", constErr, false},
		{"key not value", Tag(constErr, testTag, "x"), false},
		{"key and value", Tag(constErr, testTag, "x", "1"), true},
		{"wrapped match", Errorf("wrap: %w", Tag(constErr, testTag, "x", "1")), true},
		{"wrapped mismatch", Errorf("wrap: %w", Tag(constErr, testTag, "x", "2")), false},
		{"multi keys match", Tag(constErr, testTag, "v", "1", "w", "2", "x", "1", "y"), true},
		{"multi keys mismatch", Tag(constErr, testTag, "v", "1", "w", "2", "x", "3", "y"), false},
		{"multi wrap match", Tag(Errorf("wrap: %w", Tag(constErr, testTag, "x", "1")), testTag, "v", "1"), true},
		{"multi wrap mismatch", Tag(Errorf("wrap: %w", Tag(constErr, testTag, "x", "2")), testTag, "v", "1"), false},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ok := HasKeyValue(tc.err, wantKey, wantValue)
			require.Equal(t, ok, tc.want)
		})
	}
}

func TestHasCode(t *testing.T) {
	cases := []struct {
		desc  string
		err   error
		codes []int
		want  bool
	}{
		{"nil", nil, []int{1}, false},
		{"not tagged", constErr, []int{1}, false},
		{"other key not value", Tag(constErr, testTag, "x"), []int{1}, false},
		{"other key and value", Tag(constErr, testTag, "x", "1"), []int{1}, false},
		{"other key and code match", Tag(constErr, testTag, "x", "1", "code", "1"), []int{1}, true},
		{"other key and code no match", Tag(constErr, testTag, "x", "1", "code", "a"), []int{1}, false},
		{"wrapped match", Errorf("wrap: %w", Tag(constErr, testTag, "code", "1")), []int{1}, true},
		{"wrapped mismatch", Errorf("wrap: %w", Tag(constErr, testTag, "code", "2")), []int{1}, false},
		{"multi codes match", Tag(constErr, testTag, "code", "1", "w", "2", "code", "1", "y"), []int{1}, true},
		{"multi codes mismatch", Tag(constErr, testTag, "v", "1", "codes", "2", "codes", "3", "y"), []int{1}, false},
		{"multi wrap match", Tag(Errorf("wrap: %w", Tag(constErr, testTag, "code", "1")), testTag, "code", "2"), []int{1}, true},
		{"multi wrap mismatch", Tag(Errorf("wrap: %w", Tag(constErr, testTag, "code", "2")), testTag, "v", "1"), []int{1}, false},
		{"wrapped match one of many", Errorf("wrap: %w", Tag(constErr, testTag, "code", "2")), []int{1, 2, 3}, true},
		{"wrapped match none of many", Errorf("wrap: %w", Tag(constErr, testTag, "code", "2")), []int{1, 3}, false},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			ok := HasCode(tc.err, tc.codes...)
			require.Equal(t, ok, tc.want)
		})
	}
}
