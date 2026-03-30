package tools

import (
	"context"
	"testing"
)

func TestHtmlToText_Basic(t *testing.T) {
	html := `<html><head><title>Test</title></head>
<body>
<h1>Hello World</h1>
<p>This is a <b>test</b> paragraph.</p>
<p>Second paragraph.</p>
</body></html>`

	text := htmlToText(html)
	if text == "" {
		t.Fatal("expected non-empty text")
	}
	if !containsStr(text, "Hello World") {
		t.Error("missing heading text")
	}
	if !containsStr(text, "This is a test paragraph.") {
		t.Error("missing paragraph text")
	}
	if containsStr(text, "<") {
		t.Error("HTML tags not stripped")
	}
}

func TestHtmlToText_RemovesScriptAndStyle(t *testing.T) {
	html := `<html><body>
<script>var x = 1; alert("hi");</script>
<style>.foo { color: red; }</style>
<p>Visible content</p>
</body></html>`

	text := htmlToText(html)
	if containsStr(text, "alert") {
		t.Error("script content not removed")
	}
	if containsStr(text, "color") {
		t.Error("style content not removed")
	}
	if !containsStr(text, "Visible content") {
		t.Error("visible content missing")
	}
}

func TestHtmlToText_RemovesNavAndFooter(t *testing.T) {
	html := `<body>
<nav><a href="/">Home</a><a href="/about">About</a></nav>
<main><p>Main content here</p></main>
<footer><p>Copyright 2026</p></footer>
</body>`

	text := htmlToText(html)
	if containsStr(text, "Home") {
		t.Error("nav content not removed")
	}
	if containsStr(text, "Copyright") {
		t.Error("footer content not removed")
	}
	if !containsStr(text, "Main content here") {
		t.Error("main content missing")
	}
}

func TestHtmlToText_DecodesEntities(t *testing.T) {
	html := `<p>Tom &amp; Jerry &lt;3 &gt; &quot;friends&quot;</p>`
	text := htmlToText(html)
	if !containsStr(text, "Tom & Jerry <3 >") {
		t.Errorf("entities not decoded: %q", text)
	}
}

func TestHtmlToText_Empty(t *testing.T) {
	text := htmlToText("")
	if text != "" {
		t.Errorf("expected empty string, got %q", text)
	}
}

func TestHtmlToText_PlainText(t *testing.T) {
	text := htmlToText("Just plain text, no tags.")
	if text != "Just plain text, no tags." {
		t.Errorf("plain text mangled: %q", text)
	}
}

func TestHtmlToText_Truncation(t *testing.T) {
	// Test that the tool truncates at maxOutputChars
	// We test htmlToText directly — it doesn't truncate, the tool does.
	// Just verify htmlToText doesn't crash on large input.
	bigHTML := "<p>" + string(make([]byte, 100000)) + "</p>"
	text := htmlToText(bigHTML)
	if text == "" {
		// It's okay to be empty — null bytes produce empty text
		return
	}
}

func TestWebFetchTool_EmptyURL(t *testing.T) {
	tool := &WebFetchTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{}, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error for empty URL")
	}
}

func TestWebFetchTool_AddsHTTPS(t *testing.T) {
	// We can't easily test the full fetch without hitting the network,
	// but we can verify the tool doesn't crash on a bad domain.
	tool := &WebFetchTool{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel to avoid network call
	result, err := tool.Execute(ctx, map[string]interface{}{
		"url": "example.invalid",
	}, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should get a fetch error (cancelled context), not a URL parsing error
	if result.Error == "" {
		t.Error("expected error for cancelled fetch")
	}
}

func TestWebFetchTool_Schema(t *testing.T) {
	tool := &WebFetchTool{}
	if tool.Name() != "web_fetch" {
		t.Errorf("name = %q, want web_fetch", tool.Name())
	}
	if tool.Category() != CategoryReadOnly {
		t.Errorf("category = %v, want read_only", tool.Category())
	}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("missing properties in schema")
	}
	if _, ok := props["url"]; !ok {
		t.Error("missing 'url' in schema properties")
	}
}
