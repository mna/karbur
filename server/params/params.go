// Package params implements a parser to decode HTTP parameters from the query
// string, path parameters and request body into a struct.
package params

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"sync"

	"codeberg.org/mna/karbur/errors"
	"github.com/gorilla/schema"
)

// Parser decodes request parameters into a struct and validates
// the values if the struct implements Validator.
//
// The Parser is safe for concurrent use.
type Parser struct {
	// Form is the function to use to decode form values from a map.
	// If it is nil, gorilla/schema.Decoder.Decode is used, configured to fail
	// on unknown keys.
	Form func(v any, vals map[string][]string) error

	// JSON is the function to use to unmarshal JSON from a
	// slice of bytes. If it is nil, json.Unmarshal from the
	// standard library is used.
	JSON func(data []byte, v any) error

	once sync.Once
	form func(v any, vals map[string][]string) error
	json func(data []byte, v any) error
	// TODO: cache of path fields for a given type
}

// Validator defines the method required for a type to validate itself.
type Validator interface {
	Validate() error
}

func (p *Parser) init() {
	p.form = p.Form
	if p.form == nil {
		dec := schema.NewDecoder()
		dec.IgnoreUnknownKeys(false)
		dec.MaxSize(100)
		dec.ZeroEmpty(true)
		p.form = dec.Decode
	}
	p.json = p.JSON
	if p.json == nil {
		p.json = json.Unmarshal
	}
}

func (p *Parser) Parse(r *http.Request, dst any) error {
	p.once.Do(p.init)

	var jsonBody []byte
	formValues := r.URL.Query()
	if r.ContentLength != 0 {
		switch ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct {
		case "application/x-www-form-urlencoded":
			if err := r.ParseForm(); err != nil {
				return err
			}
			formValues = r.Form

		case "application/json":
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return err
			}
			jsonBody = b

		default:
			return errors.Errorf("unsupported content type: %s", ct)
		}
	}

	if len(formValues) > 0 {
		if err := p.form(dst, formValues); err != nil {
			return err
		}
	}
	if len(jsonBody) > 0 {
		if err := p.json(jsonBody, dst); err != nil {
			return err
		}
	}

	if val, ok := dst.(Validator); ok {
		return val.Validate()
	}
	return nil
}
