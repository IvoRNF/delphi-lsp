package lsp

import "testing"

func TestParameterSeparators(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   int
	}{
		{"procedure P(A: Integer B: String);", 1},
		{"function F(A: Integer\nconst B: String): Boolean;", 1},
		{"procedure P(A: Integer, B: String);", 1},
		{"procedure P(A: Integer B: String C: Boolean);", 2},
		{"procedure P(const Buffer out Count: Integer);", 1},
		{"procedure P(A: String = 'a; b' B: Integer = 1);", 1},
		{"type T = procedure(A: Integer B: Integer) of object;", 1},
		{"procedure P(Callback: procedure(A: Integer B: Integer));", 1},
		{"procedure P(A, B: Integer; const C: String; var D: Boolean);", 0},
		{"procedure P; begin Call(A, B); end;", 0},
		{"function T.F<T>(A: TDictionary<string, TList<T>>; B: T): T;", 0},
		{"procedure P(const Buffer; var Target; Count: Integer);", 0},
		{"procedure P(A: array of const; B: array of Integer);", 0},
		{"procedure P(Callback: function(A: Integer; B: String): Boolean; X: Integer);", 0},
		{"procedure P(A: String = 'it''s; ok'; B: Integer = 2);", 0},
		{"procedure P([Ref] const A: String; B: Integer);", 0},
		{"procedure P();", 0},
	} {
		got := parameterDiagnostics(tt.source)
		if len(got) != tt.want {
			t.Errorf("%s: got %+v, want %d", tt.source, got, tt.want)
		}
	}
}

func TestUnterminatedStrings(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   int
	}{
		{"S := 'hello", 1},
		{"S := 'hello\r\nNext;", 1},
		{"S := 'it''s", 1},
		{"S := 'bad\nT := 'also bad", 2},
		{"S := ''; T := ''''; U := 'it''s fine';", 0},
		{"// 'ignored\n{ 'ignored } (* 'ignored *)", 0},
		{"{$IFDEF X} S := 'ok'; {$ELSE}\nS := 'bad\n{$ENDIF}", 0},
		{"S := '''\nmultiple 'quoted' lines\n''';", 0},
		{"S := '''\nnot closed", 1},
	} {
		_, got := scanSyntax(tt.source)
		if len(got) != tt.want {
			t.Errorf("%s: got %+v, want %d", tt.source, got, tt.want)
		}
	}
	tokens, diagnostics := scanSyntax("S := '😀\r\nNext;")
	if len(diagnostics) != 1 || diagnostics[0].Range != (Range{Start: Position{Character: 5}, End: Position{Character: 8}}) {
		t.Fatalf("UTF-16 range: %+v", diagnostics)
	}
	if tokens[len(tokens)-2].text != "next" {
		t.Fatalf("lexer did not recover after newline: %+v", tokens)
	}
}

func TestStringAndParameterDiagnosticsIntegration(t *testing.T) {
	d := Parse("file:///U.pas", "unit U; interface procedure P(A: Integer B: String); implementation end.")
	if len(d.Diagnostics) != 1 || d.Diagnostics[0].Message != "Missing ';' between parameter groups" {
		t.Fatalf("diagnostics: %+v", d.Diagnostics)
	}
	d = Parse("file:///U.pas", "unit U; interface const S = 'bad\n; implementation end.")
	if len(d.Diagnostics) != 1 || d.Diagnostics[0].Message != "Unterminated string literal: expected closing apostrophe" {
		t.Fatalf("diagnostics: %+v", d.Diagnostics)
	}
}
