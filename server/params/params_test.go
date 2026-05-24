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

	"github.com/gorilla/schema"
	"github.com/stretchr/testify/require"
)

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
