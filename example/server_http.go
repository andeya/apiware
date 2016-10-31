package main

import (
	"encoding/json"
	// "mime/multipart"
	"net/http"
)

type httpTestApiware struct {
	Id           int         `param:"type(path),required,desc(ID),range(1:2)"`
	Num          float32     `param:"type(query),name(n),range(1.1:1.19)"`
	Title        string      `param:"type(query),nonzero"`
	Paragraph    []string    `param:"type(query),name(p),len(1:10)" regexp:"(^[\\w]*$)"`
	Cookie       http.Cookie `param:"type(cookie),name(apiwareid)"`
	CookieString string      `param:"type(cookie),name(apiwareid)"`
	// Picture   multipart.FileHeader `param:"type(formData),name(pic),maxmb(30)"`
}

func httpTestHandler(resp http.ResponseWriter, req *http.Request) {
	// set cookies
	http.SetCookie(resp, &http.Cookie{
		Name:  "apiwareid",
		Value: "http_henrylee2cn",
	})

	// bind params
	params := new(httpTestApiware)
	err := myApiware.Bind(params, req, pattern)
	b, _ := json.MarshalIndent(params, "", " ")

	if err != nil {
		resp.WriteHeader(http.StatusBadRequest)
		resp.Write(append([]byte(err.Error()+"\n"), b...))
	} else {
		resp.WriteHeader(http.StatusOK)
		resp.Write(b)
	}
}

func httpServer(addr string) {
	// server
	http.HandleFunc("/test/0", httpTestHandler)
	http.HandleFunc("/test/1", httpTestHandler)
	http.HandleFunc("/test/1.1", httpTestHandler)
	http.HandleFunc("/test/2", httpTestHandler)
	http.HandleFunc("/test/3", httpTestHandler)
	http.ListenAndServe(addr, nil)
}
