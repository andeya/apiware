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
//
// Package apiware provides a tools which can bind the http/fasthttp request parameters to the structure and validate.
package apiware

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

/**
 * param tag value description:
 * tag   |   key    | required |     value     |   desc
 * ------|----------|----------|---------------|----------------------------------
 * param |   type   | only one |     path      | when 'required' is unsetted, auto set it. e.g. url: "http://www.abc.com/a/{path}"
 * param |   type   | only one |     query     | e.g. url: "http://www.abc.com/a?b={query}"
 * param |   type   | only one |     formData  | e.g. "request body: a=123&b={formData}"
 * param |   type   | only one |     body      | request body can be any content
 * param |   type   | only one |     header    | request header info
 * param |   type   | only one |     cookie    | request cookie info, must be http.Cookie type
 * param |   name   |    no    |  (e.g. "id")  | specify request parameter's name
 * param | required |    no    |   required    | request parameter is required
 * param |   desc   |    no    |  (e.g. "id")  | request parameter description
 * param |   len    |    no    | (e.g. 3:6, 3) | length range of parameter
 * param |   range  |    no    |  (e.g. 0:10)  | numerical range of parameter
 * param |  nonzero |    no    |    nonzero    | parameter's value can not be zero
 * param |   maxmb  |    no    |   (e.g. 32)   | when request Content-Type is multipart/form-data, the max memory for body. (multi-parameter, whichever is greater)
 * regexp|          |    no    |(e.g. "^\\w+$")| parameter value can not be null
 *
 *  note:
 *    'regexp' or 'param' tag is only usable when 'param:"type(xxx)"' is exist.
 *     when tag!=`param:"-"`, anonymous field will be parsed.
 *     when param type is 'formData' and field type is 'multipart.FileHeader', the field receives file uploaded.
 *     when param type is 'cookie', field type must be 'http.Cookie'.
 */

const (
	TAG_PARAM        = "param"  //request param tag name
	TAG_REGEXP       = "regexp" //regexp validate tag name(optio)
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
		Name      string
		Fields    []*StructField
		MaxMemory int64
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
	fileTypeName   = reflect.TypeOf(multipart.FileHeader{}).Name()
	cookieTypeName = reflect.TypeOf(http.Cookie{}).Name()
)

// Parse the structure object, get *Struct
// if @paramNameFunc is not setted, paramNameFunc=toSnake
func ToStruct(object interface{}, paramNameFunc ...ParamNameFunc) (*Struct, error) {
	v := reflect.Indirect(reflect.ValueOf(object))
	if v.Kind() != reflect.Struct {
		return nil, errors.New("[apiware] parameter object is not a struct")
	}
	t := v.Type()
	m, ok := defaultSchema.Get(t.String())
	if !ok {
		m, err := toStruct(t, v, paramNameFunc)
		if err != nil {
			return nil, err
		}
		defaultSchema.Set(*m)
		return m, nil
	}

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

func (schema *Schema) Get(typeName string) (Struct, bool) {
	schema.RLock()
	defer schema.RUnlock()
	m, ok := schema.lib[typeName]
	return m, ok
}

func (schema *Schema) Set(m Struct) {
	schema.Lock()
	schema.lib[m.Name] = m
	defer schema.Unlock()
}

// Parse the structure object, get *Struct
// if @paramNameFunc is not setted, paramNameFunc=toSnake
func toStruct(t reflect.Type, v reflect.Value, paramNameFunc []ParamNameFunc) (*Struct, error) {
	m := &Struct{
		Name:   t.String(),
		Fields: []*StructField{},
	}
	var err error
	// defer func() {
	// 	if p := recover(); p != nil {
	// 		err = fmt.Errorf("%v", p)
	// 	}
	// }()
	if len(paramNameFunc) > 0 {
		err = addFields(m, t, v, paramNameFunc[0])
	} else {
		err = addFields(m, t, v, toSnake)
	}
	return m, err
}

func addFields(m *Struct, t reflect.Type, v reflect.Value, paramNameFunc ParamNameFunc) error {
	var err error
	var maxMemoryMB int64
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		tag, ok := field.Tag.Lookup(TAG_PARAM)
		if tag == TAG_IGNORE_PARAM {
			continue
		}
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			if err = addFields(m, field.Type, v.Field(i), paramNameFunc); err != nil {
				return err
			}
			continue
		}
		if !ok {
			continue
		}
		parsedTags := parseTags(tag)
		// fmt.Printf("%#v\n", parsedTags)
		paramType := parsedTags["type"]
		if !ParamTypes[paramType] {
			return errors.New("[apiware] it must be setted correctly, refer to the following: path/query/formData/body/header/cookie")
		}

		if field.Type.Name() == fileTypeName && paramType != "formData" {
			return errors.New("invalid file parameter, it must be 'formData'")
		}

		if (paramType == "cookie" && field.Type.Name() != cookieTypeName) ||
			(paramType != "cookie" && field.Type.Name() == cookieTypeName) {
			// fmt.Printf("cookie: %s %s %s\n%#v\n\n", paramType, field.Type.Name(), cookieTypeName, parsedTags)
			return errors.New("[apiware] invalid cookie parameter, it must be 'cookie' and value type must be 'http.Cookie'")
		}

		if a, ok := parsedTags["maxmb"]; ok {
			i, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				return errors.New("[apiware] invalid maxmb parameter, it must be positive integer")
			}
			if i > maxMemoryMB {
				maxMemoryMB = i
			}
		}

		if paramType == "path" {
			parsedTags["required"] = "required"
		}

		if r, ok := field.Tag.Lookup(TAG_REGEXP); ok {
			parsedTags["regexp"] = r
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

		fd.isFile = fd.Value.Type().Name() == fileTypeName
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

func (model *Struct) Validate() error {
	for _, field := range model.Fields {
		err := field.Validate()
		if err != nil {
			return err
		}
	}
	return nil
}

// Validate tests if the field conforms to it's validation constraints specified
// int the TAG_REGEXP struct tag
func (field *StructField) Validate() (err error) {
	defer func() {
		if p := recover(); p != nil {
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
		i, ok := field.Int()
		if ok {
			if err = validateRange(i, tuple, field.Name); err != nil {
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
	if reg, ok := field.Tags["regexp"]; ok {
		s, ok := field.String()
		if ok {
			if err = validateRegexp(s, reg, field.Name); err != nil {
				return err
			}
		}
	}

	return
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

func validateRange(i int64, tuple, field string) error {
	a, b := parseTuple(tuple)
	if len(a) > 0 {
		min, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			return err
		}
		if i < min {
			return NewValidationError(ValidationErrorValueTooSmall, field)
		}
	}
	if len(b) > 0 {
		max, err := strconv.ParseInt(b, 10, 64)
		if err != nil {
			return err
		}
		if i > max {
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

// Int returns the field int value and a bool flag indication if the conversion
// was successful
func (field *StructField) Int() (int64, bool) {
	switch field.Value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Value.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(field.Value.Uint()), true
	}
	return 0, false
}

// func (field *StructField) GoDeclaration() string {
// 	t := ""
// 	if x := field.RawTag; len(x) > 0 {
// 		t = fmt.Sprintf("\t`%s`", x)
// 	}
// 	return fmt.Sprintf(
// 		"%s\t%s%s",
// 		field.Name,
// 		reflect.TypeOf(field.Value).String(),
// 		t,
// 	)
// }
