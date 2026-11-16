package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	healthapp "github.com/hectorruiz23/temper/api/internal/health/app"
	plansapp "github.com/hectorruiz23/temper/api/internal/plans/app"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
	trainingapp "github.com/hectorruiz23/temper/api/internal/training/app"
)

const (
	serverName    = "temper"
	serverVersion = "0.1.0"
	maxBodyBytes  = 64 << 10
)

type Handler struct {
	plans     *plansapp.PlanService
	catalogue *plansapp.CatalogueService
	health    *healthapp.Service
	training  *trainingapp.Service
}

func New(plans *plansapp.PlanService, catalogue *plansapp.CatalogueService, health *healthapp.Service, training *trainingapp.Service) *Handler {
	return &Handler{plans: plans, catalogue: catalogue, health: health, training: training}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !sameOrigin(r) {
		httpserver.WriteError(w, http.StatusForbidden, "forbidden_origin", "origin is not allowed")
		return
	}

	if version := r.Header.Get("MCP-Protocol-Version"); version != "" && version != protocolVersion {
		writeRPC(w, http.StatusOK, errorResponse(nil, codeInvalidRequest, "unsupported MCP-Protocol-Version"))
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil || len(body) > maxBodyBytes {
		writeRPC(w, http.StatusOK, errorResponse(nil, codeParseError, "parse error"))
		return
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		writeRPC(w, http.StatusOK, errorResponse(nil, codeParseError, "parse error"))
		return
	}
	if trimmed[0] != '{' {
		writeRPC(w, http.StatusOK, errorResponse(nil, codeInvalidRequest, "invalid request"))
		return
	}

	var decoded request
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		writeRPC(w, http.StatusOK, errorResponse(nil, codeInvalidRequest, "invalid request"))
		return
	}

	if decoded.JSONRPC != jsonrpcVersion || decoded.Method == "" {
		writeRPC(w, http.StatusOK, errorResponse(nil, codeInvalidRequest, "invalid request"))
		return
	}

	if !validID(decoded.ID) {
		writeRPC(w, http.StatusOK, errorResponse(nil, codeInvalidRequest, "invalid request id"))
		return
	}

	if len(decoded.ID) == 0 {
		w.Header().Set("MCP-Protocol-Version", protocolVersion)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch decoded.Method {
	case "initialize":
		writeRPC(w, http.StatusOK, resultResponse(decoded.ID, h.initializeResult()))
	case "ping":
		writeRPC(w, http.StatusOK, resultResponse(decoded.ID, struct{}{}))
	case "tools/list":
		writeRPC(w, http.StatusOK, resultResponse(decoded.ID, map[string]any{"tools": h.definitions()}))
	case "tools/call":
		h.handleToolCall(w, r, decoded)
	default:
		writeRPC(w, http.StatusOK, errorResponse(decoded.ID, codeMethodNotFound, "unknown method"))
	}
}

func (h *Handler) handleToolCall(w http.ResponseWriter, r *http.Request, decoded request) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Meta      json.RawMessage `json:"_meta"`
	}
	if err := decodeStrict(decoded.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
		writeRPC(w, http.StatusOK, errorResponse(decoded.ID, codeInvalidParams, "invalid params"))
		return
	}

	result, err := h.callTool(r.Context(), params.Name, params.Arguments)
	if err != nil {
		var invalid paramsError
		if errors.As(err, &invalid) {
			writeRPC(w, http.StatusOK, errorResponse(decoded.ID, codeInvalidParams, invalid.message))
			return
		}
		writeRPC(w, http.StatusOK, errorResponse(decoded.ID, codeInternalError, "internal error"))
		return
	}

	writeRPC(w, http.StatusOK, resultResponse(decoded.ID, result))
}

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
}

type capabilities struct {
	Tools struct{} `json:"tools"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (h *Handler) initializeResult() initializeResult {
	return initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    capabilities{},
		ServerInfo:      serverInfo{Name: serverName, Version: serverVersion},
	}
}

func writeRPC(w http.ResponseWriter, status int, value response) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("MCP-Protocol-Version", protocolVersion)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	return strings.EqualFold(parsed.Host, r.Host)
}
