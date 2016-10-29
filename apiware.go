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
	"net/http"
	"reflect"

	"github.com/valyala/fasthttp"
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
	return obj.BindParam(req, pattern, a.PathDecodeFunc, a.BodyDecodeFunc)
}

// Bind the fasthttp request params to the structure and validate.
// If the struct has not been registered, it will be registered at the same time.
// note: structReceiverPtr must be structure pointer.
func (a *Apiware) FasthttpBindParam(structReceiverPtr interface{}, reqCtx *fasthttp.RequestCtx, pattern string) (err error) {
	obj, err := ToStruct(structReceiverPtr, a.ParamNameFunc)
	if err != nil {
		return err
	}
	return obj.FasthttpBindParam(reqCtx, pattern, a.PathDecodeFunc, a.BodyDecodeFunc)
}
