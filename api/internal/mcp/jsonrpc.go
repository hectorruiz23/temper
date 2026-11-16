package mcp

import (
	"bytes"
	"encoding/json"
	"io"
)

const (
	protocolVersion = "2025-11-25"
	jsonrpcVersion  = "2.0"
)

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type paramsError struct {
	message string
}

func (e paramsError) Error() string {
	return e.message
}

func errorResponse(id json.RawMessage, code int, message string) response {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return response{JSONRPC: jsonrpcVersion, ID: id, Error: &rpcError{Code: code, Message: message}}
}

func resultResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: jsonrpcVersion, ID: id, Result: result}
}

func validID(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return false
	}

	switch value.(type) {
	case nil, string, json.Number:
		return true
	default:
		return false
	}
}
