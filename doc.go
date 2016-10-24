/*
Package apiware provides a tools which can bind the http/fasthttp request params to the structure and validate.

Copyright 2016 HenryLee. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Param tag value description:
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

    NOTES:
        1. the binding object must be a struct pointer
        2. the binding struct field can not be a pointer
        3. `regexp` or `param` tag is only usable when `param:"type(xxx)"` is exist
        4. if the `param` tag is not exist, anonymous field will be parsed
        5. when param type is `formData` and field type is `multipart.FileHeader`, the field receives file uploaded
        6. if param type is `cookie`, field type must be `http.Cookie`
        7. `formData` and `body` params can not exist at the same time
        8. there should not be more than one `body` param

List of supported param value types:
    base    |   slice    | special
    --------|------------|-------------------------------------------------------
    string  |  []string  | [][]byte
    byte    |  []byte    | [][]uint8
    uint8   |  []uint8   | multipart.FileHeader (only for `formData` param)
    bool    |  []bool    | http.Cookie (only for `net/http`'s `cookie` param)
    int     |  []int     | fasthttp.Cookie (only for `fasthttp`'s `cookie` param)
    int8    |  []int8    | struct (struct type only for `body` param or as an anonymous field to extend params)
    int16   |  []int16   |
    int32   |  []int32   |
    int64   |  []int64   |
    uint8   |  []uint8   |
    uint16  |  []uint16  |
    uint32  |  []uint32  |
    uint64  |  []uint64  |
    float32 |  []float32 |
    float64 |  []float64 |
*/
package apiware
