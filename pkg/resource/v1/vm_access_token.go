package v1

import "time"

type IssueVMAccessTokenRequest struct {
	TTLSeconds *uint64 `json:"ttlSeconds,omitempty"`
}

type VMAccessToken struct {
	Token       string    `json:"token,omitempty"`
	TokenType   string    `json:"tokenType,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt,omitempty"`
	SSHUsername string    `json:"sshUsername,omitempty"`
	VMName      string    `json:"vmName,omitempty"`
	VMUID       string    `json:"vmUID,omitempty"`
}
