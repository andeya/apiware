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
    Id        int      `param:"type(path),required,desc(ID),range(1:1)"`
    Title     string   `param:"type(query),required,nonzero,len(3:15)"`
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
        resp.Write(append([]byte(err.Error()+":\n"), b...))
    } else {
        resp.WriteHeader(http.StatusOK)
        resp.Write(b)
    }
}

func main() {
    // server
    http.HandleFunc("/test/0", test1)
    http.HandleFunc("/test/1", test1)
    http.HandleFunc("/test/2", test1)
    http.ListenAndServe(":7310", nil)
}
```