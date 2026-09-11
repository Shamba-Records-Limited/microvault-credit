package handlers

import (
	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v2"
)

// render writes a templ component as the response body.
func render(c *fiber.Ctx, component templ.Component) error {
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return component.Render(c.UserContext(), c.Response().BodyWriter())
}
