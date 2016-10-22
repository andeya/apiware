package main

import (
	"encoding/json"
	// "mime/multipart"
	"github.com/valyala/fasthttp"
	"net/http"
)

type fasthttpTestApiware struct {
	Id        int      `param:"type(path),required,desc(ID),range(1:2)"`
	Num       float32  `param:"type(query),name(n),range(1.1:1.19)"`
	Title     string   `param:"type(query),nonzero"`
	Paragraph []string `param:"type(query),name(p),len(1:10)" regexp:"(^[\\w]*$)"`
	Cookie    string   `param:"type(cookie),name(apiwareid)"`
	// Picture   multipart.FileHeader `param:"type(formData),name(pic),maxmb(30)"`
}

func fasthttpTestHandler(ctx *fasthttp.RequestCtx) {
	// set cookies
	var c fasthttp.Cookie
	c.SetKey("apiwareid")
	c.SetValue("fasthttp_henrylee2cn")
	ctx.Response.Header.SetCookie(&c)

	// bind params
	params := new(fasthttpTestApiware)
	err := myApiware.FasthttpBindParam(params, ctx, pattern)
	b, _ := json.MarshalIndent(params, "", " ")

	if err != nil {
		ctx.SetStatusCode(http.StatusBadRequest)
		ctx.Write(append([]byte(err.Error()+"\n"), b...))
	} else {
		ctx.SetStatusCode(http.StatusOK)
		ctx.Write(b)
	}
}

func fasthttpServer(addr string) {
	// server
	fasthttp.ListenAndServe(addr, fasthttpTestHandler)
}
