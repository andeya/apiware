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
	"encoding/json"
	"errors"
	"fmt"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
)

type (
	Apiware struct {
		ParamNameFunc
		PathDecodeFunc
		BodyDecodeFunc
	}

	// Parse path params function, return pathParams of `[tag]:[value]` format
	PathDecodeFunc func(urlPath, pattern string) (pathParams map[string]string)

	// Create param name from struct field name
	ParamNameFunc func(fieldName string) (paramName string)

	// Decode params from request body
	BodyDecodeFunc func(fieldValue reflect.Value, body []byte) error
)

// Create a new apiware engine
func New(pathDecodeFunc PathDecodeFunc, bodyDecodeFunc BodyDecodeFunc, paramNameFunc ...ParamNameFunc) *Apiware {
	var _paramNameFunc ParamNameFunc
	if len(paramNameFunc) == 0 {
		_paramNameFunc = toSnake
	}
	return &Apiware{
		PathDecodeFunc: pathDecodeFunc,
		BodyDecodeFunc: bodyDecodeFunc,
		ParamNameFunc:  _paramNameFunc,
	}
}

// New middleware engine, and the default use json form at to decode the body
func NewWithJSONBody(pathDecodeFunc PathDecodeFunc, paramNameFunc ...ParamNameFunc) *Apiware {
	var bodyDecodeFunc BodyDecodeFunc = func(fieldValue reflect.Value, body []byte) error {
		var err error
		if fieldValue.Kind() == reflect.Ptr {
			err = json.Unmarshal(body, fieldValue.Interface())
		} else {
			err = json.Unmarshal(body, fieldValue.Addr().Interface())
		}
		return err
	}

	return New(pathDecodeFunc, bodyDecodeFunc, paramNameFunc...)
}

// Check whether structs meet the requirements of apiware, and register them.
// note: requires a structure pointer.
func (a *Apiware) RegStruct(structReceiverPtr ...interface{}) error {
	var errStr string
	for _, obj := range structReceiverPtr {
		_, err := ToStruct(obj, a.ParamNameFunc)
		if err != nil {
			errStr += err.Error() + "\n"
		}
	}
	if len(errStr) > 0 {
		return errors.New(errStr)
	}
	return nil
}

// Bind the net/http request params to the structure and validate.
// If the struct has not been registered, it will be registered at the same time.
// note: structReceiverPtr must be structure pointer.
func (a *Apiware) BindParam(structReceiverPtr interface{}, req *http.Request, pattern string) (err error) {
	obj, err := ToStruct(structReceiverPtr, a.ParamNameFunc)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			err = NewError(obj.Name, "?", fmt.Sprint(p))
		}
	}()

	var query, formValues url.Values
	var params = a.PathDecodeFunc(req.URL.Path, pattern)
	for _, field := range obj.Fields {
		switch field.Type() {
		case "path":
			paramValue, ok := params[field.Name]
			if !ok {
				return NewError(obj.Name, field.Name, "missing path param")
			}
			// fmt.Printf("fieldName:%s\nvalue:%#v\n\n", field.Name, paramValue)
			err = convertAssign(field.Value, []string{paramValue})
			if err != nil {
				return NewError(obj.Name, field.Name, err.Error())
			}

		case "query":
			if query == nil {
				query = req.URL.Query()
			}
			paramValues, ok := query[field.Name]
			if ok {
				err = convertAssign(field.Value, paramValues)
				if err != nil {
					return NewError(obj.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing query param")
			}

		case "formData":
			// Can not exist with `body` param at the same time
			if formValues == nil {
				err = req.ParseMultipartForm(obj.MaxMemory)
				if err != nil {
					return NewError(obj.Name, field.Name, err.Error())
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
						return NewError(obj.Name, field.Name, "missing formData param")
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
					return NewError(obj.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing formData param")
			}

		case "body":
			// Theoretically there should be at most one `body` param, and can not exist with `formData` at the same time
			body, err := ioutil.ReadAll(req.Body)
			req.Body.Close()
			if err == nil {
				err = a.BodyDecodeFunc(field.Value, body)
				if err != nil {
					return NewError(obj.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing body param")
			}

		case "header":
			paramValues, ok := req.Header[field.Name]
			if ok {
				err = convertAssign(field.Value, paramValues)
				if err != nil {
					return NewError(obj.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing header param")
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
					return NewError(obj.Name, field.Name, "invalid cookie param type, it must be `http.Cookie`, `string` or `[]byte`")
				}
			} else if field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing cookie param")
			}
		}
	}
	return obj.Validate()
}

// Bind the fasthttp request params to the structure and validate.
// If the struct has not been registered, it will be registered at the same time.
// note: structReceiverPtr must be structure pointer.
func (a *Apiware) FasthttpBindParam(structReceiverPtr interface{}, reqCtx *fasthttp.RequestCtx, pattern string) (err error) {
	obj, err := ToStruct(structReceiverPtr, a.ParamNameFunc)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			err = NewError(obj.Name, "?", fmt.Sprint(p))
		}
	}()

	var formValues = fasthttpFormValues(reqCtx)
	var params = a.PathDecodeFunc(string(reqCtx.Path()), pattern)
	for _, field := range obj.Fields {
		switch field.Type() {
		case "path":
			paramValue, ok := params[field.Name]
			if !ok {
				return NewError(obj.Name, field.Name, "missing path param")
			}
			// fmt.Printf("fieldName:%s\nvalue:%#v\n\n", field.Name, paramValue)
			err = convertAssign(field.Value, []string{paramValue})
			if err != nil {
				return NewError(obj.Name, field.Name, err.Error())
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
					return NewError(obj.Name, field.Name, err.Error())
				}
			} else if len(paramValuesBytes) == 0 && field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing query param")
			}

		case "formData":
			// Can not exist with `body` param at the same time
			if field.IsFile() {
				fh, err := reqCtx.FormFile(field.Name)
				if err != nil {
					if field.IsRequired() {
						return NewError(obj.Name, field.Name, "missing formData param")
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
					return NewError(obj.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing formData param")
			}

		case "body":
			// Theoretically there should be at most one `body` param, and can not exist with `formData` at the same time
			body := reqCtx.PostBody()
			if body != nil {
				err = a.BodyDecodeFunc(field.Value, body)
				if err != nil {
					return NewError(obj.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing body param")
			}

		case "header":
			paramValueBytes := reqCtx.Request.Header.Peek(field.Name)
			if paramValueBytes != nil {
				err = convertAssign(field.Value, []string{string(paramValueBytes)})
				if err != nil {
					return NewError(obj.Name, field.Name, err.Error())
				}
			} else if field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing header param")
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
						return NewError(obj.Name, field.Name, err.Error())
					}
					field.Value.Set(reflect.ValueOf(*c))

				case stringTypeString:
					field.Value.Set(reflect.ValueOf(string(bcookie)))

				case bytesTypeString, bytes2TypeString:
					field.Value.Set(reflect.ValueOf(bcookie))

				default:
					return NewError(obj.Name, field.Name, "invalid cookie param type, it must be `fasthttp.Cookie`, `string` or `[]byte`")
				}

			} else if field.IsRequired() {
				return NewError(obj.Name, field.Name, "missing cookie param")
			}
		}
	}

	return obj.Validate()
}

// fasthttpFormValues returns all post data values with their keys
// multipart, formValues    data, post arguments
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
