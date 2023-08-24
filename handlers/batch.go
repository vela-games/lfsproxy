package handlers

import "time"

type BatchRequest struct {
	Operation string                 `json:"operation"`
	Objects   []*BatchObjectResponse `json:"objects"`
	Transfers []string               `json:"transfers"`
	Ref       map[string]string      `json:"ref"`
	HashAlgo  string                 `json:"hash_algo"`
}

// BatchResponse represents a batch response payload.
//
// https://github.com/git-lfs/git-lfs/blob/master/docs/api/batch.md#successful-responses
type BatchResponse struct {
	Transfer string                 `json:"transfer,omitempty"`
	Objects  []*BatchObjectResponse `json:"objects"`
}

// BatchObjectResponse is the object item of a BatchResponse
type BatchObjectResponse struct {
	OID           string                                `json:"oid"`
	Size          int64                                 `json:"size"`
	Authenticated bool                                  `json:"authenticated,omitempty"`
	Actions       map[string]*BatchObjectActionResponse `json:"actions,omitempty"`
}

// BatchObjectActionResponse is the action item of a BatchObjectResponse
type BatchObjectActionResponse struct {
	Href      string            `json:"href"`
	HeadHref  string            `json:"head_href,omitempty"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresIn int               `json:"expires_in,omitempty"`
	ExpiresAt time.Time         `json:"expires_at,omitempty"`
}
