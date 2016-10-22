package main

import (
	"github.com/henrylee2cn/apiware"
	"strings"
)

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

func main() {
	// Check whether these structs meet the requirements of apiware, and register them
	err := myApiware.RegStruct(
		new(httpTestApiware),
		new(fasthttpTestApiware),
	)
	if err != nil {
		panic(err)
	}

	// http server
	go httpServer(":8080")

	// fasthttp server
	fasthttpServer(":8081")
}
