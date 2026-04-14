package lsp

import "net/url"

// --- URI helpers ---

// FileURI converts an absolute file path to a file:// URI.
func FileURI(path string) string {
	return "file://" + path
}

// URIToFile converts a file:// URI back to an absolute path.
func URIToFile(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	return u.Path
}

// --- Initialize ---

type InitializeParams struct {
	ProcessID        int                `json:"processId"`
	RootURI          string             `json:"rootUri"`
	Capabilities     ClientCapabilities `json:"capabilities"`
	WorkspaceFolders []WorkspaceFolder  `json:"workspaceFolders,omitempty"`
}

type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type ClientCapabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument,omitempty"`
	Window       WindowClientCapabilities       `json:"window,omitempty"`
}

type TextDocumentClientCapabilities struct {
	CallHierarchy CallHierarchyClientCapabilities `json:"callHierarchy,omitempty"`
}

type CallHierarchyClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

type WindowClientCapabilities struct {
	WorkDoneProgress bool `json:"workDoneProgress,omitempty"`
}

// --- Text Document ---

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// --- Position / Range ---

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// --- Call Hierarchy ---

type CallHierarchyItem struct {
	Name           string `json:"name"`
	Kind           int    `json:"kind"`
	Tags           []int  `json:"tags,omitempty"`
	Detail         string `json:"detail,omitempty"`
	URI            string `json:"uri"`
	Range          Range  `json:"range"`
	SelectionRange Range  `json:"selectionRange"`
	// Data is opaque data gopls attaches for resolving incoming/outgoing calls.
	Data any `json:"data,omitempty"`
}

type CallHierarchyIncomingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

type CallHierarchyIncomingCall struct {
	From       CallHierarchyItem `json:"from"`
	FromRanges []Range           `json:"fromRanges"`
}

type CallHierarchyOutgoingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

type CallHierarchyOutgoingCall struct {
	To         CallHierarchyItem `json:"to"`
	FromRanges []Range           `json:"fromRanges"`
}

// --- Implementation ---

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}
