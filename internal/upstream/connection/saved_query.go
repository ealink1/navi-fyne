package connection

type SavedQuery struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	SQL                   string            `json:"sql"`
	ConnectionID          string            `json:"connectionId"`
	DBName                string            `json:"dbName"`
	CreatedAt             int64             `json:"createdAt"`
	ConnectionFingerprint string            `json:"connectionFingerprint,omitempty"`
	FingerprintVersion    string            `json:"fingerprintVersion,omitempty"`
	BindingStatus         string            `json:"bindingStatus,omitempty"`
	OriginalConnectionID  string            `json:"originalConnectionId,omitempty"`
	Parameters            []SavedQueryParam `json:"parameters,omitempty"`
}

// SavedQueryParam 是保存查询随附的参数声明：名称、类型、展示标签与可选默认值。
// 只有声明（含显式默认值）随保存查询持久化与同步；运行时输入值绝不持久化。
type SavedQueryParam struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Label   string `json:"label,omitempty"`
	Default any    `json:"default,omitempty"`
}

// SavedQueryGroup describes a user-managed saved SQL folder. QueryIDs contains
// only the group's direct queries; ChildOrder can mix query:<id> and group:<id>
// tokens to retain the visible order of direct queries and child groups.
type SavedQueryGroup struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ParentGroupID string   `json:"parentGroupId"`
	QueryIDs      []string `json:"queryIds"`
	ChildOrder    []string `json:"childOrder"`
}

type SavedQueryImportPayload struct {
	Queries           []SavedQuery           `json:"queries"`
	Groups            []SavedQueryGroup      `json:"groups,omitempty"`
	LegacyConnections []SavedConnectionInput `json:"legacyConnections,omitempty"`
}
