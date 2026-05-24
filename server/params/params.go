// Package params implements a decoder of HTTP parameters from the query
// string, path parameters and request body into a struct.
package params

import (
	"encoding/base64"
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"codeberg.org/mna/karbur/errors"
	"github.com/gorilla/schema"
)

// Decoder decodes request parameters into a struct and validates the values if
// the struct implements Validator. It uses strict decoding, where unknown
// fields and unexpected data raises errors.
//
// The Decoder is safe for concurrent use. Use the "schema" struct tag for form
// decoding, "json" for JSON unmarshaling, "path" to decode path values, and
// "cookie" to decode a cookie value into a field.
//
// For cookie decoding other than the "raw" option, it assumes the value is
// base64URL encoded without any padding.
type Decoder struct {
	once  sync.Once
	form  func(v any, vals map[string][]string) error
	path  func(v any, vals map[string][]string) error
	cache sync.Map
}

// Validator defines the method required for a type to validate itself.
type Validator interface {
	Validate() error
}

type cacheEntry struct {
	// the path values to extract for decoding into values of this type
	pathValues   []string
	cookieValues []*cookieEntry
}

type cookieEntry struct {
	cookieName string
	// by default, the cookie is assumed to be base64-encoded and the value is
	// set base64-decoded. By setting raw as option, the value is set as-is.
	raw bool
	// by default, the cookie value is assigned to the field after
	// base64-decoding. By setting asJSON as option, the base64-decoded value is
	// JSON-unmarshaled into the field.
	asJSON bool
	dst    reflect.StructField
}

func (d *Decoder) cacheGet(t reflect.Type) (entry cacheEntry, found bool) {
	v, ok := d.cache.Load(t)
	if ok {
		return v.(cacheEntry), true
	}
	return cacheEntry{}, false
}

func (d *Decoder) cacheSet(t reflect.Type) (entry cacheEntry, didSet bool) {
	fields := reflect.VisibleFields(t)
	for _, f := range fields {
		if !f.IsExported() {
			continue
		}
		if path := strings.TrimSpace(f.Tag.Get("path")); path != "" && path != "-" {
			entry.pathValues = append(entry.pathValues, path)
		}
		if ck := strings.TrimSpace(f.Tag.Get("cookie")); ck != "" && ck != "-" {
			name, opts, _ := strings.Cut(ck, ",")
			entry.cookieValues = append(entry.cookieValues, &cookieEntry{
				cookieName: strings.TrimSpace(name),
				dst:        f,
				raw:        strings.TrimSpace(opts) == "raw",
				asJSON:     strings.TrimSpace(opts) == "asJSON",
			})
		}
	}

	v, loaded := d.cache.LoadOrStore(t, entry)
	return v.(cacheEntry), !loaded
}

func (d *Decoder) init() {
	decForm := schema.NewDecoder()
	decForm.IgnoreUnknownKeys(false)
	decForm.MaxSize(100)
	decForm.ZeroEmpty(true)
	d.form = decForm.Decode

	decPath := schema.NewDecoder()
	decPath.SetAliasTag("path")
	decPath.IgnoreUnknownKeys(false)
	decPath.MaxSize(1)
	decPath.ZeroEmpty(true)
	d.path = decPath.Decode
}

func (d *Decoder) decodeCookie(r *http.Request, v reflect.Value, entry *cookieEntry) error {
	fld := v.FieldByIndex(entry.dst.Index)
	if !fld.CanAddr() || !fld.CanSet() {
		return errors.Errorf("params: struct field %s is not addressable or settable", entry.dst.Name)
	}

	ck, _ := r.Cookie(entry.cookieName)
	if ck == nil {
		// no such cookie, continue
		return nil
	}

	ckVal := ck.Value
	if !entry.raw {
		b, err := base64.RawURLEncoding.DecodeString(ckVal)
		if err != nil {
			return errors.Errorf("params: cookie %s is not base64-url-encoded: %w", ck.Name, err)
		}
		ckVal = string(b)
	}

	if entry.asJSON {
		if fld.Kind() != reflect.Pointer {
			fld = fld.Addr()
		}
		jsonDec := json.NewDecoder(strings.NewReader(ckVal))
		jsonDec.DisallowUnknownFields()
		if err := jsonDec.Decode(fld.Interface()); err != nil {
			return errors.Errorf("params: cookie %s failed to JSON unmarshal: %w", ck.Name, err)
		}
		if jsonDec.More() {
			return errors.Errorf("params: cookie %s JSON contains extraneous values", ck.Name)
		}
		return nil
	}

	if fld.Kind() == reflect.Pointer {
		if fld.IsNil() {
			if fld.Elem().Kind() != reflect.String {
				return errors.Errorf("params: struct field %s must be a string or pointer to string", entry.dst.Name)
			}
			fld.Set(reflect.ValueOf(new(ckVal)))
			return nil
		}
		fld = fld.Elem()
	}
	if fld.Kind() != reflect.String {
		return errors.Errorf("params: struct field %s must be a string or pointer to string", entry.dst.Name)
	}
	fld.SetString(ckVal)
	return nil
}

// Decode decodes the http parameters from r into dst. Body decoding depends on
// the content-type header.
func (d *Decoder) Decode(r *http.Request, dst any) error {
	d.once.Do(d.init)

	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return errors.New("params: interface must be a pointer to struct")
	}
	v = v.Elem()
	t := v.Type()
	cache, found := d.cacheGet(t)
	if !found {
		cache, _ = d.cacheSet(t)
	}

	var pathValues url.Values
	if len(cache.pathValues) > 0 {
		pathValues = make(url.Values, len(cache.pathValues))
		for _, v := range cache.pathValues {
			pathValues[v] = []string{r.PathValue(v)}
		}
	}

	var jsonDec *json.Decoder
	var formValues url.Values
	if r.ContentLength != 0 {
		switch ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct {
		case "application/x-www-form-urlencoded":
			if err := r.ParseForm(); err != nil {
				return err
			}
			formValues = r.Form

		case "application/json":
			jsonDec = json.NewDecoder(r.Body)
			jsonDec.DisallowUnknownFields()

		default:
			return errors.Errorf("params: unsupported content type: %s", ct)
		}
	}

	if formValues == nil {
		formValues = r.URL.Query()
	}

	if len(pathValues) > 0 {
		if err := d.path(dst, pathValues); err != nil {
			return err
		}
	}
	for _, ck := range cache.cookieValues {
		if err := d.decodeCookie(r, v, ck); err != nil {
			return err
		}
	}
	if len(formValues) > 0 {
		if err := d.form(dst, formValues); err != nil {
			return err
		}
	}
	if jsonDec != nil {
		if err := jsonDec.Decode(dst); err != nil {
			return err
		}
		if jsonDec.More() {
			return errors.New("params: JSON body contains extraneous values")
		}
	}

	if val, ok := dst.(Validator); ok {
		return val.Validate()
	}
	return nil
}
