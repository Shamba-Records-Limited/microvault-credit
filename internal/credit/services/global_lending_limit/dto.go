package globallendinglimit

import "time"

// CreateGlobalLendingLimitRequest represents the request to create a new global lending limit
type CreateGlobalLendingLimitRequest struct {
	ConfigKey   string  `json:"config_key" validate:"required"`
	ConfigValue string  `json:"config_value" validate:"required"`
	ValueType   string  `json:"value_type" validate:"required"`
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"`
	IsActive    bool    `json:"is_active"`
	CreatedBy   *string `json:"created_by,omitempty"`
}

// UpdateGlobalLendingLimitRequest represents the request to update global lending limit information
type UpdateGlobalLendingLimitRequest struct {
	ConfigKey   *string `json:"config_key,omitempty"`
	ConfigValue *string `json:"config_value,omitempty"`
	ValueType   *string `json:"value_type,omitempty"`
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
	UpdatedBy   *string `json:"updated_by,omitempty"`
}

// GlobalLendingLimitResponse represents the response containing global lending limit information
type GlobalLendingLimitResponse struct {
	ID          string    `json:"id"`
	ConfigKey   string    `json:"config_key"`
	ConfigValue string    `json:"config_value"`
	ValueType   string    `json:"value_type"`
	Description *string   `json:"description,omitempty"`
	Category    *string   `json:"category,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   *string   `json:"updated_by,omitempty"`
}
