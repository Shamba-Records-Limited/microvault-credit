package creditconfigauditlog

import "time"

// CreateCreditConfigAuditLogRequest represents the request to create a new audit log
type CreateCreditConfigAuditLogRequest struct {
	ConfigTable  string         `json:"config_table" validate:"required"`
	ConfigID     string         `json:"config_id" validate:"required"`
	Action       string         `json:"action" validate:"required"`
	OldValues    map[string]any `json:"old_values,omitempty"`
	NewValues    map[string]any `json:"new_values,omitempty"`
	ChangedBy    *string        `json:"changed_by,omitempty"`
	ChangeReason *string        `json:"change_reason,omitempty"`
}

// CreditConfigAuditLogResponse represents the response containing audit log information
type CreditConfigAuditLogResponse struct {
	ID           string         `json:"id"`
	ConfigTable  string         `json:"config_table"`
	ConfigID     string         `json:"config_id"`
	Action       string         `json:"action"`
	OldValues    map[string]any `json:"old_values,omitempty"`
	NewValues    map[string]any `json:"new_values,omitempty"`
	ChangedBy    *string        `json:"changed_by,omitempty"`
	ChangeReason *string        `json:"change_reason,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// AuditLogFilters represents filters for listing audit logs
type AuditLogFilters struct {
	ConfigTable string    `json:"config_table,omitempty"`
	ConfigID    string    `json:"config_id,omitempty"`
	Action      string    `json:"action,omitempty"`
	StartDate   time.Time `json:"start_date,omitempty"`
	EndDate     time.Time `json:"end_date,omitempty"`
}
