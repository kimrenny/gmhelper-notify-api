package direct

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var placeholderRegex = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

type RenderedEmail struct {
	Subject       string
	HTMLBody      string
	PlainTextBody string
}

// Render replaces {{variable}} placeholders with values from vars.
// If escapeHTML is true, the variable values are HTML-escaped to prevent injection.
// If a variable in the template is missing from vars, an error is returned.
func Render(templateText string, vars map[string]any, escapeHTML bool) (string, error) {
	var missingVars []string

	result := placeholderRegex.ReplaceAllStringFunc(templateText, func(match string) string {
		submatches := placeholderRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		varName := strings.TrimSpace(submatches[1])
		val, exists := lookupVar(vars, varName)
		if !exists {
			missingVars = append(missingVars, varName)
			return match
		}

		valStr := formatVarValue(val)
		if escapeHTML {
			return html.EscapeString(valStr)
		}
		return valStr
	})

	if len(missingVars) > 0 {
		return "", fmt.Errorf("%w: %s", ErrMissingVariable, strings.Join(missingVars, ", "))
	}

	return result, nil
}

func lookupVar(vars map[string]any, key string) (any, bool) {
	if vars == nil {
		return nil, false
	}
	val, ok := vars[key]
	return val, ok
}

func formatVarValue(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// RenderEmail renders the subject, HTML body, and optional plain-text body of a template using vars.
func RenderEmail(subjectTmpl, htmlBodyTmpl, plainTextBodyTmpl string, vars map[string]any) (*RenderedEmail, error) {
	subject, err := Render(subjectTmpl, vars, false)
	if err != nil {
		return nil, fmt.Errorf("failed to render subject: %w", err)
	}

	htmlBody, err := Render(htmlBodyTmpl, vars, true)
	if err != nil {
		return nil, fmt.Errorf("failed to render html body: %w", err)
	}

	var plainTextBody string
	if strings.TrimSpace(plainTextBodyTmpl) != "" {
		plainTextBody, err = Render(plainTextBodyTmpl, vars, false)
		if err != nil {
			return nil, fmt.Errorf("failed to render plain text body: %w", err)
		}
	}

	return &RenderedEmail{
		Subject:       subject,
		HTMLBody:      htmlBody,
		PlainTextBody: plainTextBody,
	}, nil
}
