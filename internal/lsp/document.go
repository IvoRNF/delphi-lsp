package lsp

import (
	"regexp"
	"sort"
	"strings"
)

const (
	symbolModule   = 2
	symbolClass    = 5
	symbolMethod   = 6
	symbolProperty = 7
	symbolField    = 8
	symbolFunction = 12
	symbolVariable = 13
	symbolConstant = 14
)

type Symbol struct {
	Name           string
	Detail         string
	Documentation  string
	Owner          string
	Kind           int
	Range          Range
	Selection      Range
	Scope          Range
	Implementation bool
}

type Document struct {
	URI, Text   string
	Symbols     []Symbol
	Uses        []UnitReference
	Diagnostics []Diagnostic
}

// UnitReference is a unit name in a Delphi uses clause.
type UnitReference struct {
	Name  string
	Range Range
}

var declaration = regexp.MustCompile(`(?i)^\s*(procedure|function|constructor|destructor|type|var|const|property)\s+([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)(.*)$`)
var typedVariable = regexp.MustCompile(`(?i)^\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)\s*:\s*([^;]+);`)
var typeDefinition = regexp.MustCompile(`(?i)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(class|record|interface|dispinterface|object)\b`)
var typeAlias = regexp.MustCompile(`(?i)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+);`)
var unitHeader = regexp.MustCompile(`(?i)^\s*unit\s+([A-Za-z_][A-Za-z0-9_.]*)\s*;`)
var usesStart = regexp.MustCompile(`(?i)^\s*uses\b(.*)$`)
var unitName = regexp.MustCompile(`(?i)^\s*([A-Za-z_][A-Za-z0-9_.]*)`)

func Parse(uri, text string) *Document {
	document := &Document{URI: uri, Text: text}
	lines := strings.Split(text, "\n")
	active := []bool{true}
	inVarSection, routineBody := false, false
	inTypeSection := false
	inImplementation, inUses := false, false
	currentRoutine, currentTypeIndex, routineDepth := -1, -1, 0
	currentType := ""
	endPosition := func(line int) Position {
		if line < 0 {
			line = 0
		}
		return Position{Line: line, Character: len(lines[line])}
	}
	closeRoutine := func(line int) {
		if currentRoutine >= 0 {
			if line < document.Symbols[currentRoutine].Selection.Start.Line {
				line = document.Symbols[currentRoutine].Selection.Start.Line
			}
			document.Symbols[currentRoutine].Scope.End = endPosition(line)
		}
		currentRoutine, routineDepth, routineBody = -1, 0, false
	}
	closeType := func(line int) {
		if currentTypeIndex >= 0 {
			document.Symbols[currentTypeIndex].Scope.End = endPosition(line)
		}
		currentType, currentTypeIndex = "", -1
	}

	for lineNumber, line := range lines {
		trimmed, upper := strings.TrimSpace(line), strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(upper, "{$IFDEF ") || strings.HasPrefix(upper, "{$IFNDEF ") {
			active = append(active, active[len(active)-1])
			continue
		}
		if strings.HasPrefix(upper, "{$ELSE") && len(active) > 1 {
			active[len(active)-1] = !active[len(active)-2] && active[len(active)-1]
			continue
		}
		if strings.HasPrefix(upper, "{$ENDIF") && len(active) > 1 {
			active = active[:len(active)-1]
			continue
		}
		if !active[len(active)-1] {
			continue
		}
		if match := unitHeader.FindStringSubmatch(line); match != nil {
			start := strings.Index(strings.ToLower(line), strings.ToLower(match[1]))
			selection := Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: start + len(match[1])}}
			document.Symbols = append(document.Symbols, Symbol{Name: match[1], Detail: "unit " + match[1], Kind: symbolModule, Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Selection: selection})
			continue
		}
		if strings.EqualFold(trimmed, "implementation") {
			inImplementation, inUses, inTypeSection, inVarSection = true, false, false, false
			continue
		}
		if match := usesStart.FindStringSubmatch(line); match != nil {
			inUses = true
			addUses(document, match[1], lineNumber, len(line)-len(match[1]))
			if strings.Contains(match[1], ";") {
				inUses = false
			}
			continue
		}
		if inUses {
			addUses(document, line, lineNumber, 0)
			if strings.Contains(line, ";") {
				inUses = false
			}
			continue
		}

		if strings.EqualFold(trimmed, "type") {
			inTypeSection = true
			continue
		}
		if currentRoutine < 0 && currentType == "" && inTypeSection {
			if match := typeDefinition.FindStringSubmatch(line); match != nil {
				start := strings.Index(strings.ToLower(line), strings.ToLower(match[1]))
				selection := Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: start + len(match[1])}}
				document.Symbols = append(document.Symbols, Symbol{Name: match[1], Detail: match[1] + " = " + strings.ToLower(match[2]), Documentation: summaryBefore(lines, lineNumber), Kind: symbolClass, Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Selection: selection, Scope: Range{Start: selection.Start, End: endPosition(len(lines) - 1)}})
				currentType, currentTypeIndex, inVarSection = match[1], len(document.Symbols)-1, false
				continue
			}
		}
		if currentRoutine < 0 && currentType == "" && inTypeSection {
			if match := typeAlias.FindStringSubmatch(line); match != nil {
				start := strings.Index(strings.ToLower(line), strings.ToLower(match[1]))
				selection := Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: start + len(match[1])}}
				document.Symbols = append(document.Symbols, Symbol{Name: match[1], Detail: strings.TrimSpace(line), Documentation: summaryBefore(lines, lineNumber), Kind: symbolClass, Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Selection: selection})
				continue
			}
		}
		if currentRoutine < 0 && currentType != "" && strings.HasPrefix(strings.ToLower(trimmed), "end") {
			closeType(lineNumber)
			continue
		}
		if match := declaration.FindStringSubmatch(line); match != nil {
			word, rawName := strings.ToLower(match[1]), match[2]
			name := rawName
			if dot := strings.LastIndex(name, "."); dot >= 0 {
				name = name[dot+1:]
			}
			kind := symbolVariable
			switch word {
			case "function", "procedure", "constructor", "destructor":
				kind = symbolFunction
			case "type":
				kind = symbolClass
			case "const":
				kind = symbolConstant
			case "property":
				kind = symbolProperty
			}
			isRoutine := kind == symbolFunction && currentType == ""
			if currentType != "" && kind == symbolFunction {
				kind = symbolMethod
			}
			if isRoutine {
				closeRoutine(lineNumber - 1)
			}
			owner := ""
			if currentType != "" && (kind == symbolMethod || kind == symbolProperty) {
				owner = currentType
			}
			if strings.Contains(rawName, ".") && kind == symbolFunction {
				owner = strings.TrimSuffix(rawName, "."+name)
			}
			start := strings.Index(strings.ToLower(line), strings.ToLower(rawName))
			if start < 0 {
				start = 0
			}
			nameStart := start + strings.LastIndex(rawName, ".") + 1
			selection := Range{Start: Position{Line: lineNumber, Character: nameStart}, End: Position{Line: lineNumber, Character: nameStart + len(name)}}
			symbol := Symbol{Name: name, Detail: strings.TrimSpace(match[1] + " " + rawName + match[3]), Documentation: summaryBefore(lines, lineNumber), Owner: owner, Kind: kind, Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Selection: selection, Scope: Range{Start: selection.Start, End: endPosition(len(lines) - 1)}, Implementation: inImplementation && kind == symbolFunction}
			document.Symbols = append(document.Symbols, symbol)
			if word == "type" && (strings.Contains(strings.ToLower(match[3]), "= class") || strings.Contains(strings.ToLower(match[3]), "= record")) {
				closeType(lineNumber - 1)
				currentType, currentTypeIndex = name, len(document.Symbols)-1
				inVarSection = false
				continue
			}
			if isRoutine {
				currentRoutine = len(document.Symbols) - 1
				addParameters(document, line, lineNumber, name)
				inVarSection = false
			} else if word == "var" {
				inVarSection = true
			}
			continue
		}

		if currentRoutine >= 0 {
			if strings.EqualFold(trimmed, "var") {
				inVarSection = true
				continue
			}
			if inVarSection && addTypedVariables(document, line, lineNumber, document.Symbols[currentRoutine].Name) {
				continue
			}
			lower := strings.ToLower(trimmed)
			if strings.Contains(lower, "begin") {
				routineBody = true
				routineDepth += strings.Count(lower, "begin")
				inVarSection = false
			}
			if routineBody && strings.HasPrefix(lower, "end") {
				routineDepth--
				if routineDepth <= 0 {
					closeRoutine(lineNumber)
				}
			}
		} else if currentType != "" {
			if addTypedVariables(document, line, lineNumber, currentType) {
				continue
			}
		} else if inVarSection {
			if addTypedVariables(document, line, lineNumber, "") {
				continue
			}
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				inVarSection = false
			}
		}
		if strings.Contains(upper, "TODO") {
			document.Diagnostics = append(document.Diagnostics, Diagnostic{Range: Range{Start: Position{Line: lineNumber}, End: endPosition(lineNumber)}, Severity: 3, Source: "delphi-lsp", Message: "TODO"})
		}
	}
	closeRoutine(len(lines) - 1)
	closeType(len(lines) - 1)
	if len(active) != 1 {
		document.Diagnostics = append(document.Diagnostics, Diagnostic{Severity: 1, Source: "delphi-lsp", Message: "Unclosed compiler directive ({$IFDEF / {$IFNDEF)"})
	}
	sort.SliceStable(document.Symbols, func(i, j int) bool { return document.Symbols[i].Name < document.Symbols[j].Name })
	return document
}

func addUses(document *Document, text string, lineNumber, offset int) {
	cursor := 0
	for _, part := range strings.Split(text, ",") {
		if semicolon := strings.Index(part, ";"); semicolon >= 0 {
			part = part[:semicolon]
		}
		match := unitName.FindStringSubmatch(part)
		if match != nil {
			start := strings.Index(strings.ToLower(part), strings.ToLower(match[1]))
			character := offset + cursor + start
			document.Uses = append(document.Uses, UnitReference{Name: match[1], Range: Range{Start: Position{Line: lineNumber, Character: character}, End: Position{Line: lineNumber, Character: character + len(match[1])}}})
		}
		cursor += len(part) + 1
	}
}

func addTypedVariables(document *Document, line string, lineNumber int, owner string) bool {
	match := typedVariable.FindStringSubmatch(line)
	if match == nil {
		return false
	}
	kind := symbolVariable
	if owner != "" {
		kind = symbolField
	}
	for _, name := range strings.Split(match[1], ",") {
		name = strings.TrimSpace(name)
		start := strings.Index(strings.ToLower(line), strings.ToLower(name))
		selection := Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: start + len(name)}}
		document.Symbols = append(document.Symbols, Symbol{Name: name, Detail: name + ": " + strings.TrimSpace(match[2]), Owner: owner, Kind: kind, Range: Range{Start: Position{Line: lineNumber}, End: Position{Line: lineNumber, Character: len(line)}}, Selection: selection})
	}
	return true
}

func addParameters(document *Document, line string, lineNumber int, owner string) {
	open, close := strings.Index(line, "("), strings.LastIndex(line, ")")
	if open < 0 || close <= open {
		return
	}
	for _, group := range strings.Split(line[open+1:close], ";") {
		group = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(group, "const "), "var "), "out "), "constref "))
		parts := strings.SplitN(group, ":", 2)
		if len(parts) != 2 {
			continue
		}
		for _, name := range strings.Split(parts[0], ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			start := strings.Index(strings.ToLower(line), strings.ToLower(name))
			selection := Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: start + len(name)}}
			document.Symbols = append(document.Symbols, Symbol{Name: name, Detail: strings.TrimSpace(group), Owner: owner, Kind: symbolVariable, Range: Range{Start: Position{Line: lineNumber}, End: Position{Line: lineNumber, Character: len(line)}}, Selection: selection})
		}
	}
}

func wordAt(text string, position Position) string {
	lines := strings.Split(text, "\n")
	if position.Line < 0 || position.Line >= len(lines) {
		return ""
	}
	line := lines[position.Line]
	if position.Character > len(line) {
		position.Character = len(line)
	}
	start, end := position.Character, position.Character
	isWord := func(character byte) bool {
		return character == '_' || character >= '0' && character <= '9' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
	}
	for start > 0 && isWord(line[start-1]) {
		start--
	}
	for end < len(line) && isWord(line[end]) {
		end++
	}
	return line[start:end]
}
