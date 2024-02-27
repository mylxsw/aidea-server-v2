package repo

import "database/sql"

type RobotRepo struct {
	db *sql.DB
}

func NewRobotRepo(db *sql.DB) *RobotRepo {
	return &RobotRepo{db: db}
}

// RobotType robot type
type RobotType int

const (
	// RobotTypeModelDriven robot type: model driven
	RobotTypeModelDriven RobotType = 1
	// RobotTypeCustomServer robot type: custom server
	RobotTypeCustomServer RobotType = 2
)

// RobotPrivilege robot privilege
type RobotPrivilege int

const (
	// RobotPrivilegePrivate robot privilege: private
	RobotPrivilegePrivate RobotPrivilege = 1
	// RobotPrivilegePublic robot privilege: public
	RobotPrivilegePublic RobotPrivilege = 2
)

// RobotMeta bot meta information
type RobotMeta struct {
	// ShowPrompt controls whether Prompt is shown to the user
	ShowPrompt bool `json:"show_prompt,omitempty"`
	// KnowledgeBase knowledge base
	KnowledgeBases []string `json:"knowledge_bases,omitempty"`
	// OriginReference whether to display knowledge base sources
	OriginReference bool `json:"origin_reference,omitempty"`
}

type Robot struct {
	// RobotID robot id
	RobotID string `json:"robot_id"`
	// Name robot name
	Name string `json:"name"`
	// Type robot type
	Type RobotType `json:"type"`
	// Description robot description
	Description string `json:"description,omitempty"`

	// Privilege robot privilege
	Privilege RobotPrivilege `json:"privilege,omitempty"`

	// Model robot model (if type is model driven)
	Model string `json:"model,omitempty"`
	// Prompt robot prompt (if type is model driven)
	Prompt string `json:"prompt,omitempty"`
	// WelcomeMessage robot welcome message (if type is model driven)
	WelcomeMessage string `json:"welcome_message,omitempty"`

	// ServerURL robot server url (if type is custom server)
	ServerURL string `json:"server_url,omitempty"`
	// ServerToken robot server jwt (if type is custom server)
	ServerToken string `json:"server_token,omitempty"`

	// RobotMeta some bot meta information
	RobotMeta RobotMeta `json:"robot_meta,omitempty"`
}
