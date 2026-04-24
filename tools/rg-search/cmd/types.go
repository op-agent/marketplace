package main

type Query struct {
	Root       string
	Pattern    string
	Regex      bool
	MatchCase  bool
	WholeWord  bool
	Includes   []string
	Excludes   []string
	MaxFiles   int
	MaxMatches int
}

type Match struct {
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndColumn int    `json:"endColumn"`
	Text      string `json:"text"`
}

type FileResult struct {
	Path    string  `json:"path"`
	Matches []Match `json:"matches"`
	Count   int     `json:"count"`
}

type Result struct {
	Files      []FileResult `json:"files"`
	TotalCount int          `json:"totalCount"`
	Truncated  bool         `json:"truncated"`
}
