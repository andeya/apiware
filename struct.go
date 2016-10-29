// Copyright 2016 HenryLee. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apiware

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
)

/*
Param tag value description:
    tag   |   key    | required |     value     |   desc
    ------|----------|----------|---------------|----------------------------------
    param |   type   | only one |     path      | if `required` is unsetted, auto set it. e.g. url: "http://www.abc.com/a/{path}"
    param |   type   | only one |     query     | e.g. url: "http://www.abc.com/a?b={query}"
    param |   type   | only one |     formData  | e.g. "request body: a=123&b={formData}"
    param |   type   | only one |     body      | request body can be any content
    param |   type   | only one |     header    | request header info
    param |   type   | only one |     cookie    | request cookie info, support type: `http.Cookie`,`fasthttp.Cookie`,`string`,`[]byte`
    param |   name   |    no    |  (e.g. "id")  | specify request param`s name
    param | required |    no    |   required    | request param is required
    param |   desc   |    no    |  (e.g. "id")  | request param description
    param |   len    |    no    | (e.g. 3:6, 3) | length range of param
    param |   range  |    no    |  (e.g. 0:10)  | numerical range of param
    param |  nonzero |    no    |    nonzero    | param`s value can not be zero
    param |   maxmb  |    no    |   (e.g. 32)   | when request Content-Type is multipart/form-data, the max memory for body.(multi-param, whichever is greater)
    regexp|          |    no    |(e.g. "^\\w+$")| param value can not be null
    err   |          |    no    |(e.g. "incorrect password format")| customize the prompt for validation error

    NOTES:
        1. the binding object must be a struct pointer
        2. the binding struct field can not be a pointer
        3. `regexp` or `param` tag is only usable when `param:"type(xxx)"` is exist
        4. if the `param` tag is not exist, anonymous field will be parsed
        5. when param type is `formData` and field type is `multipart.FileHeader`, the field receives file uploaded
        6. if param type is `cookie`, field type must be `http.Cookie`
        7. `formData` and `body` params can not exist at the same time
        8. there should not be more than one `body` param

List of supported param value types:
    base    |   slice    | special
    --------|------------|-------------------------------------------------------
    string  |  []string  | [][]byte
    byte    |  []byte    | [][]uint8
    uint8   |  []uint8   | multipart.FileHeader (only for `formData` param)
    bool    |  []bool    | http.Cookie (only for `net/http`'s `cookie` param)
    int     |  []int     | fasthttp.Cookie (only for `fasthttp`'s `cookie` param)
    int8    |  []int8    | struct (struct type only for `body` param or as an anonymous field to extend params)
    int16   |  []int16   |
    int32   |  []int32   |
    int64   |  []int64   |
    uint8   |  []uint8   |
    uint16  |  []uint16  |
    uint32  |  []uint32  |
    uint64  |  []uint64  |
    float32 |  []float32 |
    float64 |  []float64 |
*/

const (
	TAG_PARAM        = "param"  //request param tag name
	TAG_REGEXP       = "regexp" //regexp validate tag name(optio)
	TAG_ERR          = "err"    //customize the prompt for validation error(optio)
	TAG_IGNORE_PARAM = "-"      //ignore request param tag value

	MB                 = 1 << 20 // 1MB
	defaultMaxMemory   = 32 * MB // 32 MB
	defaultMaxMemoryMB = 32
)

var (
	ParamTypes = map[string]bool{
		"path":     true,
		"query":    true,
		"formData": true,
		"body":     true,
		"header":   true,
		"cookie":   true,
	}
)

type (
	// StructField represents a schema field of a parsed model.
	StructField struct {
		Index      int
		Name       string            // Field name
		Value      reflect.Value     // Value
		isRequired bool              // file is required or not
		isFile     bool              // is file field or not
		Tags       map[string]string // Struct tags for this field
		RawTag     reflect.StructTag // The raw tag
	}

	// Struct represents a parsed schema interface{}.
	Struct struct {
		Name   string
		Fields []*StructField
		//when request Content-Type is multipart/form-data, the max memory for body.
		MaxMemory int64
		//used to create a new struct (non-pointer)
		structType reflect.Type
		//the value of the struct (non-pointer)
		structValue reflect.Value
	}

	// Schema is a collection of Struct
	Schema struct {
		lib map[string]Struct
		sync.RWMutex
	}
)

var (
	defaultSchema = &Schema{
		lib: map[string]Struct{},
	}
)

const (
	fileTypeString           = "multipart.FileHeader"
	cookieTypeString         = "http.Cookie"
	fasthttpCookieTypeString = "fasthttp.Cookie"
	stringTypeString         = "string"
	bytesTypeString          = "[]byte"
	bytes2TypeString         = "[]uint8"
)

// Parse and store the structure object, requires a structure pointer,
// if `paramNameFunc` is not setted, `paramNameFunc=toSnake`.
func ToStruct(structReceiverPtr interface{}, paramNameFunc ...ParamNameFunc) (*Struct, error) {
	v := reflect.ValueOf(structReceiverPtr)
	if v.Kind() != reflect.Ptr {
		return nil, NewError(reflect.TypeOf(structReceiverPtr).String(), "*", "the binding object must be a struct pointer")
	}
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct {
		return nil, NewError(reflect.TypeOf(structReceiverPtr).String(), "*", "the binding object must be a struct pointer")
	}
	t := v.Type()
	name := t.String()
	m, ok := defaultSchema.get(name)
	if ok {
		m.structValue = v
		fields := make([]*StructField, len(m.Fields))
		for i, field := range m.Fields {
			fields[i] = &StructField{
				Index:      field.Index,
				Name:       field.Name,
				Value:      v.Field(field.Index),
				isRequired: field.isRequired,
				isFile:     field.isFile,
				Tags:       field.Tags,
				RawTag:     field.RawTag,
			}
		}
		m.Fields = fields
		return &m, nil
	}

	m.Name = name
	m.Fields = []*StructField{}
	m.structType = t
	m.structValue = v

	var err error
	if len(paramNameFunc) > 0 {
		err = addFields(&m, t, v, paramNameFunc[0])
	} else {
		err = addFields(&m, t, v, toSnake)
	}
	if err != nil {
		return nil, err
	}
	defaultSchema.set(m)
	return &m, nil
}

// get the `Struct` object according to the type name
func GetStruct(typeName string) (Struct, bool) {
	return defaultSchema.get(typeName)
}

// cache `Struct`
func SetStruct(m Struct) {
	defaultSchema.set(m)
}

func (schema *Schema) get(typeName string) (Struct, bool) {
	schema.RLock()
	defer schema.RUnlock()
	m, ok := schema.lib[typeName]
	return m, ok
}

func (schema *Schema) set(m Struct) {
	schema.Lock()
	schema.lib[m.Name] = m
	defer schema.Unlock()
}

func addFields(m *Struct, t reflect.Type, v reflect.Value, paramNameFunc ParamNameFunc) error {
	var err error
	var maxMemoryMB int64
	var hasFormData, hasBody bool
	for i := 0; i < t.NumField(); i++ {
		var field = t.Field(i)

		tag, ok := field.Tag.Lookup(TAG_PARAM)
		if !ok {
			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				if err = addFields(m, field.Type, v.Field(i), paramNameFunc); err != nil {
					return err
				}
			}
			continue
		}

		if tag == TAG_IGNORE_PARAM {
			continue
		}

		if field.Type.Kind() == reflect.Ptr {
			return NewError(t.String(), field.Name, "field can not be a pointer")
		}

		var parsedTags = parseTags(tag)
		var paramType = parsedTags["type"]
		var fieldTypeString = field.Type.String()

		switch fieldTypeString {
		case fileTypeString:
			if paramType != "formData" {
				return NewError(t.String(), field.Name, "when field type is `"+fieldTypeString+"`, param type must be `formData`")
			}
		case cookieTypeString, fasthttpCookieTypeString:
			if paramType != "cookie" {
				return NewError(t.String(), field.Name, "when field type is `"+fieldTypeString+"`, param type must be `cookie`")
			}
		}

		switch paramType {
		case "formData":
			if hasBody {
				return NewError(t.String(), field.Name, "`formData` and `body` params can not exist at the same time")
			}
			hasFormData = true
		case "body":
			if hasFormData {
				return NewError(t.String(), field.Name, "`formData` and `body` params can not exist at the same time")
			}
			if hasBody {
				return NewError(t.String(), field.Name, "there should not be more than one `body` param")
			}
			hasBody = true
		case "path":
			parsedTags["required"] = "required"
		case "cookie":
			switch fieldTypeString {
			case cookieTypeString, fasthttpCookieTypeString, stringTypeString, bytesTypeString, bytes2TypeString:
			default:
				return NewError(t.String(), field.Name, "invalid field type for `cookie` param, refer to the following: `http.Cookie`, `fasthttp.Cookie`, `string`, `[]byte` or `[]uint8`")
			}
		default:
			if !ParamTypes[paramType] {
				return NewError(t.String(), field.Name, "invalid param type, refer to the following: `path`, `query`, `formData`, `body`, `header` or `cookie`")
			}
		}

		if a, ok := parsedTags["maxmb"]; ok {
			i, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				return NewError(t.String(), field.Name, "invalid `maxmb` tag, it must be positive integer")
			}
			if i > maxMemoryMB {
				maxMemoryMB = i
			}
		}

		if r, ok := field.Tag.Lookup(TAG_REGEXP); ok {
			parsedTags[TAG_REGEXP] = r
		}

		if errStr, ok := field.Tag.Lookup(TAG_ERR); ok {
			parsedTags[TAG_ERR] = errStr
		}

		// fmt.Printf("%#v\n", parsedTags)

		fd := &StructField{
			Index:  i,
			Value:  v.Field(i),
			Tags:   parsedTags,
			RawTag: field.Tag,
		}

		if fd.Name, ok = parsedTags["name"]; !ok {
			fd.Name = paramNameFunc(field.Name)
		}

		fd.isFile = fd.Value.Type().Name() == fileTypeString
		_, fd.isRequired = parsedTags["required"]

		m.Fields = append(m.Fields, fd)
	}
	if maxMemoryMB > 0 {
		m.MaxMemory = maxMemoryMB * MB
	} else {
		m.MaxMemory = defaultMaxMemory
	}
	return nil
}

func parseTags(s string) map[string]string {
	c := strings.Split(s, ",")
	m := make(map[string]string)
	for _, v := range c {
		c2 := strings.Split(v, "(")
		if len(c2) == 2 && len(c2[1]) > 1 {
			m[c2[0]] = c2[1][:len(c2[1])-1]
		} else {
			m[v] = ""
		}
	}
	return m
}

// Create a copy `*Struct`
func (model *Struct) Copy() *Struct {
	var newStruct = new(Struct)
	*newStruct = *model
	newStruct.structValue = reflect.New(model.structType).Elem()
	fields := make([]*StructField, len(model.Fields))
	for i, field := range model.Fields {
		fields[i] = &StructField{
			Index:      field.Index,
			Name:       field.Name,
			Value:      newStruct.structValue.Field(field.Index),
			isRequired: field.isRequired,
			isFile:     field.isFile,
			Tags:       field.Tags,
			RawTag:     field.RawTag,
		}
	}
	newStruct.Fields = fields
	return newStruct
}

// Gets the object of the pointer type
func (model *Struct) Interface() interface{} {
	return model.structValue.Addr().Interface()
}

// Bind the net/http request params to the structure and validate.
// If the struct has not been registered, it will be registered at the same time.
// note: structReceiverPtr must be structure pointer.
func (model *Struct) BindParam(
	req *http.Request,
	pattern string,
	pathDecodeFunc PathDecodeFunc,
	bodyDecodeFunc BodyDecodeFunc,
) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = NewError(model.Name, "?", fmt.Sprint(p))
		}
	}()

	var query, formValues url.Values
	var params = pathDecodeFunc(req.URL.Path, pattern)
	for _, field := range model.Fields {
		switch field.Type() {
		case "path":
			paramValue, ok := params[field.Name]
			if !ok {
				return NewError(model.Name, field.Name, "missing path param")
			}
			// fmt.Printf("fieldName:%s\nvalue:%#v\n\n", field.Name, paramValue)
			err = convertAssign(field.Value, []string{paramValue})
			if err != nil {
				return NewError(model.Name, field.Name, err.Error())
			}

		case "query":
			if query == nil {
				query = req.URL.Query()
			}
			paramValues, ok := query[field.Name]
			if ok {
				err = convertAssign(field.Value, paramValues)
				if err != nil {
					return NewError(model.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(model.Name, field.Name, "missing query param")
			}

		case "formData":
			// Can not exist with `body` param at the same time
			if formValues == nil {
				err = req.ParseMultipartForm(model.MaxMemory)
				if err != nil {
					return NewError(model.Name, field.Name, err.Error())
				}
				formValues = req.PostForm
				if req.MultipartForm != nil {
					for k, v := range req.MultipartForm.Value {
						if _, ok := formValues[k]; ok {
							formValues[k] = append(formValues[k], v...)
						} else {
							formValues[k] = v
						}
					}
				}
			}

			if field.IsFile() && req.MultipartForm != nil && req.MultipartForm.File != nil {
				fhs := req.MultipartForm.File[field.Name]
				if len(fhs) == 0 {
					if field.IsRequired() {
						return NewError(model.Name, field.Name, "missing formData param")
					}
					continue
				}
				field.Value.Set(reflect.ValueOf(fhs[0]).Elem())
				continue
			}

			paramValues, ok := formValues[field.Name]
			if ok {
				err = convertAssign(field.Value, paramValues)
				if err != nil {
					return NewError(model.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(model.Name, field.Name, "missing formData param")
			}

		case "body":
			// Theoretically there should be at most one `body` param, and can not exist with `formData` at the same time
			body, err := ioutil.ReadAll(req.Body)
			req.Body.Close()
			if err == nil {
				err = bodyDecodeFunc(field.Value, body)
				if err != nil {
					return NewError(model.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(model.Name, field.Name, "missing body param")
			}

		case "header":
			paramValues, ok := req.Header[field.Name]
			if ok {
				err = convertAssign(field.Value, paramValues)
				if err != nil {
					return NewError(model.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(model.Name, field.Name, "missing header param")
			}

		case "cookie":
			c, _ := req.Cookie(field.Name)
			if c != nil {
				switch field.Value.Type().String() {
				case cookieTypeString:
					field.Value.Set(reflect.ValueOf(c).Elem())

				case stringTypeString:
					field.Value.Set(reflect.ValueOf(c.String()))

				case bytesTypeString, bytes2TypeString:
					field.Value.Set(reflect.ValueOf([]byte(c.String())))

				default:
					return NewError(model.Name, field.Name, "invalid cookie param type, it must be `http.Cookie`, `string` or `[]byte`")
				}
			} else if field.IsRequired() {
				return NewError(model.Name, field.Name, "missing cookie param")
			}
		}
	}
	return model.Validate()
}

// Bind the fasthttp request params to the structure and validate.
// If the struct has not been registered, it will be registered at the same time.
// note: structReceiverPtr must be structure pointer.
func (model *Struct) FasthttpBindParam(
	reqCtx *fasthttp.RequestCtx,
	pattern string,
	pathDecodeFunc PathDecodeFunc,
	bodyDecodeFunc BodyDecodeFunc,
) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = NewError(model.Name, "?", fmt.Sprint(p))
		}
	}()

	var formValues = fasthttpFormValues(reqCtx)
	var params = pathDecodeFunc(string(reqCtx.Path()), pattern)
	for _, field := range model.Fields {
		switch field.Type() {
		case "path":
			paramValue, ok := params[field.Name]
			if !ok {
				return NewError(model.Name, field.Name, "missing path param")
			}
			// fmt.Printf("fieldName:%s\nvalue:%#v\n\n", field.Name, paramValue)
			err = convertAssign(field.Value, []string{paramValue})
			if err != nil {
				return NewError(model.Name, field.Name, err.Error())
			}

		case "query":
			paramValuesBytes := reqCtx.QueryArgs().PeekMulti(field.Name)
			if len(paramValuesBytes) > 0 {
				var paramValues = make([]string, len(paramValuesBytes))
				for i, b := range paramValuesBytes {
					paramValues[i] = string(b)
				}
				err = convertAssign(field.Value, paramValues)
				if err != nil {
					return NewError(model.Name, field.Name, err.Error())
				}
			} else if len(paramValuesBytes) == 0 && field.IsRequired() {
				return NewError(model.Name, field.Name, "missing query param")
			}

		case "formData":
			// Can not exist with `body` param at the same time
			if field.IsFile() {
				fh, err := reqCtx.FormFile(field.Name)
				if err != nil {
					if field.IsRequired() {
						return NewError(model.Name, field.Name, "missing formData param")
					}
					continue
				}
				field.Value.Set(reflect.ValueOf(fh).Elem())
				continue
			}

			paramValues, ok := formValues[field.Name]
			if ok {
				err = convertAssign(field.Value, paramValues)
				if err != nil {
					return NewError(model.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(model.Name, field.Name, "missing formData param")
			}

		case "body":
			// Theoretically there should be at most one `body` param, and can not exist with `formData` at the same time
			body := reqCtx.PostBody()
			if body != nil {
				err = bodyDecodeFunc(field.Value, body)
				if err != nil {
					return NewError(model.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(model.Name, field.Name, "missing body param")
			}

		case "header":
			paramValueBytes := reqCtx.Request.Header.Peek(field.Name)
			if paramValueBytes != nil {
				err = convertAssign(field.Value, []string{string(paramValueBytes)})
				if err != nil {
					return NewError(model.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(model.Name, field.Name, "missing header param")
			}

		case "cookie":
			bcookie := reqCtx.Request.Header.Cookie(field.Name)
			if bcookie != nil {
				switch field.Value.Type().String() {
				case fasthttpCookieTypeString:
					c := fasthttp.AcquireCookie()
					defer fasthttp.ReleaseCookie(c)
					err = c.ParseBytes(bcookie)
					if err != nil {
						return NewError(model.Name, field.Name, err.Error())
					}
					field.Value.Set(reflect.ValueOf(*c))

				case stringTypeString:
					field.Value.Set(reflect.ValueOf(string(bcookie)))

				case bytesTypeString, bytes2TypeString:
					field.Value.Set(reflect.ValueOf(bcookie))

				default:
					return NewError(model.Name, field.Name, "invalid cookie param type, it must be `fasthttp.Cookie`, `string` or `[]byte`")
				}

			} else if field.IsRequired() {
				return NewError(model.Name, field.Name, "missing cookie param")
			}
		}
	}
	return model.Validate()
}

// Validate validates the provided struct
func Validate(f interface{}) error {
	model, err := ToStruct(f)
	if err != nil {
		return err
	}
	err = model.Validate()
	if err != nil {
		return err
	}
	return nil
}

// Validate validates the provided struct
func (model *Struct) Validate() error {
	for _, field := range model.Fields {
		err := field.Validate()
		if err != nil {
			return NewError(model.Name, field.Name, err.Error())
		}
	}
	return nil
}

// Validate tests if the field conforms to it's validation constraints specified
// int the TAG_REGEXP struct tag
func (field *StructField) Validate() (err error) {
	defer func() {
		p := recover()
		if errStr, ok := field.Tags[TAG_ERR]; ok {
			if err != nil {
				err = errors.New(errStr)
			}
		} else if p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()

	// length
	if tuple, ok := field.Tags["len"]; ok {
		s, ok := field.String()
		if ok {
			if err = validateLen(s, tuple, field.Name); err != nil {
				return err
			}
		}
	}
	// range
	if tuple, ok := field.Tags["range"]; ok {
		f64, ok := field.Float()
		if ok {
			if err = validateRange(f64, tuple, field.Name); err != nil {
				return err
			}
		}
	}
	// nonzero
	if _, ok := field.Tags["nonzero"]; ok {
		if field.IsZero() {
			return NewValidationError(ValidationErrorValueNotSet, field.Name)
		}
	}
	// regexp
	if reg, ok := field.Tags[TAG_REGEXP]; ok {
		s, ok := field.String()
		if ok {
			if err = validateRegexp(s, reg, field.Name); err != nil {
				return err
			}
		}
	}

	return
}

// Type returns the type value for the field
func (field *StructField) Type() string {
	return field.Tags["type"]
}

// IsRequired tests if the field is declared
func (field *StructField) IsRequired() bool {
	return field.isRequired
}

// Description returns the description value for the field
func (field *StructField) Description() string {
	return field.Tags["desc"]
}

// IsFile tests if the field is type *multipart.FileHeader
func (field *StructField) IsFile() bool {
	return field.isFile
}

// IsZero tests wether or not the field is set
func (field *StructField) IsZero() bool {
	x := field.Value
	return x.Interface() == reflect.Zero(x.Type()).Interface()
}

// String returns the field string value and a bool flag indicating if the
// conversion was successful
func (field *StructField) String() (string, bool) {
	t, ok := field.Value.Interface().(string)
	return t, ok
}

// Float returns the field int value and a bool flag indication if the conversion
// was successful
func (field *StructField) Float() (float64, bool) {
	switch field.Value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(field.Value.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(field.Value.Uint()), true
	case reflect.Float32, reflect.Float64:
		return field.Value.Float(), true
	}
	return 0, false
}

func parseTuple(tuple string) (string, string) {
	c := strings.Split(tuple, ":")
	var a, b string
	switch len(c) {
	case 1:
		a = c[0]
		if len(a) > 0 {
			return a, a
		}
	case 2:
		a = c[0]
		b = c[1]
		if len(a) > 0 || len(b) > 0 {
			return a, b
		}
	}
	panic("invalid validation tuple")
}

func validateLen(s, tuple, field string) error {
	a, b := parseTuple(tuple)
	if len(a) > 0 {
		min, err := strconv.Atoi(a)
		if err != nil {
			panic(err)
		}
		if len(s) < min {
			return NewValidationError(ValidationErrorValueTooShort, field)
		}
	}
	if len(b) > 0 {
		max, err := strconv.Atoi(b)
		if err != nil {
			panic(err)
		}
		if len(s) > max {
			return NewValidationError(ValidationErrorValueTooLong, field)
		}
	}
	return nil
}

const accuracy = 0.0000001

func validateRange(f64 float64, tuple, field string) error {
	a, b := parseTuple(tuple)
	if len(a) > 0 {
		min, err := strconv.ParseFloat(a, 64)
		if err != nil {
			return err
		}
		if math.Min(f64, min) == f64 && math.Abs(f64-min) > accuracy {
			return NewValidationError(ValidationErrorValueTooSmall, field)
		}
	}
	if len(b) > 0 {
		max, err := strconv.ParseFloat(b, 64)
		if err != nil {
			return err
		}
		if math.Max(f64, max) == f64 && math.Abs(f64-max) > accuracy {
			return NewValidationError(ValidationErrorValueTooBig, field)
		}
	}
	return nil
}

func validateRegexp(s, reg, field string) error {
	matched, err := regexp.MatchString(reg, s)
	if err != nil {
		return err
	}
	if !matched {
		return NewValidationError(ValidationErrorValueNotMatch, field)
	}
	return nil
}

// fasthttpFormValues returns all post data values with their keys
// multipart, formValues data, post arguments
func fasthttpFormValues(reqCtx *fasthttp.RequestCtx) (valuesAll map[string][]string) {
	valuesAll = make(map[string][]string)
	// first check if we have multipart formValues
	multipartForm, err := reqCtx.MultipartForm()
	if err == nil {
		//we have multipart formValues
		return multipartForm.Value
	}
	// if no multipart and post arguments ( means normal formValues   )
	if reqCtx.PostArgs().Len() == 0 {
		return // no found
	}
	reqCtx.PostArgs().VisitAll(func(k []byte, v []byte) {
		key := string(k)
		value := string(v)
		// for slices
		if valuesAll[key] != nil {
			valuesAll[key] = append(valuesAll[key], value)
		} else {
			valuesAll[key] = []string{value}
		}

	})
	return
}
