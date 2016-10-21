// Package apiware provides a tools which can bind the http/fasthttp request parameters to the structure and validate
package apiware

import (
	// "github.com/valyala/fasthttp"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
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

type (
	Apiware struct {
		ParamNameFunc
		PathDecodeFunc
		BodyDecodeFunc
	}

	// Parse path parameters function, return format [tag]:[value]
	PathDecodeFunc func(urlPath, pattern string) (pathParams map[string]string)

	// Create parameter name from struct field name
	ParamNameFunc func(fieldName string) (paramName string)

	// Decode parameters from request body
	BodyDecodeFunc func(fieldValues map[string]reflect.Value, body []byte) error
)

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

func NewWithJSONBody(pathDecodeFunc PathDecodeFunc, paramNameFunc ...ParamNameFunc) *Apiware {
	var bodyDecodeFunc BodyDecodeFunc = func(fieldValues map[string]reflect.Value, body []byte) error {
		var src map[string]interface{}
		err := json.Unmarshal(body, &src)
		if err != nil {
			return err
		}
		for name, fieldValue := range fieldValues {
			value, ok := src[name]
			if !ok {
				continue
			}
			fieldValue = reflect.Indirect(fieldValue)
			if !fieldValue.CanSet() {
				return fmt.Errorf("[apiware] type %s can be setted", fieldValue.Type().Name())
			}
			t := fieldValue.Type()
			t2 := reflect.TypeOf(value)
			if t2.AssignableTo(t) {
				return fmt.Errorf("[apiware] type %s is not assignable to %s", t2.Name(), t.Name())
			}
			fieldValue.Set(reflect.ValueOf(value))
		}
		return nil
	}

	return New(pathDecodeFunc, bodyDecodeFunc, paramNameFunc...)
}

// Bind the net/http request parameters to the structure and validate
// if @paramNameFunc is not setted, paramNameFunc=toSnake
func (a *Apiware) BindParam(structReceiverPtr interface{}, req *http.Request, pattern string) error {
	obj, err := ToStruct(structReceiverPtr, a.ParamNameFunc)
	if err != nil {
		return err
	}

	var query, form url.Values
	var params = a.PathDecodeFunc(req.URL.Path, pattern)
	var bodyValues = map[string]reflect.Value{}
	var body []byte
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()
	for _, field := range obj.Fields {
		switch field.Type() {
		case "path":
			paramValue, ok := params[field.Name]
			if !ok {
				return errors.New("[apiware] missing path parameter: " + field.Name)
			}
			// fmt.Printf("fieldName:%s\nvalue:%#v\n\n", field.Name, paramValue)
			err = convertAssign(field.Value, []string{paramValue})
			if err != nil {
				return err
			}

		case "query":
			if query == nil {
				query = req.URL.Query()
			}
			paramValues, ok := query[field.Name]
			if !ok && field.IsRequired() {
				return errors.New("[apiware] missing query parameter: " + field.Name)
			}
			err = convertAssign(field.Value, paramValues)
			if err != nil {
				return err
			}

		case "formData":
			if form == nil {
				err = req.ParseMultipartForm(obj.MaxMemory)
				if err != nil {
					return err
				}
				form = req.PostForm
				if req.MultipartForm != nil {
					for k, v := range req.MultipartForm.Value {
						if _, ok := form[k]; ok {
							form[k] = append(form[k], v...)
						} else {
							form[k] = v
						}
					}
				}
			}

			if field.IsFile() && req.MultipartForm != nil && req.MultipartForm.File != nil {
				fhs := req.MultipartForm.File[field.Name]
				if len(fhs) == 0 {
					if field.IsRequired() {
						return errors.New("[apiware] missing formData parameter: " + field.Name)
					}
					continue
				}
				field.Value.Set(reflect.ValueOf(fhs[0]).Elem())
				continue
			}

			paramValues, ok := form[field.Name]
			if !ok && field.IsRequired() {
				return errors.New("[apiware] missing formData parameter: " + field.Name)
			}
			err = convertAssign(field.Value, paramValues)
			if err != nil {
				return err
			}

		case "body":
			if body == nil {
				body, err = ioutil.ReadAll(req.Body)
				req.Body.Close()
				if err != nil {
					return err
				}
				if len(body) == 0 && field.IsRequired() {
					return errors.New("[apiware] missing body parameter: " + field.Name)
				}
			}
			bodyValues[field.Name] = field.Value

		case "header":
			paramValues, ok := req.Header[field.Name]
			if !ok && field.IsRequired() {
				return errors.New("[apiware] missing header parameter: " + field.Name)
			}
			err = convertAssign(field.Value, paramValues)
			if err != nil {
				return err
			}

		case "cookie":
			c, _ := req.Cookie(field.Name)
			if c == nil && field.IsRequired() {
				return errors.New("[apiware] missing cookie parameter: " + field.Name)
			}

			field.Value.Set(reflect.ValueOf(c).Elem())
		}
	}

	if len(bodyValues) > 0 {
		err = a.BodyDecodeFunc(bodyValues, body)
		if err != nil {
			return err
		}
	}

	err = obj.Validate()

	return err
}
