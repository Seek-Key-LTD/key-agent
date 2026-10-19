// Copyright 2026 Google LLC
//
// Tests for Intent Router and OTP extractor

package main

import (
	"testing"
)

func TestExtractOTPCode(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"WhatsApp 验证码：407-704", "407-704"},
		{"Your WhatsApp code is 407-704", "407-704"},
		{"【验证码】892301 请于5分钟内输入", "892301"},
		{"Verification code: 123456", "123456"},
		{"OTP: 789012", "789012"},
		{"No code here in this regular text", ""},
	}

	for _, c := range cases {
		got := extractOTPCode(c.input)
		if got != c.expected {
			t.Errorf("extractOTPCode(%q) = %q; want %q", c.input, got, c.expected)
		}
	}
}

func TestStripHTML(t *testing.T) {
	input := `<p><span class="h-card"><a href="https://flowers.capitaltrain.cn/@moli" class="u-url mention">@<span>moli</span></a></span> 查一下你的验证码是多少</p>`
	expected := `@moli 查一下你的验证码是多少`
	got := stripHTML(input)
	if got != expected {
		t.Errorf("stripHTML(%q) = %q; want %q", input, got, expected)
	}
}
