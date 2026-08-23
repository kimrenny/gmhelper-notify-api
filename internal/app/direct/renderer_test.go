package direct

import (
	"errors"
	"testing"
)

func TestRender_SimpleVariables(t *testing.T) {
	tmpl := "Hello, {{userName}}! Your code is {{code}}."
	vars := map[string]any{
		"userName": "Renny",
		"code":     123456,
	}

	result, err := Render(tmpl, vars, false)
	if err != nil {
		t.Fatalf("expected render success, got %v", err)
	}

	expected := "Hello, Renny! Your code is 123456."
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestRender_RepeatedVariables(t *testing.T) {
	tmpl := "Hello, {{name}}! Once again, welcome {{name}}!"
	vars := map[string]any{
		"name": "Alex",
	}

	result, err := Render(tmpl, vars, false)
	if err != nil {
		t.Fatalf("expected render success, got %v", err)
	}

	expected := "Hello, Alex! Once again, welcome Alex!"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestRender_MissingVariableError(t *testing.T) {
	tmpl := "Hello, {{firstName}} {{lastName}}!"
	vars := map[string]any{
		"firstName": "John",
	}

	_, err := Render(tmpl, vars, false)
	if err == nil {
		t.Fatal("expected error on missing variable, got nil")
	}

	if !errors.Is(err, ErrMissingVariable) {
		t.Errorf("expected ErrMissingVariable, got %v", err)
	}
}

func TestRender_HTMLEscaping(t *testing.T) {
	tmpl := "<p>Welcome, {{userName}}! Click <a href=\"{{link}}\">here</a>.</p>"
	vars := map[string]any{
		"userName": "<script>alert('xss')</script>",
		"link":     "https://example.com/verify?a=1&b=2",
	}

	result, err := Render(tmpl, vars, true)
	if err != nil {
		t.Fatalf("expected render success, got %v", err)
	}

	expected := "<p>Welcome, &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;! Click <a href=\"https://example.com/verify?a=1&amp;b=2\">here</a>.</p>"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestRender_PlainTextNoEscaping(t *testing.T) {
	tmpl := "Hello {{name}} & welcome to {{company}}!"
	vars := map[string]any{
		"name":    "Renny <GM>",
		"company": "Rock & Roll",
	}

	result, err := Render(tmpl, vars, false)
	if err != nil {
		t.Fatalf("expected render success, got %v", err)
	}

	expected := "Hello Renny <GM> & welcome to Rock & Roll!"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestRender_SpacingInsidePlaceholders(t *testing.T) {
	tmpl := "Hello {{  user_name  }}! Welcome to {{ app.name }}."
	vars := map[string]any{
		"user_name": "Sam",
		"app.name":  "GMHelper",
	}

	result, err := Render(tmpl, vars, false)
	if err != nil {
		t.Fatalf("expected render success, got %v", err)
	}

	expected := "Hello Sam! Welcome to GMHelper."
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestRenderEmail_Full(t *testing.T) {
	subjectTmpl := "Welcome {{name}}!"
	htmlTmpl := "<h1>Hello {{name}}</h1><p>Your pin is {{pin}}</p>"
	plainTmpl := "Hello {{name}}, your pin is {{pin}}"

	vars := map[string]any{
		"name": "Tom & Jerry",
		"pin":  9988,
	}

	rendered, err := RenderEmail(subjectTmpl, htmlTmpl, plainTmpl, vars)
	if err != nil {
		t.Fatalf("expected render success, got %v", err)
	}

	if rendered.Subject != "Welcome Tom & Jerry!" {
		t.Errorf("unexpected subject: %s", rendered.Subject)
	}
	if rendered.HTMLBody != "<h1>Hello Tom &amp; Jerry</h1><p>Your pin is 9988</p>" {
		t.Errorf("unexpected html body: %s", rendered.HTMLBody)
	}
	if rendered.PlainTextBody != "Hello Tom & Jerry, your pin is 9988" {
		t.Errorf("unexpected plain text body: %s", rendered.PlainTextBody)
	}
}
