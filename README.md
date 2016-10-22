# Apiware    [![GoDoc](https://godoc.org/github.com/tsuna/gohbase?status.png)](https://godoc.org/github.com/henrylee2cn/apiware)

Apiware为Go语言的net/http及fasthttp提供请求参数的绑定、验证服务。

# 示例

```
package main

import (
    "encoding/json"
    "github.com/henrylee2cn/apiware"
    // "mime/multipart"
    "net/http"
    "strings"
)

type IndexTest1 struct {
    Id        int      `param:"type(path),required,desc(ID),range(1:2)"`
    Num       float64  `param:"type(query),name(n),range(1.1:1.19)"`
    Title     string   `param:"type(query),nonzero"`
    Paragraph []string `param:"type(query),name(p),len(1:10)" regexp:"(^[\\w]*$)"`
    // Picture   *multipart.FileHeader `param:"type(formData),name(pic),maxmb(30)"`
}

var Apiware = apiware.NewWithJSONBody(pathDecodeFunc)

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

func test1(resp http.ResponseWriter, req *http.Request) {
    params := new(IndexTest1)
    err := Apiware.BindParam(params, req, pattern)
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
    // server
    http.HandleFunc("/test/0", test1)
    http.HandleFunc("/test/1", test1)
    http.HandleFunc("/test/1.1", test1)
    http.HandleFunc("/test/2", test1)
    http.HandleFunc("/test/3", test1)
    http.ListenAndServe(":7310", nil)
}
```

# 结构体标签说明

tag   |   key    | required |       value       |   desc
------|----------|----------|-------------------|----------------------------------
param |   type   | only one |       path        | when 'required' is unsetted, auto set it. e.g. url: "http://www.abc.com/a/{path}"
param |   type   | only one |       query       | e.g. url: "http://www.abc.com/a?b={query}"
param |   type   | only one |       formData    | e.g. "request body: a=123&b={formData}"
param |   type   | only one |       body        | request body can be any content
param |   type   | only one |       header      | request header info
param |   type   | only one |       cookie      | request cookie info, must be http.Cookie type
param |   name   |    no    |    (e.g. "id")    | specify request parameter's name
param | required |    no    |     required      | request parameter is required
param |   desc   |    no    |    (e.g. "id")    | request parameter description
param |   len    |    no    |   (e.g. 3:6, 3)   | length range of parameter
param |   range  |    no    |    (e.g. 0:10)    | numerical range of parameter
param |  nonzero |    no    |      nonzero      | parameter's value can not be zero
param |   maxmb  |    no    |     (e.g. 32)     | when request Content-Type is multipart/form-data, the max memory for body. (multi-parameter, whichever is greater)
regexp|          |    no    |  (e.g. "^\\w+$")  | parameter value can not be null


note:
    'regexp' or 'param' tag is only usable when `param:"type(xxx)"` is exist.
    when tag!=`param:"-"`, anonymous field will be parsed.
    when param type is 'formData' and field type is 'multipart.FileHeader', the field receives file uploaded.
    when param type is 'cookie', field type must be 'http.Cookie'.
