package errors

import (
	"fmt"
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

func TestTagOnce(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		err := TagOnce(nil, testTag)
		require.Nil(t, err)
	})

	t.Run("untagged no key value", func(t *testing.T) {
		err := TagOnce(constErr, testTag)
		require.NotNil(t, err)
		require.ErrorContains(t, err, string(testTag))
		require.ErrorContains(t, err, constErr.Error())
	})

	t.Run("already tagged same", func(t *testing.T) {
		err := Tag(constErr, testTag, "x", "1", "y", "2")
		err = TagOnce(err, testTag, "x", "1", "y", "2")
		require.ErrorContains(t, err, string(testTag))
		require.ErrorContains(t, err, "[x: 1, y: 2]")
	})

	t.Run("already tagged different", func(t *testing.T) {
		err := Tag(constErr, testTag, "x", "1", "y", "2")
		err = TagOnce(err, nopeTag, "a", "1", "b", "2")
		require.ErrorContains(t, err, string(testTag))
		require.ErrorContains(t, err, "[x: 1, y: 2]")
		require.NotContains(t, err.Error(), string(nopeTag))
		require.NotContains(t, err.Error(), "[a: 1, b: 2]")
	})

	t.Run("already tagged deeper", func(t *testing.T) {
		err := Tag(constErr, testTag, "x", "1", "y", "2")
		err = fmt.Errorf("deeper: %w", err)
		err = TagOnce(err, nopeTag)
		require.ErrorContains(t, err, string(testTag))
		require.ErrorContains(t, err, "[x: 1, y: 2]")
		require.NotContains(t, err.Error(), string(nopeTag))
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
			require.Equal(t, tc.want, ok)
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
			require.Equal(t, tc.want, ok)
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
			require.Equal(t, tc.want, ok)
		})
	}
}

func TestCode(t *testing.T) {
	cases := []struct {
		desc string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"not tagged", constErr, 0},
		{"other key not value", Tag(constErr, testTag, "x"), 0},
		{"other key and value", Tag(constErr, testTag, "x", "1"), 0},
		{"other key and code", Tag(constErr, testTag, "x", "1", "code", "1"), 1},
		{"other key and code not numeric", Tag(constErr, testTag, "x", "1", "code", "a"), 0},
		{"wrapped", Errorf("wrap: %w", Tag(constErr, testTag, "code", "2")), 2},
		{"multi codes", Tag(constErr, testTag, "code", "1", "w", "2", "code", "1", "y"), 1},
		{"multi wrap", Tag(Errorf("wrap: %w", Tag(constErr, testTag, "code", "1")), testTag, "code", "2"), 2},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := Code(tc.err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestWithKeyValue(t *testing.T) {
	cases := []struct {
		desc      string
		err       error
		wantValue string
	}{
		{"nil", nil, ""},
		{"not tagged", constErr, "v"},
		{"tagged no meta", Tag(constErr, testTag), "v"},
		{"tagged other key", Tag(constErr, testTag, "x", "y"), "v"},
		{"tagged same key", Tag(constErr, testTag, "k", "w"), "v"},
		{"wrapped", Errorf("wrap: %w", Tag(constErr, testTag, "x", "y")), "v"},
		{"multi wrap", Errorf("wrap2: %w", Errorf("wrap: %w", Tag(constErr, testTag, "k", "w"))), "v"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := WithKeyValue(tc.err, "k", "v")
			require.Equal(t, tc.wantValue, KeyValue(got, "k"))
			if got != nil {
				t.Log(got.Error())
			}
		})
	}
}

func TestUnwrapAll(t *testing.T) {
	cases := []struct {
		desc string
		err  error
		want error
	}{
		{"nil", nil, nil},
		{"not tagged", constErr, constErr},
		{"tagged", Tag(constErr, testTag), constErr},
		{"wrapped", Errorf("wrap: %w", Tag(constErr, testTag, "x", "y")), constErr},
		{"multi wrap", Errorf("wrap2: %w", Errorf("wrap: %w", Tag(constErr, testTag, "k", "w"))), constErr},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := UnwrapAll(tc.err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestUserMessage(t *testing.T) {
	cases := []struct {
		desc   string
		err    error
		defMsg string
		want   string
	}{
		{"nil with default", nil, "z", "z"},
		{"nil with root", nil, RootErrAsUserMsg, ""},
		{"unwrapped with default", constErr, "z", "z"},
		{"unwrapped with root", constErr, RootErrAsUserMsg, constErr.Error()},
		{"wrapped with default", fmt.Errorf("wrapped: %w", constErr), "z", "z"},
		{"wrapped with root", fmt.Errorf("wrapped: %w", constErr), RootErrAsUserMsg, constErr.Error()},
		{"tagged with user and default", Tag(constErr, testTag, UserMessageKey, "abc"), "z", "abc"},
		{"tagged with user and root", Tag(constErr, testTag, UserMessageKey, "abc"), RootErrAsUserMsg, "abc"},
		{"tagged with default", Tag(constErr, testTag, "x", "abc"), "z", "z"},
		{"tagged with root", Tag(constErr, testTag, "x", "abc"), RootErrAsUserMsg, constErr.Error()},
		{"tagged deep with user and default", fmt.Errorf("wrapped: %w", Tag(fmt.Errorf("deep: %w", constErr), testTag, UserMessageKey, "xyz")), "z", "xyz"},
		{"tagged deep with user and root", fmt.Errorf("wrapped: %w", Tag(fmt.Errorf("deep: %w", constErr), testTag, UserMessageKey, "xyz")), RootErrAsUserMsg, "xyz"},
		{"tagged deep default", fmt.Errorf("wrapped: %w", Tag(fmt.Errorf("deep: %w", constErr), testTag, "notuser", "xyz")), "z", "z"},
		{"tagged deep root", fmt.Errorf("wrapped: %w", Tag(fmt.Errorf("deep: %w", constErr), testTag, "notuser", "xyz")), RootErrAsUserMsg, constErr.Error()},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := UserMessage(tc.err, tc.defMsg)
			require.Equal(t, tc.want, got)
		})
	}
}
