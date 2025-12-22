package req

import (
	"encoding/json"

	"github.com/calypr/data-client/client/common"
)

// New addition to your request package
type RequestBuilder struct {
	Req     *Request // the underlying retry client holder
	Method  string
	Url     string
	Body    []byte // store as []byte for easy reuse
	Headers map[string]string
	Token   string
	Timeout bool
}

func (r *Request) New(method, url string) *RequestBuilder {
	return &RequestBuilder{
		Req:     r,
		Method:  method,
		Url:     url,
		Headers: make(map[string]string),
	}
}

func (ar *RequestBuilder) WithToken(token string) *RequestBuilder {
	ar.Token = token
	return ar
}

func (ar *RequestBuilder) WithJSONBody(v any) *RequestBuilder {
	bodyBytes, _ := json.Marshal(v) // handle error higher up if needed
	ar.Body = bodyBytes
	ar.Headers[common.HeaderContentType] = common.MIMEApplicationJSON
	return ar

}

func (ar *RequestBuilder) WithBody(body []byte) *RequestBuilder {
	ar.Body = body
	return ar
}

func (ar *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	ar.Headers[key] = value
	return ar
}

func (ar *RequestBuilder) WithTimeout() *RequestBuilder {
	ar.Timeout = true
	return ar
}
