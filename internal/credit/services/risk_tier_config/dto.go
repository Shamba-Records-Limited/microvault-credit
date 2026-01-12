package risktierconfig

import "time"

// CreateRiskTierConfigRequest represents the request to create a new risk tier config
type CreateRiskTierConfigRequest struct {
	TierName    string  `json:"tier_name" validate:"required"`
	MinScore    int     `json:"min_score" validate:"required,gte=0"`
	MaxScore    int     `json:"max_score" validate:"required,gt=0"`
	TierOrder   int     `json:"tier_order" validate:"required,gte=0"`
	Description *string `json:"description,omitempty"`
	ColorCode   *string `json:"color_code,omitempty"`
	IsActive    bool    `json:"is_active"`
	CreatedBy   *string `json:"created_by,omitempty"`
}

// UpdateRiskTierConfigRequest represents the request to update risk tier config information
type UpdateRiskTierConfigRequest struct {
	TierName    *string `json:"tier_name,omitempty"`
	MinScore    *int    `json:"min_score,omitempty"`
	MaxScore    *int    `json:"max_score,omitempty"`
	TierOrder   *int    `json:"tier_order,omitempty"`
	Description *string `json:"description,omitempty"`
	ColorCode   *string `json:"color_code,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

// RiskTierConfigResponse represents the response containing risk tier config information
type RiskTierConfigResponse struct {
	ID          string    `json:"id"`
	TierName    string    `json:"tier_name"`
	MinScore    int       `json:"min_score"`
	MaxScore    int       `json:"max_score"`
	TierOrder   int       `json:"tier_order"`
	Description *string   `json:"description,omitempty"`
	ColorCode   *string   `json:"color_code,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	UpdatedBy   *string   `json:"updated_by,omitempty"`
}
