package params

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"codeberg.org/mna/karbur/errors"
	"github.com/gorilla/schema"
	"github.com/stretchr/testify/require"
)

type validator struct {
	I int    `schema:"i"`
	S string `schema:"s"`
}

func (v *validator) Validate() error {
	if v.I > 2 {
		return errors.New("value too high")
	}
	if len(v.S) > 10 {
		return errors.New("string too long")
	}
	return nil
}

func TestDecoder_Decode(t *testing.T) {
	var decodeFn func(r *http.Request)

	mux := http.NewServeMux()
	mux.Handle("/params/{id}/{rest...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeFn(r)
		w.WriteHeader(204)
	}))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeFn(r)
		w.WriteHeader(204)
	}))

	srv := httptest.NewServer(mux)
	defer srv.Close() //nolint

	newRequest := func(method, rawURL string, body any, bodyAsForm bool, cookies ...string) *http.Request {
		var buf bytes.Buffer

		if body != nil {
			if bodyAsForm {
				form := make(url.Values)
				enc := schema.NewEncoder()
				err := enc.Encode(body, form)
				require.NoError(t, err)
				_, _ = buf.WriteString(form.Encode())
			} else {
				enc := json.NewEncoder(&buf)
				err := enc.Encode(body)
				require.NoError(t, err)
			}
		}
		req, err := http.NewRequest(method, srv.URL+rawURL, &buf)
		require.NoError(t, err)

		if body != nil {
			if bodyAsForm {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			} else {
				req.Header.Set("Content-Type", "application/json")
			}
		}
		for i := 0; i < len(cookies); i += 2 {
			name, val := cookies[i], cookies[i+1]
			val = base64.RawURLEncoding.EncodeToString([]byte(val))
			req.AddCookie(&http.Cookie{Name: name, Value: val})
		}
		return req
	}

	type AID struct {
		A  int `schema:"a"`
		ID int `path:"id" schema:"-"`
	}

	type IDCk1 struct {
		ID  int    `path:"id" schema:"-"`
		Ck1 string `cookie:"ck1" schema:"-"`
	}

	cases := []struct {
		desc    string
		r       *http.Request
		dst     any
		want    any
		wantErr string
	}{
		{
			"not an interface to struct",
			newRequest("GET", "/", nil, false),
			struct{}{},
			nil,
			"must be a pointer to struct",
		},
		{
			"no params, empty destination",
			newRequest("GET", "/", nil, false),
			&struct{}{},
			struct{}{},
			"",
		},
		{
			"query string, empty destination",
			newRequest("GET", "/?q=a", nil, false),
			&struct{}{},
			nil,
			`invalid path "q"`,
		},
		{
			"form body, empty destination",
			newRequest("POST", "/", &struct{ V string }{V: "a"}, true),
			&struct{}{},
			nil,
			`invalid path "V"`,
		},
		{
			"json body, empty destination",
			newRequest("POST", "/", &struct{ V string }{V: "a"}, false),
			&struct{}{},
			nil,
			`unknown field "V"`,
		},
		{
			// this case is fine because path values not in the struct are not
			// attempted to be decoded (there is no way to know about existing path
			// values).
			"path value, empty destination",
			newRequest("GET", "/params/123/abc", nil, false),
			&struct{}{},
			struct{}{},
			``,
		},
		{
			// existing cookie values without a destination are fine, it isn't
			// expected that all cookies should be decoded into a struct.
			"cookie value, empty destination",
			newRequest("POST", "/", nil, false, "ck1", "v1"),
			&struct{}{},
			struct{}{},
			``,
		},
		{
			"query string only",
			newRequest("GET", "/?a=1&b=2", nil, false),
			&struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{},
			struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 1, B: "2"},
			``,
		},
		{
			"query string defaults override",
			newRequest("GET", "/?a=1&b", nil, false),
			&struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 123, B: "abc"},
			struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 1, B: ""},
			``,
		},
		{
			"query string defaults untouched",
			newRequest("GET", "/", nil, false),
			&struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 123, B: "abc"},
			struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 123, B: "abc"},
			``,
		},
		{
			"form body defaults override",
			newRequest("POST", "/", &struct {
				A int
				B string
			}{A: 1, B: ""}, true),
			&struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 123, B: "abc"},
			struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 1, B: ""},
			``,
		},
		{
			"form body defaults untouched",
			newRequest("POST", "/", &struct{}{}, true),
			&struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 123, B: "abc"},
			struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 123, B: "abc"},
			``,
		},
		{
			"json body defaults override",
			newRequest("POST", "/", &struct {
				A int
				B string
			}{A: 1, B: ""}, false),
			&struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 123, B: "abc"},
			struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 1, B: ""},
			``,
		},
		{
			"json body defaults untouched",
			newRequest("POST", "/", &struct{}{}, false),
			&struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 123, B: "abc"},
			struct {
				A int    `schema:"a"`
				B string `schema:"b"`
			}{A: 123, B: "abc"},
			``,
		},
		{
			"path value defaults override",
			newRequest("GET", "/params/123/abc", nil, false),
			&struct {
				ID   int    `path:"id"`
				Rest string `path:"rest"`
			}{ID: 999, Rest: "zzz"},
			struct {
				ID   int    `path:"id"`
				Rest string `path:"rest"`
			}{ID: 123, Rest: "abc"},
			``,
		},
		{
			// again, path values are a bit different, the struct is saying those
			// path values should be there, so they are read as empty, which
			// overrides the defaults.
			"path value defaults cleared",
			newRequest("GET", "/", nil, false),
			&struct {
				ID   int    `path:"id"`
				Rest string `path:"rest"`
			}{ID: 999, Rest: "zzz"},
			struct {
				ID   int    `path:"id"`
				Rest string `path:"rest"`
			}{ID: 0, Rest: ""},
			``,
		},
		{
			"cookie value defaults override",
			newRequest("GET", "/", nil, false, "ck1", "val1", "ck2", ""),
			&struct {
				Ck1 string `cookie:"ck1"`
				Ck2 string `cookie:"ck2"`
			}{Ck1: "zzz", Ck2: "zzz"},
			struct {
				Ck1 string `cookie:"ck1"`
				Ck2 string `cookie:"ck2"`
			}{Ck1: "val1", Ck2: ""},
			``,
		},
		{
			"cookie value defaults untouched",
			newRequest("GET", "/", nil, false, "ck3", "val3"),
			&struct {
				Ck1 string `cookie:"ck1"`
				Ck2 string `cookie:"ck2"`
			}{Ck1: "zzz", Ck2: "zzz"},
			struct {
				Ck1 string `cookie:"ck1"`
				Ck2 string `cookie:"ck2"`
			}{Ck1: "zzz", Ck2: "zzz"},
			``,
		},
		{
			"cookie raw and json",
			newRequest("GET", "/", nil, false, "ck1", "a", "ck2", `{"x":1}`),
			&struct {
				Ck1 string `cookie:"ck1,raw"`
				Ck2 struct {
					X int
				} `cookie:"ck2,json"`
			}{},
			struct {
				Ck1 string `cookie:"ck1,raw"`
				Ck2 struct {
					X int
				} `cookie:"ck2,json"`
			}{Ck1: base64.RawURLEncoding.EncodeToString([]byte(`a`)), Ck2: struct {
				X int
			}{X: 1}},
			``,
		},
		{
			// gorilla/schema uses the last value provided in url.Values as the one
			// to set a field to, so query string wins in this case.
			"query and form body",
			newRequest("POST", "/?B=y&C=3", &struct {
				A int
				B string
			}{A: 2, B: "z"}, true),
			&struct {
				A int    `schema:"A"`
				B string `schema:"B"`
				C int    `schema:"C"`
			}{},
			struct {
				A int    `schema:"A"`
				B string `schema:"B"`
				C int    `schema:"C"`
			}{A: 2, B: "y", C: 3},
			``,
		},
		{
			"query and json body, implicit targets",
			newRequest("POST", "/?a=1", &struct {
				B string `json:"b"`
			}{B: "z"}, false),
			&struct {
				A int
				B string
			}{},
			struct {
				A int
				B string
			}{A: 1, B: "z"},
			``,
		},
		{
			"query and json body, implicit conflicts",
			newRequest("POST", "/?a=1&b=y", &struct {
				A int    `json:"a"`
				B string `json:"b"`
			}{A: 2, B: "z"}, false),
			&struct {
				A int    `schema:"a"`
				B string `json:"b"`
			}{},
			struct {
				A int    `schema:"a"`
				B string `json:"b"`
			}{A: 2, B: "z"},
			``,
		},
		{
			"query and json body, explicit conflict resolution",
			newRequest("POST", "/?a=1&b=y", &struct {
				A int    `json:"a"`
				B string `json:"b"`
			}{A: 2, B: "z"}, false),
			&struct {
				A int    `schema:"a" json:"-"`
				B string `json:"b" schema:"-"`
			}{},
			nil,
			`invalid path "b"`,
		},
		{
			"embedded struct query and path",
			newRequest("GET", "/params/3/ok?a=4", nil, false),
			&struct {
				AID
				Rest string `path:"rest" schema:"-"`
			}{},
			struct {
				AID
				Rest string `path:"rest" schema:"-"`
			}{AID: AID{A: 4, ID: 3}, Rest: "ok"},
			``,
		},
		{
			"embedded struct pointer cookie and path",
			newRequest("GET", "/params/3/ok", nil, false, "ck1", "abc"),
			&struct {
				*IDCk1
			}{},
			struct {
				*IDCk1
			}{IDCk1: &IDCk1{ID: 3, Ck1: "abc"}},
			``,
		},
		{
			"everything form",
			newRequest("POST", "/params/1/end?a=2", &struct {
				B string `schema:"b"`
				C int    `schema:"c"`
			}{B: "z", C: 3}, true, "ck1", "abc"),
			&struct {
				A    int    `schema:"a"`
				B    string `schema:"b"`
				C    int    `schema:"c"`
				ID   int    `path:"id" schema:"-"`
				Rest string `path:"rest" schema:"-"`
				Ck1  string `cookie:"ck1" schema:"-"`
			}{},
			struct {
				A    int    `schema:"a"`
				B    string `schema:"b"`
				C    int    `schema:"c"`
				ID   int    `path:"id" schema:"-"`
				Rest string `path:"rest" schema:"-"`
				Ck1  string `cookie:"ck1" schema:"-"`
			}{A: 2, B: "z", C: 3, ID: 1, Rest: "end", Ck1: "abc"},
			``,
		},
		{
			"everything json",
			newRequest("POST", "/params/1/end?a=2", &struct {
				B string `json:"b"`
				C int    `json:"c"`
			}{B: "z", C: 3}, false, "ck1", "abc"),
			&struct {
				A    int    `schema:"a" json:"-"`
				B    string `json:"b" schema:"-"`
				C    int    `json:"c" schema:"-"`
				ID   int    `path:"id" schema:"-" json:"-"`
				Rest string `path:"rest" schema:"-" json:"-"`
				Ck1  string `cookie:"ck1" schema:"-" json:"-"`
			}{},
			struct {
				A    int    `schema:"a" json:"-"`
				B    string `json:"b" schema:"-"`
				C    int    `json:"c" schema:"-"`
				ID   int    `path:"id" schema:"-" json:"-"`
				Rest string `path:"rest" schema:"-" json:"-"`
				Ck1  string `cookie:"ck1" schema:"-" json:"-"`
			}{A: 2, B: "z", C: 3, ID: 1, Rest: "end", Ck1: "abc"},
			``,
		},
		{
			"cookie value into non-string",
			newRequest("GET", "/", nil, false, "ck1", "val1"),
			&struct {
				Ck1 int `cookie:"ck1"`
			}{},
			nil,
			`must be a string or pointer to string`,
		},
		{
			"validator fails",
			newRequest("GET", "/?i=3&s=abc", nil, false),
			&validator{},
			nil,
			`value too high`,
		},
		{
			"validator fails 2",
			newRequest("GET", "/?i=1&s=abcdefghijklm", nil, false),
			&validator{},
			nil,
			`string too long`,
		},
		{
			"validator succeeds",
			newRequest("GET", "/?i=1&s=abcd", nil, false),
			&validator{},
			validator{
				I: 1,
				S: "abcd",
			},
			``,
		},
	}
	var dec Decoder
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			decodeFn = func(r *http.Request) {
				err := dec.Decode(r, c.dst)
				if c.wantErr != "" {
					require.ErrorContains(t, err, c.wantErr)
				} else {
					require.NoError(t, err)
					v := reflect.ValueOf(c.dst)
					got := v.Elem().Interface()
					require.Equal(t, c.want, got)
				}
			}
			_, err := http.DefaultClient.Do(c.r)
			require.NoError(t, err)
		})
	}
}
