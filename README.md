# Apiware    [![GoDoc](https://godoc.org/github.com/tsuna/gohbase?status.png)](https://godoc.org/github.com/henrylee2cn/apiware)

Apiware binds the specified parameters of the Golang `net/http` and `fasthttp` requests to the structure and verifies the validity of the parameter values.

It is suggested that you can use the struct as the Handler of the web framework, and use the middleware to quickly bind the request parameters, saving a lot of parameter type conversion and validity verification. At the same time through the struct tag, create swagger json configuration file, easy to create api document services.

Apiware将Go语言`net/http`及`fasthttp`请求的指定参数绑定到结构体，并验证参数值的合法性。
建议您可以使用结构体作为web框架的Handler，并用该中间件快速绑定请求参数，节省了大量参数类型转换与有效性验证的工作。同时还可以通过该结构体标签，创建swagger的json配置文件，轻松创建api文档服务。

# Demo 示例

```
package main

import (
    "encoding/json"
    "github.com/henrylee2cn/apiware"
    // "mime/multipart"
    "net/http"
    "strings"
)

type TestApiware struct {
    Id           int         `param:"type(path),required,desc(ID),range(1:2)"`
    Num          float32     `param:"type(query),name(n),range(1.1:1.19)"`
    Title        string      `param:"type(query),nonzero"`
    Paragraph    []string    `param:"type(query),name(p),len(1:10)" regexp:"(^[\\w]*$)"`
    Cookie       http.Cookie `param:"type(cookie),name(apiwareid)"`
    CookieString string      `param:"type(cookie),name(apiwareid)"`
    // Picture   multipart.FileHeader `param:"type(formData),name(pic),maxmb(30)"`
}

var myApiware = apiware.NewWithJSONBody(pathDecodeFunc)

var pattern = "/test/:id"

func pathDecodeFunc(urlPath, pattern string) (pathParams map[string]string) {
    idx := map[int]string{}
    for k, v := range strings.Split(pattern, "/") {
        if !strings.HasPrefix(v, ":") {
            continue
        }
        idx[k] = v[1:]
    }
    pathParams = make(map[string]string, len(idx))
    for k, v := range strings.Split(urlPath, "/") {
        name, ok := idx[k]
        if !ok {
            continue
        }
        pathParams[name] = v
    }
    return
}

func testHandler(resp http.ResponseWriter, req *http.Request) {
    // set cookies
    http.SetCookie(resp, &http.Cookie{
        Name:  "apiwareid",
        Value: "http_henrylee2cn",
    })

    // bind params
    params := new(TestApiware)
    err := myApiware.BindParam(params, req, pattern)
    b, _ := json.MarshalIndent(params, "", " ")
    if err != nil {
        resp.WriteHeader(http.StatusBadRequest)
        resp.Write(append([]byte(err.Error()+"\n"), b...))
    } else {
        resp.WriteHeader(http.StatusOK)
        resp.Write(b)
    }
}

func main() {
    // Check whether `testHandler` meet the requirements of apiware, and register it
    err := myApiware.RegStruct(new(TestApiware))
    if err != nil {
        panic(err)
    }

    // server
    http.HandleFunc("/test/0", testHandler)
    http.HandleFunc("/test/1", testHandler)
    http.HandleFunc("/test/1.1", testHandler)
    http.HandleFunc("/test/2", testHandler)
    http.HandleFunc("/test/3", testHandler)
    http.ListenAndServe(":8080", nil)
}
```

# Struct&Tag 结构体及其标签说明

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

**NOTES**:
* 1. `regexp` or `param` tag is only usable when `param:"type(xxx)"` is exist;
* 2. if tag!=`param:"-"`, anonymous field will be parsed;
* 3. when param type is `formData` and field type is `multipart.FileHeader`, the field receives file uploaded;
* 4. if param type is `cookie`, field type must be `http.Cookie`;
* 5. `formData` and `body` params can not exist at the same time;
* 6. there should not be more than one `body` param;
* 7. the binding object must be a struct pointer;
* 8. the binding struct field can not be a pointer.


# Field Types 结构体字段类型范围

base type| slice type  | special type
---------|-------------|-------------------------------------------------------
`string`  |  `[]string`  | `multipart.FileHeader` (only for `formData` param)
`byte`    |  `[]byte`    | `http.Cookie` (only for `net/http`'s `cookie` param)
`uint8`   |  `[]uint8`   | `fasthttp.Cookie` (only for `fasthttp`'s `cookie` param)
`bool`    |  `[]bool`    | `[][]byte`
`int`     |  `[]int`     | `[][]uint8`
`int8`    |  `[]int8`    | `struct` (struct type only for `body` param or as an anonymous field to extend params)
`int16`   |  `[]int16`   |
`int32`   |  `[]int32`   |
`int64`   |  `[]int64`   |
`uint8`   |  `[]uint8`   |
`uint16`  |  `[]uint16`  |
`uint32`  |  `[]uint32`  |
`uint64`  |  `[]uint64`  |
`float32` |  `[]float32` |
`float64` |  `[]float64` |
