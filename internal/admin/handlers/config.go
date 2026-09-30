package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	globallendinglimit "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/global_lending_limit"
)

// Config lists and creates global lending limits.
type Config struct {
	limits globallendinglimit.Service
}

func NewConfig(limits globallendinglimit.Service) *Config {
	return &Config{limits: limits}
}

func (h *Config) List(c *fiber.Ctx) error {
	page := views.ConfigPage{
		Page:     queryInt(c, "page", 1),
		PageSize: defaultPageSize,
		Flash:    c.Query("flash"),
		ShowForm: c.Query("new") != "",
		Form:     views.DefaultConfigForm(),
	}

	if limits, err := h.load(c, page.Page); err != nil {
		page.Error = "Could not load global lending limits: " + err.Error()
	} else {
		page.Limits = limits
	}

	return render(c, views.Config(page))
}

func (h *Config) Create(c *fiber.Ctx) error {
	form := views.ConfigForm{
		ConfigKey:   strings.TrimSpace(c.FormValue("config_key")),
		ConfigValue: strings.TrimSpace(c.FormValue("config_value")),
		ValueType:   c.FormValue("value_type"),
		Description: strings.TrimSpace(c.FormValue("description")),
		Category:    c.FormValue("category"),
		IsActive:    c.FormValue("is_active") == "true",
	}

	req, err := buildConfigRequest(form)
	if err != nil {
		return h.redisplay(c, form, err.Error())
	}

	if _, err := h.limits.Create(c.UserContext(), req); err != nil {
		return h.redisplay(c, form, err.Error())
	}

	return c.Redirect("/config?flash=Setting+created", fiber.StatusSeeOther)
}

func (h *Config) load(c *fiber.Ctx, page int) ([]globallendinglimit.GlobalLendingLimitResponse, error) {
	result, err := h.limits.GetAll(c.UserContext(), services.Pagination{Page: page, PageSize: defaultPageSize})
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (h *Config) redisplay(c *fiber.Ctx, form views.ConfigForm, message string) error {
	page := views.ConfigPage{
		Page:     1,
		PageSize: defaultPageSize,
		Error:    message,
		ShowForm: true,
		Form:     form,
	}
	if limits, err := h.load(c, 1); err == nil {
		page.Limits = limits
	}

	c.Status(fiber.StatusUnprocessableEntity)
	return render(c, views.Config(page))
}

func buildConfigRequest(f views.ConfigForm) (globallendinglimit.CreateGlobalLendingLimitRequest, error) {
	var req globallendinglimit.CreateGlobalLendingLimitRequest

	if f.ConfigKey == "" {
		return req, fiber.NewError(fiber.StatusBadRequest, "key is required")
	}
	if f.ConfigValue == "" {
		return req, fiber.NewError(fiber.StatusBadRequest, "value is required")
	}
	if err := checkValueType(f.ValueType, f.ConfigValue); err != nil {
		return req, err
	}

	req = globallendinglimit.CreateGlobalLendingLimitRequest{
		ConfigKey:   f.ConfigKey,
		ConfigValue: f.ConfigValue,
		ValueType:   f.ValueType,
		Description: optionalString(f.Description),
		Category:    optionalString(f.Category),
		IsActive:    f.IsActive,
	}

	return req, nil
}

// checkValueType rejects a value that does not parse as its declared type, so a
// typo cannot land a value the lending logic will fail to read later.
func checkValueType(valueType, value string) error {
	switch valueType {
	case "integer":
		if _, err := parseInt64(value, "value"); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "value must be a whole number for type integer")
		}
	case "decimal":
		if _, err := parseFloat(value); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "value must be a decimal number for type decimal")
		}
	case "boolean":
		if value != "true" && value != "false" {
			return fiber.NewError(fiber.StatusBadRequest, "value must be true or false for type boolean")
		}
	case "string":
	default:
		return fiber.NewError(fiber.StatusBadRequest, "unknown value type")
	}
	return nil
}
